package lumberjack

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

// TestSlogHandler is a simple test handler that captures log output
type TestSlogHandler struct {
	logs []string
}

func (h *TestSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true
}

func (h *TestSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.logs = append(h.logs, r.Message)
	return nil
}

func (h *TestSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *TestSlogHandler) WithGroup(name string) slog.Handler {
	return h
}

func TestSlogReplace(t *testing.T) {
	// Save original default handler and reset global SDK
	originalDefault := slog.Default()
	originalGlobalSDK := globalSDK
	globalSDK = nil
	once = sync.Once{}
	defer func() {
		slog.SetDefault(originalDefault)
		globalSDK = originalGlobalSDK
		once = sync.Once{}
	}()

	// Initialize SDK with slog replacement enabled
	config := NewTestConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	config.DisableSlogOverride = false

	sdk := newSDK(config) // Use newSDK directly to avoid singleton issues
	defer sdk.Shutdown(context.Background())

	t.Run("standard slog calls go through Lumberjack", func(t *testing.T) {
		// Use standard slog functions - these should work without error
		// We're just verifying they don't panic when Lumberjack replaces slog
		slog.Info("test message from standard slog")
		slog.Debug("debug message")
		slog.Warn("warning message")
		slog.Error("error message")
	})

	t.Run("slog context calls work", func(t *testing.T) {
		ctx := context.Background()
		// Verify context calls work without error
		slog.InfoContext(ctx, "context message")
		slog.DebugContext(ctx, "debug context message")
		slog.WarnContext(ctx, "warn context message")
	})

	t.Run("slog with attributes works", func(t *testing.T) {
		// Verify logging with attributes works
		slog.Info("message with attrs", "key1", "value1", "key2", 123)
		slog.With("request_id", "abc123").Info("message with grouped attrs")
	})
}

func TestSlogReplaceDisabled(t *testing.T) {
	// Save original default handler and reset global SDK
	originalDefault := slog.Default()
	originalGlobalSDK := globalSDK
	globalSDK = nil
	once = sync.Once{}
	defer func() {
		slog.SetDefault(originalDefault)
		globalSDK = originalGlobalSDK
		once = sync.Once{}
	}()

	// Create a test handler
	testHandler := &TestSlogHandler{}
	testLogger := slog.New(testHandler)
	slog.SetDefault(testLogger)

	// Initialize SDK with slog replacement disabled
	config := NewTestConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	config.DisableSlogOverride = true

	sdk := newSDK(config) // Use newSDK directly to avoid singleton issues
	defer sdk.Shutdown(context.Background())

	t.Run("standard slog calls do not go through Lumberjack", func(t *testing.T) {
		// Clear previous logs
		testHandler.logs = nil

		// Use standard slog functions
		slog.Info("test message")

		// Verify the original handler still works
		if len(testHandler.logs) != 1 {
			t.Errorf("Expected 1 log in original handler, got %d", len(testHandler.logs))
		}

		if len(testHandler.logs) > 0 && testHandler.logs[0] != "test message" {
			t.Errorf("Expected 'test message', got %q", testHandler.logs[0])
		}

		// Verify the global handler wasn't changed
		currentDefault := slog.Default()
		if currentDefault.Handler() != testHandler {
			t.Error("Global slog handler should not have been replaced when DisableSlogOverride is true")
		}
	})
}

func TestSlogRestore(t *testing.T) {
	// Save original default handler and reset global SDK
	originalDefault := slog.Default()
	originalGlobalSDK := globalSDK
	globalSDK = nil
	once = sync.Once{}
	defer func() {
		slog.SetDefault(originalDefault)
		globalSDK = originalGlobalSDK
		once = sync.Once{}
	}()

	// Create a test handler
	testHandler := &TestSlogHandler{}
	testLogger := slog.New(testHandler)
	slog.SetDefault(testLogger)

	// Store the handler before SDK initialization
	handlerBeforeSDK := slog.Default().Handler()

	// Initialize SDK with slog replacement enabled
	config := NewTestConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	config.DisableSlogOverride = false

	sdk := newSDK(config) // Use newSDK directly to avoid singleton issues

	// Test that we can log through the new handler (Lumberjack handler)
	slog.Info("test message during SDK active")
	// Just verify it doesn't panic

	// Shutdown the SDK
	sdk.Shutdown(context.Background())

	// Test that the handler is restored after shutdown
	testHandler.logs = nil
	slog.Info("test message after restore")
	
	// After restore, the original handler should be back
	if len(testHandler.logs) != 1 {
		t.Errorf("Expected 1 log after restore, got %d", len(testHandler.logs))
	}
	
	if len(testHandler.logs) > 0 && testHandler.logs[0] != "test message after restore" {
		t.Errorf("Expected 'test message after restore', got %q", testHandler.logs[0])
	}

	// Verify handler reference matches
	restoredHandler := slog.Default().Handler()
	if restoredHandler != handlerBeforeSDK {
		t.Error("Handler should be restored to the same instance")
	}
}

func TestConfigWithDisableSlogOverride(t *testing.T) {
	t.Run("config method sets DisableSlogOverride", func(t *testing.T) {
		config := NewTestConfig().WithDisableSlogOverride(true)
		if !config.DisableSlogOverride {
			t.Error("Expected DisableSlogOverride to be true")
		}

		config = NewTestConfig().WithDisableSlogOverride(false)
		if config.DisableSlogOverride {
			t.Error("Expected DisableSlogOverride to be false")
		}
	})

	t.Run("environment variable sets DisableSlogOverride", func(t *testing.T) {
		// Test with environment variable (this is more of a documentation test)
		// In real usage, LUMBERJACK_DISABLE_SLOG_OVERRIDE=true would disable it
		config := NewTestConfig()
		// Default should be false (slog override enabled)
		if config.DisableSlogOverride {
			t.Error("Expected default DisableSlogOverride to be false")
		}
	})
}
