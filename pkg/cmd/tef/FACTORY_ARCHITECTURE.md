# TEF Factory Pattern Architecture

## Overview

The Task Execution Framework (TEF) uses a **factory pattern** to enable dependency injection of orchestration backends. This allows the framework to remain orchestration-agnostic while supporting different execution engines (Temporal, local executor, etc.).

## Architecture Components

### 1. Framework Layer (Cockroach Repo)

**Location**: `pkg/cmd/tef/`

Contains orchestration-agnostic code:

```
pkg/cmd/tef/
├── planners/
│   ├── factory.go          # PlannerManagerFactory interface
│   ├── definitions.go      # Core interfaces
│   ├── planner.go          # BasePlanner (validation, task graph)
│   ├── tasks.go            # Task types
│   └── ...
├── cli/
│   ├── initializer.go      # Initialize() and InitializeStandalone()
│   └── commands.go         # CLI commands
├── plans/
│   ├── demo/               # Demo plans
│   └── pua/                # PUA plans
└── api/                    # REST API server
```

**Key Interface**:

```go
// planners/factory.go
type PlannerManagerFactory interface {
    CreateManager(ctx context.Context, r Registry) (PlannerManager, error)
}
```

**CLI Entry Point**:

```go
// cli/initializer.go
func Initialize(factory planners.PlannerManagerFactory) {
    // Create root command
    // Register plans
    // Create managers using factory
    // Execute CLI
}
```

### 2. Implementation Layer (Private Repos)

**Example**: `github.com/task-exec-framework/temporal`

Contains orchestration-specific implementation:

```
task-exec-framework/
├── temporal/
│   ├── factory.go          # Implements PlannerManagerFactory
│   ├── manager.go          # Temporal client/worker
│   ├── workflow.go         # Temporal workflows
│   └── status.go           # Status queries
└── main.go                 # Entry point
```

**Factory Implementation**:

```go
// temporal/factory.go
package temporal

import "github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"

type temporalFactory struct{}

func NewTemporalFactory() planners.PlannerManagerFactory {
    return &temporalFactory{}
}

func (f *temporalFactory) CreateManager(ctx context.Context, r planners.Registry) (planners.PlannerManager, error) {
    return NewPlannerManager(ctx, r)
}
```

**Main Entry Point**:

```go
// main.go
package main

import (
    "github.com/cockroachdb/cockroach/pkg/cmd/tef/cli"
    "github.com/task-exec-framework/temporal"
)

func main() {
    factory := temporal.NewTemporalFactory()
    cli.Initialize(factory)
}
```

## Dependency Flow

```
┌─────────────────────────────────────────┐
│ Private Repo (e.g., task-exec-framework)│
│   ├── main.go                           │
│   └── temporal/                         │
│       └── factory.go                    │
└──────────────┬──────────────────────────┘
               │ depends on (imports)
               ↓
┌─────────────────────────────────────────┐
│ Cockroach Repo                          │
│   pkg/cmd/tef/                          │
│   ├── cli/                              │
│   ├── planners/ (interfaces)            │
│   └── plans/                            │
└─────────────────────────────────────────┘
```

**Correct Direction**: Private repos depend on cockroach, not vice versa ✓

## How It Works

### 1. Framework Initialization

```go
// In private repo's main.go
factory := temporal.NewTemporalFactory()
cli.Initialize(factory)
```

### 2. CLI Creates Managers

```go
// In cockroach repo's cli/commands.go
for _, registry := range registries {
    manager, err := factory.CreateManager(ctx, registry)
    // manager is a Temporal-based PlannerManager
    // but CLI only knows it as a PlannerManager interface
}
```

### 3. Manager Execution

```go
// CLI calls interface methods
manager.StartWorker(ctx, planID)
manager.ExecutePlan(ctx, input, planID)
manager.GetExecutionStatus(ctx, planID, workflowID)
```

The Temporal implementation handles the actual execution.

## Benefits

### 1. Framework is Orchestration-Agnostic

The cockroach repo has **zero knowledge** of Temporal:
- No Temporal imports
- No Temporal types
- No Temporal dependencies

### 2. Multiple Backends Possible

Different repos can provide different backends:

```
github.com/task-exec-framework/temporal    (Temporal backend)
github.com/task-exec-framework/local       (Local executor)
github.com/task-exec-framework/kubernetes  (Kubernetes Jobs)
```

Each implements the same `PlannerManagerFactory` interface.

### 3. Clean Separation

- **Framework code**: Lives in cockroach (open, shared)
- **Backend code**: Lives in private repos (closed, specific)
- **Plan code**: Lives in cockroach (can use cockroach dependencies)

### 4. Correct Dependencies

```
Private repo (backend) → depends on → Cockroach repo (framework)
```

Plans in cockroach can use cockroach dependencies without any issues.

## Adding a New Backend

To add a new orchestration backend:

### Step 1: Create New Repo

```bash
mkdir task-exec-framework-local
cd task-exec-framework-local
go mod init github.com/your-org/task-exec-framework-local
```

### Step 2: Add Cockroach Dependency

```go
// go.mod
require github.com/cockroachdb/cockroach v0.0.0-00010101000000-000000000000
replace github.com/cockroachdb/cockroach => /path/to/cockroach
```

### Step 3: Implement Factory

```go
// local/factory.go
package local

import "github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"

type localFactory struct{}

func NewLocalFactory() planners.PlannerManagerFactory {
    return &localFactory{}
}

func (f *localFactory) CreateManager(ctx context.Context, r planners.Registry) (planners.PlannerManager, error) {
    return NewLocalManager(ctx, r)
}
```

### Step 4: Implement PlannerManager

```go
// local/manager.go
package local

import "github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"

type localManager struct {
    basePlanner *planners.BasePlanner
    // ... your implementation
}

func NewLocalManager(ctx context.Context, r planners.Registry) (planners.PlannerManager, error) {
    p, err := planners.NewBasePlanner(ctx, r)
    return &localManager{basePlanner: p}, err
}

// Implement all PlannerManager interface methods
func (m *localManager) StartWorker(ctx context.Context, planID string) error { ... }
func (m *localManager) ExecutePlan(ctx context.Context, input interface{}, planID string) (string, error) { ... }
// ... etc
```

### Step 5: Create Main Entry Point

```go
// main.go
package main

import (
    "github.com/cockroachdb/cockroach/pkg/cmd/tef/cli"
    "github.com/your-org/task-exec-framework-local/local"
)

func main() {
    factory := local.NewLocalFactory()
    cli.Initialize(factory)
}
```

### Step 6: Build and Run

```bash
go build -o bin/tef .
./bin/tef --help
```

Your backend is now integrated!

## Testing

The factory pattern also enables easy testing:

```go
// In tests
type mockFactory struct{}

func (f *mockFactory) CreateManager(ctx context.Context, r planners.Registry) (planners.PlannerManager, error) {
    return &mockManager{}, nil
}

// Test CLI with mock factory
cli.Initialize(&mockFactory{})
```

## Summary

The factory pattern provides:
- ✅ Clean separation between framework and backend
- ✅ Multiple backend support
- ✅ Correct dependency direction
- ✅ Testability
- ✅ Framework remains orchestration-agnostic

The cockroach repo defines "what" (interfaces), while backend repos define "how" (implementations).
