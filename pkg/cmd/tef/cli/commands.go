// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// TODO: Implement CLI command generation in initializeWorkerCLI.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/sdk"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// getFirstManagerDeterministic returns the first manager from a map in a deterministic way.
// This is necessary because Go randomizes map iteration order. By sorting the keys and
// selecting the first one alphabetically, we ensure consistent behavior across multiple
// calls to GetAllManagers().
func getFirstManagerDeterministic(
	managers map[string]planners.PlannerManager,
) planners.PlannerManager {
	if len(managers) == 0 {
		return nil
	}

	// Sort keys to get deterministic ordering
	keys := make([]string, 0, len(managers))
	for k := range managers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Return manager for the first key alphabetically
	return managers[keys[0]]
}

// initializeCommonCLI adds commands that work in both worker and standalone modes.
// These commands don't require orchestration backend:
//   - gen-view: Generate visual diagrams of plan workflows
//   - list-plans: List all available plans via API
//   - list-executions: List executions for a plan via API
//   - get-status: Get execution status via API
func initializeCommonCLI(ctx context.Context, rootCmd *cobra.Command, rs []planners.Registry) {
	// Create a gen-view parent command
	genViewCmd := &cobra.Command{
		Use:   "gen-view",
		Short: "Generate a view for a plan",
		Long:  "Generate the view for the plan sequence",
	}

	// Add gen-view subcommands for each plan
	for _, r := range rs {
		genViewCmd.AddCommand(getViewPlanSubcommand(ctx, r))
	}

	// Add regex-based gen-view subcommand
	genViewCmd.AddCommand(getRegexViewPlanSubcommand(ctx, rs))

	// Add to root command
	rootCmd.AddCommand(genViewCmd)

	// Add API-based commands (manager-agnostic)
	rootCmd.AddCommand(getListPlansCommand(ctx))
	rootCmd.AddCommand(getListExecutionsCommand(ctx))
	rootCmd.AddCommand(getExecutionStatusCommand(ctx))
}

// initializeWorkerCLI creates and registers CLI commands for worker operations with orchestration.
// For each registered plan, this function generates the following commands:
//   - start-worker <planname>: Start a worker to process plan executions
//   - execute <planname>: Execute the plan with JSON input
//   - resume <planname>: Resume a callback task (if applicable)
//   - serve: Start the REST API server
//
// This function is called from Initialize (used by task-exec-framework repo).
// The context passed should contain a logger via planners.ContextWithLogger.
// The factory is used to create PlannerManager instances for each plan.
func initializeWorkerCLI(
	ctx context.Context,
	rootCmd *cobra.Command,
	rs []planners.Registry,
	factory planners.PlannerManagerFactory,
) {
	// Create parent commands
	executeCmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute a plan",
		Long:  "Execute a plan sequence",
	}

	startWorkerCmd := &cobra.Command{
		Use:   "start-worker",
		Short: "Start a worker for a plan",
		Long:  "Start a worker that executes the specified plan sequence",
	}

	resumeCmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume an async task in a plan execution",
		Long:  "Resume an async task that is waiting for external input in a running plan execution",
	}

	// Create managers for each registry upfront using the injected factory.
	// Managers are automatically registered in the global registry via the factory.
	for _, r := range rs {
		// The manager is created via factory to ensure that the plan is correct.
		// The command execution creates a panic if the workflow is wrongly configured.
		// We use this manager to add planner-specific flags.
		manager, err := factory.CreateManager(ctx, r)
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize the plan manager for plan %s: %v", r.GetPlanName(), err))
		}
		planners.RegisterManager(r.GetPlanName(), manager)

		// Add subcommands for each plan under the parent commands
		executeCmd.AddCommand(getExecutePlanSubcommand(ctx, r, manager))
		startWorkerCmd.AddCommand(getStartWorkerSubcommand(ctx, r, manager))
		resumeCmd.AddCommand(getResumePlanSubcommand(ctx, r, manager))
	}

	// Add regex-based multi-worker subcommand
	startWorkerCmd.AddCommand(getRegexStartWorkerSubcommand(ctx, rs))

	// Add common commands (gen-view, list-plans, etc.)
	initializeCommonCLI(ctx, rootCmd, rs)

	// Add worker-specific commands to the root
	rootCmd.AddCommand(executeCmd, startWorkerCmd, resumeCmd)

	// Get a shared manager for commands that need planner-specific flags.
	// Use deterministic selection to ensure consistency.
	managers := planners.GetAllManagers()
	sharedManager := getFirstManagerDeterministic(managers)

	rootCmd.AddCommand(getServeCommand(ctx, rs, sharedManager))
}

