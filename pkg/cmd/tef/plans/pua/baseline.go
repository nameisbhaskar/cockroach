// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package pua

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
)

const baselinePlanName = "pua-baseline"

type Baseline struct{}

func registerBaselinePlan(pr *planners.PlanRegistry) {
	pr.Register(&Baseline{})
}

var _ planners.Registry = &Baseline{}

func (b *Baseline) PrepareExecution(_ context.Context) error {
	return nil
}

func (b *Baseline) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (b *Baseline) GetPlanName() string {
	return baselinePlanName
}

func (b *Baseline) GetPlanDescription() string {
	return "Phase-1: Baseline Performance - Establishes baseline metrics"
}

func (b *Baseline) GetPlanVersion() int {
	return 1
}

func (b *Baseline) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (b *Baseline) GeneratePlan(ctx context.Context, planner planners.Planner) {
	b.registerExecutors(ctx, planner)

	// Phase-1: Baseline Performance Task
	runBaselineWorkloadTask := planner.NewExecutionTask(ctx, "run baseline workload")
	runBaselineWorkloadTask.ExecutorFn = runBaselineWorkload

	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	runBaselineWorkloadTask.Next = endTask
	runBaselineWorkloadTask.Fail = endTask

	planner.RegisterPlan(ctx, runBaselineWorkloadTask, nil)
}

func (b *Baseline) registerExecutors(ctx context.Context, planner planners.Planner) {
	// Register executor used by this plan
	planner.RegisterExecutor(ctx, &planners.Executor{
		Name:        "run baseline workload",
		Description: "Runs baseline TPCC workload for 1 hour",
		Func:        runBaselineWorkload,
	})
}

// ========== Phase-1 Executor Function ==========

func runBaselineWorkload(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData,
) (string, error) {
	// TODO: Implement baseline workload execution
	// roachprod run <workload-cluster> -- "sudo systemd-run --unit tpcc_run --same-dir --uid $(id -u) --gid $(id -g) bash ./tpcc_run_cct_tpcc.sh"
	// wait_after: 3600 seconds (1 hour)
	return "baseline-workload-completed", nil
}
