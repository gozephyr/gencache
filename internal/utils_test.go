package internal

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExpired(t *testing.T) {
	t.Run("Future time", func(t *testing.T) {
		futureTime := time.Now().Add(time.Hour)
		require.False(t, Expired(futureTime))
	})

	t.Run("Past time", func(t *testing.T) {
		pastTime := time.Now().Add(-time.Hour)
		require.True(t, Expired(pastTime))
	})

	t.Run("Current time", func(t *testing.T) {
		// Add a small buffer to ensure the time hasn't expired
		currentTime := time.Now().Add(time.Millisecond)
		require.False(t, Expired(currentTime))
	})
}

func TestSafeCounter(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		counter := NewSafeCounter()
		require.NotNil(t, counter)
		require.Equal(t, int64(0), counter.Get())

		counter.Increment()
		require.Equal(t, int64(1), counter.Get())

		counter.Increment()
		require.Equal(t, int64(2), counter.Get())

		counter.Decrement()
		require.Equal(t, int64(1), counter.Get())

		counter.Reset()
		require.Equal(t, int64(0), counter.Get())
	})

	t.Run("Decrement Below Zero", func(t *testing.T) {
		counter := NewSafeCounter()
		require.Equal(t, int64(0), counter.Get())

		counter.Decrement()
		require.Equal(t, int64(0), counter.Get(), "Counter should not go below zero")
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		counter := NewSafeCounter()
		var wg sync.WaitGroup
		iterations := 1000

		// Start multiple goroutines that increment and decrement
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					counter.Increment()
					counter.Decrement()
				}
			}()
		}

		wg.Wait()
		require.Equal(t, int64(0), counter.Get(), "Counter should be zero after equal increments and decrements")
	})
}

func TestSafeMap(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		m := NewSafeMap[string, int]()
		require.NotNil(t, m)
		require.Equal(t, 0, m.Size())

		// Test Set and Get
		m.Set("key1", 1)
		value, exists := m.Get("key1")
		require.True(t, exists)
		require.Equal(t, 1, value)
		require.Equal(t, 1, m.Size())

		// Test Update
		m.Set("key1", 2)
		value, exists = m.Get("key1")
		require.True(t, exists)
		require.Equal(t, 2, value)

		// Test Delete
		m.Delete("key1")
		_, exists = m.Get("key1")
		require.False(t, exists)
		require.Equal(t, 0, m.Size())

		// Test Clear
		m.Set("key1", 1)
		m.Set("key2", 2)
		require.Equal(t, 2, m.Size())
		m.Clear()
		require.Equal(t, 0, m.Size())
	})

	t.Run("Non-existent Key", func(t *testing.T) {
		m := NewSafeMap[string, int]()
		value, exists := m.Get("nonexistent")
		require.False(t, exists)
		require.Equal(t, 0, value)
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		m := NewSafeMap[string, int]()
		var wg sync.WaitGroup
		iterations := 1000

		// Start multiple goroutines that write and read
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					key := "key" + string(rune(id))
					m.Set(key, j)
					value, exists := m.Get(key)
					require.True(t, exists)
					require.Equal(t, j, value)
				}
			}(i)
		}

		wg.Wait()
		require.LessOrEqual(t, m.Size(), 10, "Map should have at most 10 keys")
	})

	t.Run("Generic Types", func(t *testing.T) {
		// Test with different types
		m1 := NewSafeMap[int, string]()
		m1.Set(1, "one")
		value, exists := m1.Get(1)
		require.True(t, exists)
		require.Equal(t, "one", value)

		m2 := NewSafeMap[float64, bool]()
		m2.Set(1.5, true)
		value2, exists := m2.Get(1.5)
		require.True(t, exists)
		require.True(t, value2)
	})
}
