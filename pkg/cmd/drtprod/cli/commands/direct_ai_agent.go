// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/slack-go/slack"
)

const (
	// OpenAIAPIKeyEnv is the environment variable name for the OpenAI API key
	OpenAIAPIKeyEnv = "OPENAI_API_KEY"
	// DefaultOpenAIModel is the default OpenAI model to use
	DefaultOpenAIModel = "gpt-4o"
)

// DirectAIAgent is responsible for processing operation requests using OpenAI
// without relying on Temporal client code
type DirectAIAgent struct {
	client *openai.Client
	model  string
}

// ClarificationContext tracks the context of a clarification question
type ClarificationContext struct {
	OriginalInput string                  // The original user input that triggered the clarification
	Question      string                  // The clarification question that was asked
	ThreadTS      string                  // The thread timestamp for the conversation
	ChannelID     string                  // The channel ID where the conversation is happening
	Timestamp     time.Time               // When the clarification was requested
	History       []ClarificationExchange // History of clarification exchanges
}

// ClarificationExchange represents a single exchange of clarification question and answer
type ClarificationExchange struct {
	Question  string    // The clarification question that was asked
	Answer    string    // The answer provided by the user
	Timestamp time.Time // When the exchange occurred
}

// Map to store clarification contexts
var (
	clarificationContexts     = make(map[string]*ClarificationContext)
	clarificationContextsLock sync.Mutex
)

// NewDirectAIAgent creates a new DirectAIAgent with the given configuration
func NewDirectAIAgent() (*DirectAIAgent, error) {
	apiKey := os.Getenv(OpenAIAPIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not found in environment variable %s", OpenAIAPIKeyEnv)
	}

	// Create OpenAI client configuration
	config := openai.DefaultConfig(apiKey)

	// Create the OpenAI client
	client := openai.NewClientWithConfig(config)

	return &DirectAIAgent{
		client: client,
		model:  DefaultOpenAIModel,
	}, nil
}

