// Package gencache provides a generic caching mechanism
package gencache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/metrics"
	"github.com/gozephyr/gencache/policy"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
)

// CacheEventType represents the type of cache event
type CacheEventType int

const (
	EventTypeSet CacheEventType = iota
	EventTypeGet
	EventTypeDelete
	EventTypeEviction
	EventTypeExpiration
)

// CacheEvent represents an event that occurred in the cache
type CacheEvent[K comparable, V any] struct {
	Type      CacheEventType
	Key       K
	Value     V
	Timestamp time.Time
}

// CacheCallback is a function that handles cache events
type CacheCallback[K comparable, V any] func(CacheEvent[K, V])

// Cache defines the interface for a generic cache
type Cache[K comparable, V any] interface {
	// Get retrieves a value from the cache
	Get(key K) (V, error)

	// GetWithContext retrieves a value from the cache with context
	GetWithContext(ctx context.Context, key K) (V, error)

	// Set stores a value in the cache with TTL
	Set(key K, value V, ttl time.Duration) error

	// SetWithContext stores a value in the cache with context and TTL
	SetWithContext(ctx context.Context, key K, value V, ttl time.Duration) error

	// Delete removes a value from the cache
	Delete(key K) error

	// DeleteWithContext removes a value from the cache with context
	DeleteWithContext(ctx context.Context, key K) error

	// Clear removes all values from the cache
	Clear() error

	// ClearWithContext removes all values from the cache with context
	ClearWithContext(ctx context.Context) error

	// Close closes the cache and releases resources
	Close() error

	// Stats returns the current cache statistics
	Stats() *Stats

	// OnEvent registers a callback function for cache events
	OnEvent(callback CacheCallback[K, V])
}

// Entry represents a cached value with metadata
type Entry[V any] struct {
	Value       V         `json:"value"`
	Expires     time.Time `json:"expires"`
	CreatedAt   time.Time `json:"createdAt"`
	LastAccess  time.Time `json:"lastAccess"`
	AccessCount int64     `json:"accessCount"`
}

// Stats tracks cache statistics
type Stats struct {
	Hits                  atomic.Int64
	Misses                atomic.Int64
	Sets                  atomic.Int64
	Deletes               atomic.Int64
	Clears                atomic.Int64
	Errors                atomic.Int64
	Size                  atomic.Int64
	Capacity              atomic.Int64
	Evictions             atomic.Int64
	LastOperationDuration atomic.Int64
}

// IncHits increments the hit counter
func (s *Stats) IncHits() {
	s.Hits.Add(1)
}

// IncMisses increments the miss counter
func (s *Stats) IncMisses() {
	s.Misses.Add(1)
}

// IncSets increments the set counter
func (s *Stats) IncSets() {
	s.Sets.Add(1)
}

// IncDeletes increments the delete counter
func (s *Stats) IncDeletes() {
	s.Deletes.Add(1)
}

// IncClears increments the clear counter
func (s *Stats) IncClears() {
	s.Clears.Add(1)
}

// IncErrors increments the error counter
func (s *Stats) IncErrors() {
	s.Errors.Add(1)
}

// StatsSnapshot is a copy of Stats without the mutex for safe return
// from GetStats.
type StatsSnapshot struct {
	Size                  int64
	Capacity              int64
	Hits                  int64
	Misses                int64
	Evictions             int64
	PanicCount            int64
	LastPanic             time.Time
	LastOperationDuration int64
}

// CacheWithStats extends Cache with statistics tracking
type CacheWithStats[K comparable, V any] interface {
	Cache[K, V]

	// GetStats returns the current cache statistics
	GetStats() StatsSnapshot

	// ResetStats resets the cache statistics
	ResetStats()
}

// item represents a cache item with expiration
type item[V any] struct {
	value     V
	expiresAt time.Time
}

// Client interface for underlying storage
type Client interface {
	Close() error
}

// itemsMap is a type that can be stored in atomic.Value
type itemsMap[K comparable, V any] struct {
	items map[K]*item[V]
}

