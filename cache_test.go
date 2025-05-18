package gencache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	cacheerrors "github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/policy"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestCacheBasicOperations(t *testing.T) {
	cache := New[string, string]()
	defer cache.Close()

	// Test Set and Get
	err := cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)

	value, err := cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, "value1", value)

	// Test Delete
	err = cache.Delete("key1")
	require.NoError(t, err)

	_, err = cache.Get("key1")
	require.Error(t, err)
	require.True(t, cacheerrors.IsKeyNotFound(err))

	// Test Clear
	err = cache.Set("key2", "value2", time.Minute)
	require.NoError(t, err)
	err = cache.Set("key3", "value3", time.Minute)
	require.NoError(t, err)

	err = cache.Clear()
	require.NoError(t, err)

	_, err = cache.Get("key2")
	require.Error(t, err)
	_, err = cache.Get("key3")
	require.Error(t, err)
}

func TestCacheWithContext(t *testing.T) {
	cache := New[string, string]()
	defer cache.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test SetWithContext with canceled context
	err := cache.SetWithContext(ctx, "key1", "value1", time.Minute)
	require.Error(t, err)
	require.True(t, cacheerrors.IsContextCanceled(err))

	// Test GetWithContext with canceled context
	_, err = cache.GetWithContext(ctx, "key1")
	require.Error(t, err)
	require.True(t, cacheerrors.IsContextCanceled(err))

	// Test DeleteWithContext with canceled context
	err = cache.DeleteWithContext(ctx, "key1")
	require.Error(t, err)
	require.True(t, cacheerrors.IsContextCanceled(err))

	// Test ClearWithContext with canceled context
	err = cache.ClearWithContext(ctx)
	require.Error(t, err)
	require.True(t, cacheerrors.IsContextCanceled(err))
}

func TestCacheCleanup(t *testing.T) {
	cache := New[string, string](
		WithCleanupInterval[string, string](time.Millisecond*100),
		WithTTLConfig[string, string](ttl.Config{
			MinTTL: time.Millisecond * 10,
			MaxTTL: time.Hour,
		}),
	)
	defer cache.Close()

	// Set items with short TTL
	err := cache.Set("key1", "value1", time.Millisecond*50)
	require.NoError(t, err)
	err = cache.Set("key2", "value2", time.Millisecond*50)
	require.NoError(t, err)

	// Wait for cleanup
	time.Sleep(time.Millisecond * 200)

	// Verify items are cleaned up
	_, err = cache.Get("key1")
	require.Error(t, err)
	_, err = cache.Get("key2")
	require.Error(t, err)
}

func TestCacheClose(t *testing.T) {
	cache := New[string, string]()

	// Test operations after close
	err := cache.Close()
	require.NoError(t, err)

	// All operations should fail after close
	err = cache.Set("key1", "value1", time.Minute)
	require.Error(t, err)
	require.True(t, cacheerrors.IsCacheClosed(err))

	_, err = cache.Get("key1")
	require.Error(t, err)
	require.True(t, cacheerrors.IsCacheClosed(err))

	err = cache.Delete("key1")
	require.Error(t, err)
	require.True(t, cacheerrors.IsCacheClosed(err))

	err = cache.Clear()
	require.Error(t, err)
	require.True(t, cacheerrors.IsCacheClosed(err))

	// Test double close
	err = cache.Close()
	require.NoError(t, err)
}

func TestCacheEviction(t *testing.T) {
	cache := New[string, string](
		WithMaxSize[string, string](2),
		WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(2))),
	)
	defer cache.Close()

	// Fill cache to capacity
	err := cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)
	err = cache.Set("key2", "value2", time.Minute)
	require.NoError(t, err)

	// Add one more item to trigger eviction
	err = cache.Set("key3", "value3", time.Minute)
	require.NoError(t, err)

	// Verify one item was evicted
	stats := cache.Stats()
	require.Equal(t, int64(1), stats.Evictions.Load())
}

func TestCacheWithStore(t *testing.T) {
	ctx := context.Background()
	memStore, err := store.NewMemoryStore[string, string](ctx)
	require.NoError(t, err)

	cache := New[string, string](
		WithStore[string, string](memStore),
	)
	defer cache.Close()

	// Test Set and Get with store
	err = cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)

	value, err := cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, "value1", value)

	// Verify store has the value
	storeValue, found := memStore.Get(ctx, "key1")
	require.True(t, found)
	require.Equal(t, "value1", storeValue)
}

func TestCacheWithStats(t *testing.T) {
	cache := New[string, string]()
	defer cache.Close()

	// Test stats after operations
	err := cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)

	value, err := cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, "value1", value)

	_, err = cache.Get("nonexistent")
	require.Error(t, err)

	stats := cache.Stats()
	require.Equal(t, int64(1), stats.Hits.Load())
	require.Equal(t, int64(1), stats.Misses.Load())
	require.Equal(t, int64(1), stats.Sets.Load())
}

func TestCacheWithPanicRecovery(t *testing.T) {
	cache := New[string, string]()
	defer cache.Close()

	// Test panic recovery in Get
	value, err := cache.Get("key1")
	require.Error(t, err)
	require.Empty(t, value)

	// Test panic recovery in Set
	err = cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)

	// Test panic recovery in Delete
	err = cache.Delete("key1")
	require.NoError(t, err)

	// Test panic recovery in Clear
	err = cache.Clear()
	require.NoError(t, err)
}

