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

const clusterSetupPlanName = "pua-cluster-setup"

type ClusterSetup struct{}

func registerClusterSetupPlan(pr *planners.PlanRegistry) {
	pr.Register(&ClusterSetup{})
}

var _ planners.Registry = &ClusterSetup{}

func (c *ClusterSetup) PrepareExecution(_ context.Context) error {
	return nil
}

func (c *ClusterSetup) AddStartWorkerCmdFlags(_ *cobra.Command) {
}

func (c *ClusterSetup) GetPlanName() string {
	return clusterSetupPlanName
}

func (c *ClusterSetup) GetPlanDescription() string {
	return "Cluster Setup - Creates and configures a multi-node CockroachDB cluster"
}

func (c *ClusterSetup) GetPlanVersion() int {
	return 1
}

func (c *ClusterSetup) ParsePlanInput(input string) (interface{}, error) {
	data := &puaData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *ClusterSetup) GeneratePlan(ctx context.Context, planner planners.Planner) {
	c.registerExecutors(ctx, planner)

	// Step 1: Create cluster
	createClusterTask := planner.NewExecutionTask(ctx, "create cluster")
	createClusterTask.ExecutorFn = createCluster

	// Step 2: Sync roachprod
	syncRoachprodTask := planner.NewExecutionTask(ctx, "sync roachprod")
	syncRoachprodTask.ExecutorFn = syncRoachprod

	// Step 3: Stage CockroachDB binary
	stageBinaryTask := planner.NewExecutionTask(ctx, "stage cockroach binary")
	stageBinaryTask.ExecutorFn = stageCockroachBinary

	// Step 4: Setup dmsetup disk staller script
	setupDiskStallerTask := planner.NewExecutionTask(ctx, "setup disk staller")
	setupDiskStallerTask.ExecutorFn = setupDiskStaller

	// Step 5: Setup Datadog monitoring
	setupDatadogTask := planner.NewExecutionTask(ctx, "setup datadog cluster monitoring")
	setupDatadogTask.ExecutorFn = setupDatadogCluster

	// Step 6: Start CockroachDB cluster
	startClusterTask := planner.NewExecutionTask(ctx, "start cockroach cluster")
	startClusterTask.ExecutorFn = startCluster

	// Step 7-11: Configure cluster settings (can be done in parallel)
	forkConfigTask := planner.NewForkTask(ctx, "configure cluster settings")
	forkEndTask := planner.NewForkJoinTask(ctx, "fork end")

	// Config branch 1: Enable rangefeed
	enableRangefeedTask := planner.NewExecutionTask(ctx, "enable rangefeed")
	enableRangefeedTask.ExecutorFn = enableRangefeed

	// Config branch 2: Set shutdown timeout
	setShutdownTimeoutTask := planner.NewExecutionTask(ctx, "set shutdown timeout")
	setShutdownTimeoutTask.ExecutorFn = setShutdownTimeout

	// Config branch 3: Set drain wait
	setDrainWaitTask := planner.NewExecutionTask(ctx, "set drain wait")
	setDrainWaitTask.ExecutorFn = setDrainWait

	// Config branch 4: Enable goschedstats
	enableGoschedstatsTask := planner.NewExecutionTask(ctx, "enable goschedstats")
	enableGoschedstatsTask.ExecutorFn = enableGoschedstats

	// Config branch 5: Enable write buffering
	enableWriteBufferingTask := planner.NewExecutionTask(ctx, "enable write buffering")
	enableWriteBufferingTask.ExecutorFn = enableWriteBuffering

	// Step 12: Create load balancer
	createLBTask := planner.NewExecutionTask(ctx, "create load balancer")
	createLBTask.ExecutorFn = createLoadBalancer

	// End task
	endTask := planner.NewEndTask(ctx, "end")

	// Wire up the flow
	createClusterTask.Next = syncRoachprodTask
	createClusterTask.Fail = endTask

	syncRoachprodTask.Next = stageBinaryTask
	syncRoachprodTask.Fail = endTask

	stageBinaryTask.Next = setupDiskStallerTask
	stageBinaryTask.Fail = endTask

	setupDiskStallerTask.Next = setupDatadogTask
	setupDiskStallerTask.Fail = endTask

	setupDatadogTask.Next = startClusterTask
	setupDatadogTask.Fail = endTask

	startClusterTask.Next = forkConfigTask
	startClusterTask.Fail = endTask

	// Fork for parallel configuration
	forkConfigTask.Tasks = []planners.Task{
		enableRangefeedTask,
		setShutdownTimeoutTask,
		setDrainWaitTask,
		enableGoschedstatsTask,
		enableWriteBufferingTask,
	}
	forkConfigTask.Next = createLBTask
	forkConfigTask.Join = forkEndTask
	forkConfigTask.Fail = endTask

	// Each config task goes to fork end
	enableRangefeedTask.Next = forkEndTask
	enableRangefeedTask.Fail = forkEndTask
	setShutdownTimeoutTask.Next = forkEndTask
	setShutdownTimeoutTask.Fail = forkEndTask
	setDrainWaitTask.Next = forkEndTask
	setDrainWaitTask.Fail = forkEndTask
	enableGoschedstatsTask.Next = forkEndTask
	enableGoschedstatsTask.Fail = forkEndTask
	enableWriteBufferingTask.Next = forkEndTask
	enableWriteBufferingTask.Fail = forkEndTask

	createLBTask.Next = endTask
	createLBTask.Fail = endTask

	planner.RegisterPlan(ctx, createClusterTask, nil)
}

