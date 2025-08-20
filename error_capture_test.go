package lumberjack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// TestLogsExporter captures logs for testing
type TestLogsExporter struct {
	logs []LogEntry
}

func (e *TestLogsExporter) Export(ctx context.Context, records []*sdklog.Record) error {
	// Convert records to log entries using the same logic as DefaultLogsExporter
	exporter := &DefaultLogsExporter{config: NewTestConfig()}
	for _, record := range records {
		entry := exporter.convertRecordToEntry(record)
		e.logs = append(e.logs, entry)
	}
	return nil
}

func (e *TestLogsExporter) Shutdown(ctx context.Context) error {
	return nil
}

func TestErrorCapture(t *testing.T) {
	// Create test exporter to capture logs
	testExporter := &TestLogsExporter{}
	
	// Initialize SDK with test exporter
	config := NewTestConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.CustomLogsExporter = testExporter
	config.DisableSlogOverride = true // Don't replace slog to avoid circular logging
	
	sdk := newSDK(config) // Use newSDK to avoid singleton issues
	defer sdk.Shutdown(context.Background())
	
	logger := sdk.Logger()
	
	t.Run("captures error with stack trace", func(t *testing.T) {
		testExporter.logs = nil
		
		err := errors.New("test error")
		logger.LogError(err, "operation failed", "operation", "test")
		
		// Force flush to ensure logs are processed
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		sdk.loggerProvider.ForceFlush(ctx)
		
		if len(testExporter.logs) != 1 {
			t.Fatalf("Expected 1 log entry, got %d", len(testExporter.logs))
		}
		
		entry := testExporter.logs[0]
		
		// Check error fields
		if entry.Exv != "test error" {
			t.Errorf("Expected Exv='test error', got %q", entry.Exv)
		}
		
		if entry.Ext != "*errors.errorString" {
			t.Errorf("Expected Ext='*errors.errorString', got %q", entry.Ext)
		}
		
		if entry.Tb == "" {
			t.Error("Expected stack trace in Tb field")
		}
		
		// Check that stack trace contains this test function
		if !strings.Contains(entry.Tb, "TestErrorCapture") {
			t.Errorf("Stack trace should contain TestErrorCapture, got: %s", entry.Tb)
		}
	})
	
	t.Run("captures wrapped error", func(t *testing.T) {
		testExporter.logs = nil
		
		baseErr := errors.New("base error")
		wrappedErr := fmt.Errorf("wrapped: %w", baseErr)
		
		logger.LogError(wrappedErr, "wrapped error test")
		
		// Force flush to ensure logs are processed
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		sdk.loggerProvider.ForceFlush(ctx)
		
		if len(testExporter.logs) != 1 {
			t.Fatalf("Expected 1 log entry, got %d", len(testExporter.logs))
		}
		
		entry := testExporter.logs[0]
		
		if entry.Exv != "wrapped: base error" {
			t.Errorf("Expected full error message, got %q", entry.Exv)
		}
		
		// Check for error_cause in props
		if cause, ok := entry.Props["error_cause"].(string); !ok || cause != "base error" {
			t.Errorf("Expected error_cause='base error', got %v", entry.Props["error_cause"])
		}
	})
	
	// Note: Panic capture testing is more complex due to goroutine isolation
	// The panic capture works as demonstrated in the examples, but testing it
	// requires careful setup to avoid interfering with the test runner
}

func TestErrorInfoExtraction(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		info := ExtractErrorInfo(nil)
		if info != nil {
			t.Error("Expected nil for nil error")
		}
	})
	
	t.Run("simple error", func(t *testing.T) {
		err := errors.New("simple error")
		info := ExtractErrorInfo(err)
		
		if info.Message != "simple error" {
			t.Errorf("Expected message='simple error', got %q", info.Message)
		}
		
		if info.Type != "*errors.errorString" {
			t.Errorf("Expected type='*errors.errorString', got %q", info.Type)
		}
		
		if len(info.StackTrace) == 0 {
			t.Error("Expected stack trace")
		}
	})
	
	t.Run("wrapped error", func(t *testing.T) {
		baseErr := errors.New("base")
		wrappedErr := fmt.Errorf("wrapped: %w", baseErr)
		
		info := ExtractErrorInfo(wrappedErr)
		
		if !info.IsWrapped {
			t.Error("Expected IsWrapped=true")
		}
		
		if info.Cause != "base" {
			t.Errorf("Expected cause='base', got %q", info.Cause)
		}
	})
}