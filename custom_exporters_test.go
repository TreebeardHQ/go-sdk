package lumberjack

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// ConsoleLogsExporter is a simple console exporter for logs that implements LogsExporter interface
type ConsoleLogsExporter struct{}

func (e *ConsoleLogsExporter) Export(ctx context.Context, records []*sdklog.Record) error {
	for _, record := range records {
		// Use fmt.Fprintf to stderr to avoid any logging loops
		fmt.Fprintf(os.Stderr, "CONSOLE LOG: %s %s\n", record.SeverityText(), record.Body().String())
	}
	return nil
}

func (e *ConsoleLogsExporter) Shutdown(ctx context.Context) error {
	return nil
}

func TestCustomSpanExporter(t *testing.T) {
	// Create a stdout trace exporter (standard OTEL exporter)
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		t.Fatalf("Failed to create trace exporter: %v", err)
	}
	
	config := NewConfig().
		WithProjectName("test-project").
		WithCustomSpanExporter(traceExporter)
		
	// Create a new SDK instance directly
	sdk := newSDK(config)
	defer sdk.Shutdown(context.Background())
	
	// Test that traces work
	ctx, span := sdk.StartSpan(context.Background(), "test-span")
	time.Sleep(10 * time.Millisecond) // simulate work
	span.End()
	
	// Force flush to see output
	sdk.tracerProvider.ForceFlush(ctx)
}

func TestCustomMetricsExporter(t *testing.T) {
	// Create a stdout metrics exporter (standard OTEL exporter)
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		t.Fatalf("Failed to create metric exporter: %v", err)
	}
	
	config := NewConfig().
		WithProjectName("test-project").
		WithCustomMetricsExporter(metricExporter)
		
	sdk := newSDK(config)
	defer sdk.Shutdown(context.Background())
	
	// Test that metrics work
	meter := sdk.Meter()
	counter, err := meter.Int64Counter("test_counter")
	if err != nil {
		t.Fatalf("Failed to create counter: %v", err)
	}
	
	counter.Add(context.Background(), 1)
	
	// Force flush to see output
	sdk.meterProvider.ForceFlush(context.Background())
}

func TestCustomLogsExporter(t *testing.T) {
	// Create our custom console logs exporter
	logsExporter := &ConsoleLogsExporter{}
	
	config := NewConfig().
		WithProjectName("test-project").
		WithCustomLogsExporter(logsExporter).
		WithCaptureStdLog(true)
		
	sdk := newSDK(config)
	defer sdk.Shutdown(context.Background())
	
	// Test that logs work
	logger := sdk.Logger()
	logger.Info("Test log message", "key", "value")
}



func TestAllCustomExporters(t *testing.T) {
	// Create all custom exporters
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		t.Fatalf("Failed to create trace exporter: %v", err)
	}
	
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		t.Fatalf("Failed to create metric exporter: %v", err)
	}
	
	logsExporter := &ConsoleLogsExporter{}
	
	config := NewConfig().
		WithProjectName("test-project").
		WithCustomSpanExporter(traceExporter).
		WithCustomMetricsExporter(metricExporter).
		WithCustomLogsExporter(logsExporter).
		WithReplaceSlog(false)
		
	sdk := newSDK(config)
	defer sdk.Shutdown(context.Background())
	
	// Test all observability signals
	ctx, span := sdk.StartSpan(context.Background(), "test-operation")
	defer span.End()
	
	logger := sdk.Logger()
	logger.Info("Starting test operation")
	
	meter := sdk.Meter()
	counter, err := meter.Int64Counter("operation_count")
	if err != nil {
		t.Fatalf("Failed to create counter: %v", err)
	}
	counter.Add(ctx, 1)
	
	logger.Info("Test operation completed")
	
	// Force flush to see output
	sdk.tracerProvider.ForceFlush(ctx)
	sdk.meterProvider.ForceFlush(ctx)
}