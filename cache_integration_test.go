package gencache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestMemoryStoreIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	memStore, err := store.NewMemoryStore[string, string](ctx)
	require.NoError(t, err)

	cache := New[string, string](
		WithStore[string, string](memStore),
		WithTTLConfig[string, string](ttl.DefaultConfig()),
	)
	defer cache.Close()

	err = cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)

	value, err := cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, "value1", value)

	err = cache.Set("key2", "value2", time.Second)
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)
	_, err = cache.Get("key2")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrKeyNotFound)
}

func TestFileStoreIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	testDir := "/tmp/gencache_test"
	err := os.MkdirAll(testDir, 0o755)
	require.NoError(t, err)
	defer os.RemoveAll(testDir)

	fileStore, err := store.NewFileStore[string, string](ctx, &store.FileConfig{
		Directory:          testDir,
		FileExtension:      ".cache",
		CompressionEnabled: true,
		CompressionLevel:   6,
		CleanupInterval:    5 * time.Second,
	})
	require.NoError(t, err)
	defer fileStore.Close(ctx)

	cache := New[string, string](
		WithStore[string, string](fileStore),
		WithTTLConfig[string, string](ttl.DefaultConfig()),
	)
	defer cache.Close()

	err = cache.Set("key1", "value1", time.Minute)
	require.NoError(t, err)

	newCache := New[string, string](
		WithStore[string, string](fileStore),
		WithTTLConfig[string, string](ttl.DefaultConfig()),
	)
	defer newCache.Close()

	value, err := newCache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, "value1", value)

	err = newCache.Set("key2", "value2", time.Second)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	time.Sleep(100 * time.Millisecond)

	_, err = newCache.Get("key2")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrKeyNotFound)
}
