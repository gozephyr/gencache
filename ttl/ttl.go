// Package ttl provides functionality for managing time-to-live (TTL) values in the cache.
// It includes utilities for validating TTL durations, calculating expiration times,
// and checking if values have expired.
package ttl

import (
	"time"

	"github.com/gozephyr/gencache/errors"
)

// Config represents configuration for TTL behavior
type Config struct {
	// DefaultTTL is the default time-to-live for cache entries
	DefaultTTL time.Duration

	// MinTTL is the minimum allowed TTL value
	MinTTL time.Duration

	// MaxTTL is the maximum allowed TTL value
	MaxTTL time.Duration

	// ZeroTTLMeansNoExpiry determines if TTL of 0 means no expiry
	ZeroTTLMeansNoExpiry bool
}

// DefaultConfig returns the default TTL configuration
func DefaultConfig() Config {
	return Config{
		DefaultTTL:           5 * time.Minute,
		MinTTL:               1 * time.Second,
		MaxTTL:               24 * time.Hour,
		ZeroTTLMeansNoExpiry: true,
	}
}

// Validate validates a TTL value against the configuration
func Validate(ttl time.Duration, config Config) error {
	if ttl < 0 {
		return errors.WrapError("Validate", nil, errors.ErrInvalidTTL)
	}

	if ttl == 0 && !config.ZeroTTLMeansNoExpiry {
		return errors.WrapError("Validate", nil, errors.ErrInvalidTTL)
	}

	if ttl > 0 {
		if ttl < config.MinTTL {
			return errors.WrapError("Validate", nil, errors.ErrTTLTooShort)
		}
		if ttl > config.MaxTTL {
			return errors.WrapError("Validate", nil, errors.ErrTTLTooLong)
		}
	}

	return nil
}

// Normalize normalizes a TTL value according to the configuration
func Normalize(ttl time.Duration, config Config) time.Duration {
	if ttl == 0 && config.ZeroTTLMeansNoExpiry {
		return 0
	}

	if ttl < config.MinTTL {
		return config.MinTTL
	}

	if ttl > config.MaxTTL {
		return config.MaxTTL
	}

	return ttl
}

// GetExpirationTime calculates the expiration time for a TTL value
func GetExpirationTime(ttl time.Duration, config Config) time.Time {
	if ttl == 0 && config.ZeroTTLMeansNoExpiry {
		return time.Time{} // Zero time means no expiration
	}

	normalizedTTL := Normalize(ttl, config)
	return time.Now().Add(normalizedTTL)
}

// IsExpired checks if a given time has expired
func IsExpired(expirationTime time.Time) bool {
	if expirationTime.IsZero() {
		return false // Zero time means no expiration
	}
	return time.Now().After(expirationTime)
}
