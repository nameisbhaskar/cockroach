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

	"github.com/slack-go/slack"
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
