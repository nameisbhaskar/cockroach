# Logger Usage in TEF

The TEF framework now has integrated logging support throughout the entire code flow.

## How the Logger Flows

1. **CLI Initialization** (`cli/initialiser.go`):
   - Creates a logger instance: `logger := planners.NewLogger()`
   - Attaches it to context: `ctx := planners.ContextWithLogger(context.Background(), logger)`
   - Passes context to all CLI commands

2. **Commands** (`cli/commands.go`):
   - Receives context with logger
   - Passes context to `temporal_planner.NewPlannerManager()`

3. **Plan Execution**:
   - Context flows through plan generation and execution
   - All executor functions receive context as first parameter

## Using the Logger in Executor Functions

Any executor function can access the logger from context:

```go
func setupCluster(ctx context.Context, input *demoData) (string, error) {
    // Get the logger from context
    logger := planners.LoggerFromContext(ctx)

    // Use the logger
    logger.Infof(ctx, "Setting up cluster: %s", input.Name)

    // Do work...
    time.Sleep(2 * time.Minute)

    logger.Infof(ctx, "Cluster setup completed: %s", input.Name)
    return fmt.Sprintf("cluster-%s-setup", input.Name), nil
}

func validateConfiguration(ctx context.Context, input *demoData) (string, error) {
    logger := planners.LoggerFromContext(ctx)

    logger.Infof(ctx, "Validating configuration for: %s", input.Name)
    time.Sleep(90 * time.Second)

    if rand.Intn(2) == 0 {
        logger.Errorf(ctx, "Validation failed for %s: configuration mismatch detected", input.Name)
        return "", fmt.Errorf("validation failed: configuration mismatch detected")
    }

    logger.Infof(ctx, "Configuration validated successfully for: %s", input.Name)
    return "validation-passed", nil
}
```

## Logger Interface

The logger provides three methods:

- `Infof(ctx context.Context, format string, args ...interface{})` - Log informational messages
- `Warningf(ctx context.Context, format string, args ...interface{})` - Log warning messages
- `Errorf(ctx context.Context, format string, args ...interface{})` - Log error messages

## Example: Using Logger in Plan Generation

```go
func (d *Demo) GeneratePlan(ctx context.Context, p planners.Planner) {
    logger := planners.LoggerFromContext(ctx)
    logger.Infof(ctx, "Generating plan: %s", d.GetPlanName())

    registerExecutors(ctx, p)

    // Create tasks...
    initTask := p.NewExecutionTask(ctx, "initialize environment")
    initTask.ExecutorFn = initializeEnvironment

    // More task creation...

    p.RegisterPlan(ctx, initTask, initTask)
    logger.Infof(ctx, "Plan generation completed: %s", d.GetPlanName())
}
```

## Notes

- The logger uses CockroachDB's `log.Dev` channel internally
- By default, logs go to stdout/stderr
- If no logger is found in context, `LoggerFromContext` returns a default logger instance (safe fallback)
- Context is already threaded through all TEF components, so no structural changes are needed
