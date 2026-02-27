// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/middleware"
	"github.com/cockroachdb/cockroach/pkg/cmd/tef/api/types"
	"github.com/google/uuid"
)

// Client represents a client for the TEF REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	requestID  string // Optional client-provided request ID for tracing
}

// NewClient creates a new TEF REST API client.
func NewClient(apiBaseUrl string) *Client {
	// Create a custom dialer that uses the Go DNS resolver.
	// This is necessary for Kubernetes environments where the default
	// resolver may not work correctly with service DNS names.
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Create a custom transport with the dialer.
	transport := &http.Transport{
		DialContext: dialer.DialContext,
		// Force the use of the Go DNS resolver instead of cgo.
		// This ensures proper DNS resolution in Kubernetes.
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		baseURL: apiBaseUrl,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
	}
}

// WithRequestID sets a custom request ID for tracing.
// If not set, a unique ID will be generated for each request.
func (c *Client) WithRequestID(requestID string) *Client {
	c.requestID = requestID
	return c
}

// addTracingHeaders adds request ID and trace context headers to the request.
// If no request ID is set, generates a new one for this request.
// Use middleware.ContextWithTraceParent() and middleware.ContextWithTraceState()
// to add trace context to the context before calling SDK methods.
func (c *Client) addTracingHeaders(ctx context.Context, req *http.Request) string {
	requestID := c.requestID
	if requestID == "" {
		requestID = uuid.New().String()
	}
	req.Header.Set("X-Request-ID", requestID)

	// Extract trace context from context if present (for propagation)
	if traceParent := middleware.TraceParentFromContext(ctx); traceParent != "" {
		req.Header.Set("traceparent", traceParent)
	}
	if traceState := middleware.TraceStateFromContext(ctx); traceState != "" {
		req.Header.Set("tracestate", traceState)
	}

	return requestID
}

// ExecutePlan executes a plan via the REST API.
func (c *Client) ExecutePlan(
	ctx context.Context, planID string, request map[string]interface{},
) (*types.ExecutionResponse, error) {
	// Prepare the request body
	execReq := types.ExecutionRequest{
		Request: request,
	}

	body, err := json.Marshal(execReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/plans/%s/executions", c.baseURL, planID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Add tracing headers
	c.addTracingHeaders(ctx, req)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp types.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("server error: %s", errResp.Error)
	}

	// Parse success response
	var execResp types.ExecutionResponse
	if err := json.Unmarshal(respBody, &execResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &execResp, nil
}

// ResumePlan resumes an async task in a running workflow via the REST API.
func (c *Client) ResumePlan(
	ctx context.Context, planID, executionID, stepID string, result string,
) (*types.ResumeResponse, error) {
	// Prepare request body
	resumeReq := types.ResumeRequest{
		Result: result,
	}

	body, err := json.Marshal(resumeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/plans/%s/executions/%s/steps/%s/resume", c.baseURL, planID, executionID, stepID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Add tracing headers
	c.addTracingHeaders(ctx, req)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp types.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("server error: %s", errResp.Error)
	}

	// Parse success response
	var resumeResp types.ResumeResponse
	if err := json.Unmarshal(respBody, &resumeResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resumeResp, nil
}

// ListPlans lists all available plans via the REST API.
func (c *Client) ListPlans(ctx context.Context) (*types.ListPlansResponse, error) {
	// Create HTTP request
	url := fmt.Sprintf("%s/v1/plans", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add tracing headers
	c.addTracingHeaders(ctx, req)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp types.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("server error: %s", errResp.Error)
	}

	// Parse success response
	var listResp types.ListPlansResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &listResp, nil
}

// ListExecutions lists all executions for a specific plan via the REST API.
func (c *Client) ListExecutions(
	ctx context.Context, planID string,
) (*types.ListExecutionsResponse, error) {
	// Create HTTP request
	url := fmt.Sprintf("%s/v1/plans/%s/executions", c.baseURL, planID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add tracing headers
	c.addTracingHeaders(ctx, req)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp types.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("server error: %s", errResp.Error)
	}

	// Parse success response
	var listResp types.ListExecutionsResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &listResp, nil
}

// GetExecutionStatus gets the status of a specific execution via the REST API.
func (c *Client) GetExecutionStatus(
	ctx context.Context, planID, executionID string,
) (*types.ExecutionStatus, error) {
	// Create HTTP request
	url := fmt.Sprintf("%s/v1/plans/%s/executions/%s", c.baseURL, planID, executionID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add tracing headers
	c.addTracingHeaders(ctx, req)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp types.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("server error: %s", errResp.Error)
	}

	// Parse success response
	var statusResp types.ExecutionStatus
	if err := json.Unmarshal(respBody, &statusResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &statusResp, nil
}
