// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/spf13/cobra"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/sdk"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
)

const demoPlanName = "demo"

type Demo struct {
	slackBotToken string
	slackChannel  string
	slackAppToken string
	apiBaseUrl    string
	slackApprover *SlackApprover
}

func RegisterDemoPlans(pr *planners.PlanRegistry) {
	pr.Register(&Demo{})
	RegisterClusterSetupPlans(pr)
}

var _ planners.Registry = &Demo{}

func (d *Demo) PrepareExecution(_ context.Context) (err error) {
	// Initialize Slack approver
	d.slackApprover, err = NewSlackApprover(d.slackBotToken, d.slackChannel, d.slackAppToken, d.apiBaseUrl)
	if err != nil {
		return fmt.Errorf("failed to create Slack approver: %w", err)
	}
	return
}
func (d *Demo) GetPlanName() string {
	return demoPlanName
}

func (d *Demo) GetPlanDescription() string {
	return "Comprehensive demo showcasing all task types with random delays and failures"
}

func (d *Demo) GetPlanVersion() int {
	return 1
}

type demoData struct {
	Name string `json:"name,omitempty"`
}

func (d *Demo) ParsePlanInput(input string) (interface{}, error) {
	data := &demoData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (d *Demo) AddStartWorkerCmdFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&d.slackBotToken, "slack-bot-token", "",
		"Slack bot token for sending approval messages (can also use SLACK_BOT_TOKEN env var)")
	cmd.Flags().StringVar(&d.slackChannel, "slack-channel", "",
		"Slack channel to send approval messages to (can also use SLACK_CHANNEL env var)")
	cmd.Flags().StringVar(&d.slackAppToken, "slack-app-token", "",
		"Slack app-level token for Socket Mode (can also use SLACK_APP_TOKEN env var)")
	cmd.Flags().StringVar(&d.apiBaseUrl, "api-base-url", "http://localhost:8081", "API server base url")
}

