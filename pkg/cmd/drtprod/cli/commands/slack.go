// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"go.temporal.io/sdk/client"
)

// SlackConfig holds the configuration for Slack integration
type SlackConfig struct {
	Token    string // Slack bot token
	Channel  string // Slack channel to post messages to
	AppToken string // Slack app-level token for Socket Mode
	Enabled  bool   // Whether Slack integration is enabled
}

// PendingCommand holds information about a command waiting for confirmation
type PendingCommand struct {
	Command   string // The command to execute
	ChannelID string // Slack channel ID where the command was initiated
	MessageTS string // Timestamp of the message with confirmation buttons
	ThreadTS  string // Timestamp of the thread parent message
}

var (
	// Global Slack configuration
	slackConfig = SlackConfig{
		Token:    os.Getenv("SLACK_BOT_TOKEN"),
		Channel:  os.Getenv("SLACK_CHANNEL"),
		AppToken: os.Getenv("SLACK_APP_TOKEN"),
		Enabled:  false,
	}

	// Global Slack client
	slackClient *slack.Client

	// Global Socket Mode client for receiving events
	socketClient *socketmode.Client

	// Map to track active workflows
	activeWorkflows     = make(map[string]bool)
	activeWorkflowsLock sync.Mutex

	// Map to store thread timestamps for each workflow+target combination
	threadTimestamps     = make(map[string]string)
	threadTimestampsLock sync.Mutex

	// Map to store pending commands waiting for confirmation
	pendingCommands     = make(map[string]*PendingCommand)
	pendingCommandsLock sync.Mutex
)

// InitSlackIntegration initializes the Slack integration
func InitSlackIntegration() error {
	// Check if Slack integration is enabled
	if slackConfig.Token == "" || slackConfig.Channel == "" {
		fmt.Println("Slack integration is not enabled. Set SLACK_BOT_TOKEN and SLACK_CHANNEL environment variables to enable it.")
		return nil
	}

	// Create Slack client
	slackClient = slack.New(
		slackConfig.Token,
		slack.OptionAppLevelToken(slackConfig.AppToken),
	)

	// If app token is provided, create a socket mode client for receiving events
	if slackConfig.AppToken != "" {
		socketClient = socketmode.New(
			slackClient,
			socketmode.OptionDebug(true),
		)

		// Start listening for Slack events in a goroutine
		go startSlackEventListener()
	}

	slackConfig.Enabled = true
	fmt.Println("Slack integration initialized successfully")
	return nil
}

// startSlackEventListener starts listening for Slack events
func startSlackEventListener() {
	if socketClient == nil {
		return
	}

	// Listen for events
	go func() {
		for evt := range socketClient.Events {
			switch evt.Type {
			case socketmode.EventTypeInteractive:
				// Handle interactive events (buttons, etc.)
				if callback, ok := evt.Data.(slack.InteractionCallback); ok {
					// Acknowledge the event
					socketClient.Ack(*evt.Request)

					// Handle the interaction
					handleSlackInteraction(callback)
				}
			case socketmode.EventTypeSlashCommand:
				// Handle slash commands
				if cmd, ok := evt.Data.(slack.SlashCommand); ok {
					// Acknowledge the event
					socketClient.Ack(*evt.Request)

					// Handle the slash command
					handleSlashCommand(cmd)
				}
			case socketmode.EventTypeEventsAPI:
				// Handle Events API events
				socketClient.Ack(*evt.Request)

				// Check if this is a message event
				if eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent); ok {
					if eventsAPIEvent.Type == slackevents.CallbackEvent {
						switch innerEvent := eventsAPIEvent.InnerEvent.Data.(type) {
						case *slackevents.MessageEvent:
							// Only process messages that are in a thread and not from a bot
							if innerEvent.ThreadTimeStamp != "" && innerEvent.BotID == "" {
								// Process the message in a goroutine
								go processRequestFromMessage(innerEvent.Channel, innerEvent.Text, innerEvent.ThreadTimeStamp)
							}
						}
					}
				}
			}
		}
	}()

	// Start the socket mode client
	err := socketClient.Run()
	if err != nil {
		log.Printf("Error starting socket mode client: %v", err)
	}
}

