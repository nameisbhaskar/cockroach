// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/middleware"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/types"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/gorilla/mux"
)

// PlansHandler handles all plan-related API endpoints.
type PlansHandler struct {
	registries        map[string]planners.Registry
	sharedPlanService planners.SharedPlanService
}

// NewPlansHandler creates a new PlansHandler.
func NewPlansHandler(
	registries map[string]planners.Registry, sharedPlanService planners.SharedPlanService,
) *PlansHandler {
	return &PlansHandler{
		registries:        registries,
		sharedPlanService: sharedPlanService,
	}
}

// GetRoutes returns the routes for plan-related endpoints.
func (h *PlansHandler) GetRoutes() []types.RouteHandler {
	return []types.RouteHandler{
		{
			Path:    "/v1/plans",
			Handler: h.handleListPlans,
			Methods: []string{"GET"},
		},
		{
			Path:    "/v1/plans/{plan_id}/executions",
			Handler: h.handleListExecutions,
			Methods: []string{"GET"},
		},
		{
			Path:    "/v1/plans/{plan_id}/executions/{execution_id}",
			Handler: h.handleGetExecutionStatus,
			Methods: []string{"GET"},
		},
		{
			Path:    "/v1/plans/{plan_id}/executions/{execution_id}/steps/{step_id}/resume",
			Handler: h.handleResumeTask,
			Methods: []string{"POST"},
		},
	}
}

// handleListPlans handles GET /v1/plans
// This endpoint queries the execution framework to find all active plan instances
// and returns them along with their plan metadata.
func (h *PlansHandler) handleListPlans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := planners.LoggerFromContext(ctx)
	requestID := middleware.RequestIDFromContext(ctx)

	// Query the shared plan service for all active plans with metadata
	activePlans, err := h.sharedPlanService.ListAllPlanIDs(ctx)
	if err != nil {
		logger.Error("Failed to list plans", "request_id", requestID, "error", err)
		writeError(ctx, w, http.StatusInternalServerError, fmt.Sprintf("failed to list plans: %v", err))
		return
	}

	// Initialize map with all registered plan names, even if they have no active instances
	plansByName := make(map[string]*types.PlanInfo)
	for planName, registry := range h.registries {
		// Initialize with an empty slice for all registered plans
		plansByName[planName] = &types.PlanInfo{
			PlanName:        planName,
			PlanDescription: registry.GetPlanDescription(),
			PlanIDs:         make([]string, 0),
		}
	}

	// Add active plan instances, grouping by plan name
	for _, planMetadata := range activePlans {
		// Extract plan name from plan ID
		planName, _, err := planners.ExtractPlanNameAndVariant(planMetadata.PlanID)
		if err != nil {
			logger.Error("Invalid plan ID", "request_id", requestID, "plan_id", planMetadata.PlanID, "error", err)
			writeError(ctx, w, http.StatusBadRequest, fmt.Sprintf("invalid plan ID: %v", err))
			return
		}
		// If plan exists in registries, add to existing entry (registry description takes precedence)
		if info, exists := plansByName[planName]; exists {
			info.PlanIDs = append(info.PlanIDs, planMetadata.PlanID)
		} else {
			// Plan found in Temporal but not in registries - use description from Temporal
			plansByName[planName] = &types.PlanInfo{
				PlanName:        planName,
				PlanDescription: planMetadata.Description,
				PlanIDs:         []string{planMetadata.PlanID},
			}
		}
	}
	resp := types.ListPlansResponse{
		Plans: plansByName,
	}
	writeJSON(ctx, w, http.StatusOK, resp)
}

// handleListExecutions handles GET /v1/plans/{plan_id}/executions
func (h *PlansHandler) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := planners.LoggerFromContext(ctx)
	requestID := middleware.RequestIDFromContext(ctx)

	vars := mux.Vars(r)
	planID := vars["plan_id"]

	// List executions using the shared plan service
	executions, err := h.sharedPlanService.ListExecutions(ctx, planID)
	if err != nil {
		logger.Error("Failed to list executions", "request_id", requestID, "plan_id", planID, "error", err)
		writeError(ctx, w, http.StatusInternalServerError, fmt.Sprintf("failed to list executions: %v", err))
		return
	}

	// Extract plan name from plan ID for response metadata
	planName, _, err := planners.ExtractPlanNameAndVariant(planID)
	if err != nil {
		logger.Error("Invalid plan ID", "request_id", requestID, "plan_id", planID, "error", err)
		writeError(ctx, w, http.StatusBadRequest, fmt.Sprintf("invalid plan ID: %v", err))
		return
	}

	// Convert to response format
	executionInfos := make([]types.ExecutionInfo, 0, len(executions))
	for _, exec := range executions {
		startTime := ""
		if exec.StartTime != nil {
			startTime = exec.StartTime.Format(time.RFC3339Nano)
		}
		endTime := ""
		if exec.EndTime != nil {
			endTime = exec.EndTime.Format(time.RFC3339Nano)
		}
		executionInfos = append(executionInfos, types.ExecutionInfo{
			WorkflowID: exec.WorkflowID,
			RunID:      exec.RunID,
			Status:     exec.Status,
			StartTime:  startTime,
			EndTime:    endTime,
		})
	}

	resp := types.ListExecutionsResponse{
		Executions: executionInfos,
		PlanID:     planID,
		PlanName:   planName,
	}
	writeJSON(ctx, w, http.StatusOK, resp)
}

