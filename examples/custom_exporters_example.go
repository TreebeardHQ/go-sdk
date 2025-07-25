package main

import (
	"context"
	"log"
	"time"

	lumberjack "github.com/TreebeardHQ/go-sdk"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// CustomConsoleLogsExporter implements the LogsExporter interface
type CustomConsoleLogsExporter struct{}

func (e *CustomConsoleLogsExporter) Export(ctx context.Context, records []*sdklog.Record) error {
	for _, record := range records {
		log.Printf("[CUSTOM LOG] %s: %s", record.SeverityText(), record.Body().String())
	}
	return nil
}

func (e *CustomConsoleLogsExporter) Shutdown(ctx context.Context) error {
	log.Println("Custom logs exporter shut down")
	return nil
}

func mainExampleCustomExporters() {
	// Create custom OTEL exporters
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("Failed to create trace exporter: %v", err)
	}

	metricExporter, err := stdoutmetric.New()
	if err != nil {
		log.Fatalf("Failed to create metric exporter: %v", err)
	}

	logsExporter := &CustomConsoleLogsExporter{}

	// Configure SDK with custom exporters
	config := lumberjack.NewConfig().
		WithProjectName("custom-exporters-example").
		WithCustomSpanExporter(traceExporter).
		WithCustomMetricsExporter(metricExporter).
		WithCustomLogsExporter(logsExporter)

	sdk := lumberjack.Init(config)
	defer sdk.Shutdown(context.Background())

	log.Println("Running example with custom exporters...")

	// Use all observability signals
	ctx, span := sdk.StartSpan(context.Background(), "example-operation")
	defer span.End()

	logger := sdk.Logger()
	logger.Info("Starting example operation", "operation", "custom-exporters")

	meter := sdk.Meter()
	counter, err := meter.Int64Counter("example_operations_total")
	if err != nil {
		log.Fatalf("Failed to create counter: %v", err)
	}

	for i := 0; i < 3; i++ {
		counter.Add(ctx, 1)
		logger.Info("Processing item", "item", i)
		time.Sleep(100 * time.Millisecond)
	}

	logger.Info("Example operation completed", "total_items", 3)

	// Force flush to ensure all data is exported
	// Note: In real usage, the SDK will automatically flush on shutdown
	time.Sleep(100 * time.Millisecond) // Allow time for async operations

	log.Println("Example completed successfully!")
}