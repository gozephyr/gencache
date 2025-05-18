package policy

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFIFO(t *testing.T) {
	// Create a new FIFO policy with small capacity for testing
	fifo := NewFIFO[string, string](WithMaxSize(3))
	require.NotNil(t, fifo)

	t.Run("Basic Operations", func(t *testing.T) {
		// Test OnSet
		fifo.OnSet("key1", "value1", time.Minute)
		fifo.OnSet("key2", "value2", time.Minute)
		fifo.OnSet("key3", "value3", time.Minute)

		require.Equal(t, 3, fifo.Size())
		require.Equal(t, 3, fifo.Capacity())

		// Test OnGet - should not affect order in FIFO
		fifo.OnGet("key1", "value1")

		// Test Evict - should evict key1 (first in)
		key, ok := fifo.Evict()
		require.True(t, ok)
		require.Equal(t, "key1", key)
		require.Equal(t, 2, fifo.Size())

		// Test OnDelete
		fifo.OnDelete("key2")
		require.Equal(t, 1, fifo.Size())

		// Test OnClear
		fifo.OnClear()
		require.Equal(t, 0, fifo.Size())
	})

	t.Run("Eviction Order", func(t *testing.T) {
		fifo := NewFIFO[string, string](WithMaxSize(3))

		// Add items
		fifo.OnSet("key1", "value1", time.Minute)
		fifo.OnSet("key2", "value2", time.Minute)
		fifo.OnSet("key3", "value3", time.Minute)

		// Access key1 multiple times - should not affect order
		for i := 0; i < 3; i++ {
			fifo.OnGet("key1", "value1")
		}

		// Add a new item, should evict key2 (second in)
		fifo.OnSet("key4", "value4", time.Minute)

		// Verify key2 is evicted
		key, ok := fifo.Evict()
		require.True(t, ok)
		require.Equal(t, "key2", key)
	})

	t.Run("Update Existing Item", func(t *testing.T) {
		fifo := NewFIFO[string, string](WithMaxSize(2))

		// Add initial item
		fifo.OnSet("key1", "value1", time.Minute)
		require.Equal(t, 1, fifo.Size())

		// Update existing item - should not change order
		fifo.OnSet("key1", "new-value", time.Minute)
		require.Equal(t, 1, fifo.Size())

		// Add another item
		fifo.OnSet("key2", "value2", time.Minute)
		require.Equal(t, 2, fifo.Size())

		// Add a new item, should evict key2 (second in)
		fifo.OnSet("key3", "value3", time.Minute)
		require.Equal(t, 2, fifo.Size())

		// Verify key2 is evicted
		key, ok := fifo.Evict()
		require.True(t, ok)
		require.Equal(t, "key2", key)
	})

	t.Run("Empty Policy", func(t *testing.T) {
		fifo := NewFIFO[string, string](WithMaxSize(2))

		// Test Evict on empty policy
		_, ok := fifo.Evict()
		require.False(t, ok)

		// Test Size and Capacity
		require.Equal(t, 0, fifo.Size())
		require.Equal(t, 2, fifo.Capacity())
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		fifo := NewFIFO[string, string](WithMaxSize(100))
		done := make(chan bool)

		// Start multiple goroutines
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					key := fmt.Sprintf("key-%d-%d", id, j)
					fifo.OnSet(key, "value", time.Minute)
					fifo.OnGet(key, "value")
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify final state
		require.LessOrEqual(t, fifo.Size(), fifo.Capacity())
	})
}
