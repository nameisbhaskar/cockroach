// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package inmemory

import (
	"context"
	"reflect"
	"sync"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/cockroachdb/errors"
)

// workflowExecutor handles the execution of a workflow in-memory.
type workflowExecutor struct {
	ctx         context.Context
	planner     *planners.BasePlanner
	storage     *storage
	planID      string
	workflowID  string
	runID       string
	input       interface{}
	execInfo    *planners.PlanExecutionInfo
	taskResults map[string]interface{} // taskName -> result
	mu          sync.RWMutex
}

// newWorkflowExecutor creates a new workflow executor.
func newWorkflowExecutor(
	ctx context.Context,
	planner *planners.BasePlanner,
	storage *storage,
	planID, workflowID, runID string,
	input interface{},
	execInfo *planners.PlanExecutionInfo,
) *workflowExecutor {
	return &workflowExecutor{
		ctx:         ctx,
		planner:     planner,
		storage:     storage,
		planID:      planID,
		workflowID:  workflowID,
		runID:       runID,
		input:       input,
		execInfo:    execInfo,
		taskResults: make(map[string]interface{}),
	}
}

// execute runs the workflow from start to finish.
func (w *workflowExecutor) execute() (interface{}, error) {
	logger := planners.LoggerFromContext(w.ctx)

	// Create execution info for this plan
	execInfo := &planners.PlanExecutionInfo{
		WorkflowID:      w.workflowID,
		PlanID:          w.planID,
		PlanVariant:     extractVariant(w.planID),
		WorkflowVersion: w.planner.WorkflowVersion,
	}

	logger.Info("Starting workflow execution",
		"workflow_id", w.workflowID,
		"plan", w.planner.Name,
		"first_task", w.planner.First.Name(),
	)

	// Execute the workflow task chain starting from the first task
	_, err := w.executeTaskChain(w.planner.First, execInfo, nil, nil)
	if err != nil {
		return nil, err
	}

	// Get the output from the output task if specified
	var output interface{}
	if w.planner.Output != nil {
		output = w.getTaskResult(w.planner.Output.Name())
	}

	logger.Info("Workflow execution completed", "workflow_id", w.workflowID)
	return output, nil
}

// executeExecutionTask executes an execution task.
func (w *workflowExecutor) executeExecutionTask(
	task *planners.ExecutionTask, execInfo *planners.PlanExecutionInfo, lastError error,
) (planners.Task, error) {
	// Gather parameters from previous task results
	params := w.gatherParams(task.Params)

	// Store input parameters for this task
	taskInput := make([]interface{}, 0, len(params)+1)
	taskInput = append(taskInput, w.input)
	taskInput = append(taskInput, params...)
	w.updateTaskInput(task.Name(), taskInput)

	// Build function arguments: (ctx, execInfo, input, ...params, [errorString])
	args := []reflect.Value{
		reflect.ValueOf(w.ctx),
		reflect.ValueOf(execInfo),
		reflect.ValueOf(w.input),
	}

	for _, param := range params {
		args = append(args, reflect.ValueOf(param))
	}

	// If there was a previous error (failure path), add error string
	if lastError != nil {
		args = append(args, reflect.ValueOf(lastError.Error()))
	}

	// Call the executor function
	fn := reflect.ValueOf(task.ExecutorFn)
	results := fn.Call(args)

	// Extract result and error
	result := results[0].Interface()
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}

	if err != nil {
		return nil, err
	}

	// Store result for future tasks
	w.setTaskResult(task.Name(), result)

	return nil, nil
}

