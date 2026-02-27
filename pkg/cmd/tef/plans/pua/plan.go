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

const puaPlanName = "pua"

// puaData contains the configuration data for the PUA test plan
type puaData struct {
	// Cluster configuration
	Cluster            string `json:"cluster"`
	ClusterNodes       int    `json:"cluster_nodes"`
	StoreCount         int    `json:"store_count"`
	Insecure           bool   `json:"insecure"`
	CRDBVersion        string `json:"crdb_version"`
	CRDBUpgradeVersion string `json:"crdb_upgrade_version"`

	// Workload cluster configuration
	WorkloadCluster string `json:"workload_cluster"`
	WorkloadNodes   int    `json:"workload_nodes"`

	// TPCC configuration
	TPCCWarehouses       int    `json:"tpcc_warehouses"`
	TPCCActiveWarehouses int    `json:"tpcc_active_warehouses"`
	DBName               string `json:"db_name"`
	RunDuration          string `json:"run_duration"`
	MaxConnLifetime      string `json:"max_conn_lifetime"`
	Conns                int    `json:"conns"`

	// Backup configuration
	BucketUSEast1 string `json:"bucket_us_east_1"`
}

type PUA struct{}

// RegisterPUAPlans registers all PUA-related plans
func RegisterPUAPlans(pr *planners.PlanRegistry) {
	pr.Register(&PUA{})
	registerClusterSetupPlan(pr)
	registerWorkloadSetupPlan(pr)
	registerCertsSetupPlan(pr)
	registerDataImportPlan(pr)
	registerBaselinePlan(pr)
	registerOpsStressPlan(pr)
	registerDiskStallsPlan(pr)
	registerNetworkFailuresPlan(pr)
	registerNodeRestartsPlan(pr)
	registerZoneOutagesPlan(pr)
	registerCleanupPlan(pr)
}

var _ planners.Registry = &PUA{}

func (p *PUA) PrepareExecution(_ context.Context) error {
	return nil
}

func (p *PUA) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (p *PUA) GetPlanName() string {
	return puaPlanName
}

func (p *PUA) GetPlanDescription() string {
	return "PUA (Planned Unplanned Availability) - Comprehensive multi-phase cluster testing"
}

func (p *PUA) GetPlanVersion() int {
	return 1
}

