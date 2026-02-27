# Task Execution Framework (TEF)

TEF is a workflow orchestration framework for building multi-step task execution systems with support for sequential execution, parallel execution, conditional branching, failure handling, and automatic validation.

## Key Concepts

- **Plan**: A workflow definition implementing the `Registry` interface. Plans describe complex multi-step processes like cluster provisioning, performance testing, or deployment automation.
- **Task**: Individual workflow steps with different types (Execution, Fork, Condition, Callback, ChildPlan, End)
- **Executor**: Functions that perform the actual work in each task
- **Planner**: Interface for building and validating task graphs
- **Task Graph**: The workflow structure showing how tasks connect and flow

### Example Workflow

```
Start → Setup Cluster → Check Ready? ─┬─ Yes → Deploy App → End
                                      └─ No  → Retry Setup → End
```

## Creating Plans

Plans are defined in Go code by implementing the `Registry` interface. Here's a minimal example:

```go
package myplan

import (
    "context"
    "github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
)

type MyPlan struct{}

func (p *MyPlan) GetPlanName() string { return "myplan" }
func (p *MyPlan) GetPlanDescription() string { return "My custom plan" }

func (p *MyPlan) GeneratePlan(ctx context.Context, planner planners.Planner) {
    // Register executors
    exec := &planners.Executor{
        Name: "setup",
        Func: setupCluster,
    }
    planner.RegisterExecutor(ctx, exec)

    // Create task graph
    task1 := planner.NewExecutionTask(ctx, "setup cluster")
    task1.ExecutorFn = setupCluster

    endTask := planner.NewEndTask(ctx, "end")
    task1.Next = endTask

    // Register the plan
    planner.RegisterPlan(ctx, task1, nil)
}

func setupCluster(ctx context.Context, info *planners.PlanExecutionInfo, input interface{}) (interface{}, error) {
    // Your implementation
    return "cluster-ready", nil
}
```

### Task Types

TEF provides seven task types:

1. **ExecutionTask** - Runs an executor function
2. **ForkTask** - Parallel execution of multiple branches
3. **ForkJoinTask** - Synchronization point for parallel branches
4. **ConditionTask** - Conditional branching (if/else)
5. **CallbackTask** - Async execution with external completion signal
6. **ChildPlanTask** - Execute another plan as a sub-workflow
7. **EndTask** - Workflow termination point

For detailed plan development guide, examples, and best practices, see **[plans/README.md](plans/README.md)**.

## Visualizing Workflows

TEF can generate visual representations of your workflow:

```bash
./dev build tef
./bin/tef gen-view demo
```

This creates:
- `demo.dot` - DOT format graph definition
- `demo.png` - Visual diagram showing task flow, parallel execution, and branches

**Requirements:** Graphviz must be installed (`brew install graphviz` on macOS)

The visualization helps verify:
- All execution paths converge to a common EndTask
- Fork branches properly converge to their join point
- Conditional branches lead to expected tasks
- No unexpected cycles exist

Use `--with-failure-paths` to include error handling paths in the visualization.

## Framework Flexibility

TEF is designed to be **framework-agnostic**. The core framework (`planners/`) defines interfaces and validation logic without depending on any specific orchestration backend. This allows you to:

- **Develop plans** using CockroachDB dependencies
- **Execute plans** using different orchestration backends (Temporal, in-memory, Kubernetes Jobs, etc.)
- **Switch backends** without changing plan code

### Separation of Concerns

```
┌─────────────────────────────────────┐
│ Plans (this repository)             │
│   - Workflow definitions            │
│   - Executor implementations        │
│   - Use cockroach dependencies      │
└─────────────────┬───────────────────┘
                  │ imports framework
                  ↓
┌─────────────────────────────────────┐
│ Framework (this repository)         │
│   - Interfaces (Planner, Registry)  │
│   - Validation logic                │
│   - Task definitions                │
│   - CLI & API implementation        │
└─────────────────┬───────────────────┘
                  │ imports
                  ↓
┌─────────────────────────────────────┐
│ Backend (separate repository)       │
│   - Orchestration implementation    │
│   - Factory pattern                 │
│   - Runtime execution               │
└─────────────────────────────────────┘
```

## Factory Pattern and Backends

TEF uses a **factory pattern** to inject orchestration backends at runtime:

```go
// Framework defines the interface
type PlannerManagerFactory interface {
    CreateManager(ctx context.Context, registry Registry) (PlannerManager, error)
}

// Backends implement this interface
type TemporalFactory struct { /* ... */ }
type InMemoryFactory struct { /* ... */ }
```

This pattern enables:
- Multiple backend implementations
- Runtime backend selection
- Testing with lightweight backends
- Production deployment with robust orchestration

### Available Backends

**1. Temporal Backend (Production)**

The primary production backend uses Temporal for distributed, durable workflow execution:

- Repository: `github.com/task-exec-framework`
- Features: Distributed execution, durability, worker pools, retry policies
- Use case: Production deployments, long-running workflows, distributed systems

**2. In-Memory Backend (Development/Testing)**

A lightweight in-memory backend for development and testing:

- Location: `pkg/cmd/tef/planners/inmemory/`
- Features: Synchronous execution, no external dependencies, callback support
- Use case: Testing, development, workflow visualization

See **[FACTORY_ARCHITECTURE.md](FACTORY_ARCHITECTURE.md)** for detailed architecture documentation.

