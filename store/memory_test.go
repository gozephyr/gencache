package store

import (
	"context"
	"testing"
	"time"

	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore(t *testing.T) {
	store, err := NewMemoryStore[string, string](context.Background())
	require.NoError(t, err)
	require.NotNil(t, store)

	// Ensure store is closed after tests
	defer store.Close(context.Background())

	t.Run("Basic Operations", func(t *testing.T) {
		// Test Set and Get
		err = store.Set(context.Background(), "key1", "value1", 0)
		require.NoError(t, err)

		value, exists := store.Get(context.Background(), "key1")
		require.True(t, exists)
		require.Equal(t, "value1", value)

		// Test Delete
		err = store.Delete(context.Background(), "key1")
		require.NoError(t, err)

		_, exists = store.Get(context.Background(), "key1")
		require.False(t, exists)

		// Test Clear
		err = store.Set(context.Background(), "key2", "value2", 0)
		require.NoError(t, err)
		err = store.Set(context.Background(), "key3", "value3", 0)
		require.NoError(t, err)

		err = store.Clear(context.Background())
		require.NoError(t, err)

		_, exists = store.Get(context.Background(), "key2")
		require.False(t, exists)
		_, exists = store.Get(context.Background(), "key3")
		require.False(t, exists)
	})

	t.Run("TTL Operations", func(t *testing.T) {
		// Test TTL expiration
		store, err := NewMemoryStore[string, string](context.Background(), WithTTLConfig(ttl.TestConfig()))
		require.NoError(t, err)
		defer store.Close(context.Background())
		err = store.Set(context.Background(), "ttl-key", "ttl-value", 100*time.Millisecond)
		require.NoError(t, err)

		value, exists := store.Get(context.Background(), "ttl-key")
		require.True(t, exists)
		require.Equal(t, "ttl-value", value)

		time.Sleep(200 * time.Millisecond)
		// Wait for TTL cleanup goroutine
		time.Sleep(100 * time.Millisecond)
		// Force cleanup for deterministic test
		ms, ok := store.(*memoryStore[string, string])
		require.True(t, ok, "store should be of type *memoryStore[string, string]")
		ms.ForceCleanup()
		_, exists = store.Get(context.Background(), "ttl-key")
		require.False(t, exists)
	})

	t.Run("Memory Tracking", func(t *testing.T) {
		// Create a store with memory tracking
		memStore, err := NewMemoryStore[string, string](context.Background(),
			WithMaxMemory(1024*1024), // 1MB
			WithMemoryTracking(true),
		)
		require.NoError(t, err)
		defer memStore.Close(context.Background())

		initialUsage := memStore.MemoryUsage(context.Background())
		err = memStore.Set(context.Background(), "mem-key", "mem-value", 0)
		require.NoError(t, err)
		require.Greater(t, memStore.MemoryUsage(context.Background()), initialUsage)

		// Test memory limit
		largeValue := make([]byte, 2*1024*1024) // 2MB
		err = memStore.Set(context.Background(), "too-large", string(largeValue), 0)
		require.Error(t, err) // Should error due to memory limit
		_, exists := memStore.Get(context.Background(), "too-large")
		require.False(t, exists) // Should be rejected due to size
	})

	t.Run("Batch Operations", func(t *testing.T) {
		// Test GetMany
		err = store.Set(context.Background(), "batch1", "value1", 0)
		require.NoError(t, err)
		err = store.Set(context.Background(), "batch2", "value2", 0)
		require.NoError(t, err)

		values := store.GetMany(context.Background(), []string{"batch1", "batch2", "nonexistent"})
		require.Equal(t, 2, len(values))
		require.Equal(t, "value1", values["batch1"])
		require.Equal(t, "value2", values["batch2"])

		// Test SetMany
		entries := map[string]string{
			"many1": "value1",
			"many2": "value2",
			"many3": "value3",
		}

		err = store.SetMany(context.Background(), entries, 0)
		require.NoError(t, err)

		values = store.GetMany(context.Background(), []string{"many1", "many2", "many3"})
		require.Equal(t, 3, len(values))
		require.Equal(t, "value1", values["many1"])
		require.Equal(t, "value2", values["many2"])
		require.Equal(t, "value3", values["many3"])
	})

	t.Run("Capacity Limits", func(t *testing.T) {
		// Create a store with small capacity
		smallStore, err := NewMemoryStore[string, string](context.Background(), WithMaxSize(2))
		require.NoError(t, err)

		err = smallStore.Set(context.Background(), "key1", "value1", 0)
		require.NoError(t, err)
		err = smallStore.Set(context.Background(), "key2", "value2", 0)
		require.NoError(t, err)
		err = smallStore.Set(context.Background(), "key3", "value3", 0) // Should not be stored
		require.Error(t, err)                                           // Should error due to capacity limit

		require.Equal(t, 2, smallStore.Size(context.Background()))
		_, exists := smallStore.Get(context.Background(), "key3")
		require.False(t, exists)
	})

	t.Run("Keys Operation", func(t *testing.T) {
		err = store.Clear(context.Background())
		require.NoError(t, err)
		err = store.Set(context.Background(), "key1", "value1", 0)
		require.NoError(t, err)
		err = store.Set(context.Background(), "key2", "value2", 0)
		require.NoError(t, err)

		keys := store.Keys(context.Background())
		require.Equal(t, 2, len(keys))
		require.Contains(t, keys, "key1")
		require.Contains(t, keys, "key2")
	})
}