func (d *Demo) GeneratePlan(ctx context.Context, p planners.Planner) {
	d.registerExecutors(ctx, p)
	// Task 1: Initialize environment (succeeds with 2min delay)
	initTask := p.NewCallbackTask(ctx, "initialize environment")
	initTask.ExecutionFn = d.initializeEnvironment
	initTask.ResultProcessorFn = d.initializeEnvironmentResult

	// Task 2: Fork - Setup infrastructure in parallel
	forkSetupTask := p.NewForkTask(ctx, "parallel infrastructure setup")
	forkEndTask := p.NewForkJoinTask(ctx, "fork end")

	// Fork Branch 1: Network setup
	setupNetworkTask := p.NewExecutionTask(ctx, "setup network")
	setupNetworkTask.ExecutorFn = setupNetwork

	configureFirewallTask := p.NewExecutionTask(ctx, "configure firewall")
	configureFirewallTask.ExecutorFn = configureFirewall

	// Fork Branch 2: Storage setup
	setupStorageTask := p.NewExecutionTask(ctx, "setup storage")
	setupStorageTask.ExecutorFn = setupStorage

	formatDisksTask := p.NewExecutionTask(ctx, "format disks")
	formatDisksTask.ExecutorFn = formatDisks

	// Task 3: Sleep for 1 minute
	sleep1Task := p.NewExecutionTask(ctx, "wait for infrastructure")
	sleep1Task.ExecutorFn = sleepFor1Min

	// Task 5: Setup cluster (succeeds with 3min delay)
	setupClusterTask := p.NewChildPlanTask(ctx, "setup cluster")
	setupClusterTask.PlanName = "cluster-setup"
	setupClusterTask.ChildTaskInfoFn = getChildTaskInfo
	setupClusterTask.Params = []planners.Task{initTask}

	// Task 6: Check for cluster readiness (random true/false)
	checkReadyTask := p.NewConditionTask(ctx, "check cluster readiness")
	checkReadyTask.ExecutorFn = checkClusterReady

	// Task 7: Configure cluster settings (succeeds with 2min delay)
	configureTask := p.NewExecutionTask(ctx, "configure cluster settings")
	configureTask.ExecutorFn = configureCluster

	// Task 8: Sleep for 30 seconds
	sleep2Task := p.NewExecutionTask(ctx, "wait before validation")
	sleep2Task.ExecutorFn = sleepFor30Sec

	// Task 9: Validate configuration (randomly fails ~50% of time)
	validateTask := p.NewExecutionTask(ctx, "validate configuration")
	validateTask.ExecutorFn = validateConfiguration

	// Task 10: Import TPCC workload (succeeds with 5min delay)
	importTPCC := p.NewExecutionTask(ctx, "import TPCC workload")
	importTPCC.ExecutorFn = importWorkload

	// Task 11: Check if data import succeeded (random true/false)
	checkImportTask := p.NewConditionTask(ctx, "verify data import")
	checkImportTask.ExecutorFn = verifyDataImport

	// Task 12: Run workload (succeeds with 3min delay)
	runWorkloadTask := p.NewExecutionTask(ctx, "run workload")
	runWorkloadTask.ExecutorFn = runWorkload

	// Task 13: Sleep for 45 seconds
	sleep3Task := p.NewExecutionTask(ctx, "wait for metrics")
	sleep3Task.ExecutorFn = sleepFor45Sec

	// Task 14: Collect metrics (succeeds with 1min delay)
	collectMetricsTask := p.NewExecutionTask(ctx, "collect metrics")
	collectMetricsTask.ExecutorFn = collectMetrics

	// Task 15: Retry import (alternative path, 2min delay)
	retryImportTask := p.NewExecutionTask(ctx, "retry data import")
	retryImportTask.ExecutorFn = retryImport

	// Task 16: Cleanup resources (succeeds with 1min delay)
	cleanupTask := p.NewExecutionTask(ctx, "cleanup resources")
	cleanupTask.ExecutorFn = cleanupResources
	cleanupTaskError := p.NewExecutionTask(ctx, "cleanup resources with error")
	cleanupTaskError.ExecutorFn = cleanupResourcesWithError

	// End task
	endTask := p.NewEndTask(ctx, "end")

	// Wire up the flow:
	// initTask -> forkSetupTask
	initTask.Next = forkSetupTask
	initTask.Fail = cleanupTaskError

	// Fork setup with parallel branches
	forkSetupTask.Tasks = []planners.Task{
		setupNetworkTask,
		setupStorageTask,
	}
	forkSetupTask.Next = sleep1Task
	forkSetupTask.Join = forkEndTask
	forkSetupTask.Fail = cleanupTaskError

	// Fork Branch 1: Network setup chain
	setupNetworkTask.Next = configureFirewallTask
	setupNetworkTask.Fail = forkEndTask
	configureFirewallTask.Next = forkEndTask
	configureFirewallTask.Fail = forkEndTask

	// Fork Branch 2: Storage setup chain
	setupStorageTask.Next = formatDisksTask
	setupStorageTask.Fail = forkEndTask
	formatDisksTask.Next = forkEndTask
	formatDisksTask.Fail = forkEndTask

	// After fork completes
	sleep1Task.Next = setupClusterTask
	sleep1Task.Fail = cleanupTaskError

	setupClusterTask.Next = checkReadyTask
	setupClusterTask.Fail = endTask

	// checkReadyTask branches:
	// - Then: configureTask -> sleep2Task -> validateTask
	// - Else: retryImportTask -> endTask
	checkReadyTask.Then = configureTask
	checkReadyTask.Else = retryImportTask

	configureTask.Next = sleep2Task
	configureTask.Fail = endTask

	sleep2Task.Next = validateTask

	// validateTask can fail (goes to cleanup) or succeed (goes to importTPCC)
	validateTask.Next = importTPCC
	validateTask.Fail = cleanupTaskError

	importTPCC.Next = checkImportTask
	importTPCC.Fail = endTask

	// checkImportTask branches:
	// - Then: runWorkloadTask -> sleep3Task -> collectMetricsTask -> endTask
	// - Else: cleanupTask -> endTask
	checkImportTask.Then = runWorkloadTask
	checkImportTask.Else = cleanupTask

	runWorkloadTask.Next = sleep3Task
	runWorkloadTask.Fail = cleanupTaskError

	sleep3Task.Next = collectMetricsTask

	collectMetricsTask.Next = endTask
	collectMetricsTask.Fail = endTask

	// Alternative paths
	retryImportTask.Next = endTask
	retryImportTask.Fail = endTask

	cleanupTask.Next = endTask
	cleanupTask.Fail = endTask
	cleanupTaskError.Fail = endTask

	p.RegisterPlan(ctx, initTask, initTask)
}