// executeForkTask executes a fork task with parallel branches.
// Each branch is executed in its own goroutine and must reach the Join point.
func (w *workflowExecutor) executeForkTask(
	task *planners.ForkTask, execInfo *planners.PlanExecutionInfo, lastError error,
) (planners.Task, error) {
	logger := planners.LoggerFromContext(w.ctx)

	// Execute all branches in parallel
	var wg sync.WaitGroup
	errChan := make(chan error, len(task.Tasks))

	logger.Info("Starting fork task with parallel branches",
		"task_name", task.Name(),
		"num_branches", len(task.Tasks),
	)

	for _, branchTask := range task.Tasks {
		wg.Add(1)
		go func(bt planners.Task) {
			defer wg.Done()

			// Execute the entire branch task chain starting from this task
			_, err := w.executeTaskChain(bt, execInfo, lastError, task.Join)
			if err != nil {
				errChan <- err
			}
		}(branchTask)
	}

	wg.Wait()
	close(errChan)

	// Collect errors from all branches
	var firstErr error
	for err := range errChan {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// If any branch failed, follow the Fail path if available
	if firstErr != nil {
		logger.Info("Fork task branch failed",
			"task_name", task.Name(),
			"error", firstErr,
		)
		// The fork task itself has failed, return to main executor to handle Fail path
		return nil, firstErr
	}

	logger.Info("Fork task completed successfully",
		"task_name", task.Name(),
	)

	// All branches completed successfully, continue from fork's Next
	return nil, nil
}

// executeTaskChain recursively executes a chain of tasks starting from the given task,
// following Next and Fail paths until reaching a termination point (EndTask, ForkJoinTask, or stopAt).
func (w *workflowExecutor) executeTaskChain(
	currentTask planners.Task,
	execInfo *planners.PlanExecutionInfo,
	lastError error,
	stopAt planners.Task,
) (interface{}, error) {
	logger := planners.LoggerFromContext(w.ctx)

	for currentTask != nil {
		// Stop if we've reached the stopAt task (e.g., ForkJoinTask)
		if stopAt != nil && currentTask == stopAt {
			logger.Info("Reached stop point", "stop_at", stopAt.Name())
			return nil, nil
		}

		// Handle different task types
		switch task := currentTask.(type) {
		case *planners.ExecutionTask:
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "InProgress", nil, nil)

			nextTask, err := w.executeExecutionTask(task, execInfo, lastError)
			if err != nil {
				w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Failed", nil, err)

				// Follow failure path if available
				if task.Fail != nil {
					logger.Info("Following failure path", "task_name", task.Name(), "fail_task", task.Fail.Name())
					currentTask = task.Fail
					lastError = err
					continue
				}
				return nil, err
			}

			// Task succeeded
			taskResult := w.getTaskResult(task.Name())
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", taskResult, nil)
			lastError = nil

			// Move to next task
			if nextTask != nil {
				currentTask = nextTask
			} else {
				currentTask = task.Next
			}

		case *planners.ConditionTask:
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "InProgress", nil, nil)

			nextTask, err := w.executeConditionTask(task, execInfo)
			if err != nil {
				w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Failed", nil, err)
				return nil, err
			}

			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", nil, nil)
			currentTask = nextTask

		case *planners.CallbackTask:
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "InProgress", nil, nil)

			nextTask, err := w.executeCallbackTask(task, execInfo, lastError)
			if err != nil {
				w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Failed", nil, err)

				// Follow failure path if available
				if task.Fail != nil {
					currentTask = task.Fail
					lastError = err
					continue
				}
				return nil, err
			}

			// Task succeeded
			taskResult := w.getTaskResult(task.Name())
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", taskResult, nil)
			lastError = nil

			// Move to next task
			if nextTask != nil {
				currentTask = nextTask
			} else {
				currentTask = task.Next
			}

		case *planners.ChildPlanTask:
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "InProgress", nil, nil)

			nextTask, err := w.executeChildPlanTask(task, execInfo, lastError)
			if err != nil {
				w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Failed", nil, err)

				// Follow failure path if available
				if task.Fail != nil {
					currentTask = task.Fail
					lastError = err
					continue
				}
				return nil, err
			}

			// Task succeeded
			taskResult := w.getTaskResult(task.Name())
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", taskResult, nil)
			lastError = nil

			// Move to next task
			if nextTask != nil {
				currentTask = nextTask
			} else {
				currentTask = task.Next
			}

		case *planners.ForkTask:
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "InProgress", nil, nil)

			nextTask, err := w.executeForkTask(task, execInfo, lastError)
			if err != nil {
				w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Failed", nil, err)

				// Follow failure path if available
				if task.Fail != nil {
					currentTask = task.Fail
					lastError = err
					continue
				}
				return nil, err
			}

			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", nil, nil)
			lastError = nil

			// Move to next task
			if nextTask != nil {
				currentTask = nextTask
			} else {
				currentTask = task.Next
			}

		case *planners.ForkJoinTask:
			// ForkJoinTask is a synchronization barrier - just mark it and return
			logger.Info("Reached fork join task", "task_name", task.Name())
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", nil, nil)
			return nil, nil

		case *planners.EndTask:
			// End task marks completion
			logger.Info("Reached end task", "task_name", task.Name())
			w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "Completed", nil, nil)
			return nil, nil

		default:
			return nil, errors.Newf("unknown task type: %T", currentTask)
		}
	}

	return nil, nil
}

