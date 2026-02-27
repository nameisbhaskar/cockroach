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

const workloadSetupPlanName = "pua-workload-setup"

type WorkloadSetup struct{}

func registerWorkloadSetupPlan(pr *planners.PlanRegistry) {
	pr.Register(&WorkloadSetup{})
}

var _ planners.Registry = &WorkloadSetup{}

func (w *WorkloadSetup) PrepareExecution(_ context.Context) error {
	return nil
}

func (w *WorkloadSetup) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (w *WorkloadSetup) GetPlanName() string {
	return workloadSetupPlanName
}

func (w *WorkloadSetup) GetPlanDescription() string {
	return "Workload Setup - Creates and configures workload cluster"
}

func (w *WorkloadSetup) GetPlanVersion() int {
	return 1
}

func (w *WorkloadSetup) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (w *WorkloadSetup) GeneratePlan(ctx context.Context, planner planners.Planner) {
	w.registerExecutors(ctx, planner)

	// Step 1: Create workload cluster
	createWorkloadTask := planner.NewExecutionTask(ctx, "create workload cluster")
	createWorkloadTask.ExecutorFn = createWorkloadCluster

	// Step 2: Sync roachprod
	syncTask := planner.NewExecutionTask(ctx, "sync roachprod")
	syncTask.ExecutorFn = syncWorkloadRoachprod

	// Step 3: Stage cockroach binary
	stageCockroachTask := planner.NewExecutionTask(ctx, "stage cockroach binary")
	stageCockroachTask.ExecutorFn = stageWorkloadCockroach

	// Step 4: Stage workload binary
	stageWorkloadTask := planner.NewExecutionTask(ctx, "stage workload binary")
	stageWorkloadTask.ExecutorFn = stageWorkload

	// Step 5-7: Put binaries (can be done in parallel)
	forkPutTask := planner.NewForkTask(ctx, "put binaries")
	forkEndTask := planner.NewForkJoinTask(ctx, "fork end")

	putRoachprodTask := planner.NewExecutionTask(ctx, "put roachprod binary")
	putRoachprodTask.ExecutorFn = putRoachprod

	putDrtprodTask := planner.NewExecutionTask(ctx, "put drtprod binary")
	putDrtprodTask.ExecutorFn = putDrtprod

	putRoachtestTask := planner.NewExecutionTask(ctx, "put roachtest binary")
	putRoachtestTask.ExecutorFn = putRoachtest

	// Step 8: Setup Datadog
	setupDatadogTask := planner.NewExecutionTask(ctx, "setup datadog workload monitoring")
	setupDatadogTask.ExecutorFn = setupDatadogWorkload

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	createWorkloadTask.Next = syncTask
	createWorkloadTask.Fail = endTask

	syncTask.Next = stageCockroachTask
	syncTask.Fail = endTask

	stageCockroachTask.Next = stageWorkloadTask
	stageCockroachTask.Fail = endTask

	stageWorkloadTask.Next = forkPutTask
	stageWorkloadTask.Fail = endTask

	// Fork for parallel binary deployment
	forkPutTask.Tasks = []planners.Task{
		putRoachprodTask,
		putDrtprodTask,
		putRoachtestTask,
	}
	forkPutTask.Next = setupDatadogTask
	forkPutTask.Join = forkEndTask
	forkPutTask.Fail = endTask

	// Each put task goes to fork end
	putRoachprodTask.Next = forkEndTask
	putRoachprodTask.Fail = forkEndTask
	putDrtprodTask.Next = forkEndTask
	putDrtprodTask.Fail = forkEndTask
	putRoachtestTask.Next = forkEndTask
	putRoachtestTask.Fail = forkEndTask

	setupDatadogTask.Next = endTask
	setupDatadogTask.Fail = endTask

	planner.RegisterPlan(ctx, createWorkloadTask, nil)
}

func (w *WorkloadSetup) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "create workload cluster",
			Description: "Creates workload cluster with 1 node",
			Func:        createWorkloadCluster,
		},
		{
			Name:        "sync workload roachprod",
			Description: "Syncs roachprod configuration",
			Func:        syncWorkloadRoachprod,
		},
		{
			Name:        "stage workload cockroach",
			Description: "Stages CockroachDB binary on workload nodes",
			Func:        stageWorkloadCockroach,
		},
		{
			Name:        "stage workload",
			Description: "Stages workload binary",
			Func:        stageWorkload,
		},
		{
			Name:        "put roachprod",
			Description: "Uploads roachprod binary to workload cluster",
			Func:        putRoachprod,
		},
		{
			Name:        "put drtprod",
			Description: "Uploads drtprod binary to workload cluster",
			Func:        putDrtprod,
		},
		{
			Name:        "put roachtest",
			Description: "Uploads roachtest binary to workload cluster",
			Func:        putRoachtest,
		},
		{
			Name:        "setup datadog workload monitoring",
			Description: "Configures Datadog monitoring for workload cluster",
			Func:        setupDatadogWorkload,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Executor Functions ==========

func createWorkloadCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement workload cluster creation
	// roachprod create $WORKLOAD_CLUSTER --clouds=gce --gce-zones="us-east1-c"
	// --nodes=$WORKLOAD_NODES --gce-machine-type=n2-standard-8
	// --os-volume-size=100 --username=workload --lifetime=15h
	return "workload-cluster-created", nil
}

func syncWorkloadRoachprod(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement roachprod sync
	// roachprod sync --clouds=gce
	return "roachprod-synced", nil
}

func stageWorkloadCockroach(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cockroach staging
	// roachprod stage $WORKLOAD_CLUSTER cockroach
	return "cockroach-staged", nil
}

func stageWorkload(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement workload staging
	// roachprod stage $WORKLOAD_CLUSTER workload
	return "workload-staged", nil
}

func putRoachprod(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement roachprod binary upload
	// roachprod put $WORKLOAD_CLUSTER artifacts/roachprod roachprod
	return "roachprod-uploaded", nil
}

func putDrtprod(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement drtprod binary upload
	// roachprod put $WORKLOAD_CLUSTER artifacts/drtprod drtprod
	return "drtprod-uploaded", nil
}

func putRoachtest(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement roachtest binary upload
	// roachprod put $WORKLOAD_CLUSTER:1 artifacts/roachtest roachtest
	return "roachtest-uploaded", nil
}

func setupDatadogWorkload(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement Datadog setup for workload cluster
	// Run script: pkg/cmd/drtprod/scripts/setup_datadog_workload
	return "datadog-workload-setup", nil
}
