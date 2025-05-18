package policy

import (
	"time"
)

// Policy defines the interface for cache eviction policies
type Policy[K comparable, V any] interface {
	// OnGet is called when an item is retrieved from the cache
	OnGet(key K, value V)

	// OnSet is called when an item is added to the cache
	OnSet(key K, value V, ttl time.Duration)

	// OnDelete is called when an item is removed from the cache
	OnDelete(key K)

	// OnClear is called when the cache is cleared
	OnClear()

	// Evict returns the next key to be evicted from the cache
	Evict() (K, bool)

	// Size returns the number of items in the policy
	Size() int

	// Capacity returns the maximum number of items the policy can hold
	Capacity() int
}

// Entry represents a cache entry with metadata for policy decisions
type Entry[V any] struct {
	Value       V         `json:"value"`
	Expires     time.Time `json:"expires"`
	CreatedAt   time.Time `json:"createdAt"`
	LastAccess  time.Time `json:"lastAccess"`
	AccessCount int64     `json:"accessCount"`
}

// Options represents policy configuration options
type Options struct {
	// MaxSize is the maximum number of items the policy can hold
	MaxSize int

	// DefaultTTL is the default time-to-live for entries
	DefaultTTL time.Duration
}

// Option is a function that configures policy options
type Option func(*Options)

// WithMaxSize sets the maximum size of the policy
func WithMaxSize(size int) Option {
	return func(o *Options) {
		o.MaxSize = size
	}
}

// WithDefaultTTL sets the default TTL for entries
func WithDefaultTTL(ttl time.Duration) Option {
	return func(o *Options) {
		o.DefaultTTL = ttl
	}
}
