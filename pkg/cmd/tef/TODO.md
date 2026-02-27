# TEF Framework TODO

This document tracks pending work for the Task Execution Framework **framework code** in this repository.

**Note**: Backend-specific work (Temporal, other orchestration engines) is tracked in their respective repositories.

## Framework Status

### ✅ Implemented

- Core interfaces (Planner, Registry, PlannerManager, etc.)
- Factory pattern for backend injection
- BasePlanner with full validation logic
- All seven task types with validation
- Plan registry infrastructure  
- Manager registry for cross-plan task execution
- CLI framework with factory injection
- API server framework
- Status and execution tracking types
- Logger interface and implementation
- Utility functions for plan ID management

### 🚧 In Progress

- Additional plan implementations
- Enhanced validation and error reporting
- Performance optimizations

### Potential Future Work

## 1. Additional Task Types

Consider adding new task types if needed:

- **LoopTask**: Execute a task repeatedly based on a condition
- **MapTask**: Execute a task in parallel for each item in a collection
- **WaitTask**: Delay execution for a specified duration
- **RetryTask**: Retry a task with custom backoff logic

**Status**: Design needed - gather requirements from plan authors

## 2. Enhanced Validation

Potential validation improvements:

- **Resource validation**: Check executor parameter types match usage
- **Deadlock detection**: Detect potential deadlocks in complex fork/join patterns
- **Reachability analysis**: Warn about unreachable tasks
- **Performance hints**: Suggest optimizations (e.g., parallel execution opportunities)

**Status**: Nice to have - requires cost/benefit analysis

## 3. Testing Infrastructure

Framework testing improvements:

- **Integration test helpers**: Simplified testing for plan implementations
- **Mock factory**: Built-in mock factory for testing
- **Validation test suite**: Comprehensive validation test cases
- **Performance benchmarks**: Benchmark suite for validation and execution

**Status**: Can be done incrementally as plans are added

## 4. Documentation

Documentation improvements:

- **Tutorial series**: Step-by-step guides for common patterns
- **Best practices guide**: Patterns and anti-patterns
- **Troubleshooting guide**: Common issues and solutions
- **API versioning guide**: Strategy for evolving interfaces

**Status**: Living document - add as patterns emerge

## 5. Plan Development Tools

Tooling to help plan authors:

- **Plan visualizer**: Generate flowcharts from plan definitions
- **Plan linter**: Detect common mistakes in plan code
- **Plan scaffold generator**: CLI tool to generate plan boilerplate
- **Execution replay**: Replay workflow executions for debugging

**Status**: Visualizer implemented (gen-view), others TBD

## 6. Framework Interfaces

Potential interface enhancements:

- **Context propagation**: Standardize metadata/context passing
- **Resource cleanup**: Lifecycle hooks for resource management
- **Plan composition**: Better support for plan reuse
- **Dynamic task generation**: Support for runtime task graph modification

**Status**: Gather feedback from plan authors first

## 7. Backend Support

Framework changes to better support various backends:

- **Local executor**: Support for local/in-process execution
- **Kubernetes Jobs**: Support for K8s-based orchestration
- **AWS Step Functions**: Support for Step Functions backend
- **Custom backends**: Make it easier to implement new backends

**Status**: Factory pattern is in place, gather backend requirements

## Notes

- **Backend-specific work** (Temporal improvements, etc.) should be tracked in backend repositories
- **Plan-specific work** should be tracked in plan-specific issues/docs
- This file tracks **framework enhancements** that benefit all backends and plans

## Contributing

When adding framework features:

1. Ensure backward compatibility
2. Add comprehensive tests
3. Update FACTORY_ARCHITECTURE.md if interfaces change
4. Add examples in demo plans
5. Update relevant documentation

## Questions?

- Framework design: See FACTORY_ARCHITECTURE.md
- Plan development: See README.md
- API usage: See API.md
- CLI usage: See CLI.md
