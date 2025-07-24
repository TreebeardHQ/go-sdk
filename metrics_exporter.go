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
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MetricPoint represents a single metric data point
type MetricPoint struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "counter", "gauge", "histogram"
	Value       interface{}       `json:"value"`
	Timestamp   int64             `json:"timestamp"`
	Unit        string            `json:"unit,omitempty"`
	Description string            `json:"description,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// HistogramValue represents histogram metric data
type HistogramValue struct {
	Count   uint64    `json:"count"`
	Sum     float64   `json:"sum"`
	Min     float64   `json:"min,omitempty"`
	Max     float64   `json:"max,omitempty"`
	Buckets []Bucket  `json:"buckets,omitempty"`
}

// Bucket represents a histogram bucket
type Bucket struct {
	UpperBound float64 `json:"upper_bound"`
	Count      uint64  `json:"count"`
}

// MetricsBatchRequest represents the payload sent to /metrics/batch
type MetricsBatchRequest struct {
	Type    string               `json:"type"`
	Env     string               `json:"env"`
	Ts      int64                `json:"ts"`
	Payload MetricsBatchPayload  `json:"payload"`
}

type MetricsBatchPayload struct {
	Metrics     []MetricPoint `json:"metrics"`
	ProjectId   string        `json:"projectId,omitempty"`
	ReleaseId   string        `json:"releaseId,omitempty"`
	ReleaseType string        `json:"releaseType,omitempty"`
}

type MetricsExporter struct {
	config      *Config
	client      *http.Client
	batch       []MetricPoint
	batchMu     sync.Mutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
	flushTicker *time.Ticker
}

func NewMetricsExporter(config *Config) *MetricsExporter {
	exporter := &MetricsExporter{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		batch:  make([]MetricPoint, 0, config.BatchSize),
		stopCh: make(chan struct{}),
	}
	
	exporter.flushTicker = time.NewTicker(config.BatchTimeout)
	exporter.wg.Add(1)
	go exporter.runFlusher()
	
	return exporter
}

func (e *MetricsExporter) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (e *MetricsExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

func (e *MetricsExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			points := e.convertMetric(m)
			
			e.batchMu.Lock()
			e.batch = append(e.batch, points...)
			shouldFlush := len(e.batch) >= e.config.BatchSize
			e.batchMu.Unlock()
			
			if shouldFlush {
				e.flush()
			}
		}
	}
	
	return nil
}

func (e *MetricsExporter) convertMetric(m metricdata.Metrics) []MetricPoint {
	var points []MetricPoint
	
	switch data := m.Data.(type) {
	case metricdata.Gauge[int64]:
		for _, dp := range data.DataPoints {
			points = append(points, MetricPoint{
				Name:        m.Name,
				Type:        "gauge",
				Value:       dp.Value,
				Timestamp:   dp.Time.UnixMilli(),
				Unit:        m.Unit,
				Description: m.Description,
				Attributes:  convertAttributes(dp.Attributes),
			})
		}
		
	case metricdata.Gauge[float64]:
		for _, dp := range data.DataPoints {
			points = append(points, MetricPoint{
				Name:        m.Name,
				Type:        "gauge", 
				Value:       dp.Value,
				Timestamp:   dp.Time.UnixMilli(),
				Unit:        m.Unit,
				Description: m.Description,
				Attributes:  convertAttributes(dp.Attributes),
			})
		}
		
	case metricdata.Sum[int64]:
		for _, dp := range data.DataPoints {
			points = append(points, MetricPoint{
				Name:        m.Name,
				Type:        "counter",
				Value:       dp.Value,
				Timestamp:   dp.Time.UnixMilli(),
				Unit:        m.Unit,
				Description: m.Description,
				Attributes:  convertAttributes(dp.Attributes),
			})
		}
		
	case metricdata.Sum[float64]:
		for _, dp := range data.DataPoints {
			points = append(points, MetricPoint{
				Name:        m.Name,
				Type:        "counter",
				Value:       dp.Value,
				Timestamp:   dp.Time.UnixMilli(),
				Unit:        m.Unit,
				Description: m.Description,
				Attributes:  convertAttributes(dp.Attributes),
			})
		}
		
	case metricdata.Histogram[int64]:
		for _, dp := range data.DataPoints {
			buckets := make([]Bucket, len(dp.Bounds))
			for i, bound := range dp.Bounds {
				count := uint64(0)
				if i < len(dp.BucketCounts) {
					count = dp.BucketCounts[i]
				}
				buckets[i] = Bucket{
					UpperBound: bound,
					Count:      count,
				}
			}
			
			histValue := HistogramValue{
				Count:   dp.Count,
				Sum:     float64(dp.Sum),
				Buckets: buckets,
			}
			
			if min, hasMin := dp.Min.Value(); hasMin {
				minFloat := float64(min)
				histValue.Min = minFloat
			}
			if max, hasMax := dp.Max.Value(); hasMax {
				maxFloat := float64(max)
				histValue.Max = maxFloat
			}
			
			points = append(points, MetricPoint{
				Name:        m.Name,
				Type:        "histogram",
				Value:       histValue,
				Timestamp:   dp.Time.UnixMilli(),
				Unit:        m.Unit,
				Description: m.Description,
				Attributes:  convertAttributes(dp.Attributes),
			})
		}
		
	case metricdata.Histogram[float64]:
		for _, dp := range data.DataPoints {
			buckets := make([]Bucket, len(dp.Bounds))
			for i, bound := range dp.Bounds {
				count := uint64(0)
				if i < len(dp.BucketCounts) {
					count = dp.BucketCounts[i]
				}
				buckets[i] = Bucket{
					UpperBound: bound,
					Count:      count,
				}
			}
			
			histValue := HistogramValue{
				Count:   dp.Count,
				Sum:     dp.Sum,
				Buckets: buckets,
			}
			
			if min, hasMin := dp.Min.Value(); hasMin {
				histValue.Min = min
			}
			if max, hasMax := dp.Max.Value(); hasMax {
				histValue.Max = max
			}
			
			points = append(points, MetricPoint{
				Name:        m.Name,
				Type:        "histogram",
				Value:       histValue,
				Timestamp:   dp.Time.UnixMilli(),
				Unit:        m.Unit,
				Description: m.Description,
				Attributes:  convertAttributes(dp.Attributes),
			})
		}
	}
	
	return points
}

func convertAttributes(attrs attribute.Set) map[string]string {
	result := make(map[string]string)
	for _, kv := range attrs.ToSlice() {
		result[string(kv.Key)] = kv.Value.AsString()
	}
	return result
}

func (e *MetricsExporter) runFlusher() {
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

func (e *MetricsExporter) flush() {
	e.batchMu.Lock()
	if len(e.batch) == 0 {
		e.batchMu.Unlock()
		return
	}
	
	metrics := make([]MetricPoint, len(e.batch))
	copy(metrics, e.batch)
	e.batch = e.batch[:0]
	e.batchMu.Unlock()
	
	e.sendBatch(metrics)
}

func (e *MetricsExporter) sendBatch(metrics []MetricPoint) {
	env := "production"
	if e.config.Debug {
		env = "development"
	}
	
	payload := MetricsBatchPayload{
		Metrics: metrics,
	}
	
	if releaseId := os.Getenv("LUMBERJACK_RELEASE_ID"); releaseId != "" {
		payload.ReleaseId = releaseId
	}
	
	if releaseType := os.Getenv("LUMBERJACK_RELEASE_TYPE"); releaseType != "" {
		payload.ReleaseType = releaseType
	}
	
	request := MetricsBatchRequest{
		Type:    "metric_batch",
		Env:     env,
		Ts:      time.Now().UnixMilli(),
		Payload: payload,
	}
	
	data, err := json.Marshal(request)
	if err != nil {
		if e.config.Debug {
			fmt.Printf("Failed to marshal metrics: %v\n", err)
		}
		return
	}
	
	e.sendWithRetry(data)
}

func (e *MetricsExporter) sendWithRetry(data []byte) {
	url := fmt.Sprintf("%s/metrics/batch", e.config.BaseURL)
	retries := 0
	backoff := e.config.RetryBackoff
	
	for retries <= e.config.MaxRetries {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
		if err != nil {
			if e.config.Debug {
				fmt.Printf("Failed to create metrics request: %v\n", err)
			}
			return
		}
		
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.config.APIKey)
		
		resp, err := e.client.Do(req)
		if err != nil {
			if e.config.Debug {
				fmt.Printf("Failed to send metrics (attempt %d): %v\n", retries+1, err)
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
				var request MetricsBatchRequest
				json.Unmarshal(data, &request)
				fmt.Printf("Successfully sent %d metrics\n", len(request.Payload.Metrics))
			}
			return
		}
		
		if e.config.Debug {
			fmt.Printf("Failed to send metrics, status: %d\n", resp.StatusCode)
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
		fmt.Printf("Max retries exceeded for metrics batch\n")
	}
}

func (e *MetricsExporter) ForceFlush(ctx context.Context) error {
	e.flush()
	return nil
}

func (e *MetricsExporter) Shutdown(ctx context.Context) error {
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