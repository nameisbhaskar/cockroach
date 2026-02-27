// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package api

import "github.com/cockroachdb/cockroach/pkg/cmd/tef/api/types"

// HandlerRegistry is an interface for registering HTTP routes.
// Implementations can define groups of related routes that will be
// registered with the server's router.
type HandlerRegistry interface {
	// GetRoutes returns a slice of RouteHandler containing the routes
	// to be registered with the server.
	GetRoutes() []types.RouteHandler
}
