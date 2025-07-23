package lumberjack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
	
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

type SpanExporter struct {
	config      *Config
	client      *http.Client
	batch       []InternalSpan
	batchMu     sync.Mutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
	flushTicker *time.Ticker
}

type InternalSpan struct {
	TraceID     string                 `json:"TraceID"`
	SpanID      string                 `json:"SpanID"`
	ParentSpanID string                `json:"ParentSpanID,omitempty"`
	Service     string                 `json:"Service"`
	Name        string                 `json:"Name"`
	Kind        int                    `json:"Kind"`
	StatusCode  int                    `json:"StatusCode"`
	StartTime   string                 `json:"StartTime"`
	EndTime     string                 `json:"EndTime"`
	DurationUS  int64                  `json:"DurationUS"`
	Attributes  map[string]string      `json:"Attributes"`
	Events      []SpanEvent            `json:"Events,omitempty"`
}

type SpanEvent struct {
	TimeUnixNano int64             `json:"timeUnixNano"`
	Name         string            `json:"name"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type SpanBatchRequest struct {
	Type    string                 `json:"type"`
	Env     string                 `json:"env"`
	Ts      int64                  `json:"ts"`
	Payload SpanBatchPayload       `json:"payload"`
}

type SpanBatchPayload struct {
	Spans       []InternalSpan `json:"spans"`
	ProjectId   string         `json:"projectId,omitempty"`
	ReleaseId   string         `json:"releaseId,omitempty"`
	ReleaseType string         `json:"releaseType,omitempty"`
}

func NewSpanExporter(config *Config) *SpanExporter {
	exporter := &SpanExporter{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		batch:  make([]InternalSpan, 0, config.BatchSize),
		stopCh: make(chan struct{}),
	}
	
	exporter.flushTicker = time.NewTicker(config.BatchTimeout)
	exporter.wg.Add(1)
	go exporter.runFlusher()
	
	return exporter
}

func (e *SpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		internalSpan := e.convertSpan(span)
		
		e.batchMu.Lock()
		e.batch = append(e.batch, internalSpan)
		shouldFlush := len(e.batch) >= e.config.BatchSize
		e.batchMu.Unlock()
		
		if shouldFlush {
			e.flush()
		}
	}
	
	return nil
}

func (e *SpanExporter) convertSpan(span sdktrace.ReadOnlySpan) InternalSpan {
	startTime := span.StartTime().Format(time.RFC3339Nano)
	endTime := span.EndTime().Format(time.RFC3339Nano)
	durationUS := span.EndTime().Sub(span.StartTime()).Microseconds()
	
	attributes := make(map[string]string)
	
	serviceName := e.config.ProjectName
	for _, attr := range span.Resource().Attributes() {
		if attr.Key == semconv.ServiceNameKey {
			serviceName = attr.Value.AsString()
		}
		attributes[string(attr.Key)] = attr.Value.AsString()
	}
	
	for _, attr := range span.Attributes() {
		attributes[string(attr.Key)] = attr.Value.AsString()
	}
	
	statusCode := 0
	if span.Status().Code == codes.Error {
		statusCode = 2
	} else if span.Status().Code == codes.Ok {
		statusCode = 1
	}
	
	parentSpanID := ""
	if span.Parent().IsValid() {
		parentSpanID = span.Parent().SpanID().String()
	}
	
	events := make([]SpanEvent, 0, len(span.Events()))
	for _, event := range span.Events() {
		eventAttrs := make(map[string]string)
		for _, attr := range event.Attributes {
			eventAttrs[string(attr.Key)] = attr.Value.AsString()
		}
		
		events = append(events, SpanEvent{
			TimeUnixNano: event.Time.UnixNano(),
			Name:         event.Name,
			Attributes:   eventAttrs,
		})
	}
	
	return InternalSpan{
		TraceID:      span.SpanContext().TraceID().String(),
		SpanID:       span.SpanContext().SpanID().String(),
		ParentSpanID: parentSpanID,
		Service:      serviceName,
		Name:         span.Name(),
		Kind:         int(span.SpanKind()),
		StatusCode:   statusCode,
		StartTime:    startTime,
		EndTime:      endTime,
		DurationUS:   durationUS,
		Attributes:   attributes,
		Events:       events,
	}
}

func (e *SpanExporter) runFlusher() {
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

func (e *SpanExporter) flush() {
	e.batchMu.Lock()
	if len(e.batch) == 0 {
		e.batchMu.Unlock()
		return
	}
	
	spans := make([]InternalSpan, len(e.batch))
	copy(spans, e.batch)
	e.batch = e.batch[:0]
	e.batchMu.Unlock()
	
	e.sendBatch(spans)
}

func (e *SpanExporter) sendBatch(spans []InternalSpan) {
	env := "production"
	if e.config.Debug {
		env = "development"
	}
	
	payload := SpanBatchPayload{
		Spans: spans,
	}
	
	if releaseId := os.Getenv("LUMBERJACK_RELEASE_ID"); releaseId != "" {
		payload.ReleaseId = releaseId
	}
	
	if releaseType := os.Getenv("LUMBERJACK_RELEASE_TYPE"); releaseType != "" {
		payload.ReleaseType = releaseType
	}
	
	request := SpanBatchRequest{
		Type:    "span_batch",
		Env:     env,
		Ts:      time.Now().UnixMilli(),
		Payload: payload,
	}
	
	data, err := json.Marshal(request)
	if err != nil {
		if e.config.Debug {
			fmt.Printf("Failed to marshal spans: %v\n", err)
		}
		return
	}
	
	e.sendWithRetry(data)
}

func (e *SpanExporter) sendWithRetry(data []byte) {
	url := fmt.Sprintf("%s/spans/batch", e.config.BaseURL)
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
				fmt.Printf("Failed to send spans (attempt %d): %v\n", retries+1, err)
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
				var request SpanBatchRequest
				json.Unmarshal(data, &request)
				fmt.Printf("Successfully sent %d spans\n", len(request.Payload.Spans))
			}
			return
		}
		
		if e.config.Debug {
			fmt.Printf("Failed to send spans, status: %d\n", resp.StatusCode)
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
		fmt.Printf("Max retries exceeded for span batch\n")
	}
}

func (e *SpanExporter) Shutdown(ctx context.Context) error {
	close(e.stopCh)
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

func attributesToMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string)
	for _, attr := range attrs {
		m[string(attr.Key)] = attr.Value.AsString()
	}
	return m
}