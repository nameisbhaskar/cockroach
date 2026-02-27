// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// getStandaloneCommand creates the parent standalone command with subcommands for each plan.
func getStandaloneCommand(
	ctx context.Context, registries []planners.Registry, factory planners.PlannerManagerFactory,
) *cobra.Command {
	standaloneCmd := &cobra.Command{
		Use:   "standalone",
		Short: "Run a plan with in-memory execution and HTTP server in a single process",
		Long: `Standalone executes a plan entirely in-memory within a single process.

This command:
1. Starts an HTTP server for status queries and visualization
2. Initializes workers for specified plans (defaults to the executed plan)
3. Executes the specified workflow
4. Keeps the server running to serve status queries

Examples:
  tef-light standalone demo --input '{"name":"test"}' --with-plans demo,cluster-setup --port 8081
  tef-light standalone demo --input '{"name":"test"}' --with-plans-regex "pua.*" --port 8081

All execution happens in-process without external dependencies.`,
	}

	// Add subcommands for each plan
	for _, r := range registries {
		standaloneCmd.AddCommand(getStandaloneSubcommand(ctx, r, registries, factory))
	}

	return standaloneCmd
}

// getStandaloneSubcommand creates a subcommand for running a specific plan standalone.
func getStandaloneSubcommand(
	ctx context.Context,
	r planners.Registry,
	allRegistries []planners.Registry,
	factory planners.PlannerManagerFactory,
) *cobra.Command {
	var withPlans string
	var withPlansRegex string
	var planVariant string
	var port int
	var input string

	planName := r.GetPlanName()

	standaloneCmd := &cobra.Command{
		Use:   planName,
		Short: fmt.Sprintf("Run %s standalone: %s", planName, r.GetPlanDescription()),
		Long:  "Run the plan with in-memory execution and HTTP server in a single process",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStandalone(ctx, planName, withPlans, withPlansRegex, planVariant, port, input, allRegistries, factory)
		},
	}

	standaloneCmd.Flags().StringVar(&withPlans, "with-plans", "",
		"Comma-separated list of plans to start workers for (defaults to executed plan)")
	standaloneCmd.Flags().StringVar(&withPlansRegex, "with-plans-regex", "",
		"Regex pattern to match plan names for starting workers (e.g., 'pua' matches 'pua-*')")
	standaloneCmd.Flags().StringVarP(&planVariant, "plan-variant", "v", "",
		"Plan variant (defaults to auto-generated dev-UUID)")
	standaloneCmd.Flags().IntVar(&port, "port", 8081,
		"HTTP server port")
	standaloneCmd.Flags().StringVar(&input, "input", "",
		"JSON input for the workflow")

	// Register plan-specific flags
	r.AddStartWorkerCmdFlags(standaloneCmd)

	return standaloneCmd
}

// runStandalone executes the standalone command logic.
func runStandalone(
	ctx context.Context,
	planName, withPlans, withPlansRegex, planVariant string,
	port int,
	input string,
	registries []planners.Registry,
	factory planners.PlannerManagerFactory,
) error {
	logger := planners.LoggerFromContext(ctx)

	// Find the plan to execute (use the registries that have flags set)
	var targetRegistry planners.Registry
	registryMap := make(map[string]planners.Registry)
	for _, r := range registries {
		registryMap[r.GetPlanName()] = r
		if r.GetPlanName() == planName {
			targetRegistry = r
		}
	}

	if targetRegistry == nil {
		return fmt.Errorf("plan '%s' not found", planName)
	}

	// Determine which plans need workers
	workerPlans := []string{planName}
	if withPlans != "" && withPlansRegex != "" {
		return fmt.Errorf("cannot specify both --with-plans and --with-plans-regex")
	}

	if withPlans != "" {
		workerPlans = strings.Split(withPlans, ",")
	} else if withPlansRegex != "" {
		// Find all plan names matching the pattern
		matchingPlans, err := matchPlanNamesByRegex(withPlansRegex, registries)
		if err != nil {
			return err
		}

		workerPlans = matchingPlans
		logger.Info("Matched plans via regex", "count", len(matchingPlans), "pattern", withPlansRegex)
	}

	// Auto-generate plan variant if not provided
	if planVariant == "" {
		planVariant = fmt.Sprintf("dev-%s", uuid.New().String())
	}

	logger.Info("Starting standalone execution",
		"plan", planName,
		"worker_plans", workerPlans,
		"variant", planVariant,
		"port", port,
	)

	// Create managers for all worker plans
	managers := make(map[string]planners.PlannerManager)
	for _, planName := range workerPlans {
		registry, ok := registryMap[planName]
		if !ok {
			return fmt.Errorf("plan '%s' not found in --with-plans", planName)
		}

		manager, err := factory.CreateManager(ctx, registry)
		if err != nil {
			return fmt.Errorf("failed to create manager for plan %s: %w", planName, err)
		}

		planners.RegisterManager(planName, manager)
		managers[planName] = manager

		logger.Info("Registered in-memory worker", "plan", planName)
	}

	// Prepare execution for target plan
	if err := targetRegistry.PrepareExecution(ctx); err != nil {
		return fmt.Errorf("failed to prepare execution for plan %s: %w", planName, err)
	}

	// Start an HTTP server in the background
	addr := fmt.Sprintf("localhost:%d", port)

	// Use the target plan's manager as the shared manager since all in-memory managers
	// share the same storage backend
	sharedManager, ok := managers[planName]
	if !ok {
		return fmt.Errorf("manager not found for plan %s", planName)
	}

	srv, err := api.NewServer(registries, managers, addr, sharedManager)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		logger.Info("Starting HTTP server", "address", addr)
		if err := srv.Start(ctx); err != nil {
			errChan <- fmt.Errorf("server failed: %w", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Execute the workflow
	planID := planners.GetPlanID(planName, planVariant)
	manager := managers[planName]

	parsedInput, err := targetRegistry.ParsePlanInput(input)
	if err != nil {
		return fmt.Errorf("failed to parse input: %w", err)
	}

	logger.Info("Executing workflow", "plan_id", planID)
	workflowID, err := manager.ExecutePlan(ctx, parsedInput, planID)
	if err != nil {
		return fmt.Errorf("failed to execute plan: %w", err)
	}

	logger.Info("Workflow executed successfully",
		"workflow_id", workflowID,
		"plan_id", planID,
	)

	logger.Info("HTTP server running", "address", addr)
	logger.Info("Press Ctrl+C to stop")

	// Block forever, keeping server running
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
