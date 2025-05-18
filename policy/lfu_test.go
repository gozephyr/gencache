package policy

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLFU(t *testing.T) {
	// Create a new LFU policy with small capacity for testing
	lfu := NewLFU[string, string](WithMaxSize(3))
	require.NotNil(t, lfu)

	t.Run("Basic Operations", func(t *testing.T) {
		// Test OnSet
		lfu.OnSet("key1", "value1", time.Minute)
		lfu.OnSet("key2", "value2", time.Minute)
		lfu.OnSet("key3", "value3", time.Minute)

		require.Equal(t, 3, lfu.Size())
		require.Equal(t, 3, lfu.Capacity())

		// Test OnGet - should increase access count for key1
		lfu.OnGet("key1", "value1")
		lfu.OnGet("key1", "value1") // Access twice

		// Test Evict - should evict key2 or key3 (least frequently used)
		key, ok := lfu.Evict()
		require.True(t, ok)
		require.NotEqual(t, "key1", key) // key1 should not be evicted
		require.Equal(t, 2, lfu.Size())

		// Test OnDelete
		lfu.OnDelete("key1")
		require.Equal(t, 1, lfu.Size())

		// Test OnClear
		lfu.OnClear()
		require.Equal(t, 0, lfu.Size())
	})

	t.Run("Eviction Order", func(t *testing.T) {
		lfu := NewLFU[string, string](WithMaxSize(3))

		// Add items
		lfu.OnSet("key1", "value1", time.Minute)
		lfu.OnSet("key2", "value2", time.Minute)
		lfu.OnSet("key3", "value3", time.Minute)

		// Access key1 multiple times
		for i := 0; i < 3; i++ {
			lfu.OnGet("key1", "value1")
		}

		// Access key2 once
		lfu.OnGet("key2", "value2")

		// Add a new item, should evict key4 (least frequently used)
		lfu.OnSet("key4", "value4", time.Minute)

		// Verify key4 is evicted
		key, ok := lfu.Evict()
		require.True(t, ok)
		require.Equal(t, "key4", key)
	})

	t.Run("Update Existing Item", func(t *testing.T) {
		lfu := NewLFU[string, string](WithMaxSize(2))

		// Add initial item
		lfu.OnSet("key1", "value1", time.Minute)
		require.Equal(t, 1, lfu.Size())

		// Update existing item
		lfu.OnSet("key1", "new-value", time.Minute)
		require.Equal(t, 1, lfu.Size())

		// Add another item
		lfu.OnSet("key2", "value2", time.Minute)
		require.Equal(t, 2, lfu.Size())

		// Access key1 multiple times
		for i := 0; i < 3; i++ {
			lfu.OnGet("key1", "new-value")
		}

		// Add a new item, should evict key3 (least frequently used)
		lfu.OnSet("key3", "value3", time.Minute)
		require.Equal(t, 2, lfu.Size())

		// Verify key3 is evicted
		key, ok := lfu.Evict()
		require.True(t, ok)
		require.Equal(t, "key3", key)
	})

	t.Run("Empty Policy", func(t *testing.T) {
		lfu := NewLFU[string, string](WithMaxSize(2))

		// Test Evict on empty policy
		_, ok := lfu.Evict()
		require.False(t, ok)

		// Test Size and Capacity
		require.Equal(t, 0, lfu.Size())
		require.Equal(t, 2, lfu.Capacity())
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		lfu := NewLFU[string, string](WithMaxSize(100))
		done := make(chan bool)

		// Start multiple goroutines
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					key := fmt.Sprintf("key-%d-%d", id, j)
					lfu.OnSet(key, "value", time.Minute)
					lfu.OnGet(key, "value")
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify final state
		require.LessOrEqual(t, lfu.Size(), lfu.Capacity())
	})
}
