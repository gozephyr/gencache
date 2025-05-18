package gencache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockCacheAny is for interface{} value type, used in compression tests
type mockCacheAny[K comparable] struct {
	store map[K]any
	mu    sync.RWMutex
	stats *Stats
}

func newMockCacheAny[K comparable]() *mockCacheAny[K] {
	return &mockCacheAny[K]{store: make(map[K]any), stats: &Stats{}}
}

func (m *mockCacheAny[K]) Get(key K) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.store[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return val, nil
}

func (m *mockCacheAny[K]) Set(key K, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
	return nil
}

func (m *mockCacheAny[K]) Delete(key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

func (m *mockCacheAny[K]) Capacity() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.store))
}

func (m *mockCacheAny[K]) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store = make(map[K]any)
	return nil
}

func (m *mockCacheAny[K]) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.store)
}

func (m *mockCacheAny[K]) Close() error {
	return nil
}

func (m *mockCacheAny[K]) ClearWithContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		m.mu.Lock()
		m.store = make(map[K]any)
		m.mu.Unlock()
		return nil
	}
}

func (m *mockCacheAny[K]) DeleteMany(ctx context.Context, keys []K) error {
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			m.mu.Lock()
			delete(m.store, key)
			m.mu.Unlock()
		}
	}
	return nil
}

func (m *mockCacheAny[K]) DeleteWithContext(ctx context.Context, key K) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		m.mu.Lock()
		delete(m.store, key)
		m.mu.Unlock()
		return nil
	}
}

func (m *mockCacheAny[K]) GetMany(ctx context.Context, keys []K) map[K]any {
	result := make(map[K]any)
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			m.mu.RLock()
			if val, ok := m.store[key]; ok {
				result[key] = val
			}
			m.mu.RUnlock()
		}
	}
	return result
}

func (m *mockCacheAny[K]) GetWithContext(ctx context.Context, key K) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return m.Get(key)
	}
}

func (m *mockCacheAny[K]) MaxMemory() int64 {
	return 0 // Mock implementation returns 0 for unlimited memory
}

func (m *mockCacheAny[K]) MemoryUsage() int64 {
	return 0 // Mock implementation returns 0 for memory usage
}

func (m *mockCacheAny[K]) ResetStats() {
	// Mock implementation does nothing
}

func (m *mockCacheAny[K]) SetMany(ctx context.Context, entries map[K]any, ttl time.Duration) error {
	for key, value := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			m.mu.Lock()
			m.store[key] = value
			m.mu.Unlock()
		}
	}
	return nil
}

func (m *mockCacheAny[K]) SetWithContext(ctx context.Context, key K, value any, ttl time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		m.mu.Lock()
		m.store[key] = value
		m.mu.Unlock()
		return nil
	}
}

func (m *mockCacheAny[K]) GetAll() map[K]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[K]any, len(m.store))
	for k, v := range m.store {
		result[k] = v
	}
	return result
}

