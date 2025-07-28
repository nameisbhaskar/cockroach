package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
	"github.com/cockroachdb/cockroach/pkg/roachprod/logger"
	"github.com/cockroachdb/errors"
	"github.com/sashabaranov/go-openai"
)

const (
	// OpenAIAPIKeyEnv is the environment variable name for the OpenAI API key
	OpenAIAPIKeyEnv = "OPENAI_API_KEY"
	// DefaultOpenAIModel is the default OpenAI model to use
	DefaultOpenAIModel = "gpt-4o"
)

// AIAgent is responsible for processing operation requests using OpenAI
type AIAgent struct {
	client *openai.Client
	model  string
	logger *logger.Logger
}

// NewAIAgent creates a new AIAgent with the given configuration
func NewAIAgent(l *logger.Logger) (*AIAgent, error) {
	apiKey := os.Getenv(OpenAIAPIKeyEnv)

	// Create OpenAI client configuration
	config := openai.DefaultConfig(apiKey)

	// Create the OpenAI client
	client := openai.NewClientWithConfig(config)

	return &AIAgent{
		client: client,
		model:  DefaultOpenAIModel,
		logger: l,
	}, nil
}

// ProcessOperationRequest processes an operation request using OpenAI
func (a *AIAgent) ProcessOperationRequest(
	ctx context.Context, userInput string, availableOperations []registry.OperationSpec,
) (*ExecuteOperationRequest, error) {
	// Create a list of available operations for the prompt
	var operationsList strings.Builder
	for i, op := range availableOperations {
		operationsList.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, op.Name, getOperationDescription(op)))
	}

	// Create the prompt for OpenAI
	systemPrompt := "You are a helpful assistant that generates JSON responses based on user requests."

	userPrompt := fmt.Sprintf(`You are an AI assistant for CockroachDB operations. 
Your task is to help the user execute operations on a CockroachDB cluster.

Available operations:
%s

User request: %s

Based on the user's request, determine which operation(s) they want to run.
Respond with a JSON object containing:
1. "operationRegex": A regex pattern that matches the operation(s) the user wants to run
3. "runInInterval": (Optional) Duration to wait before running the operation again (e.g., "10m", "1h"). This must be set only if the operation should be repeated. e.g. run x operation every 30 minutes must set "30m" as value. The default is 0.
4. "maxRepetitions": (Optional) Maximum number of times to repeat the operation (0 = unlimited, negative = don't repeat). The default value is -1.
Note: Set the default value of "runInInterval" to 0 and "maxRepetitions" to -1.

Example response:
{
  "operationRegex": "add-index",
  "runInInterval": "30m",
  "maxRepetitions": 5
}

Only respond with the JSON object, no additional text.`, operationsList.String(), userInput)

	// Create the OpenAI request
	a.logger.Printf("Sending request to OpenAI API")
	resp, err := a.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: a.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: userPrompt,
				},
			},
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request to OpenAI API")
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI API returned empty choices")
	}

	// Extract the JSON response
	jsonResponse := resp.Choices[0].Message.Content
	jsonResponse = strings.TrimSpace(jsonResponse)

	// If the response is wrapped in code blocks, extract the JSON
	if strings.HasPrefix(jsonResponse, "```json") {
		jsonResponse = strings.TrimPrefix(jsonResponse, "```json")
		jsonResponse = strings.TrimSuffix(jsonResponse, "```")
		jsonResponse = strings.TrimSpace(jsonResponse)
	} else if strings.HasPrefix(jsonResponse, "```") {
		jsonResponse = strings.TrimPrefix(jsonResponse, "```")
		jsonResponse = strings.TrimSuffix(jsonResponse, "```")
		jsonResponse = strings.TrimSpace(jsonResponse)
	}

	// Create a custom struct for unmarshaling the JSON response
	type jsonRequest struct {
		OperationRegex        string `json:"operationRegex"`
		TerminateOnCompletion bool   `json:"terminateOnCompletion"`
		RunInInterval         string `json:"runInInterval"`
		MaxRepetitions        int    `json:"maxRepetitions"`
	}

	// Parse the JSON response into the custom struct
	var jsonReq jsonRequest
	if err := json.Unmarshal([]byte(jsonResponse), &jsonReq); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal operation request")
	}

	// Create the ExecuteOperationRequest
	request := ExecuteOperationRequest{
		OperationRegex: jsonReq.OperationRegex,
		MaxRepetitions: jsonReq.MaxRepetitions,
	}

	// Convert RunInInterval from string to time.Duration if it's not empty
	if jsonReq.RunInInterval != "" {
		duration, err := time.ParseDuration(jsonReq.RunInInterval)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse runInInterval as duration")
		}
		request.RunInInterval = duration
	}

	a.logger.Printf("AI agent generated operation request: %+v", request)
	return &request, nil
}

// getOperationDescription returns a description of the operation based on its properties
func getOperationDescription(op registry.OperationSpec) string {
	var description strings.Builder

	// Add concurrency info
	switch op.CanRunConcurrently {
	case registry.OperationCanRunConcurrently:
		description.WriteString(" (can run concurrently)")
	case registry.OperationCannotRunConcurrentlyWithItself:
		description.WriteString(" (cannot run concurrently with itself)")
	case registry.OperationCannotRunConcurrently:
		description.WriteString(" (cannot run concurrently with any operation)")
	}

	return description.String()
}
