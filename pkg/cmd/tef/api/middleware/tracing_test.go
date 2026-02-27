// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracingMiddleware(t *testing.T) {
	tests := []struct {
		name                   string
		clientRequestID        string // X-Request-ID sent by client
		clientTraceParent      string // traceparent sent by client
		clientTraceState       string // tracestate sent by client
		expectRequestIDSet     bool   // expect X-Request-ID in response
		expectRequestIDInCtx   bool   // expect request ID in context
		expectTraceParentInCtx bool   // expect traceparent in context
		expectTraceStateInCtx  bool   // expect tracestate in context
	}{
		{
			name:                   "client provides request ID",
			clientRequestID:        "client-request-123",
			expectRequestIDSet:     true,
			expectRequestIDInCtx:   true,
			expectTraceParentInCtx: false,
			expectTraceStateInCtx:  false,
		},
		{
			name:                   "client does not provide request ID",
			clientRequestID:        "",
			expectRequestIDSet:     true,
			expectRequestIDInCtx:   true,
			expectTraceParentInCtx: false,
			expectTraceStateInCtx:  false,
		},
		{
			name:                   "client provides trace context",
			clientRequestID:        "req-456",
			clientTraceParent:      "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			clientTraceState:       "rojo=00f067aa0ba902b7",
			expectRequestIDSet:     true,
			expectRequestIDInCtx:   true,
			expectTraceParentInCtx: true,
			expectTraceStateInCtx:  true,
		},
		{
			name:                   "client provides only traceparent",
			clientRequestID:        "req-789",
			clientTraceParent:      "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			expectRequestIDSet:     true,
			expectRequestIDInCtx:   true,
			expectTraceParentInCtx: true,
			expectTraceStateInCtx:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler that captures context
			var capturedCtx context.Context
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedCtx = r.Context()
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with tracing middleware
			handler := TracingMiddleware(testHandler)

			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)

			// Add logger to context
			ctx := planners.ContextWithLogger(req.Context(), planners.NewLogger("info"))
			req = req.WithContext(ctx)

			// Set client headers if provided
			if tt.clientRequestID != "" {
				req.Header.Set("X-Request-ID", tt.clientRequestID)
			}
			if tt.clientTraceParent != "" {
				req.Header.Set("traceparent", tt.clientTraceParent)
			}
			if tt.clientTraceState != "" {
				req.Header.Set("tracestate", tt.clientTraceState)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Execute middleware
			handler.ServeHTTP(rr, req)

			// Verify X-Request-ID in response
			if tt.expectRequestIDSet {
				requestID := rr.Header().Get("X-Request-ID")
				assert.NotEmpty(t, requestID, "X-Request-ID should be set in response")

				if tt.clientRequestID != "" {
					assert.Equal(t, tt.clientRequestID, requestID, "Response X-Request-ID should match client request ID")
				}
			}

			// Verify request ID in context
			if tt.expectRequestIDInCtx {
				requestID := RequestIDFromContext(capturedCtx)
				assert.NotEmpty(t, requestID, "Request ID should be in context")

				if tt.clientRequestID != "" {
					assert.Equal(t, tt.clientRequestID, requestID, "Context request ID should match client request ID")
				}
			}

			// Verify traceparent in context
			if tt.expectTraceParentInCtx {
				traceParent := TraceParentFromContext(capturedCtx)
				assert.Equal(t, tt.clientTraceParent, traceParent, "traceparent should match")
			} else {
				traceParent := TraceParentFromContext(capturedCtx)
				assert.Empty(t, traceParent, "traceparent should be empty")
			}

			// Verify tracestate in context
			if tt.expectTraceStateInCtx {
				traceState := TraceStateFromContext(capturedCtx)
				assert.Equal(t, tt.clientTraceState, traceState, "tracestate should match")
			} else {
				traceState := TraceStateFromContext(capturedCtx)
				assert.Empty(t, traceState, "tracestate should be empty")
			}
		})
	}
}

