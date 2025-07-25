package lumberjack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type LogEntry struct {
	Msg   string                 `json:"msg"`
	Lvl   string                 `json:"lvl"`
	Ts    float64                `json:"ts"`
	Props map[string]interface{} `json:"props,omitempty"`
	Tid   string                 `json:"tid,omitempty"`
	Fl    string                 `json:"fl,omitempty"`
	Tb    string                 `json:"tb,omitempty"`
	Ln    int                    `json:"ln,omitempty"`
	Src   string                 `json:"src"`
}

type LogRequest struct {
	Logs        []LogEntry `json:"logs"`
	ProjectName string     `json:"project_name,omitempty"`
	SdkVersion  int        `json:"sdk_version"`
	ReleaseId   string     `json:"release_id,omitempty"`
	ReleaseType string     `json:"release_type,omitempty"`
}

type DefaultLogsExporter struct {
	config      *Config
	client      *http.Client
	batch       []LogEntry
	batchMu     sync.Mutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
	flushTicker *time.Ticker
}

func NewLogsExporter(config *Config) *DefaultLogsExporter {
	exporter := &DefaultLogsExporter{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		batch:  make([]LogEntry, 0, config.BatchSize),
		stopCh: make(chan struct{}),
	}

	exporter.flushTicker = time.NewTicker(config.BatchTimeout)
	exporter.wg.Add(1)
	go exporter.runFlusher()

	return exporter
}

func (e *DefaultLogsExporter) Export(ctx context.Context, records []*sdklog.Record) error {
	// Convert Record to LogEntry
	entries := make([]LogEntry, 0, len(records))
	for _, record := range records {
		entry := e.convertRecordToEntry(record)
		entries = append(entries, entry)
	}

	e.batchMu.Lock()
	e.batch = append(e.batch, entries...)
	shouldFlush := len(e.batch) >= e.config.BatchSize
	e.batchMu.Unlock()

	if shouldFlush {
		e.flush()
	}

	return nil
}

func (e *DefaultLogsExporter) convertRecordToEntry(record *sdklog.Record) LogEntry {
	entry := LogEntry{
		Msg: record.Body().String(),
		Lvl: severityToString(record.Severity()),
		Ts:  float64(record.Timestamp().UnixNano()) / 1e9,
		Src: "lumberjack-go",
	}

	// Extract trace context if available
	if record.TraceID().IsValid() {
		entry.Tid = record.TraceID().String()
	}

	// Convert attributes to props
	props := make(map[string]interface{})
	record.WalkAttributes(func(kv log.KeyValue) bool {
		props[string(kv.Key)] = kv.Value.AsString()
		return true
	})

	if len(props) > 0 {
		entry.Props = props
	}

	// Try to extract file and line info from attributes
	if file, ok := props["file"].(string); ok {
		entry.Fl = file
		delete(props, "file")
	}
	if line, ok := props["line"]; ok {
		if lineInt, err := convertToInt(line); err == nil {
			entry.Ln = lineInt
			delete(props, "line")
		}
	}

	return entry
}

func severityToString(sev log.Severity) string {
	switch {
	case sev >= log.SeverityFatal:
		return "FATAL"
	case sev >= log.SeverityError:
		return "ERROR"
	case sev >= log.SeverityWarn:
		return "WARN"
	case sev >= log.SeverityInfo:
		return "INFO"
	case sev >= log.SeverityDebug:
		return "DEBUG"
	default:
		return "TRACE"
	}
}

func convertToInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		var i int
		_, err := fmt.Sscanf(val, "%d", &i)
		return i, err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func (e *DefaultLogsExporter) runFlusher() {
	defer e.wg.Done()

	for {
		select {
		case <-e.flushTicker.C:
			e.flush()
		case <-e.stopCh:
			return
		}
	}
}

func (e *DefaultLogsExporter) flush() {
	e.batchMu.Lock()
	if len(e.batch) == 0 {
		e.batchMu.Unlock()
		return
	}

	entries := make([]LogEntry, len(e.batch))
	copy(entries, e.batch)
	e.batch = e.batch[:0]
	e.batchMu.Unlock()

	e.sendBatch(entries)
}

func (e *DefaultLogsExporter) sendBatch(entries []LogEntry) {
	request := LogRequest{
		Logs:        entries,
		ProjectName: e.config.ProjectName,
		SdkVersion:  2,
	}

	if releaseId := os.Getenv("LUMBERJACK_RELEASE_ID"); releaseId != "" {
		request.ReleaseId = releaseId
	}

	if releaseType := os.Getenv("LUMBERJACK_RELEASE_TYPE"); releaseType != "" {
		request.ReleaseType = releaseType
	}

	data, err := json.Marshal(request)
	if err != nil {
		if e.config.Debug {
			fmt.Printf("Failed to marshal logs: %v\n", err)
		}
		return
	}

	e.sendWithRetry(data)
}

