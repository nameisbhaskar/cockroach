// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package demo

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/sdk"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

const (
	envSlackBotToken = "SLACK_BOT_TOKEN"
	envSlackChannel  = "SLACK_CHANNEL"
	envSlackAppToken = "SLACK_APP_TOKEN"
)

// SlackApprover handles sending approval requests to Slack with interactive buttons.
// It uses Socket Mode to receive button click events and automatically invokes the resume API.
type SlackApprover struct {
	client       *slack.Client
	socketClient *socketmode.Client
	apiBaseURL   string
	channel      string
	appToken     string
	enabled      bool

	// pendingApprovals tracks stepID -> approval context for handling button clicks
	pendingApprovals sync.Map
}

// approvalContext holds information needed to process an approval response
type approvalContext struct {
	planID     string
	workflowID string
	stepID     string
}

// NewSlackApprover creates a new SlackApprover from environment variables and flags.
// If appToken is provided, it starts a Socket Mode listener for interactive buttons.
func NewSlackApprover(botToken, channel, appToken, apiBaseURL string) (*SlackApprover, error) {
	// Use env vars as fallback
	if botToken == "" {
		botToken = os.Getenv(envSlackBotToken)
	}
	if channel == "" {
		channel = os.Getenv(envSlackChannel)
	}
	if appToken == "" {
		appToken = os.Getenv(envSlackAppToken)
	}

	// If bot token or channel is missing, return a disabled approver
	if botToken == "" || channel == "" {
		return &SlackApprover{
			enabled: false,
		}, nil
	}

	var client *slack.Client

	// Create slack client with app-level token if Socket Mode is enabled
	if appToken != "" {
		client = slack.New(
			botToken,
			slack.OptionAppLevelToken(appToken),
		)
	} else {
		client = slack.New(botToken)
	}

	sa := &SlackApprover{
		client:     client,
		channel:    channel,
		enabled:    true,
		apiBaseURL: apiBaseURL,
		appToken:   appToken,
	}

	// If an app token is provided, enable Socket Mode for interactive buttons
	if appToken != "" {
		socketClient := socketmode.New(client)
		sa.socketClient = socketClient

		// Start Socket Mode listener in background
		go sa.startSocketModeListener()
	}

	return sa, nil
}

// startSocketModeListener starts listening for Slack interactive events via Socket Mode
func (sa *SlackApprover) startSocketModeListener() {
	if sa.socketClient == nil {
		return
	}

	go func() {
		for evt := range sa.socketClient.Events {
			switch evt.Type {
			case socketmode.EventTypeInteractive:
				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					continue
				}

				// Acknowledge the event
				sa.socketClient.Ack(*evt.Request)

				// Handle button click
				if callback.Type == slack.InteractionTypeBlockActions {
					sa.handleButtonClick(callback)
				}
			}
		}
	}()

	// Start the Socket Mode connection
	_ = sa.socketClient.Run()
}

// handleButtonClick processes a button click from Slack
func (sa *SlackApprover) handleButtonClick(callback slack.InteractionCallback) {
	if len(callback.ActionCallback.BlockActions) == 0 {
		return
	}

	action := callback.ActionCallback.BlockActions[0]
	stepID := action.Value
	result := action.ActionID // "approve" or "reject"

	// Look up the approval context
	ctxVal, ok := sa.pendingApprovals.Load(stepID)
	if !ok {
		return
	}
	ctx := ctxVal.(approvalContext)

	// Remove from pending
	sa.pendingApprovals.Delete(stepID)

	// Map action to result string
	var resultStr string
	if result == "approve" {
		resultStr = "approved"
	} else {
		resultStr = "rejected"
	}

	// Invoke the resume API
	if err := sa.invokeResumeAPI(context.Background(), ctx.planID, ctx.workflowID, stepID, resultStr); err != nil {
		// Update Slack message with error
		sa.updateMessageWithError(callback.Channel.ID, callback.Message.Timestamp, stepID, err)
		return
	}

	// Update Slack message with result
	sa.updateMessageWithResult(callback.Channel.ID, callback.Message.Timestamp, stepID, resultStr)
}

// invokeResumeAPI calls the TEF resume API endpoint
func (sa *SlackApprover) invokeResumeAPI(
	ctx context.Context, planID, workflowID, stepID, result string,
) error {
	client := sdk.NewClient(sa.apiBaseURL)
	_, err := client.ResumePlan(ctx, planID, workflowID, stepID, result)
	return err
}

// updateMessageWithResult updates the Slack message to show the approval result
func (sa *SlackApprover) updateMessageWithResult(channel, timestamp, stepID, result string) {
	var emoji, status string
	if result == "approved" {
		emoji = "✅"
		status = "APPROVED"
	} else {
		emoji = "❌"
		status = "REJECTED"
	}

	message := fmt.Sprintf("%s *Approval Request %s:* %s", emoji, stepID, status)
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", message, false, false),
			nil,
			nil,
		),
	}

	_, _, _, _ = sa.client.UpdateMessage(
		channel,
		timestamp,
		slack.MsgOptionBlocks(blocks...),
	)
}

