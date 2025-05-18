package gencache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozephyr/gencache/errors"
)

// ObjectPool provides a pool of reusable objects
type ObjectPool[T any] struct {
	pool sync.Pool
}

// NewObjectPool creates a new object pool
func NewObjectPool[T any](newFunc func() T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any {
				return newFunc()
			},
		},
	}
}

// Get retrieves an object from the pool
func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put returns an object to the pool
func (p *ObjectPool[T]) Put(x T) {
	p.pool.Put(x)
}

// EntryPool provides a pool of cache entries
type EntryPool[V any] struct {
	pool *ObjectPool[Entry[V]]
}

// NewEntryPool creates a new pool of cache entries
func NewEntryPool[V any]() *EntryPool[V] {
	return &EntryPool[V]{
		pool: NewObjectPool(func() Entry[V] {
			return Entry[V]{
				AccessCount: 0,
			}
		}),
	}
}

// Get retrieves an entry from the pool
func (p *EntryPool[V]) Get() Entry[V] {
	return p.pool.Get()
}

// Put returns an entry to the pool
func (p *EntryPool[V]) Put(entry Entry[V]) {
	// Reset the entry before putting it back in the pool
	entry.AccessCount = 0
	p.pool.Put(entry)
}

// BatchResultPool provides a pool of batch results
type BatchResultPool[K comparable, V any] struct {
	pool *ObjectPool[BatchResult[K, V]]
}

// NewBatchResultPool creates a new pool of batch results
func NewBatchResultPool[K comparable, V any]() *BatchResultPool[K, V] {
	return &BatchResultPool[K, V]{
		pool: NewObjectPool(func() BatchResult[K, V] {
			return BatchResult[K, V]{}
		}),
	}
}

// Get retrieves a batch result from the pool
func (p *BatchResultPool[K, V]) Get() BatchResult[K, V] {
	return p.pool.Get()
}

// Put returns a batch result to the pool
func (p *BatchResultPool[K, V]) Put(result BatchResult[K, V]) {
	// Reset the result before putting it back in the pool
	result.Error = nil
	p.pool.Put(result)
}

// PoolConfig represents configuration for the object pool
type PoolConfig struct {
	MaxSize       int
	MinSize       int
	CleanupPeriod time.Duration
	MaxIdleTime   time.Duration
	ShrinkFactor  float64
}

// DefaultPoolConfig returns the default pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxSize:       1000,
		MinSize:       10,
		CleanupPeriod: 5 * time.Minute,
		MaxIdleTime:   10 * time.Minute,
		ShrinkFactor:  0.5,
	}
}

// PoolStats represents statistics for the object pool
type PoolStats struct {
	TotalCreated   atomic.Int64
	TotalDestroyed atomic.Int64
	CurrentSize    atomic.Int64
	MaxSize        atomic.Int64
	MinSize        atomic.Int64
	LastCleanup    atomic.Value // time.Time
	LastShrink     atomic.Value // time.Time
}

// PooledCache extends Cache with object pooling
type PooledCache[K comparable, V any] interface {
	Cache[K, V]

	// GetPoolStats returns the current pool statistics
	GetPoolStats() *PoolStats

	// ResetPoolStats resets the pool statistics
	ResetPoolStats()

	// Cleanup performs manual cleanup of idle objects
	Cleanup()

	// OnEvent registers a callback for cache events, converting *V to V
	OnEvent(cb CacheCallback[K, V])

	// Close stops the cleanup goroutine and releases resources
	Close() error
}

// pooledCache implements PooledCache
// Now, all cache operations use the pool for allocations and deallocations.
type pooledCache[K comparable, V any] struct {
	Cache[K, *V] // Underlying cache stores *V
	config       PoolConfig
	stats        PoolStats
	mu           chan struct{} // channel-based mutex for try-lock
	pool         sync.Pool
	stop         chan struct{}
	wg           sync.WaitGroup // Add WaitGroup to track cleanup goroutine
}

// NewPooledCache creates a new cache with object pooling
func NewPooledCache[K comparable, V any](c Cache[K, *V], config PoolConfig) PooledCache[K, V] {
	pc := &pooledCache[K, V]{
		Cache:  c,
		config: config,
		stats:  PoolStats{},
		mu:     make(chan struct{}, 1), // channel-based mutex
		stop:   make(chan struct{}),
	}
	pc.mu <- struct{}{} // initialize the mutex as unlocked

	// Initialize pool with New function
	pc.pool = sync.Pool{
		New: func() any {
			var v V
			if pc.tryLock() {
				pc.stats.TotalCreated.Add(1)
				pc.stats.CurrentSize.Add(1)
				pc.unlock()
			}
			return &v
		},
	}

	// Set initial values
	pc.stats.MaxSize.Store(int64(config.MaxSize))
	pc.stats.MinSize.Store(int64(config.MinSize))

	// Register event callback for delete/eviction
	pc.Cache.OnEvent(func(evt CacheEvent[K, *V]) {
		if evt.Type == EventTypeDelete || evt.Type == EventTypeEviction {
			if evt.Value != nil {
				pc.pool.Put(evt.Value)
				if pc.tryLock() {
					pc.stats.CurrentSize.Add(1)
					pc.unlock()
				}
			}
		}
	})

	// Start cleanup goroutine
	pc.wg.Add(1)
	go pc.cleanupLoop()

	return pc
}