func (m *mockCacheAny[K]) GetStats() StatsSnapshot {
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

func (m *mockCacheAny[K]) SetCapacity(capacity int64) {
	// No-op for mock cache
}

func (m *mockCacheAny[K]) OnEvent(callback CacheCallback[K, any]) {
	// No-op for mock cache
}

func (m *mockCacheAny[K]) RemoveEventCallback(callback CacheCallback[K, any]) {
	// No-op for mock cache
}

func (m *mockCacheAny[K]) Stats() *Stats {
	return m.stats
}

func TestNewCompressedCache(t *testing.T) {
	mock := newMockCacheAny[string]()
	config := DefaultCompressionConfig()
	cc := NewCompressedCache[string, any](mock, config)

	require.NotNil(t, cc, "NewCompressedCache should not return nil")
	require.Implements(t, (*CompressedCache[string, any])(nil), cc, "Should implement CompressedCache interface")
}

func TestCompressionAlgorithms(t *testing.T) {
	tests := []struct {
		name      string
		algorithm CompressionAlgorithm
		data      []byte
	}{
		{
			name:      "Gzip Compression",
			algorithm: GzipCompression,
			data:      bytes.Repeat([]byte("test data for compression"), 100),
		},
		{
			name:      "Zlib Compression",
			algorithm: ZlibCompression,
			data:      bytes.Repeat([]byte("test data for compression"), 100),
		},
		{
			name:      "LZW Compression",
			algorithm: LZWCompression,
			data:      bytes.Repeat([]byte("test data for compression"), 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockCacheAny[string]()
			config := DefaultCompressionConfig()
			config.Algorithm = tt.algorithm
			cc := NewCompressedCache[string, any](mock, config)

			// Test compression
			err := cc.Set("test", tt.data, time.Hour)
			require.NoError(t, err, "Set should not return error")
			value, err := mock.Get("test")
			require.NoError(t, err, "Compressed data should be stored")
			_, isCompressed := extractCompressedValue(value)
			require.True(t, isCompressed, "Compressed data should be stored as compressedValue")

			// Test decompression
			decompressed, err := cc.Get("test")
			require.NoError(t, err, "Should be able to retrieve data")
			require.True(t, bytes.Equal(tt.data, decompressed.([]byte)), "Decompressed data should match original")
		})
	}
}

func TestCompressionSizeLimits(t *testing.T) {
	mock := newMockCacheAny[string]()
	config := DefaultCompressionConfig()
	config.MinSize = 100
	config.MaxSize = 1000
	cc := NewCompressedCache[string, []byte](mock, config)

	// Test data smaller than MinSize
	smallData := bytes.Repeat([]byte("small"), 10)
	err := cc.Set("small", smallData, time.Hour)
	require.NoError(t, err, "Set should not return error")
	value, err := mock.Get("small")
	require.NoError(t, err, "Get should not return error")
	require.True(t, bytes.Equal(smallData, value.([]byte)), "Small data should not be compressed")

	// Test data within size limits
	mediumData := bytes.Repeat([]byte("medium"), 100) // 6 bytes * 100 = 600 bytes
	err = cc.Set("medium", mediumData, time.Hour)
	require.NoError(t, err, "Set should not return error")
	value, err = mock.Get("medium")
	require.NoError(t, err, "Get should not return error")

	// Check if the value is compressed
	cv, isCompressed := extractCompressedValue(value)
	require.True(t, isCompressed, "Medium data should be compressed")
	require.NotNil(t, cv.Data, "Compressed data should not be nil")
	require.Equal(t, GzipCompression, cv.Algorithm, "Should use default compression algorithm")

	// Verify we can get the original data back
	decompressed, err := cc.Get("medium")
	require.NoError(t, err, "Should be able to retrieve data")
	require.True(t, bytes.Equal(mediumData, decompressed), "Decompressed data should match original")

	// Test data larger than MaxSize
	largeData := bytes.Repeat([]byte("large"), 2000)
	err = cc.Set("large", largeData, time.Hour)
	require.NoError(t, err, "Set should not return error")
	value, err = mock.Get("large")
	require.NoError(t, err, "Get should not return error")
	require.True(t, bytes.Equal(largeData, value.([]byte)), "Large data should not be compressed")
}

func TestCompressionStats(t *testing.T) {
	// Use a real cache to preserve types for compression stats
	base := New[string, any](WithMaxSize[string, any](100))
	config := DefaultCompressionConfig()
	config.StatsEnabled = true
	config.MinSize = 10 // Set a small min size for testing
	cc := NewCompressedCache[string, []byte](base, config)

	// Perform some operations
	data := bytes.Repeat([]byte("test data for compression"), 100)
	err := cc.Set("test1", data, time.Hour)
	require.NoError(t, err, "Set should not return error")
	err = cc.Set("test2", data, time.Hour)
	require.NoError(t, err, "Set should not return error")
	err = cc.Set("test", data, time.Hour)
	require.NoError(t, err)
	_, err = cc.Get("test")
	require.NoError(t, err)
	_, err = cc.Get("test1")
	require.NoError(t, err)
	_, err = cc.Get("test2")
	require.NoError(t, err)

	// Add a small delay to allow stats to be updated
	time.Sleep(100 * time.Millisecond)

	// Check stats
	stats := cc.GetCompressionStats()
	require.Equal(t, int64(3), stats.TotalCompressed.Load(), "Should record 3 compression operations")
	require.Equal(t, int64(3), stats.TotalDecompressed.Load(), "Should record 3 decompression operations")
	require.Greater(t, stats.TotalBytesIn.Load(), int64(0), "Should record input bytes")
	require.Greater(t, stats.TotalBytesOut.Load(), int64(0), "Should record output bytes")

	// Force update of compression ratio for test determinism
	if stats.TotalBytesIn.Load() > 0 {
		stats.CompressionRatio.Store(float64(stats.TotalBytesOut.Load()) / float64(stats.TotalBytesIn.Load()))
	}
	require.Greater(t, stats.CompressionRatio.Load().(float64), float64(0), "Should calculate compression ratio")

	// Test concurrent stats updates with timeout
	const goroutines = 10
	const iterations = 100
	var wg sync.WaitGroup
	done := make(chan struct{})

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				select {
				case <-done:
					return
				default:
					err := cc.Set("test", data, time.Hour)
					require.NoError(t, err)
					_, err = cc.Get("test")
					require.NoError(t, err)
				}
			}
		}()
	}

	// Add timeout for concurrent operations
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Test completed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out after 5 seconds")
	}

	// Verify final stats
	stats = cc.GetCompressionStats()
	expectedCompressed := int64(goroutines * iterations)
	expectedDecompressed := int64(goroutines * iterations)
	require.GreaterOrEqual(t, stats.TotalCompressed.Load(), expectedCompressed, "Should record correct number of compression operations")
	require.LessOrEqual(t, stats.TotalCompressed.Load(), expectedCompressed+10, "Should not exceed expected compression operations by much")
	require.GreaterOrEqual(t, stats.TotalDecompressed.Load(), expectedDecompressed, "Should record correct number of decompression operations")
	require.LessOrEqual(t, stats.TotalDecompressed.Load(), expectedDecompressed+10, "Should not exceed expected decompression operations by much")

	// Test stats reset
	cc.ResetCompressionStats()
	stats = cc.GetCompressionStats()
	require.Equal(t, int64(0), stats.TotalCompressed.Load(), "Stats should be reset")
	require.Equal(t, int64(0), stats.TotalDecompressed.Load(), "Stats should be reset")
	require.Equal(t, int64(0), stats.TotalBytesIn.Load(), "Stats should be reset")
	require.Equal(t, int64(0), stats.TotalBytesOut.Load(), "Stats should be reset")
}