func (p *PUA) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PUA) GeneratePlan(ctx context.Context, planner planners.Planner) {
	p.registerExecutors(ctx, planner)

	// Create child plan tasks for each phase
	// Phase 0: Cluster Setup
	clusterSetupTask := planner.NewChildPlanTask(ctx, "cluster setup")
	clusterSetupTask.PlanName = clusterSetupPlanName
	clusterSetupTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 1: Workload Setup (depends on Cluster Setup)
	workloadSetupTask := planner.NewChildPlanTask(ctx, "workload setup")
	workloadSetupTask.PlanName = workloadSetupPlanName
	workloadSetupTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 2: Setup Certs & SSH Keys (depends on Workload Setup)
	certsSetupTask := planner.NewChildPlanTask(ctx, "setup certs & ssh keys")
	certsSetupTask.PlanName = certsSetupPlanName
	certsSetupTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 3: Data Import (depends on Certs Setup)
	dataImportTask := planner.NewChildPlanTask(ctx, "data import")
	dataImportTask.PlanName = dataImportPlanName
	dataImportTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 4: Baseline Performance (depends on Data Import)
	baselineTask := planner.NewChildPlanTask(ctx, "phase-1: baseline performance")
	baselineTask.PlanName = baselinePlanName
	baselineTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 5: Internal Operational Stress (depends on Baseline)
	opsStressTask := planner.NewChildPlanTask(ctx, "phase-2: internal operational stress")
	opsStressTask.PlanName = opsStressPlanName
	opsStressTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 6: Disk Stalls (depends on Ops Stress)
	diskStallsTask := planner.NewChildPlanTask(ctx, "phase-3: disk stalls")
	diskStallsTask.PlanName = diskStallsPlanName
	diskStallsTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 7: Network Failures (depends on Disk Stalls)
	networkFailuresTask := planner.NewChildPlanTask(ctx, "phase-4: network failures")
	networkFailuresTask.PlanName = networkFailuresPlanName
	networkFailuresTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 8: Node Restarts (depends on Network Failures)
	nodeRestartsTask := planner.NewChildPlanTask(ctx, "phase-5: node restarts")
	nodeRestartsTask.PlanName = nodeRestartsPlanName
	nodeRestartsTask.ChildTaskInfoFn = getChildTaskInfo

	// Phase 9: Zone Outages (depends on Node Restarts)
	zoneOutagesTask := planner.NewChildPlanTask(ctx, "phase-6: zone outages")
	zoneOutagesTask.PlanName = zoneOutagesPlanName
	zoneOutagesTask.ChildTaskInfoFn = getChildTaskInfo

	// Cleanup tasks
	cleanupTask := planner.NewExecutionTask(ctx, "cleanup resources")
	cleanupTask.ExecutorFn = cleanupResources

	cleanupWithErrorTask := planner.NewExecutionTask(ctx, "cleanup resources on error")
	cleanupWithErrorTask.ExecutorFn = cleanupResourcesWithError

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the sequential flow of phases
	clusterSetupTask.Next = workloadSetupTask
	clusterSetupTask.Fail = cleanupWithErrorTask

	workloadSetupTask.Next = certsSetupTask
	workloadSetupTask.Fail = cleanupWithErrorTask

	certsSetupTask.Next = dataImportTask
	certsSetupTask.Fail = cleanupWithErrorTask

	dataImportTask.Next = baselineTask
	dataImportTask.Fail = cleanupWithErrorTask

	baselineTask.Next = opsStressTask
	baselineTask.Fail = cleanupWithErrorTask

	opsStressTask.Next = diskStallsTask
	opsStressTask.Fail = cleanupWithErrorTask

	diskStallsTask.Next = networkFailuresTask
	diskStallsTask.Fail = cleanupWithErrorTask

	networkFailuresTask.Next = nodeRestartsTask
	networkFailuresTask.Fail = cleanupWithErrorTask

	nodeRestartsTask.Next = zoneOutagesTask
	nodeRestartsTask.Fail = cleanupWithErrorTask

	zoneOutagesTask.Next = cleanupTask
	zoneOutagesTask.Fail = cleanupWithErrorTask

	cleanupTask.Next = endTask
	cleanupTask.Fail = endTask

	cleanupWithErrorTask.Next = endTask
	cleanupWithErrorTask.Fail = endTask

	// Register the plan starting from cluster setup
	planner.RegisterPlan(ctx, clusterSetupTask, nil)
}

func (p *PUA) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "get child task info",
			Description: "Returns the child task info (variant and input) for child workflow task",
			Func:        getChildTaskInfo,
		},
		{
			Name:        "cleanup resources",
			Description: "Cleans up all clusters and resources",
			Func:        cleanupResources,
		},
		{
			Name:        "cleanup resources on error",
			Description: "Cleans up all clusters and resources after an error",
			Func:        cleanupResourcesWithError,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// getChildTaskInfo returns the configuration for child plan tasks
func getChildTaskInfo(
	_ context.Context, i *planners.PlanExecutionInfo, input *puaData,
) (planners.ChildTaskInfo, error) {
	return planners.ChildTaskInfo{
		PlanVariant: i.PlanVariant,
		Input:       input,
	}, nil
}

// cleanupResources destroys all clusters and cleans up resources
func cleanupResources(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *puaData,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("[PUA] Cleaning up resources", "cluster", input.Cluster, "workload_cluster", input.WorkloadCluster)

	// TODO: Implement cleanup
	// roachprod destroy $CLUSTER
	// roachprod destroy $WORKLOAD_CLUSTER
	// rm -rf certs-$CLUSTER

	return "cleanup-completed", nil
}

// cleanupResourcesWithError destroys all clusters after an error
func cleanupResourcesWithError(
	ctx context.Context, _ *planners.PlanExecutionInfo, input *puaData, errorMsg string,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)
	logger.Error("[PUA] Cleaning up resources after error", "error", errorMsg, "cluster", input.Cluster, "workload_cluster", input.WorkloadCluster)

	// TODO: Implement cleanup
	// roachprod destroy $CLUSTER
	// roachprod destroy $WORKLOAD_CLUSTER
	// rm -rf certs-$CLUSTER

	return "cleanup-completed", nil
}
