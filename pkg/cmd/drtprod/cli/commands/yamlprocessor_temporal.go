package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachprod/cli"
	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// Constants for Temporal workflow
const (
	// YamlTaskQueue is the task queue for YAML processing
	YamlTaskQueue = "yaml-task-queue"
	// YamlWorkflowID is the workflow ID prefix for YAML processing
	YamlWorkflowID = "yaml-workflow"
	// CommandFailureSignal is the signal type for command failures
	CommandFailureSignal = "command-failure-signal"
	// CommandResponseSignal is the signal type for user responses to command failures
	CommandResponseSignal = "command-response-signal"
)

// GetStartWorkflowCommand creates a new Cobra command for starting a Temporal worker
func GetStartWorkflowCommand(ctx context.Context) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "start-workflow",
		Short: "Start a Temporal worker for processing workflows",
		Long:  `Start a Temporal worker for processing workflows. This command blocks until interrupted.`,
		Run: cli.Wrap(func(cmd *cobra.Command, args []string) (retErr error) {
			return startTemporalWorker(ctx)
		}),
	}

	return cobraCmd
}

// GetYamlWorkflowCommand creates a new Cobra command for sending signals to a YAML workflow
func GetYamlWorkflowCommand(ctx context.Context) *cobra.Command {
	var workflowID string
	var targetName string
	var action string

	cobraCmd := &cobra.Command{
		Use:   "workflow [flags]",
		Short: "Interact with a running YAML workflow",
		Long:  `Interact with a running YAML workflow. Use this to continue or abort a paused workflow.`,
		Run: cli.Wrap(func(cmd *cobra.Command, args []string) (retErr error) {
			if workflowID == "" {
				return errors.New("workflow-id is required")
			}
			if targetName == "" {
				return errors.New("target-name is required")
			}
			if action != "continue" && action != "abort" && action != "retry" {
				return errors.New("action must be either 'continue', 'abort', or 'retry'")
			}

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
			err = c.SignalWorkflow(ctx, workflowID, "", CommandResponseSignal, response)
			if err != nil {
				return err
			}

			fmt.Printf("Signal sent to workflow %s: %s %s\n", workflowID, targetName, action)
			return nil
		}),
	}

	cobraCmd.Flags().StringVar(&workflowID, "workflow-id", "", "ID of the workflow to signal")
	cobraCmd.Flags().StringVar(&targetName, "target-name", "", "Name of the target to continue or abort")
	cobraCmd.Flags().StringVar(&action, "action", "", "Action to take: 'continue' or 'abort'")

	return cobraCmd
}

// startTemporalWorker starts a Temporal worker to process activities
func startTemporalWorker(ctx context.Context) error {
	// Initialize Slack integration
	if err := InitSlackIntegration(); err != nil {
		fmt.Printf("Warning: Failed to initialize Slack integration: %v\n", err)
	}

	// Create a Temporal client
	c, err := client.Dial(client.Options{})
	if err != nil {
		return err
	}
	defer c.Close()

	// Start a worker to process activities
	w := worker.New(c, YamlTaskQueue, worker.Options{})
	w.RegisterWorkflow(YamlWorkflow)
	w.RegisterActivity(ExecuteStepActivity)

	// Start worker (blocking)
	err = w.Start()
	if err != nil {
		return err
	}

	fmt.Printf("Temporal worker started. Listening for tasks on queue: %s\n", YamlTaskQueue)
	fmt.Printf("Press Ctrl+C to exit.\n")

	// Block until context is cancelled
	<-ctx.Done()
	w.Stop()
	return nil
}