// initializeStandaloneCLI creates and registers CLI commands for standalone mode.
// This includes common commands (gen-view, list-plans, etc.) plus the standalone command
// for in-memory execution without orchestration backend.
//
// This function is called from InitializeStandalone (used by cockroach repo's tef-light binary).
// The factory is used to create in-memory PlannerManager instances for each plan.
func initializeStandaloneCLI(
	ctx context.Context,
	rootCmd *cobra.Command,
	rs []planners.Registry,
	factory planners.PlannerManagerFactory,
) {
	// Add common commands that work everywhere
	initializeCommonCLI(ctx, rootCmd, rs)

	// Add standalone command for in-memory execution
	rootCmd.AddCommand(getStandaloneCommand(ctx, rs, factory))
}

// getStartWorkerSubcommand creates a Cobra subcommand for starting a plan-specific worker.
// The worker listens for workflow executions and runs until terminated.
func getStartWorkerSubcommand(
	ctx context.Context, r planners.Registry, manager planners.PlannerManager,
) *cobra.Command {
	var planVariant string

	startWorkerCmd := &cobra.Command{
		Use:   r.GetPlanName(),
		Short: fmt.Sprintf("Start a worker for %s: %s", r.GetPlanName(), r.GetPlanDescription()),
		Long:  "Start a worker that executes the specified plan sequence",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := planners.LoggerFromContext(ctx)
			logger.Info("Starting worker for plan", "plan", r.GetPlanName())
			logger.Info("Description", "description", r.GetPlanDescription())

			// Auto-generate plan variant if isn't provided
			if planVariant == "" {
				planVariant = fmt.Sprintf("dev-%s", uuid.New().String())
			}

			planID := planners.GetPlanID(r.GetPlanName(), planVariant)

			err := r.PrepareExecution(ctx)
			if err != nil {
				panic(fmt.Sprintf("Failed to initialize the plan %s: %v", r.GetPlanName(), err))
			}

			// Flag parsing has populated the manager's flags (temporal-address, temporal-namespace, etc.)
			// Clone these properties to all other managers so child workflows can use them
			allManagers := planners.GetAllManagers()
			for _, otherManager := range allManagers {
				if otherManager != manager {
					otherManager.ClonePropertiesFrom(manager)
				}
			}

			// Now we can use it to start the worker
			return manager.StartWorker(ctx, planID)
		},
	}

	// Allow planner manager to add planner-specific flags (e.g., temporal-address, temporal-namespace)
	manager.AddPlannerFlags(startWorkerCmd)

	// Add a plan-variant flag
	startWorkerCmd.Flags().StringVarP(&planVariant, "plan-variant", "v", "", "Plan variant for creating unique plan instances (auto-generated UUID if not provided for workers)")

	// Allow registry to add worker-specific flags
	r.AddStartWorkerCmdFlags(startWorkerCmd)

	return startWorkerCmd
}