// executeConditionTask executes a condition task.
func (w *workflowExecutor) executeConditionTask(
	task *planners.ConditionTask, execInfo *planners.PlanExecutionInfo,
) (planners.Task, error) {
	// Gather parameters
	params := w.gatherParams(task.Params)

	// Store input parameters for this task
	taskInput := make([]interface{}, 0, len(params)+1)
	taskInput = append(taskInput, w.input)
	taskInput = append(taskInput, params...)
	w.updateTaskInput(task.Name(), taskInput)

	// Build function arguments: (ctx, execInfo, input, ...params)
	args := []reflect.Value{
		reflect.ValueOf(w.ctx),
		reflect.ValueOf(execInfo),
		reflect.ValueOf(w.input),
	}

	for _, param := range params {
		args = append(args, reflect.ValueOf(param))
	}

	// Call the executor function
	fn := reflect.ValueOf(task.ExecutorFn)
	results := fn.Call(args)

	// Extract result and error
	condition := results[0].Bool()
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}

	if err != nil {
		return nil, err
	}

	// Branch based on condition
	if condition {
		return task.Then, nil
	}
	return task.Else, nil
}

// executeCallbackTask executes a callback task.
func (w *workflowExecutor) executeCallbackTask(
	task *planners.CallbackTask, execInfo *planners.PlanExecutionInfo, lastError error,
) (planners.Task, error) {
	logger := planners.LoggerFromContext(w.ctx)

	// Gather parameters
	params := w.gatherParams(task.Params)

	// Store input parameters for this task
	taskInput := make([]interface{}, 0, len(params)+1)
	taskInput = append(taskInput, w.input)
	taskInput = append(taskInput, params...)
	w.updateTaskInput(task.Name(), taskInput)

	// Build function arguments for ExecutionFn: (ctx, execInfo, input, ...params, [errorString])
	args := []reflect.Value{
		reflect.ValueOf(w.ctx),
		reflect.ValueOf(execInfo),
		reflect.ValueOf(w.input),
	}

	for _, param := range params {
		args = append(args, reflect.ValueOf(param))
	}

	if lastError != nil {
		args = append(args, reflect.ValueOf(lastError.Error()))
	}

	// Call ExecutionFn to get step ID
	fn := reflect.ValueOf(task.ExecutionFn)
	results := fn.Call(args)

	stepID := results[0].String()
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}

	if err != nil {
		return nil, err
	}

	logger.Info("Callback task waiting for resume",
		"task_name", task.Name(),
		"step_id", stepID,
	)

	// Register callback channel and wait for resume
	ch := make(chan string)
	w.storage.RegisterCallback(w.planID, w.workflowID, stepID, ch)
	w.storage.UpdateStepState(w.planID, w.workflowID, task.Name(), "InProgress", stepID, nil)

	// Block waiting for callback
	asyncResult := <-ch

	logger.Info("Callback received",
		"task_name", task.Name(),
		"step_id", stepID,
		"result", asyncResult,
	)

	// Call ResultProcessorFn with the result
	processorArgs := []reflect.Value{
		reflect.ValueOf(w.ctx),
		reflect.ValueOf(execInfo),
		reflect.ValueOf(w.input),
		reflect.ValueOf(stepID),
		reflect.ValueOf(asyncResult),
	}

	processorFn := reflect.ValueOf(task.ResultProcessorFn)
	processorResults := processorFn.Call(processorArgs)

	result := processorResults[0].Interface()
	if !processorResults[1].IsNil() {
		err = processorResults[1].Interface().(error)
	}

	if err != nil {
		return nil, err
	}

	// Store result
	w.setTaskResult(task.Name(), result)

	return nil, nil
}