// handleSlackInteraction handles interactions from Slack (button clicks, etc.)
func handleSlackInteraction(callback slack.InteractionCallback) {
	// Check if this is a button action
	if callback.Type == slack.InteractionTypeBlockActions {
		for _, action := range callback.ActionCallback.BlockActions {
			// Check if this is a command action (format: "command_action:action:commandID")
			if strings.HasPrefix(action.ActionID, "command_action:") {
				parts := strings.Split(action.ActionID, ":")
				if len(parts) == 3 {
					actionType := parts[1]
					commandID := parts[2]

					// Handle the command action
					handleCommandAction(callback, actionType, commandID)
					return
				}
			}

			// Parse the action ID which should be in the format "workflow_action:workflowID:targetName:action"
			parts := strings.Split(action.ActionID, ":")
			if len(parts) != 4 || parts[0] != "workflow_action" {
				continue
			}

			workflowID := parts[1]
			targetName := parts[2]
			actionType := parts[3]

			// Send the signal to the workflow
			err := signalWorkflow(context.Background(), workflowID, targetName, actionType)

			// Check if we have a thread timestamp for this workflow+target
			threadTS := getThreadTimestamp(workflowID, targetName)

			var options []slack.MsgOption
			var messageText string

			if err != nil {
				// Prepare error message
				messageText = fmt.Sprintf("Error signaling workflow: %v", err)
			} else {
				// Prepare confirmation message
				messageText = fmt.Sprintf("Signal sent to workflow %s: %s %s", workflowID, targetName, actionType)
			}

			options = append(options, slack.MsgOptionText(messageText, false))

			// If we have a thread timestamp, add it to the options
			if threadTS != "" {
				options = append(options, slack.MsgOptionTS(threadTS))
			}

			// Post the message to Slack
			_, _, _ = slackClient.PostMessage(
				callback.Channel.ID,
				options...,
			)

			// Update the original message to remove the action buttons
			if callback.Message.Timestamp != "" {
				// Create new blocks without the action block
				var updatedBlocks []slack.Block
				for _, block := range callback.Message.Blocks.BlockSet {
					// Skip the action block
					if block.BlockType() != slack.MBTAction {
						updatedBlocks = append(updatedBlocks, block)
					}
				}

				// Add a new section block indicating which action was taken
				updatedBlocks = append(updatedBlocks, slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Action taken:* %s", actionType), false, false),
					nil,
					nil,
				))

				// Update the original message
				_, _, _, _ = slackClient.UpdateMessage(
					callback.Channel.ID,
					callback.Message.Timestamp,
					slack.MsgOptionBlocks(updatedBlocks...),
				)
			}
		}
	}
}