func (e *DefaultLogsExporter) sendWithRetry(data []byte) {
	url := fmt.Sprintf("%s/logs/batch", e.config.BaseURL)
	retries := 0
	backoff := e.config.RetryBackoff

	for retries <= e.config.MaxRetries {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
		if err != nil {
			if e.config.Debug {
				fmt.Printf("Failed to create request: %v\n", err)
			}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.config.APIKey)

		resp, err := e.client.Do(req)
		if err != nil {
			if e.config.Debug {
				fmt.Printf("Failed to send logs (attempt %d): %v\n", retries+1, err)
			}
			retries++
			if retries <= e.config.MaxRetries {
				jitter := time.Duration(rand.Float64() * float64(backoff))
				time.Sleep(backoff + jitter)
				backoff *= 2
			}
			continue
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			if e.config.Debug {
				var request LogRequest
				json.Unmarshal(data, &request)
				fmt.Printf("Successfully sent %d log entries\n", len(request.Logs))
			}
			return
		}

		if e.config.Debug {
			fmt.Printf("Failed to send logs, status: %d\n", resp.StatusCode)
		}

		if resp.StatusCode >= 500 {
			retries++
			if retries <= e.config.MaxRetries {
				jitter := time.Duration(rand.Float64() * float64(backoff))
				time.Sleep(backoff + jitter)
				backoff *= 2
			}
		} else {
			break
		}
	}

	if e.config.Debug && retries > e.config.MaxRetries {
		fmt.Printf("Max retries exceeded for log batch\n")
	}
}

func (e *DefaultLogsExporter) Shutdown(ctx context.Context) error {
	select {
	case <-e.stopCh:
		// Already shutdown
		return nil
	default:
		close(e.stopCh)
	}

	e.flushTicker.Stop()
	e.flush()

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// LumberjackLogProcessor is an OpenTelemetry log processor that exports to our LogsExporter
type LumberjackLogProcessor struct {
	exporter LogsExporter
}

func NewLumberjackLogProcessor(exporter LogsExporter) *LumberjackLogProcessor {
	return &LumberjackLogProcessor{
		exporter: exporter,
	}
}

func (p *LumberjackLogProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	return p.exporter.Export(ctx, []*sdklog.Record{record})
}

func (p *LumberjackLogProcessor) Shutdown(ctx context.Context) error {
	return p.exporter.Shutdown(ctx)
}

func (p *LumberjackLogProcessor) ForceFlush(ctx context.Context) error {
	// If the exporter supports force flush, call it here
	return nil
}

// CreateLumberjackSlogHandler creates a slog handler that uses OpenTelemetry logging
func CreateLumberjackSlogHandler(loggerProvider *sdklog.LoggerProvider, previousHandler slog.Handler) slog.Handler {
	// Create an OpenTelemetry slog bridge handler
	otelHandler := otelslog.NewHandler("lumberjack-go", otelslog.WithLoggerProvider(loggerProvider))
	
	// If there's a previous handler, we need to chain them
	if previousHandler != nil {
		return &chainedHandler{
			primary:  otelHandler,
			secondary: previousHandler,
		}
	}
	
	return otelHandler
}

type chainedHandler struct {
	primary   slog.Handler
	secondary slog.Handler
}

func (h *chainedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.primary.Enabled(ctx, level) || h.secondary.Enabled(ctx, level)
}

func (h *chainedHandler) Handle(ctx context.Context, record slog.Record) error {
	var primaryErr error
	if h.primary.Enabled(ctx, record.Level) {
		primaryErr = h.primary.Handle(ctx, record)
	}
	
	var secondaryErr error
	if h.secondary != nil && h.secondary.Enabled(ctx, record.Level) {
		secondaryErr = h.secondary.Handle(ctx, record)
	}
	
	// Return primary error if any, otherwise secondary
	if primaryErr != nil {
		return primaryErr
	}
	return secondaryErr
}

func (h *chainedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &chainedHandler{
		primary:   h.primary.WithAttrs(attrs),
		secondary: h.secondary.WithAttrs(attrs),
	}
}

func (h *chainedHandler) WithGroup(name string) slog.Handler {
	return &chainedHandler{
		primary:   h.primary.WithGroup(name),
		secondary: h.secondary.WithGroup(name),
	}
}

