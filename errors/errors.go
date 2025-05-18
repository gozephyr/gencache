// Package errors provides error types and utilities for the cache package.
package errors

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// ErrorType represents the category of error
type ErrorType string

const (
	// ErrorTypeCache represents cache-specific errors
	ErrorTypeCache ErrorType = "cache"
	// ErrorTypeStore represents storage backend errors
	ErrorTypeStore ErrorType = "store"
	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeOperation represents operation-specific errors
	ErrorTypeOperation ErrorType = "operation"
)

// Common error types
var (
	// Cache errors
	ErrCacheClosed     = errors.New("cache is closed")
	ErrKeyNotFound     = errors.New("key not found")
	ErrInvalidKey      = errors.New("invalid key")
	ErrInvalidValue    = errors.New("invalid value")
	ErrMemoryLimit     = errors.New("memory limit exceeded")
	ErrCapacityLimit   = errors.New("capacity limit exceeded")
	ErrContextCanceled = errors.New("operation canceled by context")

	// TTL errors
	ErrInvalidTTL  = errors.New("invalid TTL value")
	ErrTTLTooShort = errors.New("TTL value is too short")
	ErrTTLTooLong  = errors.New("TTL value is too long")

	// Store errors
	ErrStoreError         = errors.New("store operation failed")
	ErrStoreConnection    = errors.New("store connection failed")
	ErrStoreTimeout       = errors.New("store operation timed out")
	ErrInvalidSize        = errors.New("max size must be greater than 0")
	ErrInvalidMemoryLimit = errors.New("max memory cannot be negative")

	// Data errors
	ErrCompression     = errors.New("compression error")
	ErrDecompression   = errors.New("decompression error")
	ErrSerialization   = errors.New("serialization error")
	ErrDeserialization = errors.New("deserialization error")

	// Operation errors
	ErrReplicationError = errors.New("replication error")
	ErrBatchOperation   = errors.New("batch operation failed")
	ErrInvalidOperation = errors.New("invalid operation")
	ErrPermissionDenied = errors.New("permission denied")
)

// CacheError represents a cache operation error
type CacheError struct {
	Op      string
	Key     any
	Err     error
	ErrType ErrorType
}

// determineErrorType determines the error type based on the error
func determineErrorType(err error) ErrorType {
	switch {
	case errors.Is(err, ErrCacheClosed) || errors.Is(err, ErrKeyNotFound) ||
		errors.Is(err, ErrInvalidKey) || errors.Is(err, ErrInvalidValue) ||
		errors.Is(err, ErrMemoryLimit) || errors.Is(err, ErrCapacityLimit):
		return ErrorTypeCache
	case errors.Is(err, ErrStoreError) || errors.Is(err, ErrStoreConnection) ||
		errors.Is(err, ErrStoreTimeout):
		return ErrorTypeStore
	case errors.Is(err, ErrCompression) || errors.Is(err, ErrDecompression) ||
		errors.Is(err, ErrSerialization) || errors.Is(err, ErrDeserialization):
		return ErrorTypeValidation
	case errors.Is(err, ErrReplicationError) || errors.Is(err, ErrBatchOperation) ||
		errors.Is(err, ErrInvalidOperation):
		return ErrorTypeOperation
	default:
		return ErrorTypeOperation
	}
}