// SendWorkflowPausedMessage sends a message to Slack when a workflow is paused
func SendWorkflowPausedMessage(workflowID, targetName, command string, failError error) error {
	if !slackConfig.Enabled || slackClient == nil {
		return nil
	}

	// Track this workflow as active
	activeWorkflowsLock.Lock()
	activeWorkflows[workflowID] = true
	activeWorkflowsLock.Unlock()

	// Create message text
	var messageText string
	if failError != nil {
		messageText = fmt.Sprintf("Workflow *%s* paused due to command failure in target *%s*", workflowID, targetName)
	} else {
		messageText = fmt.Sprintf("Workflow *%s* paused at target *%s*", workflowID, targetName)
	}

	// Create buttons for continue, retry (if failure), and abort
	var blocks []slack.Block
	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", messageText, false, false),
		nil,
		nil,
	))

	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Command: `%s`", command), false, false),
		nil,
		nil,
	))

	// Create action block with buttons
	var actionElements []slack.BlockElement

	buttonText := "Start"
	if failError != nil {
		buttonText = "Continue"
	}
	// Add continue button
	actionElements = append(
		actionElements,
		slack.NewButtonBlockElement(
			fmt.Sprintf("workflow_action:%s:%s:continue", workflowID, targetName),
			"continue",
			slack.NewTextBlockObject("plain_text", buttonText, false, false),
		).WithStyle(slack.StylePrimary),
	)

	// Add retry button if this is a failure
	if failError != nil {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Error Message ```%s```", failError.Error()), false, false),
			nil,
			nil,
		))
		actionElements = append(
			actionElements,
			slack.NewButtonBlockElement(
				fmt.Sprintf("workflow_action:%s:%s:retry", workflowID, targetName),
				"retry",
				slack.NewTextBlockObject("plain_text", "Retry", false, false),
			),
		)
	}

	// Add abort button
	actionElements = append(
		actionElements,
		slack.NewButtonBlockElement(
			fmt.Sprintf("workflow_action:%s:%s:abort", workflowID, targetName),
			"abort",
			slack.NewTextBlockObject("plain_text", "Abort", false, false),
		).WithStyle(slack.StyleDanger),
	)

	// Create the action block with the elements
	actionBlock := slack.NewActionBlock(
		"workflow_actions",
		actionElements...,
	)

	// Check if we have a thread timestamp for this workflow+target
	threadTS := getThreadTimestamp(workflowID, targetName)
	if threadTS == "" {
		blocks = append(blocks, actionBlock)
	}
	var options []slack.MsgOption
	options = append(options,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(messageText, false),
	)

	// If we have a thread timestamp, add it to the options
	if threadTS != "" {
		options = append(options, slack.MsgOptionTS(threadTS))
	}

	// Send the message to Slack
	_, timestamp, err := slackClient.PostMessage(
		slackConfig.Channel,
		options...,
	)

	// If this is the first message for this workflow+target, store the timestamp
	if threadTS == "" && err == nil {
		setThreadTimestamp(workflowID, targetName, timestamp)
	}

	// If there's an error, also send the message to the main thread (without thread timestamp)
	if threadTS != "" {
		blocks = append(blocks, actionBlock)
		// Create new options for the main thread message (without thread timestamp)
		mainOptions := []slack.MsgOption{
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionText(messageText, false),
		}

		// Send the message to the main channel
		_, _, _ = slackClient.PostMessage(
			slackConfig.Channel,
			mainOptions...,
		)
	}

	return err
}

// SendStepNotification sends a notification to Slack when a step starts or finishes
func SendStepNotification(workflowID, targetName, command string, isStart bool) error {
	if !slackConfig.Enabled || slackClient == nil {
		return nil
	}
	messageText := fmt.Sprintf("Workflow ID *%s* for Target *%s*", workflowID, targetName)

	// Create buttons for continue, retry (if failure), and abort
	var blocks []slack.Block
	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", messageText, false, false),
		nil,
		nil,
	))
	stepState := "Completed"
	if isStart {
		stepState = "Starting"
	}

	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("%s Command: `%s`", stepState, command), false, false),
		nil,
		nil,
	))

	// Check if we have a thread timestamp for this workflow+target
	threadTS := getThreadTimestamp(workflowID, targetName)

	var options []slack.MsgOption
	options = append(options, slack.MsgOptionBlocks(blocks...))

	// If we have a thread timestamp, add it to the options
	if threadTS != "" {
		options = append(options, slack.MsgOptionTS(threadTS))
	}

	// Send the message to Slack
	_, timestamp, err := slackClient.PostMessage(
		slackConfig.Channel,
		options...,
	)

	// If this is the first message for this workflow+target, store the timestamp
	if threadTS == "" && err == nil {
		setThreadTimestamp(workflowID, targetName, timestamp)
	}

	return err
}

