package lumberjack

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIKey      string
	BaseURL     string
	Debug       bool
	ProjectName string
	
	BatchSize     int
	BatchTimeout  time.Duration
	MaxRetries    int
	RetryBackoff  time.Duration
}

func NewConfig() *Config {
	debug := false
	if debugStr := os.Getenv("LUMBERJACK_DEBUG"); debugStr != "" {
		debug, _ = strconv.ParseBool(debugStr)
	}
	
	batchSize := 100
	if batchSizeStr := os.Getenv("LUMBERJACK_BATCH_SIZE"); batchSizeStr != "" {
		if size, err := strconv.Atoi(batchSizeStr); err == nil && size > 0 {
			batchSize = size
		}
	}
	
	return &Config{
		APIKey:       os.Getenv("LUMBERJACK_API_KEY"),
		BaseURL:      getEnvOrDefault("LUMBERJACK_BASE_URL", "https://api.lumberjackhq.com"),
		Debug:        debug,
		ProjectName:  os.Getenv("LUMBERJACK_PROJECT_NAME"),
		BatchSize:    batchSize,
		BatchTimeout: 5 * time.Second,
		MaxRetries:   3,
		RetryBackoff: 250 * time.Millisecond,
	}
}

func (c *Config) WithAPIKey(key string) *Config {
	c.APIKey = key
	return c
}

func (c *Config) WithBaseURL(url string) *Config {
	c.BaseURL = url
	return c
}

func (c *Config) WithDebug(debug bool) *Config {
	c.Debug = debug
	return c
}

func (c *Config) WithProjectName(name string) *Config {
	c.ProjectName = name
	return c
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}