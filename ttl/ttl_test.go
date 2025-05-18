package ttl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.Equal(t, 5*time.Minute, cfg.DefaultTTL)
	require.Equal(t, 1*time.Second, cfg.MinTTL)
	require.Equal(t, 24*time.Hour, cfg.MaxTTL)
	require.True(t, cfg.ZeroTTLMeansNoExpiry)
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("Negative TTL", func(t *testing.T) {
		err := Validate(-1*time.Second, cfg)
		require.Error(t, err)
	})

	t.Run("Zero TTL allowed", func(t *testing.T) {
		err := Validate(0, cfg)
		require.NoError(t, err)
	})

	t.Run("Zero TTL not allowed", func(t *testing.T) {
		cfg2 := cfg
		cfg2.ZeroTTLMeansNoExpiry = false
		err := Validate(0, cfg2)
		require.Error(t, err)
	})

	t.Run("TTL too short", func(t *testing.T) {
		err := Validate(500*time.Millisecond, cfg)
		require.Error(t, err)
	})

	t.Run("TTL too long", func(t *testing.T) {
		err := Validate(48*time.Hour, cfg)
		require.Error(t, err)
	})

	t.Run("TTL valid", func(t *testing.T) {
		err := Validate(10*time.Second, cfg)
		require.NoError(t, err)
	})
}

func TestNormalize(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("Zero TTL means no expiry", func(t *testing.T) {
		n := Normalize(0, cfg)
		require.Equal(t, time.Duration(0), n)
	})

	t.Run("TTL below min", func(t *testing.T) {
		n := Normalize(500*time.Millisecond, cfg)
		require.Equal(t, cfg.MinTTL, n)
	})

	t.Run("TTL above max", func(t *testing.T) {
		n := Normalize(48*time.Hour, cfg)
		require.Equal(t, cfg.MaxTTL, n)
	})

	t.Run("TTL in range", func(t *testing.T) {
		n := Normalize(10*time.Second, cfg)
		require.Equal(t, 10*time.Second, n)
	})
}

func TestGetExpirationTime(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("Zero TTL means no expiry", func(t *testing.T) {
		exp := GetExpirationTime(0, cfg)
		require.True(t, exp.IsZero())
	})

	t.Run("Non-zero TTL", func(t *testing.T) {
		d := 2 * time.Second
		start := time.Now()
		exp := GetExpirationTime(d, cfg)
		require.WithinDuration(t, start.Add(d), exp, 50*time.Millisecond)
	})
}

func TestIsExpired(t *testing.T) {
	t.Run("Zero time is never expired", func(t *testing.T) {
		require.False(t, IsExpired(time.Time{}))
	})

	t.Run("Future time is not expired", func(t *testing.T) {
		require.False(t, IsExpired(time.Now().Add(1*time.Hour)))
	})

	t.Run("Past time is expired", func(t *testing.T) {
		require.True(t, IsExpired(time.Now().Add(-1*time.Second)))
	})
}