// SendTargetNotification sends a notification to Slack when a step starts or finishes
func SendTargetNotification(workflowID, targetName string, isStart bool) error {
	if !slackConfig.Enabled || slackClient == nil {
		return nil
	}
	targetState := "Completed"
	if isStart {
		targetState = "Starting"
	}
	messageText := fmt.Sprintf("%s Target *%s* in Workflow ID *%s*.", targetState, targetName, workflowID)

	// Create buttons for continue, retry (if failure), and abort
	var blocks []slack.Block
	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", messageText, false, false),
		nil,
		nil,
	))
	// Check if we have a thread timestamp for this workflow+target
	threadTS := getThreadTimestamp(workflowID, targetName)

	var options []slack.MsgOption
	options = append(options, slack.MsgOptionBlocks(blocks...))

	// If we have a thread timestamp, add it to the options
	if threadTS != "" {
		options = append(options, slack.MsgOptionTS(threadTS))
	}

	// Send the message to Slack
	_, timestamp, err := slackClient.PostMessage(
		slackConfig.Channel,
		options...,
	)

	// If this is the first message for this workflow+target, store the timestamp
	if threadTS == "" && err == nil {
		setThreadTimestamp(workflowID, targetName, timestamp)
	}

	return err
}

// getThreadTimestamp returns the thread timestamp for a workflow+target combination
// If no thread timestamp exists, it returns an empty string
func getThreadTimestamp(workflowID, targetName string) string {
	threadTimestampsLock.Lock()
	defer threadTimestampsLock.Unlock()

	key := fmt.Sprintf("%s:%s", workflowID, targetName)
	return threadTimestamps[key]
}

// setThreadTimestamp sets the thread timestamp for a workflow+target combination
func setThreadTimestamp(workflowID, targetName, timestamp string) {
	threadTimestampsLock.Lock()
	defer threadTimestampsLock.Unlock()

	key := fmt.Sprintf("%s:%s", workflowID, targetName)
	threadTimestamps[key] = timestamp
}

// signalWorkflow sends a signal to a Temporal workflow
func signalWorkflow(ctx context.Context, workflowID, targetName, action string) error {
	// Create a Temporal client
	c, err := client.Dial(client.Options{})
	if err != nil {
		return err
	}
	defer c.Close()

	// Send the signal to the workflow
	response := CommandResponse{
		TargetName: targetName,
		Action:     action,
	}
	return c.SignalWorkflow(ctx, workflowID, "", CommandResponseSignal, response)
}

// handleSlashCommand handles slash commands from Slack
func handleSlashCommand(cmd slack.SlashCommand) {
	// Respond initially to acknowledge receipt
	initialResponse := fmt.Sprintf("Understanding your request: `%s`", cmd.Text)

	// Send the initial message and capture its timestamp to use as the thread parent
	_, threadTS, _ := slackClient.PostMessage(
		cmd.ChannelID,
		slack.MsgOptionText(initialResponse, false),
	)

	// Process the request in a goroutine, passing the thread timestamp
	go processRequestFromMessage(cmd.ChannelID, cmd.Text, threadTS)
}

