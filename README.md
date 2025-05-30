# Treebeard Go SDK

A Go SDK for collecting and transferring logs to TreebeardHQ. This SDK provides structured logging with automatic trace management, batching, and HTTP delivery.

## Features

- **Structured Logging**: JSON-formatted log entries with multiple levels
- **Trace Management**: Automatic trace ID generation and context propagation
- **Batch Collection**: Efficient batching of log entries before transmission
- **Caller Information**: Automatic capture of file names, line numbers, and function names
- **Stack Traces**: Automatic stack trace collection for errors
- **Context Support**: Integration with Go's context package
- **Background Flushing**: Automatic periodic flushing of log batches
- **Fallback Logging**: Stdout logging when API key is not configured

## Installation

```bash
go get github.com/treebeard/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/treebeard/go-sdk"
    "github.com/treebeard/go-sdk/pkg/core"
    "github.com/treebeard/go-sdk/pkg/logger"
)

func main() {
    // Initialize the SDK
    treebeard.Init(core.Config{
        ProjectName: "my-project",
        APIKey:      "your-api-key", // or set TREEBEARD_API_KEY env var
        LogToStdout: true,
    })
    defer treebeard.Close()

    // Basic logging
    treebeard.Info("Application started")
    treebeard.Error("Something went wrong", map[string]interface{}{
        "error_code": "E001",
        "user_id":    12345,
    })

    // Trace-based logging
    ctx := context.Background()
    traceCtx := treebeard.StartTrace(ctx, "user_operation")
    
    treebeard.InfoWithContext(traceCtx, "Processing request")
    // ... do work ...
    treebeard.CompleteTraceSuccess(traceCtx)
}
```

## Configuration

The SDK can be configured using the `core.Config` struct or environment variables:

```go
config := core.Config{
    ProjectName:    "my-project",           // Required: Your project identifier
    APIKey:         "your-api-key",         // Or set TREEBEARD_API_KEY
    APIURL:         "https://...",          // Or set TREEBEARD_API_URL (optional)
    BatchSize:      100,                    // Logs per batch (default: 100)
    BatchAge:       5 * time.Second,        // Max time before flush (default: 5s)
    FlushInterval:  30 * time.Second,       // Background flush interval (default: 30s)
    LogToStdout:    true,                   // Also log to stdout (default: false)
    StdoutLogLevel: logger.InfoLevel,       // Minimum level for stdout (default: Info)
}
```

### Environment Variables

- `TREEBEARD_API_KEY`: Your API key
- `TREEBEARD_API_URL`: API endpoint (defaults to production)

## Logging Levels

Available logging levels in order of severity:

```go
treebeard.Debug("Debug message")
treebeard.Info("Info message")
treebeard.Warn("Warning message")
treebeard.Error("Error message")
treebeard.Critical("Critical message")
```

## Trace Management

Traces allow you to group related log entries and track operations:

### Manual Trace Management

```go
ctx := context.Background()
traceCtx := treebeard.StartTrace(ctx, "database_operation", map[string]interface{}{
    "table": "users",
    "operation": "select",
})

treebeard.InfoWithContext(traceCtx, "Executing query")
// ... perform database operation ...

// On success
treebeard.CompleteTraceSuccess(traceCtx, map[string]interface{}{
    "rows_affected": 5,
})

// On error
treebeard.CompleteTraceError(traceCtx, err, map[string]interface{}{
    "query": "SELECT * FROM users",
})
```

### Function Tracing

Automatically trace function execution:

```go
// Trace a function that returns only an error
tracedFunc := treebeard.TraceFunc("process_data", func(ctx context.Context) error {
    treebeard.InfoWithContext(ctx, "Processing data")
    // ... do work ...
    return nil
})

if err := tracedFunc(context.Background()); err != nil {
    // Error is automatically logged
}

// Trace a function that returns a value and error
calculateFunc := treebeard.TraceFuncWithResult("calculate", func(ctx context.Context) (int, error) {
    result := 42
    return result, nil
})

result, err := calculateFunc(context.Background())
```

## Context-Aware Logging

Use context to maintain trace information across function calls:

```go
func handleRequest(ctx context.Context) {
    traceCtx := treebeard.StartTrace(ctx, "handle_request")
    defer treebeard.CompleteTraceSuccess(traceCtx) // or CompleteTraceError on error
    
    processUser(traceCtx)
    validateData(traceCtx)
}

func processUser(ctx context.Context) {
    // This log will include the trace ID from the parent context
    treebeard.InfoWithContext(ctx, "Processing user data")
}
```