func (d *Demo) registerExecutors(ctx context.Context, p planners.Planner) {
	for _, exe := range []*planners.Executor{
		{
			Name:        "initialize environment",
			Description: "Initializes the demo environment",
			Func:        d.initializeEnvironment,
			RetryConfig: &planners.RetryConfig{},
		},
		{
			Name:        "initialize environment response",
			Description: "Initializes the demo environment response",
			Func:        d.initializeEnvironmentResult,
		},
		{
			Name:        "setup network",
			Description: "Sets up network infrastructure",
			Func:        setupNetwork,
		},
		{
			Name:        "configure firewall",
			Description: "Configures firewall rules",
			Func:        configureFirewall,
		},
		{
			Name:        "setup storage",
			Description: "Sets up storage volumes",
			Func:        setupStorage,
		},
		{
			Name:        "format disks",
			Description: "Formats storage disks",
			Func:        formatDisks,
		},
		{
			Name:        "check cluster readiness",
			Description: "Checks if cluster is ready (random result)",
			Func:        checkClusterReady,
		},
		{
			Name:        "configure cluster settings",
			Description: "Configures cluster settings",
			Func:        configureCluster,
		},
		{
			Name:        "validate configuration",
			Description: "Validates configuration (may fail randomly)",
			Func:        validateConfiguration,
		},
		{
			Name:        "import TPCC workload",
			Description: "Imports TPCC workload data",
			Func:        importWorkload,
		},
		{
			Name:        "verify data import",
			Description: "Verifies data import success (random result)",
			Func:        verifyDataImport,
		},
		{
			Name:        "run workload",
			Description: "Runs the workload against the cluster",
			Func:        runWorkload,
		},
		{
			Name:        "collect metrics",
			Description: "Collects performance metrics",
			Func:        collectMetrics,
		},
		{
			Name:        "retry data import",
			Description: "Retries data import operation",
			Func:        retryImport,
		},
		{
			Name:        "cleanup resources",
			Description: "Cleans up allocated resources",
			Func:        cleanupResources,
		},
		{
			Name:        "cleanup resources error",
			Description: "Cleans up allocated resources with error",
			Func:        cleanupResourcesWithError,
		},
		{
			Name:        "sleep 1min",
			Description: "Sleeps for 1 minute",
			Func:        sleepFor1Min,
		},
		{
			Name:        "sleep 30sec",
			Description: "Sleeps for 30 seconds",
			Func:        sleepFor30Sec,
		},
		{
			Name:        "sleep 45sec",
			Description: "Sleeps for 45 seconds",
			Func:        sleepFor45Sec,
		},
		{
			Name:        "get child task info",
			Description: "Returns the child task info (variant and input) for child workflow task",
			Func:        getChildTaskInfo,
		},
	} {
		p.RegisterExecutor(ctx, exe)
	}
}

// Executor functions

func (d *Demo) initializeEnvironment(
	ctx context.Context, info *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Initializing environment", "name", input.Name)

	// If Slack is enabled, send approval request
	if d.slackApprover != nil && d.slackApprover.enabled {
		message := fmt.Sprintf("*Environment Initialization Request*\n\nEnvironment: `%s`\n\nPlease approve or reject this initialization.", input.Name)

		// Note: workflow ID is not available in the execution function
		// The Slack message will include instructions on how to resume the workflow
		stepID, err := d.slackApprover.SendApprovalRequest(ctx, info, message)
		if err != nil {
			logger.Error("[DEMO] Failed to send Slack approval, proceeding without approval", "error", err)
			return "env_id_no_slack", nil
		}

		logger.Info("[DEMO] Sent Slack approval request. Workflow will wait for external signal via ResumeTask API.")
		return stepID, nil
	}

	// If Slack is not enabled, auto-approve after 1 minute
	// This makes the demo self-contained and doesn't require manual API calls
	logger.Info("[DEMO] Slack not enabled, will auto-approve after 1 minute to demonstrate callback workflow")

	// Start goroutine to automatically resume the workflow after 1 minute
	go d.autoResumeCallback(info, "env_id_no_slack", "approved")

	return "env_id_no_slack", nil
}

// autoResumeCallback automatically resumes a callback task after a delay.
// This is used when Slack is not configured to make the demo self-contained.
func (d *Demo) autoResumeCallback(info *planners.PlanExecutionInfo, stepID, result string) {
	// Wait 1 minute before auto-resuming
	time.Sleep(1 * time.Minute)

	// Create SDK client to call the resume API
	client := sdk.NewClient(d.apiBaseUrl)

	// Call the resume API
	ctx := context.Background()
	logger := planners.NewLogger("info")
	logger.Info("[DEMO] Auto-resuming callback task (Slack not configured)",
		"plan_id", info.PlanID,
		"workflow_id", info.WorkflowID,
		"step_id", stepID,
		"result", result)

	_, err := client.ResumePlan(ctx, info.PlanID, info.WorkflowID, stepID, result)
	if err != nil {
		logger.Error("[DEMO] Failed to auto-resume callback task", "error", err)
		return
	}

	logger.Info("[DEMO] Successfully auto-resumed callback task")
}

