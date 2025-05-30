// Package treebeard provides a Go SDK for collecting and transferring logs to TreebeardHQ
package treebeard

import (
	"context"
	"runtime"

	"github.com/treebeard/go-sdk/pkg/core"
)

var defaultClient *core.Client

// Init initializes the Treebeard SDK with the given configuration
func Init(config core.Config) {
	defaultClient = core.New(config)

	// Set up panic recovery
	setupPanicRecovery()
}

// setupPanicRecovery sets up automatic panic logging and recovery
func setupPanicRecovery() {
	if r := recover(); r != nil {
		if defaultClient != nil {
			// Get stack trace
			buf := make([]byte, 4096)
			stackSize := runtime.Stack(buf, false)
			stackTrace := string(buf[:stackSize])

			defaultClient.Critical("Panic recovered", map[string]interface{}{
				"panic_value": r,
				"stack_trace": stackTrace,
			})
		}
		panic(r) // Re-panic to maintain original behavior
	}
}

// GetClient returns the default client instance
func GetClient() *core.Client {
	if defaultClient == nil {
		defaultClient = core.GetInstance()
	}
	return defaultClient
}

// Package-level convenience functions that use the default client

// Debug logs a debug message
func Debug(message string, fields ...map[string]interface{}) {
	GetClient().Debug(message, fields...)
}

// Info logs an info message
func Info(message string, fields ...map[string]interface{}) {
	GetClient().Info(message, fields...)
}

// Warn logs a warning message
func Warn(message string, fields ...map[string]interface{}) {
	GetClient().Warn(message, fields...)
}

// Error logs an error message
func Error(message string, fields ...map[string]interface{}) {
	GetClient().Error(message, fields...)
}

// Critical logs a critical message
func Critical(message string, fields ...map[string]interface{}) {
	GetClient().Critical(message, fields...)
}

// Context-aware logging functions

// DebugWithContext logs a debug message with context
func DebugWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	GetClient().DebugWithContext(ctx, message, fields...)
}

// InfoWithContext logs an info message with context
func InfoWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	GetClient().InfoWithContext(ctx, message, fields...)
}

// WarnWithContext logs a warning message with context
func WarnWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	GetClient().WarnWithContext(ctx, message, fields...)
}

// ErrorWithContext logs an error message with context
func ErrorWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	GetClient().ErrorWithContext(ctx, message, fields...)
}

// CriticalWithContext logs a critical message with context
func CriticalWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	GetClient().CriticalWithContext(ctx, message, fields...)
}

// Tracing functions

// StartTrace creates a new trace context
func StartTrace(ctx context.Context, name string, fields ...map[string]interface{}) context.Context {
	return GetClient().StartTrace(ctx, name, fields...)
}

// CompleteTraceSuccess marks a trace as successfully completed
func CompleteTraceSuccess(ctx context.Context, fields ...map[string]interface{}) {
	GetClient().CompleteTraceSuccess(ctx, fields...)
}

// CompleteTraceError marks a trace as completed with an error
func CompleteTraceError(ctx context.Context, err error, fields ...map[string]interface{}) {
	GetClient().CompleteTraceError(ctx, err, fields...)
}

// TraceFunc wraps a function with tracing
func TraceFunc(name string, fn func(ctx context.Context) error) func(context.Context) error {
	return GetClient().TraceFunc(name, fn)
}

// TraceFuncWithResult wraps a function that returns a result and an error
func TraceFuncWithResult(name string, fn func(ctx context.Context) (interface{}, error)) func(context.Context) (interface{}, error) {
	return GetClient().TraceFuncWithResult(name, fn)
}

// Flush sends all batched logs immediately
func Flush() {
	GetClient().Flush()
}

// Close shuts down the SDK and sends any remaining logs
func Close() {
	if defaultClient != nil {
		defaultClient.Close()
	}
}