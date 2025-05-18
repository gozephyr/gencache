package policy

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLRU(t *testing.T) {
	// Create a new LRU policy with small capacity for testing
	lru := NewLRU[string, string](WithMaxSize(3))
	require.NotNil(t, lru)

	t.Run("Basic Operations", func(t *testing.T) {
		// Test OnSet
		lru.OnSet("key1", "value1", time.Minute)
		lru.OnSet("key2", "value2", time.Minute)
		lru.OnSet("key3", "value3", time.Minute)

		require.Equal(t, 3, lru.Size())
		require.Equal(t, 3, lru.Capacity())

		// Test OnGet - should move key1 to front
		lru.OnGet("key1", "value1")

		// Test Evict - should evict key2 (least recently used)
		key, ok := lru.Evict()
		require.True(t, ok)
		require.Equal(t, "key2", key)
		require.Equal(t, 2, lru.Size())

		// Test OnDelete
		lru.OnDelete("key1")
		require.Equal(t, 1, lru.Size())

		// Test OnClear
		lru.OnClear()
		require.Equal(t, 0, lru.Size())
	})

	t.Run("Eviction Order", func(t *testing.T) {
		lru := NewLRU[string, string](WithMaxSize(3))

		// Add items
		lru.OnSet("key1", "value1", time.Minute)
		lru.OnSet("key2", "value2", time.Minute)
		lru.OnSet("key3", "value3", time.Minute)

		// Access key1 to make it most recently used
		lru.OnGet("key1", "value1")

		// Add a new item, should evict key3 (least recently used)
		lru.OnSet("key4", "value4", time.Minute)

		// Verify key3 is evicted
		key, ok := lru.Evict()
		require.True(t, ok)
		require.Equal(t, "key3", key)
	})

	t.Run("Update Existing Item", func(t *testing.T) {
		lru := NewLRU[string, string](WithMaxSize(2))

		// Add initial items
		lru.OnSet("key1", "value1", time.Minute)
		lru.OnSet("key2", "value2", time.Minute)
		require.Equal(t, 2, lru.Size())

		// Update key1 to make it most recently used
		lru.OnSet("key1", "new-value", time.Minute)

		// Add a new item, should evict key1 (least recently used)
		lru.OnSet("key3", "value3", time.Minute)
		require.Equal(t, 2, lru.Size())

		// Verify key1 is evicted (since it was least recently used)
		key, ok := lru.Evict()
		require.True(t, ok)
		require.Equal(t, "key1", key)
	})

	t.Run("Empty Policy", func(t *testing.T) {
		lru := NewLRU[string, string](WithMaxSize(2))

		// Test Evict on empty policy
		_, ok := lru.Evict()
		require.False(t, ok)

		// Test Size and Capacity
		require.Equal(t, 0, lru.Size())
		require.Equal(t, 2, lru.Capacity())
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		lru := NewLRU[string, string](WithMaxSize(100))
		done := make(chan bool)

		// Start multiple goroutines
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					key := fmt.Sprintf("key-%d-%d", id, j)
					lru.OnSet(key, "value", time.Minute)
					lru.OnGet(key, "value")
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify final state
		require.LessOrEqual(t, lru.Size(), lru.Capacity())
	})
}
