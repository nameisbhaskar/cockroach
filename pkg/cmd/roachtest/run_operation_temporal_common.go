package main

import (
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
)

// This file contains shared constants and types for the Temporal workflow implementation
// that are used by both the client and server components.

const (
	// Temporal task queue name for operations
	operationsTaskQueue = "operations-task-queue"
	// Workflow ID for the operations workflow
	operationsWorkflowID = "operations-workflow"
	// ExecuteOperationSignal is the signal type for requesting operation execution
	ExecuteOperationSignal = "execute-operation"
)

// ExecuteOperationRequest contains the information needed to execute an operation
type ExecuteOperationRequest struct {
	OperationRegex string
	RunInInterval  time.Duration
	MaxRepetitions int // Maximum number of times to repeat the operation (0 = unlimited)
}

// SerializableOperationSpec is a serializable version of registry.OperationSpec without the Run field
type SerializableOperationSpec struct {
	Skip               string
	Name               string
	Owner              registry.Owner
	Timeout            time.Duration
	CompatibleClouds   registry.CloudSet
	Dependencies       []registry.OperationDependency
	CanRunConcurrently registry.OperationIsolation
	WaitBeforeCleanup  time.Duration
}

// NamePrefix returns the first part of `s.Name` after splitting with delimiter `/`
// This is a copy of the same method from registry.OperationSpec
func (s *SerializableOperationSpec) NamePrefix() string {
	parts := strings.Split(s.Name, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return s.Name
}

// ToOperationSpec converts a SerializableOperationSpec back to a registry.OperationSpec
// by looking up the original OperationSpec with the Run field from the provided map
func (s *SerializableOperationSpec) ToOperationSpec(
	origSpecs map[string]*registry.OperationSpec,
) *registry.OperationSpec {
	if orig, ok := origSpecs[s.Name]; ok {
		return orig
	}
	// This should never happen if the code is correct
	return nil
}

// FromOperationSpec creates a SerializableOperationSpec from a registry.OperationSpec
func FromOperationSpec(spec *registry.OperationSpec) SerializableOperationSpec {
	return SerializableOperationSpec{
		Skip:               spec.Skip,
		Name:               spec.Name,
		Owner:              spec.Owner,
		Timeout:            spec.Timeout,
		CompatibleClouds:   spec.CompatibleClouds,
		Dependencies:       spec.Dependencies,
		CanRunConcurrently: spec.CanRunConcurrently,
		WaitBeforeCleanup:  spec.WaitBeforeCleanup,
	}
}

// OperationsWorkflowInput contains all the input parameters for the operations workflow
type OperationsWorkflowInput struct {
	ClusterName         string
	NodeCount           int
	Seed                int64
	Parallelism         int
	WorkloadClusterName string
	WorkloadNodes       int
	DatadogTags         []string
	WaitBeforeNextExec  time.Duration
}

// OperationActivityInput contains the input for a single operation activity
type OperationActivityInput struct {
	ClusterName         string
	NodeCount           int
	OperationSpec       *SerializableOperationSpec
	Seed                int64
	WorkerIdx           int
	WorkloadClusterName string
	WorkloadNodes       int
	DatadogTags         []string
}