// handleGetExecutionStatus handles GET /v1/plans/{plan_id}/executions/{execution_id}
func (h *PlansHandler) handleGetExecutionStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := planners.LoggerFromContext(ctx)
	requestID := middleware.RequestIDFromContext(ctx)

	vars := mux.Vars(r)
	planID := vars["plan_id"]
	executionID := vars["execution_id"]

	// Get execution status using the shared plan service
	plannerStatus, err := h.sharedPlanService.GetExecutionStatus(ctx, planID, executionID)
	if err != nil {
		logger.Error("Failed to get execution status", "request_id", requestID, "plan_id", planID, "execution_id", executionID, "error", err)
		writeError(ctx, w, http.StatusInternalServerError, fmt.Sprintf("failed to get execution status: %v", err))
		return
	}

	// Convert to API type
	status := convertExecutionStatus(plannerStatus)
	writeJSON(ctx, w, http.StatusOK, status)
}

// writeJSON writes a JSON response.
func writeJSON(ctx context.Context, w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log error, but we've already written headers so we can't change response
		logger := planners.LoggerFromContext(ctx)
		logger.Error("Failed to encode JSON response", "error", err)
	}
}

// handleResumeTask handles POST /v1/plans/{plan_id}/executions/{execution_id}/steps/{step_id}/resume
func (h *PlansHandler) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := planners.LoggerFromContext(ctx)
	requestID := middleware.RequestIDFromContext(ctx)

	vars := mux.Vars(r)
	planID := vars["plan_id"]
	executionID := vars["execution_id"]
	stepID := vars["step_id"]

	// Parse request body
	var resumeReq types.ResumeRequest
	if err := json.NewDecoder(r.Body).Decode(&resumeReq); err != nil {
		logger.Error("Invalid request body", "request_id", requestID, "error", err)
		writeError(ctx, w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Resume the task using the shared plan service (framework-level operation)
	err := h.sharedPlanService.ResumeTask(ctx, planID, executionID, stepID, resumeReq.Result)
	if err != nil {
		logger.Error("Failed to resume task", "request_id", requestID, "plan_id", planID, "execution_id", executionID, "step_id", stepID, "error", err)
		writeError(ctx, w, http.StatusInternalServerError, fmt.Sprintf("failed to resume task: %v", err))
		return
	}

	// Return success response
	resp := types.ResumeResponse{
		Success:    true,
		WorkflowID: executionID,
		StepID:     stepID,
	}
	writeJSON(ctx, w, http.StatusOK, resp)
}

// writeError writes an error response.
func writeError(ctx context.Context, w http.ResponseWriter, status int, message string) {
	writeJSON(ctx, w, status, types.ErrorResponse{Error: message})
}

// convertExecutionStatus converts a planners.ExecutionStatus to types.ExecutionStatus.
func convertExecutionStatus(plannerStatus *planners.ExecutionStatus) *types.ExecutionStatus {
	if plannerStatus == nil {
		return nil
	}

	status := &types.ExecutionStatus{
		WorkflowID:          plannerStatus.WorkflowID,
		Status:              plannerStatus.Status,
		CurrentTasks:        plannerStatus.CurrentTasks,
		ParentWorkflowURL:   plannerStatus.ParentWorkflowURL,
		ParentWorkflowUIURL: plannerStatus.ParentWorkflowUIURL,
	}

	if plannerStatus.Workflow != nil {
		status.Workflow = convertWorkflowInfo(plannerStatus.Workflow)
	}

	return status
}

// convertWorkflowInfo converts a planners.WorkflowInfo to types.WorkflowInfo.
func convertWorkflowInfo(plannerWorkflow *planners.WorkflowInfo) *types.WorkflowInfo {
	if plannerWorkflow == nil {
		return nil
	}

	workflow := &types.WorkflowInfo{
		Name:        plannerWorkflow.Name,
		Description: plannerWorkflow.Description,
		FirstTask:   plannerWorkflow.FirstTask,
		OutputTask:  plannerWorkflow.OutputTask,
		Input:       plannerWorkflow.Input,
		Output:      plannerWorkflow.Output,
		Tasks:       make(map[string]types.TaskInfo),
	}

	for taskName, plannerTask := range plannerWorkflow.Tasks {
		workflow.Tasks[taskName] = convertTaskInfo(&plannerTask)
	}

	return workflow
}

// convertTaskInfo converts a planners.TaskInfo to types.TaskInfo.
func convertTaskInfo(plannerTask *planners.TaskInfo) types.TaskInfo {
	task := types.TaskInfo{
		Name:            plannerTask.Name,
		Type:            plannerTask.Type,
		Next:            plannerTask.Next,
		Fail:            plannerTask.Fail,
		Then:            plannerTask.Then,
		Else:            plannerTask.Else,
		Executor:        plannerTask.ExecutorFunc,
		ResultProcessor: plannerTask.ResultProcessor,
		Params:          plannerTask.Params,
		ForkTasks:       plannerTask.ForkTasks,
		Input:           plannerTask.Input,
		Output:          plannerTask.Output,
		Error:           plannerTask.Error,
		Status:          plannerTask.Status,
		Properties:      plannerTask.Properties,
	}

	if plannerTask.ChildPlanInfo != nil {
		task.ChildPlanInfo = &types.ChildPlanInfo{
			PlanName:           plannerTask.ChildPlanInfo.PlanName,
			PlanVariant:        plannerTask.ChildPlanInfo.PlanVariant,
			ChildWorkflowURL:   plannerTask.ChildPlanInfo.ChildWorkflowURL,
			ChildWorkflowUIURL: plannerTask.ChildPlanInfo.ChildWorkflowUIURL,
		}
	}

	if plannerTask.StartTime != nil {
		task.StartTime = plannerTask.StartTime.Format(time.RFC3339Nano)
	}

	if plannerTask.EndTime != nil {
		task.EndTime = plannerTask.EndTime.Format(time.RFC3339Nano)
	}

	return task
}