// getRegexStartWorkerSubcommand creates a Cobra subcommand for starting workers for multiple plans
// that match a regex pattern. This allows one process to handle workers for multiple plan types.
func getRegexStartWorkerSubcommand(ctx context.Context, rs []planners.Registry) *cobra.Command {
	var regexPattern string
	var planVariant string

	regexWorkerCmd := &cobra.Command{
		Use:   "regex",
		Short: "Start workers for all plans matching a regex pattern",
		Long:  "Start workers for multiple plans in a single process by matching plan names against a regex pattern (e.g., 'pua' matches 'pua-*')",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := planners.LoggerFromContext(ctx)

			if regexPattern == "" {
				return fmt.Errorf("--regex-plan-name flag is required")
			}

			// Find all registries matching the pattern
			matchingRegistries, err := matchRegistriesByRegex(regexPattern, rs)
			if err != nil {
				return err
			}

			logger.Info("Starting workers for plans matching pattern", "count", len(matchingRegistries), "pattern", regexPattern)
			for _, r := range matchingRegistries {
				logger.Info("  - plan", "name", r.GetPlanName(), "description", r.GetPlanDescription())
			}

			// Auto-generate plan variant if not provided
			if planVariant == "" {
				planVariant = fmt.Sprintf("dev-%s", uuid.New().String())
			}

			// Clone properties from the first manager to all others
			// This ensures all managers have the same temporal connection configuration.
			// Use deterministic selection to ensure we clone from the same manager that received the flags.
			allManagers := planners.GetAllManagers()
			if len(allManagers) > 0 {
				sourceManager := getFirstManagerDeterministic(allManagers)
				for _, otherManager := range allManagers {
					if otherManager != sourceManager {
						otherManager.ClonePropertiesFrom(sourceManager)
					}
				}
			}

			// Start workers for all matching plans in goroutines
			var wg sync.WaitGroup
			errChan := make(chan error, len(matchingRegistries))

			for _, r := range matchingRegistries {
				wg.Add(1)
				go func(registry planners.Registry) {
					defer wg.Done()

					planName := registry.GetPlanName()
					logger.Info("Starting worker for plan", "plan", planName)

					// Get the manager for this plan from the global registry
					manager, err := planners.GetManagerForPlan(planName)
					if err != nil {
						errChan <- fmt.Errorf("failed to get manager for plan %s: %w", planName, err)
						return
					}

					// Prepare execution for this plan
					if err := registry.PrepareExecution(ctx); err != nil {
						errChan <- fmt.Errorf("failed to prepare execution for plan %s: %w", planName, err)
						return
					}

					// Create plan ID with variant
					planID := planners.GetPlanID(planName, planVariant)

					// Start the worker
					if err := manager.StartWorker(ctx, planID); err != nil {
						errChan <- fmt.Errorf("worker for plan %s failed: %w", planName, err)
						return
					}
				}(r)
			}

			// Wait for all workers to complete or until one errors
			go func() {
				wg.Wait()
				close(errChan)
			}()

			// Check for errors from any worker
			for err := range errChan {
				if err != nil {
					return err
				}
			}

			logger.Info("All workers completed successfully")
			return nil
		},
	}

	// Add flags
	regexWorkerCmd.Flags().StringVar(&regexPattern, "regex-plan-name", "", "Regex pattern to match plan names (required)")
	regexWorkerCmd.Flags().StringVarP(&planVariant, "plan-variant", "v", "", "Plan variant for creating unique plan instances (auto-generated UUID if not provided)")

	// Get the first manager (deterministically) for shared planner flags.
	// This must be the same manager used as the clone source in the RunE function.
	managers := planners.GetAllManagers()
	if firstManager := getFirstManagerDeterministic(managers); firstManager != nil {
		firstManager.AddPlannerFlags(regexWorkerCmd)
	}

	return regexWorkerCmd
}

// getExecutePlanSubcommand creates a Cobra subcommand for executing a plan with provided input.
// The input is parsed from the command argument and passed directly to the planner manager.
func getExecutePlanSubcommand(
	ctx context.Context, r planners.Registry, manager planners.PlannerManager,
) *cobra.Command {
	executePlanCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <input> <plan_id>", r.GetPlanName()),
		Short: fmt.Sprintf("Execute the plan %s: %s", r.GetPlanName(), r.GetPlanDescription()),
		Long:  "Execute the specified plan sequence",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := planners.LoggerFromContext(ctx)
			logger.Info("Executing plan", "plan", r.GetPlanName())
			logger.Info("Description", "description", r.GetPlanDescription())

			// Clone properties to all other managers so child workflows can use them
			allManagers := planners.GetAllManagers()
			for _, otherManager := range allManagers {
				if otherManager != manager {
					otherManager.ClonePropertiesFrom(manager)
				}
			}

			// Execute directly via planner manager
			return executePlan(ctx, r, manager, args[0], args[1])
		},
	}
	// Allow planner manager to add planner-specific flags (e.g., temporal-address, temporal-namespace)
	manager.AddPlannerFlags(executePlanCmd)

	return executePlanCmd
}

