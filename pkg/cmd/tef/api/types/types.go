// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package types

import "net/http"

// RouteHandler defines a single HTTP route with its path, handler, and methods.
type RouteHandler struct {
	Path    string
	Handler http.HandlerFunc
	Methods []string
}

// ExecutionRequest represents the JSON request body for plan execution.
type ExecutionRequest struct {
	Request map[string]interface{} `json:"request"`
}

// ExecutionResponse represents the JSON response for plan execution.
type ExecutionResponse struct {
	WorkflowID string `json:"workflow_id"`
	PlanID     string `json:"plan_id"`
}

// PlanInfo represents information about a plan instance.
type PlanInfo struct {
	PlanName        string   `json:"plan_name"`
	PlanDescription string   `json:"plan_description"`
	PlanIDs         []string `json:"plan_ids"`
}

// ListPlansResponse represents the JSON response for listing plans.
type ListPlansResponse struct {
	Plans map[string]*PlanInfo `json:"plans"`
}

// ExecutionInfo represents information about a workflow execution.
type ExecutionInfo struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	StartTime  string `json:"start_time,omitempty"`
	EndTime    string `json:"end_time,omitempty"`
}

// ListExecutionsResponse represents the JSON response for listing executions.
type ListExecutionsResponse struct {
	Executions []ExecutionInfo `json:"executions"`
	PlanID     string          `json:"plan_id"`
	PlanName   string          `json:"plan_name"`
}

// ErrorResponse represents the JSON response for errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ResumeRequest represents the JSON request body for resuming an async task.
type ResumeRequest struct {
	Result string `json:"result"`
}

// ResumeResponse represents the JSON response for resuming an async task.
type ResumeResponse struct {
	Success    bool   `json:"success"`
	WorkflowID string `json:"workflow_id"`
	StepID     string `json:"step_id"`
}

// ChildPlanInfo represents details about a specific child plan within a workflow system.
type ChildPlanInfo struct {
	PlanName           string `json:"child_plan_name,omitempty"`
	PlanVariant        string `json:"child_plan_variant,omitempty"`
	ChildWorkflowURL   string `json:"child_workflow_url,omitempty"`    // API endpoint
	ChildWorkflowUIURL string `json:"child_workflow_ui_url,omitempty"` // UI page URL
}

// TaskInfo represents information about a task in a workflow.
type TaskInfo struct {
	Name            string                 `json:"name"`
	Type            string                 `json:"type"`
	Next            string                 `json:"next,omitempty"`
	Fail            string                 `json:"fail,omitempty"`
	Then            string                 `json:"then,omitempty"`
	Else            string                 `json:"else,omitempty"`
	Executor        string                 `json:"executor,omitempty"`
	ResultProcessor string                 `json:"result_processor,omitempty"`
	Params          []string               `json:"params,omitempty"`
	ForkTasks       []string               `json:"fork_tasks,omitempty"`
	ChildPlanInfo   *ChildPlanInfo         `json:"child_plan_info,omitempty"`
	Input           []interface{}          `json:"input,omitempty"`
	Output          interface{}            `json:"output,omitempty"`
	Error           string                 `json:"error,omitempty"`
	Status          string                 `json:"status,omitempty"` // Pending, InProgress, Completed, Failed
	StartTime       string                 `json:"start_time,omitempty"`
	EndTime         string                 `json:"end_time,omitempty"`
	Properties      map[string]interface{} `json:"properties,omitempty"`
}

// WorkflowInfo represents information about a workflow.
type WorkflowInfo struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	FirstTask   string              `json:"first_task"`
	OutputTask  string              `json:"output_task"`
	Tasks       map[string]TaskInfo `json:"tasks"`
	Input       []interface{}       `json:"input,omitempty"`
	Output      interface{}         `json:"output,omitempty"`
}

// ExecutionStatus represents the status of a workflow execution.
type ExecutionStatus struct {
	WorkflowID          string        `json:"workflow_id"`
	Status              string        `json:"status"` // Running, Completed, Failed, etc.
	CurrentTasks        []string      `json:"current_tasks,omitempty"`
	Workflow            *WorkflowInfo `json:"workflow,omitempty"`
	ParentWorkflowURL   string        `json:"parent_workflow_url,omitempty"`    // API endpoint
	ParentWorkflowUIURL string        `json:"parent_workflow_ui_url,omitempty"` // UI page URL
}