// cache implements the Cache interface
type cache[K comparable, V any] struct {
	items           atomic.Value // *itemsMap[K, V]
	mu              sync.RWMutex // Only used for operations that need to modify multiple fields atomically
	maxSize         int
	maxMemory       int64
	ttlConfig       ttl.Config
	policy          policy.Policy[K, V]
	stats           *Stats
	callbacks       []CacheCallback[K, V]
	callbacksMu     sync.RWMutex
	cleanupInterval time.Duration
	cleanupTicker   *time.Ticker
	cleanupDone     chan struct{}
	cleanupStop     chan struct{}
	store           store.Store[K, V]
	metrics         metrics.MetricsExporter
	closeOnce       sync.Once
	closed          atomic.Bool
}

// New creates a new cache with the given options
func New[K comparable, V any](opts ...Option[K, V]) Cache[K, V] {
	options := DefaultOptions[K, V]()
	for _, opt := range opts {
		opt(options)
	}

	// Create metrics exporter based on configuration
	metricsExporter := metrics.NewMetricsExporter(
		metrics.StandardExporter,
		options.MetricsConfig.CacheName,
		options.MetricsConfig.Labels,
	)

	// Initialize cache with default policy if none provided
	if options.Policy == nil {
		options.Policy = policy.NewLRU[K, V](policy.WithMaxSize(options.MaxSize))
	}

	c := &cache[K, V]{
		maxSize:         options.MaxSize,
		maxMemory:       options.MaxMemory,
		ttlConfig:       options.TTLConfig,
		policy:          options.Policy,
		stats:           &Stats{},
		cleanupInterval: options.CleanupInterval,
		cleanupDone:     make(chan struct{}),
		cleanupStop:     make(chan struct{}),
		store:           options.Store,
		metrics:         metricsExporter,
	}

	// Initialize items map
	c.items.Store(&itemsMap[K, V]{
		items: make(map[K]*item[V]),
	})

	// Start cleanup goroutine
	if c.cleanupInterval > 0 {
		c.cleanupTicker = time.NewTicker(c.cleanupInterval)
		go c.cleanup()
	}

	return c
}

// getItems returns the current items map
func (c *cache[K, V]) getItems() map[K]*item[V] {
	if c.closed.Load() {
		return make(map[K]*item[V])
	}
	items := c.items.Load()
	if items == nil {
		return make(map[K]*item[V])
	}
	im := items.(*itemsMap[K, V])
	if im.items == nil {
		return make(map[K]*item[V])
	}
	return im.items
}

// emitEvent emits a cache event
func (c *cache[K, V]) emitEvent(eventType CacheEventType, key K, value V) {
	if c.closed.Load() {
		return
	}

	event := CacheEvent[K, V]{
		Type:      eventType,
		Key:       key,
		Value:     value,
		Timestamp: time.Now(),
	}

	c.callbacksMu.RLock()
	defer c.callbacksMu.RUnlock()

	for _, callback := range c.callbacks {
		callback(event)
	}
}

// cleanup periodically removes expired items
func (c *cache[K, V]) cleanup() {
	defer func() {
		if c.cleanupDone != nil {
			close(c.cleanupDone)
		}
	}()

	const maxBatchSize = 1000

	for {
		select {
		case <-c.cleanupTicker.C:
			// First collect expired keys with minimal lock time
			var expiredKeys []K
			now := time.Now()

			c.mu.RLock()
			for k, v := range c.items.Load().(*itemsMap[K, V]).items {
				if v.expiresAt.Before(now) {
					expiredKeys = append(expiredKeys, k)
					if len(expiredKeys) >= maxBatchSize {
						break
					}
				}
			}
			c.mu.RUnlock()

			if len(expiredKeys) == 0 {
				continue
			}

			// Process expired keys in smaller chunks
			const chunkSize = 100
			for i := 0; i < len(expiredKeys); i += chunkSize {
				end := i + chunkSize
				if end > len(expiredKeys) {
					end = len(expiredKeys)
				}

				chunk := expiredKeys[i:end]
				c.mu.Lock()
				for _, key := range chunk {
					if item, exists := c.items.Load().(*itemsMap[K, V]).items[key]; exists && item.expiresAt.Before(now) {
						value := item.value
						delete(c.items.Load().(*itemsMap[K, V]).items, key)
						if c.stats != nil {
							c.stats.Evictions.Add(1)
						}
						c.emitEvent(EventTypeExpiration, key, value)
					}
				}
				c.mu.Unlock()
			}
		case <-c.cleanupStop:
			return
		}
	}
}