// executeWorkflow executes a workflow with the given input
func executeWorkflow(
	ctx context.Context, yamlFileLocation string, config yamlConfig, userProvidedTargetNames []string,
) error {
	// Initialize Slack integration
	if err := InitSlackIntegration(); err != nil {
		fmt.Printf("Warning: Failed to initialize Slack integration: %v\n", err)
	}

	// Create a Temporal client
	c, err := client.Dial(client.Options{})
	if err != nil {
		return err
	}
	defer c.Close()

	// Prepare workflow input
	input := &YamlWorkflowInput{
		YamlFileLocation:        yamlFileLocation,
		Config:                  config,
		UserProvidedTargetNames: userProvidedTargetNames,
	}

	// Execute workflow
	workflowID := fmt.Sprintf("%s-%s", YamlWorkflowID, time.Now().Format("20060102-150405"))
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: YamlTaskQueue,
	}

	we, err := c.ExecuteWorkflow(ctx, options, YamlWorkflow, input)
	if err != nil {
		return err
	}

	fmt.Printf("Workflow started with ID: %s and Run ID: %s\n", we.GetID(), we.GetRunID())
	fmt.Printf("Workflow is running. Press Ctrl+C to exit.\n")

	// Wait for workflow completion
	var result string
	err = we.Get(ctx, &result)
	if err != nil {
		return err
	}

	fmt.Printf("Workflow completed: %s\n", result)
	return nil
}

// processYamlWorkflow executes the YAML in a Temporal workflow
func processYamlWorkflow(
	ctx context.Context,
	yamlFileLocation string,
	config yamlConfig,
	displayOnly bool,
	userProvidedTargetNames []string,
) error {
	if displayOnly {
		return errors.Errorf("display option is not valid for workflow execution")
	}

	return executeWorkflow(ctx, yamlFileLocation, config, userProvidedTargetNames)
}

// YamlWorkflowInput contains the input for the YAML workflow
type YamlWorkflowInput struct {
	YamlFileLocation        string
	Config                  yamlConfig
	UserProvidedTargetNames []string
}

// CommandFailureInfo contains information about a failed command
type CommandFailureInfo struct {
	TargetName string
	Command    string
	Error      string
}

// CommandResponse contains the user's response to a command failure
type CommandResponse struct {
	TargetName string
	Action     string // "continue" or "abort"
}

