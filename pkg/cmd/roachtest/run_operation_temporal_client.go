package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/operations"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
	"github.com/cockroachdb/cockroach/pkg/roachprod"
	"github.com/cockroachdb/cockroach/pkg/roachprod/logger"
	"go.temporal.io/sdk/client"
)

// submitOperationRequest submits a request to execute operations matching the given regex
// on the specified cluster. It signals a running workflow to execute the operations.
// If operationRegex is empty, it uses OpenAI to process the request and determine the appropriate operations.
// The userInput parameter is used as the prompt for the AI agent when operationRegex is empty.
func submitOperationRequest(
	register func(registry.Registry), operationRegex, clusterName, userInput string,
) error {
	r := makeTestRegistry()
	register(&r)

	// Validate that the cluster exists and is loaded
	if err := roachprod.LoadClusters(); err != nil {
		return err
	}
	//_, err := getCachedCluster(clusterName)
	//if err != nil {
	//	return err
	//}

	l, err := logger.RootLogger("", logger.NoTee)
	if err != nil {
		return err
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{})
	if err != nil {
		return err
	}
	defer c.Close()

	// Check if the workflow is running
	//_, err = c.DescribeWorkflowExecution(context.Background(), getWorkflowID(clusterName), "")
	//if err != nil {
	//	return fmt.Errorf("workflow not running: %w. Please start the workflow server first using run-operation-temporal-server", err)
	//}

	// Create the request
	var request ExecuteOperationRequest

	// If operationRegex is empty, use AI agent to process the request
	if operationRegex == "" {
		l.Printf("No operation regex provided, using AI agent to process request")

		// Create AI agent
		agent, err := NewAIAgent(l)
		if err != nil {
			return fmt.Errorf("failed to create AI agent: %w", err)
		}

		// Register all operations to get a complete list
		operationsRegistry := makeTestRegistry()
		operations.RegisterOperations(operationsRegistry)

		// Get all available operations
		allOps := operationsRegistry.AllOperations()

		// Use the provided user input or a default prompt
		promptText := userInput
		if promptText == "" {
			promptText = "I want to add an index to the database"
		}
		l.Printf("Using prompt for AI agent: %s", promptText)

		// Process the request using the AI agent
		aiRequest, err := agent.ProcessOperationRequest(context.Background(), promptText, allOps)
		if err != nil {
			return fmt.Errorf("AI agent failed to process request: %w", err)
		}

		request = *aiRequest
		l.Printf("AI agent generated operation regex: %s", request.OperationRegex)
	} else {
		// Use the provided operation regex
		request = ExecuteOperationRequest{
			OperationRegex: operationRegex,
			MaxRepetitions: 0, // Default to unlimited repetitions
		}
	}
	// Convert the request to JSON for better readability
	jsonBytes, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal request to JSON: %w", err)
	}

	// Display the JSON and ask for confirmation
	fmt.Printf("Generated JSON request:\n%s\n", string(jsonBytes))

	if !promptConfirmation("Do you want to submit this request?") {
		l.Printf("Request submission cancelled by user")
		return nil
	}

	// Signal the workflow to execute the operation
	err = c.SignalWorkflow(context.Background(), getWorkflowID(clusterName), "", ExecuteOperationSignal, request)
	if err != nil {
		return fmt.Errorf("failed to signal workflow: %w", err)
	}

	l.Printf("Successfully submitted request to execute operations matching '%s' on cluster '%s'", request.OperationRegex, clusterName)
	return nil
}

func getWorkflowID(clusterName string) string {
	return operationsWorkflowID + "-" + clusterName
}

// promptConfirmation asks the user to confirm the action with a yes/no prompt
func promptConfirmation(msg string) bool {
	fmt.Printf("%s (y/n): ", msg)

	var answer string
	_, _ = fmt.Scanln(&answer)
	answer = strings.TrimSpace(answer)

	return answer == "y" || answer == "Y"
}
