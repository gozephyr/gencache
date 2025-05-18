// Package store provides in-memory storage implementation for the cache.
package store

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/ttl"
)

// memoryStore implements Store interface with in-memory storage
type memoryStore[K comparable, V any] struct {
	mu sync.RWMutex
	// items stores the cache entries
	items map[K]*Entry[V]
	// maxSize is the maximum number of items the store can hold
	maxSize int
	// ttlConfig is the configuration for TTL behavior
	ttlConfig ttl.Config
	// maxMemory is the maximum memory usage in bytes
	maxMemory int64
	// memoryUsage is the current memory usage in bytes
	memoryUsage atomic.Int64
	// enableMemoryTracking enables memory usage tracking
	enableMemoryTracking bool
	stop                 chan struct{}
	stats                *Stats
}

// Stats tracks cache statistics
type Stats struct {
	Hits            atomic.Int64
	Misses          atomic.Int64
	Sets            atomic.Int64
	Deletes         atomic.Int64
	Clears          atomic.Int64
	Errors          atomic.Int64
	Size            atomic.Int64
	Capacity        atomic.Int64
	Evictions       atomic.Int64
	BatchOperations atomic.Int64
	BatchErrors     atomic.Int64
	BatchSuccess    atomic.Int64
}

// NewMemoryStore creates a new memory store
func NewMemoryStore[K comparable, V any](ctx context.Context, opts ...Option) (Store[K, V], error) {
	options := NewOptions()
	if err := options.Apply(opts...); err != nil {
		return nil, err
	}

	store := &memoryStore[K, V]{
		items:                make(map[K]*Entry[V]),
		maxSize:              options.MaxSize,
		ttlConfig:            options.TTLConfig,
		maxMemory:            options.MaxMemory,
		enableMemoryTracking: options.EnableMemoryTracking,
		stop:                 make(chan struct{}),
		stats:                &Stats{},
	}

	go store.startTTLCleanup(ctx)
	return store, nil
}

// Get retrieves a value from the store
func (m *memoryStore[K, V]) Get(ctx context.Context, key K) (V, bool) {
	m.mu.RLock()
	entry, exists := m.items[key]
	m.mu.RUnlock()
	if !exists {
		m.stats.Misses.Add(1)
		var zero V
		return zero, false
	}

	// Check if entry has expired
	if !entry.Expires.IsZero() && ttl.IsExpired(entry.Expires) {
		m.stats.Misses.Add(1)
		// Delete expired entry
		m.mu.Lock()
		delete(m.items, key)
		m.stats.Size.Store(int64(len(m.items)))
		m.mu.Unlock()
		var zero V
		return zero, false
	}

	m.stats.Hits.Add(1)
	// Use write lock to update entry fields safely
	m.mu.Lock()
	entry.LastAccess = time.Now()
	entry.AccessCount++
	m.mu.Unlock()
	return entry.Value, true
}

// calculateMemoryUsage calculates the memory usage of a key-value pair
func (m *memoryStore[K, V]) calculateMemoryUsage(key K, value V) int64 {
	var total int64
	// Size of the key
	total += int64(unsafe.Sizeof(key))
	// Size of the value
	total += int64(unsafe.Sizeof(value))

	// If value is a string, add its length
	switch v := any(value).(type) {
	case string:
		total += int64(len(v))
	case []byte:
		total += int64(len(v))
	}

	// Size of the Entry struct
	total += int64(unsafe.Sizeof(Entry[V]{}))
	// Size of the map entry overhead
	total += int64(unsafe.Sizeof(map[K]*Entry[V]{}))
	return total
}

// Set stores a value in the store
func (m *memoryStore[K, V]) Set(ctx context.Context, key K, value V, ttlDuration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("Set", key, errors.ErrContextCanceled)
	}

	// Check size limit - only if this is a new key
	if m.maxSize > 0 {
		_, exists := m.items[key]
		if !exists && len(m.items) >= m.maxSize {
			return errors.WrapError("Set", key, errors.ErrCapacityLimit)
		}
	}

	// Check memory limit if tracking is enabled
	if m.enableMemoryTracking {
		newItemMemory := m.calculateMemoryUsage(key, value)
		currentMemory := m.memoryUsage.Load()
		if m.maxMemory > 0 && currentMemory+newItemMemory > m.maxMemory {
			return errors.WrapError("Set", key, errors.ErrMemoryLimit)
		}
	}

	// Normalize TTL duration
	ttlDuration = ttl.Normalize(ttlDuration, m.ttlConfig)

	// Create entry with expiration time
	expiresAt := ttl.GetExpirationTime(ttlDuration, m.ttlConfig)
	entry := &Entry[V]{
		Value:       value,
		Expires:     expiresAt,
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
	}

	// Update memory usage if tracking is enabled
	if m.enableMemoryTracking {
		oldEntry, exists := m.items[key]
		if exists {
			oldMemory := m.calculateMemoryUsage(key, oldEntry.Value)
			m.memoryUsage.Add(-oldMemory)
		}
		newMemory := m.calculateMemoryUsage(key, value)
		m.memoryUsage.Add(newMemory)
	}

	// Store the item
	m.items[key] = entry
	m.stats.Sets.Add(1)
	m.stats.Size.Store(int64(len(m.items)))

	return nil
}

