package ttl

import "time"

// TestConfig returns a TTL configuration suitable for testing
// It allows shorter TTLs than the default configuration
func TestConfig() Config {
	return Config{
		DefaultTTL:           5 * time.Minute,
		MinTTL:               100 * time.Millisecond, // Allow shorter TTLs for testing
		MaxTTL:               24 * time.Hour,
		ZeroTTLMeansNoExpiry: true,
	}
}