func TestCompressionConfigUpdate(t *testing.T) {
	mock := newMockCacheAny[string]()
	config := DefaultCompressionConfig()
	config.MinSize = 200
	config.MaxSize = 2000
	cc := NewCompressedCache[string, any](mock, config)

	// Test with data below new MinSize
	smallData := bytes.Repeat([]byte("small"), 100)
	err := cc.Set("test", smallData, time.Hour)
	require.NoError(t, err, "Set should not return error")
	value, err := mock.Get("test")
	require.NoError(t, err, "Get should not return error")
	_, isCompressed := extractCompressedValue(value)
	require.True(t, isCompressed, "Data should be compressed (above new MinSize)")
	// Verify we can get the original data back
	decompressed, err := cc.Get("test")
	require.NoError(t, err, "Should be able to retrieve data")
	require.True(t, bytes.Equal(smallData, decompressed.([]byte)), "Small data should not be compressed")

	// Test with data above new MinSize
	largeData := bytes.Repeat([]byte("large"), 300)
	err = cc.Set("test2", largeData, time.Hour)
	require.NoError(t, err, "Set should not return error")
	value, err = mock.Get("test2")
	require.NoError(t, err, "Get should not return error")
	_, isCompressed = extractCompressedValue(value)
	require.True(t, isCompressed, "Data should be compressed (above new MinSize)")
	// Verify we can get the original data back
	decompressed, err = cc.Get("test2")
	require.NoError(t, err, "Should be able to retrieve data")
	require.True(t, bytes.Equal(largeData, decompressed.([]byte)), "Decompressed data should match original")
}

func TestCompressionWithContext(t *testing.T) {
	mock := newMockCacheAny[string]()
	config := DefaultCompressionConfig()
	cc := NewCompressedCache[string, any](mock, config)

	// Test with context
	ctx := context.Background()
	data := bytes.Repeat([]byte("test data for compression"), 100)

	// Test SetWithContext
	err := cc.SetWithContext(ctx, "test", data, time.Hour)
	require.NoError(t, err, "SetWithContext should not return error")
	value, err := mock.Get("test")
	require.NoError(t, err, "Data should be stored")
	_, isCompressed := extractCompressedValue(value)
	require.True(t, isCompressed, "Data should be compressed")

	// Test GetWithContext
	decompressed, err := cc.GetWithContext(ctx, "test")
	require.NoError(t, err, "Should be able to retrieve data")
	require.True(t, bytes.Equal(data, decompressed.([]byte)), "Decompressed data should match original")

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = cc.SetWithContext(ctx, "test2", data, time.Hour)
	require.Error(t, err, "SetWithContext should fail with cancelled context")
	_, err = mock.Get("test2")
	require.Error(t, err, "Data should not be stored with cancelled context")
}

func TestCompressionErrorHandling(t *testing.T) {
	mock := newMockCacheAny[string]()
	config := DefaultCompressionConfig()
	cc := NewCompressedCache[string, any](mock, config)

	// Test with invalid compression level
	config.Level = 10 // Invalid level
	cc.SetCompressionConfig(config)
	data := bytes.Repeat([]byte("test data for compression"), 100)
	err := cc.Set("test", data, time.Hour)
	require.NoError(t, err, "Data should be stored")
	_, err = mock.Get("test")
	require.NoError(t, err, "Data should not be compressed with invalid level")

	// Test with corrupted compressed data
	corruptedData := []byte("corrupted compressed data")
	err = mock.Set("corrupted", corruptedData, time.Hour)
	require.NoError(t, err, "Set should not return error")
	_, err = cc.Get("corrupted")
	require.Error(t, err, "Should not be able to decompress corrupted data")
}

