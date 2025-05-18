package integration

import (
	"context"
	"testing"
	"time"

	"github.com/gozephyr/gencache"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestCacheWithBatchOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create memory store
	memStore, err := store.NewMemoryStore[string, string](ctx)
	require.NoError(t, err)

	// Create cache with batch operations
	cache := gencache.New[string, string](
		gencache.WithStore[string, string](memStore),
		gencache.WithTTLConfig[string, string](ttl.DefaultConfig()),
		gencache.WithBatchConfig[string, string](gencache.BatchConfig{
			MaxBatchSize:     1000,
			OperationTimeout: 5 * time.Second,
			MaxConcurrent:    10,
		}),
	)
	defer cache.Close()

	// Create batch cache
	batchCache := gencache.NewBatchCache(cache, gencache.DefaultBatchConfig())

	// Test batch operations
	keys := []string{"key1", "key2", "key3"}
	values := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	// Set multiple values
	err = batchCache.SetMany(ctx, values, time.Minute)
	require.NoError(t, err)

	// Get multiple values
	result := batchCache.GetMany(ctx, keys)
	require.Equal(t, 3, len(result))
	require.Equal(t, "value1", result["key1"])
	require.Equal(t, "value2", result["key2"])
	require.Equal(t, "value3", result["key3"])

	// Delete multiple values
	err = batchCache.DeleteMany(ctx, keys)
	require.NoError(t, err)

	// Verify deletion
	result = batchCache.GetMany(ctx, keys)
	require.Equal(t, 0, len(result))
}

func TestCacheWithObjectPooling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create memory store
	memStore, err := store.NewMemoryStore[string, *TestStruct](ctx)
	require.NoError(t, err)

	// Create cache with object pooling
	cache := gencache.New[string, *TestStruct](
		gencache.WithStore[string, *TestStruct](memStore),
		gencache.WithTTLConfig[string, *TestStruct](ttl.DefaultConfig()),
		gencache.WithPoolConfig[string, *TestStruct](gencache.PoolConfig{
			MaxSize:       1000,
			MinSize:       10,
			CleanupPeriod: 5 * time.Minute,
			MaxIdleTime:   10 * time.Minute,
		}),
	)
	defer cache.Close()

	// Create pooled cache
	pooledCache := gencache.NewPooledCache(cache, gencache.DefaultPoolConfig())

	// Test object pooling with multiple operations
	keys := []string{"key1", "key2", "key3"}
	values := []TestStruct{
		{ID: 1, Name: "test1"},
		{ID: 2, Name: "test2"},
		{ID: 3, Name: "test3"},
	}

	// Set multiple values
	for i, key := range keys {
		err = pooledCache.Set(key, values[i], time.Minute)
		require.NoError(t, err)
	}

	// Get values multiple times to ensure pool usage
	for i := 0; i < 3; i++ {
		for _, key := range keys {
			result, err := pooledCache.Get(key)
			require.NoError(t, err)
			require.NotNil(t, result)
		}
	}

	// Check pool stats
	stats := pooledCache.GetPoolStats()
	require.Greater(t, stats.TotalCreated.Load(), int64(0))
	require.Greater(t, stats.CurrentSize.Load(), int64(0))

	// Delete values
	for _, key := range keys {
		err = pooledCache.Delete(key)
		require.NoError(t, err)
	}

	// Verify deletion
	for _, key := range keys {
		_, err = pooledCache.Get(key)
		require.Error(t, err)
	}
}

func TestCacheWithCompression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create memory store
	memStore, err := store.NewMemoryStore[string, any](ctx)
	require.NoError(t, err)

	// Create cache with compression
	cache := gencache.New[string, any](
		gencache.WithStore[string, any](memStore),
		gencache.WithTTLConfig[string, any](ttl.DefaultConfig()),
		gencache.WithCompressionConfig[string, any](gencache.CompressionConfig{
			Algorithm:     gencache.GzipCompression,
			Level:         6,
			MinSize:       1024,
			MaxSize:       10 * 1024 * 1024,
			StatsEnabled:  true,
			StatsInterval: 5 * time.Minute,
		}),
	)
	defer cache.Close()

	// Create compressed cache
	compressedCache := gencache.NewCompressedCache[string, []byte](cache, gencache.DefaultCompressionConfig())

	// Test compression
	key := "test-key"
	value := make([]byte, 2000) // Create a value larger than MinSize
	for i := range value {
		value[i] = byte(i % 256)
	}

	// Set value
	err = compressedCache.Set(key, value, time.Minute)
	require.NoError(t, err)

	// Get value
	result, err := compressedCache.Get(key)
	require.NoError(t, err)
	require.Equal(t, len(value), len(result))
	require.Equal(t, value, result)

	// Check compression stats
	stats := compressedCache.GetCompressionStats()
	require.Greater(t, stats.TotalCompressed.Load(), int64(0))
	require.Greater(t, stats.TotalDecompressed.Load(), int64(0))
	require.Greater(t, stats.TotalBytesIn.Load(), int64(0))
	require.Greater(t, stats.TotalBytesOut.Load(), int64(0))

	// Delete value
	err = compressedCache.Delete(key)
	require.NoError(t, err)

	// Verify deletion
	_, err = compressedCache.Get(key)
	require.Error(t, err)
}

// TestStruct is a simple struct for testing object pooling
type TestStruct struct {
	ID   int
	Name string
}
