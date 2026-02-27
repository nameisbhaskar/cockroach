// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package pua

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/spf13/cobra"
)

const networkFailuresPlanName = "pua-network-failures"

type NetworkFailures struct{}

func registerNetworkFailuresPlan(pr *planners.PlanRegistry) {
	pr.Register(&NetworkFailures{})
}

var _ planners.Registry = &NetworkFailures{}

func (n *NetworkFailures) PrepareExecution(_ context.Context) error {
	return nil
}

func (n *NetworkFailures) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (n *NetworkFailures) GetPlanName() string {
	return networkFailuresPlanName
}

func (n *NetworkFailures) GetPlanDescription() string {
	return "Phase-4: Network Failures - Tests partial and full network partitions"
}

func (n *NetworkFailures) GetPlanVersion() int {
	return 1
}

func (n *NetworkFailures) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (n *NetworkFailures) GeneratePlan(ctx context.Context, planner planners.Planner) {
	n.registerExecutors(ctx, planner)

	// Phase-4: Network Failures Tasks
	partialNetworkPartitionTask := planner.NewExecutionTask(ctx, "partial network partition")
	partialNetworkPartitionTask.ExecutorFn = partialNetworkPartition

	fullNetworkPartitionTask := planner.NewExecutionTask(ctx, "full network partition")
	fullNetworkPartitionTask.ExecutorFn = fullNetworkPartition

	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	partialNetworkPartitionTask.Next = fullNetworkPartitionTask
	partialNetworkPartitionTask.Fail = endTask

	fullNetworkPartitionTask.Next = endTask
	fullNetworkPartitionTask.Fail = endTask

	planner.RegisterPlan(ctx, partialNetworkPartitionTask, nil)
}

func (n *NetworkFailures) registerExecutors(ctx context.Context, planner planners.Planner) {
	// Register executors used by this plan
	for _, exe := range []*planners.Executor{
		{
			Name:        "partial network partition",
			Description: "Injects partial network partition (25 min wait)",
			Func:        partialNetworkPartition,
		},
		{
			Name:        "full network partition",
			Description: "Injects full network partition (25 min wait)",
			Func:        fullNetworkPartition,
		},
	} {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Phase-4 Executor Functions ==========

func partialNetworkPartition(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData,
) (string, error) {
	// TODO: Implement partial network partition
	// roachprod run <workload-cluster>:1 -- "./run_ops_network-partition-partial.sh"
	// wait_after: 1500 seconds (25 min)
	return "partial-network-partition-injected", nil
}

func fullNetworkPartition(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData,
) (string, error) {
	// TODO: Implement full network partition
	// roachprod run <workload-cluster>:1 -- "./run_ops_network-partition-full.sh"
	// wait_after: 1500 seconds (25 min)
	return "full-network-partition-injected", nil
}