func processRequestFromMessage(channelID, userInput, threadTS string) {
	responseText := fmt.Sprintf("Sorry, things did not go as expected. Please tell me again.")
	// If we have a specific operation text, use the old roachtest command for backward compatibility
	if userInput != "" {
		// If we have userInput but no specific operation text, use the DirectAIAgent to generate a shell command
		agent, err := NewDirectAIAgent()
		if err != nil {
			_, _, _ = slackClient.PostMessage(
				channelID,
				slack.MsgOptionText(responseText, false),
				slack.MsgOptionTS(threadTS),
			)
			fmt.Printf("AI Agent initialization error: %v\n", err)
			return
		}

		// We'll set clarificationResponse to empty string since we're using the history in the context
		clarificationResponse := ""
		originalUserInput := userInput
		func() {
			// Check if this is a response to a clarification question
			clarificationContextsLock.Lock()
			previousContext, exists := clarificationContexts[threadTS]
			clarificationContextsLock.Unlock()
			if exists {
				// This is a response to a clarification question
				// Use the original input as the user input
				originalUserInput = previousContext.OriginalInput
				log.Printf("Found clarification context for thread %s. Original input: %s, New response: %s",
					threadTS, originalUserInput, userInput)

				// Update the history with the answer to the most recent question
				if len(previousContext.History) > 0 {
					// Update the last question's answer if it matches the current question
					for i := len(previousContext.History) - 1; i >= 0; i-- {
						if previousContext.History[i].Question == previousContext.Question && previousContext.History[i].Answer == "" {
							previousContext.History[i].Answer = userInput
							previousContext.History[i].Timestamp = time.Now()
							break
						}
					}
				} else {
					// If history is empty but we have a question, add it to history
					previousContext.History = append(previousContext.History, ClarificationExchange{
						Question:  previousContext.Question,
						Answer:    userInput,
						Timestamp: time.Now(),
					})
				}

				// Keep the context for future clarifications
				// We'll use the history in GenerateShellCommand, so no need to pass clarificationResponse
			}
		}()
		// Generate the shell command using the AI agent
		// Pass empty string for clarificationResponse since we're using the history in the context
		shellCmd, err := agent.GenerateShellCommand(context.Background(), originalUserInput, slackClient, channelID, threadTS, clarificationResponse)
		if err != nil {
			fmt.Printf("Error generating shell command: %v\n", err)
			_, _, _ = slackClient.PostMessage(
				channelID,
				slack.MsgOptionText(responseText, false),
				slack.MsgOptionTS(threadTS),
			)
			return
		}
		if shellCmd == "" {
			inClarificationContext := false
			func() {
				// Check if we're in a clarification context
				clarificationContextsLock.Lock()
				defer clarificationContextsLock.Unlock()
				if previousContext, exists := clarificationContexts[threadTS]; exists && previousContext.Question != "" {
					inClarificationContext = true
				}
			}()

			// If we're in a clarification context but no command was generated,
			// it means the AI still needs more information
			if inClarificationContext {
				// Don't send a generic message, as the AI has already sent a clarification question
				return
			}
			responseText = "No command generated. Please provide a valid request."
			_, _, _ = slackClient.PostMessage(
				channelID,
				slack.MsgOptionText(responseText, false),
				slack.MsgOptionTS(threadTS),
			)
			return
		}

		// Generate a command ID
		commandID := generateCommandID(shellCmd)

		// Send the generated command back to the user for confirmation with buttons
		confirmationText := fmt.Sprintf("Do you want me to execute the following command? ```%s```", shellCmd)

		// Create message blocks
		var blocks []slack.Block
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", confirmationText, false, false),
			nil,
			nil,
		))

		// Create action block with confirm/cancel buttons
		var actionElements []slack.BlockElement

		// Add confirm button
		actionElements = append(
			actionElements,
			slack.NewButtonBlockElement(
				fmt.Sprintf("command_action:confirm:%s", commandID),
				"confirm",
				slack.NewTextBlockObject("plain_text", "Confirm & Execute", false, false),
			).WithStyle(slack.StylePrimary),
		)

		// Add cancel button
		actionElements = append(
			actionElements,
			slack.NewButtonBlockElement(
				fmt.Sprintf("command_action:cancel:%s", commandID),
				"cancel",
				slack.NewTextBlockObject("plain_text", "Cancel", false, false),
			).WithStyle(slack.StyleDanger),
		)

		// Create the action block with the elements
		actionBlock := slack.NewActionBlock(
			"command_actions",
			actionElements...,
		)

		blocks = append(blocks, actionBlock)

		// Send the message with buttons
		_, timestamp, err := slackClient.PostMessage(
			channelID,
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionText(confirmationText, false),
			slack.MsgOptionTS(threadTS),
		)

		if err != nil {
			fmt.Printf("Error sending confirmation message: %v\n", err)
			_, _, _ = slackClient.PostMessage(
				channelID,
				slack.MsgOptionText(responseText, false),
				slack.MsgOptionTS(threadTS),
			)
			return
		}

		// Store the command in the pending commands map
		func() {
			pendingCommandsLock.Lock()
			defer pendingCommandsLock.Unlock()
			pendingCommands[commandID] = &PendingCommand{
				Command:   shellCmd,
				ChannelID: channelID,
				MessageTS: timestamp,
				ThreadTS:  threadTS,
			}
		}()

		// Return without executing the command - it will be executed after confirmation
		return
	} else {
		responseText = "Please provide a command to execute. For example: `/drt-workflow run an add column operation on the drt-chaos cluster`"
		_, _, _ = slackClient.PostMessage(
			channelID,
			slack.MsgOptionText(responseText, false),
			slack.MsgOptionTS(threadTS),
		)
		return
	}

}

