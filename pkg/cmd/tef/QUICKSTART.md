# TEF Quick Start

Get started with TEF using standalone mode - no external dependencies required!

## Prerequisites

- Go 1.23+
- Graphviz (for workflow visualization): `brew install graphviz` on macOS

## Build TEF Binary

TEF provides a lightweight `tef` binary for development and testing:

```bash
# From cockroach repo root
./dev build tef

# Verify
./bin/tef --help
```

## Visualize a Plan

The fastest way to understand TEF is to visualize an existing plan:

```bash
# Generate workflow diagram for the demo plan
./bin/tef gen-view demo

# This creates:
# - demo.dot (graph definition)
# - demo.png (visual diagram)

# View the diagram
open demo/demo.png  # macOS
xdg-open demo/demo.png  # Linux
```

### Visualization Options

```bash
# Include failure paths (error handling)
./bin/tef gen-view demo --with-failure-paths

# Custom output directory
./bin/tef gen-view demo --output-dir ./diagrams

# Visualize different plans
./bin/tef gen-view pua
./bin/tef gen-view roachprod
```

The visualization shows:
- **Shapes**: Different task types (boxes, diamonds, circles)
- **Colors**: Execution paths (green for "then", orange for "else", red for failures)
- **Arrows**: Task flow and dependencies
- **Parallel branches**: Fork tasks with synchronization points

## Run a Plan (Standalone Mode)

Execute plans without any external dependencies using standalone mode:

```bash
# Execute the demo plan
./bin/tef standalone demo \
  --input '{"name":"quickstart-test"}' \
  --plan-variant dev \
  --port 8081
```

This:
1. Starts an HTTP server on port 8081
2. Executes the plan in-memory
3. Keeps the server running for status queries

### Monitor Execution

In another terminal, query the execution status:

```bash
# List all plans
curl http://localhost:8081/v1/plans

# Get plan executions
curl http://localhost:8081/v1/plans/tef_plan_demo.dev/executions

# Check execution status (use workflow_id from executions list)
curl http://localhost:8081/v1/plans/tef_plan_demo.dev/executions/<workflow-id>/status
```

### Resume Callback Tasks

If your plan uses callback tasks (async operations), resume them via API:

```bash
curl -X POST http://localhost:8081/v1/resume \
  -H "Content-Type: application/json" \
  -d '{
    "plan_id": "tef_plan_demo.dev",
    "workflow_id": "<workflow-id>",
    "step_id": "<step-id>",
    "result": "operation-complete"
  }'
```

## Create Your First Plan

### 1. Create Plan Structure

```bash
cd pkg/cmd/tef/plans
mkdir myplan
cd myplan
```

### 2. Implement the Plan

Create `plan.go`:

```go
package myplan

import (
    "context"
    "encoding/json"
    "github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
)

const myPlanName = "myplan"

type MyPlan struct{}

func NewMyPlan() *MyPlan {
    return &MyPlan{}
}

var _ planners.Registry = &MyPlan{}

func (m *MyPlan) GetPlanName() string {
    return myPlanName
}

func (m *MyPlan) GetPlanDescription() string {
    return "My first TEF plan"
}

type myPlanData struct {
    Name string `json:"name"`
}

func (m *MyPlan) ParsePlanInput(input string) (interface{}, error) {
    data := &myPlanData{}
    if err := json.Unmarshal([]byte(input), data); err != nil {
        return nil, err
    }
    return data, nil
}

func (m *MyPlan) GeneratePlan(ctx context.Context, p planners.Planner) {
    // Register executors
    p.RegisterExecutor(ctx, &planners.Executor{
        Name: "greet",
        Func: greet,
    })

    // Create tasks
    greetTask := p.NewExecutionTask(ctx, "greet user")
    greetTask.ExecutorFn = greet

    endTask := p.NewEndTask(ctx, "end")

    // Wire tasks
    greetTask.Next = endTask
    greetTask.Fail = endTask

    // Register plan
    p.RegisterPlan(ctx, greetTask, greetTask)
}

func greet(ctx context.Context, info *planners.PlanExecutionInfo, input *myPlanData) (string, error) {
    logger := planners.LoggerFromContext(ctx)
    logger.Info("Greeting user", "name", input.Name)
    return "Hello, " + input.Name + "!", nil
}
```

### 3. Register the Plan

Edit `pkg/cmd/tef/plans/registry.go`:

```go
func GetRegistrations() []planners.Registry {
    return []planners.Registry{
        roachprod.NewRoachprod(),
        demo.NewDemo(),
        pua.NewPUA(),
        myplan.NewMyPlan(),  // Add your plan here
    }
}
```

