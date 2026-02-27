// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package inmemory

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

// manager implements PlannerManager for in-memory execution.
// It executes workflows synchronously in the same process without external orchestration.
type manager struct {
	registry *planners.Registry
	planner  *planners.BasePlanner
	storage  *storage
}

// Verify interface implementation.
var _ planners.PlannerManager = &manager{}

// newManager creates a new in-memory manager for a plan.
func newManager(
	ctx context.Context, registry planners.Registry, storage *storage,
) (*manager, error) {
	// Create the base planner from the registry
	planner, err := planners.NewBasePlanner(ctx, registry)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create base planner")
	}

	return &manager{
		registry: &registry,
		planner:  planner,
		storage:  storage,
	}, nil
}

// StartWorker is a no-op for in-memory execution since workers run in-process.
// In the simulate command, we start all workers in the same process.
func (m *manager) StartWorker(ctx context.Context, planID string) error {
	logger := planners.LoggerFromContext(ctx)
	logger.Info("In-memory worker ready (no-op)", "plan_id", planID)
	return nil
}

// ExecutePlan executes the plan asynchronously in a background goroutine.
// This allows the workflow to run independently and be resumed via ResumeTask.
func (m *manager) ExecutePlan(
	ctx context.Context, input interface{}, planID string,
) (string, error) {
	return m.executePlanInternal(ctx, input, planID, "", "")
}

// executePlanInternal executes the plan asynchronously with optional parent workflow info.
func (m *manager) executePlanInternal(
	ctx context.Context, input interface{}, planID, parentWorkflowID, parentPlanID string,
) (string, error) {
	logger := planners.LoggerFromContext(ctx)

	// Generate unique workflow ID and run ID using standard utility functions
	workflowID := planners.GenerateWorkflowID(planID)
	runID := planners.GenerateRunID()

	logger.Info("Starting in-memory plan execution",
		"plan_id", planID,
		"workflow_id", workflowID,
		"run_id", runID,
		"parent_workflow_id", parentWorkflowID,
	)

	// Extract plan variant from planID
	_, planVariant, err := planners.ExtractPlanNameAndVariant(planID)
	if err != nil {
		return "", err
	}

	// Create execution info matching Temporal workflow signature
	execInfo := &planners.PlanExecutionInfo{
		WorkflowID:      workflowID,
		PlanID:          planID,
		PlanVariant:     planVariant,
		WorkflowVersion: m.planner.WorkflowVersion,
	}

	// Serialize plan structure for status queries
	workflow := planners.SerializePlanStructure(m.planner)

	// Create execution state with parent info
	m.storage.CreateExecution(planID, workflowID, runID, input, execInfo, workflow, parentWorkflowID, parentPlanID)

	// Create executor and run the workflow in a background goroutine
	executor := newWorkflowExecutor(ctx, m.planner, m.storage, planID, workflowID, runID, input, execInfo)

	// Execute the workflow asynchronously so ExecutePlan returns immediately
	// This allows callback tasks to block without blocking the caller
	go func() {
		logger.Info("Executing plan tasks...", "plan_id", planID, "workflow_id", workflowID)

		result, err := executor.execute()
		if err != nil {
			logger.Info("Plan execution failed",
				"plan_id", planID,
				"workflow_id", workflowID,
				"error", err,
			)
			m.storage.UpdateExecutionStatus(planID, workflowID, "Failed", nil, err)
			return
		}

		// Execution succeeded
		m.storage.UpdateExecutionStatus(planID, workflowID, "Completed", result, nil)

		logger.Info("Plan execution completed",
			"plan_id", planID,
			"workflow_id", workflowID,
		)
	}()

	logger.Info("Workflow started in background", "workflow_id", workflowID)

	return workflowID, nil
}

// GetExecutionStatus retrieves the status of a workflow execution.
func (m *manager) GetExecutionStatus(
	ctx context.Context, planID, workflowID string,
) (*planners.ExecutionStatus, error) {
	exec := m.storage.GetExecution(planID, workflowID)
	if exec == nil {
		return nil, errors.Newf("execution not found: plan_id=%s workflow_id=%s", planID, workflowID)
	}

	return exec.ToExecutionStatus(), nil
}

// ListExecutions returns all executions for a plan.
func (m *manager) ListExecutions(
	ctx context.Context, planID string,
) ([]*planners.WorkflowExecutionInfo, error) {
	executions := m.storage.ListExecutions(planID)

	result := make([]*planners.WorkflowExecutionInfo, len(executions))
	for i, exec := range executions {
		result[i] = exec.ToWorkflowExecutionInfo()
	}

	return result, nil
}

// ListAllPlanIDs returns all active plan IDs.
func (m *manager) ListAllPlanIDs(ctx context.Context) ([]planners.PlanMetadata, error) {
	planIDs := m.storage.ListAllPlanIDs()

	result := make([]planners.PlanMetadata, len(planIDs))
	for i, planID := range planIDs {
		result[i] = planners.PlanMetadata{
			PlanID:      planID,
			Description: m.planner.Description,
		}
	}

	return result, nil
}

// ResumeTask resumes a callback task with the provided result.
func (m *manager) ResumeTask(
	ctx context.Context, planID, workflowID, stepID, result string,
) error {
	logger := planners.LoggerFromContext(ctx)

	// Get the callback channel
	ch := m.storage.GetCallback(planID, workflowID, stepID)
	if ch == nil {
		return errors.Newf(
			"callback not found: plan_id=%s workflow_id=%s step_id=%s",
			planID, workflowID, stepID,
		)
	}

	logger.Info("Resuming callback task",
		"plan_id", planID,
		"workflow_id", workflowID,
		"step_id", stepID,
	)

	// Send result to callback channel
	ch <- result

	return nil
}

// AddPlannerFlags adds in-memory specific flags (none needed for in-memory).
func (m *manager) AddPlannerFlags(cmd *cobra.Command) {
	// No flags needed for in-memory execution
}

// ClonePropertiesFrom is a no-op for in-memory manager (no properties to clone).
func (m *manager) ClonePropertiesFrom(source planners.PlannerManager) {
	// No-op: in-memory managers don't have connection properties to clone
}

// GetPlanner returns the underlying BasePlanner for this manager.
func (m *manager) GetPlanner() *planners.BasePlanner {
	return m.planner
}

// GetRegistry returns the registry for this manager.
func (m *manager) GetRegistry() planners.Registry {
	return *m.registry
}
