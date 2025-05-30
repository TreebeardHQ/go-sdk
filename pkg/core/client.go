package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/treebeard/go-sdk/pkg/batch"
	"github.com/treebeard/go-sdk/pkg/logger"
	"github.com/treebeard/go-sdk/pkg/trace"
	tbcontext "github.com/treebeard/go-sdk/pkg/context"
)

const (
	DefaultAPIURL    = "https://api.treebeardhq.com/logs/batch"
	DefaultBatchSize = 100
	DefaultBatchAge  = 5 * time.Second
	DefaultFlushInterval = 30 * time.Second
)

// Config holds configuration for the Treebeard client
type Config struct {
	ProjectName    string
	APIKey         string
	APIURL         string
	BatchSize      int
	BatchAge       time.Duration
	FlushInterval  time.Duration
	LogToStdout    bool
	StdoutLogLevel logger.Level
}

// Client is the main Treebeard client
type Client struct {
	config     Config
	batch      *batch.Batch
	httpClient *http.Client
	mu         sync.RWMutex
	
	// Background flushing
	flushTicker *time.Ticker
	stopCh      chan struct{}
	
	// Singleton pattern
	initialized bool
}

var (
	instance *Client
	once     sync.Once
)

// New creates a new Treebeard client with the given configuration
func New(config Config) *Client {
	once.Do(func() {
		// Set defaults
		if config.APIURL == "" {
			config.APIURL = getEnvOrDefault("TREEBEARD_API_URL", DefaultAPIURL)
		}
		if config.APIKey == "" {
			config.APIKey = os.Getenv("TREEBEARD_API_KEY")
		}
		if config.BatchSize == 0 {
			config.BatchSize = DefaultBatchSize
		}
		if config.BatchAge == 0 {
			config.BatchAge = DefaultBatchAge
		}
		if config.FlushInterval == 0 {
			config.FlushInterval = DefaultFlushInterval
		}

		instance = &Client{
			config:     config,
			batch:      batch.New(config.BatchSize, config.BatchAge),
			httpClient: &http.Client{Timeout: 30 * time.Second},
			stopCh:     make(chan struct{}),
		}

		// Start background flushing
		instance.startBackgroundFlushing()
		instance.initialized = true

		if config.APIKey != "" {
			log.Printf("Treebeard Go SDK initialized for project: %s", config.ProjectName)
		} else {
			log.Printf("Treebeard Go SDK initialized without API key - logs will go to stdout")
		}
	})

	return instance
}

// GetInstance returns the singleton instance
func GetInstance() *Client {
	if instance == nil {
		return New(Config{})
	}
	return instance
}

// Log creates a log entry with the specified level and message
func (c *Client) Log(level logger.Level, message string, fields ...map[string]interface{}) {
	if !c.initialized {
		return
	}

	// Get caller information (skip this function and the public wrapper)
	file, line, _ := trace.GetFirstNonSDKCaller(1)

	entry := logger.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		File:      file,
		Line:      line,
		Source:    "treebeard",
	}

	// Add any additional fields
	if len(fields) > 0 && fields[0] != nil {
		entry.Properties = fields[0]
	}

	// Add trace information if available from context
	// In Go, we'll use a more explicit trace management approach
	c.addEntry(entry)
}

// LogWithContext logs with a Go context that may contain trace information
func (c *Client) LogWithContext(ctx context.Context, level logger.Level, message string, fields ...map[string]interface{}) {
	if !c.initialized {
		return
	}

	file, line, _ := trace.GetFirstNonSDKCaller(1)

	entry := logger.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		File:      file,
		Line:      line,
		Source:    "treebeard",
		TraceID:   tbcontext.GetTraceID(ctx),
		TraceName: tbcontext.GetTraceName(ctx),
	}

	// Merge context properties with provided fields
	if ctxProps := tbcontext.GetProps(ctx); len(ctxProps) > 0 {
		if entry.Properties == nil {
			entry.Properties = make(map[string]interface{})
		}
		for k, v := range ctxProps {
			entry.Properties[k] = v
		}
	}

	if len(fields) > 0 && fields[0] != nil {
		if entry.Properties == nil {
			entry.Properties = make(map[string]interface{})
		}
		for k, v := range fields[0] {
			entry.Properties[k] = v
		}
	}


	c.addEntry(entry)
}

