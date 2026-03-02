// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package cli provides the initialization logic for the TEF command-line interface.
// This file contains the main initialization sequence that sets up the CLI,
// creates the root command, and registers all plan-specific sub-commands.
package cli

import (
	"context"
	"os"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/spf13/cobra"
)

// Initialize sets up and executes the TEF CLI with worker commands and orchestration support.
// This enables dependency injection of framework-specific manager implementations (e.g., Temporal).
// The factory is used to create PlannerManager instances for each registered plan.
// The registry contains all plans that have been registered by the caller.
//
// This function is called from the private task-exec-framework repository's main.go:
// https://github.com/task-exec-framework
//
// The task-exec-framework repository provides the orchestration backend implementation.
func Initialize(factory planners.PlannerManagerFactory, pr *planners.PlanRegistry) {
	// Create a structured logger instance for the TEF CLI using slog.
	logger := planners.NewLogger("info")

	// Create a context with the logger attached.
	ctx := planners.ContextWithLogger(context.Background(), logger)

	// The root Cobra command is created with usage information.
	rootCmd := &cobra.Command{
		Use:   "tef [sub-command] [flags]",
		Short: "Task Execution Framework",
		Long:  "TEF is a task execution framework for managing and running plan sequences",
	}

	// Add worker CLI commands with the provided factory
	initializeWorkerCLI(ctx, rootCmd, pr.GetRegistries(), factory)

	// Execute the CLI and exit with an error code if execution fails.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// InitializeStandalone sets up and executes TEF CLI with only non-orchestration commands.
// This enables running commands like gen-view directly from the cockroach repository
// without requiring an orchestration backend (e.g., Temporal).
//
// This function is called from pkg/cmd/tef/main.go in the cockroach repository
// to build a standalone binary with limited functionality.
//
// The factory is used to create in-memory PlannerManager instances for each plan.
// The registry contains all plans that have been registered by the caller.
//
// Supported commands:
//   - gen-view: Generate visual diagrams of plan workflows
//   - standalone: Run workflows in-memory with HTTP server for testing
//
// For commands that require orchestration (start-worker, execute, resume, serve),
// use Initialize from a repository that provides an orchestration backend.
func InitializeStandalone(factory planners.PlannerManagerFactory, pr *planners.PlanRegistry) {
	// Create a structured logger instance for the TEF CLI using slog.
	logger := planners.NewLogger("info")

	// Create a context with the logger attached.
	ctx := planners.ContextWithLogger(context.Background(), logger)

	// The root Cobra command is created with usage information.
	rootCmd := &cobra.Command{
		Use:   "tef-light [sub-command] [flags]",
		Short: "Task Execution Framework",
		Long: `TEF is a task execution framework for managing and running plan sequences.

This standalone binary supports commands that don't require orchestration:
  - gen-view: Generate visual diagrams of plan workflows
  - standalone: Run workflows in-memory for testing and visualization

For commands that require orchestration (start-worker, execute, resume, serve),
build the TEF binary from a repository that provides an orchestration backend
(e.g., github.com/task-exec-framework).`,
	}

	// Add standalone CLI commands with the in-memory factory
	initializeStandaloneCLI(ctx, rootCmd, pr.GetRegistries(), factory)

	// Execute the CLI and exit with an error code if execution fails.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
