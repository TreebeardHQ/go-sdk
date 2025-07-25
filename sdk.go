package lumberjack

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	globalSDK *SDK
	once      sync.Once
)

type SDK struct {
	config               *Config
	logger               *Logger
	tracer               trace.Tracer
	meter                metric.Meter
	spanExporter         sdktrace.SpanExporter
	logsExporter         LogsExporter
	metricsExporter      sdkmetric.Exporter
	tracerProvider       *sdktrace.TracerProvider
	meterProvider        *sdkmetric.MeterProvider
	defaultSpanExporter  *SpanExporter
	defaultLogsExporter  *DefaultLogsExporter
	defaultMetricsExporter *MetricsExporter
}

func Init(config *Config) *SDK {
	once.Do(func() {
		globalSDK = newSDK(config)
	})
	return globalSDK
}

func InitWithConfig(cfg Config) *SDK {
	return Init(&cfg)
}

func Get() *SDK {
	if globalSDK == nil {
		panic("treebeard SDK not initialized. Call Init() first")
	}
	return globalSDK
}

func newSDK(config *Config) *SDK {
	if config == nil {
		config = NewConfig()
	}
	
	if config.APIKey == "" && !config.Debug {
		fmt.Println("Warning: Lumberjack SDK initialized without API key. Logs will only go to stdout.")
	}
	
	var logsExporter LogsExporter
	var defaultLogsExporter *DefaultLogsExporter
	if config.CustomLogsExporter != nil {
		logsExporter = config.CustomLogsExporter
	} else {
		defaultLogsExporter = NewLogsExporter(config)
		logsExporter = defaultLogsExporter
	}
	
	var spanExporter sdktrace.SpanExporter
	var defaultSpanExporter *SpanExporter
	if config.CustomSpanExporter != nil {
		spanExporter = config.CustomSpanExporter
	} else {
		defaultSpanExporter = NewSpanExporter(config)
		spanExporter = defaultSpanExporter
	}
	
	var metricsExporter sdkmetric.Exporter
	var defaultMetricsExporter *MetricsExporter
	if config.CustomMetricsExporter != nil {
		metricsExporter = config.CustomMetricsExporter
	} else {
		defaultMetricsExporter = NewMetricsExporter(config)
		metricsExporter = defaultMetricsExporter
	}
	
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ProjectName),
			semconv.ServiceVersion(os.Getenv("LUMBERJACK_SERVICE_VERSION")),
		),
	)
	if err != nil && config.Debug {
		fmt.Printf("Failed to create resource: %v\n", err)
	}
	
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)
	
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			metricsExporter,
			sdkmetric.WithInterval(30*time.Second),
		)),
	)
	otel.SetMeterProvider(meterProvider)
	
	base := baselineHandler() // <-- CLEAN handler, never Lumberjack

	var handler *LumberjackHandler
	if config.ReplaceSlog {
		// Only wrap once
		if _, already := slog.Default().Handler().(*LumberjackHandler); !already {
			handler = NewLumberjackHandlerWithChain(logsExporter, config.ProjectName, base)
			slog.SetDefault(slog.New(handler))

			if config.CaptureStdLog {
				// std logger -> baseline (so it never re-enters Lumberjack)
				log.SetFlags(0)
				log.SetOutput(slog.NewLogLogger(base, slog.LevelInfo).Writer())
			}
		}
	} else {
		handler = NewLumberjackHandlerWithChain(logsExporter, config.ProjectName, base)
	}
		
	logger := NewLogger(handler)
	
	sdk := &SDK{
		config:                 config,
		logger:                 logger,
		tracer:                 tracerProvider.Tracer("lumberjack"),
		meter:                  meterProvider.Meter("lumberjack"),
		spanExporter:           spanExporter,
		logsExporter:           logsExporter,
		metricsExporter:        metricsExporter,
		tracerProvider:         tracerProvider,
		meterProvider:          meterProvider,
		defaultSpanExporter:    defaultSpanExporter,
		defaultLogsExporter:    defaultLogsExporter,
		defaultMetricsExporter: defaultMetricsExporter,
	}
	
	if config.Debug {
		fmt.Printf("Lumberjack SDK initialized for project: %s\n", config.ProjectName)
	}
	
	return sdk
}

func (s *SDK) Logger() *Logger {
	return s.logger
}

func (s *SDK) Tracer() trace.Tracer {
	return s.tracer
}

func (s *SDK) Meter() metric.Meter {
	return s.meter
}

func (s *SDK) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return s.tracer.Start(ctx, name, opts...)
}

