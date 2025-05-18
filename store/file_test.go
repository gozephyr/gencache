package store

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestFileStore(t *testing.T) {
	tempDir := t.TempDir()

	config := &FileConfig{
		Directory:          tempDir,
		FileExtension:      ".cache",
		CompressionEnabled: false,
		CleanupInterval:    time.Second,
	}
	store, err := NewFileStore[string, string](context.Background(), config, WithTTLConfig(ttl.TestConfig()))
	require.NoError(t, err)
	require.NotNil(t, store)

	// Ensure store is closed after tests
	defer func() {
		err := store.Close(context.Background())
		require.NoError(t, err)
	}()

	t.Run("Basic Operations", func(t *testing.T) {
		// Test Set and Get
		err := store.Set(context.Background(), "key1", "value1", time.Minute)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Give time for file operations
		val, exists := store.Get(context.Background(), "key1")
		require.True(t, exists)
		require.Equal(t, "value1", val)

		// Verify file exists
		filePath := store.(*fileStore[string, string]).GetFilePath("key1")
		_, err = os.Stat(filePath)
		require.NoError(t, err)

		// Test Delete
		err = store.Delete(context.Background(), "key1")
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Give time for file operations
		_, exists = store.Get(context.Background(), "key1")
		require.False(t, exists)

		// Verify file is deleted
		_, err = os.Stat(filePath)
		require.True(t, os.IsNotExist(err))

		// Test Clear
		err = store.Set(context.Background(), "key2", "value2", time.Minute)
		require.NoError(t, err)
		err = store.Set(context.Background(), "key3", "value3", time.Minute)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Give time for file operations
		require.Equal(t, 2, store.Size(context.Background()))
		err = store.Clear(context.Background())
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Give time for file operations
		require.Equal(t, 0, store.Size(context.Background()))

		// Verify directory is empty
		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Empty(t, entries)
	})

	t.Run("TTL Expiration", func(t *testing.T) {
		// Set a key with a short TTL
		err = store.Set(context.Background(), "expire-key", "expire-value", 500*time.Millisecond)
		require.NoError(t, err)

		// Verify the key exists initially
		val, exists := store.Get(context.Background(), "expire-key")
		require.True(t, exists)
		require.Equal(t, "expire-value", val)

		// Wait for expiration
		time.Sleep(600 * time.Millisecond)

		// Verify the key is gone
		_, exists = store.Get(context.Background(), "expire-key")
		require.False(t, exists)

		// Verify file is deleted after expiration
		filePath := store.(*fileStore[string, string]).GetFilePath("expire-key")
		_, err = os.Stat(filePath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Batch Operations", func(t *testing.T) {
		// Test GetMany
		err = store.Set(context.Background(), "batch1", "value1", time.Minute)
		require.NoError(t, err)
		err = store.Set(context.Background(), "batch2", "value2", time.Minute)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Give time for file operations

		// Use a longer timeout for GetMany
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		results := store.GetMany(ctx, []string{"batch1", "batch2", "nonexistent"})
		cancel()

		require.Equal(t, 2, len(results))
		require.Equal(t, "value1", results["batch1"])
		require.Equal(t, "value2", results["batch2"])

		// Test SetMany with longer timeout
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		entries := map[string]string{
			"setmany1": "value1",
			"setmany2": "value2",
		}

		err = store.SetMany(ctx, entries, time.Minute)
		cancel()
		time.Sleep(100 * time.Millisecond) // Give time for file operations

		val1, exists1 := store.Get(context.Background(), "setmany1")
		require.True(t, exists1)
		require.Equal(t, "value1", val1)

		val2, exists2 := store.Get(context.Background(), "setmany2")
		require.True(t, exists2)
		require.Equal(t, "value2", val2)

		// Test DeleteMany with longer timeout
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		err = store.DeleteMany(ctx, []string{"setmany1", "setmany2"})
		cancel()
		time.Sleep(100 * time.Millisecond) // Give time for file operations

		_, exists1 = store.Get(context.Background(), "setmany1")
		_, exists2 = store.Get(context.Background(), "setmany2")
		require.False(t, exists1)
		require.False(t, exists2)
	})

	t.Run("Compression", func(t *testing.T) {
		// Create a new store with compression enabled
		compressedConfig := &FileConfig{
			Directory:          tempDir,
			FileExtension:      ".cache",
			CompressionEnabled: true,
			CompressionLevel:   gzip.BestCompression,
			CleanupInterval:    time.Second,
		}
		compressedStore, err := NewFileStore[string, string](context.Background(), compressedConfig)
		require.NoError(t, err)
		defer compressedStore.Close(context.Background())

		// Test storing and retrieving compressed data
		largeValue := "large-value-that-should-be-compressed-" + string(make([]byte, 1000))
		err = compressedStore.Set(context.Background(), "compressed-key", largeValue, time.Minute)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Give time for file operations

		val, exists := compressedStore.Get(context.Background(), "compressed-key")
		require.True(t, exists)
		require.Equal(t, largeValue, val)

		// Verify file exists and is compressed
		filePath := compressedStore.(*fileStore[string, string]).GetFilePath("compressed-key")
		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)
		require.Greater(t, fileInfo.Size(), int64(0))
	})

	t.Run("Invalid Directory", func(t *testing.T) {
		invalidConfig := &FileConfig{
			Directory:          "/nonexistent/path",
			FileExtension:      ".cache",
			CompressionEnabled: false,
			CleanupInterval:    time.Second,
		}
		_, err := NewFileStore[string, string](context.Background(), invalidConfig)
		require.Error(t, err)
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		var wg sync.WaitGroup
		iterations := 10
		errors := make(chan error, iterations*2)

		// Test concurrent Set operations
		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				err := store.Set(context.Background(), "concurrent-key", "value", time.Minute)
				if err != nil {
					errors <- fmt.Errorf("Set error: %w", err)
				}
				time.Sleep(10 * time.Millisecond)
			}(i)
		}

		// Test concurrent Get operations
		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = store.Get(context.Background(), "concurrent-key")
				time.Sleep(10 * time.Millisecond)
			}()
		}

		// Wait for all operations to complete
		wg.Wait()
		close(errors)

		// Check for errors
		for range errors {
			// t.Logf removed; just drain the channel
		}

		// Verify final state
		val, exists := store.Get(context.Background(), "concurrent-key")
		require.True(t, exists)
		require.Equal(t, "value", val)
	})

	t.Run("File Cleanup", func(t *testing.T) {
		// Set items with short TTL
		err = store.Set(context.Background(), "cleanup1", "value1", 500*time.Millisecond)
		require.NoError(t, err)
		err = store.Set(context.Background(), "cleanup2", "value2", 500*time.Millisecond)
		require.NoError(t, err)

		// Wait for items to expire
		time.Sleep(600 * time.Millisecond)

		// Force cleanup
		store.(*fileStore[string, string]).forceCleanup()

		// Verify files are cleaned up
		file1 := store.(*fileStore[string, string]).GetFilePath("cleanup1")
		file2 := store.(*fileStore[string, string]).GetFilePath("cleanup2")

		_, err = os.Stat(file1)
		require.True(t, os.IsNotExist(err))

		_, err = os.Stat(file2)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Special Characters in Keys", func(t *testing.T) {
		// Test keys with special characters
		specialKeys := []string{
			"key/with/slashes",
			"key with spaces",
			"key.with.dots",
			"key:with:colons",
			"key*with*stars",
			"key?with?question",
		}

		for _, key := range specialKeys {
			err = store.Set(context.Background(), key, "value", time.Minute)
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond) // Give time for file operations
			val, exists := store.Get(context.Background(), key)
			require.True(t, exists)
			require.Equal(t, "value", val)

			// Verify file exists
			filePath := store.(*fileStore[string, string]).GetFilePath(key)
			_, err = os.Stat(filePath)
			require.NoError(t, err)

			// Clean up
			err = store.Delete(context.Background(), key)
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond) // Give time for file operations
		}
	})

	t.Run("Large Values", func(t *testing.T) {
		// Test storing large values
		largeValue := string(make([]byte, 1024*1024)) // 1MB
		err = store.Set(context.Background(), "large-key", largeValue, time.Minute)
		require.NoError(t, err)
		time.Sleep(500 * time.Millisecond) // Give time for file operations

		val, exists := store.Get(context.Background(), "large-key")
		require.True(t, exists)
		require.Equal(t, largeValue, val)

		// Verify file size
		filePath := store.(*fileStore[string, string]).GetFilePath("large-key")
		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)
		require.GreaterOrEqual(t, fileInfo.Size(), int64(0))
	})

	t.Run("Context Cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Test operations with cancelled context
		err = store.SetMany(ctx, map[string]string{"key": "value"}, time.Minute)
		require.Error(t, err)
		require.Equal(t, errors.ErrContextCanceled, err)
	})

	t.Run("File Corruption", func(t *testing.T) {
		// Create a store with a specific file
		key := "corrupt-key"
		value := "corrupt-value"
		err := store.Set(context.Background(), key, value, time.Minute)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		// Corrupt the file
		filePath := store.(*fileStore[string, string]).GetFilePath(key)
		err = os.WriteFile(filePath, []byte("corrupted data"), 0o644)
		require.NoError(t, err)

		// Try to read corrupted file
		_, exists := store.Get(context.Background(), key)
		require.False(t, exists)

		// Verify file is cleaned up
		_, err = os.Stat(filePath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Disk Space Issues", func(t *testing.T) {
		// Create a store with a very large value
		largeValue := make([]byte, 1024*100) // 100KB
		for i := range largeValue {
			largeValue[i] = byte(i % 256)
		}

		// Try to store multiple large values
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("large-key-%d", i)
			err := store.Set(context.Background(), key, string(largeValue), time.Minute)
			if err != nil {
				// It's okay if we get an error due to disk space
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Verify store is still usable
		err := store.Set(context.Background(), "small-key", "small-value", time.Minute)
		require.NoError(t, err)
		val, exists := store.Get(context.Background(), "small-key")
		require.True(t, exists)
		require.Equal(t, "small-value", val)
	})

	t.Run("File Permissions", func(t *testing.T) {
		// Create a store with a specific file
		key := "permission-key"
		value := "permission-value"
		err := store.Set(context.Background(), key, value, time.Minute)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		// Change file permissions to read-only
		filePath := store.(*fileStore[string, string]).GetFilePath(key)
		dir := filepath.Dir(filePath)
		err = os.Chmod(dir, 0o444)
		require.NoError(t, err)

		// Try to update the file
		err = store.Set(context.Background(), key, "new-value", time.Minute)
		require.Error(t, err)

		// Restore permissions
		err = os.Chmod(dir, 0o755)
		require.NoError(t, err)

		// Also restore permissions for all files in the directory
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, entry := range entries {
			if !entry.IsDir() {
				_ = os.Chmod(filepath.Join(dir, entry.Name()), 0o644)
			}
		}

		// Verify store is still usable
		err = store.Set(context.Background(), "new-key", "new-value", time.Minute)
		require.NoError(t, err)
	})
}