// addEntry adds an entry to the batch and potentially triggers a flush
func (c *Client) addEntry(entry logger.LogEntry) {
	// Log to stdout if configured
	if c.config.LogToStdout {
		c.logToStdout(entry)
	}

	// If no API key, only log to stdout
	if c.config.APIKey == "" {
		return
	}

	// Add to batch
	if c.batch.Add(entry) {
		go c.flush() // Flush in background to avoid blocking
	}
}

// logToStdout logs the entry to stdout
func (c *Client) logToStdout(entry logger.LogEntry) {
	levelStr := strings.ToUpper(entry.Level)
	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
	
	output := fmt.Sprintf("%s [%s] %s", timestamp, levelStr, entry.Message)
	
	if entry.TraceID != "" {
		output = fmt.Sprintf("%s [%s] [%s] %s", timestamp, levelStr, entry.TraceID, entry.Message)
	}
	
	if entry.File != "" && entry.Line > 0 {
		output += fmt.Sprintf(" (%s:%d)", entry.File, entry.Line)
	}

	if entry.Properties != nil && len(entry.Properties) > 0 {
		if propsJSON, err := json.Marshal(entry.Properties); err == nil {
			output += fmt.Sprintf(" -- %s", string(propsJSON))
		}
	}

	log.Println(output)
}

// Flush sends all batched logs immediately
func (c *Client) Flush() {
	c.flush()
}

func (c *Client) flush() {
	if !c.initialized || c.config.APIKey == "" {
		return
	}

	entries := c.batch.GetEntries()
	if len(entries) == 0 {
		return
	}

	c.sendLogs(entries)
}

// sendLogs sends log entries to the API
func (c *Client) sendLogs(entries []logger.LogEntry) {
	payload := map[string]interface{}{
		"logs":         entries,
		"project_name": c.config.ProjectName,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal logs: %v", err)
		return
	}

	req, err := http.NewRequest("POST", c.config.APIURL, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to send logs: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to send logs, status: %d", resp.StatusCode)
		return
	}

	log.Printf("Successfully sent %d log entries", len(entries))
}

// startBackgroundFlushing starts a goroutine that periodically flushes logs
func (c *Client) startBackgroundFlushing() {
	c.flushTicker = time.NewTicker(c.config.FlushInterval)
	
	go func() {
		for {
			select {
			case <-c.flushTicker.C:
				c.flush()
			case <-c.stopCh:
				c.flushTicker.Stop()
				return
			}
		}
	}()
}

// Close stops background flushing and sends any remaining logs
func (c *Client) Close() {
	if c.stopCh != nil {
		close(c.stopCh)
	}
	c.flush() // Final flush
}

// Convenience methods for different log levels
func (c *Client) Debug(message string, fields ...map[string]interface{}) {
	c.Log(logger.DebugLevel, message, fields...)
}

func (c *Client) Info(message string, fields ...map[string]interface{}) {
	c.Log(logger.InfoLevel, message, fields...)
}

func (c *Client) Warn(message string, fields ...map[string]interface{}) {
	c.Log(logger.WarnLevel, message, fields...)
}

func (c *Client) Error(message string, fields ...map[string]interface{}) {
	c.Log(logger.ErrorLevel, message, fields...)
}

func (c *Client) Critical(message string, fields ...map[string]interface{}) {
	c.Log(logger.CriticalLevel, message, fields...)
}

// Context-aware convenience methods
func (c *Client) DebugWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	c.LogWithContext(ctx, logger.DebugLevel, message, fields...)
}

func (c *Client) InfoWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	c.LogWithContext(ctx, logger.InfoLevel, message, fields...)
}

func (c *Client) WarnWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	c.LogWithContext(ctx, logger.WarnLevel, message, fields...)
}

func (c *Client) ErrorWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	c.LogWithContext(ctx, logger.ErrorLevel, message, fields...)
}

func (c *Client) CriticalWithContext(ctx context.Context, message string, fields ...map[string]interface{}) {
	c.LogWithContext(ctx, logger.CriticalLevel, message, fields...)
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}