// executePlan executes a plan directly using the planner manager.
func executePlan(
	ctx context.Context,
	registry planners.Registry,
	manager planners.PlannerManager,
	inputStr string,
	planID string,
) error {
	// Parse plan input using the registry
	input, err := registry.ParsePlanInput(inputStr)
	if err != nil {
		return fmt.Errorf("failed to parse plan input: %w", err)
	}

	// Validate that the plan ID matches the plan name
	planNameFromID, _, err := planners.ExtractPlanNameAndVariant(planID)
	if err != nil {
		return fmt.Errorf("invalid plan ID format: %w", err)
	}
	if planNameFromID != registry.GetPlanName() {
		return fmt.Errorf("plan ID '%s' does not match plan name '%s'", planID, registry.GetPlanName())
	}

	// Execute plan directly via manager
	workflowID, err := manager.ExecutePlan(ctx, input, planID)
	if err != nil {
		return fmt.Errorf("failed to execute plan: %w", err)
	}

	logger := planners.LoggerFromContext(ctx)
	logger.Info("Plan executed successfully")
	logger.Info("Workflow ID", "workflow_id", workflowID)
	logger.Info("Plan ID", "plan_id", planID)
	return nil
}

func getViewPlanSubcommand(ctx context.Context, r planners.Registry) *cobra.Command {
	var withFailurePath bool

	viewPlanCmd := &cobra.Command{
		Use:   r.GetPlanName(),
		Short: fmt.Sprintf("Generate the view for the plan %s: %s", r.GetPlanName(), r.GetPlanDescription()),
		Long:  "Generate the view for the plan sequence",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A planner manager is created, and the plan is validated during initialization.
			// We pass empty strings for config since flags haven't been parsed yet.
			planner, err := planners.NewBasePlanner(ctx, r)
			if err != nil {
				return err
			}
			logger := planners.LoggerFromContext(ctx)
			logger.Info("Generating view for the plan", "plan", r.GetPlanName())
			logger.Info("Description", "description", r.GetPlanDescription())
			// The input is parsed from the first command argument.
			// The plan is executed with the parsed input.
			return planners.GeneratePlan(ctx, planner, r.GetPlanName(), withFailurePath)
		},
	}

	// Add flag for controlling failure path display
	viewPlanCmd.Flags().BoolVar(&withFailurePath, "with-failure-path", false, "Include failure paths in the generated view")

	return viewPlanCmd
}

// getRegexViewPlanSubcommand creates a Cobra subcommand for generating views for multiple plans
// that match a regex pattern. This allows generating views for multiple plan types at once.
func getRegexViewPlanSubcommand(ctx context.Context, rs []planners.Registry) *cobra.Command {
	var regexPattern string
	var withFailurePath bool

	regexViewCmd := &cobra.Command{
		Use:   "regex",
		Short: "Generate views for all plans matching a regex pattern",
		Long:  "Generate views for multiple plans by matching plan names against a regex pattern (e.g., 'pua' matches 'pua-*')",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := planners.LoggerFromContext(ctx)

			if regexPattern == "" {
				return fmt.Errorf("--regex-plan-name flag is required")
			}

			// Find all registries matching the pattern
			matchingRegistries, err := matchRegistriesByRegex(regexPattern, rs)
			if err != nil {
				return err
			}

			logger.Info("Generating views for plans matching pattern", "count", len(matchingRegistries), "pattern", regexPattern)
			for _, r := range matchingRegistries {
				logger.Info("  - plan", "name", r.GetPlanName(), "description", r.GetPlanDescription())
			}

			// Generate views for all matching plans
			for _, r := range matchingRegistries {
				logger.Info("Generating view for plan", "plan", r.GetPlanName())
				logger.Info("Description", "description", r.GetPlanDescription())

				planner, err := planners.NewBasePlanner(ctx, r)
				if err != nil {
					return fmt.Errorf("failed to create planner for plan %s: %w", r.GetPlanName(), err)
				}

				err = planners.GeneratePlan(ctx, planner, r.GetPlanName(), withFailurePath)
				if err != nil {
					return fmt.Errorf("failed to generate view for plan %s: %w", r.GetPlanName(), err)
				}
			}

			logger.Info("Successfully generated views for all matching plans")
			return nil
		},
	}

	// Add flags
	regexViewCmd.Flags().StringVar(&regexPattern, "regex-plan-name", "", "Regex pattern to match plan names (required)")
	regexViewCmd.Flags().BoolVar(&withFailurePath, "with-failure-path", false, "Include failure paths in the generated view")

	return regexViewCmd
}

