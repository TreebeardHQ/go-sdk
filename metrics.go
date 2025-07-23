package lumberjack

import (
	"context"
	"runtime"
	"time"
	
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	meter metric.Meter
	
	requestCounter    metric.Int64Counter
	requestDuration   metric.Float64Histogram
	activeRequests    metric.Int64UpDownCounter
	
	memoryUsage       metric.Int64ObservableGauge
	goroutineCount    metric.Int64ObservableGauge
	cpuUsage          metric.Float64ObservableGauge
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{
		meter: meter,
	}
	
	var err error
	
	m.requestCounter, err = meter.Int64Counter(
		"lumberjack.requests",
		metric.WithDescription("Total number of requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	
	m.requestDuration, err = meter.Float64Histogram(
		"lumberjack.request.duration",
		metric.WithDescription("Request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	
	m.activeRequests, err = meter.Int64UpDownCounter(
		"lumberjack.requests.active",
		metric.WithDescription("Number of active requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	
	m.memoryUsage, err = meter.Int64ObservableGauge(
		"lumberjack.memory.usage",
		metric.WithDescription("Memory usage in bytes"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			o.Observe(int64(ms.Alloc))
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	
	m.goroutineCount, err = meter.Int64ObservableGauge(
		"lumberjack.goroutines",
		metric.WithDescription("Number of goroutines"),
		metric.WithUnit("1"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			o.Observe(int64(runtime.NumGoroutine()))
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	
	return m, nil
}

func (m *Metrics) RecordRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.Int("status_code", statusCode),
	}
	
	m.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

func (m *Metrics) IncrementActiveRequests(ctx context.Context) {
	m.activeRequests.Add(ctx, 1)
}

func (m *Metrics) DecrementActiveRequests(ctx context.Context) {
	m.activeRequests.Add(ctx, -1)
}

type RequestTimer struct {
	metrics   *Metrics
	ctx       context.Context
	method    string
	path      string
	startTime time.Time
}

func (m *Metrics) StartRequest(ctx context.Context, method, path string) *RequestTimer {
	m.IncrementActiveRequests(ctx)
	return &RequestTimer{
		metrics:   m,
		ctx:       ctx,
		method:    method,
		path:      path,
		startTime: time.Now(),
	}
}

func (rt *RequestTimer) End(statusCode int) {
	duration := time.Since(rt.startTime)
	rt.metrics.RecordRequest(rt.ctx, rt.method, rt.path, statusCode, duration)
	rt.metrics.DecrementActiveRequests(rt.ctx)
}