// YamlWorkflow is the Temporal workflow that processes the YAML
func YamlWorkflow(ctx workflow.Context, input *YamlWorkflowInput) (string, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting YAML workflow", "yamlFile", input.YamlFileLocation)

	// Create channel for command response signals
	commandResponseChannel := workflow.GetSignalChannel(ctx, CommandResponseSignal)

	// Set environment variables
	// Note: In a real implementation, you might want to use workflow.SetQueryHandler
	// or other mechanisms to make environment variables available to activities
	logger.Info("Setting environment variables", "env", input.Config.Environment)

	// Set environment variables for the workflow
	for key, value := range input.Config.Environment {
		logger.Info("Setting env", "key", key, "value", value)
		err := os.Setenv(key, value)
		if err != nil {
			return "", errors.Wrapf(err, "failed to set environment variable %s", key)
		}
	}

	// Create a map to track paused targets
	pausedTargets := make(map[string]bool)

	// Process targets
	targetNameMap := make(map[string]struct{})
	for _, tn := range input.UserProvidedTargetNames {
		targetNameMap[tn] = struct{}{}
	}

	// Resolve environment variables in target names and dependent targets
	for i := range input.Config.Targets {
		// Expand environment variables in target Name
		input.Config.Targets[i].TargetName = os.ExpandEnv(input.Config.Targets[i].TargetName)

		// Expand environment variables in dependent target names
		for j := range input.Config.Targets[i].DependentTargets {
			input.Config.Targets[i].DependentTargets[j] = os.ExpandEnv(input.Config.Targets[i].DependentTargets[j])
		}

		// Generate the commands for each target's steps
		targetSteps, err := generateCmdsFromSteps(input.Config.Targets[i].TargetName, input.Config.Targets[i].Steps)
		if err != nil {
			return "", err
		}
		input.Config.Targets[i].commands = targetSteps
	}

	// Track target statuses and completion channels
	targetStatuses := make(map[string]targetStatus)
	targetCompletionChannels := make(map[string]workflow.Channel)
	// Create a map to track dependent target channels
	dependentTargetChannels := make(map[string]map[string]workflow.Channel)

	// Create a completion channel for each target
	for _, t := range input.Config.Targets {
		if !shouldSkipTarget(targetNameMap, t, input.UserProvidedTargetNames) {
			targetCompletionChannels[t.TargetName] = workflow.NewChannel(ctx)
			// Initialize the map for dependent targets
			dependentTargetChannels[t.TargetName] = make(map[string]workflow.Channel)
		}
	}

	// Pre-create channels for all dependent target relationships
	for _, t := range input.Config.Targets {
		if !shouldSkipTarget(targetNameMap, t, input.UserProvidedTargetNames) {
			for _, dt := range t.DependentTargets {
				if _, ok := targetCompletionChannels[dt]; ok {
					// Create a dedicated channel for this dependent target relationship
					dependentCh := workflow.NewChannel(ctx)
					dependentTargetChannels[dt][t.TargetName] = dependentCh
					logger.Info("Pre-created channel for dependent target", "target", t.TargetName, "dependentTarget", dt)
				}
			}
		}
	}

	// Process each target
	for _, t := range input.Config.Targets {
		if shouldSkipTarget(targetNameMap, t, input.UserProvidedTargetNames) {
			logger.Info("Skipping target", "target", t.TargetName)
			continue
		}

		// Process target in a separate goroutine
		targetName := t.TargetName // Create a copy for the closure
		targetInfo := t            // Create a copy for the closure
		workflow.Go(ctx, func(ctx workflow.Context) {
			// Wait for dependent targets
			for _, dt := range targetInfo.DependentTargets {
				if _, ok := targetCompletionChannels[dt]; ok {
					logger.Info("Waiting for dependent target", "target", targetName, "dependentTarget", dt)

					// Get the pre-created channel for this dependent target relationship
					dependentCh := dependentTargetChannels[dt][targetName]

					// Wait for the dependent target to complete
					var status targetStatus
					dependentCh.Receive(ctx, &status)

					// Check status of dependent target
					if !targetInfo.IgnoreDependentFailure && status == targetResultFailure {
						logger.Info("Not proceeding due to dependent target failure", "target", targetName, "dependentTarget", dt)
						targetStatuses[targetName] = targetResultFailure

						// Send to main completion channel
						targetCompletionChannels[targetName].Send(ctx, targetResultFailure)

						// Send to any dependent target channels
						if dependentChannels, ok := dependentTargetChannels[targetName]; ok {
							for depTarget, depCh := range dependentChannels {
								logger.Info("Sending failure signal to dependent target", "target", targetName, "dependentTarget", depTarget)
								depCh.Send(ctx, targetResultFailure)
							}
						}
						return
					}
				} else {
					// If the dependent target is not in the workflow (skipped or not included),
					// log a message and continue without waiting
					logger.Info("Dependent target not in workflow, skipping wait", "target", targetName, "dependentTarget", dt)
				}
			}
			// Get workflow ID
			workflowID := workflow.GetInfo(ctx).WorkflowExecution.ID

			// Send notification to Slack
			err := SendTargetNotification(workflowID, targetName, true)
			if err != nil {
				logger.Error("Failed to send Slack message", "error", err)
			}
			// Execute target steps
			index := 0
			for {
				if index >= len(targetInfo.commands) {
					logger.Info("All steps completed for target", "target", targetName)
					break
				}
				cmd := targetInfo.commands[index]

				// Check if Wait is negative, which means we should pause and wait for user action
				if cmd.Wait < 0 {

					// Print instructions for the user
					logger.Info("Workflow paused due to negative Wait value",
						"workflowID", workflowID,
						"target", targetName,
						"command", cmd.String(),
						"wait", cmd.Wait)

					// Log instructions for continuing, retrying, or aborting
					logger.Info("To continue execution, run:",
						"command", fmt.Sprintf("drtprod execute workflow --workflow-id=%s --target-name=%s --action=continue",
							workflowID, targetName))
					logger.Info("To abort execution, run:",
						"command", fmt.Sprintf("drtprod execute workflow --workflow-id=%s --target-name=%s --action=abort",
							workflowID, targetName))

					// Send message to Slack
					err := SendWorkflowPausedMessage(workflowID, targetName, cmd.String(), nil)
					if err != nil {
						logger.Error("Failed to send Slack message", "error", err)
					}

					// Mark target as paused
					pausedTargets[targetName] = true

					// Wait for user response
					logger.Info("Waiting for user response", "target", targetName)
					var response CommandResponse
					for {
						// Create a selector to wait for the response signal
						selector := workflow.NewSelector(ctx)
						selector.AddReceive(commandResponseChannel, func(c workflow.ReceiveChannel, more bool) {
							c.Receive(ctx, &response)
						})

						// Wait for the response
						selector.Select(ctx)

						// Check if the response is for this target
						if response.TargetName == targetName {
							break
						}
					}

					// Handle user response
					if response.Action == "continue" {
						logger.Info("Continuing execution after pause", "target", targetName)
						// Remove target from paused targets
						delete(pausedTargets, targetName)
					} else if response.Action == "abort" {
						logger.Info("Aborting execution after pause", "target", targetName)
						// Mark target as failed and return
						targetStatuses[targetName] = targetResultFailure

						// Send to main completion channel
						targetCompletionChannels[targetName].Send(ctx, targetResultFailure)

						// Send to any dependent target channels
						if dependentChannels, ok := dependentTargetChannels[targetName]; ok {
							for depTarget, depCh := range dependentChannels {
								logger.Info("Sending failure signal to dependent target", "target", targetName, "dependentTarget", depTarget)
								depCh.Send(ctx, targetResultFailure)
							}
						}
						return
					}
				}

				// Execute step as an activity
				activityInput := &StepActivityInput{
					WorkflowID: workflowID,
					TargetName: targetName,
					Command:    cmd,
				}

				activityOptions := workflow.ActivityOptions{
					StartToCloseTimeout: time.Hour,
					RetryPolicy: &temporal.RetryPolicy{
						MaximumAttempts: 1, // No retries, fail after first attempt
					},
				}

				ctx = workflow.WithActivityOptions(ctx, activityOptions)

				var result bool
				err := workflow.ExecuteActivity(ctx, ExecuteStepActivity, activityInput).Get(ctx, &result)

				if err != nil {
					logger.Error("Step execution failed", "target", targetName, "command", cmd.String(), "error", err)
					if !cmd.ContinueOnFailure {
						// Log command failure
						logger.Info("Command failed", "target", targetName, "command", cmd.String())

						// Print instructions for the user
						logger.Info("Workflow paused due to command failure",
							"workflowID", workflowID,
							"target", targetName,
							"command", cmd.String(),
							"error", err.Error())

						// Log instructions for continuing, retrying, or aborting
						logger.Info("To continue execution, run:",
							"command", fmt.Sprintf("drtprod execute workflow --workflow-id=%s --target-name=%s --action=continue",
								workflowID, targetName))
						logger.Info("To retry the failed step, run:",
							"command", fmt.Sprintf("drtprod execute workflow --workflow-id=%s --target-name=%s --action=retry",
								workflowID, targetName))
						logger.Info("To abort execution, run:",
							"command", fmt.Sprintf("drtprod execute workflow --workflow-id=%s --target-name=%s --action=abort",
								workflowID, targetName))

						// Send message to Slack
						err = SendWorkflowPausedMessage(workflowID, targetName, cmd.String(), err)
						if err != nil {
							logger.Error("Failed to send Slack message", "error", err)
						}

						// Mark target as paused
						pausedTargets[targetName] = true

						// Wait for user response
						logger.Info("Waiting for user response", "target", targetName)
						var response CommandResponse
						for {
							// Create a selector to wait for the response signal
							selector := workflow.NewSelector(ctx)
							selector.AddReceive(commandResponseChannel, func(c workflow.ReceiveChannel, more bool) {
								c.Receive(ctx, &response)
							})

							// Wait for the response
							selector.Select(ctx)

							// Check if the response is for this target
							if response.TargetName == targetName {
								break
							}
						}

						// Handle user response
						if response.Action == "continue" {
							logger.Info("Continuing execution after failure", "target", targetName)
							// Remove target from paused targets
							delete(pausedTargets, targetName)
						} else if response.Action == "retry" {
							index--
						} else if response.Action == "abort" {
							logger.Info("Aborting execution after failure", "target", targetName)
							// Mark target as failed and return
							targetStatuses[targetName] = targetResultFailure

							// Send to main completion channel
							targetCompletionChannels[targetName].Send(ctx, targetResultFailure)

							// Send to any dependent target channels
							if dependentChannels, ok := dependentTargetChannels[targetName]; ok {
								for depTarget, depCh := range dependentChannels {
									logger.Info("Sending failure signal to dependent target", "target", targetName, "dependentTarget", depTarget)
									depCh.Send(ctx, targetResultFailure)
								}
							}
							return
						}
					}
				}

				// Wait if specified
				if cmd.Wait > 0 {
					_ = workflow.Sleep(ctx, time.Duration(cmd.Wait)*time.Second)
				}
				index++
			}

			// Mark target as successful
			targetStatuses[targetName] = targetResultSuccess
			logger.Info("Workflow completed successfully. Sending completion signal.", "target", targetName)

			// Send to main completion channel
			targetCompletionChannels[targetName].Send(ctx, targetResultSuccess)

			// Send to any dependent target channels
			if dependentChannels, ok := dependentTargetChannels[targetName]; ok {
				for depTarget, depCh := range dependentChannels {
					logger.Info("Sending completion signal to dependent target", "target", targetName, "dependentTarget", depTarget)
					depCh.Send(ctx, targetResultSuccess)
				}
			}

			// Send notification to Slack
			err = SendTargetNotification(workflowID, targetName, false)
			if err != nil {
				logger.Error("Failed to send Slack message", "error", err)
			}
		})
	}

	// Wait for all targets to complete
	selector := workflow.NewSelector(ctx)
	remainingTargets := 0

	for _, t := range input.Config.Targets {
		if !shouldSkipTarget(targetNameMap, t, input.UserProvidedTargetNames) {
			remainingTargets++
			ch := targetCompletionChannels[t.TargetName]
			selector.AddReceive(ch, func(c workflow.ReceiveChannel, more bool) {
				var status targetStatus
				c.Receive(ctx, &status)
				remainingTargets--
			})
		}
	}

	// Wait until all targets are complete
	for remainingTargets > 0 {
		selector.Select(ctx)
	}

	// Check if any target failed
	for _, t := range input.Config.Targets {
		if !shouldSkipTarget(targetNameMap, t, input.UserProvidedTargetNames) {
			if targetStatuses[t.TargetName] == targetResultFailure {
				return "Workflow completed with failures", nil
			}
		}
	}

	return "Workflow completed successfully", nil
}

