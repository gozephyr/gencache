package gencache

import (
	"time"

	"github.com/gozephyr/gencache/metrics"
	"github.com/gozephyr/gencache/policy"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
)

// Options represents cache configuration options
type Options[K comparable, V any] struct {
	// MaxSize is the maximum number of items the cache can hold
	MaxSize int

	// TTLConfig is the configuration for TTL behavior
	TTLConfig ttl.Config

	// EnableStats enables statistics tracking
	EnableStats bool

	// Store is the storage backend to use
	Store store.Store[K, V]

	// Policy is the eviction policy to use
	Policy policy.Policy[K, V]

	// MaxMemory is the maximum memory usage in bytes (0 means unlimited)
	MaxMemory int64

	// EnableMemoryTracking enables memory usage tracking
	EnableMemoryTracking bool

	// ShardCount is the number of shards for sharded cache
	ShardCount int

	// RebalanceThreshold is the threshold for triggering shard rebalancing
	RebalanceThreshold float64

	// BatchConfig configures batch operations
	BatchConfig BatchConfig

	// CompressionConfig configures compression settings
	CompressionConfig CompressionConfig

	// PoolConfig configures object pooling
	PoolConfig PoolConfig

	// CleanupInterval is the interval for cache cleanup
	CleanupInterval time.Duration

	// MetricsConfig defines the configuration for metrics
	MetricsConfig MetricsConfig
}

// Option is a function that configures cache options
type Option[K comparable, V any] func(*Options[K, V])

// WithMaxSize sets the maximum size of the cache
func WithMaxSize[K comparable, V any](size int) Option[K, V] {
	return func(o *Options[K, V]) {
		o.MaxSize = size
	}
}

// WithTTLConfig sets the TTL configuration
func WithTTLConfig[K comparable, V any](config ttl.Config) Option[K, V] {
	return func(o *Options[K, V]) {
		o.TTLConfig = config
	}
}

// WithStats enables statistics tracking
func WithStats[K comparable, V any](enable bool) Option[K, V] {
	return func(o *Options[K, V]) {
		o.EnableStats = enable
	}
}

// WithStore sets the storage backend
func WithStore[K comparable, V any](s store.Store[K, V]) Option[K, V] {
	return func(o *Options[K, V]) {
		o.Store = s
	}
}

// WithPolicy sets the eviction policy
func WithPolicy[K comparable, V any](p policy.Policy[K, V]) Option[K, V] {
	return func(o *Options[K, V]) {
		o.Policy = p
	}
}

// WithMaxMemory sets the maximum memory usage in bytes
func WithMaxMemory[K comparable, V any](maxMemory int64) Option[K, V] {
	return func(o *Options[K, V]) {
		o.MaxMemory = maxMemory
		o.EnableMemoryTracking = true
	}
}

// WithMemoryTracking enables memory usage tracking
func WithMemoryTracking[K comparable, V any](enable bool) Option[K, V] {
	return func(o *Options[K, V]) {
		o.EnableMemoryTracking = enable
	}
}

// WithShardCount sets the number of shards for sharded cache
func WithShardCount[K comparable, V any](count int) Option[K, V] {
	return func(o *Options[K, V]) {
		o.ShardCount = count
	}
}

// WithRebalanceThreshold sets the threshold for triggering shard rebalancing
func WithRebalanceThreshold[K comparable, V any](threshold float64) Option[K, V] {
	return func(o *Options[K, V]) {
		o.RebalanceThreshold = threshold
	}
}

// WithBatchConfig sets the batch operation configuration
func WithBatchConfig[K comparable, V any](config BatchConfig) Option[K, V] {
	return func(o *Options[K, V]) {
		o.BatchConfig = config
	}
}

// WithCompressionConfig sets the compression configuration
func WithCompressionConfig[K comparable, V any](config CompressionConfig) Option[K, V] {
	return func(o *Options[K, V]) {
		o.CompressionConfig = config
	}
}

// WithPoolConfig sets the object pool configuration
func WithPoolConfig[K comparable, V any](config PoolConfig) Option[K, V] {
	return func(o *Options[K, V]) {
		o.PoolConfig = config
	}
}

// WithCleanupInterval sets the cleanup interval for the cache
func WithCleanupInterval[K comparable, V any](interval time.Duration) Option[K, V] {
	return func(o *Options[K, V]) {
		o.CleanupInterval = interval
	}
}

// WithMetricsConfig sets the metrics configuration
func WithMetricsConfig[K comparable, V any](config MetricsConfig) Option[K, V] {
	return func(o *Options[K, V]) {
		o.MetricsConfig = config
	}
}

// DefaultOptions returns the default cache options
func DefaultOptions[K comparable, V any]() *Options[K, V] {
	return &Options[K, V]{
		MaxSize:            1000,
		TTLConfig:          ttl.DefaultConfig(),
		EnableStats:        false,
		ShardCount:         32,
		RebalanceThreshold: 0.2,
		BatchConfig:        DefaultBatchConfig(),
		CompressionConfig:  DefaultCompressionConfig(),
		PoolConfig:         DefaultPoolConfig(),
		CleanupInterval:    time.Second,
	}
}

// MetricsConfig defines the configuration for metrics
type MetricsConfig struct {
	// ExporterType specifies the type of metrics exporter to use
	ExporterType metrics.ExporterType
	// CacheName is used as a label for Prometheus metrics
	CacheName string
	// Labels are additional labels to be added to metrics
	Labels map[string]string
}
