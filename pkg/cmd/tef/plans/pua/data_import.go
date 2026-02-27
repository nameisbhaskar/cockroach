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

const dataImportPlanName = "pua-data-import"

type DataImport struct{}

func registerDataImportPlan(pr *planners.PlanRegistry) {
	pr.Register(&DataImport{})
}

var _ planners.Registry = &DataImport{}

func (d *DataImport) PrepareExecution(_ context.Context) error {
	return nil
}

func (d *DataImport) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (d *DataImport) GetPlanName() string {
	return dataImportPlanName
}

func (d *DataImport) GetPlanDescription() string {
	return "Data Import - Imports TPCC data for testing"
}

func (d *DataImport) GetPlanVersion() int {
	return 1
}

func (d *DataImport) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (d *DataImport) GeneratePlan(ctx context.Context, planner planners.Planner) {
	d.registerExecutors(ctx, planner)

	// Data Import Task
	importTPCCDataTask := planner.NewExecutionTask(ctx, "import tpcc data")
	importTPCCDataTask.ExecutorFn = importTPCCData

	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	importTPCCDataTask.Next = endTask
	importTPCCDataTask.Fail = endTask

	planner.RegisterPlan(ctx, importTPCCDataTask, nil)
}

func (d *DataImport) registerExecutors(ctx context.Context, planner planners.Planner) {
	// Register executor used by this plan
	planner.RegisterExecutor(ctx, &planners.Executor{
		Name:        "import tpcc data",
		Description: "Imports TPCC data with 5000 warehouses (1 hour wait)",
		Func:        importTPCCData,
	})
}

// ========== Data Import Executor Function ==========

func importTPCCData(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement TPCC data import
	// roachprod run <workload-cluster>:1 -- "sudo systemd-run --unit tpcc_init --same-dir --uid $(id -u) --gid $(id -g) bash ./tpcc_init_cct_tpcc.sh"
	// wait_after: 3600 seconds (1 hour)
	return "tpcc-data-imported", nil
}
