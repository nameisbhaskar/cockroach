// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package demo

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestDemo_GetPlanName(t *testing.T) {
	demo := &Demo{}
	require.Equal(t, "demo", demo.GetPlanName())
}

func TestDemo_GetPlanDescription(t *testing.T) {
	demo := &Demo{}
	description := demo.GetPlanDescription()
	require.NotEmpty(t, description)
	require.Contains(t, description, "demo")
}

func TestDemo_ParsePlanInput(t *testing.T) {
	demo := &Demo{}

	t.Run("valid input", func(t *testing.T) {
		input := `{"name":"test-demo"}`
		result, err := demo.ParsePlanInput(input)
		require.NoError(t, err)
		require.NotNil(t, result)

		data, ok := result.(*demoData)
		require.True(t, ok)
		require.Equal(t, "test-demo", data.Name)
	})

	t.Run("empty name", func(t *testing.T) {
		input := `{}`
		result, err := demo.ParsePlanInput(input)
		require.NoError(t, err)
		require.NotNil(t, result)

		data, ok := result.(*demoData)
		require.True(t, ok)
		require.Empty(t, data.Name)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"name":"test-demo"`
		result, err := demo.ParsePlanInput(input)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("malformed json", func(t *testing.T) {
		input := `not a json`
		result, err := demo.ParsePlanInput(input)
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestDemo_GeneratePlan(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPlanner := NewMockPlanner(ctrl)
	demo := &Demo{}
	ctx := context.Background()

	// Track registered executors
	var registeredExecutors []*planners.Executor
	mockPlanner.EXPECT().
		RegisterExecutor(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, executor *planners.Executor) {
			registeredExecutors = append(registeredExecutors, executor)
		}).
		AnyTimes()

	// Track created tasks
	var createdExecutionTasks []*planners.ExecutionTask
	mockPlanner.EXPECT().
		NewExecutionTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.ExecutionTask {
			task := &planners.ExecutionTask{}
			createdExecutionTasks = append(createdExecutionTasks, task)
			return task
		}).
		AnyTimes()

	var createdForkTasks []*planners.ForkTask
	mockPlanner.EXPECT().
		NewForkTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.ForkTask {
			task := &planners.ForkTask{}
			createdForkTasks = append(createdForkTasks, task)
			return task
		}).
		AnyTimes()

	var createdForkJoinTasks []*planners.ForkJoinTask
	mockPlanner.EXPECT().
		NewForkJoinTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.ForkJoinTask {
			task := &planners.ForkJoinTask{}
			createdForkJoinTasks = append(createdForkJoinTasks, task)
			return task
		}).
		AnyTimes()

	var createdConditionTasks []*planners.ConditionTask
	mockPlanner.EXPECT().
		NewConditionTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.ConditionTask {
			task := &planners.ConditionTask{}
			createdConditionTasks = append(createdConditionTasks, task)
			return task
		}).
		AnyTimes()

	var createdEndTasks []*planners.EndTask
	mockPlanner.EXPECT().
		NewEndTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.EndTask {
			task := &planners.EndTask{}
			createdEndTasks = append(createdEndTasks, task)
			return task
		}).
		AnyTimes()

	var createdCallbackTasks []*planners.CallbackTask
	mockPlanner.EXPECT().
		NewCallbackTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.CallbackTask {
			task := &planners.CallbackTask{}
			createdCallbackTasks = append(createdCallbackTasks, task)
			return task
		}).
		AnyTimes()

	var createdChildWorkflowTasks []*planners.ChildPlanTask
	mockPlanner.EXPECT().
		NewChildPlanTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) *planners.ChildPlanTask {
			task := &planners.ChildPlanTask{}
			createdChildWorkflowTasks = append(createdChildWorkflowTasks, task)
			return task
		}).
		AnyTimes()

	// Expect RegisterPlan to be called once
	mockPlanner.EXPECT().
		RegisterPlan(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1)

	// Execute the plan generation
	demo.GeneratePlan(ctx, mockPlanner)

	// Verify that executors were registered
	require.NotEmpty(t, registeredExecutors, "Expected executors to be registered")

	// Verify all expected executors are present
	expectedExecutors := []string{
		"initialize environment",
		"setup network",
		"configure firewall",
		"setup storage",
		"format disks",
		"check cluster readiness",
		"configure cluster settings",
		"validate configuration",
		"import TPCC workload",
		"verify data import",
		"run workload",
		"collect metrics",
		"retry data import",
		"cleanup resources",
		"sleep 1min",
		"sleep 30sec",
		"sleep 45sec",
		"get child task info",
		"initialize environment response",
	}

	executorNames := make(map[string]bool)
	for _, executor := range registeredExecutors {
		executorNames[executor.Name] = true
		require.NotNil(t, executor.Func, "Executor %s should have a Func", executor.Name)
		require.NotEmpty(t, executor.Description, "Executor %s should have a Description", executor.Name)
	}

	for _, expectedName := range expectedExecutors {
		require.True(t, executorNames[expectedName], "Expected executor %s to be registered", expectedName)
	}

	// Verify tasks were created
	require.NotEmpty(t, createdExecutionTasks, "Expected execution tasks to be created")
	require.NotEmpty(t, createdForkTasks, "Expected fork tasks to be created")
	require.NotEmpty(t, createdForkJoinTasks, "Expected fork join tasks to be created")
	require.NotEmpty(t, createdConditionTasks, "Expected if tasks to be created")
	require.NotEmpty(t, createdCallbackTasks, "Expected callback tasks to be created")
	require.NotEmpty(t, createdChildWorkflowTasks, "Expected child workflow tasks to be created")
	require.NotEmpty(t, createdEndTasks, "Expected end tasks to be created")
}

func TestDemo_ImplementsRegistry(t *testing.T) {
	var _ planners.Registry = &Demo{}
}

func TestDemoData_JSONMarshaling(t *testing.T) {
	t.Run("marshal", func(t *testing.T) {
		data := &demoData{Name: "test"}
		bytes, err := json.Marshal(data)
		require.NoError(t, err)
		require.Equal(t, `{"name":"test"}`, string(bytes))
	})

	t.Run("unmarshal", func(t *testing.T) {
		input := `{"name":"test"}`
		var data demoData
		err := json.Unmarshal([]byte(input), &data)
		require.NoError(t, err)
		require.Equal(t, "test", data.Name)
	})
}

func TestDemo_PrepareExecution(t *testing.T) {
	ctx := context.Background()

	t.Run("without slack tokens and without env vars", func(t *testing.T) {
		// Unset env vars for this test
		oldBotToken := os.Getenv("SLACK_BOT_TOKEN")
		oldChannel := os.Getenv("SLACK_CHANNEL")
		oldAppToken := os.Getenv("SLACK_APP_TOKEN")
		defer func() {
			_ = os.Setenv("SLACK_BOT_TOKEN", oldBotToken)
			_ = os.Setenv("SLACK_CHANNEL", oldChannel)
			_ = os.Setenv("SLACK_APP_TOKEN", oldAppToken)
		}()
		_ = os.Unsetenv("SLACK_BOT_TOKEN")
		_ = os.Unsetenv("SLACK_CHANNEL")
		_ = os.Unsetenv("SLACK_APP_TOKEN")

		demo := &Demo{}
		err := demo.PrepareExecution(ctx)
		// Should succeed but create disabled approver when tokens are missing
		require.NoError(t, err)
		require.NotNil(t, demo.slackApprover)
		require.False(t, demo.slackApprover.enabled)
	})

	t.Run("with slack tokens", func(t *testing.T) {
		demo := &Demo{
			slackBotToken: "xoxb-test-token",
			slackChannel:  "#test-channel",
			apiBaseUrl:    "http://tef-server.roachprod.crdb.io:25780",
		}
		err := demo.PrepareExecution(ctx)
		require.NoError(t, err)
		require.NotNil(t, demo.slackApprover)
		require.True(t, demo.slackApprover.enabled)
	})

	t.Run("without explicit tokens but env vars set", func(t *testing.T) {
		// Set env vars for this test
		oldBotToken := os.Getenv("SLACK_BOT_TOKEN")
		oldChannel := os.Getenv("SLACK_CHANNEL")
		defer func() {
			if oldBotToken == "" {
				_ = os.Unsetenv("SLACK_BOT_TOKEN")
			} else {
				_ = os.Setenv("SLACK_BOT_TOKEN", oldBotToken)
			}
			if oldChannel == "" {
				_ = os.Unsetenv("SLACK_CHANNEL")
			} else {
				_ = os.Setenv("SLACK_CHANNEL", oldChannel)
			}
		}()
		_ = os.Setenv("SLACK_BOT_TOKEN", "xoxb-env-token")
		_ = os.Setenv("SLACK_CHANNEL", "#env-channel")

		demo := &Demo{}
		err := demo.PrepareExecution(ctx)
		require.NoError(t, err)
		require.NotNil(t, demo.slackApprover)
		// Should be enabled since env vars are set
		require.True(t, demo.slackApprover.enabled)
	})
}

func TestDemo_AddStartWorkerCmdFlags(t *testing.T) {
	demo := &Demo{}
	cmd := &cobra.Command{
		Use: "test",
	}

	demo.AddStartWorkerCmdFlags(cmd)

	// Verify flags are added
	slackBotTokenFlag := cmd.Flags().Lookup("slack-bot-token")
	require.NotNil(t, slackBotTokenFlag)
	require.Equal(t, "", slackBotTokenFlag.DefValue)

	slackChannelFlag := cmd.Flags().Lookup("slack-channel")
	require.NotNil(t, slackChannelFlag)
	require.Equal(t, "", slackChannelFlag.DefValue)

	slackAppTokenFlag := cmd.Flags().Lookup("slack-app-token")
	require.NotNil(t, slackAppTokenFlag)
	require.Equal(t, "", slackAppTokenFlag.DefValue)

	apiBaseUrlFlag := cmd.Flags().Lookup("api-base-url")
	require.NotNil(t, apiBaseUrlFlag)
	require.Equal(t, "http://tef-server.roachprod.crdb.io:25780", apiBaseUrlFlag.DefValue)
}