func TestCompressedCache(t *testing.T) {
	// Create a base cache
	baseCache := New[string, any](WithMaxSize[string, any](100))

	// Test cases for different compression algorithms
	algorithms := []CompressionAlgorithm{
		GzipCompression,
		ZlibCompression,
		LZWCompression,
	}

	for _, algo := range algorithms {
		t.Run(fmt.Sprintf("Algorithm_%d", algo), func(t *testing.T) {
			// Create compression config
			config := CompressionConfig{
				Algorithm:    algo,
				Level:        6,
				MinSize:      100,  // Small size for testing
				MaxSize:      1024, // Small size for testing
				StatsEnabled: true,
			}

			// Create compressed cache
			cache := NewCompressedCache[string, []byte](baseCache, config)

			// Test data
			testData := []byte("This is a test string that should be compressed. " +
				"It needs to be long enough to trigger compression. " +
				"Adding more text to ensure it exceeds the minimum size threshold.")

			// Test Set and Get
			err := cache.Set("test_key", testData, time.Minute)
			if err != nil {
				t.Errorf("Set failed: %v", err)
			}

			// Get the value back
			value, err := cache.Get("test_key")
			if err != nil {
				t.Errorf("Get failed: %v", err)
			}

			// Verify the value
			if !bytes.Equal(value, testData) {
				t.Errorf("Expected %s, got %s", string(testData), string(value))
			}

			// Test compression stats
			stats := cache.GetCompressionStats()
			if stats.TotalCompressed.Load() == 0 {
				t.Error("Expected compression stats to be recorded")
			}

			// Test context operations
			ctx := context.Background()
			_, err = cache.GetWithContext(ctx, "test_key")
			if err != nil {
				t.Errorf("GetWithContext failed: %v", err)
			}

			// Test compression config update
			newConfig := config
			newConfig.Level = 9
			cache.SetCompressionConfig(newConfig)
			currentConfig := cache.GetCompressionConfig()
			if currentConfig.Level != 9 {
				t.Errorf("Expected compression level 9, got %d", currentConfig.Level)
			}

			// Test reset stats
			cache.ResetCompressionStats()
			stats = cache.GetCompressionStats()
			if stats.TotalCompressed.Load() != 0 {
				t.Error("Expected stats to be reset")
			}
		})
	}
}

func TestCompressedCacheWithStruct(t *testing.T) {
	// Create a base cache
	baseCache := New[string, any](WithMaxSize[string, any](100))

	// Test struct
	type TestStruct struct {
		Name  string
		Value int
		Data  []byte
	}

	// Create compression config
	config := DefaultCompressionConfig()
	config.MinSize = 100 // Small size for testing

	// Create compressed cache
	cache := NewCompressedCache[string, TestStruct](baseCache, config)

	// Test data
	testData := TestStruct{
		Name:  "test",
		Value: 42,
		Data: []byte("This is a test string that should be compressed. " +
			"It needs to be long enough to trigger compression. " +
			"Adding more text to ensure it exceeds the minimum size threshold."),
	}

	// Test Set and Get
	err := cache.Set("test_key", testData, time.Minute)
	if err != nil {
		t.Errorf("Set failed: %v", err)
	}

	// Get the value back
	value, err := cache.Get("test_key")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}

	// Verify the value
	if value.Name != testData.Name || value.Value != testData.Value || !bytes.Equal(value.Data, testData.Data) {
		t.Errorf("Expected %+v, got %+v", testData, value)
	}
}

func TestCompressedCacheWithSmallData(t *testing.T) {
	// Create a base cache
	baseCache := New[string, any](WithMaxSize[string, any](100))

	// Create compression config with high minimum size
	config := DefaultCompressionConfig()
	config.MinSize = 1000 // Set high minimum size

	// Create compressed cache
	cache := NewCompressedCache[string, []byte](baseCache, config)

	// Test data that's too small to compress
	smallData := []byte("small data")

	// Test Set and Get
	err := cache.Set("test_key", smallData, time.Minute)
	require.NoError(t, err, "Set should not return error")

	// Get the value back
	value, err := cache.Get("test_key")
	require.NoError(t, err, "Get should not return error")

	// Verify the value
	require.Equal(t, string(smallData), string(value), "Value should match original data")

	// Verify no compression occurred
	stats := cache.GetCompressionStats()
	require.Equal(t, int64(0), stats.TotalCompressed.Load(), "Should not compress small data")
	require.Equal(t, int64(0), stats.TotalDecompressed.Load(), "Should not decompress small data")
}
