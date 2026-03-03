---
name: tef-plan-generator
description: Interactive assistant for creating skeleton plans for the Task Execution Framework (TEF). Asks questions to understand tasks and relationships, then generates a complete plan structure.
---

# TEF Plan Generator Skill

This skill helps you create skeleton plans for the Task Execution Framework (TEF) by asking targeted questions to understand your workflow requirements and generating the appropriate plan structure.

## When to Use This Skill

Use this skill when:
- Creating a new TEF plan from scratch
- Designing complex multi-step workflows for CockroachDB operations
- Need to translate operational requirements into TEF task graphs
- Want to prototype a workflow structure before implementing executor functions

## What This Skill Does

The skill will:

1. **Understand Plan Scope**: Ask about the plan's purpose, name, and input requirements
2. **Identify Tasks**: Gather information about each task that needs to be performed
3. **Determine Task Relationships**: Understand sequencing, parallelism, conditionals, and error handling
4. **Generate Skeleton Code**: Create a complete plan.go file with:
   - Plan struct and Registry interface implementation
   - Input data structure
   - GeneratePlan() with complete task graph
   - Stub executor functions with proper signatures
   - Executor registration
   - Plan registration function (Register[PlanName]Plans)
5. **Register Plan**: Automatically add the plan to `pkg/cmd/tef/plans/registry.go` to make it available in TEF CLI

## Question Flow

### Phase 1: Plan Metadata

Ask these questions to establish basic plan information:

1. **Plan Purpose**: "What is the main purpose of this plan? (e.g., 'Provision and configure a CockroachDB cluster for performance testing')"

2. **Plan Name**: "What should the plan be named? (lowercase, no spaces, e.g., 'cluster-setup', 'perf-test', 'data-migration')"

3. **Plan Description**: "Provide a brief description for the plan (will be shown in CLI help)"

4. **Input Requirements**: "What input data does this plan need from users? Describe the fields and their types (e.g., cluster_name: string, node_count: int, region: string)"

### Phase 2: Task Discovery

For each task in the workflow:

1. **Task List**: "List all the tasks/steps this plan needs to perform, in rough order. Include what each task does."
   - Example: "1. Initialize cluster configuration, 2. Provision nodes, 3. Configure networking, 4. Import initial data, 5. Run health checks"

2. **For each task, ask:**
   - **Task Type**: "What type of task is this?"
     - `execution`: Performs an operation and returns a result
     - `fork`: Executes multiple tasks in parallel
     - `condition`: Makes a true/false decision and branches
     - `callback`: Starts an async operation and waits for external completion signal
     - `child-plan`: Executes another TEF plan as a subtask

   - **Input/Output**: "What does this task receive as input and what does it output?"

   - **Error Handling**: "What should happen if this task fails? (continue to next task, cleanup, or end workflow)"

### Phase 3: Task Relationships

3. **Sequential or Parallel?**: For each pair of tasks, ask:
   - "Can tasks X and Y run in parallel, or must they run sequentially?"
   - "Does task Y depend on task X's output?"

4. **Conditional Branches**: For any decision points:
   - "What condition determines the branch? (describe the true/false logic)"
   - "What happens on the 'true' path?"
   - "What happens on the 'false' path?"

5. **Fork Convergence**: For parallel tasks:
   - "After the parallel tasks complete, where should execution continue?"
   - "Should all parallel branches complete before continuing, or can some fail?"

### Phase 4: Failure Handling

6. **Failure Paths**: "Which tasks require explicit failure handling?"
   - For each: "What should the failure handler do? (cleanup resources, retry, alert, etc.)"

7. **Shared Tasks in Failure Paths**: Identify tasks that need to be used in BOTH normal and failure paths
   - If a task is used as a `Fail` handler AND in normal execution, create TWO separate tasks:
     - One for normal path: with full parameters
     - One for failure path: with same parameters minus one, plus `errorString` at the end
   - Example: `reportTask` (normal) and `reportTaskError` (failure path)

