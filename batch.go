package gencache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/ttl"
)

// BatchConfig represents configuration for batch operations
type BatchConfig struct {
	MaxBatchSize     int
	OperationTimeout time.Duration
	MaxConcurrent    int
	TTLConfig        ttl.Config
}

// DefaultBatchConfig returns the default batch configuration
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		MaxBatchSize:     1000,
		OperationTimeout: 5 * time.Second,
		MaxConcurrent:    10,
		TTLConfig:        ttl.DefaultConfig(),
	}
}

// BatchMetrics represents metrics for batch operations
type BatchMetrics struct {
	TotalOperations atomic.Int64
	TotalItems      atomic.Int64
	SuccessCount    atomic.Int64
	ErrorCount      atomic.Int64
	TimeoutCount    atomic.Int64
	LastOperation   atomic.Value // time.Time
}

// BatchResult represents the result of a batch operation
type BatchResult[K comparable, V any] struct {
	Key   K
	Value V
	Error error
}

// BatchCache extends Cache with batch operations
type BatchCache[K comparable, V any] interface {
	Cache[K, V]

	// GetMany retrieves multiple values from the cache
	GetMany(ctx context.Context, keys []K) map[K]V

	// SetMany stores multiple values in the cache
	SetMany(ctx context.Context, entries map[K]V, ttl time.Duration) error

	// DeleteMany removes multiple values from the cache
	DeleteMany(ctx context.Context, keys []K) error

	// GetBatchMetrics returns the current batch operation metrics
	GetBatchMetrics() *BatchMetrics

	// ResetBatchMetrics resets the batch operation metrics
	ResetBatchMetrics()
}

// batchCache implements BatchCache
type batchCache[K comparable, V any] struct {
	Cache[K, V]
	config  BatchConfig
	metrics BatchMetrics
	mu      sync.RWMutex
}

// NewBatchCache creates a new cache with batch operations
func NewBatchCache[K comparable, V any](c Cache[K, V], config BatchConfig) BatchCache[K, V] {
	return &batchCache[K, V]{
		Cache:  c,
		config: config,
	}
}

// GetMany retrieves multiple values from the cache
func (bc *batchCache[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	// Update metrics atomically
	bc.metrics.TotalOperations.Add(1)
	bc.metrics.TotalItems.Add(int64(len(keys)))
	bc.metrics.LastOperation.Store(time.Now())

	// Limit batch size
	if len(keys) > bc.config.MaxBatchSize {
		keys = keys[:bc.config.MaxBatchSize]
	}

	result := make(map[K]V, len(keys))
	var resultMu sync.RWMutex

	// Create a context with timeout for individual operations
	opCtx, cancel := context.WithTimeout(ctx, bc.config.OperationTimeout)
	defer cancel()

	// Use a buffered channel for results to prevent goroutine leaks
	results := make(chan struct {
		key   K
		value V
		err   error
	}, len(keys))

	var wg sync.WaitGroup

	// Process keys in parallel with timeout
	for _, key := range keys {
		select {
		case <-opCtx.Done():
			bc.metrics.TimeoutCount.Add(1)
			return result
		default:
			wg.Add(1)
			go func(k K) {
				defer wg.Done()
				value, err := bc.Get(k)
				if err == nil {
					select {
					case results <- struct {
						key   K
						value V
						err   error
					}{k, value, nil}:
					case <-opCtx.Done():
						return
					}
				}
			}(key)
		}
	}

	// Start a goroutine to close results channel when all operations complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for {
		select {
		case <-opCtx.Done():
			bc.metrics.TimeoutCount.Add(1)
			return result
		case r, ok := <-results:
			if !ok {
				return result
			}
			if r.err == nil {
				resultMu.Lock()
				result[r.key] = r.value
				resultMu.Unlock()
				bc.metrics.SuccessCount.Add(1)
			}
		}
	}
}

