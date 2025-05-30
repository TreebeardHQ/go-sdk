package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/treebeard/go-sdk"
	"github.com/treebeard/go-sdk/pkg/core"
	"github.com/treebeard/go-sdk/pkg/logger"
)

func main() {
	// Initialize Treebeard SDK
	treebeard.Init(core.Config{
		ProjectName:    "go-sdk-example",
		APIKey:         "", // Will use TREEBEARD_API_KEY env var
		LogToStdout:    true,
		StdoutLogLevel: logger.InfoLevel,
		BatchSize:      10,
		BatchAge:       2 * time.Second,
		FlushInterval:  10 * time.Second,
	})

	// Ensure logs are sent before program exits
	defer treebeard.Close()

	// Basic logging
	fmt.Println("=== Basic Logging ===")
	treebeard.Info("Application started")
	treebeard.Debug("This is a debug message")
	treebeard.Warn("This is a warning", map[string]interface{}{
		"component": "example",
		"user_id":   12345,
	})

	// Context-based logging
	fmt.Println("\n=== Context-based Logging ===")
	ctx := context.Background()
	
	// Start a trace
	traceCtx := treebeard.StartTrace(ctx, "example_operation", map[string]interface{}{
		"operation_type": "demo",
		"version":        "1.0.0",
	})

	treebeard.InfoWithContext(traceCtx, "Processing user request", map[string]interface{}{
		"request_id": "req-123",
		"user_id":    12345,
	})

	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	treebeard.DebugWithContext(traceCtx, "Intermediate step completed")

	// Complete the trace successfully
	treebeard.CompleteTraceSuccess(traceCtx, map[string]interface{}{
		"processed_items": 42,
		"cache_hits":      38,
	})

	// Function tracing
	fmt.Println("\n=== Function Tracing ===")
	
	// Wrap a function with automatic tracing
	processDataFunc := treebeard.TraceFunc("process_data", func(ctx context.Context) error {
		treebeard.InfoWithContext(ctx, "Starting data processing")
		
		// Simulate processing
		time.Sleep(50 * time.Millisecond)
		
		treebeard.InfoWithContext(ctx, "Data processing step 1 complete")
		time.Sleep(25 * time.Millisecond)
		treebeard.InfoWithContext(ctx, "Data processing step 2 complete")
		
		return nil
	})

	// Execute the traced function
	if err := processDataFunc(ctx); err != nil {
		treebeard.Error("Data processing failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Function tracing with results
	fmt.Println("\n=== Function Tracing with Results ===")
	
	calculateFunc := treebeard.TraceFuncWithResult("calculate_result", func(ctx context.Context) (interface{}, error) {
		treebeard.InfoWithContext(ctx, "Starting calculation")
		
		// Simulate calculation
		time.Sleep(30 * time.Millisecond)
		result := 42
		
		treebeard.InfoWithContext(ctx, "Calculation complete", map[string]interface{}{
			"result": result,
		})
		
		return result, nil
	})

	result, err := calculateFunc(ctx)
	if err != nil {
		treebeard.Error("Calculation failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		treebeard.Info("Got calculation result", map[string]interface{}{
			"result": result,
		})
	}

	// Error handling and tracing
	fmt.Println("\n=== Error Handling ===")
	
	errorFunc := treebeard.TraceFunc("operation_with_error", func(ctx context.Context) error {
		treebeard.InfoWithContext(ctx, "Starting operation that will fail")
		
		// Simulate an error
		time.Sleep(20 * time.Millisecond)
		
		return errors.New("simulated processing error")
	})

	if err := errorFunc(ctx); err != nil {
		// The error is automatically logged by the trace wrapper
		fmt.Printf("Operation failed as expected: %v\n", err)
	}

	// Manual error trace completion
	fmt.Println("\n=== Manual Error Trace ===")
	errorTraceCtx := treebeard.StartTrace(ctx, "manual_error_handling")
	
	treebeard.InfoWithContext(errorTraceCtx, "Starting risky operation")
	
	// Simulate error
	err = errors.New("manual error example")
	if err != nil {
		treebeard.CompleteTraceError(errorTraceCtx, err, map[string]interface{}{
			"error_code":    "E001",
			"retry_count":   3,
			"last_attempt": time.Now().Unix(),
		})
	}

	fmt.Println("\n=== Flushing logs ===")
	treebeard.Info("Example completed, flushing remaining logs...")
	
	// Force flush any remaining logs
	treebeard.Flush()
	
	// Give time for async operations to complete
	time.Sleep(500 * time.Millisecond)
	
	fmt.Println("Example finished!")
}