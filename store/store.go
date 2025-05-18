// Package store provides interfaces and implementations for cache storage backends.
package store

import (
	"context"
	"time"
)

// Store defines the interface for cache storage backends.
// This is the public interface that cache implementations can use.
type Store[K comparable, V any] interface {
	// Get retrieves a value from the store
	Get(ctx context.Context, key K) (V, bool)

	// Set stores a value in the store
	Set(ctx context.Context, key K, value V, ttl time.Duration) error

	// Delete removes a value from the store
	Delete(ctx context.Context, key K) error

	// Clear removes all values from the store
	Clear(ctx context.Context) error

	// Size returns the number of items in the store
	Size(ctx context.Context) int

	// Capacity returns the maximum number of items the store can hold
	Capacity(ctx context.Context) int

	// Keys returns all keys in the store
	Keys(ctx context.Context) []K

	// MemoryUsage returns the approximate memory usage in bytes
	MemoryUsage(ctx context.Context) int64

	// MaxMemory returns the maximum allowed memory usage in bytes
	MaxMemory(ctx context.Context) int64

	// Batch operations
	GetMany(ctx context.Context, keys []K) map[K]V
	SetMany(ctx context.Context, entries map[K]V, ttl time.Duration) error
	DeleteMany(ctx context.Context, keys []K) error

	// Close releases any resources used by the store
	Close(ctx context.Context) error
}

// Entry represents a stored value with metadata.
// This is a public type that can be used by cache implementations.
type Entry[V any] struct {
	Value       V         `json:"value"`
	Expires     time.Time `json:"expires"`
	CreatedAt   time.Time `json:"createdAt"`
	LastAccess  time.Time `json:"lastAccess"`
	AccessCount int64     `json:"accessCount"`
	Size        int64     `json:"size"` // Size of the entry in bytes
}
