// Package store provides a configurable caching store implementation
package store

import (
	cacheerrors "github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/ttl"
)

// Default values for store options
const (
	DefaultMaxSize       = 10000
	DefaultMaxMemory     = int64(100 * 1024 * 1024) // 100MB
	DefaultMaxBatchSize  = 100
	DefaultMaxConcurrent = 10
)

// Options represents store configuration options
type Options struct {
	// MaxSize is the maximum number of items the store can hold
	MaxSize int

	// TTLConfig is the configuration for TTL behavior
	TTLConfig ttl.Config

	// MaxMemory is the maximum memory usage in bytes (0 means unlimited)
	MaxMemory int64

	// EnableMemoryTracking enables memory usage tracking
	EnableMemoryTracking bool
}

// NewOptions creates a new Options instance with default values
func NewOptions() *Options {
	return &Options{
		MaxSize:   DefaultMaxSize,
		TTLConfig: ttl.DefaultConfig(),
		MaxMemory: DefaultMaxMemory,
	}
}

// Option is a function that configures store options
type Option func(*Options) error

// WithMaxSize sets the maximum size of the store
func WithMaxSize(size int) Option {
	return func(o *Options) error {
		if size <= 0 {
			return cacheerrors.ErrInvalidSize
		}
		o.MaxSize = size
		return nil
	}
}

// WithTTLConfig sets the TTL configuration
func WithTTLConfig(config ttl.Config) Option {
	return func(o *Options) error {
		o.TTLConfig = config
		return nil
	}
}

// WithMaxMemory sets the maximum memory usage in bytes
func WithMaxMemory(maxMemory int64) Option {
	return func(o *Options) error {
		if maxMemory < 0 {
			return cacheerrors.ErrInvalidMemoryLimit
		}
		o.MaxMemory = maxMemory
		o.EnableMemoryTracking = true
		return nil
	}
}

// WithMemoryTracking enables memory usage tracking
func WithMemoryTracking(enable bool) Option {
	return func(o *Options) error {
		o.EnableMemoryTracking = enable
		return nil
	}
}

// Apply applies the given options to the Options struct
func (o *Options) Apply(opts ...Option) error {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return err
		}
	}

	// Validate TTL config
	if err := ttl.Validate(0, o.TTLConfig); err != nil {
		return err
	}

	// Validate memory tracking
	if o.MaxMemory > 0 && !o.EnableMemoryTracking {
		o.EnableMemoryTracking = true
	}

	return nil
}
