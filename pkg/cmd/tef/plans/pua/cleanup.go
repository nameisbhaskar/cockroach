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

const cleanupPlanName = "pua-cleanup"

type Cleanup struct{}

func registerCleanupPlan(pr *planners.PlanRegistry) {
	pr.Register(&Cleanup{})
}

var _ planners.Registry = &Cleanup{}

func (c *Cleanup) PrepareExecution(_ context.Context) error {
	return nil
}

func (c *Cleanup) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (c *Cleanup) GetPlanName() string {
	return cleanupPlanName
}

func (c *Cleanup) GetPlanDescription() string {
	return "Cleanup - Destroys clusters and cleans up resources"
}

func (c *Cleanup) GetPlanVersion() int {
	return 1
}

func (c *Cleanup) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Cleanup) GeneratePlan(ctx context.Context, planner planners.Planner) {
	c.registerExecutors(ctx, planner)

	// Fork to destroy clusters in parallel
	forkDestroyTask := planner.NewForkTask(ctx, "destroy clusters")
	forkEndTask := planner.NewForkJoinTask(ctx, "fork end")

	// Destroy main cluster
	destroyClusterTask := planner.NewExecutionTask(ctx, "destroy main cluster")
	destroyClusterTask.ExecutorFn = destroyCluster

	// Destroy workload cluster
	destroyWorkloadTask := planner.NewExecutionTask(ctx, "destroy workload cluster")
	destroyWorkloadTask.ExecutorFn = destroyWorkloadCluster

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	forkDestroyTask.Tasks = []planners.Task{
		destroyClusterTask,
		destroyWorkloadTask,
	}
	forkDestroyTask.Next = endTask
	forkDestroyTask.Join = forkEndTask
	forkDestroyTask.Fail = endTask

	// Each destroy task goes to fork end
	destroyClusterTask.Next = forkEndTask
	destroyClusterTask.Fail = forkEndTask
	destroyWorkloadTask.Next = forkEndTask
	destroyWorkloadTask.Fail = forkEndTask

	planner.RegisterPlan(ctx, forkDestroyTask, nil)
}

func (c *Cleanup) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "destroy main cluster",
			Description: "Destroys the main CockroachDB cluster",
			Func:        destroyCluster,
		},
		{
			Name:        "destroy workload cluster",
			Description: "Destroys the workload cluster",
			Func:        destroyWorkloadCluster,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Executor Functions ==========

func destroyCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cluster destruction
	// roachprod destroy $CLUSTER
	return "cluster-destroyed", nil
}

func destroyWorkloadCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement workload cluster destruction
	// roachprod destroy $WORKLOAD_CLUSTER
	return "workload-cluster-destroyed", nil
}