## Error Handling and Stack Traces

Errors are automatically enhanced with stack trace information:

```go
// Manual error logging with stack trace
err := errors.New("database connection failed")
treebeard.Error("Database error occurred", map[string]interface{}{
    "error": err.Error(),
    "retry_count": 3,
})

// Automatic stack traces in traced functions
tracedFunc := treebeard.TraceFunc("risky_operation", func(ctx context.Context) error {
    return errors.New("something went wrong")
})

// Stack trace is automatically captured and logged
tracedFunc(context.Background())
```

## Batching and Flushing

The SDK automatically batches logs for efficient transmission:

- Logs are batched up to `BatchSize` entries
- Batches are automatically flushed after `BatchAge` duration
- Background flushing occurs every `FlushInterval`
- Manual flushing: `treebeard.Flush()`
- Final flush on shutdown: `treebeard.Close()`

## Examples

See the [`examples/`](examples/) directory for complete examples:

- [`examples/basic/main.go`](examples/basic/main.go): Comprehensive usage examples

## Development

### Local Development Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/treebeard/go-sdk.git
   cd go-sdk
   ```

2. **Install Go dependencies:**
   ```bash
   make deps
   ```

3. **Run the example:**
   ```bash
   # Basic example with stdout logging
   go run examples/basic/main.go
   
   # With API key for live testing
   TREEBEARD_API_KEY=your-key go run examples/basic/main.go
   ```

4. **Set up your development environment:**
   ```bash
   # Install development tools (optional)
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

### Building

```bash
make build
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests for a specific package
go test -v ./pkg/logger
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run vet
make vet

# Run all checks
make check
```

### Testing Your Changes

1. **Test the basic example:**
   ```bash
   go run examples/basic/main.go
   ```

2. **Test with your own API key:**
   ```bash
   export TREEBEARD_API_KEY=your-api-key
   export TREEBEARD_PROJECT_NAME=your-project
   go run examples/basic/main.go
   ```

3. **Create a test application:**
   ```bash
   mkdir test-app && cd test-app
   go mod init test-app
   go mod edit -replace github.com/treebeard/go-sdk=../
   ```

   Create `main.go`:
   ```go
   package main

   import (
       "github.com/treebeard/go-sdk"
       "github.com/treebeard/go-sdk/pkg/core"
   )

   func main() {
       treebeard.Init(core.Config{
           ProjectName: "test-app",
           LogToStdout: true,
       })
       defer treebeard.Close()

       treebeard.Info("Hello from test app!")
   }
   ```

   ```bash
   go mod tidy
   go run main.go
   ```

### Project Structure

```
go-sdk/
├── pkg/                    # Internal packages
│   ├── batch/             # Log batching functionality
│   ├── context/           # Context management
│   ├── core/              # Main client implementation
│   ├── logger/            # Logging levels and structures
│   └── trace/             # Stack trace utilities
├── examples/              # Usage examples
│   └── basic/            # Basic usage example
├── test/                  # Test files
├── docs/                  # Documentation
├── treebeard.go          # Public API
├── go.mod               # Go module definition
├── Makefile             # Build automation
└── README.md            # This file
```

### Contributing Guidelines

1. **Code Style:**
   - Follow standard Go conventions
   - Use `go fmt` for formatting
   - Add comments for exported functions
   - Keep functions focused and small

2. **Testing:**
   - Write tests for new functionality
   - Maintain or improve test coverage
   - Test both success and error paths

3. **Documentation:**
   - Update README for new features
   - Add examples for complex functionality
   - Comment exported APIs

4. **Pull Requests:**
   - Include tests with your changes
   - Update documentation as needed
   - Ensure all checks pass (`make check`)

### Debugging

Enable debug logging:

```bash
# Set log level to debug
export TREEBEARD_STDOUT_LOG_LEVEL=debug
go run examples/basic/main.go
```

View detailed logs:

```go
treebeard.Init(core.Config{
    ProjectName:    "debug-app",
    LogToStdout:    true,
    StdoutLogLevel: logger.DebugLevel,  // Show all log levels
})
```

## License

MIT License - see [LICENSE](LICENSE) file for details.