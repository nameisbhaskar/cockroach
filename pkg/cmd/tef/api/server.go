// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/handlers/ui"
	v1 "github.com/cockroachdb/cockroach/pkg/cmd/tef/api/handlers/v1"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/middleware"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/gorilla/mux"
)

// Server represents the TEF REST API server.
type Server struct {
	registries        map[string]planners.Registry // Map from plan name to registry
	addr              string
	sharedPlanService planners.SharedPlanService // Shared service for framework-level operations
}

// NewServer creates a new TEF REST API server.
func NewServer(
	registries []planners.Registry,
	managers map[string]planners.PlannerManager,
	addr string,
	sharedPlanService planners.SharedPlanService,
) (*Server, error) {
	s := &Server{
		registries:        make(map[string]planners.Registry),
		addr:              addr,
		sharedPlanService: sharedPlanService,
	}

	// Store registries by plan name
	for _, r := range registries {
		planName := r.GetPlanName()
		s.registries[planName] = r
	}

	return s, nil
}

// Start starts the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	router := mux.NewRouter()

	// Create handler registries
	handlerRegistries := s.createHandlerRegistries()

	// Register all routes from handler registries
	for _, registry := range handlerRegistries {
		for _, route := range registry.GetRoutes() {
			router.HandleFunc(route.Path, route.Handler).Methods(route.Methods...)
		}
	}

	// Wrap router with middleware stack
	handler := middleware.TracingMiddleware(router)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    s.addr,
		Handler: handler,
	}

	logger := planners.LoggerFromContext(ctx)

	// Start server in a goroutine
	go func() {
		logger.Info("Starting TEF REST API server", "address", s.addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Server error", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	logger.Info("Shutting down server...")
	return httpServer.Shutdown(context.Background())
}

// createHandlerRegistries creates and returns all handler registries.
// Users can add new handlers by adding them to this slice.
func (s *Server) createHandlerRegistries() []HandlerRegistry {
	return []HandlerRegistry{
		v1.NewPlansHandler(s.registries, s.sharedPlanService),
		ui.NewWorkflowHandler(),
	}
}