## Temporal Backend (task-exec-framework)

The Temporal backend is the primary production implementation:

```bash
# Clone the backend repo
git clone https://github.com/task-exec-framework

# Build
cd task-exec-framework
go build -o bin/tef .

# Run
./bin/tef --help
```

### CLI Usage (Temporal Backend)

```bash
# Start a worker
./tef start-worker myplan --temporal-address localhost:7233

# Execute a plan
./tef execute myplan '{"input":"data"}' dev

# Check status
./tef get-status tef_plan_myplan.dev <workflow-id>

# List executions
./tef list-executions tef_plan_myplan.dev
```

The Temporal backend imports framework code from this repository and provides:
- Distributed workflow execution
- Persistent workflow state
- Automatic retries and error handling
- Worker pool management
- Production-grade durability

## Standalone Mode (tef-light)

For development, testing, and visualization without external dependencies, use the standalone mode:

```bash
# Build tef-light binary
./dev build tef-light

# Execute with in-memory backend (comma-separated plans)
./bin/tef-light standalone demo \
  --with-plans demo,cluster-setup \
  --input '{"cluster":"test"}' \
  --plan-variant dev \
  --port 8081

# Execute with in-memory backend (regex pattern)
./bin/tef-light standalone demo \
  --with-plans-regex "pua.*" \
  --input '{"cluster":"test"}' \
  --plan-variant dev \
  --port 8081
```

This mode:
- Runs workflows entirely in-memory within a single process
- Executes tasks asynchronously in background goroutines
- Provides HTTP API for status queries and resume operations
- Requires no external dependencies (no Temporal, no database)
- Ideal for testing, development, and workflow visualization

### Standalone Features

- **Synchronous Execution**: Tasks execute in-process using goroutines
- **HTTP API**: Query status and resume callback tasks via REST API
- **Callback Support**: Callback tasks block until resumed via HTTP endpoint
- **Child Workflows**: Executes child plans in the same process
- **No Persistence**: State is lost on process restart

### Use Cases for Standalone Mode

- **Development**: Quickly test plan changes without orchestration setup
- **Visualization**: Generate workflow graphs and verify structure
- **Testing**: Validate plan logic in CI/CD without external dependencies
- **Demos**: Show workflow execution without infrastructure requirements

See **[planners/inmemory/README.md](planners/inmemory/README.md)** for detailed documentation.

## CLI Commands

TEF provides several CLI commands:

```bash
# Visualize workflow structure
./bin/tef gen-view <planname> [--with-failure-paths]

# Standalone execution (in-memory backend)
./bin/tef standalone <planname> --input '<json>' --plan-variant <variant>

# Temporal backend commands (from task-exec-framework repo)
./tef execute <planname> '<json-input>' <variant>
./tef start-worker <planname> --plan-variant <variant>
./tef get-status <plan-id> <workflow-id>
./tef list-executions <plan-id>
```

See **[CLI.md](CLI.md)** for complete command reference.

## REST API

The framework includes a REST API server for programmatic access:

```bash
# Start API server (from task-exec-framework binary)
./tef serve --host localhost --port 8081

# API endpoints
POST   /v1/plans/{planID}/execute
GET    /v1/plans/{planID}/executions
GET    /v1/plans/{planID}/executions/{workflowID}/status
POST   /v1/plans/{planID}/executions/{workflowID}/resume
GET    /v1/plans
```

See **[API.md](API.md)** for full API documentation.

## Development

### Working on Plans

Plans are implemented in this repository:

```bash
# In this repo (cockroach)
cd pkg/cmd/tef/plans

# Create new plan
mkdir myplan
vim myplan/plan.go

# Register in plans/registry.go
vim registry.go
```

Plans can use any cockroach dependencies and are automatically available to all backends.

### Working on Framework

Framework code is in `pkg/cmd/tef/planners/`:

```bash
# Make framework changes
cd pkg/cmd/tef/planners
vim definitions.go

# Test changes
./dev test pkg/cmd/tef/planners
```

Backend repos automatically pick up framework changes via `go.mod`.

### Testing

```bash
# Test framework code
./dev test pkg/cmd/tef/planners
./dev test pkg/cmd/tef/cli

# Test plans
./dev test pkg/cmd/tef/plans/demo

# Test in-memory backend
./dev test pkg/cmd/tef/planners/inmemory
```

## Documentation

- **[plans/README.md](plans/README.md)** - Comprehensive plan development guide
- **[planners/inmemory/README.md](planners/inmemory/README.md)** - Standalone mode documentation
- **[FACTORY_ARCHITECTURE.md](FACTORY_ARCHITECTURE.md)** - Architecture and factory pattern
- **[API.md](API.md)** - REST API documentation
- **[CLI.md](CLI.md)** - CLI command reference
- **[QUICKSTART.md](QUICKSTART.md)** - Kubernetes deployment guide
- **[architecture.md](architecture.md)** - Detailed design docs

## Status

✅ Framework-agnostic design
✅ Factory pattern for backend injection
✅ Multiple task types with automatic validation
✅ CLI and API implementation
✅ Demo and production plans
✅ Temporal backend (production-ready)
✅ In-memory backend (development/testing)
✅ Manager registry for child workflows
✅ Workflow visualization

## License

See LICENSE file in repository root.
