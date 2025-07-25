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
	"runtime"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
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

func (e *DefaultLogsExporter) Export(entry LogEntry) {
	e.batchMu.Lock()
	e.batch = append(e.batch, entry)
	shouldFlush := len(e.batch) >= e.config.BatchSize
	e.batchMu.Unlock()

	if shouldFlush {
		e.flush()
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

type LumberjackHandler struct {
	exporter        LogsExporter
	attrs           []slog.Attr
	groups          []string
	projectName     string
	previousHandler slog.Handler // For chaining to previous handler
}

func NewLumberjackHandler(exporter LogsExporter, projectName string) *LumberjackHandler {
	return &LumberjackHandler{
		exporter:    exporter,
		projectName: projectName,
	}
}

func NewLumberjackHandlerWithChain(exporter LogsExporter, projectName string, previousHandler slog.Handler) *LumberjackHandler {
	return &LumberjackHandler{
		exporter:        exporter,
		projectName:     projectName,
		previousHandler: previousHandler,
	}
}

func (h *LumberjackHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true
}

func (h *LumberjackHandler) Handle(ctx context.Context, r slog.Record) error {
	entry := LogEntry{
		Msg: r.Message,
		Lvl: levelToString(r.Level),
		Ts:  float64(r.Time.UnixNano()) / 1e9,
		Src: "lumberjack-go",
	}

	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			entry.Fl = f.File
			entry.Ln = f.Line
		}
	}

	props := make(map[string]interface{})

	for _, attr := range h.attrs {
		props[attr.Key] = attr.Value.Any()
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

	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "trace_id":
			entry.Tid = a.Value.String()
		case "span_id", "sampled":
		default:
			props[a.Key] = a.Value.Any()
		}
		return true
	})

	if len(props) > 0 {
		entry.Props = props
	}

	h.exporter.Export(entry)

	if h.previousHandler != nil {

		return h.previousHandler.Handle(ctx, r)

	}

	return nil
}

func (h *LumberjackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LumberjackHandler{
		exporter:        h.exporter,
		attrs:           append(h.attrs, attrs...),
		groups:          h.groups,
		projectName:     h.projectName,
		previousHandler: h.previousHandler,
	}
}

func (h *LumberjackHandler) WithGroup(name string) slog.Handler {
	return &LumberjackHandler{
		exporter:        h.exporter,
		attrs:           h.attrs,
		groups:          append(h.groups, name),
		projectName:     h.projectName,
		previousHandler: h.previousHandler,
	}
}

func levelToString(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	case level >= slog.LevelError:
		return "error"
	default:
		return "info"
	}
}
