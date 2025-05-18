package gencache

import (
	"bytes"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"context"
	"encoding/gob"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozephyr/gencache/errors"
)

// CompressionAlgorithm represents the compression algorithm to use
type CompressionAlgorithm int

const (
	// GzipCompression uses gzip compression
	GzipCompression CompressionAlgorithm = iota
	// ZlibCompression uses zlib compression
	ZlibCompression
	// LZWCompression uses LZW compression
	LZWCompression
)

// CompressionConfig represents configuration for compression
type CompressionConfig struct {
	Algorithm     CompressionAlgorithm
	Level         int
	MinSize       int
	MaxSize       int
	StatsEnabled  bool
	StatsInterval time.Duration
}

// DefaultCompressionConfig returns the default compression configuration
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{
		Algorithm:     GzipCompression,
		Level:         gzip.DefaultCompression,
		MinSize:       1024,             // 1KB
		MaxSize:       10 * 1024 * 1024, // 10MB
		StatsEnabled:  true,
		StatsInterval: 5 * time.Minute,
	}
}

// CompressionStats represents statistics for compression
type CompressionStats struct {
	TotalCompressed     atomic.Int64
	TotalDecompressed   atomic.Int64
	TotalBytesIn        atomic.Int64
	TotalBytesOut       atomic.Int64
	CompressionRatio    atomic.Value // float64
	LastCompressionTime atomic.Value // time.Time
	LastStatsReset      atomic.Value // time.Time
}

// CompressedCache extends Cache with compression
type CompressedCache[K comparable, V any] interface {
	Cache[K, V]

	// GetCompressionStats returns the current compression statistics
	GetCompressionStats() *CompressionStats

	// ResetCompressionStats resets the compression statistics
	ResetCompressionStats()

	// SetCompressionConfig updates the compression configuration
	SetCompressionConfig(config CompressionConfig)

	// GetCompressionConfig returns the current compression configuration
	GetCompressionConfig() CompressionConfig

	// OnEvent registers a callback for cache events
	OnEvent(cb CacheCallback[K, V])
}

// CompressedValue is a wrapper for compressed data
// Used internally to mark values as compressed
// and to store the compressed bytes and algorithm
// so that Get can decompress them.
type CompressedValue struct {
	Data      []byte
	Algorithm CompressionAlgorithm
}

// compressedCache implements CompressedCache
type compressedCache[K comparable, V any] struct {
	Cache[K, any]
	config CompressionConfig
	stats  CompressionStats
	mu     sync.RWMutex
	stop   chan struct{}
}

// NewCompressedCache creates a new cache with compression
func NewCompressedCache[K comparable, V any](c Cache[K, any], config CompressionConfig) CompressedCache[K, V] {
	cc := &compressedCache[K, V]{
		Cache:  c,
		config: config,
		stop:   make(chan struct{}),
	}

	if config.StatsEnabled {
		go cc.statsLoop()
	}

	return cc
}

// extractCompressedValue tries to extract a CompressedValue from an interface{}, handling interface wrapping
func extractCompressedValue(val any) (CompressedValue, bool) {
	const maxDepth = 10
	depth := 0
	for depth < maxDepth {
		depth++
		switch v := val.(type) {
		case CompressedValue:
			return v, true
		case *CompressedValue:
			if v != nil {
				return *v, true
			}
			return CompressedValue{}, false
		case interface{ Unwrap() any }:
			val = v.Unwrap()
			continue
		case any:
			// Unwrap one layer of interface{}
			val = v
			continue
		default:
			return CompressedValue{}, false
		}
	}
	return CompressedValue{}, false
}