func (c *ClusterSetup) registerExecutors(ctx context.Context, planner planners.Planner) {
	executors := []*planners.Executor{
		{
			Name:        "create cluster",
			Description: "Creates a 9-node GCE cluster",
			Func:        createCluster,
		},
		{
			Name:        "sync roachprod",
			Description: "Syncs roachprod configuration",
			Func:        syncRoachprod,
		},
		{
			Name:        "stage cockroach binary",
			Description: "Stages CockroachDB binary on cluster nodes",
			Func:        stageCockroachBinary,
		},
		{
			Name:        "setup disk staller",
			Description: "Sets up dmsetup disk staller script",
			Func:        setupDiskStaller,
		},
		{
			Name:        "setup datadog cluster monitoring",
			Description: "Configures Datadog monitoring for cluster",
			Func:        setupDatadogCluster,
		},
		{
			Name:        "start cockroach cluster",
			Description: "Starts CockroachDB cluster with configured settings",
			Func:        startCluster,
		},
		{
			Name:        "enable rangefeed",
			Description: "Enables rangefeed cluster setting",
			Func:        enableRangefeed,
		},
		{
			Name:        "set shutdown timeout",
			Description: "Sets shutdown connection timeout",
			Func:        setShutdownTimeout,
		},
		{
			Name:        "set drain wait",
			Description: "Sets drain wait time",
			Func:        setDrainWait,
		},
		{
			Name:        "enable goschedstats",
			Description: "Enables goschedstats short sample period",
			Func:        enableGoschedstats,
		},
		{
			Name:        "enable write buffering",
			Description: "Enables transaction write buffering",
			Func:        enableWriteBuffering,
		},
		{
			Name:        "create load balancer",
			Description: "Creates load balancer for cluster",
			Func:        createLoadBalancer,
		},
	}

	for _, exe := range executors {
		planner.RegisterExecutor(ctx, exe)
	}
}

// ========== Executor Functions ==========

func createCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cluster creation
	// roachprod create $CLUSTER --clouds=gce --gce-managed=true --gce-enable-multiple-stores=true
	// --gce-zones="us-east1-d,us-east1-b,us-east1-c" --nodes=$CLUSTER_NODES
	// --gce-machine-type=n2-standard-16 --local-ssd=true --gce-local-ssd-count=$STORE_COUNT
	// --username=drt --lifetime=15h --gce-image="ubuntu-2204-jammy-v20240319"
	return "cluster-created", nil
}

func syncRoachprod(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement roachprod sync
	// roachprod sync --clouds=gce
	return "roachprod-synced", nil
}

func stageCockroachBinary(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement binary staging
	// roachprod stage $CLUSTER release $CRDB_VERSION
	return "binary-staged", nil
}

func setupDiskStaller(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement disk staller setup
	// Run script: pkg/cmd/drtprod/scripts/setup_dmsetup_disk_staller
	return "disk-staller-setup", nil
}

func setupDatadogCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement Datadog setup for cluster
	// Run script: pkg/cmd/drtprod/scripts/setup_datadog_cluster
	return "datadog-cluster-setup", nil
}

func startCluster(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement cluster start
	// roachprod start $CLUSTER --binary=./cockroach --enable-fluent-sink=true
	// --store-count=$STORE_COUNT --args="--wal-failover=among-stores" --restart=false --sql-port=26257
	return "cluster-started", nil
}

func enableRangefeed(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement rangefeed enablement
	// roachprod sql $CLUSTER:1 -- -e "SET CLUSTER SETTING kv.rangefeed.enabled = true"
	return "rangefeed-enabled", nil
}

func setShutdownTimeout(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement shutdown timeout setting
	// roachprod sql $CLUSTER:1 -- -e "SET CLUSTER SETTING server.shutdown.connections.timeout = '330s'"
	return "shutdown-timeout-set", nil
}

func setDrainWait(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement drain wait setting
	// roachprod sql $CLUSTER:1 -- -e "SET CLUSTER SETTING server.shutdown.drain_wait = '15s'"
	return "drain-wait-set", nil
}

func enableGoschedstats(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement goschedstats enablement
	// roachprod sql $CLUSTER:1 -- -e "SET CLUSTER SETTING goschedstats.always_use_short_sample_period.enabled=true"
	return "goschedstats-enabled", nil
}

func enableWriteBuffering(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement write buffering enablement
	// roachprod sql $CLUSTER:1 -- -e "SET CLUSTER SETTING kv.transaction.write_buffering.enabled = true"
	return "write-buffering-enabled", nil
}

func createLoadBalancer(_ context.Context, _ *planners.PlanExecutionInfo, _ *puaData) (string, error) {
	// TODO: Implement load balancer creation
	// roachprod load-balancer create $CLUSTER
	return "load-balancer-created", nil
}