// updateMessageWithError updates the Slack message to show an error
func (sa *SlackApprover) updateMessageWithError(channel, timestamp, stepID string, err error) {
	message := fmt.Sprintf("⚠️ *Approval Request %s:* Failed to process - %v", stepID, err)
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", message, false, false),
			nil,
			nil,
		),
	}

	_, _, _, _ = sa.client.UpdateMessage(
		channel,
		timestamp,
		slack.MsgOptionBlocks(blocks...),
	)
}

// SendApprovalRequest sends an approval request to Slack with interactive buttons.
// If Socket Mode is enabled, clicking the buttons will automatically invoke the resume API.
// Otherwise, curl command instructions are provided as fallback.
func (sa *SlackApprover) SendApprovalRequest(
	ctx context.Context, info *planners.PlanExecutionInfo, message string,
) (string, error) {
	if !sa.enabled {
		return "", fmt.Errorf("slack integration is disabled. Set %s and %s",
			envSlackBotToken, envSlackChannel)
	}

	logger := planners.LoggerFromContext(ctx)

	// Generate a unique step ID
	stepID := fmt.Sprintf("approval_%d", time.Now().UnixNano())

	// Store approval context for button handling
	sa.pendingApprovals.Store(stepID, approvalContext{
		planID:     info.PlanID,
		workflowID: info.WorkflowID,
		stepID:     stepID,
	})

	var blocks []slack.Block

	// Create message header
	headerText := fmt.Sprintf("%s\n\n*Plan ID:* `%s`\n*Workflow ID:* `%s`\n*Step ID:* `%s`",
		message, info.PlanID, info.WorkflowID, stepID)

	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", headerText, false, false),
		nil,
		nil,
	))

	// If Socket Mode is enabled, add interactive buttons
	if sa.socketClient != nil {
		// Add action buttons
		approveButton := slack.NewButtonBlockElement(
			"approve",
			stepID,
			slack.NewTextBlockObject("plain_text", "✅ Approve", true, false),
		)
		approveButton.Style = slack.StylePrimary

		rejectButton := slack.NewButtonBlockElement(
			"reject",
			stepID,
			slack.NewTextBlockObject("plain_text", "❌ Reject", true, false),
		)
		rejectButton.Style = slack.StyleDanger

		blocks = append(blocks, slack.NewActionBlock(
			"approval_actions",
			approveButton,
			rejectButton,
		))

		logger.Info("[DEMO] Sending approval request with interactive buttons (Socket Mode enabled)")
	} else {
		// Fallback to curl instructions if Socket Mode is not available
		curlInstructions := fmt.Sprintf(
			"To approve this request, use the TEF API:\n"+
				"```\ncurl -X POST %s/v1/plans/%s/executions/%s/steps/%s/resume -d '{\"result\":\"approved\"}'\n```\n\n"+
				"To reject:\n"+
				"```\ncurl -X POST %s/v1/plans/%s/executions/%s/steps/%s/resume -d '{\"result\":\"rejected\"}'\n```",
			sa.apiBaseURL, info.PlanID, info.WorkflowID, stepID,
			sa.apiBaseURL, info.PlanID, info.WorkflowID, stepID)

		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", curlInstructions, false, false),
			nil,
			nil,
		))

		logger.Info("[DEMO] Sending approval request with curl instructions (Socket Mode disabled)")
	}

	// Send the message
	_, timestamp, err := sa.client.PostMessage(
		sa.channel,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		sa.pendingApprovals.Delete(stepID)
		return "", fmt.Errorf("failed to send Slack message: %w", err)
	}

	logger.Info("[DEMO] Sent Slack approval request", "channel", sa.channel, "timestamp", timestamp, "stepID", stepID)

	return stepID, nil
}

// SendResultNotification sends a notification to Slack about the approval result.
func (sa *SlackApprover) SendResultNotification(ctx context.Context, stepID, result string) error {
	if !sa.enabled {
		return nil // Silently skip if disabled
	}

	logger := planners.LoggerFromContext(ctx)

	var message string
	if result == "approved" {
		message = fmt.Sprintf("✅ *Approval Request %s:* APPROVED", stepID)
	} else {
		message = fmt.Sprintf("❌ *Approval Request %s:* REJECTED", stepID)
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", message, false, false),
			nil,
			nil,
		),
	}

	_, _, err := sa.client.PostMessage(
		sa.channel,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		logger.Error("[DEMO] Failed to send Slack result notification", "err", err)
		return err
	}

	logger.Info("[DEMO] Sent Slack result notification", "stepID", stepID)
	return nil
}
