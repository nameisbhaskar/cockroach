// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package planners provide the factory interface for creating PlannerManager instances.
package planners

import "context"

// PlannerManagerFactory creates PlannerManager instances from Registry.
// This interface enables dependency injection of framework-specific implementations
// (e.g., Temporal, local executor) without creating direct dependencies on those
// frameworks in the core TEF code.
//
// Framework implementers should:
//  1. Implement this interface in their framework-specific package
//  2. Provide a constructor that returns their factory implementation
//  3. Inject the factory at the application entry point
//
// Example usage:
//
//	// In framework-specific package (e.g., temporal/factory.go)
//	type temporalFactory struct{}
//
//	func NewTemporalFactory() planners.PlannerManagerFactory {
//	    return &temporalFactory{}
//	}
//
//	func (f *temporalFactory) CreateManager(ctx context.Context, r planners.Registry) (planners.PlannerManager, error) {
//	    return NewPlannerManager(ctx, r)
//	}
//
//	// In main.go
//	factory := temporal.NewTemporalFactory()
//	pr := planners.NewPlanRegistry()
//	plans.RegisterPlans(pr)
//	cli.Initialize(factory, pr)
type PlannerManagerFactory interface {
	// CreateManager creates a new PlannerManager instance for the given Registry.
	// The created manager should be automatically registered in the global managers
	// registry for child task execution.
	//
	// Returns an error if plan validation fails or if the manager cannot be created.
	CreateManager(ctx context.Context, r Registry) (PlannerManager, error)
}
