# Demo Plan - Comprehensive Task Execution Framework Example

This demo plan showcases all task types available in the Task Execution Framework (TEF) with realistic delays and randomized outcomes.

## Task Types Demonstrated

### 1. **CallbackTask** (Asynchronous callback-based tasks)
- `initialize environment` - Demonstrates async workflow with approval
  - **With Slack configured**: Sends approval request via Slack, waits for manual approval
  - **Without Slack**: Auto-approves after 1 minute to make demo self-contained
  - This shows how external systems can integrate with TEF workflows

### 2. **ExecutionTask** (Standard execution tasks)
- `setup network` - 1.5min delay (Fork branch 1)
- `configure firewall` - 1min delay (Fork branch 1)
- `setup storage` - 2min delay (Fork branch 2)
- `format disks` - 1min delay (Fork branch 2)
- `setup cluster` - 3min delay
- `configure cluster settings` - 2min delay
- `validate configuration` - 1.5min delay (randomly fails ~50%)
- `import TPCC workload` - 5min delay
- `run workload` - 3min delay
- `collect metrics` - 1min delay
- `retry data import` - 2min delay (randomly fails ~30%)
- `cleanup resources` - 1min delay

### 3. **ForkTask** (Parallel execution)
- `parallel infrastructure setup` - Executes two branches in parallel:
  - **Branch 1**: Network setup → Firewall configuration
  - **Branch 2**: Storage setup → Disk formatting
- Both branches must complete before proceeding to the next task

### 4. **ChildPlanTask** (Nested plan execution)
- `deploy CRDB cluster` - Executes the `roachprod-mini` child plan
- The child plan has its own tasks:
  - Create roachprod cluster (1min)
  - Start cockroach (1.5min)

### 5. **ConditionTask** (Conditional branching)
- `check cluster readiness` - Random true/false outcome
  - **Then**: Continue with cluster configuration
  - **Else**: Jump to retry import path
- `verify data import` - Random true/false outcome
  - **Then**: Run workload
  - **Else**: Jump to cleanup

### 6. **SleepTask** (Time-based delays)
- `wait for infrastructure` - 1min sleep
- `wait before validation` - 30sec sleep
- `wait for metrics` - 45sec sleep

### 7. **EndTask** (Termination)
- `end` - Main plan termination
- `fork end` - Fork branch termination

## Execution Flow

```
initialize environment (CallbackTask - auto-approves after 1min if Slack not configured)
  ↓
parallel infrastructure setup (FORK)
  ├─ setup network (1.5min) → configure firewall (1min) → fork end
  └─ setup storage (2min) → format disks (1min) → fork end
  ↓
wait for infrastructure (1min sleep)
  ↓
deploy CRDB cluster (CHILD PLAN: 2.5min total)
  ├─ create roachprod cluster (1min)
  └─ start cockroach (1.5min)
  ↓
setup cluster (3min)
  ↓
check cluster readiness (IF - random)
  ├─ THEN: configure cluster settings (2min)
  │         ↓
  │       wait before validation (30sec sleep)
  │         ↓
  │       validate configuration (1.5min, may fail ~50%)
  │         ↓ (on success)
  │       import TPCC workload (5min)
  │         ↓
  │       verify data import (IF - random)
  │         ├─ THEN: run workload (3min)
  │         │         ↓
  │         │       wait for metrics (45sec sleep)
  │         │         ↓
  │         │       collect metrics (1min)
  │         │         ↓
  │         │       end
  │         └─ ELSE: cleanup resources (1min) → end
  └─ ELSE: retry data import (2min, may fail ~30%) → end

(validate configuration failure path) → cleanup resources (1min) → end
```

## Random Behaviors

1. **check cluster readiness**: 50% chance of true/false
2. **validate configuration**: 50% chance of failure
3. **verify data import**: 50% chance of true/false
4. **retry data import**: 30% chance of failure

## Total Execution Time

Depending on the random outcomes, the plan will take between **~8-20 minutes** to complete:

- **Shortest path** (~8min): Init → Fork (parallel, ~2min longest) → Sleep → Child Plan → Cluster not ready → Retry import → End
- **Longest path** (~20min): Init → Fork → Sleep → Child Plan → Setup → Ready → Configure → Validate (success) → Import → Verify (success) → Run workload → Sleep → Metrics → End

## Usage

To execute this demo plan:

```bash
# Register the plan (implementation-specific)
# Then execute with sample input:
{"name": "my-demo-cluster"}
```

## Key Features Demonstrated

1. **Callback-based Workflows**: Shows async task execution with external approval (Slack integration)
   - Auto-approval mechanism makes demo self-contained when Slack is not configured
   - Demonstrates integration with external systems via ResumeTask API
2. **Parallel Execution**: Fork task shows how to run infrastructure setup in parallel
3. **Plan Composition**: Child plan demonstrates nested workflow execution
4. **Conditional Logic**: Multiple if tasks show branching based on conditions
5. **Error Handling**: Tasks have both Next and Fail paths
6. **Task Dependencies**: Tasks can depend on outputs from previous tasks (e.g., provisionCluster uses setupCluster output)
7. **Delays**: Sleep tasks demonstrate workflow pacing
8. **Random Failures**: Shows realistic failure scenarios and recovery paths

## Slack Integration (Optional)

The demo plan supports optional Slack integration for approval workflows:

**To enable Slack approvals:**
```bash
./bin/tef start-worker demo \
  --slack-bot-token "xoxb-..." \
  --slack-channel "#approvals" \
  --slack-app-token "xapp-..." \
  --api-base-url "http://localhost:25780"
```

**Without Slack configured:**
The callback task automatically approves after 1 minute, making the demo fully self-contained. You'll see this log message:
```
[DEMO] Slack not enabled, will auto-approve after 1 minute to demonstrate callback workflow
```

## Testing

### Unit Tests

The demo plan includes comprehensive unit tests in `plan_test.go` that demonstrate how to test plan implementations using gomock mocks.

**Test Coverage:**
- `TestDemo_GetPlanName` - Verifies plan name
- `TestDemo_GetPlanDescription` - Verifies plan description
- `TestDemo_ParsePlanInput` - Tests JSON input parsing (valid, empty, invalid, malformed)
- `TestDemo_GeneratePlan` - Comprehensive test that verifies:
  - All 19 executors are registered with correct names, functions, and descriptions
  - All task types are created (ExecutionTask, ForkTask, ConditionTask, EndTask)
  - RegisterPlan is called with correct parameters
- `TestDemo_ImplementsRegistry` - Compile-time interface verification
- `TestDemoData_JSONMarshaling` - JSON marshaling/unmarshaling

**Running the tests:**
```bash
make test
```

### Mock Interfaces

GoMock mocks are available for all planner interfaces to facilitate testing:

**Location:** Auto-generated in the same package with `_test.go` suffix

**Available Mocks:**
- `MockPlanner` - Mock for `planners.Planner` interface
- `MockRegistry` - Mock for `planners.Registry` interface
- `MockPlannerManager` - Mock for `planners.PlannerManager` interface

**Usage Example:**
```go
import (
    "testing"
github.com/cockroachdb/cockroach/pkg/cmd/tef/planners
    "github.com/golang/mock/gomock"
)

func TestMyPlan(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPlanner := mockplanners.NewMockPlanner(ctrl)

    // Set expectations
    mockPlanner.EXPECT().
        RegisterExecutor(gomock.Any(), gomock.Any()).
        Times(1)

    // Test your plan
    myPlan := NewMyPlan()
    myPlan.GeneratePlan(context.Background(), mockPlanner)
}
```

**Mock Generation:**
Mocks are automatically generated using `go:generate` directives. To regenerate mocks:
```bash
make generate
```

Mock files are generated in the same package with the `mock_*_generated_test.go` naming pattern.

## Implementation Notes

- All time delays use `time.Sleep()` to simulate real operations
- Random outcomes use `rand.Intn()` for demonstration purposes
- Each task prints its progress with the `[DEMO]` or `[CHILD PLAN]` prefix
- The plan is designed to complete within 12 minutes as requested
- Comprehensive unit tests demonstrate best practices for testing plans with mocks