8. **End Conditions**: "When should the workflow terminate? (after all tasks, after cleanup, on first failure, etc.)"

## Output Format

Generate a complete `plan.go` file with this structure:

```go
// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package [plan-name]

import (
    "context"
    "encoding/json"

    "github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
    "github.com/spf13/cobra"
)

const [planName]Name = "[plan-name]"

// [PlanName] implements the Registry interface for the [description] plan.
type [PlanName] struct {
    // Add plan-specific state here if needed
}

var _ planners.Registry = &[PlanName]{}

// Register[PlanName]Plans registers the [plan-name] plan with the TEF registry.
func Register[PlanName]Plans(pr *planners.PlanRegistry) {
    pr.Register(&[PlanName]{})
}

// Input data structure based on user requirements
type [planName]Data struct {
    // Fields based on Phase 1 input requirements
}

func (p *[PlanName]) PrepareExecution(_ context.Context) error {
    // Initialize plan-specific resources
    return nil
}

func (p *[PlanName]) GetPlanName() string {
    return [planName]Name
}

func (p *[PlanName]) GetPlanDescription() string {
    return "[description from Phase 1]"
}

func (p *[PlanName]) GetPlanVersion() int {
    return 1
}

func (p *[PlanName]) ParsePlanInput(input string) (interface{}, error) {
    data := &[planName]Data{}
    if err := json.Unmarshal([]byte(input), data); err != nil {
        return nil, err
    }
    // Add validation and defaults here
    return data, nil
}

func (p *[PlanName]) AddStartWorkerCmdFlags(cmd *cobra.Command) {
    // Add plan-specific flags if needed
}

func (p *[PlanName]) GeneratePlan(ctx context.Context, p planners.Planner) {
    p.registerExecutors(ctx, p)

    // Create tasks based on Phase 2 discovery
    // Wire tasks based on Phase 3 relationships
    // Add failure handling from Phase 4

    // Register the plan with first and output tasks
    p.RegisterPlan(ctx, firstTask, outputTask)
}

func (p *[PlanName]) registerExecutors(ctx context.Context, p planners.Planner) {
    for _, exe := range []*planners.Executor{
        // Executor definitions based on discovered tasks
    } {
        p.RegisterExecutor(ctx, exe)
    }
}

// Executor function stubs with proper signatures
// Based on task types from Phase 2

// Example: Normal path executor
func taskExecutor(
    ctx context.Context,
    info *planners.PlanExecutionInfo,
    input *[planName]Data,
    param1 string,
    param2 int,
) (interface{}, error) {
    // TODO: Implement logic
    return nil, nil
}

// Example: Failure path executor (if task is used as Fail handler)
// Note: Same params minus one, plus errorString at the end
func taskExecutorWithError(
    ctx context.Context,
    info *planners.PlanExecutionInfo,
    input *[planName]Data,
    param1 string,
    errorMsg string, // Error string replaces the last param
) (interface{}, error) {
    logger := planners.LoggerFromContext(ctx)
    logger.Info("Task failed", "error", errorMsg)
    // TODO: Implement error handling logic
    return nil, nil
}
```

## Example Interaction

**User**: "I need a plan for setting up a test cluster"

**Skill**:
1. "What should this plan be named?" → **User**: "test-cluster-setup"
2. "What input does it need?" → **User**: "cluster name and node count"
3. "List the tasks needed:" → **User**:
   - "Create cluster configuration"
   - "Provision nodes in parallel"
   - "Initialize cluster"
   - "Run validation"
4. "Can 'provision nodes' run in parallel?" → **User**: "Yes, each node independently"
5. "What if validation fails?" → **User**: "Clean up resources and end"