// Close implements the Cache interface
func (c *cache[K, V]) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)

		// Signal cleanup goroutine to stop before acquiring lock
		if c.cleanupStop != nil {
			close(c.cleanupStop)
		}
		// Wait for cleanup goroutine to finish if cleanupDone exists
		if c.cleanupDone != nil {
			<-c.cleanupDone
			c.cleanupDone = nil
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		// Stop the cleanup ticker if it exists
		if c.cleanupTicker != nil {
			c.cleanupTicker.Stop()
		}

		// Clear all items
		c.items.Store(&itemsMap[K, V]{items: make(map[K]*item[V])})

		// Clear callbacks
		c.callbacksMu.Lock()
		c.callbacks = nil
		c.callbacksMu.Unlock()

		// Close the store if it exists
		if c.store != nil {
			_ = c.store.Close(context.Background())
		}
	})
	return nil
}

// Get retrieves a value from the cache
func (c *cache[K, V]) Get(key K) (V, error) {
	return c.GetWithContext(context.Background(), key)
}

// GetWithContext retrieves a value from the cache with context
func (c *cache[K, V]) GetWithContext(ctx context.Context, key K) (V, error) {
	if err := c.checkState(); err != nil {
		var zero V
		return zero, err
	}

	// Check context cancellation
	if ctx.Err() != nil {
		var zero V
		return zero, errors.WrapError("Get", key, errors.ErrContextCanceled)
	}

	// Recover from panics
	defer func() {
		if errors.RecoverFromPanic("Get", key) {
			c.stats.IncErrors()
		}
	}()

	// Get value from memory first
	c.mu.RLock()
	items := c.getItems()
	if items != nil {
		if item, found := items[key]; found {
			// Check if item has expired
			if item.expiresAt.IsZero() || time.Now().Before(item.expiresAt) {
				// Update policy
				if c.policy != nil {
					c.policy.OnGet(key, item.value)
				}
				c.stats.IncHits()
				c.mu.RUnlock()
				c.emitEvent(EventTypeGet, key, item.value)
				return item.value, nil
			}
			// Item has expired, remove it
			c.mu.RUnlock()
			c.mu.Lock()
			delete(items, key)
			c.items.Store(&itemsMap[K, V]{items: items})
			c.mu.Unlock()
		} else {
			c.mu.RUnlock()
		}
	} else {
		c.mu.RUnlock()
	}

	// If not found in memory, try store
	if c.store != nil {
		value, found := c.store.Get(ctx, key)
		if found {
			// Update policy
			if c.policy != nil {
				c.policy.OnGet(key, value)
			}
			c.stats.IncHits()
			c.emitEvent(EventTypeGet, key, value)
			return value, nil
		}
	}

	c.stats.IncMisses()
	var zero V
	return zero, errors.WrapError("Get", key, errors.ErrKeyNotFound)
}

// Set stores a value in the cache with TTL
func (c *cache[K, V]) Set(key K, value V, ttl time.Duration) error {
	return c.SetWithContext(context.Background(), key, value, ttl)
}