// executeChildPlanTask executes a child plan task.
func (w *workflowExecutor) executeChildPlanTask(
	task *planners.ChildPlanTask, execInfo *planners.PlanExecutionInfo, lastError error,
) (planners.Task, error) {
	logger := planners.LoggerFromContext(w.ctx)

	// Gather parameters
	params := w.gatherParams(task.Params)

	// Store input parameters for this task
	taskInput := make([]interface{}, 0, len(params)+1)
	taskInput = append(taskInput, w.input)
	taskInput = append(taskInput, params...)
	w.updateTaskInput(task.Name(), taskInput)

	// Build function arguments: (ctx, execInfo, input, ...params, [errorString])
	args := []reflect.Value{
		reflect.ValueOf(w.ctx),
		reflect.ValueOf(execInfo),
		reflect.ValueOf(w.input),
	}

	for _, param := range params {
		args = append(args, reflect.ValueOf(param))
	}

	if lastError != nil {
		args = append(args, reflect.ValueOf(lastError.Error()))
	}

	// Call ChildTaskInfoFn to get child task info
	fn := reflect.ValueOf(task.ChildTaskInfoFn)
	results := fn.Call(args)

	childTaskInfo := results[0].Interface().(planners.ChildTaskInfo)
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}

	if err != nil {
		return nil, err
	}

	// Get manager for child plan
	childPlanID := planners.GetPlanID(task.PlanName, childTaskInfo.PlanVariant)
	mgr, err := planners.GetManagerForPlan(task.PlanName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get manager for child plan %s", task.PlanName)
	}

	logger.Info("Executing child plan",
		"task_name", task.Name(),
		"child_plan", task.PlanName,
		"child_plan_id", childPlanID,
	)

	// Execute child plan (starts asynchronously) with parent workflow info
	// Cast to manager to access executePlanInternal
	inMemoryManager, ok := mgr.(*manager)
	if !ok {
		return nil, errors.Newf("manager for child plan %s is not an in-memory manager", task.PlanName)
	}
	childWorkflowID, err := inMemoryManager.executePlanInternal(
		w.ctx, childTaskInfo.Input, childPlanID, w.workflowID, w.planID,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "child plan execution failed to start")
	}

	// Store child workflow information in step state
	w.storage.UpdateChildWorkflowInfo(
		w.planID, w.workflowID, task.Name(),
		childWorkflowID, childPlanID, task.PlanName, childTaskInfo.PlanVariant,
	)

	logger.Info("Child plan started, waiting for completion",
		"task_name", task.Name(),
		"child_plan", task.PlanName,
		"child_workflow_id", childWorkflowID,
	)

	// Wait for child plan to complete
	childResult, childErr := w.storage.WaitForCompletion(childPlanID, childWorkflowID)
	if childErr != nil {
		return nil, errors.Wrapf(childErr, "child plan execution failed")
	}

	logger.Info("Child plan completed",
		"task_name", task.Name(),
		"child_plan", task.PlanName,
		"child_workflow_id", childWorkflowID,
	)

	// Store child workflow result
	w.setTaskResult(task.Name(), childResult)

	return nil, nil
}

// gatherParams collects results from parameter tasks.
func (w *workflowExecutor) gatherParams(params []planners.Task) []interface{} {
	results := make([]interface{}, len(params))
	for i, param := range params {
		results[i] = w.getTaskResult(param.Name())
	}
	return results
}

// setTaskResult stores a task's result.
func (w *workflowExecutor) setTaskResult(taskName string, result interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.taskResults[taskName] = result
}

// getTaskResult retrieves a task's result.
func (w *workflowExecutor) getTaskResult(taskName string) interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.taskResults[taskName]
}

// updateTaskInput stores the input parameters for a task.
func (w *workflowExecutor) updateTaskInput(taskName string, input []interface{}) {
	w.storage.UpdateStepInput(w.planID, w.workflowID, taskName, input)
}

// extractVariant extracts the variant from a plan ID.
// Plan ID format: "plan-name_variant"
func extractVariant(planID string) string {
	planName, variant, err := planners.ExtractPlanNameAndVariant(planID)
	if err != nil {
		return ""
	}
	_ = planName
	return variant
}