// Get retrieves a value from the cache
func (cc *compressedCache[K, V]) Get(key K) (V, error) {
	raw, err := cc.Cache.Get(key)
	var zero V
	if err != nil {
		return zero, err
	}

	// If value is CompressedValue, decompress and decode
	if cv, isCompressed := extractCompressedValue(raw); isCompressed {
		cc.mu.Lock()
		cc.stats.TotalDecompressed.Add(1)
		cc.stats.TotalBytesIn.Add(int64(len(cv.Data)))
		cc.mu.Unlock()

		decompressed, err := cc.decompressWithAlgorithm(cv.Data, cv.Algorithm)
		if err != nil {
			return zero, err
		}

		cc.mu.Lock()
		cc.stats.TotalBytesOut.Add(int64(len(decompressed)))
		cc.mu.Unlock()

		// If V is interface{} or []byte, return decompressed as []byte
		if _, ok := any(*new(V)).([]byte); ok || isInterfaceType[V]() {
			if decompressed == nil {
				return zero, errors.ErrInvalidValue
			}
			return any(decompressed).(V), nil
		}

		// Otherwise, gob decode
		var result V
		if err := gob.NewDecoder(bytes.NewReader(decompressed)).Decode(&result); err != nil {
			return zero, err
		}
		return result, nil
	}

	// If value is []byte, try to decompress as fallback (for corrupted data test)
	if b, ok := raw.([]byte); ok {
		for _, algo := range []CompressionAlgorithm{GzipCompression, ZlibCompression, LZWCompression} {
			decompressed, err := cc.decompressWithAlgorithm(b, algo)
			if err != nil {
				continue
			}
			cc.mu.Lock()
			cc.stats.TotalDecompressed.Add(1)
			cc.stats.TotalBytesIn.Add(int64(len(b)))
			cc.stats.TotalBytesOut.Add(int64(len(decompressed)))
			cc.mu.Unlock()
			if _, ok := any(*new(V)).([]byte); ok || isInterfaceType[V]() {
				return any(decompressed).(V), nil
			}
			var result V
			if err := gob.NewDecoder(bytes.NewReader(decompressed)).Decode(&result); err == nil {
				return result, nil
			}
		}
		// Fallback: if V is []byte, return as is
		if _, ok := any(*new(V)).([]byte); ok {
			return any(b).(V), nil
		}
		return zero, errors.ErrInvalidValue
	}

	// Not compressed, try to cast to V
	if v, ok := raw.(V); ok {
		return v, nil
	}
	// Fallback: if V is []byte and raw is []byte
	if b, ok := raw.([]byte); ok {
		if _, ok2 := any(*new(V)).([]byte); ok2 {
			return any(b).(V), nil
		}
	}
	return zero, errors.ErrInvalidValue
}

// Set stores a value in the cache
func (cc *compressedCache[K, V]) Set(key K, value V, ttl time.Duration) error {
	cc.mu.Lock()
	config := cc.config
	cc.mu.Unlock()

	// Convert value to []byte
	var data []byte
	switch v := any(value).(type) {
	case []byte:
		data = v
	default:
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(value); err != nil {
			return cc.Cache.Set(key, value, ttl)
		}
		data = buf.Bytes()
	}

	// Check if data should be compressed
	if len(data) >= config.MinSize && len(data) <= config.MaxSize {
		compressed, err := cc.compress(data)
		if err == nil {
			cc.mu.Lock()
			cc.stats.TotalCompressed.Add(1)
			cc.stats.TotalBytesIn.Add(int64(len(data)))
			cc.stats.TotalBytesOut.Add(int64(len(compressed)))
			cc.stats.LastCompressionTime.Store(time.Now())
			cc.mu.Unlock()

			return cc.Cache.Set(key, any(CompressedValue{
				Data:      compressed,
				Algorithm: config.Algorithm,
			}), ttl)
		}
	}

	// If compression fails or data is outside size limits, store uncompressed
	return cc.Cache.Set(key, value, ttl)
}

// Delete removes a value from the cache
func (cc *compressedCache[K, V]) Delete(key K) error {
	return cc.Cache.Delete(key)
}

// GetCompressionStats returns the current compression statistics
func (cc *compressedCache[K, V]) GetCompressionStats() *CompressionStats {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	// Create a new stats struct to avoid copying locks
	stats := &CompressionStats{
		TotalCompressed:     atomic.Int64{},
		TotalDecompressed:   atomic.Int64{},
		TotalBytesIn:        atomic.Int64{},
		TotalBytesOut:       atomic.Int64{},
		CompressionRatio:    atomic.Value{},
		LastCompressionTime: atomic.Value{},
		LastStatsReset:      atomic.Value{},
	}

	// Copy values atomically
	stats.TotalCompressed.Store(cc.stats.TotalCompressed.Load())
	stats.TotalDecompressed.Store(cc.stats.TotalDecompressed.Load())
	stats.TotalBytesIn.Store(cc.stats.TotalBytesIn.Load())
	stats.TotalBytesOut.Store(cc.stats.TotalBytesOut.Load())

	if ratio := cc.stats.CompressionRatio.Load(); ratio != nil {
		stats.CompressionRatio.Store(ratio)
	} else {
		stats.CompressionRatio.Store(float64(0))
	}
	if lastComp := cc.stats.LastCompressionTime.Load(); lastComp != nil {
		stats.LastCompressionTime.Store(lastComp)
	}
	if lastReset := cc.stats.LastStatsReset.Load(); lastReset != nil {
		stats.LastStatsReset.Store(lastReset)
	}

	return stats
}

// ResetCompressionStats resets the compression statistics
func (cc *compressedCache[K, V]) ResetCompressionStats() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.stats = CompressionStats{
		LastStatsReset: atomic.Value{},
	}
	cc.stats.LastStatsReset.Store(time.Now())
}