**Skill Output**: Complete plan.go with:
- ExecutionTask for "create configuration"
- ForkTask for "provision nodes" with parallel branches
- ExecutionTask for "initialize cluster"
- ExecutionTask for "run validation" with Fail → cleanup
- All tasks properly wired with Next/Fail paths
- Stub executor functions with correct signatures
- Registration function: `RegisterTestClusterSetupPlans(pr)`
- Plan automatically registered in `registry.go`

## Best Practices Embedded

The generated skeleton will:
- ✓ Create a single EndTask that all paths converge to
- ✓ Ensure ForkTask branches converge to the same ForkJoinTask
- ✓ Ensure ConditionTask Then/Else branches converge to the same EndTask
- ✓ Add proper executor signatures based on task types
- ✓ Create separate tasks for normal vs failure paths when a task is used as both `Next` and `Fail` handler
- ✓ Add `errorString` parameter to executors used in failure paths
- ✓ Include comments explaining task relationships
- ✓ Set up proper error handling paths
- ✓ Include visualization hints (which task types are where)
- ✓ Add TODO comments for implementing actual executor logic

## Validation Hints

After generation, the skill will remind users to:
1. Run `./bin/tef gen-view [plan-name]` to visualize the workflow
2. Implement the executor function logic (currently stubs)
3. Build the TEF binary: `./dev build pkg/cmd/tef`
4. Test with `./bin/tef execute [plan-name] '{"input":"data"}' variant`

**Note**: The plan is automatically registered in `pkg/cmd/tef/plans/registry.go` during generation.

## Key Differences from Other Task Types

The skill understands these distinctions:

**ExecutionTask**: Normal synchronous operation
- Normal path signature: `func(ctx context.Context, info *PlanExecutionInfo, input *DataType, params...) (interface{}, error)`
- Failure path signature: `func(ctx context.Context, info *PlanExecutionInfo, input *DataType, params..., errorString string) (interface{}, error)`
- **Important**: If a task is used as a `Fail` handler, its executor must accept `errorString` as the last parameter
- **Pattern**: Create separate tasks for normal vs failure paths when needed

**ConditionTask**: Boolean decision
- Signature: `func(ctx context.Context, info *PlanExecutionInfo, input *DataType, params...) (bool, error)`

**CallbackTask**: Async with external signal
- ExecutionFn: `func(ctx context.Context, info *PlanExecutionInfo, input *DataType, params...) (string, error)` (returns stepID)
- ResultProcessorFn: `func(ctx context.Context, info *PlanExecutionInfo, input interface{}, stepID string, result string) (interface{}, error)`

**ChildPlanTask**: Execute another plan
- ChildTaskInfoFn: `func(ctx context.Context, info *PlanExecutionInfo, input *DataType, params...) (planners.ChildTaskInfo, error)`

**ForkTask**: Parallel execution
- Requires ForkJoinTask for synchronization
- All branches must converge to the same Join

## Critical Execution Path Rules

### Rule 1: Params Must Be Available in ALL Paths

**Problem**: If a task's `Params` references another task's result, that task MUST execute in ALL possible paths leading to the current task.

**Example 1 - Condition Task Problem**:
```go
// BAD: reportTask expects deleteTask result, but deleteTask doesn't run in Else path
deleteTask := p.NewExecutionTask(ctx, "delete")
reportTask := p.NewExecutionTask(ctx, "report")
reportTask.Params = []planners.Task{deleteTask}  // Expects delete result

conditionTask.Then = deleteTask → reportTask  // OK: delete runs first
conditionTask.Else = reportTask  // ERROR: delete never runs, result is nil!
```

