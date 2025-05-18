package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gozephyr/gencache"
	"github.com/gozephyr/gencache/policy"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestCacheIntegration(t *testing.T) {
	// Create a context with timeout for all tests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Memory Store Integration", func(t *testing.T) {
		// Create memory store
		memStore, err := store.NewMemoryStore[string, string](ctx)
		require.NoError(t, err)

		// Create cache with memory store
		cache := gencache.New[string, string](
			gencache.WithStore[string, string](memStore),
			gencache.WithMaxSize[string, string](1000),
			gencache.WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(1000))),
			gencache.WithTTLConfig[string, string](ttl.Config{
				MinTTL: time.Millisecond * 10,
				MaxTTL: time.Hour,
			}),
		)
		defer cache.Close()

		// Test basic operations
		err = cache.Set("key1", "value1", time.Minute)
		require.NoError(t, err)

		value, err := cache.Get("key1")
		require.NoError(t, err)
		require.Equal(t, "value1", value)

		// Test store persistence
		storeValue, found := memStore.Get(ctx, "key1")
		require.True(t, found)
		require.Equal(t, "value1", storeValue)
	})

	t.Run("File Store Integration", func(t *testing.T) {
		// Create file store
		fileStore, err := store.NewFileStore[string, string](ctx, &store.FileConfig{
			Directory:          t.TempDir(),
			FileExtension:      ".cache",
			CompressionEnabled: true,
			CleanupInterval:    time.Second,
		})
		require.NoError(t, err)

		// Create cache with file store
		cache := gencache.New[string, string](
			gencache.WithStore[string, string](fileStore),
			gencache.WithMaxSize[string, string](1000),
			gencache.WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(1000))),
		)

		// Test persistence across cache instances
		err = cache.Set("key1", "value1", time.Minute)
		require.NoError(t, err)

		// Create new cache instance with same store
		newCache := gencache.New[string, string](
			gencache.WithStore[string, string](fileStore),
		)

		// Verify data persistence
		value, err := newCache.Get("key1")
		require.NoError(t, err)
		require.Equal(t, "value1", value)

		// Close caches in reverse order
		newCache.Close()
		cache.Close()
	})

	t.Run("TTL Integration", func(t *testing.T) {
		cache := gencache.New[string, string](
			gencache.WithCleanupInterval[string, string](time.Millisecond*100),
			gencache.WithTTLConfig[string, string](ttl.Config{
				MinTTL: time.Millisecond * 10,
				MaxTTL: time.Hour,
			}),
		)
		defer cache.Close()

		// Test TTL expiration
		err := cache.Set("expire-key", "value", time.Millisecond*50)
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 100)

		_, err = cache.Get("expire-key")
		require.Error(t, err)
	})

	t.Run("Eviction Policy Integration", func(t *testing.T) {
		cache := gencache.New[string, string](
			gencache.WithMaxSize[string, string](2),
			gencache.WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(2))),
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

		// Verify eviction occurred
		stats := cache.Stats()
		require.Equal(t, int64(1), stats.Evictions.Load())
	})

	t.Run("Concurrent Operations Integration", func(t *testing.T) {
		cache := gencache.New[string, int](
			gencache.WithMaxSize[string, int](1000),
			gencache.WithPolicy[string, int](policy.NewLRU[string, int](policy.WithMaxSize(1000))),
		)
		defer cache.Close()

		var wg sync.WaitGroup
		concurrency := 10
		iterations := 100

		// Concurrent writes
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					key := "key" + string(rune(id)) + string(rune(j))
					err := cache.Set(key, id*iterations+j, time.Minute)
					require.NoError(t, err)
				}
			}(i)
		}

		// Concurrent reads
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					key := "key" + string(rune(id)) + string(rune(j))
					_, _ = cache.Get(key)
				}
			}(i)
		}

		wg.Wait()

		// Verify final state
		stats := cache.Stats()
		require.Greater(t, stats.Sets.Load(), int64(0))
		require.Greater(t, stats.Hits.Load(), int64(0))
	})

	t.Run("Error Handling Integration", func(t *testing.T) {
		cache := gencache.New[string, string]()
		defer cache.Close()

		// Test invalid TTL
		err := cache.Set("key1", "value1", -1)
		require.Error(t, err)

		// Test closed cache
		cache.Close()
		err = cache.Set("key1", "value1", time.Minute)
		require.Error(t, err)
	})
}
