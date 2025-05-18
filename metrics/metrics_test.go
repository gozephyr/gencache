package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsExporter(t *testing.T) {
	tests := []struct {
		name         string
		exporterType ExporterType
		cacheName    string
		labels       map[string]string
		wantType     any
	}{
		{
			name:         "Standard Exporter",
			exporterType: StandardExporter,
			cacheName:    "test-cache",
			wantType:     &CacheMetrics{},
		},
		{
			name:         "Prometheus Exporter",
			exporterType: PrometheusExporterType,
			cacheName:    "test-cache-prom",
			labels: map[string]string{
				"service": "test-service",
				"env":     "test",
			},
			wantType: &PrometheusMetricsExporter{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := NewMetricsExporter(tt.exporterType, tt.cacheName, tt.labels)
			assert.IsType(t, tt.wantType, exporter)
		})
	}
}

func TestPrometheusMetricsExporter(t *testing.T) {
	// Create a test registry
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	// Create exporter with unique name
	exporter := NewPrometheusMetricsExporter("test-cache-prom", map[string]string{
		"service": "test-service",
	})

	// Clean up after test
	defer func() {
		registry.Unregister(exporter.hits)
		registry.Unregister(exporter.misses)
		registry.Unregister(exporter.evictions)
		registry.Unregister(exporter.size)
		registry.Unregister(exporter.memory)
	}()

	t.Run("RecordHit", func(t *testing.T) {
		exporter.RecordHit()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.Hits)
	})

	t.Run("RecordMiss", func(t *testing.T) {
		exporter.RecordMiss()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.Misses)
	})

	t.Run("RecordEviction", func(t *testing.T) {
		exporter.RecordEviction()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.Evictions)
	})

	t.Run("UpdateSize", func(t *testing.T) {
		exporter.UpdateSize(100)
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(100), snapshot.Size)
	})

	t.Run("Reset", func(t *testing.T) {
		exporter.Reset()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(0), snapshot.Hits)
		assert.Equal(t, int64(0), snapshot.Misses)
		assert.Equal(t, int64(0), snapshot.Evictions)
		assert.Equal(t, int64(0), snapshot.Size)
	})
}

