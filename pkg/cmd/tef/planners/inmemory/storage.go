// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package inmemory provides an in-memory implementation of the TEF orchestration backend.
// This implementation runs workflows synchronously in a single process without external dependencies.
// It's designed for testing, development, and visualization workflows.
package inmemory

import (
	"sync"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/cockroachdb/errors"
)

// executionState represents the state of a single workflow execution.
type executionState struct {
	WorkflowID       string
	RunID            string
	PlanID           string
	Status           string // "Running", "Completed", "Failed"
	StartTime        time.Time
	EndTime          *time.Time
	Input            interface{}                 // User-provided input
	ExecInfo         *planners.PlanExecutionInfo // Execution metadata (workflow ID, plan ID, etc.)
	Result           interface{}
	Error            error
	Steps            map[string]*stepState
	Workflow         *planners.WorkflowInfo
	ParentWorkflowID string        // Parent workflow ID if this is a child execution
	ParentPlanID     string        // Parent plan ID if this is a child execution
	done             chan struct{} // Closed when workflow completes (success or failure)
	mu               sync.RWMutex
}

// stepState represents the state of a single step in a workflow execution.
type stepState struct {
	StepID           string
	Status           string // "Pending", "InProgress", "Completed", "Failed"
	StartTime        time.Time
	EndTime          *time.Time
	Input            []interface{} // Input parameters for this task
	Result           interface{}
	Error            error
	CallbackKey      string // For callback tasks, the key to resume
	ChildWorkflowID  string // For child plan tasks, the child workflow ID
	ChildPlanID      string // For child plan tasks, the child plan ID
	ChildPlanName    string // For child plan tasks, the child plan name
	ChildPlanVariant string // For child plan tasks, the child plan variant
}

// storage provides thread-safe storage for workflow execution state.
// It stores all execution data in memory and is lost on process restart.
type storage struct {
	// executions maps planID -> workflowID -> executionState
	executions map[string]map[string]*executionState
	// callbacks maps planID -> workflowID -> stepID -> callback channel
	callbacks map[string]map[string]map[string]chan string
	mu        sync.RWMutex
}

// newStorage creates a new in-memory storage instance.
func newStorage() *storage {
	return &storage{
		executions: make(map[string]map[string]*executionState),
		callbacks:  make(map[string]map[string]map[string]chan string),
	}
}

// CreateExecution creates a new execution state entry.
func (s *storage) CreateExecution(
	planID, workflowID, runID string,
	input interface{},
	execInfo *planners.PlanExecutionInfo,
	workflow *planners.WorkflowInfo,
	parentWorkflowID, parentPlanID string,
) *executionState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.executions[planID] == nil {
		s.executions[planID] = make(map[string]*executionState)
	}

	state := &executionState{
		WorkflowID:       workflowID,
		RunID:            runID,
		PlanID:           planID,
		Status:           "Running",
		StartTime:        time.Now(),
		Input:            input,
		ExecInfo:         execInfo,
		Steps:            make(map[string]*stepState),
		Workflow:         workflow,
		ParentWorkflowID: parentWorkflowID,
		ParentPlanID:     parentPlanID,
		done:             make(chan struct{}),
	}

	s.executions[planID][workflowID] = state
	return state
}

// GetExecution retrieves execution state for a workflow.
func (s *storage) GetExecution(planID, workflowID string) *executionState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if planMap, ok := s.executions[planID]; ok {
		return planMap[workflowID]
	}
	return nil
}

// UpdateExecutionStatus updates the status of an execution.
func (s *storage) UpdateExecutionStatus(
	planID, workflowID, status string, result interface{}, err error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if planMap, ok := s.executions[planID]; ok {
		if exec, ok := planMap[workflowID]; ok {
			exec.mu.Lock()
			defer exec.mu.Unlock()

			exec.Status = status
			now := time.Now()
			exec.EndTime = &now
			exec.Result = result
			exec.Error = err

			// Close the done channel to signal completion
			// Use select to avoid panic if already closed
			select {
			case <-exec.done:
				// Already closed
			default:
				close(exec.done)
			}
		}
	}
}

// ListExecutions returns all executions for a plan.
func (s *storage) ListExecutions(planID string) []*executionState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*executionState
	if planMap, ok := s.executions[planID]; ok {
		for _, exec := range planMap {
			result = append(result, exec)
		}
	}
	return result
}

// ListAllPlanIDs returns all plan IDs that have executions.
func (s *storage) ListAllPlanIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []string
	for planID := range s.executions {
		result = append(result, planID)
	}
	return result
}

// RegisterCallback registers a callback channel for a step.
func (s *storage) RegisterCallback(planID, workflowID, stepID string, ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.callbacks[planID] == nil {
		s.callbacks[planID] = make(map[string]map[string]chan string)
	}
	if s.callbacks[planID][workflowID] == nil {
		s.callbacks[planID][workflowID] = make(map[string]chan string)
	}
	s.callbacks[planID][workflowID][stepID] = ch
}

// GetCallback retrieves the callback channel for a step.
func (s *storage) GetCallback(planID, workflowID, stepID string) chan string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if planMap, ok := s.callbacks[planID]; ok {
		if wfMap, ok := planMap[workflowID]; ok {
			return wfMap[stepID]
		}
	}
	return nil
}

