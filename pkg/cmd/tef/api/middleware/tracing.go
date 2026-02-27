// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/google/uuid"
)

// Context keys for request metadata.
type contextKey int

const (
	requestIDKey contextKey = iota
	traceParentKey
	traceStateKey
)

// TracingMiddleware adds request tracing support to the API server.
// It handles:
// - X-Request-ID header (client-provided or auto-generated)
// - W3C Trace Context (traceparent, tracestate)
// - Request logging with timing information
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := planners.LoggerFromContext(ctx)

		// Extract or generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Extract W3C Trace Context headers if present
		traceParent := r.Header.Get("traceparent")
		traceState := r.Header.Get("tracestate")

		// Store request metadata in context
		ctx = context.WithValue(ctx, requestIDKey, requestID)
		if traceParent != "" {
			ctx = context.WithValue(ctx, traceParentKey, traceParent)
		}
		if traceState != "" {
			ctx = context.WithValue(ctx, traceStateKey, traceState)
		}

		// Create response writer wrapper to capture status code
		wrappedWriter := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // default
		}

		// Set X-Request-ID in response headers (before writing response)
		w.Header().Set("X-Request-ID", requestID)

		// Log request start
		startTime := time.Now()
		logger.Info("Request started",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"trace_parent", traceParent,
		)

		// Call the next handler with an updated context
		next.ServeHTTP(wrappedWriter, r.WithContext(ctx))

		// Log request completion
		duration := time.Since(startTime)
		logger.Info("Request completed",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status_code", wrappedWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestIDFromContext extracts the request ID from the context.
// Returns empty string if not present.
func RequestIDFromContext(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// TraceParentFromContext extracts the W3C traceparent from the context.
// Returns empty string if not present.
func TraceParentFromContext(ctx context.Context) string {
	if traceParent, ok := ctx.Value(traceParentKey).(string); ok {
		return traceParent
	}
	return ""
}

// TraceStateFromContext extracts the W3C tracestate from the context.
// Returns empty string if not present.
func TraceStateFromContext(ctx context.Context) string {
	if traceState, ok := ctx.Value(traceStateKey).(string); ok {
		return traceState
	}
	return ""
}

// ContextWithRequestID returns a new context with the request ID attached.
// Use this when propagating request IDs through application code or to downstream services.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// ContextWithTraceParent returns a new context with the W3C traceparent attached.
// Use this when propagating trace context to downstream services.
func ContextWithTraceParent(ctx context.Context, traceParent string) context.Context {
	return context.WithValue(ctx, traceParentKey, traceParent)
}

// ContextWithTraceState returns a new context with the W3C tracestate attached.
// Use this when propagating trace context to downstream services.
func ContextWithTraceState(ctx context.Context, traceState string) context.Context {
	return context.WithValue(ctx, traceStateKey, traceState)
}