// SetWithContext stores a value in the cache with context and TTL
func (c *cache[K, V]) SetWithContext(ctx context.Context, key K, value V, ttlDuration time.Duration) error {
	if err := c.checkState(); err != nil {
		return err
	}

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("Set", key, errors.ErrContextCanceled)
	}

	// Recover from panics
	defer func() {
		if errors.RecoverFromPanic("Set", key) {
			c.stats.IncErrors()
		}
	}()

	// Validate TTL before normalization
	if err := ttl.Validate(ttlDuration, c.ttlConfig); err != nil {
		c.stats.IncErrors()
		return err
	}

	// Normalize TTL duration
	ttlDuration = ttl.Normalize(ttlDuration, c.ttlConfig)

	// Create entry with expiration time
	expiresAt := ttl.GetExpirationTime(ttlDuration, c.ttlConfig)
	entry := &item[V]{
		value:     value,
		expiresAt: expiresAt,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Get current items map
	items := c.getItems()
	if items == nil {
		items = make(map[K]*item[V])
	}

	// Check if we need to evict items
	if c.maxSize > 0 && len(items) >= c.maxSize {
		// If policy is nil, we can't evict items
		if c.policy == nil {
			return errors.WrapError("Set", key, errors.ErrCapacityLimit)
		}

		// Evict one item at a time until we have space
		for len(items) >= c.maxSize {
			if keyToEvict, ok := c.policy.Evict(); ok {
				delete(items, keyToEvict)
				c.stats.Evictions.Add(1)
				c.emitEvent(EventTypeEviction, keyToEvict, value)
			} else {
				// If we can't evict any more items, return error
				return errors.WrapError("Set", key, errors.ErrCapacityLimit)
			}
		}
	}

	// Store the item
	items[key] = entry

	// Update the atomic value
	c.items.Store(&itemsMap[K, V]{items: items})

	// Update store if configured
	if c.store != nil {
		if err := c.store.Set(ctx, key, value, ttlDuration); err != nil {
			return fmt.Errorf("cache: Set: key=%v: %w", key, err)
		}
	}

	// Update policy
	if c.policy != nil {
		c.policy.OnSet(key, value, ttlDuration)
	}

	// Update metrics
	c.stats.IncSets()
	c.stats.Size.Store(int64(len(items)))

	// Emit event
	c.emitEvent(EventTypeSet, key, value)

	return nil
}

// Delete removes a value from the cache
func (c *cache[K, V]) Delete(key K) error {
	return c.DeleteWithContext(context.Background(), key)
}

// DeleteWithContext removes a value from the cache with context
func (c *cache[K, V]) DeleteWithContext(ctx context.Context, key K) error {
	if err := c.checkState(); err != nil {
		return err
	}

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("Delete", key, errors.ErrContextCanceled)
	}

	// Recover from panics
	defer func() {
		if errors.RecoverFromPanic("Delete", key) {
			c.stats.IncErrors()
		}
	}()

	// Update internal items map atomically
	c.mu.Lock()
	defer c.mu.Unlock()

	oldItemsPtr := c.items.Load()
	if oldItemsPtr == nil {
		c.stats.IncDeletes()
		return nil
	}

	oldItems := oldItemsPtr.(*itemsMap[K, V])
	if oldItems.items == nil {
		c.stats.IncDeletes()
		return nil
	}

	// Get the value before deleting for event emission
	var value V
	if item, exists := oldItems.items[key]; exists {
		value = item.value
	}

	// Update policy
	if c.policy != nil {
		c.policy.OnDelete(key)
	}

	newItems := &itemsMap[K, V]{items: make(map[K]*item[V], len(oldItems.items))}
	for k, v := range oldItems.items {
		if k != key {
			newItems.items[k] = v
		}
	}
	if _, exists := oldItems.items[key]; exists {
		c.stats.Size.Add(-1)
	}
	c.items.Store(newItems)

	// Delete value from store if it exists
	if c.store != nil {
		if err := c.store.Delete(ctx, key); err != nil {
			c.stats.IncErrors()
			return errors.WrapError("Delete", key, err)
		}
	}

	c.stats.IncDeletes()
	c.emitEvent(EventTypeDelete, key, value)
	return nil
}

// Clear removes all values from the cache
func (c *cache[K, V]) Clear() error {
	return c.ClearWithContext(context.Background())
}

// ClearWithContext removes all values from the cache with context
func (c *cache[K, V]) ClearWithContext(ctx context.Context) error {
	if err := c.checkState(); err != nil {
		return err
	}

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("Clear", nil, errors.ErrContextCanceled)
	}

	// Recover from panics
	defer func() {
		if errors.RecoverFromPanic("Clear", nil) {
			c.stats.IncErrors()
		}
	}()

	// Reset internal items map atomically
	c.mu.Lock()
	defer c.mu.Unlock()

	oldItemsPtr := c.items.Load()
	if oldItemsPtr != nil {
		oldItems := oldItemsPtr.(*itemsMap[K, V])
		if oldItems.items != nil {
			// Emit events for each item being cleared
			for key, item := range oldItems.items {
				c.emitEvent(EventTypeEviction, key, item.value)
			}
		}
	}

	// Create new empty items map
	c.items.Store(&itemsMap[K, V]{items: make(map[K]*item[V])})
	c.stats.Size.Store(0)

	// Clear store if it exists
	if c.store != nil {
		if err := c.store.Clear(ctx); err != nil {
			c.stats.IncErrors()
			return errors.WrapError("Clear", nil, err)
		}
	}

	c.stats.IncClears()
	return nil
}