// generateCommandID creates a unique identifier for a command
func generateCommandID(command string) string {
	// Create a hash of the command + current timestamp to ensure uniqueness
	h := sha256.New()
	h.Write([]byte(command))
	h.Write([]byte(time.Now().String()))
	return hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars of the hash
}

// handleCommandAction processes command confirmation or cancellation
func handleCommandAction(callback slack.InteractionCallback, actionType string, commandID string) {
	// Update the original message to remove the action buttons
	if callback.Message.Timestamp != "" {
		// Create new blocks without the action block
		var updatedBlocks []slack.Block
		for _, block := range callback.Message.Blocks.BlockSet {
			// Skip the action block
			if block.BlockType() != slack.MBTAction {
				updatedBlocks = append(updatedBlocks, block)
			}
		}

		// Add a new section block indicating which action was taken
		updatedBlocks = append(updatedBlocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Action taken:* %s", actionType), false, false),
			nil,
			nil,
		))

		// Update the original message
		_, _, _, _ = slackClient.UpdateMessage(
			callback.Channel.ID,
			callback.Message.Timestamp,
			slack.MsgOptionBlocks(updatedBlocks...),
		)
	}

	// Get the pending command from the map
	pendingCommandsLock.Lock()
	pendingCmd, exists := pendingCommands[commandID]
	pendingCommandsLock.Unlock()
	if actionType == "confirm" && exists {
		// If action is confirm, execute the command
		// Send a message indicating that the command is being executed
		_, _, _ = slackClient.PostMessage(
			callback.Channel.ID,
			slack.MsgOptionText(fmt.Sprintf("Executing command: `%s`", pendingCmd.Command), false),
			slack.MsgOptionTS(pendingCmd.ThreadTS),
		)
		commandWithArgs := strings.Split(pendingCmd.Command, " ")
		// Create the command
		cmd := exec.Command(commandWithArgs[0], commandWithArgs[1:]...)

		// Get pipes for stdout and stderr
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			// Handle error getting stdout pipe
			_, _, _ = slackClient.PostMessage(
				callback.Channel.ID,
				slack.MsgOptionText(fmt.Sprintf("Error setting up command: %v", err), false),
				slack.MsgOptionTS(pendingCmd.ThreadTS),
			)
			delete(pendingCommands, commandID)
			return
		}

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			// Handle error getting stderr pipe
			_, _, _ = slackClient.PostMessage(
				callback.Channel.ID,
				slack.MsgOptionText(fmt.Sprintf("Error setting up command: %v", err), false),
				slack.MsgOptionTS(pendingCmd.ThreadTS),
			)
			delete(pendingCommands, commandID)
			return
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			// Handle error starting command
			_, _, _ = slackClient.PostMessage(
				callback.Channel.ID,
				slack.MsgOptionText(fmt.Sprintf("Error starting command: %v", err), false),
				slack.MsgOptionTS(pendingCmd.ThreadTS),
			)
			delete(pendingCommands, commandID)
			return
		}

		// Send initial message
		_, _, _ = slackClient.PostMessage(
			callback.Channel.ID,
			slack.MsgOptionText(fmt.Sprintf("Command started: `%s`\nStreaming output...", pendingCmd.Command), false),
			slack.MsgOptionTS(pendingCmd.ThreadTS),
		)

		// Create buffers to collect output
		stdoutBuf := make([]byte, 4096)
		stderrBuf := make([]byte, 4096)

		// Create a WaitGroup to wait for both goroutines to finish
		var wg sync.WaitGroup
		wg.Add(2)

		// Stream stdout
		go func() {
			defer wg.Done()
			for {
				n, err := stdoutPipe.Read(stdoutBuf)
				if n > 0 {
					output := string(stdoutBuf[:n])
					// Send stdout to Slack
					_, _, _ = slackClient.PostMessage(
						callback.Channel.ID,
						slack.MsgOptionText(fmt.Sprintf("```%s```", output), false),
						slack.MsgOptionTS(pendingCmd.ThreadTS),
					)
				}
				if err != nil {
					// EOF or other error, exit the loop
					break
				}
			}
		}()

		// Stream stderr
		go func() {
			defer wg.Done()
			for {
				n, err := stderrPipe.Read(stderrBuf)
				if n > 0 {
					output := string(stderrBuf[:n])
					// Send stderr to Slack
					_, _, _ = slackClient.PostMessage(
						callback.Channel.ID,
						slack.MsgOptionText(fmt.Sprintf("*ERROR:*\n```%s```", output), false),
						slack.MsgOptionTS(pendingCmd.ThreadTS),
					)
				}
				if err != nil {
					// EOF or other error, exit the loop
					break
				}
			}
		}()

		// Wait for the command to finish
		cmdErr := cmd.Wait()

		// Wait for both goroutines to finish
		wg.Wait()

		// Send final status message
		var finalMessage string
		if cmdErr != nil {
			finalMessage = fmt.Sprintf("Command failed with error: %v", cmdErr)
		} else {
			finalMessage = "Command completed successfully"
		}

		// Post the final status message
		_, _, _ = slackClient.PostMessage(
			callback.Channel.ID,
			slack.MsgOptionText(finalMessage, false),
			slack.MsgOptionTS(pendingCmd.ThreadTS),
		)
		delete(pendingCommands, commandID)
	} else {
		// If action is cancel or command doesn't exist, remove it from the map now
		delete(pendingCommands, commandID)

		if !exists {
			// Command not found - we don't have the thread timestamp, so we can't thread this message
			// This should be rare and only happen if the command was already executed or if there's a bug
			_, _, _ = slackClient.PostMessage(
				callback.Channel.ID,
				slack.MsgOptionText("Error: Command not found or already executed", false),
			)
			return
		}

		if actionType == "cancel" {
			// Check if this is a response to a clarification question
			clarificationContextsLock.Lock()
			previousContext, exists := clarificationContexts[pendingCmd.ThreadTS]
			clarificationContextsLock.Unlock()
			if exists {
				previousContext.Question = fmt.Sprintf("Why was generated the command \"%s\" rejected?", pendingCmd.Command)
				previousContext.History = append(previousContext.History, ClarificationExchange{
					Question:  previousContext.Question,
					Timestamp: time.Now(),
				})
			}

			// Log the cancellation
			log.Printf("Command cancelled: %s in thread %s", pendingCmd.Command, pendingCmd.ThreadTS)

			// Command was cancelled
			_, _, _ = slackClient.PostMessage(
				callback.Channel.ID,
				slack.MsgOptionText("Command execution cancelled. This has been recorded as not an expected response.", false),
				slack.MsgOptionTS(pendingCmd.ThreadTS),
			)
			return
		}

		return
	}
}