// UpdateStepState updates the state of a step in an execution.
func (s *storage) UpdateStepState(
	planID, workflowID, stepID, status string, result interface{}, err error,
) {
	exec := s.GetExecution(planID, workflowID)
	if exec == nil {
		return
	}

	exec.mu.Lock()
	defer exec.mu.Unlock()

	step, ok := exec.Steps[stepID]
	if !ok {
		step = &stepState{
			StepID:    stepID,
			StartTime: time.Now(),
		}
		exec.Steps[stepID] = step
	}

	step.Status = status
	if status == "Completed" || status == "Failed" {
		now := time.Now()
		step.EndTime = &now
	}
	step.Result = result
	step.Error = err
}

// UpdateStepInput updates the input parameters for a step.
func (s *storage) UpdateStepInput(planID, workflowID, stepID string, input []interface{}) {
	exec := s.GetExecution(planID, workflowID)
	if exec == nil {
		return
	}

	exec.mu.Lock()
	defer exec.mu.Unlock()

	step, ok := exec.Steps[stepID]
	if !ok {
		step = &stepState{
			StepID:    stepID,
			StartTime: time.Now(),
		}
		exec.Steps[stepID] = step
	}

	step.Input = input
}

// UpdateChildWorkflowInfo updates the child workflow information for a step (used for ChildPlanTask).
func (s *storage) UpdateChildWorkflowInfo(
	planID, workflowID, stepID, childWorkflowID, childPlanID, childPlanName, childPlanVariant string,
) {
	exec := s.GetExecution(planID, workflowID)
	if exec == nil {
		return
	}

	exec.mu.Lock()
	defer exec.mu.Unlock()

	step, ok := exec.Steps[stepID]
	if !ok {
		step = &stepState{
			StepID:    stepID,
			StartTime: time.Now(),
		}
		exec.Steps[stepID] = step
	}

	step.ChildWorkflowID = childWorkflowID
	step.ChildPlanID = childPlanID
	step.ChildPlanName = childPlanName
	step.ChildPlanVariant = childPlanVariant
}

// WaitForCompletion blocks until the workflow completes (success or failure).
// Returns the final result and error.
func (s *storage) WaitForCompletion(planID, workflowID string) (interface{}, error) {
	exec := s.GetExecution(planID, workflowID)
	if exec == nil {
		return nil, errors.Newf("execution not found: plan_id=%s workflow_id=%s", planID, workflowID)
	}

	// Wait for workflow to complete
	<-exec.done

	// Return final result
	exec.mu.RLock()
	defer exec.mu.RUnlock()
	return exec.Result, exec.Error
}

// ToExecutionStatus converts executionState to planners.ExecutionStatus.
func (s *executionState) ToExecutionStatus() *planners.ExecutionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy of the workflow to avoid modifying the original
	workflowCopy := &planners.WorkflowInfo{
		Name:            s.Workflow.Name,
		Description:     s.Workflow.Description,
		WorkflowVersion: s.Workflow.WorkflowVersion,
		FirstTask:       s.Workflow.FirstTask,
		OutputTask:      s.Workflow.OutputTask,
		Tasks:           make(map[string]planners.TaskInfo),
	}

	// Set the actual runtime input and output
	// Input contains both execution info and user input, matching Temporal workflow signature:
	// func(ctx, planExecutionInfo *PlanExecutionInfo, input interface{}) (interface{}, error)
	if s.ExecInfo != nil || s.Input != nil {
		workflowCopy.Input = []interface{}{s.ExecInfo, s.Input}
	}
	if s.Result != nil {
		workflowCopy.Output = s.Result
	}

	// Copy all task definitions and set default status to "Pending"
	for taskName, taskInfo := range s.Workflow.Tasks {
		// Set default status for tasks that haven't started
		if taskInfo.Status == "" {
			taskInfo.Status = "Pending"
		}
		workflowCopy.Tasks[taskName] = taskInfo
	}

	// Merge in the runtime step states and collect currently running tasks
	var currentTasks []string
	for stepID, stepState := range s.Steps {
		if taskInfo, ok := workflowCopy.Tasks[stepID]; ok {
			// Update task info with runtime state
			taskInfo.Status = stepState.Status
			taskInfo.StartTime = &stepState.StartTime
			taskInfo.EndTime = stepState.EndTime
			taskInfo.Input = stepState.Input
			taskInfo.Output = stepState.Result
			if stepState.Error != nil {
				taskInfo.Error = stepState.Error.Error()
			}

			// Populate child workflow URLs for ChildPlanTasks
			if stepState.ChildWorkflowID != "" {
				planners.PopulateChildWorkflowInfo(
					&taskInfo.ChildPlanInfo,
					stepState.ChildPlanName,
					stepState.ChildPlanVariant,
					stepState.ChildPlanID,
					stepState.ChildWorkflowID,
				)
			}

			workflowCopy.Tasks[stepID] = taskInfo

			// Track currently running tasks
			if stepState.Status == "InProgress" {
				currentTasks = append(currentTasks, stepID)
			}
		}
	}

	status := &planners.ExecutionStatus{
		WorkflowID:   s.WorkflowID,
		Status:       s.Status,
		CurrentTasks: currentTasks,
		Workflow:     workflowCopy,
	}

	// Populate parent workflow URLs if this is a child execution
	if s.ParentWorkflowID != "" {
		planners.PopulateParentWorkflowURLs(status, s.ParentPlanID, s.ParentWorkflowID)
	}

	return status
}

// ToWorkflowExecutionInfo converts executionState to planners.WorkflowExecutionInfo.
func (s *executionState) ToWorkflowExecutionInfo() *planners.WorkflowExecutionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startTime := s.StartTime
	info := &planners.WorkflowExecutionInfo{
		WorkflowID: s.WorkflowID,
		RunID:      s.RunID,
		Status:     s.Status,
		StartTime:  &startTime,
		EndTime:    s.EndTime,
	}

	return info
}
