package lumberjack

import (
	"context"
	"sync"
	"time"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
)

// LocalServerExporter exports logs to a local Lumberjack server via OTLP gRPC
type LocalServerExporter struct {
	serviceName     string
	cacheMaxSize    int
	discoveryInterval time.Duration
	
	// Server connection state
	otlpExporter    *otlploggrpc.Exporter
	currentEndpoint string
	serverAvailable bool
	
	// Log caching with FIFO eviction
	logCache        []sdklog.Record
	cacheMutex      sync.Mutex
	
	// Background discovery
	stopDiscovery   chan struct{}
	discoveryDone   sync.WaitGroup
	
	// Statistics
	stats struct {
		cachedCount      int64
		flushedCount     int64
		evictedCount     int64
		failedFlushCount int64
	}
}

// NewLocalServerExporter creates a new local server log exporter
func NewLocalServerExporter(serviceName string, cacheMaxSize int, discoveryInterval time.Duration) *LocalServerExporter {
	if serviceName == "" {
		serviceName = "default"
	}
	
	if cacheMaxSize <= 0 {
		cacheMaxSize = 200
	}
	
	if discoveryInterval <= 0 {
		discoveryInterval = 30 * time.Second
	}
	
	exporter := &LocalServerExporter{
		serviceName:       serviceName,
		cacheMaxSize:      cacheMaxSize,
		discoveryInterval: discoveryInterval,
		logCache:          make([]sdklog.Record, 0, cacheMaxSize),
		stopDiscovery:     make(chan struct{}),
	}
	
	// Try initial discovery
	exporter.tryDiscoverServer()
	
	// Start background discovery
	exporter.startDiscoveryWorker()
	
	return exporter
}

// Export exports log records to the local server or caches them
func (e *LocalServerExporter) Export(ctx context.Context, records []sdklog.Record) error {
	// Try to discover server if not available
	if !e.serverAvailable {
		e.tryDiscoverServer()
	}
	
	// Try to export if server is available
	if e.serverAvailable && e.otlpExporter != nil {
		err := e.otlpExporter.Export(ctx, records)
		if err == nil {
			// Successful export, try to flush cache
			e.flushCache(ctx)
			return nil
		}
		
		// Export failed, mark server as unavailable
		e.markServerUnavailable()
	}
	
	// Server not available or export failed, cache the logs
	e.cacheLogs(records)
	
	return nil // Always return nil to not block the logging pipeline
}

// Shutdown gracefully shuts down the exporter
func (e *LocalServerExporter) Shutdown(ctx context.Context) error {
	// Stop discovery worker
	close(e.stopDiscovery)
	e.discoveryDone.Wait()
	
	// Try final flush if server is available
	if e.serverAvailable && e.otlpExporter != nil {
		e.flushCache(ctx)
		err := e.otlpExporter.Shutdown(ctx)
		e.otlpExporter = nil
		return err
	}
	
	return nil
}

// ForceFlush attempts to flush any pending logs
func (e *LocalServerExporter) ForceFlush(ctx context.Context) error {
	// Try to discover server if not available
	if !e.serverAvailable {
		e.tryDiscoverServer()
	}
	
	// Attempt to flush cache
	if e.serverAvailable && e.otlpExporter != nil {
		e.flushCache(ctx)
		return e.otlpExporter.ForceFlush(ctx)
	}
	
	return nil
}

// tryDiscoverServer attempts to discover the local server and create OTLP exporter
func (e *LocalServerExporter) tryDiscoverServer() {
	endpoint := GetLocalServerEndpoint()
	
	if endpoint == "" {
		// No server available
		e.markServerUnavailable()
		return
	}
	
	// Server found, check if endpoint changed
	if endpoint != e.currentEndpoint {
		e.initializeExporter(endpoint)
	}
}