// getResumePlanSubcommand creates a Cobra subcommand for resuming an async task in a plan execution.
// The command can resume via REST API or directly, controlled by the --use-api flag.
func getResumePlanSubcommand(
	ctx context.Context, r planners.Registry, manager planners.PlannerManager,
) *cobra.Command {
	var useAPI bool
	var apiBaseUrl string
	resumePlanCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <plan_id> <workflow_id> <step_id> <result>", r.GetPlanName()),
		Short: fmt.Sprintf("Resume an async task for plan %s: %s", r.GetPlanName(), r.GetPlanDescription()),
		Long:  "Resume an async task that is waiting for external input in a running plan execution",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			planID := args[0]
			workflowID := args[1]
			stepID := args[2]
			result := args[3]

			logger := planners.LoggerFromContext(ctx)
			logger.Info("Resuming async task for plan", "plan", r.GetPlanName())
			logger.Info("Resuming async task", "workflow_id", workflowID, "step_id", stepID)

			if useAPI {
				// Resume via REST API
				return resumeViaAPI(ctx, planID, workflowID, stepID, result, apiBaseUrl)
			}

			// Clone properties to all other managers for consistency
			allManagers := planners.GetAllManagers()
			for _, otherManager := range allManagers {
				if otherManager != manager {
					otherManager.ClonePropertiesFrom(manager)
				}
			}

			// Resume the task directly via manager
			err := manager.ResumeTask(ctx, planID, workflowID, stepID, result)
			if err != nil {
				return err
			}

			logger.Info("Successfully resumed async task")
			return nil
		},
	}

	// Add REST API flags
	resumePlanCmd.Flags().BoolVar(&useAPI, "use-api", true, "Use REST API for resuming")
	resumePlanCmd.Flags().StringVar(&apiBaseUrl, "api-base-url", "http://localhost:8081", "API server base url")

	return resumePlanCmd
}

// resumeViaAPI resumes an async task via the REST API.
func resumeViaAPI(
	ctx context.Context, planID, workflowID, stepID, result, apiBaseUrl string,
) error {
	// Create API client
	client := sdk.NewClient(apiBaseUrl)

	// Resume via API
	resp, err := client.ResumePlan(ctx, planID, workflowID, stepID, result)
	if err != nil {
		return fmt.Errorf("failed to resume task via API: %w", err)
	}

	logger := planners.LoggerFromContext(ctx)
	logger.Info("Task resumed successfully via API")
	logger.Info("Workflow ID", "workflow_id", resp.WorkflowID)
	logger.Info("Step ID", "step_id", resp.StepID)
	logger.Info("Success", "success", resp.Success)
	return nil
}

