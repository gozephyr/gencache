package gencache

import (
	"context"
	"sync"
	"testing"
	"time"

	gencacheerrors "github.com/gozephyr/gencache/errors"
	"github.com/stretchr/testify/require"
)

// mockCache implements Cache interface for testing
type mockCache[K comparable, V any] struct {
	store map[K]V
	mu    sync.RWMutex
	stats *Stats
}

func newMockCache[K comparable, V any]() *mockCache[K, V] {
	return &mockCache[K, V]{
		store: make(map[K]V),
		stats: &Stats{},
	}
}

func (m *mockCache[K, V]) Get(key K) (V, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.store[key]
	if !ok {
		var zero V
		return zero, gencacheerrors.ErrKeyNotFound
	}
	return val, nil
}

func (m *mockCache[K, V]) GetWithContext(ctx context.Context, key K) (V, error) {
	select {
	case <-ctx.Done():
		var zero V
		return zero, gencacheerrors.ErrContextCanceled
	default:
		return m.Get(key)
	}
}

func (m *mockCache[K, V]) Set(key K, value V, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
	return nil
}

func (m *mockCache[K, V]) SetWithContext(ctx context.Context, key K, value V, ttl time.Duration) error {
	select {
	case <-ctx.Done():
		return gencacheerrors.ErrContextCanceled
	default:
		return m.Set(key, value, ttl)
	}
}

func (m *mockCache[K, V]) Delete(key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

func (m *mockCache[K, V]) DeleteWithContext(ctx context.Context, key K) error {
	select {
	case <-ctx.Done():
		return gencacheerrors.ErrContextCanceled
	default:
		return m.Delete(key)
	}
}

func (m *mockCache[K, V]) Close() error {
	return nil
}

func (m *mockCache[K, V]) Capacity() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.store))
}

func (m *mockCache[K, V]) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store = make(map[K]V)
	return nil
}

func (m *mockCache[K, V]) ClearWithContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return gencacheerrors.ErrContextCanceled
	default:
		return m.Clear()
	}
}

func (m *mockCache[K, V]) DeleteMany(ctx context.Context, keys []K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			delete(m.store, key)
		}
	}
	return nil
}

func (m *mockCache[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	result := make(map[K]V)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			if val, ok := m.store[key]; ok {
				result[key] = val
			}
		}
	}
	return result
}

func (m *mockCache[K, V]) MaxMemory() int64 {
	return 0 // Mock implementation returns 0 for unlimited memory
}

func (m *mockCache[K, V]) MemoryUsage() int64 {
	return 0 // Mock implementation returns 0 for memory usage
}

func (m *mockCache[K, V]) RegisterCallback(callback CacheCallback[K, V]) {
	// Mock implementation does nothing
}

func (m *mockCache[K, V]) ResetStats() {
	// Mock implementation does nothing
}

func (m *mockCache[K, V]) SetMany(ctx context.Context, entries map[K]V, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			m.store[key] = value
		}
	}
	return nil
}

func (m *mockCache[K, V]) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.store)
}

func (m *mockCache[K, V]) UnregisterCallback(callback CacheCallback[K, V]) {
	// Mock implementation does nothing
}

func (m *mockCache[K, V]) GetAll() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[K]V, len(m.store))
	for k, v := range m.store {
		result[k] = v
	}
	return result
}

func (m *mockCache[K, V]) GetStats() StatsSnapshot {
	return StatsSnapshot{
		Size:                  int64(len(m.store)),
		Capacity:              0,
		Hits:                  0,
		Misses:                0,
		Evictions:             0,
		PanicCount:            0,
		LastPanic:             time.Time{},
		LastOperationDuration: 0,
	}
}

func (m *mockCache[K, V]) OnEvent(callback CacheCallback[K, V]) {
	// No-op for mock cache
}

func (m *mockCache[K, V]) RemoveEventCallback(callback CacheCallback[K, V]) {
	// No-op for mock cache
}

func (m *mockCache[K, V]) SetCapacity(capacity int64) {
	// No-op for mock cache
}

func (m *mockCache[K, V]) Stats() *Stats {
	return m.stats
}

func TestNewBatchCache(t *testing.T) {
	mock := newMockCache[string, int]()
	config := DefaultBatchConfig()
	bc := NewBatchCache[string, int](mock, config)

	require.NotNil(t, bc, "NewBatchCache should not return nil")
}

func TestGetMany(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	// Setup test data
	testData := map[string]int{
		"key1": 1,
		"key2": 2,
		"key3": 3,
	}
	for k, v := range testData {
		err := mock.Set(k, v, 0)
		require.NoError(t, err)
	}

	// Test successful retrieval
	ctx := context.Background()
	keys := []string{"key1", "key2", "key3"}
	result := bc.GetMany(ctx, keys)

	require.Equal(t, len(testData), len(result), "Expected correct number of results")
	for k, v := range testData {
		val, err := mock.Get(k)
		require.NoError(t, err)
		require.Equal(t, v, val, "Value for key %s should match", k)
	}

	// Test with non-existent keys
	nonExistentKeys := []string{"key4", "key5"}
	result = bc.GetMany(ctx, nonExistentKeys)
	require.Empty(t, result, "Result should be empty for non-existent keys")

	// Test batch size limit
	largeKeys := make([]string, 2000)
	for i := range largeKeys {
		largeKeys[i] = "key" + string(rune(i))
	}
	result = bc.GetMany(ctx, largeKeys)
	require.LessOrEqual(t, len(result), DefaultBatchConfig().MaxBatchSize, "Result size should not exceed MaxBatchSize")
}