// initializeExporter creates a new OTLP gRPC exporter for the given endpoint
func (e *LocalServerExporter) initializeExporter(endpoint string) {
	// Clean up existing exporter
	if e.otlpExporter != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		e.otlpExporter.Shutdown(ctx)
	}
	
	// Create new exporter
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(endpoint),
		otlploggrpc.WithInsecure(), // Local development server
		otlploggrpc.WithTimeout(10 * time.Second),
	}
	
	// Add service name header if specified
	if e.serviceName != "" {
		opts = append(opts, otlploggrpc.WithHeaders(map[string]string{
			"service-name": e.serviceName,
		}))
	}
	
	exporter, err := otlploggrpc.New(context.Background(), opts...)
	if err != nil {
		e.markServerUnavailable()
		return
	}
	
	e.otlpExporter = exporter
	e.currentEndpoint = endpoint
	e.serverAvailable = true
}

// markServerUnavailable marks the server as unavailable and cleans up
func (e *LocalServerExporter) markServerUnavailable() {
	e.serverAvailable = false
	e.currentEndpoint = ""
	if e.otlpExporter != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		e.otlpExporter.Shutdown(ctx)
		e.otlpExporter = nil
	}
}

// cacheLogs adds logs to the local cache with FIFO eviction
func (e *LocalServerExporter) cacheLogs(records []sdklog.Record) {
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()
	
	for _, record := range records {
		// Check if cache is full
		if len(e.logCache) >= e.cacheMaxSize {
			// Remove oldest record (FIFO)
			e.logCache = e.logCache[1:]
			e.stats.evictedCount++
		}
		
		e.logCache = append(e.logCache, record)
		e.stats.cachedCount++
	}
}

// flushCache attempts to flush cached logs to the server
func (e *LocalServerExporter) flushCache(ctx context.Context) {
	e.cacheMutex.Lock()
	
	if len(e.logCache) == 0 {
		e.cacheMutex.Unlock()
		return
	}
	
	// Copy cache for export
	cachedRecords := make([]sdklog.Record, len(e.logCache))
	copy(cachedRecords, e.logCache)
	cacheSize := len(e.logCache)
	
	e.cacheMutex.Unlock()
	
	// Try to export cached records
	if e.otlpExporter != nil {
		err := e.otlpExporter.Export(ctx, cachedRecords)
		if err == nil {
			// Successful flush, clear cache
			e.cacheMutex.Lock()
			// Only clear records that were successfully sent
			if len(e.logCache) >= cacheSize {
				e.logCache = e.logCache[cacheSize:]
			} else {
				e.logCache = e.logCache[:0]
			}
			e.stats.flushedCount += int64(cacheSize)
			e.cacheMutex.Unlock()
		} else {
			e.stats.failedFlushCount++
			// Mark server as unavailable for next discovery cycle
			e.markServerUnavailable()
		}
	}
}

// startDiscoveryWorker starts the background discovery worker
func (e *LocalServerExporter) startDiscoveryWorker() {
	e.discoveryDone.Add(1)
	go func() {
		defer e.discoveryDone.Done()
		
		ticker := time.NewTicker(e.discoveryInterval)
		defer ticker.Stop()
		
		for {
			select {
			case <-e.stopDiscovery:
				return
			case <-ticker.C:
				e.tryDiscoverServer()
				if e.serverAvailable {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					e.flushCache(ctx)
					cancel()
				}
			}
		}
	}()
}

// GetStats returns cache and export statistics
func (e *LocalServerExporter) GetStats() map[string]interface{} {
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()
	
	return map[string]interface{}{
		"cached_count":       e.stats.cachedCount,
		"flushed_count":      e.stats.flushedCount,
		"evicted_count":      e.stats.evictedCount,
		"failed_flush_count": e.stats.failedFlushCount,
		"current_cache_size": len(e.logCache),
		"cache_max_size":     e.cacheMaxSize,
		"server_available":   e.serverAvailable,
		"current_endpoint":   e.currentEndpoint,
		"service_name":       e.serviceName,
	}
}