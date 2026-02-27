// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package main provides a standalone TEF binary for running non-orchestration commands.
//
// This binary supports commands that don't require an orchestration backend:
//   - gen-view: Generate visual diagrams of plan workflows
//
// Commands that require orchestration (start-worker, execute, resume, serve) are
// not available in this standalone binary.
//
// For full orchestration support:
//  1. Clone the task-exec-framework repository
//  2. Build from that repository: go build -o bin/tef .
//
// See FACTORY_ARCHITECTURE.md for details on the architecture.
package main

import (
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/cli"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners/inmemory"
)

func main() {
	// Create in-memory factory for standalone mode
	factory := inmemory.NewInMemoryFactory()

	// Initialize standalone CLI with in-memory factory
	cli.InitializeStandalone(factory)
}
