// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package plans contains the plan registration entry point for the Task
// Execution Framework (TEF). Plan implementations register themselves here
// to be made available to the TEF CLI and execution runtime.
package plans

import (
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/plans/demo"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/plans/pua"
)

// RegisterPlans is the central registration function for all TEF plan implementations.
// Plan authors should call pr.Register() here to make their plans available to the TEF CLI.
//
// Example:
//
//	func RegisterPlans(pr *planners.PlanRegistry) {
//	    myplan.RegisterPlans(pr),
//	    demo.RegisterPlans(pr),
//	}
func RegisterPlans(pr *planners.PlanRegistry) {
	demo.RegisterDemoPlans(pr)
	pua.RegisterPUAPlans(pr)
}
