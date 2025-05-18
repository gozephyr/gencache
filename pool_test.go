package gencache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var errKeyNotFound = errors.New("key not found")

func TestObjectPool(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		pool := NewObjectPool(func() []byte {
			return make([]byte, 1024)
		})

		// Test Get and Put
		obj1 := pool.Get()
		require.NotNil(t, obj1, "Get should return a non-nil object")
		require.Len(t, obj1, 1024, "Object should have correct size")

		pool.Put(obj1)
		obj2 := pool.Get()
		require.NotNil(t, obj2, "Get should return a non-nil object after Put")
		require.Len(t, obj2, 1024, "Object should have correct size after Put")
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		pool := NewObjectPool(func() []byte {
			return make([]byte, 1024)
		})

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				obj := pool.Get()
				require.NotNil(t, obj, "Get should return a non-nil object")
				pool.Put(obj)
			}()
		}
		wg.Wait()
	})
}

func TestEntryPool(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		pool := NewEntryPool[string]()

		// Test Get and Put
		entry1 := pool.Get()
		require.Equal(t, int64(0), entry1.AccessCount, "New entry should have zero access count")

		entry1.AccessCount = 5
		pool.Put(entry1)

		entry2 := pool.Get()
		require.Equal(t, int64(0), entry2.AccessCount, "Reused entry should have reset access count")
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		pool := NewEntryPool[string]()

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				entry := pool.Get()
				entry.AccessCount++
				pool.Put(entry)
			}()
		}
		wg.Wait()
	})
}

func TestBatchResultPool(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		pool := NewBatchResultPool[string, int]()

		// Test Get and Put
		result1 := pool.Get()
		require.Nil(t, result1.Error, "New result should have nil error")

		result1.Error = errKeyNotFound
		pool.Put(result1)

		result2 := pool.Get()
		require.Nil(t, result2.Error, "Reused result should have reset error")
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		pool := NewBatchResultPool[string, int]()

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result := pool.Get()
				result.Error = errKeyNotFound
				pool.Put(result)
			}()
		}
		wg.Wait()
	})
}

// --- Mock cache for pooled cache tests ---
type pooledMockCache[K comparable, V any] struct {
	data map[K]V
	mu   sync.RWMutex
}

func newPooledMockCache[K comparable, V any]() *pooledMockCache[K, V] {
	return &pooledMockCache[K, V]{data: make(map[K]V)}
}

func (m *pooledMockCache[K, V]) Get(key K) (V, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		var zero V
		return zero, errKeyNotFound
	}
	return v, nil
}

func (m *pooledMockCache[K, V]) Set(key K, value V, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *pooledMockCache[K, V]) Delete(key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *pooledMockCache[K, V]) GetWithContext(ctx context.Context, key K) (V, error) {
	return m.Get(key)
}

func (m *pooledMockCache[K, V]) SetWithContext(ctx context.Context, key K, value V, ttl time.Duration) error {
	return m.Set(key, value, ttl)
}

func (m *pooledMockCache[K, V]) DeleteWithContext(ctx context.Context, key K) error {
	return m.Delete(key)
}

func (m *pooledMockCache[K, V]) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[K]V)
	return nil
}

func (m *pooledMockCache[K, V]) ClearWithContext(ctx context.Context) error {
	return m.Clear()
}

func (m *pooledMockCache[K, V]) Stats() *Stats                  { return &Stats{} }
func (m *pooledMockCache[K, V]) OnEvent(cb CacheCallback[K, V]) {}

func (m *pooledMockCache[K, V]) Close() error {
	return nil
}

