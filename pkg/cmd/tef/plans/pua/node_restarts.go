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

const nodeRestartsPlanName = "pua-node-restarts"

type NodeRestarts struct{}

func registerNodeRestartsPlan(pr *planners.PlanRegistry) {
	pr.Register(&NodeRestarts{})
}

var _ planners.Registry = &NodeRestarts{}

func (n *NodeRestarts) PrepareExecution(_ context.Context) error {
	return nil
}

func (n *NodeRestarts) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (n *NodeRestarts) GetPlanName() string {
	return nodeRestartsPlanName
}

func (n *NodeRestarts) GetPlanDescription() string {
	return "Phase-5: Node Restarts - Tests cluster behavior under ungraceful node shutdowns and restarts"
}

func (n *NodeRestarts) GetPlanVersion() int {
	return 1
}

func (n *NodeRestarts) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (n *NodeRestarts) GeneratePlan(ctx context.Context, planner planners.Planner) {
	n.registerExecutors(ctx, planner)

	// Step 1: Stop node 2
	stopNode2Task := planner.NewExecutionTask(ctx, "stop node 2")
	stopNode2Task.ExecutorFn = stopNode2

	// Step 2: Restart node 2 (wait 10 min after)
	restartNode2Task := planner.NewExecutionTask(ctx, "restart node 2")
	restartNode2Task.ExecutorFn = restartNode2

	// Step 3: Stop node 6
	stopNode6Task := planner.NewExecutionTask(ctx, "stop node 6")
	stopNode6Task.ExecutorFn = stopNode6

	// Step 4: Restart node 6 (wait 25 min after)
	restartNode6Task := planner.NewExecutionTask(ctx, "restart node 6")
	restartNode6Task.ExecutorFn = restartNode6

	// Step 5: Stop node 7
	stopNode7Task := planner.NewExecutionTask(ctx, "stop node 7")
	stopNode7Task.ExecutorFn = stopNode7

	// Step 6: Restart node 7 (wait 25 min after)
	restartNode7Task := planner.NewExecutionTask(ctx, "restart node 7")
	restartNode7Task.ExecutorFn = restartNode7

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	stopNode2Task.Next = restartNode2Task
	stopNode2Task.Fail = endTask

	restartNode2Task.Next = stopNode6Task
	restartNode2Task.Fail = endTask

	stopNode6Task.Next = restartNode6Task
	stopNode6Task.Fail = endTask

	restartNode6Task.Next = stopNode7Task
	restartNode6Task.Fail = endTask

	stopNode7Task.Next = restartNode7Task
	stopNode7Task.Fail = endTask

	restartNode7Task.Next = endTask
	restartNode7Task.Fail = endTask

	planner.RegisterPlan(ctx, stopNode2Task, nil)
}

func (n *NodeRestarts) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "stop node 2",
			Description: "Ungracefully stops node 2 (30s wait)",
			Func:        stopNode2,
		},
		{
			Name:        "restart node 2",
			Description: "Restarts node 2 (10 min wait)",
			Func:        restartNode2,
		},
		{
			Name:        "stop node 6",
			Description: "Ungracefully stops node 6 (30s wait)",
			Func:        stopNode6,
		},
		{
			Name:        "restart node 6",
			Description: "Restarts node 6 (25 min wait)",
			Func:        restartNode6,
		},
		{
			Name:        "stop node 7",
			Description: "Ungracefully stops node 7 (30s wait)",
			Func:        stopNode7,
		},
		{
			Name:        "restart node 7",
			Description: "Restarts node 7 (25 min wait)",
			Func:        restartNode7,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Executor Functions ==========

func stopNode2(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement ungraceful node 2 shutdown
	// roachprod stop $CLUSTER:2
	// wait_after: 30 seconds
	return "node-2-stopped", nil
}

func restartNode2(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement node 2 restart
	// roachprod start $CLUSTER:2 --restart=true
	// wait_after: 600 seconds (10 min)
	return "node-2-restarted", nil
}

func stopNode6(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement ungraceful node 6 shutdown
	// roachprod stop $CLUSTER:6
	// wait_after: 30 seconds
	return "node-6-stopped", nil
}

func restartNode6(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement node 6 restart
	// roachprod start $CLUSTER:6 --restart=true
	// wait_after: 1500 seconds (25 min)
	return "node-6-restarted", nil
}

func stopNode7(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement ungraceful node 7 shutdown
	// roachprod stop $CLUSTER:7
	// wait_after: 30 seconds
	return "node-7-stopped", nil
}

func restartNode7(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement node 7 restart
	// roachprod start $CLUSTER:7 --restart=true
	// wait_after: 1500 seconds (25 min)
	return "node-7-restarted", nil
}