// GenerateShellCommand generates a shell command based on the user input
// If there's any ambiguity in the cluster name, operation name, or database name,
// it will ask the user for clarification via Slack.
// If clarificationResponse is provided, it will be used to generate the command.
func (a *DirectAIAgent) GenerateShellCommand(
	ctx context.Context,
	userInput string,
	slackClient *slack.Client,
	channelID, threadTS string,
	clarificationResponse string,
) (string, error) {
	log.Printf("Generating shell command for user input: %s", userInput)

	// Create the prompt for OpenAI
	systemPrompt := "You are a helpful assistant that generates shell commands for CockroachDB roachtest operations and roachprod commands. You must ensure that the cluster name, operation name, and database name (if applicable) are clearly specified in the user's request. If there's any ambiguity or missing information, you must ask for clarification."

	// Format the clarification history if available
	clarificationText := ""

	// Check if we have a context for this thread
	clarificationContextsLock.Lock()
	previousContext, exists := clarificationContexts[threadTS]
	clarificationContextsLock.Unlock()

	if exists && len(previousContext.History) > 0 {
		// Format the history of clarification exchanges
		clarificationText = "\nClarification history:"
		for i, exchange := range previousContext.History {
			if exchange.Question != "" {
				clarificationText += fmt.Sprintf("\n- Question %d: %s", i+1, exchange.Question)
			}
			if exchange.Answer != "" {
				clarificationText += fmt.Sprintf("\n  Answer %d: %s", i+1, exchange.Answer)
			}
		}
		log.Printf("Using clarification history with %d exchanges", len(previousContext.History))
	} else if clarificationResponse != "" {
		// If no history but we have a single response, use it
		log.Printf("Using clarification response: %s", clarificationResponse)
		clarificationText = fmt.Sprintf("\nUser clarification: %s", clarificationResponse)
	}

	userPrompt := fmt.Sprintf(`You are an AI assistant for CockroachDB that helps users with the following tasks:
1. Running a roachtest operation
2. Running a command on a CockroachDB cluster
3. Changing a CockroachDB cluster configuration. system settings or configurations also mean the same.

Your task is to help the user by generating the appropriate shell command based on their request.

User request: %s%s

Based on the user's request, generate a valid shell command that can be executed directly.

For running a roachtest operation, extract the following information:
1. The cluster name to run the operation on. This can be a mentioned as "run a <operation name> operation on <cluster name>", without using the "cluster" word. 
2. The operation to run (can be a regular expression or a specific operation name)
3. The database name to run the operation on (if applicable and specified)

Example roachtest operation command:
- roachtest run-operation <cluster name> <operation> --wait-before-cleanup 1m --certs-dir ./certs --db <database name>
Note: The --db parameter is optional and must be omitted if the database name is not specified by the user.
Note: <cluster name>, <operation>, and <database name> are just placeholders and MUST be replaced with the actual values explicitly mentioned in the user request. DO NOT make any assumptions or use default values if the information is not clearly provided by the user.

For running a command on a cluster, extract the following information:
1. The cluster name to run the command on
2. The node number (if specified)
3. The exact command to run (must be a valid shell command)

Example command format:
- With node number: roachprod run <cluster name>:<node number> --secure -- "<command to run>"
- Without node number: roachprod run <cluster name> --secure -- "<command to run>"
Note: <cluster name>, <node number>, and <command to run> are just placeholders and MUST be replaced with the actual values explicitly mentioned in the user request. DO NOT make any assumptions or use default values if the information is not clearly provided by the user.

For changing a CockroachDB cluster configuration, extract the following information:
1. The cluster name to change the configuration on
2. The system setting to change and its new value

Example configuration commands:
- To set a value: roachprod sql <cluster name>:1 --secure -- -e "SET CLUSTER SETTING <setting name>=<new value>"
- To reset a setting: roachprod sql <cluster name>:1 --secure -- -e "RESET CLUSTER SETTING <setting name>"
Note: <cluster name>, <setting name>, and <new value> are just placeholders and MUST be replaced with the actual values explicitly mentioned in the user request. DO NOT make any assumptions or use default values if the information is not clearly provided by the user.

IMPORTANT: If there is ANY ambiguity or uncertainty about the cluster name, operation name, or database name, DO NOT generate a command. DO NOT make assumptions or use default values for any required information. Instead, respond with a question asking for clarification. Your response should start with "CLARIFICATION NEEDED:" followed by your specific question.

Examples of clarification responses:
- If the request is unclear: "CLARIFICATION NEEDED: Could you please clarify what you want to do - run a roachtest operation, run a command on the cluster, or change a cluster configuration?"
- If the cluster name is unclear: "CLARIFICATION NEEDED: Which cluster would you like to run the operation on?"
- If the operation is unclear: "CLARIFICATION NEEDED: Which operation would you like to run on the cluster?"
- If multiple interpretations are possible: "CLARIFICATION NEEDED: I'm not sure if you want to run operation X or operation Y on cluster Z. Could you please clarify?"

Only if all required information is clear and unambiguous, respond with just the shell command and nothing else.
IMPORTANT: respond with just the shell command in plain text and nothing else. Do not even add bash or sh.`, userInput, clarificationText)

	// Create the OpenAI request
	log.Printf("Sending request to OpenAI API")
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
		return "", fmt.Errorf("failed to send request to OpenAI API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", nil
	}

	// Extract the response
	aiResponse := resp.Choices[0].Message.Content
	aiResponse = strings.TrimSpace(aiResponse)

	// Check if clarification is needed
	if strings.HasPrefix(aiResponse, "CLARIFICATION NEEDED:") {
		// Send the clarification question back to the user via Slack
		if slackClient != nil {
			clarificationQuestion := aiResponse
			_, _, err := slackClient.PostMessage(
				channelID,
				slack.MsgOptionText(clarificationQuestion, false),
				slack.MsgOptionTS(threadTS),
			)
			if err != nil {
				log.Printf("Error sending clarification question to Slack: %v", err)
			} else {
				// Store the clarification operationContext
				clarificationContextsLock.Lock()
				defer clarificationContextsLock.Unlock()

				// Check if we already have a operationContext for this thread
				if operationContext, exists := clarificationContexts[threadTS]; exists {
					// Check if the current question has an answer
					currentQuestionAnswered := false
					for i := len(operationContext.History) - 1; i >= 0; i-- {
						if operationContext.History[i].Question == operationContext.Question {
							currentQuestionAnswered = operationContext.History[i].Answer != ""
							break
						}
					}

					// If the current question doesn't exist in history or has an answer, add the new question
					if currentQuestionAnswered || operationContext.Question == "" {
						operationContext.History = append(operationContext.History, ClarificationExchange{
							Question:  clarificationQuestion,
							Timestamp: time.Now(),
						})
					} else {
						// If the current question exists but doesn't have an answer, don't add a new entry
						// Just update the question in case it changed slightly
						for i := len(operationContext.History) - 1; i >= 0; i-- {
							if operationContext.History[i].Question == operationContext.Question && operationContext.History[i].Answer == "" {
								operationContext.History[i].Question = clarificationQuestion
								operationContext.History[i].Timestamp = time.Now()
								break
							}
						}
					}

					// Update the current question
					operationContext.Question = clarificationQuestion
					operationContext.Timestamp = time.Now()
				} else {
					// Create a new operationContext
					clarificationContexts[threadTS] = &ClarificationContext{
						OriginalInput: userInput,
						Question:      clarificationQuestion,
						ThreadTS:      threadTS,
						ChannelID:     channelID,
						Timestamp:     time.Now(),
						History:       []ClarificationExchange{},
					}
				}
				log.Printf("Stored clarification operationContext for thread %s", threadTS)
			}
		}
		// Return empty string to indicate that no command was generated
		return "", nil
	}

	log.Printf("Generated shell command: %s", aiResponse)
	return aiResponse, nil
}
