package lumberjack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServerConfig represents the configuration of a local Lumberjack server
type ServerConfig struct {
	ServerURL       string  `json:"server_url"`
	GRPCPort        int     `json:"grpc_port"`
	LastHeartbeat   float64 `json:"last_heartbeat"`
	TTLSeconds      int     `json:"ttl_seconds"`
	PID             int     `json:"pid"`
}

// GetLocalServerEndpoint reads the ~/.lumberjack.config file and returns the gRPC endpoint
// Returns empty string if no server is available or config file doesn't exist
func GetLocalServerEndpoint() string {
	config, err := readServerConfig()
	if err != nil {
		return ""
	}
	
	if config == nil {
		return ""
	}
	
	// Extract host from server_url and combine with grpc_port
	host := extractHost(config.ServerURL)
	if host == "" {
		return ""
	}
	
	return fmt.Sprintf("%s:%d", host, config.GRPCPort)
}

// IsLocalServerAvailable checks if a local server is available by reading the config file
func IsLocalServerAvailable() bool {
	return GetLocalServerEndpoint() != ""
}

// readServerConfig reads the server configuration from ~/.lumberjack.config
func readServerConfig() (*ServerConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	
	configPath := filepath.Join(homeDir, ".lumberjack.config")
	
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil // File doesn't exist, not an error
	}
	
	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	// Parse JSON
	var config ServerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	return &config, nil
}

// extractHost extracts the host part from a server URL
// Handles formats like "127.0.0.1:8080" -> "127.0.0.1"
// or "localhost:8080" -> "localhost"
func extractHost(serverURL string) string {
	if serverURL == "" {
		return ""
	}
	
	// Remove protocol if present
	if strings.HasPrefix(serverURL, "http://") {
		serverURL = strings.TrimPrefix(serverURL, "http://")
	} else if strings.HasPrefix(serverURL, "https://") {
		serverURL = strings.TrimPrefix(serverURL, "https://")
	}
	
	// Split by colon to get host part
	parts := strings.Split(serverURL, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	
	return serverURL
}