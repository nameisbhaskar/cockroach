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

const diskStallsPlanName = "pua-disk-stalls"

type DiskStalls struct{}

func registerDiskStallsPlan(pr *planners.PlanRegistry) {
	pr.Register(&DiskStalls{})
}

var _ planners.Registry = &DiskStalls{}

func (d *DiskStalls) PrepareExecution(_ context.Context) error {
	return nil
}

func (d *DiskStalls) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (d *DiskStalls) GetPlanName() string {
	return diskStallsPlanName
}

func (d *DiskStalls) GetPlanDescription() string {
	return "Phase-3: Disk Stalls - Tests cluster behavior under disk stall conditions"
}

func (d *DiskStalls) GetPlanVersion() int {
	return 1
}

func (d *DiskStalls) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (d *DiskStalls) GeneratePlan(ctx context.Context, planner planners.Planner) {
	d.registerExecutors(ctx, planner)

	// Phase-3: Disk Stalls Task
	injectDiskStallsTask := planner.NewExecutionTask(ctx, "inject disk stalls")
	injectDiskStallsTask.ExecutorFn = injectDiskStalls

	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	injectDiskStallsTask.Next = endTask
	injectDiskStallsTask.Fail = endTask

	planner.RegisterPlan(ctx, injectDiskStallsTask, nil)
}

func (d *DiskStalls) registerExecutors(ctx context.Context, planner planners.Planner) {
	// Register executor used by this plan
	planner.RegisterExecutor(ctx, &planners.Executor{
		Name:        "inject disk stalls",
		Description: "Injects disk stall scenarios (20 min wait)",
		Func:        injectDiskStalls,
	})
}

// ========== Phase-3 Executor Function ==========

func injectDiskStalls(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData,
) (string, error) {
	// TODO: Implement disk stall injection
	// roachprod run <workload-cluster>:1 -- "./run_ops_disk-stall.sh"
	// wait_after: 1200 seconds (20 min)
	return "disk-stalls-injected", nil
}
