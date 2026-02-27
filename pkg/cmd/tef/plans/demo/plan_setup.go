// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/spf13/cobra"
)

const clusterSetupPlanName = "cluster-setup"

type ClusterSetup struct{}

func RegisterClusterSetupPlans(pr *planners.PlanRegistry) {
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
	return "Provision and setup a CRDB cluster"
}

func (c *ClusterSetup) GetPlanVersion() int {
	return 1
}

type ProvisionData struct {
	Name string `json:"name,omitempty"`
}

func (c *ClusterSetup) ParsePlanInput(input string) (interface{}, error) {
	data := &ProvisionData{}
	if err := json.Unmarshal([]byte(input), data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *ClusterSetup) GeneratePlan(ctx context.Context, p planners.Planner) {
	registerSetupExecutors(ctx, p)

	setupClusterTask := p.NewExecutionTask(ctx, "setup cluster")
	provisionClusterTask := p.NewExecutionTask(ctx, "provision cluster")
	checkForClusterTask := p.NewConditionTask(ctx, "check for cluster")
	importTPCC := p.NewExecutionTask(ctx, "import cluster")
	sleepTask := p.NewExecutionTask(ctx, "sleep")
	endTask := p.NewEndTask(ctx, "end")

	setupClusterTask.ExecutorFn = setupCluster
	setupClusterTask.Next = provisionClusterTask
	setupClusterTask.Fail = endTask

	provisionClusterTask.ExecutorFn = provisionCluster
	provisionClusterTask.Params = []planners.Task{setupClusterTask}
	provisionClusterTask.Next = sleepTask
	provisionClusterTask.Fail = endTask

	sleepTask.ExecutorFn = sleepFor10
	sleepTask.Next = checkForClusterTask

	checkForClusterTask.ExecutorFn = checkForCluster
	checkForClusterTask.Then = importTPCC
	checkForClusterTask.Else = endTask

	importTPCC.ExecutorFn = importWL
	importTPCC.Next = endTask
	importTPCC.Fail = endTask

	p.RegisterPlan(ctx, setupClusterTask, setupClusterTask)
}

func registerSetupExecutors(ctx context.Context, p planners.Planner) {
	for _, exe := range []*planners.Executor{
		{
			Name:        "setup cluster",
			Description: "Sets up a cluster",
			Func:        setupCluster,
		},
		{
			Name:        "provision cluster",
			Description: "Provisions a cluster",
			Func:        provisionCluster,
		},
		{
			Name:        "sleep cluster",
			Description: "Sleeps for 10 seconds",
			Func:        sleepFor10,
		},
		{
			Name:        "check cluster",
			Description: "Checks cluster status",
			Func:        checkForCluster,
		},
		{
			Name:        "import cluster",
			Description: "Imports workload data",
			Func:        importWL,
		},
	} {
		p.RegisterExecutor(ctx, exe)
	}
}

func setupCluster(
	_ context.Context, _ *planners.PlanExecutionInfo, input *ProvisionData,
) (string, error) {
	fmt.Println(">>>>>>>>>>>>>>>" + input.Name)
	return "cluster-info", nil
}

func provisionCluster(
	_ context.Context, _ *planners.PlanExecutionInfo, input *ProvisionData, setOutput string,
) (string, error) {
	fmt.Println(">>>>>>>>>>>>>>>" + input.Name + "<><><><>" + setOutput)
	return "bluster-info", nil
}

func sleepFor10(
	_ context.Context, _ *planners.PlanExecutionInfo, input *ProvisionData,
) (string, error) {
	fmt.Println(">>>>>>>>>>>>>" + input.Name)
	time.Sleep(1 * time.Second)
	return "sleep-completed", nil
}

func checkForCluster(
	_ context.Context, _ *planners.PlanExecutionInfo, _ *ProvisionData,
) (bool, error) {
	return true, nil
}

func importWL(
	_ context.Context, _ *planners.PlanExecutionInfo, input *ProvisionData,
) (string, error) {
	fmt.Println(">>>>>>>>>>>>>>>" + input.Name)
	time.Sleep(1 * time.Minute)
	return "cluster-info", nil
}