// SetMany stores multiple values in the cache
func (bc *batchCache[K, V]) SetMany(ctx context.Context, entries map[K]V, ttlDuration time.Duration) error {
	bc.metrics.TotalOperations.Add(1)
	bc.metrics.TotalItems.Add(int64(len(entries)))
	bc.metrics.LastOperation.Store(time.Now())

	// Validate TTL
	if err := ttl.Validate(ttlDuration, bc.config.TTLConfig); err != nil {
		return errors.WrapError("SetMany", nil, err)
	}

	// Normalize TTL
	normalizedTTL := ttl.Normalize(ttlDuration, bc.config.TTLConfig)

	// Limit batch size
	if len(entries) > bc.config.MaxBatchSize {
		// Create a new map with limited size
		limitedEntries := make(map[K]V, bc.config.MaxBatchSize)
		count := 0
		for k, v := range entries {
			if count >= bc.config.MaxBatchSize {
				break
			}
			limitedEntries[k] = v
			count++
		}
		entries = limitedEntries
	}

	var wg sync.WaitGroup
	var timeoutOccurred atomic.Bool
	errs := make([]error, 0, len(entries)) // Pre-allocate error slice
	errChan := make(chan error, len(entries))

	for key, value := range entries {
		// Check context before launching goroutine
		if ctx.Err() != nil {
			bc.metrics.TimeoutCount.Add(1)
			return ctx.Err()
		}
		wg.Add(1)
		go func(k K, v V) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				timeoutOccurred.Store(true)
				return
			default:
				if err := bc.SetWithContext(ctx, k, v, normalizedTTL); err != nil {
					errChan <- errors.WrapError("SetMany", k, err)
				} else {
					bc.metrics.SuccessCount.Add(1)
				}
			}
		}(key, value)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	for err := range errChan {
		errs = append(errs, err)
	}

	// Check context again after all goroutines
	if ctx.Err() != nil || timeoutOccurred.Load() {
		bc.metrics.TimeoutCount.Add(1)
		return ctx.Err()
	}

	if len(errs) > 0 {
		bc.metrics.ErrorCount.Add(int64(len(errs)))
		return errors.WrapError("SetMany", nil, errors.ErrBatchOperation)
	}

	return nil
}

// DeleteMany removes multiple values from the cache
func (bc *batchCache[K, V]) DeleteMany(ctx context.Context, keys []K) error {
	bc.metrics.TotalOperations.Add(1)
	bc.metrics.TotalItems.Add(int64(len(keys)))
	bc.metrics.LastOperation.Store(time.Now())

	// Limit batch size
	if len(keys) > bc.config.MaxBatchSize {
		keys = keys[:bc.config.MaxBatchSize]
	}

	var wg sync.WaitGroup
	var errs []error
	sem := make(chan struct{}, bc.config.MaxConcurrent)
	errChan := make(chan error, len(keys))

	// Process keys in parallel with timeout
	done := make(chan struct{})
	go func() {
		for _, key := range keys {
			select {
			case <-ctx.Done():
				bc.metrics.TimeoutCount.Add(1)
				return
			default:
				wg.Add(1)
				sem <- struct{}{} // Acquire semaphore
				go func(k K) {
					defer wg.Done()
					defer func() { <-sem }() // Release semaphore
					if err := bc.DeleteWithContext(ctx, k); err != nil {
						errChan <- errors.WrapError("DeleteMany", k, err)
					} else {
						bc.metrics.SuccessCount.Add(1)
					}
				}(key)
			}
		}
		wg.Wait()
		close(done)
		close(errChan)
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Collect errors
		for err := range errChan {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			bc.metrics.ErrorCount.Add(int64(len(errs)))
			return errors.WrapError("DeleteMany", nil, errors.ErrBatchOperation)
		}
		return nil
	case <-ctx.Done():
		bc.metrics.TimeoutCount.Add(1)
		return ctx.Err()
	}
}

// GetBatchMetrics returns the current batch operation metrics
func (bc *batchCache[K, V]) GetBatchMetrics() *BatchMetrics {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return &bc.metrics
}

// ResetBatchMetrics resets the batch operation metrics
func (bc *batchCache[K, V]) ResetBatchMetrics() {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.metrics = BatchMetrics{}
}