**Example 2 - Fork Task Problem** (caught by validation):
```go
// BAD: mergeTask expects results from tasks, but taskC is NOT in fork
forkTask := p.NewForkTask(ctx, "parallel work")
taskA := p.NewExecutionTask(ctx, "task A")
taskB := p.NewExecutionTask(ctx, "task B")
taskC := p.NewExecutionTask(ctx, "task C")

forkTask.Tasks = []planners.Task{taskA, taskB}  // Only A and B in fork!

mergeTask := p.NewExecutionTask(ctx, "merge")
mergeTask.Params = []planners.Task{taskA, taskB, taskC}  // ERROR: taskC not in fork!

forkTask.Next = mergeTask

// Validation error at registration:
// "task <merge> (after fork <parallel work>) has 2 param(s) inside the fork and 1 param(s) outside.
//  This may indicate a missing task in the fork's Tasks list.
//  Example param NOT in fork: <task C>
//  If <task C> should execute in parallel with the fork, add it to fork's Tasks list."
```

**Solution for Condition Tasks**: Create separate tasks for different paths:
```go
// GOOD: Different report tasks for different paths
reportAfterDelete := p.NewExecutionTask(ctx, "report")
reportAfterDelete.Params = []planners.Task{deleteTask}

reportSkipped := p.NewExecutionTask(ctx, "report skipped")
reportSkipped.Params = []planners.Task{}  // No delete result needed

conditionTask.Then = deleteTask → reportAfterDelete  // Has delete result
conditionTask.Else = reportSkipped  // No delete result needed
```

**Solution for Fork Tasks**: Include ALL similar tasks in the fork's Tasks list:
```go
// GOOD: All similar Param tasks are in fork's Tasks list
forkTask.Tasks = []planners.Task{taskA, taskB, taskC}  // All three included!

mergeTask.Params = []planners.Task{taskA, taskB, taskC}  // Now valid!
forkTask.Next = mergeTask  // ✓ Validation passes
```

**✓ Smart Validation Implemented**: The TEF framework validates fork params at plan registration time:
- **Smart heuristic**: Only errors when 2+ similar params are in fork but 1 similar param is missing
- **Allows mixed params**: Tasks can use both pre-fork data AND fork results (different patterns)
- **Catches likely mistakes**: Detects when similar tasks (e.g., "list X", "list Y", "list Z") are incomplete
- **Example allowed**: `report.Params = [mergeResults, deleteTask]` where mergeResults is before fork
- **Example caught**: `merge.Params = [listGCE, listAWS, listAzure]` where listAzure not in fork.Tasks
- **Error at registration**: Catches errors during `gen-view` or plan registration, not at runtime

### Rule 2: Fork Params Smart Validation

**Smart Validation Rule**: The framework uses a heuristic to catch likely mistakes with fork params while allowing valid mixed patterns.

**When Validation Triggers**:
1. Task after fork has 2+ params inside the fork (shows a pattern)
2. Exactly 1 param is missing from fork
3. Missing param name is similar to fork tasks (same prefix like "list X")

**Valid Patterns (No Error)**:
```go
// Pattern 1: All params in fork (aggregate pattern)
fork.Tasks = [listGCE, listAWS, listAzure]
merge.Params = [listGCE, listAWS, listAzure]  // ✓ All in fork

// Pattern 2: No params in fork (pre-fork data)
fork.Tasks = [taskA, taskB]
nextTask.Params = [configTask, setupTask]  // ✓ Both before fork

// Pattern 3: Mixed params with different patterns (common!)
fork.Tasks = [deleteClusterBatch]
report.Params = [mergeResults, deleteClusterBatch]
// ✓ Valid: "merge" and "delete" are different patterns
// mergeResults is from before fork, deleteClusterBatch is in fork
```

**Invalid Pattern (Caught)**:
```go
// Error: Missing similar task from fork
fork.Tasks = [listGCE, listAWS]  // Missing listAzure!
merge.Params = [listGCE, listAWS, listAzure]
// ✗ Error: All three have "list" prefix, but listAzure not in fork
```

### Rule 3: Failure Path Executors Need errorString

**Critical Rule**: When a task is used as a `Fail` handler, its executor must accept an additional `errorString` parameter.

### Pattern: Separate Tasks for Normal vs Failure Paths

When a task needs to be used in BOTH a normal path (as `Next`) AND a failure path (as `Fail`), create TWO separate tasks:

