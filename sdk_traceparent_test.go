package lumberjack

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestParseTraceparent(t *testing.T) {
	tests := []struct {
		name        string
		traceparent string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid traceparent sampled",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantErr:     false,
		},
		{
			name:        "valid traceparent not sampled",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
			wantErr:     false,
		},
		{
			name:        "invalid version",
			traceparent: "01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantErr:     true,
			errContains: "unsupported traceparent version",
		},
		{
			name:        "missing parts",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
			wantErr:     true,
			errContains: "must have 4 parts",
		},
		{
			name:        "invalid trace ID length",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e473-00f067aa0ba902b7-01",
			wantErr:     true,
			errContains: "trace ID must be 32 hex characters",
		},
		{
			name:        "invalid span ID length",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b-01",
			wantErr:     true,
			errContains: "span ID must be 16 hex characters",
		},
		{
			name:        "invalid trace flags length",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-1",
			wantErr:     true,
			errContains: "trace flags must be 2 hex characters",
		},
		{
			name:        "invalid hex in trace ID",
			traceparent: "00-XYZ92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantErr:     true,
			errContains: "invalid trace ID",
		},
		{
			name:        "all zeros trace ID",
			traceparent: "00-00000000000000000000000000000000-00f067aa0ba902b7-01",
			wantErr:     true,
			errContains: "trace-id can't be all zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spanCtx, err := parseTraceparent(tt.traceparent)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTraceparent() error = nil, wantErr = true")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("parseTraceparent() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			
			if err != nil {
				t.Errorf("parseTraceparent() unexpected error = %v", err)
				return
			}
			
			if !spanCtx.IsValid() {
				t.Errorf("parseTraceparent() returned invalid span context")
			}
			
			// Verify the span context is marked as remote
			if !spanCtx.IsRemote() {
				t.Errorf("parseTraceparent() span context should be marked as remote")
			}
			
			// Check trace flags for sampled traceparent
			if tt.traceparent == "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
				if !spanCtx.IsSampled() {
					t.Errorf("parseTraceparent() expected sampled flag to be true")
				}
			}
		})
	}
}

func TestContextWithTraceparent(t *testing.T) {
	// Initialize SDK for testing with proper config
	config := NewConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	sdk := Init(config)
	defer sdk.Shutdown(context.Background())

	t.Run("valid traceparent creates proper context", func(t *testing.T) {
		ctx := context.Background()
		traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		
		newCtx, err := sdk.ContextWithTraceparent(ctx, traceparent)
		if err != nil {
			t.Fatalf("ContextWithTraceparent() unexpected error = %v", err)
		}
		
		// Extract span context from the new context
		spanCtx := trace.SpanContextFromContext(newCtx)
		if !spanCtx.IsValid() {
			t.Errorf("ContextWithTraceparent() context does not contain valid span context")
		}
		
		if spanCtx.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("ContextWithTraceparent() trace ID = %v, want 4bf92f3577b34da6a3ce929d0e0e4736", spanCtx.TraceID())
		}
		
		if spanCtx.SpanID().String() != "00f067aa0ba902b7" {
			t.Errorf("ContextWithTraceparent() span ID = %v, want 00f067aa0ba902b7", spanCtx.SpanID())
		}
		
		if !spanCtx.IsSampled() {
			t.Errorf("ContextWithTraceparent() expected sampled flag to be true")
		}
	})

	t.Run("invalid traceparent returns original context", func(t *testing.T) {
		ctx := context.Background()
		traceparent := "invalid-traceparent"
		
		newCtx, err := sdk.ContextWithTraceparent(ctx, traceparent)
		if err == nil {
			t.Errorf("ContextWithTraceparent() error = nil, want error")
		}
		
		// Should return original context on error
		if newCtx != ctx {
			t.Errorf("ContextWithTraceparent() should return original context on error")
		}
	})

	t.Run("spans created with traceparent context have correct parent", func(t *testing.T) {
		ctx := context.Background()
		traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		
		newCtx, err := sdk.ContextWithTraceparent(ctx, traceparent)
		if err != nil {
			t.Fatalf("ContextWithTraceparent() unexpected error = %v", err)
		}
		
		// Create a span with the context containing traceparent
		childCtx, span := sdk.StartSpan(newCtx, "test-operation")
		defer span.End()
		
		spanCtx := span.SpanContext()
		if spanCtx.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("Child span trace ID = %v, want 4bf92f3577b34da6a3ce929d0e0e4736", spanCtx.TraceID())
		}
		
		// The child span should have a different span ID
		if spanCtx.SpanID().String() == "00f067aa0ba902b7" {
			t.Errorf("Child span should have a different span ID than parent")
		}
		
		// Verify the context chain
		childSpanCtx := trace.SpanContextFromContext(childCtx)
		if childSpanCtx.TraceID() != spanCtx.TraceID() {
			t.Errorf("Context chain broken: trace IDs don't match")
		}
	})
}

func TestContextWithTraceparentPackageLevel(t *testing.T) {
	// Initialize SDK for testing with proper config
	config := NewConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	Init(config)
	defer Shutdown(context.Background())

	t.Run("package level function works", func(t *testing.T) {
		ctx := context.Background()
		traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		
		newCtx, err := ContextWithTraceparent(ctx, traceparent)
		if err != nil {
			t.Fatalf("ContextWithTraceparent() unexpected error = %v", err)
		}
		
		spanCtx := trace.SpanContextFromContext(newCtx)
		if !spanCtx.IsValid() {
			t.Errorf("ContextWithTraceparent() context does not contain valid span context")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}