// ContextWithTraceparent creates a context with trace context from W3C traceparent header.
// The traceparent format is: version-traceid-spanid-flags
// Example: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
func (s *SDK) ContextWithTraceparent(ctx context.Context, traceparent string) (context.Context, error) {
	spanCtx, err := parseTraceparent(traceparent)
	if err != nil {
		return ctx, fmt.Errorf("invalid traceparent: %w", err)
	}
	
	// Create context with remote span context
	return trace.ContextWithRemoteSpanContext(ctx, spanCtx), nil
}

// parseTraceparent parses a W3C traceparent header into a SpanContext
func parseTraceparent(traceparent string) (trace.SpanContext, error) {
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return trace.SpanContext{}, fmt.Errorf("traceparent must have 4 parts separated by '-', got %d", len(parts))
	}
	
	// Validate version (must be "00")
	if parts[0] != "00" {
		return trace.SpanContext{}, fmt.Errorf("unsupported traceparent version: %s", parts[0])
	}
	
	// Parse trace ID (32 hex characters)
	if len(parts[1]) != 32 {
		return trace.SpanContext{}, fmt.Errorf("trace ID must be 32 hex characters, got %d", len(parts[1]))
	}
	traceID, err := trace.TraceIDFromHex(parts[1])
	if err != nil {
		return trace.SpanContext{}, fmt.Errorf("invalid trace ID: %w", err)
	}
	
	// Parse span ID (16 hex characters)
	if len(parts[2]) != 16 {
		return trace.SpanContext{}, fmt.Errorf("span ID must be 16 hex characters, got %d", len(parts[2]))
	}
	spanID, err := trace.SpanIDFromHex(parts[2])
	if err != nil {
		return trace.SpanContext{}, fmt.Errorf("invalid span ID: %w", err)
	}
	
	// Parse trace flags (2 hex characters)
	if len(parts[3]) != 2 {
		return trace.SpanContext{}, fmt.Errorf("trace flags must be 2 hex characters, got %d", len(parts[3]))
	}
	var traceFlags trace.TraceFlags
	if parts[3] == "01" {
		traceFlags = trace.FlagsSampled
	}
	
	// Build span context
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: traceFlags,
		Remote:     true,
	})
	
	if !spanCtx.IsValid() {
		return trace.SpanContext{}, fmt.Errorf("created span context is invalid")
	}
	
	return spanCtx, nil
}

func (s *SDK) Shutdown(ctx context.Context) error {
	var errs []error
	
	// Restore previous slog handler if we replaced it
	if s.config.ReplaceSlog && s.config.PreviousSlogHandler != nil {
		restoredLogger := slog.New(s.config.PreviousSlogHandler)
		slog.SetDefault(restoredLogger)
		
		if s.config.Debug {
			fmt.Println("Lumberjack SDK: Restored previous slog handler")
		}
	}
	
	// Only shutdown default exporters if they were created
	if s.defaultLogsExporter != nil {
		if err := s.defaultLogsExporter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown logs exporter: %w", err))
		}
	}
	
	if s.defaultSpanExporter != nil {
		if err := s.defaultSpanExporter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown spans exporter: %w", err))
		}
	}
	
	if s.defaultMetricsExporter != nil {
		if err := s.defaultMetricsExporter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown metrics exporter: %w", err))
		}
	}
	
	if err := s.tracerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
	}
	
	if err := s.meterProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	
	return nil
}

func GetLogger() *Logger {
	return Get().Logger()
}

func Debug(msg string, args ...any) {
	Get().Logger().Debug(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	Get().Logger().DebugContext(ctx, msg, args...)
}

func Info(msg string, args ...any) {
	Get().Logger().Info(msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	Get().Logger().InfoContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	Get().Logger().Warn(msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	Get().Logger().WarnContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	Get().Logger().Error(msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	Get().Logger().ErrorContext(ctx, msg, args...)
}

func With(args ...any) *Logger {
	return Get().Logger().With(args...)
}

func WithGroup(name string) *Logger {
	return Get().Logger().WithGroup(name)
}

func Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	Get().Logger().Log(ctx, level, msg, args...)
}

func LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	Get().Logger().LogAttrs(ctx, level, msg, attrs...)
}

func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Get().StartSpan(ctx, name, opts...)
}

func Tracer() trace.Tracer {
	return Get().Tracer()
}

func Meter() metric.Meter {
	return Get().Meter()
}

func Shutdown(ctx context.Context) error {
	if globalSDK != nil {
		return globalSDK.Shutdown(ctx)
	}
	return nil
}

func baselineHandler() slog.Handler {
	// Anything that writes straight to a file (no slog.Default()) is OK.
	return slog.NewTextHandler(os.Stderr, nil)
}

// ContextWithTraceparent creates a context with trace context from W3C traceparent header.
// This is a package-level convenience function.
func ContextWithTraceparent(ctx context.Context, traceparent string) (context.Context, error) {
	return Get().ContextWithTraceparent(ctx, traceparent)
}