func TestCacheWithEvents(t *testing.T) {
	cache := New[string, string]()
	defer cache.Close()

	events := make([]CacheEvent[string, string], 0)
	cache.OnEvent(func(event CacheEvent[string, string]) {
		events = append(events, event)
	})

	// Test Set event
	err := cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)
	require.Equal(t, EventTypeSet, events[0].Type)
	require.Equal(t, "key1", events[0].Key)
	require.Equal(t, "value1", events[0].Value)

	// Test Get event
	_, err = cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, EventTypeGet, events[1].Type)
	require.Equal(t, "key1", events[1].Key)
	require.Equal(t, "value1", events[1].Value)

	// Test Delete event
	err = cache.Delete("key1")
	require.NoError(t, err)
	require.Equal(t, EventTypeDelete, events[2].Type)
	require.Equal(t, "key1", events[2].Key)
	require.Equal(t, "value1", events[2].Value)
}

func TestCacheConcurrency(t *testing.T) {
	cache := New[string, int](WithMaxSize[string, int](2000))
	defer cache.Close()

	var wg sync.WaitGroup
	concurrency := 10
	iterations := 100

	// Concurrently set values
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := "key" + string(rune(id)) + string(rune(j))
				err := cache.SetWithContext(context.Background(), key, id*iterations+j, 0)
				require.NoError(t, err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all values were set correctly
	for i := 0; i < concurrency; i++ {
		for j := 0; j < iterations; j++ {
			key := "key" + string(rune(i)) + string(rune(j))
			val, err := cache.GetWithContext(context.Background(), key)
			require.NoError(t, err)
			require.Equal(t, i*iterations+j, val)
		}
	}
}

func TestCacheWithMaxSize(t *testing.T) {
	store := newMockStore[string, int]()
	cache := New[string, int](
		WithStore[string, int](store),
		WithMaxSize[string, int](5),
	)

	// Fill cache beyond max size
	for i := 0; i < 10; i++ {
		err := cache.Set(fmt.Sprintf("key%d", i), i, time.Hour)
		require.NoError(t, err)
	}

	// Verify size is maintained
	c, ok := cache.(interface{ Size() int })
	require.True(t, ok)
	require.LessOrEqual(t, c.Size(), 5)
}

func TestCacheWithBatchOperations(t *testing.T) {
	store := newMockStore[string, int]()
	cache := New[string, int](
		WithStore[string, int](store),
	)

	// Test getting multiple keys
	results := make(map[string]int)
	for _, key := range []string{"key1", "key2", "key3", "key4"} {
		value, err := cache.Get(key)
		if err == nil {
			results[key] = value
		}
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	// Test setting multiple keys
	entries := map[string]int{
		"key1": 100,
		"key2": 200,
		"key3": 300,
	}

	for key, value := range entries {
		err := cache.Set(key, value, time.Hour)
		require.NoError(t, err)
	}

	// Verify values were set
	for key, expectedValue := range entries {
		value, err := cache.Get(key)
		require.NoError(t, err)
		require.Equal(t, expectedValue, value)
	}

	// Test deleting multiple keys
	for _, key := range []string{"key1", "key2", "key4"} {
		err := cache.Delete(key)
		require.NoError(t, err)
	}

	// Verify keys were deleted
	_, err := cache.Get("key1")
	require.Error(t, err)
	_, err = cache.Get("key2")
	require.Error(t, err)
	value, err := cache.Get("key3")
	require.NoError(t, err)
	require.Equal(t, 300, value)
}

// mockStore implements the Store interface for testing
type mockStore[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

func newMockStore[K comparable, V any]() store.Store[K, V] {
	return &mockStore[K, V]{
		items: make(map[K]V),
	}
}

func (m *mockStore[K, V]) Get(ctx context.Context, key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.items[key]
	return val, ok
}

func (m *mockStore[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	result := make(map[K]V)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			if val, ok := m.items[key]; ok {
				result[key] = val
			}
		}
	}
	return result
}

func (m *mockStore[K, V]) Set(ctx context.Context, key K, value V, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
	return nil
}

func (m *mockStore[K, V]) SetMany(ctx context.Context, entries map[K]V, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			m.items[key] = value
		}
	}
	return nil
}

func (m *mockStore[K, V]) Delete(ctx context.Context, key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}

func (m *mockStore[K, V]) DeleteMany(ctx context.Context, keys []K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			delete(m.items, key)
		}
	}
	return nil
}

func (m *mockStore[K, V]) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[K]V)
	return nil
}

func (m *mockStore[K, V]) Size(ctx context.Context) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *mockStore[K, V]) Capacity(ctx context.Context) int {
	return 1000 // Mock capacity
}

func (m *mockStore[K, V]) Keys(ctx context.Context) []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]K, 0, len(m.items))
	for k := range m.items {
		keys = append(keys, k)
	}
	return keys
}

func (m *mockStore[K, V]) MemoryUsage(ctx context.Context) int64 {
	return 0 // Mock memory usage
}

func (m *mockStore[K, V]) MaxMemory(ctx context.Context) int64 {
	return 0 // Mock max memory
}

func (m *mockStore[K, V]) Close(ctx context.Context) error {
	return nil
}
