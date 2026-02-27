# In-Memory TEF Execution

This package provides an in-memory implementation of the TEF orchestration backend for testing and development.

## Overview

The in-memory backend executes workflows asynchronously in background goroutines within a single process, without requiring external dependencies like Temporal. This makes it ideal for:

- **Testing**: Validate workflow logic without orchestration infrastructure
- **Development**: Quickly iterate on plan definitions with instant feedback
- **Visualization**: Generate and view workflow graphs via HTTP API

## Architecture

### Components

The package exports only `NewInMemoryFactory()` as its public API. All internal types are private:

- **factory**: Creates manager instances with shared storage
- **manager**: Implements `PlannerManager` for async workflow execution
- **storage**: Thread-safe storage for workflow execution state
- **workflowExecutor**: Handles end-to-end workflow execution in background goroutines

### Key Differences from Production

| Feature | In-Memory | Production (Temporal) |
|---------|-----------|----------------------|
| Execution | Async (goroutines) | Distributed, async |
| Durability | Lost on restart | Persisted |
| Workers | In-process | Separate processes |
| Scalability | Single process | Horizontally scalable |

## Usage

### Via tef-light standalone Command

The `tef-light standalone` command runs everything in a single process:

```bash
# Execute a workflow with in-memory backend (comma-separated plans)
tef-light standalone demo \
  --input '{"cluster":"test"}' \
  --with-plans demo,setup-cluster \
  --plan-variant dev \
  --port 8081

# Execute a workflow with in-memory backend (regex pattern)
tef-light standalone demo \
  --input '{"cluster":"test"}' \
  --with-plans-regex "pua.*" \
  --plan-variant dev \
  --port 8081
```

This command:
1. Starts an HTTP server on the specified port
2. Initializes workers for all specified plans (either via `--with-plans` comma-separated list or `--with-plans-regex` pattern)
3. Executes the workflow in a background goroutine
4. Keeps the server running for status queries and resume operations

### Programmatic Usage

```go
import (
	"context"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners/inmemory"
)

// Create factory with shared storage
factory := inmemory.NewInMemoryFactory()

// Create managers for each plan
manager, err := factory.CreateManager(ctx, registry)
if err != nil {
	return err
}

// Execute workflow (returns immediately)
workflowID, err := manager.ExecutePlan(ctx, input, planID)

// Query status
status, err := manager.GetExecutionStatus(ctx, planID, workflowID)

// Resume callback task
err = manager.ResumeTask(ctx, planID, workflowID, stepID, result)
```

## Implementation Details

### Execution Flow

```
ExecutePlan()
  ↓
Generate workflow ID and run ID
  ↓
Create execution state in storage
  ↓
Start workflow in background goroutine:
  ├─ execute()
  │   ↓
  │  Create PlanExecutionInfo
  │   ↓
  │  Start from planner.First
  │   ↓
  │  Loop through tasks:
  │   - Execute current task
  │   - Handle errors → follow Fail path if available
  │   - On success → follow Next path
  │   - Store results in taskResults map
  │   - Callback tasks block waiting for resume
  │   ↓
  │  Return output from planner.Output task
  │   ↓
  └─ Update final execution status
  ↓
Return workflow ID immediately
```

**Key Point**: `ExecutePlan` returns immediately after starting the background goroutine. The workflow continues running asynchronously, and callback tasks block the workflow goroutine (not the caller) until resumed.

### Task Type Implementations

#### ExecutionTask

Executes a registered executor function with parameters:

1. Gather parameters from previous task results
2. Build argument list: `(ctx, execInfo, input, ...params, [errorString])`
3. Call executor function using reflection
4. Store result for future tasks
5. Return next task or error

#### ForkTask

Executes multiple branches in parallel:

1. Spawn goroutine for each branch
2. Each branch executes until it reaches the Join point
3. Wait for all branches to complete
4. Check for errors from any branch
5. Continue from fork's Next task

**Current limitation**: Only `ExecutionTask` is supported in fork branches.

#### ConditionTask

Conditional branching based on boolean result:

1. Gather parameters
2. Call executor function (returns `bool, error`)
3. Branch to `Then` if true, `Else` if false

#### CallbackTask

Async operation that waits for external resumption:

1. Call `ExecutionFn` to get step ID
2. Register callback channel in storage
3. Update state to "waiting_callback"
4. **Block** waiting for callback via channel
5. When resumed, call `ResultProcessorFn` with result
6. Store processed result and continue

