# Lumberjack Go SDK v2

A Go SDK for Lumberjack observability platform built on OpenTelemetry with slog-compatible logging API.

## Features

- **OpenTelemetry Integration**: Built on standard OpenTelemetry libraries for traces, logs, and metrics
- **slog Compatible**: Drop-in replacement for Go's standard slog package with context support
- **Automatic Batching**: Efficient batching of logs and spans to your Lumberjack endpoints
- **Sensible Defaults**: Works out of the box with minimal configuration
- **Context Tracing**: Automatic trace ID injection into logs when using context
- **W3C Trace Context**: Support for distributed tracing with traceparent headers

## Installation

```bash
go get github.com/TreebeardHQ/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"

    "github.com/TreebeardHQ/go-sdk"
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
- `LUMBERJACK_BASE_URL`: Base URL for Lumberjack API (default: https://api.trylumberjack.com)
- `LUMBERJACK_PROJECT_NAME`: Project name
- `LUMBERJACK_DEBUG`: Enable debug mode (true/false)
- `LUMBERJACK_BATCH_SIZE`: Batch size for logs and spans (default: 100)
- `LUMBERJACK_REPLACE_SLOG`: Replace global slog handler (default: true)
- `LUMBERJACK_RELEASE_ID`: Release identifier
- `LUMBERJACK_RELEASE_TYPE`: Release type (commit/random)

### Programmatic Configuration

```go
config := lumberjack.NewConfig().
    WithAPIKey("your-api-key").
    WithBaseURL("https://api.trylumberjack.com").
    WithProjectName("my-project").
    WithDebug(false).
    WithReplaceSlog(true)

sdk := lumberjack.Init(config)
```

## Logging API

The SDK provides a slog-compatible logging API with automatic global slog integration:

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

// Standard slog functions work automatically (when ReplaceSlog is enabled)
slog.Info("This goes through Lumberjack too!")
slog.DebugContext(ctx, "Standard slog with context")

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

## Standard slog Integration

By default, the SDK automatically replaces the global slog handler to capture all standard `slog` calls:

### Automatic Integration (Default)

```go
// Initialize SDK - automatically replaces global slog handler
sdk := lumberjack.Init(config)
defer sdk.Shutdown(context.Background())

// Now ALL slog calls go through Lumberjack:
slog.Info("This goes to Lumberjack")
slog.Debug("Debug message")
slog.ErrorContext(ctx, "Error with context")

// Original destination (e.g., stdout) still receives logs via forwarding
```

### Disable slog Integration

```go
config := lumberjack.NewConfig().
    WithReplaceSlog(false)  // Disable global slog replacement

sdk := lumberjack.Init(config)

// Standard slog calls go to their original destination
slog.Info("This goes to default handler")

// Use Lumberjack functions explicitly
lumberjack.Info("This goes to Lumberjack")
```

### How It Works

1. **Replacement**: SDK replaces `slog.Default()` with Lumberjack handler
2. **Forwarding**: Logs are sent to **both** Lumberjack and the previous handler
3. **Restoration**: Original handler is restored on `Shutdown()`
4. **No Loops**: Safe chaining prevents infinite loops

### Environment Variable

```bash
export LUMBERJACK_REPLACE_SLOG=false  # Disable slog replacement
```

## Best Practices

1. **Always call Shutdown()**: Ensure proper cleanup and flushing of remaining data
2. **Use Context**: Prefer context-aware logging functions for automatic trace correlation
3. **Structured Logging**: Use key-value pairs for better searchability
4. **Span Lifecycle**: Always call `span.End()` (use defer for safety)
5. **Error Handling**: Set appropriate span status on errors

## Distributed Tracing

The SDK supports W3C trace context for distributed tracing across services:

### Extracting Traceparent from Incoming Request

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Extract traceparent from incoming request header
    if traceparent := r.Header.Get("traceparent"); traceparent != "" {
        var err error
        ctx, err = lumberjack.ContextWithTraceparent(ctx, traceparent)
        if err != nil {
            lumberjack.Warn("Invalid traceparent header", "error", err)
        }
    }
    
    // Start a span - will be child of remote span if traceparent was valid
    ctx, span := lumberjack.StartSpan(ctx, "http-request")
    defer span.End()

    lumberjack.InfoContext(ctx, "Handling request",
        "method", r.Method,
        "path", r.URL.Path,
        "traceparent", r.Header.Get("traceparent"),
    )

    // ... handle request
}
```

### Traceparent Format

The W3C traceparent format is: `version-traceid-spanid-flags`
- **version**: Currently only "00" is supported
- **traceid**: 32 hex characters (128-bit trace ID)
- **spanid**: 16 hex characters (64-bit span ID of parent)
- **flags**: 2 hex characters ("01" = sampled, "00" = not sampled)

Example: `00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`

## Custom Exporters

The SDK supports custom OpenTelemetry exporters for logs, spans, and metrics:

### Custom Logs Exporter

```go
// Implement the LogsExporter interface
type CustomLogsExporter struct{}

func (e *CustomLogsExporter) Export(entry lumberjack.LogEntry) {
    // Send logs to your custom destination
    fmt.Printf("[CUSTOM] %s: %s\n", entry.Lvl, entry.Msg)
}

func (e *CustomLogsExporter) Shutdown(ctx context.Context) error {
    // Cleanup resources
    return nil
}
```

### Using Custom Exporters

```go
import (
    "github.com/TreebeardHQ/go-sdk"
    "go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
    "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
)

func main() {
    // Create OpenTelemetry exporters
    traceExporter, _ := stdouttrace.New(stdouttrace.WithPrettyPrint())
    metricExporter, _ := stdoutmetric.New()
    logsExporter := &CustomLogsExporter{}

    // Configure SDK with custom exporters
    config := lumberjack.NewConfig().
        WithProjectName("my-project").
        WithCustomSpanExporter(traceExporter).
        WithCustomMetricsExporter(metricExporter).
        WithCustomLogsExporter(logsExporter)

    sdk := lumberjack.Init(config)
    defer sdk.Shutdown(context.Background())

    // Use the SDK normally - data goes to your custom exporters
    ctx, span := lumberjack.StartSpan(context.Background(), "operation")
    defer span.End()

    lumberjack.InfoContext(ctx, "Using custom exporters")
}
```

### Available Exporter Types

- **Spans**: Any `sdktrace.SpanExporter` (Jaeger, OTLP, stdout, etc.)
- **Metrics**: Any `sdkmetric.Exporter` (Prometheus, OTLP, stdout, etc.)  
- **Logs**: Custom `LogsExporter` interface for flexible log handling

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