```go
// Normal path task
reportTask := p.NewExecutionTask(ctx, "generate report")
reportTask.ExecutorFn = generateReport
reportTask.Params = []planners.Task{task1, task2}

// Failure path task (with error variant)
reportTaskError := p.NewExecutionTask(ctx, "generate report with error")
reportTaskError.ExecutorFn = generateReportWithError
reportTaskError.Params = []planners.Task{task1}  // One less param (replaced by errorString)

// Wire them up
someTask.Next = reportTask        // Normal path
someTask.Fail = reportTaskError   // Failure path
```

### Executor Signatures

```go
// Normal path executor: 2 params
func generateReport(
    ctx context.Context,
    info *planners.PlanExecutionInfo,
    input *InputData,
    param1 Type1,  // From Params[0]
    param2 Type2,  // From Params[1]
) (*Report, error) {
    // Normal processing
}

// Failure path executor: 1 param + errorString
func generateReportWithError(
    ctx context.Context,
    info *planners.PlanExecutionInfo,
    input *InputData,
    param1 Type1,   // From Params[0]
    errorMsg string, // Replaces param2 in failure path
) (*Report, error) {
    logger := planners.LoggerFromContext(ctx)
    logger.Info("Task failed", "error", errorMsg)
    // Error handling logic
}
```

**Key Points**:
- Params count differs by 1 between normal and failure variants
- The `errorString` parameter is ALWAYS the last parameter in failure path executors
- Both executors must be registered separately
- This pattern applies to ExecutionTask, CallbackTask, and ChildPlanTask

## Advanced Scenarios

The skill can handle:
- **Nested parallelism**: Fork tasks within fork branches
- **Complex conditionals**: Multiple decision points in sequence
- **Mixed workflows**: Combining sequential, parallel, and conditional flows
- **Multi-level child plans**: Plans calling other plans
- **Sophisticated error handling**: Different cleanup for different failure modes
- **Failure path executors**: Automatic creation of error variants for tasks used in both paths

## Important Notes

1. **Generated code is a skeleton** - executor functions are stubs that need implementation
2. **Plan is automatically registered** - the skill adds the plan to `registry.go` so it's immediately available in TEF CLI
3. **Validation happens at runtime** - the framework will panic if the graph is invalid
4. **Always visualize** - use `gen-view` to verify the structure before implementing executors
5. **Params availability** - tasks in `Params` must have their results available; validation catches issues at plan registration
   - **For forks**: Smart validation allows mixing pre-fork + in-fork params (common pattern)
   - **For forks**: Catches when similar tasks are missing from fork (e.g., forgot "list azure" when you have "list gce", "list aws")
   - **For conditions**: Create separate tasks for Then/Else branches if needed (different params)
   - **Mixed params allowed**: `report.Params = [configTask, deleteTask]` where configTask is before fork, deleteTask is in fork
6. **Failure paths matter** - every critical task should have a Fail handler
7. **Failure path executors** - tasks used as `Fail` handlers MUST have `errorString` as the last parameter
8. **Separate tasks for dual use** - if a task is used in both normal and failure paths, create two separate tasks with different executors
9. **Single EndTask** - all execution paths must converge to one EndTask
10. **Executor registration** - all executors must be registered before use

**✓ Validation Implemented**: The TEF framework now validates Params at plan registration time in `pkg/cmd/tef/planners/planner.go`:
- **Basic validation**: Checks that all Params tasks are registered
- **Fork validation**: Ensures tasks after a fork don't reference fork-internal tasks
- Catches configuration errors before execution, preventing runtime panics

## Integration with Existing Plans

When generating a plan, the skill will:
- Follow the same structure as `pkg/cmd/tef/plans/demo/plan.go`
- Use the same conventions as existing plans
- Match the style and patterns in the codebase
- Include proper copyright headers
- Follow Go naming conventions for CockroachDB