**Important**: Callback tasks block the workflow goroutine (not the caller) until resumed via HTTP API.

```go
// Register callback channel
ch := make(chan string)
storage.RegisterCallback(planID, workflowID, stepID, ch)

// Block waiting for resume
result := <-ch

// Resume via HTTP API
curl -X POST http://localhost:8081/api/v1/resume \
  -d '{"plan_id":"...","workflow_id":"...","step_id":"...","result":"..."}'
```

#### ChildPlanTask

Executes a child plan synchronously:

1. Call `ChildTaskInfoFn` to get child plan info (variant + input)
2. Look up manager for child plan
3. Execute child plan via `manager.ExecutePlan()`
4. Wait for child completion using `storage.WaitForCompletion()`
5. Store child workflow ID as result
6. Continue to next task

**Requirement**: Worker for child plan must be registered via `--with-plans`.

#### EndTask

Marks workflow completion - no further execution.

#### ForkJoinTask

Synchronization barrier (currently handled by fork task logic).

### State Tracking

Each task execution updates the storage with state transitions:

```
"Pending" → "InProgress" → "Completed"
                        ↓
                     "Failed"
```

Callback tasks remain in "InProgress" status while waiting for the callback.

#### Runtime State Merging

The `WorkflowInfo` structure contains both the static plan definition (task names, types, connections) and the runtime execution state (status, start time, end time, output, error). When a workflow is created:

1. The plan structure is serialized via `SerializePlanStructure()` - this captures the task graph
2. During execution, step states are updated in the `Steps` map with runtime information
3. When querying execution status, `ToExecutionStatus()` merges the step states into the workflow's task info

This allows the API to return complete execution status showing both the workflow structure and each task's current state.

### Error Handling

When a task fails:
1. Check if task has a `Fail` path
2. If yes, follow the failure path with `lastError` passed to next task
3. If no, workflow fails immediately with error

Tasks in failure paths receive an additional `errorString` parameter.

### Parameter Passing

Tasks can reference previous task results via `Params`:

```go
task1 := planner.NewExecutionTask(ctx, "task1")
task1.ExecutorFn = executeTask1 // Returns result1

task2 := planner.NewExecutionTask(ctx, "task2")
task2.ExecutorFn = executeTask2 // func(ctx, info, input, result1)
task2.Params = []Task{task1}    // Passes task1's result
```

The executor automatically gathers and passes these parameters.

### Synchronization

- **Fork branches**: Execute in parallel using goroutines
- **Callback tasks**: Block on channel until resumed
- **Child plans**: Execute synchronously (block until child completes via `WaitForCompletion`)
- **Results storage**: Protected by `sync.RWMutex`

## Limitations

- **No distributed execution**: All tasks run in a single process
- **No persistence**: State is lost when the process terminates
- **No retries**: Failed tasks don't retry (unless explicitly in the plan)
- **Single process**: Cannot scale horizontally
- **Limited fork support**: Only ExecutionTask in fork branches
- **Blocking callbacks**: Callback tasks block the workflow goroutine until resumed (but ExecutePlan returns immediately)

## Testing

The in-memory backend is ideal for unit and integration tests:

```go
func TestWorkflow(t *testing.T) {
	factory := inmemory.NewInMemoryFactory()
	manager, _ := factory.CreateManager(ctx, registry)

	workflowID, err := manager.ExecutePlan(ctx, input, planID)
	require.NoError(t, err)

	// Verify execution status
	status, _ := manager.GetExecutionStatus(ctx, planID, workflowID)
	require.Equal(t, "Completed", status.Status)

	// Check individual task statuses
	for _, task := range status.Workflow.Tasks {
		// Status values: "Pending", "InProgress", "Completed", "Failed"
		require.Contains(t, []string{"Pending", "InProgress", "Completed", "Failed"}, task.Status)
	}
}
```

## Future Enhancements

Potential improvements:

1. Support all task types in fork branches
2. Add timeout support for callback tasks
3. Implement task retries with configurable policies
4. Add execution history/audit trail
5. Support workflow cancellation
6. Implement workflow pause/resume
7. Add performance metrics and tracing

## See Also

- `/pkg/cmd/tef/cli/standalone.go`: Command implementation
- `/pkg/cmd/tef/planners/definitions.go`: Core interfaces
- `/pkg/cmd/tef/api/`: HTTP API handlers
- `executor.go`: Task execution engine implementation
- `storage.go`: State storage implementation
- `manager.go`: Manager implementation
