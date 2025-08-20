package lumberjack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)


func TestLocalServerConfiguration(t *testing.T) {
	// Test environment variable configuration
	os.Setenv("LUMBERJACK_LOCAL_SERVER_ENABLED", "true")
	defer func() {
		os.Unsetenv("LUMBERJACK_LOCAL_SERVER_ENABLED")
	}()
	
	config := NewConfig()
	
	if !config.LocalServerEnabled {
		t.Error("Expected LocalServerEnabled to be true")
	}
}

func TestLocalServerConfigurationMethods(t *testing.T) {
	config := NewConfig()
	
	// Test WithLocalServerEnabled
	config.WithLocalServerEnabled(true)
	if !config.LocalServerEnabled {
		t.Error("Expected LocalServerEnabled to be true after WithLocalServerEnabled(true)")
	}
}

func TestServiceDiscovery(t *testing.T) {
	// Test service discovery functionality
	// Note: This test may find an actual config file if one exists
	endpoint := GetLocalServerEndpoint()
	available := IsLocalServerAvailable()
	
	// These should be consistent
	if (endpoint == "") != (!available) {
		t.Errorf("Inconsistent state: endpoint='%s', available=%v", endpoint, available)
	}
	
	// If we found an endpoint, it should be properly formatted (host:port)
	if endpoint != "" {
		if len(endpoint) == 0 {
			t.Error("Endpoint should not be empty string if available")
		}
		// Basic validation - should contain a colon for host:port format
		if !strings.Contains(endpoint, ":") {
			t.Errorf("Endpoint '%s' should contain ':' for host:port format", endpoint)
		}
	}
}

func TestServiceDiscoveryWithMockConfig(t *testing.T) {
	// Create a temporary config file for testing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}
	
	configPath := filepath.Join(homeDir, ".lumberjack.config.test")
	
	// Write test config
	testConfig := `{
		"server_url": "127.0.0.1:8080",
		"grpc_port": 4317,
		"last_heartbeat": 1234567890,
		"ttl_seconds": 300,
		"pid": 12345
	}`
	
	err = os.WriteFile(configPath, []byte(testConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	defer os.Remove(configPath)
	
	// Temporarily rename real config if it exists
	realConfigPath := filepath.Join(homeDir, ".lumberjack.config")
	realConfigExists := false
	if _, err := os.Stat(realConfigPath); err == nil {
		realConfigExists = true
		os.Rename(realConfigPath, realConfigPath+".backup")
	}
	
	// Move test config to real location
	os.Rename(configPath, realConfigPath)
	
	// Clean up function
	defer func() {
		os.Remove(realConfigPath)
		if realConfigExists {
			os.Rename(realConfigPath+".backup", realConfigPath)
		}
	}()
	
	// Test service discovery
	endpoint := GetLocalServerEndpoint()
	expectedEndpoint := "127.0.0.1:4317"
	if endpoint != expectedEndpoint {
		t.Errorf("Expected endpoint '%s', got '%s'", expectedEndpoint, endpoint)
	}
	
	available := IsLocalServerAvailable()
	if !available {
		t.Error("Expected server to be available with valid config")
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"127.0.0.1:8080", "127.0.0.1"},
		{"localhost:8080", "localhost"},
		{"http://localhost:8080", "localhost"},
		{"https://example.com:8080", "example.com"},
		{"localhost", "localhost"},
		{"", ""},
	}
	
	for _, test := range tests {
		result := extractHost(test.input)
		if result != test.expected {
			t.Errorf("extractHost('%s') = '%s', expected '%s'", test.input, result, test.expected)
		}
	}
}

func TestLocalServerExporterCreation(t *testing.T) {
	// Test creating local server exporter
	exporter := NewLocalServerExporter("test-service", 100, 10*time.Second)
	if exporter == nil {
		t.Error("Expected non-nil exporter")
	}
	
	stats := exporter.GetStats()
	if stats["service_name"] != "test-service" {
		t.Errorf("Expected service_name 'test-service', got '%v'", stats["service_name"])
	}
	
	if stats["cache_max_size"] != 100 {
		t.Errorf("Expected cache_max_size 100, got %v", stats["cache_max_size"])
	}
	
	// Clean up
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exporter.Shutdown(ctx)
}

func TestSDKWithLocalServer(t *testing.T) {
	// Test SDK initialization with local server enabled
	config := NewTestConfig().
		WithProjectName("test-project").
		WithLocalServerEnabled(true)
	
	sdk := newSDK(config)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sdk.Shutdown(ctx)
	}()
	
	if sdk == nil {
		t.Error("Expected non-nil SDK")
	}
	
	// Verify the config was applied
	if !sdk.config.LocalServerEnabled {
		t.Error("Expected LocalServerEnabled to be true in SDK config")
	}
}