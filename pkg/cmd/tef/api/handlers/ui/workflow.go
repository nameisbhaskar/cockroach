// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package ui

import (
	"net/http"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/types"
)

// WorkflowHandler handles UI-related endpoints.
type WorkflowHandler struct{}

// NewWorkflowHandler creates a new WorkflowHandler.
func NewWorkflowHandler() *WorkflowHandler {
	return &WorkflowHandler{}
}

// GetRoutes returns the routes for UI endpoints.
func (h *WorkflowHandler) GetRoutes() []types.RouteHandler {
	return []types.RouteHandler{
		{
			Path:    "/",
			Handler: h.handleWorkflowUI,
			Methods: []string{"GET"},
		},
	}
}

// handleWorkflowUI serves the workflow visualization UI.
func (h *WorkflowHandler) handleWorkflowUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(workflowHTML))
}
