// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package inmemory

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
)

// factory implements PlannerManagerFactory for in-memory execution.
// It creates manager instances that share a common storage backend.
type factory struct {
	storage *storage
}

// Verify interface implementation.
var _ planners.PlannerManagerFactory = &factory{}

// NewInMemoryFactory creates a new in-memory factory with shared storage.
func NewInMemoryFactory() planners.PlannerManagerFactory {
	return &factory{
		storage: newStorage(),
	}
}

// CreateManager creates a new manager for the given registry.
// All managers created by this factory share the same storage instance,
// enabling cross-plan status queries and child plan execution.
func (f *factory) CreateManager(
	ctx context.Context, r planners.Registry,
) (planners.PlannerManager, error) {
	return newManager(ctx, r, f.storage)
}
