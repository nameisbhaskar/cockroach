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

const certsSetupPlanName = "pua-certs-setup"

type CertsSetup struct{}

func registerCertsSetupPlan(pr *planners.PlanRegistry) {
	pr.Register(&CertsSetup{})
}

var _ planners.Registry = &CertsSetup{}

func (c *CertsSetup) PrepareExecution(_ context.Context) error {
	return nil
}

func (c *CertsSetup) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (c *CertsSetup) GetPlanName() string {
	return certsSetupPlanName
}

func (c *CertsSetup) GetPlanDescription() string {
	return "Setup Certs & SSH Keys - Configures certificates and SSH keys for secure communication"
}

func (c *CertsSetup) GetPlanVersion() int {
	return 1
}

func (c *CertsSetup) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *CertsSetup) GeneratePlan(ctx context.Context, planner planners.Planner) {
	c.registerExecutors(ctx, planner)

	// Step 1: Remove old certs directory
	removeCertsTask := planner.NewExecutionTask(ctx, "remove old certs")
	removeCertsTask.ExecutorFn = removeOldCerts

	// Step 2: Get certs from cluster
	getCertsTask := planner.NewExecutionTask(ctx, "get certs from cluster")
	getCertsTask.ExecutorFn = getCertsFromCluster

	// Step 3: Remove certs from workload cluster
	removeCertsFromWorkloadTask := planner.NewExecutionTask(ctx, "remove certs from workload cluster")
	removeCertsFromWorkloadTask.ExecutorFn = removeCertsFromWorkload

	// Step 4: Put certs to workload cluster
	putCertsTask := planner.NewExecutionTask(ctx, "put certs to workload cluster")
	putCertsTask.ExecutorFn = putCertsToWorkload

	// Step 5: Set cert permissions
	setCertPermsTask := planner.NewExecutionTask(ctx, "set cert permissions")
	setCertPermsTask.ExecutorFn = setCertPermissions

	// Step 6-8: Generate scripts (can be done in parallel)
	forkScriptsTask := planner.NewForkTask(ctx, "generate scripts")
	forkEndTask := planner.NewForkJoinTask(ctx, "fork end")

	generateTPCCInitTask := planner.NewExecutionTask(ctx, "generate tpcc init script")
	generateTPCCInitTask.ExecutorFn = generateTPCCInit

	generateTPCCRunTask := planner.NewExecutionTask(ctx, "generate tpcc run script")
	generateTPCCRunTask.ExecutorFn = generateTPCCRun

	generatePUAOpsTask := planner.NewExecutionTask(ctx, "generate pua operations script")
	generatePUAOpsTask.ExecutorFn = generatePUAOperations

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	removeCertsTask.Next = getCertsTask
	removeCertsTask.Fail = endTask

	getCertsTask.Next = removeCertsFromWorkloadTask
	getCertsTask.Fail = endTask

	removeCertsFromWorkloadTask.Next = putCertsTask
	removeCertsFromWorkloadTask.Fail = endTask

	putCertsTask.Next = setCertPermsTask
	putCertsTask.Fail = endTask

	setCertPermsTask.Next = forkScriptsTask
	setCertPermsTask.Fail = endTask

	// Fork for parallel script generation
	forkScriptsTask.Tasks = []planners.Task{
		generateTPCCInitTask,
		generateTPCCRunTask,
		generatePUAOpsTask,
	}
	forkScriptsTask.Next = endTask
	forkScriptsTask.Join = forkEndTask
	forkScriptsTask.Fail = endTask

	// Each script generation task goes to fork end
	generateTPCCInitTask.Next = forkEndTask
	generateTPCCInitTask.Fail = forkEndTask
	generateTPCCRunTask.Next = forkEndTask
	generateTPCCRunTask.Fail = forkEndTask
	generatePUAOpsTask.Next = forkEndTask
	generatePUAOpsTask.Fail = forkEndTask

	planner.RegisterPlan(ctx, removeCertsTask, nil)
}

func (c *CertsSetup) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "remove old certs",
			Description: "Removes old certificates directory",
			Func:        removeOldCerts,
		},
		{
			Name:        "get certs from cluster",
			Description: "Retrieves certificates from cluster node",
			Func:        getCertsFromCluster,
		},
		{
			Name:        "remove certs from workload cluster",
			Description: "Removes old certs from workload cluster",
			Func:        removeCertsFromWorkload,
		},
		{
			Name:        "put certs to workload cluster",
			Description: "Uploads certificates to workload cluster",
			Func:        putCertsToWorkload,
		},
		{
			Name:        "set cert permissions",
			Description: "Sets proper permissions on certificate files",
			Func:        setCertPermissions,
		},
		{
			Name:        "generate tpcc init script",
			Description: "Generates TPCC initialization script",
			Func:        generateTPCCInit,
		},
		{
			Name:        "generate tpcc run script",
			Description: "Generates TPCC run script",
			Func:        generateTPCCRun,
		},
		{
			Name:        "generate pua operations script",
			Description: "Generates PUA operations script (10s wait)",
			Func:        generatePUAOperations,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Executor Functions ==========

func removeOldCerts(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cert removal
	// rm -rf certs-$CLUSTER
	return "old-certs-removed", nil
}

func getCertsFromCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cert retrieval
	// roachprod get $CLUSTER:1 certs certs-$CLUSTER
	return "certs-retrieved", nil
}

func removeCertsFromWorkload(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement workload cert removal
	// roachprod ssh $WORKLOAD_CLUSTER -- sudo rm -rf certs
	return "workload-certs-removed", nil
}

func putCertsToWorkload(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cert upload
	// roachprod put $WORKLOAD_CLUSTER certs-$CLUSTER certs
	return "certs-uploaded", nil
}

func setCertPermissions(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cert permission setting
	// roachprod ssh $WORKLOAD_CLUSTER -- chmod 600 './certs/*'
	return "cert-perms-set", nil
}

func generateTPCCInit(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement TPCC init script generation
	// Run script: pkg/cmd/drtprod/scripts/tpcc_init.sh cct_tpcc false --warehouses=$TPCC_WAREHOUSES --db=$DB_NAME
	return "tpcc-init-generated", nil
}

func generateTPCCRun(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement TPCC run script generation
	// Run script: pkg/cmd/drtprod/scripts/generate_tpcc_run.sh cct_tpcc false
	// --db=$DB_NAME --warehouses=$TPCC_WAREHOUSES --active-warehouses=$TPCC_ACTIVE_WAREHOUSES
	// --duration=$RUN_DURATION --ramp=5m --wait=true --max-conn-lifetime=$MAX_CONN_LIFETIME --conns=$CONNS
	return "tpcc-run-generated", nil
}

func generatePUAOperations(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement PUA operations script generation (wait 10s after)
	// Run script: pkg/cmd/drtprod/scripts/pua_operations.sh
	return "pua-ops-generated", nil
}
