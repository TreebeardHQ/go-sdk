package lumberjack

import "time"

// NewTestConfig returns a configuration optimized for testing with shorter timeouts
func NewTestConfig() *Config {
	config := NewConfig()
	config.BatchTimeout = 100 * time.Millisecond // Much shorter for tests
	config.RetryBackoff = 10 * time.Millisecond  // Faster retries
	return config
}