func TestCacheMetrics(t *testing.T) {
	metrics := NewCacheMetrics()

	// Test initial state
	snapshot := metrics.GetSnapshot()
	require.Equal(t, int64(0), snapshot.Size)
	require.Equal(t, int64(0), snapshot.Hits)
	require.Equal(t, int64(0), snapshot.Misses)
	require.Equal(t, int64(0), snapshot.Evictions)
	require.Equal(t, int64(0), snapshot.MemoryUsage)
	require.Equal(t, time.Time{}, snapshot.LastOperationTime)
	require.Equal(t, int64(0), snapshot.BatchOperations)
	require.Equal(t, int64(0), snapshot.BatchItems)
	require.Equal(t, int64(0), snapshot.BatchSuccess)
	require.Equal(t, int64(0), snapshot.BatchErrors)
	require.Equal(t, int64(0), snapshot.BatchTimeouts)
	require.Equal(t, time.Time{}, snapshot.LastBatchOperation)
	require.Equal(t, int64(0), snapshot.CompressedItems)
	require.Equal(t, int64(0), snapshot.DecompressedItems)
	require.Equal(t, int64(0), snapshot.CompressedBytes)
	require.Equal(t, int64(0), snapshot.DecompressedBytes)
	require.Equal(t, 0.0, snapshot.CompressionRatio)
	require.Equal(t, time.Time{}, snapshot.LastCompression)
	require.Equal(t, int64(0), snapshot.PoolCreated)
	require.Equal(t, int64(0), snapshot.PoolDestroyed)
	require.Equal(t, int64(0), snapshot.PoolCurrentSize)
	require.Equal(t, time.Time{}, snapshot.LastPoolCleanup)
	require.Equal(t, int64(0), snapshot.GoRoutines)
	require.Equal(t, time.Time{}, snapshot.LastCleanup)

	// Test recording hits and misses
	metrics.RecordHit()
	metrics.RecordMiss()
	snapshot = metrics.GetSnapshot()
	require.Equal(t, int64(1), snapshot.Hits)
	require.Equal(t, int64(1), snapshot.Misses)

	// Test recording evictions
	metrics.RecordEviction()
	snapshot = metrics.GetSnapshot()
	require.Equal(t, int64(1), snapshot.Evictions)

	// Test updating size
	metrics.UpdateSize(10)
	snapshot = metrics.GetSnapshot()
	require.Equal(t, int64(10), snapshot.Size)

	// Test resetting metrics
	metrics.Reset()
	snapshot = metrics.GetSnapshot()
	require.Equal(t, int64(0), snapshot.Size)
	require.Equal(t, int64(0), snapshot.Hits)
	require.Equal(t, int64(0), snapshot.Misses)
	require.Equal(t, int64(0), snapshot.Evictions)
	require.Equal(t, int64(0), snapshot.MemoryUsage)
	require.Equal(t, time.Time{}, snapshot.LastOperationTime)
	require.Equal(t, int64(0), snapshot.BatchOperations)
	require.Equal(t, int64(0), snapshot.BatchItems)
	require.Equal(t, int64(0), snapshot.BatchSuccess)
	require.Equal(t, int64(0), snapshot.BatchErrors)
	require.Equal(t, int64(0), snapshot.BatchTimeouts)
	require.Equal(t, time.Time{}, snapshot.LastBatchOperation)
	require.Equal(t, int64(0), snapshot.CompressedItems)
	require.Equal(t, int64(0), snapshot.DecompressedItems)
	require.Equal(t, int64(0), snapshot.CompressedBytes)
	require.Equal(t, int64(0), snapshot.DecompressedBytes)
	require.Equal(t, 0.0, snapshot.CompressionRatio)
	require.Equal(t, time.Time{}, snapshot.LastCompression)
	require.Equal(t, int64(0), snapshot.PoolCreated)
	require.Equal(t, int64(0), snapshot.PoolDestroyed)
	require.Equal(t, int64(0), snapshot.PoolCurrentSize)
	require.Equal(t, time.Time{}, snapshot.LastPoolCleanup)
	require.Equal(t, int64(0), snapshot.GoRoutines)
	require.Equal(t, time.Time{}, snapshot.LastCleanup)
}

func TestMetricsSnapshot(t *testing.T) {
	metrics := NewCacheMetrics()

	// Set some values
	metrics.Hits.Add(10)
	metrics.Misses.Add(5)
	metrics.Evictions.Add(2)
	metrics.Size.Store(100)
	metrics.MemoryUsage.Store(1000)
	now := time.Now()
	metrics.LastOperationTime.Store(now)

	// Get snapshot
	snapshot := metrics.GetSnapshot()

	// Verify snapshot values
	assert.Equal(t, int64(10), snapshot.Hits)
	assert.Equal(t, int64(5), snapshot.Misses)
	assert.Equal(t, int64(2), snapshot.Evictions)
	assert.Equal(t, int64(100), snapshot.Size)
	assert.Equal(t, int64(1000), snapshot.MemoryUsage)
	assert.Equal(t, now, snapshot.LastOperationTime)
}

func TestMetrics(t *testing.T) {
	metrics := New(100)

	// Test initial state
	stats := metrics.GetStats()
	require.Equal(t, int64(0), stats["hits"])
	require.Equal(t, int64(0), stats["misses"])
	require.Equal(t, int64(0), stats["evictions"])
	require.Equal(t, int64(0), stats["size"])
	require.Equal(t, int64(100), stats["capacity"])
	require.Equal(t, 0.0, stats["hit_ratio"])
	require.Equal(t, time.Time{}, stats["last_access"])

	// Test recording hits and misses
	metrics.hits.Increment()
	metrics.misses.Increment()
	stats = metrics.GetStats()
	require.Equal(t, int64(1), stats["hits"])
	require.Equal(t, int64(1), stats["misses"])
	require.Equal(t, 0.5, stats["hit_ratio"])

	// Test recording evictions
	metrics.evictions.Increment()
	stats = metrics.GetStats()
	require.Equal(t, int64(1), stats["evictions"])

	// Test updating size
	metrics.size.Increment()
	stats = metrics.GetStats()
	require.Equal(t, int64(1), stats["size"])
}