// Size returns the number of items in the cache
func (c *cache[K, V]) Size() int {
	if c.closed.Load() {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := c.getItems()
	if items == nil {
		return 0
	}
	return len(items)
}

// Capacity returns the maximum number of items the cache can hold
func (c *cache[K, V]) Capacity() int64 {
	return c.stats.Capacity.Load()
}

// GetStats returns the current cache statistics
func (c *cache[K, V]) GetStats() StatsSnapshot {
	return StatsSnapshot{
		Size:                  c.stats.Size.Load(),
		Capacity:              c.stats.Capacity.Load(),
		Hits:                  c.stats.Hits.Load(),
		Misses:                c.stats.Misses.Load(),
		Evictions:             c.stats.Evictions.Load(),
		PanicCount:            c.stats.Errors.Load(),
		LastPanic:             time.Time{},
		LastOperationDuration: c.stats.LastOperationDuration.Load(),
	}
}

// ResetStats resets the cache statistics
func (c *cache[K, V]) ResetStats() {
	c.stats.Size.Store(0)
	c.stats.Hits.Store(0)
	c.stats.Misses.Store(0)
	c.stats.Evictions.Store(0)
	c.stats.Errors.Store(0)
	c.stats.LastOperationDuration.Store(0)
}

// MemoryUsage returns the current memory usage of the cache
func (c *cache[K, V]) MemoryUsage() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var total int64
	for k, v := range c.items.Load().(*itemsMap[K, V]).items {
		total += int64(unsafe.Sizeof(k))
		total += int64(unsafe.Sizeof(*v))
	}
	return total
}

// MaxMemory returns the maximum memory the cache can use
func (c *cache[K, V]) MaxMemory() int64 {
	return c.maxMemory
}

// OnEvent registers a callback function for cache events
func (c *cache[K, V]) OnEvent(callback CacheCallback[K, V]) {
	c.callbacksMu.Lock()
	c.callbacks = append(c.callbacks, callback)
	c.callbacksMu.Unlock()
}

// RemoveEventCallback removes a callback function
func (c *cache[K, V]) RemoveEventCallback(callback CacheCallback[K, V]) {
	c.callbacksMu.Lock()
	for i, cb := range c.callbacks {
		if fmt.Sprintf("%p", cb) == fmt.Sprintf("%p", callback) {
			c.callbacks = append(c.callbacks[:i], c.callbacks[i+1:]...)
			break
		}
	}
	c.callbacksMu.Unlock()
}

// GetMany retrieves multiple values from the cache
func (c *cache[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	result := make(map[K]V, len(keys))
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			value, err := c.GetWithContext(ctx, key)
			if err == nil {
				result[key] = value
			}
		}
	}
	return result
}

// SetMany stores multiple values in the cache
func (c *cache[K, V]) SetMany(ctx context.Context, entries map[K]V, ttl time.Duration) error {
	for key, value := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.Set(key, value, ttl); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteMany removes multiple values from the cache
func (c *cache[K, V]) DeleteMany(ctx context.Context, keys []K) error {
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.Delete(key); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetAll returns all items in the cache that haven't expired
func (c *cache[K, V]) GetAll() map[K]V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[K]V)
	for k, v := range c.items.Load().(*itemsMap[K, V]).items {
		if !v.expiresAt.IsZero() && v.expiresAt.After(time.Now()) {
			result[k] = v.value
		}
	}
	return result
}

// SetCapacity updates the cache capacity
func (c *cache[K, V]) SetCapacity(capacity int64) {
	c.stats.Capacity.Store(capacity)
}

// checkState checks the state of the cache
func (c *cache[K, V]) checkState() error {
	if c.closed.Load() {
		return errors.ErrCacheClosed
	}
	return nil
}

// Stats returns the current cache statistics
func (c *cache[K, V]) Stats() *Stats {
	return c.stats
}
