package metrics

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// ExporterType defines the type of metrics exporter
type ExporterType string

const (
	// StandardExporter uses the default metrics implementation
	StandardExporter ExporterType = "standard"
	// PrometheusExporterType uses Prometheus metrics
	PrometheusExporterType ExporterType = "prometheus"
)

// MetricsExporter defines the interface for metrics exporters
type MetricsExporter interface {
	// RecordHit records a cache hit
	RecordHit()
	// RecordMiss records a cache miss
	RecordMiss()
	// RecordEviction records a cache eviction
	RecordEviction()
	// UpdateSize updates the current cache size
	UpdateSize(size int64)
	// GetSnapshot returns a thread-safe copy of current metrics
	GetSnapshot() MetricsSnapshot
	// Reset resets all metrics to zero
	Reset()
}

// PrometheusMetricsExporter implements MetricsExporter using Prometheus metrics
type PrometheusMetricsExporter struct {
	// Basic Cache Metrics
	hits      *prometheus.CounterVec
	misses    *prometheus.CounterVec
	evictions *prometheus.CounterVec
	size      *prometheus.GaugeVec
	memory    *prometheus.GaugeVec

	// Internal counters for snapshot
	internalHits      atomic.Int64
	internalMisses    atomic.Int64
	internalEvictions atomic.Int64
	internalSize      atomic.Int64
	internalMemory    atomic.Int64

	// Labels for metrics
	labels map[string]string
}

// NewPrometheusMetricsExporter creates a new Prometheus metrics exporter
func NewPrometheusMetricsExporter(cacheName string, labels map[string]string) *PrometheusMetricsExporter {
	if labels == nil {
		labels = make(map[string]string)
	}

	// Set default service name if not provided
	if _, exists := labels["service"]; !exists {
		labels["service"] = "gencache"
	}

	// Always include cache name
	labels["cache"] = cacheName

	exporter := &PrometheusMetricsExporter{
		labels: labels,
	}

	// Initialize Prometheus metrics
	exporter.hits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"service", "cache"},
	)

	exporter.misses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"service", "cache"},
	)

	exporter.evictions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_evictions_total",
			Help: "Total number of cache evictions",
		},
		[]string{"service", "cache"},
	)

	exporter.size = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "Current number of items in the cache",
		},
		[]string{"service", "cache"},
	)

	exporter.memory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cache_memory_bytes",
			Help: "Current memory usage of the cache",
		},
		[]string{"service", "cache"},
	)

	// Register metrics
	prometheus.MustRegister(
		exporter.hits,
		exporter.misses,
		exporter.evictions,
		exporter.size,
		exporter.memory,
	)

	return exporter
}

// RecordHit implements MetricsExporter
func (e *PrometheusMetricsExporter) RecordHit() {
	e.hits.With(e.labels).Inc()
	e.internalHits.Add(1)
}

// RecordMiss implements MetricsExporter
func (e *PrometheusMetricsExporter) RecordMiss() {
	e.misses.With(e.labels).Inc()
	e.internalMisses.Add(1)
}

// RecordEviction implements MetricsExporter
func (e *PrometheusMetricsExporter) RecordEviction() {
	e.evictions.With(e.labels).Inc()
	e.internalEvictions.Add(1)
}

// UpdateSize implements MetricsExporter
func (e *PrometheusMetricsExporter) UpdateSize(size int64) {
	e.size.With(e.labels).Set(float64(size))
	e.internalSize.Store(size)
}

// GetSnapshot implements MetricsExporter
func (e *PrometheusMetricsExporter) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Hits:        e.internalHits.Load(),
		Misses:      e.internalMisses.Load(),
		Evictions:   e.internalEvictions.Load(),
		Size:        e.internalSize.Load(),
		MemoryUsage: e.internalMemory.Load(),
	}
}

// Reset implements MetricsExporter
func (e *PrometheusMetricsExporter) Reset() {
	// Reset internal counters
	e.internalHits.Store(0)
	e.internalMisses.Store(0)
	e.internalEvictions.Store(0)
	e.internalSize.Store(0)
	e.internalMemory.Store(0)

	// Note: Prometheus metrics are not reset as they are meant to be cumulative
}

// NewMetricsExporter creates a new metrics exporter based on the specified type
func NewMetricsExporter(exporterType ExporterType, cacheName string, labels map[string]string) MetricsExporter {
	switch exporterType {
	case PrometheusExporterType:
		return NewPrometheusMetricsExporter(cacheName, labels)
	default:
		return NewCacheMetrics()
	}
}
