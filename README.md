# Lumberjack Go SDK v2

A Go SDK for Lumberjack observability platform built on OpenTelemetry with slog-compatible logging API.

## Features

- **OpenTelemetry Integration**: Built on standard OpenTelemetry libraries for traces, logs, and metrics
- **slog Compatible**: Drop-in replacement for Go's standard slog package with context support
- **Automatic Batching**: Efficient batching of logs and spans to your Lumberjack endpoints
- **Sensible Defaults**: Works out of the box with minimal configuration
- **Context Tracing**: Automatic trace ID injection into logs when using context

## Installation

```bash
go get github.com/lumberjack-dev/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    
    "github.com/lumberjack-dev/go-sdk"
)

func main() {
    // Initialize SDK
    config := lumberjack.NewConfig().
        WithAPIKey("your-api-key").
        WithProjectName("my-project").
        WithDebug(true)
    
    sdk := lumberjack.Init(config)
    defer sdk.Shutdown(context.Background())
    
    ctx := context.Background()
    
    // Basic logging (slog compatible)
    lumberjack.Info("Application starting", "version", "1.0.0")
    
    // Start a span
    ctx, span := lumberjack.StartSpan(ctx, "example-operation")
    defer span.End()
    
    // Log with context (includes trace information)
    lumberjack.InfoContext(ctx, "Processing request", "user_id", 123)
    
    // Structured logging with attributes
    lumberjack.LogAttrs(ctx, slog.LevelInfo, "Custom log",
        slog.String("operation", "user_lookup"),
        slog.Int("count", 5),
    )
}
```

## Configuration

### Environment Variables

- `LUMBERJACK_API_KEY`: Your Lumberjack API key
- `LUMBERJACK_BASE_URL`: Base URL for Lumberjack API (default: https://api.lumberjackhq.com)
- `LUMBERJACK_PROJECT_NAME`: Project name
- `LUMBERJACK_DEBUG`: Enable debug mode (true/false)
- `LUMBERJACK_BATCH_SIZE`: Batch size for logs and spans (default: 100)
- `LUMBERJACK_RELEASE_ID`: Release identifier
- `LUMBERJACK_RELEASE_TYPE`: Release type (commit/random)

### Programmatic Configuration

```go
config := lumberjack.NewConfig().
    WithAPIKey("your-api-key").
    WithBaseURL("https://api.lumberjackhq.com").
    WithProjectName("my-project").
    WithDebug(false)

sdk := lumberjack.Init(config)
```

## Logging API

The SDK provides a slog-compatible logging API:

```go
// Basic logging
lumberjack.Debug("Debug message", "key", "value")
lumberjack.Info("Info message", "key", "value")
lumberjack.Warn("Warning message", "key", "value")
lumberjack.Error("Error message", "key", "value")

// Context-aware logging (includes trace information)
lumberjack.DebugContext(ctx, "Debug with context")
lumberjack.InfoContext(ctx, "Info with context")
lumberjack.WarnContext(ctx, "Warning with context")
lumberjack.ErrorContext(ctx, "Error with context")

// Structured logging with attributes
lumberjack.LogAttrs(ctx, slog.LevelInfo, "Structured log",
    slog.String("user", "john"),
    slog.Int("age", 30),
)

// Logger with pre-configured attributes
logger := lumberjack.With("component", "database")
logger.InfoContext(ctx, "Query executed", "duration_ms", 100)
```

## Tracing

Built on OpenTelemetry tracing:

```go
ctx := context.Background()

// Start a span
ctx, span := lumberjack.StartSpan(ctx, "operation-name")
defer span.End()

// Add attributes to span
span.SetAttributes(
    attribute.String("user.id", "123"),
    attribute.Int("retry.count", 3),
)

// Set span status
span.SetStatus(codes.Ok, "Operation completed")

// Nested spans
ctx, childSpan := lumberjack.StartSpan(ctx, "child-operation")
// ... do work
childSpan.End()
```

## Metrics

Basic metrics collection:

```go
meter := lumberjack.Meter()

// Create a counter
counter, _ := meter.Int64Counter("requests_total")
counter.Add(ctx, 1, metric.WithAttributes(
    attribute.String("method", "GET"),
))

// Create a histogram
histogram, _ := meter.Float64Histogram("request_duration")
histogram.Record(ctx, 0.5) // 500ms
```

## Migration from v1

### Key Changes

1. **slog API**: The logging API now matches Go's slog package
2. **Context Required**: Context-aware functions require `context.Context`
3. **OpenTelemetry**: Built on standard OpenTelemetry libraries
4. **Configuration**: New configuration structure

### Migration Steps

**Before (v1):**
```go
lumberjack.Init(core.Config{
    ProjectName: "my-project",
    APIKey:      "api-key",
})

lumberjack.Info("Hello", map[string]interface{}{"key": "value"})
```

**After (v2):**
```go
config := lumberjack.NewConfig().
    WithProjectName("my-project").
    WithAPIKey("api-key")

sdk := lumberjack.Init(config)
defer sdk.Shutdown(context.Background())

lumberjack.Info("Hello", "key", "value")
```

### Breaking Changes

- Logging functions now use variadic key-value pairs instead of maps
- Context-aware functions require explicit context parameter
- Configuration structure has changed
- Some function names have changed to match slog conventions

## Best Practices

1. **Always call Shutdown()**: Ensure proper cleanup and flushing of remaining data
2. **Use Context**: Prefer context-aware logging functions for automatic trace correlation
3. **Structured Logging**: Use key-value pairs for better searchability
4. **Span Lifecycle**: Always call `span.End()` (use defer for safety)
5. **Error Handling**: Set appropriate span status on errors

## Example: HTTP Server

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx, span := lumberjack.StartSpan(r.Context(), "http-request")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("http.method", r.Method),
        attribute.String("http.path", r.URL.Path),
    )
    
    lumberjack.InfoContext(ctx, "Handling request",
        "method", r.Method,
        "path", r.URL.Path,
    )
    
    // ... handle request
    
    span.SetStatus(codes.Ok, "Request completed")
    w.WriteHeader(http.StatusOK)
}
```