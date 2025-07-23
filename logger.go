package lumberjack

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
	
	"go.opentelemetry.io/otel/trace"
)

type Logger struct {
	handler slog.Handler
	attrs   []slog.Attr
}

func NewLogger(handler slog.Handler) *Logger {
	return &Logger{
		handler: handler,
	}
}

func (l *Logger) With(args ...any) *Logger {
	attrs := make([]slog.Attr, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				attrs = append(attrs, slog.Any(key, args[i+1]))
			}
		}
	}
	return &Logger{
		handler: l.handler,
		attrs:   append(l.attrs, attrs...),
	}
}

func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		handler: l.handler.WithGroup(name),
		attrs:   l.attrs,
	}
}

func (l *Logger) Debug(msg string, args ...any) {
	l.log(context.Background(), slog.LevelDebug, msg, args...)
}

func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, slog.LevelDebug, msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.log(context.Background(), slog.LevelInfo, msg, args...)
}

func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, slog.LevelInfo, msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.log(context.Background(), slog.LevelWarn, msg, args...)
}

func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, slog.LevelWarn, msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.log(context.Background(), slog.LevelError, msg, args...)
}

func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, slog.LevelError, msg, args...)
}

func (l *Logger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !l.handler.Enabled(ctx, level) {
		return
	}
	
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	
	for _, attr := range l.attrs {
		r.AddAttrs(attr)
	}
	
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		r.AddAttrs(
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
		)
		
		if span.SpanContext().IsSampled() {
			r.AddAttrs(slog.Bool("sampled", true))
		}
	}
	
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				r.AddAttrs(slog.Any(key, args[i+1]))
			}
		}
	}
	
	_ = l.handler.Handle(ctx, r)
}

func (l *Logger) LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if !l.handler.Enabled(ctx, level) {
		return
	}
	
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	
	for _, attr := range l.attrs {
		r.AddAttrs(attr)
	}
	
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		r.AddAttrs(
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
		)
		
		if span.SpanContext().IsSampled() {
			r.AddAttrs(slog.Bool("sampled", true))
		}
	}
	
	r.AddAttrs(attrs...)
	
	_ = l.handler.Handle(ctx, r)
}

func (l *Logger) Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	l.log(ctx, level, msg, args...)
}

func (l *Logger) Handler() slog.Handler {
	return l.handler
}

type leveler struct {
	level slog.Level
}

func (l leveler) Level() slog.Level {
	return l.level
}

func parseSlogLevel(s string) (slog.Level, error) {
	var level slog.Level
	var err = level.UnmarshalText([]byte(s))
	if err != nil {
		return level, fmt.Errorf("unrecognized level: %q", s)
	}
	return level, nil
}