func TestSetMany(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	// Test successful batch set
	ctx := context.Background()
	entries := map[string]int{
		"key1": 1,
		"key2": 2,
		"key3": 3,
	}

	err := bc.SetMany(ctx, entries, time.Second)
	require.NoError(t, err, "SetMany should not return error")

	// Verify values were set
	for k, v := range entries {
		val, err := mock.Get(k)
		require.NoError(t, err)
		require.Equal(t, v, val, "Value for key %s should match", k)
	}

	// Test batch size limit
	largeEntries := make(map[string]int, 2000)
	for i := 0; i < 2000; i++ {
		largeEntries["key"+string(rune(i))] = i
	}
	err = bc.SetMany(ctx, largeEntries, time.Second)
	require.NoError(t, err, "SetMany with large batch should not return error")
}

func TestDeleteMany(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	// Setup test data
	testData := map[string]int{
		"key1": 1,
		"key2": 2,
		"key3": 3,
	}
	for k, v := range testData {
		err := mock.Set(k, v, 0)
		require.NoError(t, err)
	}

	// Test successful batch delete
	ctx := context.Background()
	keys := []string{"key1", "key2"}
	err := bc.DeleteMany(ctx, keys)
	require.NoError(t, err, "DeleteMany should not return error")

	// Verify keys were deleted
	for _, k := range keys {
		_, err := mock.Get(k)
		require.ErrorIs(t, err, gencacheerrors.ErrKeyNotFound, "Key %s should be deleted", k)
	}

	// Verify other keys still exist
	val, err := mock.Get("key3")
	require.NoError(t, err)
	require.Equal(t, 3, val, "Key3's value should be unchanged")
}

func TestBatchMetrics(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	// Perform some operations
	ctx := context.Background()
	bc.GetMany(ctx, []string{"key1", "key2"})
	err := bc.SetMany(ctx, map[string]int{"key1": 1}, time.Second)
	require.NoError(t, err, "SetMany should not return error")
	err = bc.DeleteMany(ctx, []string{"key1"})
	require.NoError(t, err, "DeleteMany should not return error")

	// Check metrics
	metrics := bc.GetBatchMetrics()
	require.Equal(t, int64(3), metrics.TotalOperations.Load(), "Expected 3 total operations")
	require.Equal(t, int64(4), metrics.TotalItems.Load(), "Expected 4 total items")

	// Test reset metrics
	bc.ResetBatchMetrics()
	metrics = bc.GetBatchMetrics()
	require.Equal(t, int64(0), metrics.TotalOperations.Load(), "Expected 0 total operations after reset")
}

func TestContextCancellation(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	// Test GetMany with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := bc.GetMany(ctx, []string{"key1", "key2"})
	require.Empty(t, result, "Result should be empty with cancelled context")

	// Test SetMany with cancelled context
	err := bc.SetMany(ctx, map[string]int{"key1": 1}, time.Second)
	require.ErrorIs(t, err, context.Canceled, "Expected context.Canceled error")

	// Test DeleteMany with cancelled context
	err = bc.DeleteMany(ctx, []string{"key1"})
	require.ErrorIs(t, err, context.Canceled, "Expected context.Canceled error")
}

func TestBatchConcurrency(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	var wg sync.WaitGroup
	concurrency := 10
	iterations := 100

	// Test concurrent GetMany operations
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			keys := make([]string, iterations)
			for j := 0; j < iterations; j++ {
				keys[j] = "key" + string(rune(id)) + string(rune(j))
			}
			_ = bc.GetMany(ctx, keys)
		}(i)
	}

	// Test concurrent SetMany operations
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			entries := make(map[string]int, iterations)
			for j := 0; j < iterations; j++ {
				entries["key"+string(rune(id))+string(rune(j))] = id*iterations + j
			}
			_ = bc.SetMany(ctx, entries, time.Second)
		}(i)
	}

	// Test concurrent DeleteMany operations
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			keys := make([]string, iterations)
			for j := 0; j < iterations; j++ {
				keys[j] = "key" + string(rune(id)) + string(rune(j))
			}
			_ = bc.DeleteMany(ctx, keys)
		}(i)
	}

	wg.Wait()
}

func TestBatchTimeout(t *testing.T) {
	mock := newMockCache[string, int]()
	bc := NewBatchCache[string, int](mock, DefaultBatchConfig())

	// Test GetMany with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result := bc.GetMany(ctx, []string{"key1", "key2"})
	require.Empty(t, result, "Result should be empty with timeout")

	// Test SetMany with timeout
	err := bc.SetMany(ctx, map[string]int{"key1": 1}, time.Second)
	if err != nil {
		require.ErrorIs(t, err, context.DeadlineExceeded, "Expected context.DeadlineExceeded error")
	} else {
		require.NoError(t, err, "SetMany should not return error if completed before timeout")
	}

	// Test DeleteMany with timeout
	err = bc.DeleteMany(ctx, []string{"key1"})
	if err != nil {
		require.ErrorIs(t, err, context.DeadlineExceeded, "Expected context.DeadlineExceeded error")
	} else {
		require.NoError(t, err, "DeleteMany should not return error if completed before timeout")
	}
}
