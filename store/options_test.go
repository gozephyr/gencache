package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptions(t *testing.T) {
	t.Run("Default Options", func(t *testing.T) {
		options := NewOptions()
		require.Equal(t, DefaultMaxSize, options.MaxSize)
		require.Equal(t, DefaultMaxMemory, options.MaxMemory)
		require.False(t, options.EnableMemoryTracking)
	})

	t.Run("WithMaxSize", func(t *testing.T) {
		opts := NewOptions()
		err := WithMaxSize(1000)(opts)
		require.NoError(t, err)
		require.Equal(t, 1000, opts.MaxSize)

		err = WithMaxSize(0)(opts)
		require.Error(t, err)
	})

	t.Run("WithMaxMemory", func(t *testing.T) {
		opts := NewOptions()
		err := WithMaxMemory(1024)(opts)
		require.NoError(t, err)
		require.Equal(t, int64(1024), opts.MaxMemory)
		require.True(t, opts.EnableMemoryTracking)

		err = WithMaxMemory(-1)(opts)
		require.Error(t, err)
	})

	t.Run("WithMemoryTracking", func(t *testing.T) {
		opts := NewOptions()
		err := WithMemoryTracking(true)(opts)
		require.NoError(t, err)
		require.True(t, opts.EnableMemoryTracking)
	})

	t.Run("Apply Multiple Options", func(t *testing.T) {
		opts := NewOptions()
		err := opts.Apply(
			WithMaxSize(2000),
			WithMaxMemory(2048),
			WithMemoryTracking(true),
		)
		require.NoError(t, err)
		require.Equal(t, 2000, opts.MaxSize)
		require.Equal(t, int64(2048), opts.MaxMemory)
		require.True(t, opts.EnableMemoryTracking)
	})
}