func (d *Demo) initializeEnvironmentResult(
	ctx context.Context,
	info *planners.PlanExecutionInfo,
	input *demoData,
	stepID, signalResult string,
) (*ProvisionData, error) {
	logger := planners.LoggerFromContext(ctx)

	// If Slack is enabled and we have a step ID, process the approval result
	if d.slackApprover != nil && d.slackApprover.enabled && stepID != "env_id_no_slack" {
		logger.Info("[DEMO] Processing Slack approval result", "step_id", stepID)
		logger.Info("[DEMO] Received result via ResumeTask API", "result", signalResult)

		if signalResult == "approved" {
			logger.Info("[DEMO] Environment initialization APPROVED", "name", input.Name)
			return &ProvisionData{Name: signalResult}, nil
		} else {
			logger.Info("[DEMO] Environment initialization REJECTED", "name", input.Name)
			return nil, fmt.Errorf("environment initialization was rejected by user")
		}
	}

	// If Slack is not enabled, just proceed
	logger.Info("[DEMO] Initialized environment (auto-approved after 1 minute)", "name", input.Name)
	return &ProvisionData{Name: "child plan"}, nil
}

func setupNetwork(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Setting up network (delay: 1.5min)", "name", input.Name)
	time.Sleep(3 * time.Second)
	return "network-configured", nil
}

func configureFirewall(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Configuring firewall (delay: 1min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "firewall-configured", nil
}

func setupStorage(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Setting up storage (delay: 2min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "storage-setup", nil
}

func formatDisks(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Formatting disks (delay: 1min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "disks-formatted", nil
}

func sleepFor1Min(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Sleeping for 1 minute", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "sleep-completed", nil
}

func checkClusterReady(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (bool, error) {
	logger := planners.LoggerFromContext(ctx)
	// Random true/false
	ready := rand.Intn(2) == 0
	logger.Info("[DEMO] Checking cluster readiness", "name", input.Name, "ready", ready)
	time.Sleep(3 * time.Second)
	return ready, nil
}

func configureCluster(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Configuring cluster settings (delay: 2min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "config-applied", nil
}

func sleepFor30Sec(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Sleeping for 30 seconds", "name", input.Name)
	time.Sleep(3 * time.Second)
	return "sleep-completed", nil
}

func validateConfiguration(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Validating configuration (delay: 1.5min)", "name", input.Name)
	time.Sleep(9 * time.Second)

	// Random failure ~50% of the time
	if rand.Intn(2) == 0 {
		return "", fmt.Errorf("validation failed: configuration mismatch detected")
	}
	return "validation-passed", nil
}

func importWorkload(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Importing TPCC workload (delay: 5min)", "name", input.Name)
	time.Sleep(5 * time.Second)
	return "tpcc-imported", nil
}

func verifyDataImport(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (bool, error) {
	logger := planners.LoggerFromContext(ctx)
	// Random true/false
	verified := rand.Intn(2) == 0
	logger.Info("[DEMO] Verifying data import", "name", input.Name, "verified", verified)
	time.Sleep(45 * time.Second)
	return verified, nil
}

func runWorkload(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Running workload (delay: 3min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "workload-completed", nil
}

func sleepFor45Sec(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Sleeping for 45 seconds", "name", input.Name)
	time.Sleep(45 * time.Second)
	return "sleep-completed", nil
}

func collectMetrics(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Collecting metrics (delay: 1min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "metrics-collected", nil
}

func retryImport(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Retrying data import (delay: 2min)", "name", input.Name)
	time.Sleep(1 * time.Second)

	// Random failure ~30% of the time
	if rand.Intn(10) < 3 {
		return "", fmt.Errorf("retry import failed: timeout waiting for cluster")
	}
	return "retry-import-success", nil
}

func cleanupResources(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Cleaning up resources (delay: 1min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "cleanup-completed", nil
}

func cleanupResourcesWithError(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData, errorMsg string,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Cleaning up resources (delay: 1min)", "name", input.Name)
	time.Sleep(1 * time.Second)
	return "cleanup-completed", nil
}

func getChildTaskInfo(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *demoData, childInput *ProvisionData,
) (planners.ChildTaskInfo, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[DEMO] Getting child task info", "name", input.Name)

	return planners.ChildTaskInfo{
		PlanVariant: "dev",
		Input:       childInput,
	}, nil
}
