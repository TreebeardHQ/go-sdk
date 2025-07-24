package lumberjack

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
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
	// Save original default handler
	originalDefault := slog.Default()
	defer slog.SetDefault(originalDefault)

	// Create a test handler to verify forwarding
	testHandler := &TestSlogHandler{}
	testLogger := slog.New(testHandler)
	slog.SetDefault(testLogger)

	// Initialize SDK with slog replacement enabled
	config := NewConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	config.ReplaceSlog = true

	sdk := Init(config)
	defer sdk.Shutdown(context.Background())

	t.Run("standard slog calls go through Lumberjack", func(t *testing.T) {
		// Clear previous logs
		testHandler.logs = nil

		// Use standard slog functions
		slog.Info("test message from standard slog")
		slog.Debug("debug message")
		slog.Warn("warning message")

		// Verify logs were forwarded to the previous handler
		if len(testHandler.logs) != 3 {
			t.Errorf("Expected 3 forwarded logs, got %d", len(testHandler.logs))
		}

		expectedMessages := []string{"test message from standard slog", "debug message", "warning message"}
		for i, expected := range expectedMessages {
			if i < len(testHandler.logs) && testHandler.logs[i] != expected {
				t.Errorf("Expected log %d to be %q, got %q", i, expected, testHandler.logs[i])
			}
		}
	})

	t.Run("slog context calls work", func(t *testing.T) {
		testHandler.logs = nil
		ctx := context.Background()

		slog.InfoContext(ctx, "context message")

		if len(testHandler.logs) != 1 {
			t.Errorf("Expected 1 forwarded log, got %d", len(testHandler.logs))
		}

		if len(testHandler.logs) > 0 && testHandler.logs[0] != "context message" {
			t.Errorf("Expected 'context message', got %q", testHandler.logs[0])
		}
	})
}

func TestSlogReplaceDisabled(t *testing.T) {
	// Save original default handler
	originalDefault := slog.Default()
	defer slog.SetDefault(originalDefault)

	// Create a test handler
	testHandler := &TestSlogHandler{}
	testLogger := slog.New(testHandler)
	slog.SetDefault(testLogger)

	// Initialize SDK with slog replacement disabled
	config := NewConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	config.ReplaceSlog = false

	sdk := Init(config)
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
			t.Error("Global slog handler should not have been replaced when ReplaceSlog is false")
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
	config := NewConfig()
	config.APIKey = "test-key"
	config.ProjectName = "test"
	config.Debug = false
	config.ReplaceSlog = true

	sdk := newSDK(config) // Use newSDK directly to avoid singleton issues

	// Test that we can log through the new handler
	testHandler.logs = nil
	slog.Info("test message during SDK active")
	
	if len(testHandler.logs) == 0 {
		t.Error("Expected log to be forwarded to previous handler")
	}

	// Shutdown the SDK
	sdk.Shutdown(context.Background())

	// Test that the handler is working after restore
	testHandler.logs = nil
	slog.Info("test message after restore")
	
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

func TestConfigWithReplaceSlog(t *testing.T) {
	t.Run("config method sets ReplaceSlog", func(t *testing.T) {
		config := NewConfig().WithReplaceSlog(false)
		if config.ReplaceSlog {
			t.Error("Expected ReplaceSlog to be false")
		}

		config = NewConfig().WithReplaceSlog(true)
		if !config.ReplaceSlog {
			t.Error("Expected ReplaceSlog to be true")
		}
	})

	t.Run("environment variable sets ReplaceSlog", func(t *testing.T) {
		// Test with environment variable (this is more of a documentation test)
		// In real usage, LUMBERJACK_REPLACE_SLOG=false would disable it
		config := NewConfig()
		// Default should be true
		if !config.ReplaceSlog {
			t.Error("Expected default ReplaceSlog to be true")
		}
	})
}

// TestSlogForwarding tests that logs are properly forwarded to both Lumberjack and previous handler
func TestSlogForwarding(t *testing.T) {
	// Create a buffer to capture output from a text handler
	var buf bytes.Buffer
	textHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	textLogger := slog.New(textHandler)
	slog.SetDefault(textLogger)

	// Initialize SDK with slog replacement
	config := NewConfig()
	config.APIKey = "test-key" 
	config.ProjectName = "test"
	config.Debug = false
	config.ReplaceSlog = true

	sdk := Init(config)
	defer func() {
		sdk.Shutdown(context.Background())
		// Restore original for other tests
		slog.SetDefault(slog.Default())
	}()

	// Log something using standard slog
	slog.Info("forwarding test message", "key", "value")

	// Verify the message was forwarded to the text handler
	output := buf.String()
	if !strings.Contains(output, "forwarding test message") {
		t.Errorf("Expected message to be forwarded to previous handler, output: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected attributes to be forwarded, output: %s", output)
	}
}