// StepActivityInput contains the input for a step activity
type StepActivityInput struct {
	WorkflowID string
	TargetName string
	Command    *command
}

// ExecuteStepActivity executes a single step as an activity
func ExecuteStepActivity(ctx context.Context, input *StepActivityInput) (bool, error) {
	fmt.Printf("[%s] Executing: %s\n", input.TargetName, input.Command.String())
	err := SendStepNotification(input.WorkflowID, input.TargetName, input.Command.String(), true)
	if err != nil {
		fmt.Printf("Failed to send Slack message: %v", err)
	}

	// Execute the command
	cmd := exec.Command(input.Command.Name, input.Command.Args...)
	cmd.Stdout = os.Stdout

	// Capture stderr instead of directing it to os.Stderr
	var stderrBuf strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	// Inherit the current environment
	cmd.Env = os.Environ()

	err = cmd.Run()
	if err != nil {
		stderrOutput := stderrBuf.String()
		fmt.Printf("[%s] Command failed: %v\nStderr: %s\n", input.TargetName, err, stderrOutput)

		// Create a wrapped error that includes the stderr output
		errWithStderr := fmt.Errorf("%w\nStderr: %s", err, stderrOutput)

		return false, errWithStderr
	} else {
		err = SendStepNotification(input.WorkflowID, input.TargetName, input.Command.String(), false)
		if err != nil {
			fmt.Printf("Failed to send Slack message: %v", err)
		}
	}
	return true, nil
}
