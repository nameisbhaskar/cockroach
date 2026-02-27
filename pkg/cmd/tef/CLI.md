# TEF CLI Reference

This document provides a comprehensive reference for the TEF command-line interface.

**Current Status**: CLI is fully implemented with automatic command generation for all registered plans.

> **Note**: The `tef` binary is built from the [task-exec-framework](https://github.com/task-exec-framework) repository, not this one. See [QUICKSTART.md](QUICKSTART.md) for build instructions.

## Overview

TEF automatically generates CLI commands for each registered plan. The CLI provides commands for starting workers, executing plans, checking status, and managing workflows.

## Understanding Plan IDs

Each worker and execution is associated with a **plan ID**, which is composed of:

```
plan_id = <plan_name>-<plan_variant>
```

The plan ID serves several purposes:

* **Run different versions**: Different plan IDs allow you to run multiple versions of the same plan simultaneously
* **Separate environments**: Use different variants for `dev`, `staging`, `prod`, etc.
* **Isolate experiments**: Test workflow changes without affecting production executions
* **Multiple instances**: Run the same plan with different configurations in parallel

**Plan Variant Behavior:**
* For **workers**: If no variant is provided via `--plan-variant`, a UUID is auto-generated
* For **executions**: The plan variant is required as the second argument to `execute <planname>`

## Global Flags

These flags are available for all commands:

```bash
--help                      Show help for the command
--temporal-address string   Temporal server address (default: "localhost:7233")
--temporal-namespace string Temporal namespace (default: "default")
```

## Core Commands

### start-worker

Start a worker to process plan executions.

**Syntax:**
```bash
./bin/tef start-worker <plan-name> [flags]
```

**Arguments:**
* `plan-name`: Name of the plan to start a worker for

**Flags:**
* `--plan-variant string`: Plan variant suffix for the plan ID (default: auto-generated UUID)
* Additional plan-specific flags as defined by `AddStartWorkerCmdFlags()`

**Examples:**

```bash
# Auto-generated plan ID (e.g., demo-a1b2c3d4-e5f6-7890-abcd-ef1234567890)
./bin/tef start-worker demo

# Explicit plan ID for development environment (demo-dev)
./bin/tef start-worker demo --plan-variant dev

# Explicit plan ID for production environment (demo-prod)
./bin/tef start-worker demo --plan-variant prod

# With custom Temporal settings
./bin/tef start-worker demo \
  --plan-variant dev \
  --temporal-address localhost:7233 \
  --temporal-namespace my-namespace
```

**Behavior:**
* Creates a worker that listens for execution requests on the specified plan ID
* Registers all executors with the orchestration engine
* Runs until interrupted (Ctrl+C)
* Multiple workers can run for different plan IDs simultaneously

### execute

Execute a plan with JSON input.

**Syntax:**
```bash
./bin/tef execute <plan-name> '<json-input>' <plan-variant> [flags]
```

**Arguments:**
1. `plan-name`: Name of the plan to execute
2. `json-input`: Plan input data as a JSON string
3. `plan-variant`: The variant that forms the plan ID (must match a running worker)

**Examples:**

```bash
# Execute on the 'dev' plan instance
./bin/tef execute demo \
  '{"message": "Hello", "count": 5}' \
  dev

# Execute on the 'prod' plan instance
./bin/tef execute demo \
  '{"message": "Hello production", "count": 10}' \
  prod

# Execute with complex input
./bin/tef execute myplan \
  '{"cluster_name": "test-cluster", "node_count": 5, "region": "us-east"}' \
  dev
```

**Output:**
```
Workflow started with ID: demo-20250126-123456-abc123
Plan ID: demo-dev
```

**Common Workflows:**

Development workflow:
```bash
# Terminal 1: Start dev worker
./bin/tef start-worker demo --plan-variant dev

# Terminal 2: Execute on dev
./bin/tef execute demo '{"message": "test"}' dev
```

Multiple environments:
```bash
# Start workers for different environments
./bin/tef start-worker demo --plan-variant dev
./bin/tef start-worker demo --plan-variant staging
./bin/tef start-worker demo --plan-variant prod

# Execute on specific environment
./bin/tef execute demo '{"message": "staging test"}' staging
```

A/B testing workflows:
```bash
# Start workers for different plan versions
./bin/tef start-worker demo --plan-variant v1
./bin/tef start-worker demo --plan-variant v2

# Test both versions
./bin/tef execute demo '{"message": "test"}' v1
./bin/tef execute demo '{"message": "test"}' v2
```

### status

Check the status of a workflow execution.

**Syntax:**
```bash
./bin/tef status <plan-name> <workflow-id> [flags]
```

**Arguments:**
* `plan-name`: Name of the plan
* `workflow-id`: Workflow ID returned by the execute command

**Example:**
```bash
./bin/tef status demo demo-20250126-123456-abc123
```

**Output:**
```
Workflow ID: demo-20250126-123456-abc123
Status: Running
Current Tasks: ["process branch 1", "process branch 2"]

Workflow Details:
  Name: demo-dev
  Description: Demo plan showcasing TEF features
  Input: {"message": "Hello", "count": 5}

Task Status:
  ✓ print message (Completed)
  ✓ check count (Completed)
  ⟳ process branch 1 (In Progress)
  ⟳ process branch 2 (In Progress)
  ○ wait (Pending)
  ○ end (Pending)
```

### list-executions

List all executions for a plan.

**Syntax:**
```bash
./bin/tef list-executions <plan-name> [flags]
```

**Arguments:**
* `plan-name`: Name of the plan

**Flags:**
* `--plan-variant string`: Filter by plan variant
* `--status string`: Filter by status (Running, Completed, Failed, Terminated)
* `--limit int`: Maximum number of executions to return (default: 100)

**Examples:**

```bash
# List all executions for demo plan
./bin/tef list-executions demo

# List only running executions for dev variant
./bin/tef list-executions demo --plan-variant dev --status Running

# List last 10 executions
./bin/tef list-executions demo --limit 10
```

**Output:**
```
Plan: demo
Variant: dev

Workflow ID                          Status      Start Time
demo-20250126-123456-abc123         Running     2025-01-26 12:34:56
demo-20250126-120000-def456         Completed   2025-01-26 12:00:00
demo-20250126-110000-ghi789         Failed      2025-01-26 11:00:00

Total: 3 executions
```

### resume

Resume an async task that is waiting for external input.

**Syntax:**
```bash
./bin/tef resume <plan-name> <plan-id> <workflow-id> <step-id> '<result-json>' [flags]
```

**Arguments:**
1. `plan-name`: Name of the plan
2. `plan-id`: Full plan ID including variant (e.g., `demo-dev`)
3. `workflow-id`: The workflow execution ID
4. `step-id`: The step ID returned by AsyncTask's ExecutionFn
5. `result-json`: JSON string with result data to pass to AsyncTask's ResultProcessorFn

**Flags:**
* `--use-api`: Resume via REST API (default: `true`)
* `--api-host string`: API server host (default: `localhost`)
* `--api-port int`: API server port (default: `25780`)

**Examples:**

```bash
# Resume via REST API (default)
./bin/tef resume batch-import \
  batch-import-prod \
  batch-import-abc123 \
  import-job-xyz789 \
  '{"status": "completed", "rows_imported": 10000}'

# Resume via direct Temporal connection
./bin/tef resume batch-import \
  batch-import-prod \
  batch-import-abc123 \
  import-job-xyz789 \
  '{"status": "completed"}' \
  --use-api=false

# Resume with custom API server
./bin/tef resume batch-import \
  batch-import-prod \
  batch-import-abc123 \
  import-job-xyz789 \
  '{"status": "completed"}' \
  --api-host api.production.example.com \
  --api-port 8080
```

**Workflow:**

1. Execute a plan containing an AsyncTask
2. The AsyncTask's ExecutionFn submits work to an external system and returns a step ID
3. The workflow waits for external completion
4. When the external work completes, use `resume` to signal completion and provide results
5. The AsyncTask's ResultProcessorFn processes the results and continues the workflow

**Complete Example:**

```bash
# Terminal 1: Start worker
./bin/tef start-worker batch-import --plan-variant prod

# Terminal 2: Execute plan (starts async batch import)
./bin/tef execute batch-import '{"file": "data.csv"}' prod
# Output: Workflow started with ID: batch-import-abc123
#         Step ID from AsyncTask: import-job-xyz789

# Terminal 3: Later, when external batch job completes, resume the workflow
./bin/tef resume batch-import \
  batch-import-prod \
  batch-import-abc123 \
  import-job-xyz789 \
  '{"rows_imported": 10000, "status": "success"}'
```

### gen-view

Generate a visual representation of the workflow.

**Syntax:**
```bash
./bin/tef gen-view <plan-name> [flags]
```

**Arguments:**
* `plan-name`: Name of the plan to visualize

**Flags:**
* `--output string`: Output file name (default: `<plan-name>.jpg`)
* `--format string`: Output format: `jpg`, `png`, `svg`, `dot` (default: `jpg`)

**Examples:**

```bash
# Generate visualization with default settings
./bin/tef gen-view demo

# Generate PNG instead of JPG
./bin/tef gen-view demo --format png

# Specify custom output file
./bin/tef gen-view demo --output my-workflow.svg --format svg

# Generate DOT file only (no rendering)
./bin/tef gen-view demo --format dot
```

**Output:**
```
Generating plan visualization for: demo
DOT file generated: demo.dot
Rendering diagram to: demo.jpg
Visualization complete!
```

**Requirements:**
* Graphviz must be installed for rendering (not required for DOT output)
* Install with: `brew install graphviz` (macOS) or `apt-get install graphviz` (Linux)

**Generated Diagram Shows:**
* All tasks and their relationships
* Execution paths (Next, Fail)
* Fork branches (parallel execution)
* Conditional branches (Then/Else)
* End tasks
* Task types and executors

### serve

Start the REST API server.

**Syntax:**
```bash
./bin/tef serve [flags]
```

**Flags:**
* `--port int`: Port to listen on (default: `25780`)
* `--host string`: Host to bind to (default: `localhost`)
* `--cors`: Enable CORS (default: `false`)

**Examples:**

```bash
# Start API server on default port
./bin/tef serve

# Start on custom port
./bin/tef serve --port 8080

# Start with CORS enabled
./bin/tef serve --cors

# Start on all interfaces
./bin/tef serve --host 0.0.0.0 --port 25780
```

**Output:**
```
TEF API Server starting...
Loaded plans: demo, pua, roachprod
Listening on http://localhost:25780
API documentation: http://localhost:25780/docs
```

For API endpoints, see [API.md](API.md).

### standalone

Execute a plan with in-memory backend in a single process.

**Syntax:**
```bash
./bin/tef-light standalone <plan-name> [flags]
```

**Arguments:**
* `plan-name`: Name of the plan to execute

**Flags:**
* `--input string`: JSON input for the workflow (required)
* `--with-plans string`: Comma-separated list of plans to start workers for (defaults to executed plan)
* `--with-plans-regex string`: Regex pattern to match plan names for starting workers (e.g., 'pua.*' matches 'pua-*')
* `--plan-variant string`: Plan variant (defaults to auto-generated dev-UUID)
* `--port int`: HTTP server port (default: 8081)

**Examples:**

```bash
# Execute with default settings (only demo plan worker)
./bin/tef-light standalone demo --input '{"message":"test"}'

# Execute with multiple workers (comma-separated)
./bin/tef-light standalone demo \
  --input '{"cluster":"test"}' \
  --with-plans demo,cluster-setup \
  --port 8081

# Execute with multiple workers (regex pattern)
./bin/tef-light standalone demo \
  --input '{"cluster":"test"}' \
  --with-plans-regex "pua.*" \
  --port 8082

# Execute with custom plan variant
./bin/tef-light standalone demo \
  --input '{"message":"test"}' \
  --plan-variant dev \
  --port 8081
```

**Behavior:**
* Runs workflows entirely in-memory within a single process
* Starts an HTTP server for status queries and resume operations
* Initializes workers for specified plans (via `--with-plans` or `--with-plans-regex`)
* Executes the workflow asynchronously in background goroutines
* Keeps the server running until interrupted (Ctrl+C)
* No external dependencies (no Temporal, no database)

**When to Use:**
* Development and testing without orchestration infrastructure
* Quick iteration on plan definitions
* Workflow visualization and validation
* Demos without infrastructure requirements

**Note:** The `--with-plans` and `--with-plans-regex` flags are mutually exclusive.

For detailed documentation, see [planners/inmemory/README.md](planners/inmemory/README.md).

## Command Aliases

Some commands have shorter aliases:

```bash
./bin/tef sw <plan-name>          # Alias for start-worker
./bin/tef exec <plan-name> ...    # Alias for execute
./bin/tef ls <plan-name>          # Alias for list-executions
```

## Exit Codes

TEF commands use the following exit codes:

* `0`: Success
* `1`: General error
* `2`: Invalid arguments
* `3`: Plan not found
* `4`: Workflow not found
* `5`: Execution error
* `6`: Connection error (orchestration engine unreachable)

## Environment Variables

TEF respects the following environment variables:

```bash
# Temporal configuration
TEMPORAL_ADDRESS=localhost:7233
TEMPORAL_NAMESPACE=default

# API configuration
TEF_API_HOST=localhost
TEF_API_PORT=25780

# Logging
TEF_LOG_LEVEL=info  # debug, info, warn, error
```

## Tips and Best Practices

### Development Workflow

```bash
# Terminal 1: Run Temporal locally
temporal server start-dev

# Terminal 2: Start worker
./bin/tef start-worker demo --plan-variant dev

# Terminal 3: Execute and test
./bin/tef execute demo '{"message": "test"}' dev
./bin/tef status demo <workflow-id>
```

### Production Workflow

```bash
# Use explicit plan variants
./bin/tef start-worker demo --plan-variant prod

# Always specify the same variant when executing
./bin/tef execute demo '{"message": "production"}' prod

# Monitor executions
./bin/tef list-executions demo --plan-variant prod --status Running
```

### Debugging

```bash
# Generate workflow visualization to understand structure
./bin/tef gen-view demo

# Check detailed status
./bin/tef status demo <workflow-id>

# List recent executions to find failures
./bin/tef list-executions demo --status Failed --limit 10
```

### Managing Multiple Plans

```bash
# Start workers for multiple plans
./bin/tef start-worker plan1 --plan-variant prod &
./bin/tef start-worker plan2 --plan-variant prod &
./bin/tef start-worker plan3 --plan-variant prod &

# Execute different plans
./bin/tef execute plan1 '{"input": "data"}' prod
./bin/tef execute plan2 '{"input": "data"}' prod
```

## Getting Help

```bash
# Show all available commands
./bin/tef --help

# Show help for a specific command
./bin/tef start-worker --help
./bin/tef execute --help

# Show available plans
./bin/tef serve
# Then visit http://localhost:25780/v1/plans
```

## Additional Resources

* [README.md](README.md) - Overview and getting started
* [API.md](API.md) - REST API documentation
* [plans/README.md](plans/README.md) - Plan development guide
* [TODO.md](TODO.md) - Implementation status
