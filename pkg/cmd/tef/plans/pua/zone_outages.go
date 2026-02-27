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

const zoneOutagesPlanName = "pua-zone-outages"

type ZoneOutages struct{}

func registerZoneOutagesPlan(pr *planners.PlanRegistry) {
	pr.Register(&ZoneOutages{})
}

var _ planners.Registry = &ZoneOutages{}

func (z *ZoneOutages) PrepareExecution(_ context.Context) error {
	return nil
}

func (z *ZoneOutages) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (z *ZoneOutages) GetPlanName() string {
	return zoneOutagesPlanName
}

func (z *ZoneOutages) GetPlanDescription() string {
	return "Phase-6: Zone Outages - Tests cluster behavior during zone-level outages"
}

func (z *ZoneOutages) GetPlanVersion() int {
	return 1
}

func (z *ZoneOutages) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (z *ZoneOutages) GeneratePlan(ctx context.Context, planner planners.Planner) {
	z.registerExecutors(ctx, planner)

	// Phase-6: Zone Outages Tasks
	stopZoneNodesTask := planner.NewExecutionTask(ctx, "stop zone nodes 7-9")
	stopZoneNodesTask.ExecutorFn = stopZoneNodes

	startZoneNodesTask := planner.NewExecutionTask(ctx, "start zone nodes 7-9")
	startZoneNodesTask.ExecutorFn = startZoneNodes

	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	stopZoneNodesTask.Next = startZoneNodesTask
	stopZoneNodesTask.Fail = endTask

	startZoneNodesTask.Next = endTask
	startZoneNodesTask.Fail = endTask

	planner.RegisterPlan(ctx, stopZoneNodesTask, nil)
}

func (z *ZoneOutages) registerExecutors(ctx context.Context, planner planners.Planner) {
	// Register executors used by this plan
	for _, exe := range []*planners.Executor{
		{
			Name:        "stop zone nodes 7-9",
			Description: "Stops nodes 7-9 to simulate zone outage (5 min wait)",
			Func:        stopZoneNodes,
		},
		{
			Name:        "start zone nodes 7-9",
			Description: "Restarts zone nodes 7-9 (55 min wait)",
			Func:        startZoneNodes,
		},
	} {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Phase-6 Executor Functions ==========

func stopZoneNodes(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData,
) (string, error) {
	// TODO: Implement zone nodes stop
	// roachprod stop $CLUSTER:7-9
	// wait_after: 300 seconds (5 min)
	return "zone-nodes-stopped", nil
}

func startZoneNodes(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData,
) (string, error) {
	// TODO: Implement zone nodes restart
	// roachprod start $CLUSTER:7-9 --restart=true
	// wait_after: 3300 seconds (55 min)
	return "zone-nodes-started", nil
}
