// Package metrics provides functionality for collecting and reporting cache performance metrics.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozephyr/gencache/internal"
)

// CacheMetrics represents unified metrics for all cache components
type CacheMetrics struct {
	// Basic Cache Metrics
	Size              atomic.Int64
	Hits              atomic.Int64
	Misses            atomic.Int64
	Evictions         atomic.Int64
	MemoryUsage       atomic.Int64
	LastOperationTime atomic.Value // time.Time

	// Batch Operation Metrics
	BatchOperations    atomic.Int64
	BatchItems         atomic.Int64
	BatchSuccess       atomic.Int64
	BatchErrors        atomic.Int64
	BatchTimeouts      atomic.Int64
	LastBatchOperation atomic.Value // time.Time

	// Compression Metrics
	CompressedItems   atomic.Int64
	DecompressedItems atomic.Int64
	CompressedBytes   atomic.Int64
	DecompressedBytes atomic.Int64
	CompressionRatio  atomic.Value // float64
	LastCompression   atomic.Value // time.Time

	// Pool Metrics
	PoolCreated     atomic.Int64
	PoolDestroyed   atomic.Int64
	PoolCurrentSize atomic.Int64
	LastPoolCleanup atomic.Value // time.Time

	// Resource Usage
	GoRoutines  atomic.Int64
	LastCleanup atomic.Value // time.Time
}

// MetricsSnapshot is a thread-safe copy of metrics
type MetricsSnapshot struct {
	// Basic Cache
	Size              int64
	Hits              int64
	Misses            int64
	Evictions         int64
	MemoryUsage       int64
	LastOperationTime time.Time

	// Batch Operations
	BatchOperations    int64
	BatchItems         int64
	BatchSuccess       int64
	BatchErrors        int64
	BatchTimeouts      int64
	LastBatchOperation time.Time

	// Compression
	CompressedItems   int64
	DecompressedItems int64
	CompressedBytes   int64
	DecompressedBytes int64
	CompressionRatio  float64
	LastCompression   time.Time

	// Pool
	PoolCreated     int64
	PoolDestroyed   int64
	PoolCurrentSize int64
	LastPoolCleanup time.Time

	// Resource Usage
	GoRoutines  int64
	LastCleanup time.Time
}

// Metrics tracks cache performance metrics
type Metrics struct {
	// Hits is the number of successful cache lookups
	hits *internal.SafeCounter

	// Misses is the number of failed cache lookups
	misses *internal.SafeCounter

	// Evictions is the number of items evicted from the cache
	evictions *internal.SafeCounter

	// Size is the current number of items in the cache
	size *internal.SafeCounter

	// Capacity is the maximum number of items the cache can hold
	Capacity int64

	// LastAccess is the time of the last cache access
	LastAccess time.Time

	mu sync.RWMutex
}

// New creates a new Metrics instance
func New(capacity int64) *Metrics {
	return &Metrics{
		hits:      internal.NewSafeCounter(),
		misses:    internal.NewSafeCounter(),
		evictions: internal.NewSafeCounter(),
		size:      internal.NewSafeCounter(),
		Capacity:  capacity,
	}
}

// NewCacheMetrics creates a new CacheMetrics instance
func NewCacheMetrics() *CacheMetrics {
	metrics := &CacheMetrics{}
	metrics.LastOperationTime.Store(time.Time{})
	metrics.LastBatchOperation.Store(time.Time{})
	metrics.LastCompression.Store(time.Time{})
	metrics.LastPoolCleanup.Store(time.Time{})
	metrics.LastCleanup.Store(time.Time{})
	metrics.CompressionRatio.Store(0.0)
	return metrics
}

// GetSnapshot returns a thread-safe copy of current metrics
func (m *CacheMetrics) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Size:               m.Size.Load(),
		Hits:               m.Hits.Load(),
		Misses:             m.Misses.Load(),
		Evictions:          m.Evictions.Load(),
		MemoryUsage:        m.MemoryUsage.Load(),
		LastOperationTime:  m.LastOperationTime.Load().(time.Time),
		BatchOperations:    m.BatchOperations.Load(),
		BatchItems:         m.BatchItems.Load(),
		BatchSuccess:       m.BatchSuccess.Load(),
		BatchErrors:        m.BatchErrors.Load(),
		BatchTimeouts:      m.BatchTimeouts.Load(),
		LastBatchOperation: m.LastBatchOperation.Load().(time.Time),
		CompressedItems:    m.CompressedItems.Load(),
		DecompressedItems:  m.DecompressedItems.Load(),
		CompressedBytes:    m.CompressedBytes.Load(),
		DecompressedBytes:  m.DecompressedBytes.Load(),
		CompressionRatio:   m.CompressionRatio.Load().(float64),
		LastCompression:    m.LastCompression.Load().(time.Time),
		PoolCreated:        m.PoolCreated.Load(),
		PoolDestroyed:      m.PoolDestroyed.Load(),
		PoolCurrentSize:    m.PoolCurrentSize.Load(),
		LastPoolCleanup:    m.LastPoolCleanup.Load().(time.Time),
		GoRoutines:         m.GoRoutines.Load(),
		LastCleanup:        m.LastCleanup.Load().(time.Time),
	}
}

// RecordHit records a cache hit
func (m *CacheMetrics) RecordHit() {
	m.Hits.Add(1)
	m.LastOperationTime.Store(time.Now())
}

// RecordMiss records a cache miss
func (m *CacheMetrics) RecordMiss() {
	m.Misses.Add(1)
	m.LastOperationTime.Store(time.Now())
}

// RecordEviction records a cache eviction
func (m *CacheMetrics) RecordEviction() {
	m.Evictions.Add(1)
}

// UpdateSize updates the current cache size
func (m *CacheMetrics) UpdateSize(size int64) {
	m.Size.Store(size)
}

// GetStats returns the current metrics
func (m *Metrics) GetStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]any{
		"hits":        m.hits.Get(),
		"misses":      m.misses.Get(),
		"evictions":   m.evictions.Get(),
		"size":        m.size.Get(),
		"capacity":    m.Capacity,
		"hit_ratio":   m.HitRatio(),
		"last_access": m.LastAccess,
	}
}

// HitRatio returns the cache hit ratio
func (m *Metrics) HitRatio() float64 {
	hits := m.hits.Get()
	misses := m.misses.Get()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// Reset resets all metrics to zero
func (m *CacheMetrics) Reset() {
	m.Size.Store(0)
	m.Hits.Store(0)
	m.Misses.Store(0)
	m.Evictions.Store(0)
	m.MemoryUsage.Store(0)
	m.LastOperationTime.Store(time.Time{})
	m.BatchOperations.Store(0)
	m.BatchItems.Store(0)
	m.BatchSuccess.Store(0)
	m.BatchErrors.Store(0)
	m.BatchTimeouts.Store(0)
	m.LastBatchOperation.Store(time.Time{})
	m.CompressedItems.Store(0)
	m.DecompressedItems.Store(0)
	m.CompressedBytes.Store(0)
	m.DecompressedBytes.Store(0)
	m.CompressionRatio.Store(0.0)
	m.LastCompression.Store(time.Time{})
	m.PoolCreated.Store(0)
	m.PoolDestroyed.Store(0)
	m.PoolCurrentSize.Store(0)
	m.LastPoolCleanup.Store(time.Time{})
	m.GoRoutines.Store(0)
	m.LastCleanup.Store(time.Time{})
}
