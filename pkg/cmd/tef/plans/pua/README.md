# PUA (Performance Under Adversity) Test Plan

This directory contains the implementation of the PUA test plan, which is a comprehensive multi-phase cluster testing framework based on the YAML configuration at `/Users/bhaskar.bora/go/src/github.com/cockroachdb/cockroach/pkg/cmd/drtprod/configs/drt_pua_9.yaml`.

> ⚠️ **Note**: This is currently a **skeleton implementation**. The plan structure, task orchestration, and parallel execution flows are complete, but the individual executor functions contain TODO stubs and need actual implementation.

## Plan Structure

The PUA plan orchestrates multiple child plans in a sequential workflow. Each phase depends on the successful completion of previous phases.

### Main Plan (plan.go)

The main PUA plan coordinates all phases:

```
plan.go (Main Orchestrator)
├── puaData struct - Configuration data for all phases
└── RegisterPUAPlans() - Registers all child plans
```

### Sequential Phases

1. **Cluster Setup** (`cluster_setup.go`)
   - Creates a 9-node GCE cluster
   - Stages CockroachDB binary
   - Configures monitoring and disk stallers
   - Starts cluster with WAL failover
   - **Parallel execution**: Cluster settings configuration (5 SQL settings in parallel using fork)
   - Creates load balancer

2. **Workload Setup** (`workload_setup.go`)
   - Creates workload cluster (1 node)
   - Stages cockroach and workload binaries
   - **Parallel execution**: Uploads roachprod, drtprod, and roachtest binaries (using fork)
   - Configures Datadog monitoring

3. **Setup Certs & SSH Keys** (`certs_setup.go`)
   - Retrieves and distributes certificates
   - Sets proper permissions
   - **Parallel execution**: Generates TPCC init, run, and PUA operations scripts (using fork)

4. **Data Import** (`data_import.go`)
   - Imports TPCC data (5000 warehouses)
   - Single task with 1-hour wait

5. **Phase-1: Baseline Performance** (`baseline.go`)
   - Runs baseline TPCC workload
   - Establishes performance metrics
   - 1-hour execution

6. **Phase-2: Internal Operational Stress** (`ops_stress.go`)
   - Sequential stress operations:
     - Creates backup schedule (30 min wait)
     - Creates changefeed (10 min wait)
     - Creates index (11.67 min wait)
     - Performs rolling upgrade (5 min wait)

7. **Phase-3: Disk Stalls** (`disk_stalls.go`)
   - Injects disk stall scenarios
   - 20-minute observation period

8. **Phase-4: Network Failures** (`network_failures.go`)
   - Sequential network partition tests:
     - Partial network partition (25 min wait)
     - Full network partition (25 min wait)

9. **Phase-5: Node Restarts** (`node_restarts.go`)
   - Sequential ungraceful node shutdowns and restarts:
     - Stop node 2 → Restart (10 min wait)
     - Stop node 6 → Restart (25 min wait)
     - Stop node 7 → Restart (25 min wait)

10. **Phase-6: Zone Outages** (`zone_outages.go`)
    - Simulates zone-level outage:
      - Stops nodes 7-9 (5 min wait)
      - Restarts nodes 7-9 (55 min wait)

11. **Cleanup** (`cleanup.go`)
    - **Parallel execution**: Destroys both main and workload clusters (using fork)

## Parallel Execution (Fork Tasks)

The following phases use fork tasks to run operations in parallel:

1. **Cluster Setup**:
   - 5 cluster settings configured simultaneously

2. **Workload Setup**:
   - 3 binaries (roachprod, drtprod, roachtest) uploaded simultaneously

3. **Certs Setup**:
   - 3 scripts (TPCC init, TPCC run, PUA ops) generated simultaneously

4. **Cleanup**:
   - Main and workload clusters destroyed simultaneously

## Key Features

### Dependency Management
- Each phase depends on the previous phase's successful completion
- On failure, execution jumps to cleanup
- Linear progression ensures proper test setup

### Error Handling
- All phases have proper failure paths
- Cleanup is executed on any failure
- End task consolidates all completion paths

### Configuration
- Centralized `puaData` struct contains all configuration
- Configuration passed to child plans via `ChildTaskInfo`
- Environment-based configuration (cluster names, sizes, timeouts, etc.)

## Implementation Status

All executor functions are currently TODOs marked with implementation notes that reference the original YAML commands. The structure is complete and ready for implementation.

### Next Steps for Implementation

1. Implement roachprod command execution in each executor function
2. Add proper wait/sleep mechanisms for `wait_after` directives
3. Integrate with actual roachprod/roachtest binaries
4. Add error handling and retry logic where appropriate
5. Implement result validation and metrics collection

## Files

- `plan.go` - Main orchestrator and data structure
- `cluster_setup.go` - Phase 0: Cluster creation and configuration
- `workload_setup.go` - Phase 1: Workload cluster setup
- `certs_setup.go` - Phase 2: Certificate and SSH key setup
- `data_import.go` - Phase 3: TPCC data import
- `baseline.go` - Phase 4: Baseline performance testing
- `ops_stress.go` - Phase 5: Operational stress testing
- `disk_stalls.go` - Phase 6: Disk stall injection
- `network_failures.go` - Phase 7: Network partition testing
- `node_restarts.go` - Phase 8: Node restart testing
- `zone_outages.go` - Phase 9: Zone outage simulation
- `cleanup.go` - Cleanup and resource destruction