func TestPooledCache(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		mock := newPooledMockCache[string, *int]()
		config := DefaultPoolConfig()
		pc := NewPooledCache[string, int](mock, config)
		t.Cleanup(func() { pc.Close() })

		// Test Set and Get
		v1 := 100
		err := pc.Set("key1", v1, time.Hour)
		require.NoError(t, err)
		value, err := pc.Get("key1")
		require.NoError(t, err)
		require.Equal(t, v1, value, "Value should match what was set")

		// Test Delete
		err = pc.Delete("key1")
		require.NoError(t, err)
		_, err = pc.Get("key1")
		require.Error(t, err, "Value should be deleted")
	})

	t.Run("Pool Statistics", func(t *testing.T) {
		mock := newPooledMockCache[string, *int]()
		config := DefaultPoolConfig()
		config.CleanupPeriod = 100 * time.Millisecond
		pc := NewPooledCache[string, int](mock, config)
		t.Cleanup(func() { pc.Close() })

		// Test initial stats
		stats := pc.GetPoolStats()
		require.Equal(t, int64(config.MaxSize), stats.MaxSize.Load(), "MaxSize should match config")
		require.Equal(t, int64(config.MinSize), stats.MinSize.Load(), "MinSize should match config")
		require.Equal(t, int64(0), stats.TotalCreated.Load(), "Initial TotalCreated should be 0")
		require.Equal(t, int64(0), stats.TotalDestroyed.Load(), "Initial TotalDestroyed should be 0")

		// Test stats after operations
		v1 := 100
		v2 := 200
		err := pc.Set("key1", v1, time.Hour)
		require.NoError(t, err)
		err = pc.Set("key2", v2, time.Hour)
		require.NoError(t, err)
		_, err = pc.Get("key1")
		require.NoError(t, err)
		_, err = pc.Get("key2")
		require.NoError(t, err)

		// Force the pool to create new objects
		pool := pc.(interface{ TestGetFromPool() any })
		for i := 0; i < 5; i++ {
			pool.TestGetFromPool()
		}

		// Directly call the pool's Get method to ensure TotalCreated increases
		for i := 0; i < 5; i++ {
			_ = pool.TestGetFromPool()
			time.Sleep(10 * time.Millisecond) // Small delay to avoid race conditions
			stats = pc.GetPoolStats()
			require.Greater(t, stats.TotalCreated.Load(), int64(0), "TotalCreated should increase")
		}

		// Wait for cleanup
		time.Sleep(200 * time.Millisecond)

		// Check final stats
		stats = pc.GetPoolStats()
		require.LessOrEqual(t, stats.CurrentSize.Load(), int64(config.MaxSize), "Pool size should be within limits")
	})

	t.Run("Cleanup", func(t *testing.T) {
		mock := newPooledMockCache[string, *int]()
		config := DefaultPoolConfig()
		config.MaxSize = 5
		config.MinSize = 2
		config.CleanupPeriod = 100 * time.Millisecond // Shorter period
		config.ShrinkFactor = 0.5
		pc := NewPooledCache[string, int](mock, config)

		// Fill the pool beyond max size
		for i := 0; i < 8; i++ { // Reduced to 8 items
			v := i
			err := pc.Set("key"+string(rune(i)), v, time.Hour)
			require.NoError(t, err)
		}

		// Get initial stats
		initialStats := pc.GetPoolStats()
		initialSize := initialStats.CurrentSize.Load()
		require.Greater(t, initialSize, int64(config.MaxSize), "Initial size should be greater than max size")

		// Wait for cleanup
		time.Sleep(300 * time.Millisecond)

		// Check if pool was cleaned up
		stats := pc.GetPoolStats()
		require.LessOrEqual(t, stats.CurrentSize.Load(), int64(config.MaxSize), "Pool size should be reduced after cleanup")
		require.Less(t, stats.CurrentSize.Load(), initialSize, "Pool size should be smaller after cleanup")

		// Close the cache
		err := pc.Close()
		require.NoError(t, err, "Close should not timeout")

		// Verify final stats
		finalStats := pc.GetPoolStats()
		require.LessOrEqual(t, finalStats.CurrentSize.Load(), int64(config.MaxSize), "Final pool size should be within limits")
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		mock := newPooledMockCache[string, *int]()
		config := DefaultPoolConfig()
		pc := NewPooledCache[string, int](mock, config)
		t.Cleanup(func() { pc.Close() })

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				key := "key" + string(rune(i))
				err := pc.Set(key, i, time.Hour)
				require.NoError(t, err)
				value, err := pc.Get(key)
				require.NoError(t, err)
				require.Equal(t, i, value, "Value should match what was set")
			}(i)
		}
		wg.Wait()
	})
}

func TestPoolStats(t *testing.T) {
	mock := newPooledMockCache[string, *string]()
	config := DefaultPoolConfig()
	pc := NewPooledCache[string, string](mock, config)

	// Perform some operations
	v1 := "value1"
	v2 := "value2"
	err := pc.Set("test1", v1, time.Hour)
	require.NoError(t, err)
	err = pc.Set("test2", v2, time.Hour)
	require.NoError(t, err)
	_, err = pc.Get("test1")
	require.NoError(t, err)
	_, err = pc.Get("test2")
	require.NoError(t, err)

	// Check stats
	// Explicitly exercise the pool to ensure stats are updated
	pool := pc.(interface{ TestGetFromPool() any })
	for i := 0; i < 5; i++ {
		_ = pool.TestGetFromPool()
	}
	stats := pc.GetPoolStats()
	require.Greater(t, stats.TotalCreated.Load(), int64(0), "Should record created objects")
	require.Greater(t, stats.CurrentSize.Load(), int64(0), "Should record current size")
	require.Equal(t, int64(config.MaxSize), stats.MaxSize.Load(), "Should record max size")
	require.Equal(t, int64(config.MinSize), stats.MinSize.Load(), "Should record min size")

	// Test concurrent stats updates
	const goroutines = 10
	const iterations = 1000
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				v := "value"
				err := pc.Set("test", v, time.Hour)
				require.NoError(t, err)
				_, err = pc.Get("test")
				require.NoError(t, err)
			}
		}()
	}
	wg.Wait()

	// Verify final stats
	stats = pc.GetPoolStats()
	require.Greater(t, stats.TotalCreated.Load(), int64(0), "Should record created objects")
	require.Greater(t, stats.CurrentSize.Load(), int64(0), "Should record current size")
	require.Equal(t, int64(config.MaxSize), stats.MaxSize.Load(), "Should record max size")
	require.Equal(t, int64(config.MinSize), stats.MinSize.Load(), "Should record min size")

	// Test stats reset
	pc.ResetPoolStats()
	stats = pc.GetPoolStats()
	require.Equal(t, int64(0), stats.TotalCreated.Load(), "Stats should be reset")
	require.Equal(t, int64(0), stats.TotalDestroyed.Load(), "Stats should be reset")
	require.Equal(t, int64(0), stats.CurrentSize.Load(), "Stats should be reset")
	require.Equal(t, int64(config.MaxSize), stats.MaxSize.Load(), "Max size should be preserved")
	require.Equal(t, int64(config.MinSize), stats.MinSize.Load(), "Min size should be preserved")
}