func TestTracingMiddleware_StatusCodeCapture(t *testing.T) {
	tests := []struct {
		name              string
		handlerStatusCode int
		expectStatusCode  int
	}{
		{
			name:              "200 OK",
			handlerStatusCode: http.StatusOK,
			expectStatusCode:  http.StatusOK,
		},
		{
			name:              "404 Not Found",
			handlerStatusCode: http.StatusNotFound,
			expectStatusCode:  http.StatusNotFound,
		},
		{
			name:              "500 Internal Server Error",
			handlerStatusCode: http.StatusInternalServerError,
			expectStatusCode:  http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.handlerStatusCode)
			})

			// Wrap with tracing middleware
			handler := TracingMiddleware(testHandler)

			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)

			// Add logger to context
			ctx := planners.ContextWithLogger(req.Context(), planners.NewLogger("info"))
			req = req.WithContext(ctx)

			// Create response recorder
			rr := httptest.NewRecorder()

			// Execute middleware
			handler.ServeHTTP(rr, req)

			// Verify status code
			assert.Equal(t, tt.expectStatusCode, rr.Code, "Status code should match")
		})
	}
}

func TestTracingMiddleware_GeneratesUUID(t *testing.T) {
	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with tracing middleware
	handler := TracingMiddleware(testHandler)

	// Create test request (no X-Request-ID header)
	req := httptest.NewRequest("GET", "/test", nil)

	// Add logger to context
	ctx := planners.ContextWithLogger(req.Context(), planners.NewLogger("info"))
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute middleware
	handler.ServeHTTP(rr, req)

	// Verify X-Request-ID is generated
	requestID := rr.Header().Get("X-Request-ID")
	require.NotEmpty(t, requestID, "X-Request-ID should be generated")

	// Verify it's a valid UUID format (simple check)
	assert.Len(t, requestID, 36, "Generated request ID should be UUID length")
	assert.Contains(t, requestID, "-", "Generated request ID should contain hyphens like UUID")
}

func TestContextHelpers(t *testing.T) {
	t.Run("RequestIDFromContext returns empty string when not present", func(t *testing.T) {
		ctx := context.Background()
		requestID := RequestIDFromContext(ctx)
		assert.Empty(t, requestID)
	})

	t.Run("RequestIDFromContext returns value when present", func(t *testing.T) {
		ctx := ContextWithRequestID(context.Background(), "test-request-id")
		requestID := RequestIDFromContext(ctx)
		assert.Equal(t, "test-request-id", requestID)
	})

	t.Run("TraceParentFromContext returns empty string when not present", func(t *testing.T) {
		ctx := context.Background()
		traceParent := TraceParentFromContext(ctx)
		assert.Empty(t, traceParent)
	})

	t.Run("TraceParentFromContext returns value when present", func(t *testing.T) {
		ctx := ContextWithTraceParent(context.Background(), "00-trace-parent")
		traceParent := TraceParentFromContext(ctx)
		assert.Equal(t, "00-trace-parent", traceParent)
	})

	t.Run("TraceStateFromContext returns empty string when not present", func(t *testing.T) {
		ctx := context.Background()
		traceState := TraceStateFromContext(ctx)
		assert.Empty(t, traceState)
	})

	t.Run("TraceStateFromContext returns value when present", func(t *testing.T) {
		ctx := ContextWithTraceState(context.Background(), "state=value")
		traceState := TraceStateFromContext(ctx)
		assert.Equal(t, "state=value", traceState)
	})

	t.Run("Context helpers can be chained", func(t *testing.T) {
		ctx := context.Background()
		ctx = ContextWithRequestID(ctx, "req-123")
		ctx = ContextWithTraceParent(ctx, "00-trace-id")
		ctx = ContextWithTraceState(ctx, "vendor=data")

		assert.Equal(t, "req-123", RequestIDFromContext(ctx))
		assert.Equal(t, "00-trace-id", TraceParentFromContext(ctx))
		assert.Equal(t, "vendor=data", TraceStateFromContext(ctx))
	})
}