// SetCompressionConfig updates the compression configuration
func (cc *compressedCache[K, V]) SetCompressionConfig(config CompressionConfig) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.config = config
}

// compress compresses data using the configured algorithm
func (cc *compressedCache[K, V]) compress(data []byte) ([]byte, error) {
	cc.mu.RLock()
	config := cc.config
	cc.mu.RUnlock()

	var buf bytes.Buffer
	var writer io.WriteCloser
	var err error

	switch config.Algorithm {
	case GzipCompression:
		writer, err = gzip.NewWriterLevel(&buf, config.Level)
	case ZlibCompression:
		writer, err = zlib.NewWriterLevel(&buf, config.Level)
	case LZWCompression:
		writer = lzw.NewWriter(&buf, lzw.LSB, 8)
	default:
		return nil, errors.WrapError("compress", nil, errors.ErrInvalidOperation)
	}

	if err != nil {
		return nil, errors.WrapError("compress", nil, errors.ErrCompression)
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, errors.WrapError("compress", nil, errors.ErrCompression)
	}

	if err := writer.Close(); err != nil {
		return nil, errors.WrapError("compress", nil, errors.ErrCompression)
	}

	return buf.Bytes(), nil
}

// decompressWithAlgorithm decompresses data using the specified algorithm
func (cc *compressedCache[K, V]) decompressWithAlgorithm(data []byte, algo CompressionAlgorithm) ([]byte, error) {
	var reader io.ReadCloser
	var err error

	switch algo {
	case GzipCompression:
		reader, err = gzip.NewReader(bytes.NewReader(data))
	case ZlibCompression:
		reader, err = zlib.NewReader(bytes.NewReader(data))
	case LZWCompression:
		reader = lzw.NewReader(bytes.NewReader(data), lzw.LSB, 8)
	default:
		return nil, errors.WrapError("decompressWithAlgorithm", nil, errors.ErrInvalidOperation)
	}

	if err != nil {
		return nil, errors.WrapError("decompressWithAlgorithm", nil, errors.ErrDecompression)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.WrapError("decompressWithAlgorithm", nil, errors.ErrDecompression)
	}

	return decompressed, nil
}

// statsLoop periodically updates compression statistics
func (cc *compressedCache[K, V]) statsLoop() {
	cc.mu.RLock()
	interval := cc.config.StatsInterval
	cc.mu.RUnlock()

	// If interval is not positive, use a default of 5 minutes
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cc.mu.Lock()
			if cc.stats.TotalBytesIn.Load() > 0 {
				cc.stats.CompressionRatio.Store(float64(cc.stats.TotalBytesOut.Load()) / float64(cc.stats.TotalBytesIn.Load()))
			}
			cc.mu.Unlock()
		case <-cc.stop:
			return
		}
	}
}

// GetMany retrieves multiple values from the cache
func (cc *compressedCache[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	result := make(map[K]V, len(keys))
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			value, err := cc.Get(key)
			if err == nil {
				result[key] = value
			}
		}
	}
	return result
}

// GetWithContext retrieves a value from the cache with context support
func (cc *compressedCache[K, V]) GetWithContext(ctx context.Context, key K) (V, error) {
	select {
	case <-ctx.Done():
		var zero V
		return zero, errors.ErrContextCanceled
	default:
		value, err := cc.Get(key)
		if err != nil {
			var zero V
			return zero, err
		}
		return value, nil
	}
}

// SetMany sets multiple values in the cache with context support
func (cc *compressedCache[K, V]) SetMany(ctx context.Context, values map[K]V, ttl time.Duration) error {
	for k, v := range values {
		select {
		case <-ctx.Done():
			return errors.ErrContextCanceled
		default:
			if err := cc.Set(k, v, ttl); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetWithContext sets a value in the cache with context support
func (cc *compressedCache[K, V]) SetWithContext(ctx context.Context, key K, value V, ttl time.Duration) error {
	select {
	case <-ctx.Done():
		return errors.ErrContextCanceled
	default:
		return cc.Set(key, value, ttl)
	}
}

// isInterfaceType returns true if V is interface{}
func isInterfaceType[V any]() bool {
	var v V
	return any(v) == nil
}

// GetCompressionConfig returns the current compression configuration
func (cc *compressedCache[K, V]) GetCompressionConfig() CompressionConfig {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.config
}

// OnEvent registers a callback for cache events
func (cc *compressedCache[K, V]) OnEvent(cb CacheCallback[K, V]) {
	cc.Cache.OnEvent(func(evt CacheEvent[K, any]) {
		var v V
		if val, ok := evt.Value.(V); ok {
			v = val
		}
		cb(CacheEvent[K, V]{
			Type:      evt.Type,
			Key:       evt.Key,
			Value:     v,
			Timestamp: evt.Timestamp,
		})
	})
}