// Error implements the error interface
func (e *CacheError) Error() string {
	if e.Key != nil {
		return fmt.Sprintf("%s: %s: key=%v: %v", e.ErrType, e.Op, e.Key, e.Err)
	}
	return fmt.Sprintf("%s: %s: %v", e.ErrType, e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *CacheError) Unwrap() error {
	return e.Err
}

// Is reports whether the target error is of the same type as the receiver
func (e *CacheError) Is(target error) bool {
	t, ok := target.(*CacheError)
	if !ok {
		return false
	}
	return e.ErrType == t.ErrType && e.Op == t.Op && errors.Is(e.Err, t.Err)
}

// NewCacheError creates a new CacheError
func NewCacheError(errType ErrorType, op string, key any, err error) error {
	return &CacheError{
		ErrType: errType,
		Op:      op,
		Key:     key,
		Err:     err,
	}
}

// ErrorMetrics tracks error statistics
type ErrorMetrics struct {
	// Error counts by type
	CacheErrors      atomic.Int64
	StoreErrors      atomic.Int64
	ValidationErrors atomic.Int64
	OperationErrors  atomic.Int64

	// Last error timestamps
	LastCacheError      atomic.Value // time.Time
	LastStoreError      atomic.Value // time.Time
	LastValidationError atomic.Value // time.Time
	LastOperationError  atomic.Value // time.Time

	// Error recovery stats
	PanicRecoveries atomic.Int64
	LastPanic       atomic.Value // time.Time
}

var metrics = &ErrorMetrics{}

// GetErrorMetrics returns the current error metrics
func GetErrorMetrics() *ErrorMetrics {
	return metrics
}

// ResetErrorMetrics resets all error metrics
func ResetErrorMetrics() {
	metrics.CacheErrors.Store(0)
	metrics.StoreErrors.Store(0)
	metrics.ValidationErrors.Store(0)
	metrics.OperationErrors.Store(0)
	metrics.PanicRecoveries.Store(0)
	metrics.LastCacheError.Store(time.Time{})
	metrics.LastStoreError.Store(time.Time{})
	metrics.LastValidationError.Store(time.Time{})
	metrics.LastOperationError.Store(time.Time{})
	metrics.LastPanic.Store(time.Time{})
}

// updateErrorMetrics updates metrics for the given error type
func updateErrorMetrics(errType ErrorType) {
	now := time.Now()
	switch errType {
	case ErrorTypeCache:
		metrics.CacheErrors.Add(1)
		metrics.LastCacheError.Store(now)
	case ErrorTypeStore:
		metrics.StoreErrors.Add(1)
		metrics.LastStoreError.Store(now)
	case ErrorTypeValidation:
		metrics.ValidationErrors.Add(1)
		metrics.LastValidationError.Store(now)
	case ErrorTypeOperation:
		metrics.OperationErrors.Add(1)
		metrics.LastOperationError.Store(now)
	}
}

// WrapError wraps an error with context and updates metrics
func WrapError(op string, key any, err error) error {
	if err == nil {
		return nil
	}

	// Determine error type
	errType := determineErrorType(err)

	// Update metrics
	updateErrorMetrics(errType)

	// Create and return wrapped error
	return NewCacheError(errType, op, key, err)
}

// RecoverFromPanic recovers from a panic and updates metrics
func RecoverFromPanic(op string, key any) bool {
	if r := recover(); r != nil {
		metrics.PanicRecoveries.Add(1)
		metrics.LastPanic.Store(time.Now())
		return true
	}
	return false
}

// IsCacheError checks if an error is a CacheError
func IsCacheError(err error) bool {
	_, ok := err.(*CacheError)
	return ok
}

// GetCacheError returns the CacheError if the error is a CacheError
func GetCacheError(err error) *CacheError {
	if cacheErr, ok := err.(*CacheError); ok {
		return cacheErr
	}
	return nil
}

// IsErrorType checks if an error is of a specific type
func IsErrorType(err error, errType ErrorType) bool {
	if cacheErr, ok := err.(*CacheError); ok {
		return cacheErr.ErrType == errType
	}
	return false
}

// IsKeyNotFound checks if the error is a key not found error
func IsKeyNotFound(err error) bool {
	return errors.Is(err, ErrKeyNotFound)
}

// IsContextCanceled checks if the error is a context canceled error
func IsContextCanceled(err error) bool {
	return errors.Is(err, ErrContextCanceled)
}

// IsCacheClosed checks if the error is a cache closed error
func IsCacheClosed(err error) bool {
	return errors.Is(err, ErrCacheClosed)
}
