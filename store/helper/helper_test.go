package helper

import (
	"context"
	"errors"
	"testing"
	"time"
	"unsafe"

	"github.com/gozephyr/gencache/ttl"
	"github.com/stretchr/testify/require"
)

func TestCalculateMemoryUsage(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected int64
	}{
		{
			name:     "String value",
			value:    "test",
			expected: int64(unsafe.Sizeof("test")),
		},
		{
			name:     "Integer value",
			value:    42,
			expected: int64(unsafe.Sizeof(42)),
		},
		{
			name:     "Slice value",
			value:    []byte{1, 2, 3},
			expected: int64(unsafe.Sizeof([]byte{1, 2, 3})),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateMemoryUsage(tt.value)
			require.Greater(t, result, int64(0))
		})
	}
}

func TestValidateTTL(t *testing.T) {
	config := ttl.Config{
		MinTTL: time.Second,
		MaxTTL: time.Hour,
	}

	tests := []struct {
		name        string
		duration    time.Duration
		config      ttl.Config
		expectError bool
	}{
		{
			name:        "Valid TTL",
			duration:    time.Minute,
			config:      config,
			expectError: false,
		},
		{
			name:        "TTL too short",
			duration:    time.Millisecond,
			config:      config,
			expectError: true,
		},
		{
			name:        "TTL too long",
			duration:    2 * time.Hour,
			config:      config,
			expectError: true,
		},
		{
			name:        "Zero TTL",
			duration:    0,
			config:      config,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTTL(tt.duration, tt.config)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetExpirationTime(t *testing.T) {
	config := ttl.Config{
		MinTTL: time.Second,
		MaxTTL: time.Hour,
	}

	tests := []struct {
		name     string
		duration time.Duration
		config   ttl.Config
	}{
		{
			name:     "One minute TTL",
			duration: time.Minute,
			config:   config,
		},
		{
			name:     "One hour TTL",
			duration: time.Hour,
			config:   config,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiration := GetExpirationTime(tt.duration, tt.config)
			require.True(t, expiration.After(time.Now()))
			require.True(t, expiration.Before(time.Now().Add(tt.duration+time.Second)))
		})
	}
}

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		expected bool
	}{
		{
			name:     "Expired time",
			time:     time.Now().Add(-time.Hour),
			expected: true,
		},
		{
			name:     "Future time",
			time:     time.Now().Add(time.Hour),
			expected: false,
		},
		{
			name:     "Current time",
			time:     time.Now().Add(time.Millisecond),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsExpired(tt.time)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateEntry(t *testing.T) {
	config := ttl.Config{
		MinTTL: time.Second,
		MaxTTL: time.Hour,
	}

	tests := []struct {
		name   string
		value  any
		ttl    time.Duration
		config ttl.Config
	}{
		{
			name:   "String entry",
			value:  "test",
			ttl:    time.Minute,
			config: config,
		},
		{
			name:   "Integer entry",
			value:  42,
			ttl:    time.Hour,
			config: config,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := CreateEntry(tt.value, tt.ttl, tt.config)
			require.NotNil(t, entry)
			require.Equal(t, tt.value, entry.Value)
			require.True(t, entry.Expires.After(time.Now()))
			require.True(t, entry.CreatedAt.Before(time.Now().Add(time.Second)))
			require.True(t, entry.LastAccess.Before(time.Now().Add(time.Second)))
			require.Equal(t, int64(1), entry.AccessCount)
			require.Equal(t, CalculateMemoryUsage(tt.value), entry.Size)
		})
	}
}

func TestCheckContext(t *testing.T) {
	//nolint:containedctx // This is test code; storing context in struct is safe here.
	type testCase struct {
		name        string
		ctx         context.Context
		expectError bool
	}

	tests := []testCase{
		{
			name:        "Valid context",
			ctx:         context.Background(),
			expectError: false,
		},
		{
			name: "Canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			expectError: true,
		},
		{
			name: "Timeout context",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
				defer cancel()
				time.Sleep(2 * time.Millisecond)
				return ctx
			}(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckContext(tt.ctx)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBatchOperation(t *testing.T) {
	//nolint:containedctx // This is test code; storing context in struct is safe here.
	type testCase struct {
		name        string
		ctx         context.Context
		keys        []string
		operation   func(string) error
		expectError bool
	}

	tests := []testCase{
		{
			name: "Successful batch operation",
			ctx:  context.Background(),
			keys: []string{"key1", "key2", "key3"},
			operation: func(key string) error {
				return nil
			},
			expectError: false,
		},
		{
			name: "Failed batch operation",
			ctx:  context.Background(),
			keys: []string{"key1", "key2", "key3"},
			operation: func(key string) error {
				return errors.New("operation failed")
			},
			expectError: true,
		},
		{
			name: "Canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			keys: []string{"key1", "key2", "key3"},
			operation: func(key string) error {
				return nil
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := BatchOperation[string, string](tt.ctx, tt.keys, tt.operation)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