### 4. Rebuild and Test

```bash
# Rebuild binary
./dev build tef

# Visualize your plan
./bin/tef gen-view myplan
open myplan.png

# Execute your plan
./bin/tef standalone myplan \
  --input '{"name":"World"}' \
  --plan-variant dev \
  --port 8082
```

## Understanding Plan Execution

### Execution Flow

```
User executes plan
    ↓
CLI parses input using ParsePlanInput()
    ↓
GeneratePlan() builds task graph
    ↓
Framework validates task graph
    ↓
Standalone backend executes tasks
    ↓
Each task runs its executor function
    ↓
Results flow to next tasks via Params
    ↓
Workflow completes at EndTask
```

### Task Graph Validation

TEF automatically validates:
- ✅ All execution paths converge to a common EndTask
- ✅ No cyclic dependencies
- ✅ All executor functions are registered
- ✅ Conditional tasks have both Then and Else branches
- ✅ Fork branches converge to their join point

If validation fails, TEF panics with a descriptive error message.

## Next Steps

### Develop More Complex Plans

See **[plans/README.md](plans/README.md)** for comprehensive guide covering:
- All task types (ExecutionTask, ForkTask, ConditionTask, CallbackTask, ChildPlanTask)
- Executor function signatures
- Parallel execution with fork/join
- Conditional branching
- Error handling
- Best practices

### Example Plans

Explore existing plans for patterns:

```bash
# View demo plan (comprehensive example)
./bin/tef gen-view demo --with-failure-paths

# View PUA plan (performance testing)
./bin/tef gen-view pua

# View roachprod plan (simple workflow)
./bin/tef gen-view roachprod
```

Study the implementation:
- `pkg/cmd/tef/plans/demo/plan.go` - Shows all task types
- `pkg/cmd/tef/plans/pua/plan.go` - Real-world testing workflow
- `pkg/cmd/tef/plans/roachprod/plan.go` - Simple cluster provisioning

### Standalone Mode Features

Learn more about standalone execution:
- **[planners/inmemory/README.md](planners/inmemory/README.md)** - In-memory backend documentation
- Async execution with goroutines
- Callback task resumption
- Child workflow execution
- HTTP API for monitoring

### Production Deployment

For production workflows with durability and distributed execution, see the Temporal backend implementation in the `task-exec-framework` repository (separate from this framework).

## Common Operations

### Test Plan Changes

```bash
# Rebuild after code changes
./dev build tef

# Visualize to verify structure
./bin/tef gen-view myplan --with-failure-paths

# Execute to test logic
./bin/tef standalone myplan --input '{"data":"test"}' --plan-variant dev
```

### Debug Plan Issues

```bash
# Check validation errors
./dev build tef  # Panics with detailed error if validation fails

# Visualize to spot structural issues
./bin/tef gen-view myplan --with-failure-paths

# Check for:
# - Missing EndTask connections
# - Cyclic dependencies
# - Unregistered executors
# - Missing Then/Else branches
```

### Query Execution Status

```bash
# List all plans
curl http://localhost:8081/v1/plans | jq

# List executions for a plan
curl http://localhost:8081/v1/plans/tef_plan_myplan.dev/executions | jq

# Get detailed status
curl http://localhost:8081/v1/plans/tef_plan_myplan.dev/executions/<workflow-id>/status | jq
```

## Troubleshooting

### Validation Errors

If you see panic messages during build:
- Read the error message carefully - it tells you exactly what's wrong
- Common issues:
  - Missing `RegisterExecutor()` call
  - Task has no Next task and doesn't end with EndTask
  - Conditional task missing Then or Else branch
  - Fork branches don't converge to same EndTask

### Visualization Fails

If `gen-view` fails:
- Install Graphviz: `brew install graphviz` (macOS) or `apt install graphviz` (Linux)
- Check plan name is correct
- Ensure plan is registered in `plans/registry.go`

### Standalone Execution Issues

If execution fails:
- Check input JSON matches your `ParsePlanInput()` structure
- Verify plan-variant matches registered variant
- Check executor functions return correct types
- Review logs for executor errors

## Resources

- **[README.md](README.md)** - Framework overview and architecture
- **[plans/README.md](plans/README.md)** - Comprehensive plan development guide
- **[planners/inmemory/README.md](planners/inmemory/README.md)** - Standalone mode details
- **[CLI.md](CLI.md)** - Complete CLI reference
- **[API.md](API.md)** - REST API documentation
- **[FACTORY_ARCHITECTURE.md](FACTORY_ARCHITECTURE.md)** - Framework architecture
