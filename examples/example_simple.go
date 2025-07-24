package main

import (
	"context"
	"log/slog"
	"time"

	lumberjack "github.com/TreebeardHQ/go-sdk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	config := lumberjack.NewConfig().
		WithAPIKey("your-api-key").
		WithProjectName("my-project").
		WithDebug(true)
	
	sdk := lumberjack.Init(config)
	defer sdk.Shutdown(context.Background())
	
	ctx := context.Background()
	
	lumberjack.Info("Application starting", "version", "1.0.0")
	
	ctx, span := lumberjack.StartSpan(ctx, "example-operation")
	defer span.End()
	
	lumberjack.InfoContext(ctx, "Processing request", "user_id", 123)
	
	time.Sleep(100 * time.Millisecond)
	
	lumberjack.WarnContext(ctx, "Something worth noting", "warning_type", "deprecated_api")
	
	logger := lumberjack.With("component", "database")
	logger.InfoContext(ctx, "Database query executed", "query_time_ms", 45)
	
	lumberjack.LogAttrs(ctx, slog.LevelInfo, "Custom log with attributes",
		slog.String("operation", "user_lookup"),
		slog.Int("count", 5),
	)
	
	span.SetStatus(codes.Ok, "Operation completed successfully")
	
	// Metrics examples
	lumberjack.Info("Recording metrics...")
	
	meter := lumberjack.Meter()
	
	// Create a counter
	requestCounter, _ := meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("1"),
	)
	
	// Create a histogram for latency
	requestDuration, _ := meter.Float64Histogram(
		"http_request_duration",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("s"),
	)
	
	// Create a gauge for active connections
	activeConnections, _ := meter.Int64UpDownCounter(
		"active_connections",
		metric.WithDescription("Number of active connections"),
		metric.WithUnit("1"),
	)
	
	// Record some metrics
	requestCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("method", "GET"),
		attribute.String("status", "200"),
	))
	
	requestDuration.Record(ctx, 0.123, metric.WithAttributes(
		attribute.String("method", "GET"),
		attribute.String("endpoint", "/api/users"),
	))
	
	activeConnections.Add(ctx, 5)
	
	lumberjack.Info("Metrics recorded successfully")
	
	lumberjack.Info("Application shutting down")
}