// Delete removes a value from the store
func (m *memoryStore[K, V]) Delete(ctx context.Context, key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("Delete", key, errors.ErrContextCanceled)
	}

	// Update memory usage if tracking is enabled
	if m.enableMemoryTracking {
		if entry, exists := m.items[key]; exists {
			memory := m.calculateMemoryUsage(key, entry.Value)
			m.memoryUsage.Add(-memory)
		}
	}

	delete(m.items, key)
	m.stats.Deletes.Add(1)
	m.stats.Size.Store(int64(len(m.items)))

	return nil
}

// Clear removes all values from the store
func (m *memoryStore[K, V]) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("Clear", nil, errors.ErrContextCanceled)
	}

	// Reset memory usage if tracking is enabled
	if m.enableMemoryTracking {
		m.memoryUsage.Store(0)
	}

	m.items = make(map[K]*Entry[V])
	m.stats.Clears.Add(1)
	m.stats.Size.Store(0)

	return nil
}

// Size returns the number of items in the store
func (m *memoryStore[K, V]) Size(ctx context.Context) int {
	select {
	case <-ctx.Done():
		return 0
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// Capacity returns the maximum number of items the store can hold
func (m *memoryStore[K, V]) Capacity(ctx context.Context) int {
	return m.maxSize
}

// Keys returns all keys in the store
func (m *memoryStore[K, V]) Keys(ctx context.Context) []K {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]K, 0, len(m.items))
	for k := range m.items {
		keys = append(keys, k)
	}
	return keys
}

// MemoryUsage returns the current memory usage of the store
func (m *memoryStore[K, V]) MemoryUsage(ctx context.Context) int64 {
	if !m.enableMemoryTracking {
		return 0
	}
	return m.memoryUsage.Load()
}

// MaxMemory returns the maximum allowed memory usage in bytes
func (m *memoryStore[K, V]) MaxMemory(ctx context.Context) int64 {
	return m.maxMemory
}

// GetMany retrieves multiple values from the store
func (m *memoryStore[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	result := make(map[K]V)
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			if value, ok := m.Get(ctx, key); ok {
				result[key] = value
			}
		}
	}
	return result
}

// SetMany sets multiple key-value pairs in the store
func (m *memoryStore[K, V]) SetMany(ctx context.Context, entries map[K]V, ttlDuration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check context cancellation
	if ctx.Err() != nil {
		return errors.WrapError("SetMany", nil, errors.ErrContextCanceled)
	}

	// Check capacity limit for new entries
	if m.maxSize > 0 {
		newKeys := make(map[K]struct{})
		for k := range entries {
			if _, exists := m.items[k]; !exists {
				newKeys[k] = struct{}{}
			}
		}
		if len(m.items)+len(newKeys) > m.maxSize {
			return errors.WrapError("SetMany", nil, errors.ErrCapacityLimit)
		}
	}

	// Check memory limit if tracking is enabled
	if m.enableMemoryTracking {
		var totalNewMemory int64
		for k, v := range entries {
			if _, exists := m.items[k]; !exists {
				totalNewMemory += m.calculateMemoryUsage(k, v)
			}
		}
		if m.maxMemory > 0 && m.memoryUsage.Load()+totalNewMemory > m.maxMemory {
			return errors.WrapError("SetMany", nil, errors.ErrMemoryLimit)
		}
	}

	// Process each entry
	for key, value := range entries {
		// Normalize TTL duration
		ttlDuration := ttl.Normalize(ttlDuration, m.ttlConfig)

		// Create entry with expiration time
		expiresAt := ttl.GetExpirationTime(ttlDuration, m.ttlConfig)
		entry := &Entry[V]{
			Value:       value,
			Expires:     expiresAt,
			CreatedAt:   time.Now(),
			LastAccess:  time.Now(),
			AccessCount: 1,
		}

		// Update memory usage if tracking is enabled
		if m.enableMemoryTracking {
			oldEntry, exists := m.items[key]
			if exists {
				oldMemory := m.calculateMemoryUsage(key, oldEntry.Value)
				m.memoryUsage.Add(-oldMemory)
			}
			newMemory := m.calculateMemoryUsage(key, value)
			m.memoryUsage.Add(newMemory)
		}

		// Store the item
		m.items[key] = entry
		m.stats.Sets.Add(1)
	}

	// Update size atomically
	m.stats.Size.Store(int64(len(m.items)))

	return nil
}

// DeleteMany deletes multiple keys from the store
func (m *memoryStore[K, V]) DeleteMany(ctx context.Context, keys []K) error {
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_ = m.Delete(ctx, key)
		}
	}
	return nil
}

// startTTLCleanup begins the TTL cleanup process
func (m *memoryStore[K, V]) startTTLCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.cleanupExpiredEntries()
			case <-m.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// cleanupExpiredEntries removes expired entries from the store
func (m *memoryStore[K, V]) cleanupExpiredEntries() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, entry := range m.items {
		if !entry.Expires.IsZero() && ttl.IsExpired(entry.Expires) {
			delete(m.items, key)
			m.stats.Evictions.Add(1)
		}
	}
}

// Close closes the store
func (m *memoryStore[K, V]) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return errors.WrapError("Close", nil, errors.ErrContextCanceled)
	default:
	}
	close(m.stop)
	return nil
}

// ForceCleanup triggers cleanup of expired entries (for testing)
func (m *memoryStore[K, V]) ForceCleanup() {
	m.cleanupExpiredEntries()
}
