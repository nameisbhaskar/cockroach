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

const opsStressPlanName = "pua-ops-stress"

type OpsStress struct{}

func registerOpsStressPlan(pr *planners.PlanRegistry) {
	pr.Register(&OpsStress{})
}

var _ planners.Registry = &OpsStress{}

func (o *OpsStress) PrepareExecution(_ context.Context) error {
	return nil
}

func (o *OpsStress) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (o *OpsStress) GetPlanName() string {
	return opsStressPlanName
}

func (o *OpsStress) GetPlanDescription() string {
	return "Phase-2: Internal Operational Stress - Tests cluster under operational load (backup, changefeed, index creation, upgrade)"
}

func (o *OpsStress) GetPlanVersion() int {
	return 1
}

func (o *OpsStress) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (o *OpsStress) GeneratePlan(ctx context.Context, planner planners.Planner) {
	o.registerExecutors(ctx, planner)

	// Step 1: Create backup schedule (wait 30 min after)
	createBackupTask := planner.NewExecutionTask(ctx, "create backup schedule")
	createBackupTask.ExecutorFn = createBackupSchedule

	// Step 2: Create changefeed (wait 10 min after)
	createChangefeedTask := planner.NewExecutionTask(ctx, "create changefeed")
	createChangefeedTask.ExecutorFn = createChangefeed

	// Step 3: Create index (wait 11.67 min after)
	createIndexTask := planner.NewExecutionTask(ctx, "create index")
	createIndexTask.ExecutorFn = createIndex

	// Step 4: Rolling upgrade (wait 5 min after)
	rollingUpgradeTask := planner.NewExecutionTask(ctx, "rolling upgrade")
	rollingUpgradeTask.ExecutorFn = performRollingUpgrade

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	createBackupTask.Next = createChangefeedTask
	createBackupTask.Fail = endTask

	createChangefeedTask.Next = createIndexTask
	createChangefeedTask.Fail = endTask

	createIndexTask.Next = rollingUpgradeTask
	createIndexTask.Fail = endTask

	rollingUpgradeTask.Next = endTask
	rollingUpgradeTask.Fail = endTask

	planner.RegisterPlan(ctx, createBackupTask, nil)
}

func (o *OpsStress) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "create backup schedule",
			Description: "Creates backup to GCS (30 min wait)",
			Func:        createBackupSchedule,
		},
		{
			Name:        "create changefeed",
			Description: "Creates changefeed without initial scan (10 min wait)",
			Func:        createChangefeed,
		},
		{
			Name:        "create index",
			Description: "Creates index on order table (11.67 min wait)",
			Func:        createIndex,
		},
		{
			Name:        "rolling upgrade",
			Description: "Performs rolling upgrade to new CRDB version (5 min wait)",
			Func:        performRollingUpgrade,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Executor Functions ==========

func createBackupSchedule(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement backup schedule creation
	// roachprod sql $CLUSTER:1 -- -e "BACKUP INTO 'gs://$BUCKET_US_EAST_1/$CLUSTER?AUTH=implicit' WITH OPTIONS (revision_history = true, detached)"
	// wait_after: 1800 seconds (30 min)
	return "backup-created", nil
}

func createChangefeed(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement changefeed creation
	// roachprod sql $CLUSTER:1 -- -e "CREATE CHANGEFEED FOR TABLE cct_tpcc.public.order_line INTO 'null://' WITH initial_scan = 'no'"
	// wait_after: 600 seconds (10 min)
	return "changefeed-created", nil
}

func createIndex(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement index creation
	// roachprod sql $CLUSTER:1 -- -e "CREATE INDEX add_index_o_w_id ON cct_tpcc.public.order (o_w_id)"
	// wait_after: 700 seconds (11.67 min)
	return "index-created", nil
}

func performRollingUpgrade(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement rolling upgrade
	// roachprod deploy $CLUSTER release $CRDB_UPGRADE_VERSION --pause=5m --grace-period=500
	// wait_after: 300 seconds (5 min)
	return "upgrade-completed", nil
}