// getServeCommand creates a Cobra command for starting the REST API server.
func getServeCommand(
	ctx context.Context, rs []planners.Registry, sharedManager planners.PlannerManager,
) *cobra.Command {
	var host string
	var port int

	// Get managers from the global registry
	managers := planners.GetAllManagers()

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the TEF REST API server",
		Long:  "Start a REST API server that exposes endpoints for executing plans",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := fmt.Sprintf("%s:%d", host, port)

			// Clone properties from the shared manager to all other managers
			// This ensures all managers have the same temporal connection configuration
			if sharedManager != nil {
				for _, otherManager := range managers {
					if otherManager != sharedManager {
						otherManager.ClonePropertiesFrom(sharedManager)
					}
				}
			}

			// Use the managers from the global registry
			// Their flags have been populated by flag parsing
			srv, err := api.NewServer(rs, managers, addr, sharedManager)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}
			return srv.Start(ctx)
		},
	}

	serveCmd.Flags().StringVar(&host, "host", "localhost", "Server host address")
	serveCmd.Flags().IntVar(&port, "port", 8081, "Server port")

	// Allow planner manager to add planner-specific flags
	if sharedManager != nil {
		sharedManager.AddPlannerFlags(serveCmd)
	}

	return serveCmd
}

// getListPlansCommand creates a Cobra command for listing all plans.
func getListPlansCommand(ctx context.Context) *cobra.Command {
	var apiBaseUrl string

	listPlansCmd := &cobra.Command{
		Use:   "list-plans",
		Short: "List all available plans",
		Long:  "Query the execution framework to list all registered plans and their active instances",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := planners.LoggerFromContext(ctx)

			// Create API client
			client := sdk.NewClient(apiBaseUrl)

			// List plans via API
			resp, err := client.ListPlans(ctx)
			if err != nil {
				return fmt.Errorf("failed to list plans via API: %w", err)
			}

			// Pretty print the response
			respJSON, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal response: %w", err)
			}

			logger.Info("Available plans", "plans", string(respJSON))
			return nil
		},
	}

	listPlansCmd.Flags().StringVar(&apiBaseUrl, "api-base-url", "http://localhost:8081", "API server base url")

	return listPlansCmd
}

// getListExecutionsCommand creates a Cobra command for listing executions for a plan.
func getListExecutionsCommand(ctx context.Context) *cobra.Command {
	var apiBaseUrl string

	listExecutionsCmd := &cobra.Command{
		Use:   "list-executions <plan_id>",
		Short: "List all executions for a plan",
		Long:  "Query the execution framework to list all executions for a specific plan instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planID := args[0]
			logger := planners.LoggerFromContext(ctx)

			// Create API client
			client := sdk.NewClient(apiBaseUrl)

			// List executions via API
			resp, err := client.ListExecutions(ctx, planID)
			if err != nil {
				return fmt.Errorf("failed to list executions via API: %w", err)
			}

			// Pretty print the response
			respJSON, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal response: %w", err)
			}

			logger.Info("Executions for plan", "plan_id", planID, "executions", string(respJSON))
			return nil
		},
	}

	listExecutionsCmd.Flags().StringVar(&apiBaseUrl, "api-base-url", "http://localhost:8081", "API server base url")

	return listExecutionsCmd
}

// getExecutionStatusCommand creates a Cobra command for getting execution status.
func getExecutionStatusCommand(ctx context.Context) *cobra.Command {
	var apiBaseUrl string

	getStatusCmd := &cobra.Command{
		Use:   "get-status <plan_id> <execution_id>",
		Short: "Get the status of a plan execution",
		Long:  "Query the execution framework to get the current status of a specific plan execution",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planID := args[0]
			executionID := args[1]
			logger := planners.LoggerFromContext(ctx)

			// Create API client
			client := sdk.NewClient(apiBaseUrl)

			// Get execution status via API
			resp, err := client.GetExecutionStatus(ctx, planID, executionID)
			if err != nil {
				return fmt.Errorf("failed to get execution status via API: %w", err)
			}

			// Pretty print the response
			respJSON, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal response: %w", err)
			}

			logger.Info("Execution status", "plan_id", planID, "execution_id", executionID, "status", string(respJSON))
			return nil
		},
	}

	getStatusCmd.Flags().StringVar(&apiBaseUrl, "api-base-url", "http://localhost:8081", "API server base url")

	return getStatusCmd
}
