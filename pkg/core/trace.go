package core

import (
	"context"
	"time"

	"github.com/treebeard/go-sdk/pkg/trace"
	tbcontext "github.com/treebeard/go-sdk/pkg/context"
)

// StartTrace creates a new trace context with a generated trace ID
func (c *Client) StartTrace(ctx context.Context, name string, fields ...map[string]interface{}) context.Context {
	traceID := trace.GenerateTraceID()
	
	newCtx := tbcontext.WithTraceID(ctx, traceID)
	newCtx = tbcontext.WithTraceName(newCtx, name)

	// Add trace start marker
	props := map[string]interface{}{
		"tb_trace_start": true,
		"trace_name":     name,
	}

	if len(fields) > 0 && fields[0] != nil {
		for k, v := range fields[0] {
			props[k] = v
		}
	}

	newCtx = tbcontext.WithProps(newCtx, props)

	// Log the trace start
	c.InfoWithContext(newCtx, "Beginning "+name, props)

	return newCtx
}

// CompleteTraceSuccess marks a trace as successfully completed
func (c *Client) CompleteTraceSuccess(ctx context.Context, fields ...map[string]interface{}) {
	traceName := tbcontext.GetTraceName(ctx)
	
	props := map[string]interface{}{
		"tb_trace_complete_success": true,
		"trace_name":                traceName,
	}

	if len(fields) > 0 && fields[0] != nil {
		for k, v := range fields[0] {
			props[k] = v
		}
	}

	c.InfoWithContext(ctx, "Completed "+traceName, props)
}

// CompleteTraceError marks a trace as completed with an error
func (c *Client) CompleteTraceError(ctx context.Context, err error, fields ...map[string]interface{}) {
	traceName := tbcontext.GetTraceName(ctx)
	
	props := map[string]interface{}{
		"tb_trace_complete_error": true,
		"trace_name":              traceName,
		"error":                   err.Error(),
	}

	if len(fields) > 0 && fields[0] != nil {
		for k, v := range fields[0] {
			props[k] = v
		}
	}

	// Add stack trace
	if err != nil {
		props["traceback"] = trace.GetStackTrace(1)
	}

	c.ErrorWithContext(ctx, "Failed "+traceName, props)
}

// TraceFunc is a higher-order function that wraps a function with tracing
func (c *Client) TraceFunc(name string, fn func(ctx context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		traceCtx := c.StartTrace(ctx, name)
		
		start := time.Now()
		err := fn(traceCtx)
		duration := time.Since(start)

		fields := map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
		}

		if err != nil {
			c.CompleteTraceError(traceCtx, err, fields)
		} else {
			c.CompleteTraceSuccess(traceCtx, fields)
		}

		return err
	}
}

// TraceFuncWithResult wraps a function that returns a result and an error
func (c *Client) TraceFuncWithResult(name string, fn func(ctx context.Context) (interface{}, error)) func(context.Context) (interface{}, error) {
	return func(ctx context.Context) (interface{}, error) {
		traceCtx := c.StartTrace(ctx, name)
		
		start := time.Now()
		result, err := fn(traceCtx)
		duration := time.Since(start)

		fields := map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
		}

		if err != nil {
			c.CompleteTraceError(traceCtx, err, fields)
		} else {
			c.CompleteTraceSuccess(traceCtx, fields)
		}

		return result, err
	}
}