// tryLock attempts to acquire the lock without blocking
func (pc *pooledCache[K, V]) tryLock() bool {
	select {
	case <-pc.mu:
		return true
	default:
		return false
	}
}

// unlock releases the lock
func (pc *pooledCache[K, V]) unlock() {
	pc.mu <- struct{}{}
}

// Set stores a value in the cache, always using the pool
func (pc *pooledCache[K, V]) Set(key K, value V, ttl time.Duration) error {
	ptr := pc.pool.Get().(*V)
	*ptr = value // Copy value into pooled pointer
	return pc.Cache.Set(key, ptr, ttl)
}

// Get retrieves a value from the cache (returns the pooled object)
func (pc *pooledCache[K, V]) Get(key K) (V, error) {
	ptr, err := pc.Cache.Get(key)
	if err != nil || ptr == nil {
		var zero V
		return zero, err
	}
	return *ptr, nil
}

// Delete removes a value from the cache (handled by event callback)
func (pc *pooledCache[K, V]) Delete(key K) error {
	return pc.Cache.Delete(key)
}

// GetWithContext retrieves a value from the cache with context (returns the pooled object)
func (pc *pooledCache[K, V]) GetWithContext(ctx context.Context, key K) (V, error) {
	ptr, err := pc.Cache.GetWithContext(ctx, key)
	if err != nil || ptr == nil {
		var zero V
		return zero, err
	}
	return *ptr, nil
}

// Clear removes all values from the cache (calls Clear on the underlying cache)
func (pc *pooledCache[K, V]) Clear() error {
	// If you want to return all objects to the pool, you need a way to get all keys.
	// For now, just clear the underlying cache.
	return pc.Cache.Clear()
}

// GetPoolStats returns the current pool statistics
func (pc *pooledCache[K, V]) GetPoolStats() *PoolStats {
	return &pc.stats
}

// ResetPoolStats resets the pool statistics
func (pc *pooledCache[K, V]) ResetPoolStats() {
	pc.stats = PoolStats{}
	pc.stats.MaxSize.Store(int64(pc.config.MaxSize))
	pc.stats.MinSize.Store(int64(pc.config.MinSize))
}

// Cleanup performs manual cleanup of idle objects
func (pc *pooledCache[K, V]) Cleanup() {
	if !pc.tryLock() {
		return
	}
	defer pc.unlock()

	currentSize := pc.stats.CurrentSize.Load()
	maxSize := pc.stats.MaxSize.Load()

	// Cleanup if we're over max size
	if currentSize > maxSize {
		shrinkTarget := int64(float64(currentSize) * pc.config.ShrinkFactor)
		if shrinkTarget < pc.stats.MinSize.Load() {
			shrinkTarget = pc.stats.MinSize.Load()
		}

		// Remove excess objects
		for pc.stats.CurrentSize.Load() > shrinkTarget {
			pc.pool.Get() // Remove one object
			pc.stats.CurrentSize.Add(-1)
			pc.stats.TotalDestroyed.Add(1)
		}

		pc.stats.LastShrink.Store(time.Now())
	}

	pc.stats.LastCleanup.Store(time.Now())
}

// cleanupLoop periodically cleans up idle objects
func (pc *pooledCache[K, V]) cleanupLoop() {
	defer pc.wg.Done()
	ticker := time.NewTicker(pc.config.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pc.Cleanup()
		case <-pc.stop:
			return
		default:
			// Add a small delay to prevent busy waiting
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// TestGetFromPool is a test helper to directly get from the pool
func (pc *pooledCache[K, V]) TestGetFromPool() any {
	return pc.pool.Get()
}

// OnEvent registers a callback for cache events, converting *V to V
func (pc *pooledCache[K, V]) OnEvent(cb CacheCallback[K, V]) {
	pc.Cache.OnEvent(func(evt CacheEvent[K, *V]) {
		var v V
		if evt.Value != nil {
			v = *evt.Value
		}
		cb(CacheEvent[K, V]{
			Type:      evt.Type,
			Key:       evt.Key,
			Value:     v,
			Timestamp: evt.Timestamp,
		})
	})
}

// SetWithContext stores a value in the cache with context, always using the pool
func (pc *pooledCache[K, V]) SetWithContext(ctx context.Context, key K, value V, ttl time.Duration) error {
	ptr := pc.pool.Get().(*V)
	*ptr = value // Copy value into pooled pointer
	return pc.Cache.SetWithContext(ctx, key, ptr, ttl)
}

// Close stops the cleanup goroutine and releases resources
func (pc *pooledCache[K, V]) Close() error {
	close(pc.stop)

	// Wait for cleanup goroutine to finish with a timeout
	done := make(chan struct{})
	go func() {
		pc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(1 * time.Second):
		return errors.WrapError("Close", nil, errors.ErrStoreTimeout)
	}
}
