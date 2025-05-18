// Package helper provides utility functions for store implementations
package helper

import (
	"context"
	"time"
	"unsafe"

	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
)

// CalculateMemoryUsage calculates the approximate memory usage of a value
func CalculateMemoryUsage[V any](value V) int64 {
	return int64(unsafe.Sizeof(value))
}

// ValidateTTL validates the TTL duration against the configuration
func ValidateTTL(duration time.Duration, config ttl.Config) error {
	return ttl.Validate(duration, config)
}

// GetExpirationTime calculates the expiration time for a TTL duration
func GetExpirationTime(duration time.Duration, config ttl.Config) time.Time {
	return ttl.GetExpirationTime(duration, config)
}

// IsExpired checks if a time has expired
func IsExpired(t time.Time) bool {
	return ttl.IsExpired(t)
}

// CreateEntry creates a new store entry with the given value and TTL
func CreateEntry[V any](value V, ttlDuration time.Duration, config ttl.Config) *store.Entry[V] {
	return &store.Entry[V]{
		Value:       value,
		Expires:     GetExpirationTime(ttlDuration, config),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
		Size:        CalculateMemoryUsage(value),
	}
}

// CheckContext checks if the context is done
func CheckContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// BatchOperation performs a batch operation with context checking
func BatchOperation[K comparable, V any](
	ctx context.Context,
	keys []K,
	operation func(K) error,
) error {
	for _, key := range keys {
		if err := CheckContext(ctx); err != nil {
			return err
		}
		if err := operation(key); err != nil {
			return err
		}
	}
	return nil
}
