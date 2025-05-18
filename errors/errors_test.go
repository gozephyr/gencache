package errors

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCacheErrorBasics(t *testing.T) {
	err := errors.New("base error")
	ce := &CacheError{
		Op:      "Get",
		Key:     "foo",
		Err:     err,
		ErrType: ErrorTypeCache,
	}
	require.Contains(t, ce.Error(), "Get")
	require.Contains(t, ce.Error(), "foo")
	require.Contains(t, ce.Error(), "base error")
	require.Equal(t, err, ce.Unwrap())

	ce2 := &CacheError{
		Op:      "Get",
		Key:     "foo",
		Err:     err,
		ErrType: ErrorTypeCache,
	}
	require.True(t, ce.Is(ce2))
}

func TestWrapErrorAndTypeChecks(t *testing.T) {
	ResetErrorMetrics()
	base := ErrKeyNotFound
	wrapped := WrapError("Get", "bar", base)
	require.Error(t, wrapped)
	ce, ok := wrapped.(*CacheError)
	require.True(t, ok)
	require.Equal(t, ErrorTypeCache, ce.ErrType)
	require.Equal(t, "Get", ce.Op)
	require.Equal(t, "bar", ce.Key)
	require.True(t, errors.Is(wrapped, ErrKeyNotFound))

	require.True(t, IsCacheError(wrapped))
	ce2 := GetCacheError(wrapped)
	require.NotNil(t, ce2)
	require.True(t, IsErrorType(wrapped, ErrorTypeCache))
}

func TestErrorMetrics(t *testing.T) {
	ResetErrorMetrics()
	_ = WrapError("Set", "baz", ErrStoreError)
	_ = WrapError("Set", "baz", ErrCompression)
	_ = WrapError("Set", "baz", ErrReplicationError)
	metrics := GetErrorMetrics()
	require.GreaterOrEqual(t, metrics.CacheErrors.Load(), int64(0))
	require.GreaterOrEqual(t, metrics.StoreErrors.Load(), int64(1))
	require.GreaterOrEqual(t, metrics.ValidationErrors.Load(), int64(1))
	require.GreaterOrEqual(t, metrics.OperationErrors.Load(), int64(1))
	ResetErrorMetrics()
	metrics = GetErrorMetrics()
	require.Equal(t, int64(0), metrics.CacheErrors.Load())
	require.Equal(t, int64(0), metrics.StoreErrors.Load())
	require.Equal(t, int64(0), metrics.ValidationErrors.Load())
	require.Equal(t, int64(0), metrics.OperationErrors.Load())
}

func TestRecoverFromPanic(t *testing.T) {
	ResetErrorMetrics()
	defer func() {
		metrics := GetErrorMetrics()
		require.Equal(t, int64(1), metrics.PanicRecoveries.Load())
		last := metrics.LastPanic.Load()
		require.IsType(t, time.Now(), last)
	}()
	func() {
		defer RecoverFromPanic("Test", "panic-key")
		panic("test panic")
	}()
}
