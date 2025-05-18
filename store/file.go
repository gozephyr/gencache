package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	cacheErrors "github.com/gozephyr/gencache/errors"
	"github.com/gozephyr/gencache/ttl"
)

// FileConfig holds file-based storage configuration
type FileConfig struct {
	// Directory is the base directory for storing files
	Directory string

	// FileExtension is the extension for data files
	FileExtension string

	// CompressionEnabled enables gzip compression
	CompressionEnabled bool

	// CompressionLevel sets the gzip compression level (1-9)
	CompressionLevel int

	// CleanupInterval is the interval for cleaning up expired files
	CleanupInterval time.Duration
}

// DefaultFileConfig returns a FileConfig with sensible defaults
func DefaultFileConfig() *FileConfig {
	return &FileConfig{
		Directory:          "cache",
		FileExtension:      ".cache",
		CompressionEnabled: true,
		CompressionLevel:   6,
		CleanupInterval:    100 * time.Millisecond,
	}
}

// fileEntry represents a stored value with metadata
type fileEntry[V any] struct {
	Value       V         `json:"value"`
	Expires     time.Time `json:"expires"`
	CreatedAt   time.Time `json:"createdAt"`
	LastAccess  time.Time `json:"lastAccess"`
	AccessCount int64     `json:"accessCount"`
	Size        int64     `json:"size"`
}

// fileStore implements the Store interface using the filesystem
type fileStore[K comparable, V any] struct {
	config      *FileConfig
	opts        *Options
	mu          sync.RWMutex // Use RWMutex for concurrent access
	stop        chan struct{}
	done        chan struct{}
	lastCleanup time.Time
	path        string
	ttlConfig   ttl.Config
	maxSize     int
	stats       *Stats
	closeOnce   sync.Once  // Ensure Close is idempotent
	cleanupMu   sync.Mutex // Separate mutex for cleanup operations
}

// NewFileStore creates a new file-based store
func NewFileStore[K comparable, V any](ctx context.Context, config *FileConfig, opts ...Option) (Store[K, V], error) {
	select {
	case <-ctx.Done():
		return nil, cacheErrors.WrapError("NewFileStore", nil, cacheErrors.ErrContextCanceled)
	default:
	}

	if config == nil {
		config = DefaultFileConfig()
	}

	// Validate config
	if config.CleanupInterval < 100*time.Millisecond {
		config.CleanupInterval = 100 * time.Millisecond // Ensure minimum cleanup interval
	}

	options := NewOptions()
	if err := options.Apply(opts...); err != nil {
		return nil, cacheErrors.WrapError("NewFileStore", nil, cacheErrors.ErrInvalidOperation)
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.Directory, 0o755); err != nil {
		return nil, cacheErrors.WrapError("NewFileStore", err, cacheErrors.ErrStoreError)
	}

	// Verify directory is writable
	if err := verifyDirectoryWritable(config.Directory); err != nil {
		return nil, cacheErrors.WrapError("NewFileStore", err, cacheErrors.ErrStoreError)
	}

	store := &fileStore[K, V]{
		config:      config,
		opts:        options,
		mu:          sync.RWMutex{},
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		lastCleanup: time.Now(),
		path:        config.Directory,
		ttlConfig:   options.TTLConfig,
		maxSize:     options.MaxSize,
		stats:       &Stats{},
		cleanupMu:   sync.Mutex{},
	}

	// Start cleanup goroutine
	go store.startCleanupGoroutine()

	return store, nil
}

// verifyDirectoryWritable checks if the directory is writable
func verifyDirectoryWritable(dir string) error {
	testFile := filepath.Join(dir, ".test_write")
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// startCleanupGoroutine starts the cleanup goroutine
func (f *fileStore[K, V]) startCleanupGoroutine() {
	defer close(f.done)
	ticker := time.NewTicker(f.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.stop:
			return
		case <-ticker.C:
			f.forceCleanup()
		}
	}
}

// Get retrieves a value from the store
func (f *fileStore[K, V]) Get(ctx context.Context, key K) (V, bool) {
	select {
	case <-ctx.Done():
		var zero V
		return zero, false
	default:
	}

	f.maybeCleanup() // Call before acquiring lock

	// Use RLock for read operations
	f.mu.RLock()
	defer func() {
		f.mu.RUnlock()
	}()

	path := f.getPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			// Unexpected error reading file; could log or increment error stats
			f.stats.Errors.Add(1)
		}
		f.stats.Misses.Add(1)
		var zero V
		return zero, false
	}

	var entry fileEntry[V]
	if err := json.Unmarshal(data, &entry); err != nil {
		f.stats.Misses.Add(1)
		// Delete corrupted file
		_ = os.Remove(path)
		var zero V
		return zero, false
	}

	// Check if entry has expired
	if !entry.Expires.IsZero() && ttl.IsExpired(entry.Expires) {
		f.stats.Misses.Add(1)
		// Use a separate goroutine for cleanup to avoid deadlock
		go func() {
			_ = f.Delete(ctx, key)
		}()
		var zero V
		return zero, false
	}

	f.stats.Hits.Add(1)
	entry.LastAccess = time.Now()
	entry.AccessCount++

	// Update last access time without calling Set
	entryData, err := json.Marshal(entry)
	if err != nil {
		// Log the error but continue since the value was already retrieved
		f.stats.Errors.Add(1)
	} else {
		// Use atomic write to prevent corruption
		tempPath := path + ".tmp"
		if err := os.WriteFile(tempPath, entryData, 0o644); err != nil {
			// Log the error but continue since the value was already retrieved
			f.stats.Errors.Add(1)
		} else if err := os.Rename(tempPath, path); err != nil {
			_ = os.Remove(tempPath)
			// Log the error but continue since the value was already retrieved
			f.stats.Errors.Add(1)
		}
	}

	return entry.Value, true
}

// Set stores a value in the store
func (f *fileStore[K, V]) Set(ctx context.Context, key K, value V, ttlDuration time.Duration) error {
	select {
	case <-ctx.Done():
		return cacheErrors.WrapError("Set", key, cacheErrors.ErrContextCanceled)
	default:
	}

	f.maybeCleanup() // Call before acquiring lock

	// Use Lock for write operations
	f.mu.Lock()
	defer func() {
		f.mu.Unlock()
	}()

	// Validate TTL
	if err := ttl.Validate(ttlDuration, f.ttlConfig); err != nil {
		return cacheErrors.WrapError("Set", key, err)
	}

	// Get expiration time
	expires := ttl.GetExpirationTime(ttlDuration, f.ttlConfig)

	entry := &fileEntry[V]{
		Value:       value,
		Expires:     expires,
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
		Size:        int64(unsafe.Sizeof(value)),
	}

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return cacheErrors.WrapError("Set", key, err)
	}

	// Write to file using atomic write
	filePath := f.getPath(key)
	tempPath := filePath + ".tmp"

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if os.IsPermission(err) {
			return cacheErrors.WrapError("Set", key, cacheErrors.ErrPermissionDenied)
		}
		return cacheErrors.WrapError("Set", key, err)
	}

	// Try to write to temp file
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		if os.IsPermission(err) {
			return cacheErrors.WrapError("Set", key, cacheErrors.ErrPermissionDenied)
		}
		return cacheErrors.WrapError("Set", key, err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath)
		if os.IsPermission(err) {
			return cacheErrors.WrapError("Set", key, cacheErrors.ErrPermissionDenied)
		}
		return cacheErrors.WrapError("Set", key, err)
	}

	f.stats.Sets.Add(1)
	f.stats.Size.Add(1)
	return nil
}

// Delete removes a value from the store
func (f *fileStore[K, V]) Delete(ctx context.Context, key K) error {
	f.maybeCleanup() // Call before acquiring lock

	if !f.tryLock() {
		return cacheErrors.WrapError("Delete", key, cacheErrors.ErrInvalidOperation)
	}
	defer func() {
		f.unlock()
	}()

	select {
	case <-ctx.Done():
		return cacheErrors.WrapError("Delete", key, cacheErrors.ErrContextCanceled)
	default:
	}

	path := f.getPath(key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return cacheErrors.WrapError("Delete", key, cacheErrors.ErrStoreError)
	}

	return nil
}

// Clear removes all values from the store
func (f *fileStore[K, V]) Clear(ctx context.Context) error {
	f.maybeCleanup() // Call before acquiring lock

	if !f.tryLock() {
		return cacheErrors.WrapError("Clear", nil, cacheErrors.ErrInvalidOperation)
	}
	defer func() {
		f.unlock()
	}()

	select {
	case <-ctx.Done():
		return cacheErrors.WrapError("Clear", nil, cacheErrors.ErrContextCanceled)
	default:
	}

	if err := os.RemoveAll(f.config.Directory); err != nil {
		return cacheErrors.WrapError("Clear", nil, cacheErrors.ErrStoreError)
	}

	// Recreate the directory
	if err := os.MkdirAll(f.config.Directory, 0o755); err != nil {
		return cacheErrors.WrapError("Clear", nil, cacheErrors.ErrStoreError)
	}

	return nil
}

// Size returns the number of items in the store
func (f *fileStore[K, V]) Size(ctx context.Context) int {
	f.maybeCleanup() // Call before acquiring lock

	if !f.tryLock() {
		return 0
	}
	defer func() {
		f.unlock()
	}()

	files, err := os.ReadDir(f.config.Directory)
	if err != nil {
		return 0
	}

	count := 0
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == f.config.FileExtension {
			count++
		}
	}
	return count
}

// Capacity returns the maximum number of items the store can hold
func (f *fileStore[K, V]) Capacity(ctx context.Context) int {
	return f.maxSize
}

// Keys returns all keys in the store
func (f *fileStore[K, V]) Keys(ctx context.Context) []K {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	if !f.tryLock() {
		return nil
	}
	defer func() {
		f.unlock()
	}()

	files, err := os.ReadDir(f.config.Directory)
	if err != nil {
		return nil
	}

	keys := make([]K, 0)
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == f.config.FileExtension {
			key := f.getKeyFromPath(file.Name())
			if k, ok := fromString[K](key); ok {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// MemoryUsage returns the approximate memory usage in bytes
func (f *fileStore[K, V]) MemoryUsage(ctx context.Context) int64 {
	select {
	case <-ctx.Done():
		return 0
	default:
	}

	if !f.tryLock() {
		return 0
	}
	defer func() {
		f.unlock()
	}()

	var total int64
	if err := filepath.Walk(f.config.Directory, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(path) == f.config.FileExtension {
			total += info.Size()
		}
		return nil
	}); err != nil {
		return 0
	}
	return total
}

// MaxMemory returns the maximum allowed memory usage in bytes
func (f *fileStore[K, V]) MaxMemory(ctx context.Context) int64 {
	return f.opts.MaxMemory
}

// GetMany retrieves multiple values from the store
func (f *fileStore[K, V]) GetMany(ctx context.Context, keys []K) map[K]V {
	f.maybeCleanup() // Call before acquiring lock

	if len(keys) == 0 {
		return make(map[K]V)
	}

	if !f.tryLock() {
		return make(map[K]V)
	}
	defer func() {
		f.unlock()
	}()

	result := make(map[K]V)
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return result
		default:
			path := f.getPath(key)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var entry fileEntry[V]
			if err := json.Unmarshal(data, &entry); err != nil {
				continue
			}

			// Check if entry has expired
			if !entry.Expires.IsZero() && ttl.IsExpired(entry.Expires) {
				_ = f.Delete(ctx, key)
				continue
			}

			result[key] = entry.Value
		}
	}
	return result
}

// SetMany stores multiple values in the store
func (f *fileStore[K, V]) SetMany(ctx context.Context, entries map[K]V, ttlDuration time.Duration) error {
	f.maybeCleanup() // Call before acquiring lock

	if len(entries) == 0 {
		return nil
	}

	// Check context cancellation first
	select {
	case <-ctx.Done():
		return cacheErrors.ErrContextCanceled
	default:
	}

	if !f.tryLock() {
		return cacheErrors.WrapError("SetMany", nil, cacheErrors.ErrInvalidOperation)
	}
	defer func() {
		f.unlock()
	}()

	for key, value := range entries {
		select {
		case <-ctx.Done():
			return cacheErrors.ErrContextCanceled
		default:
			path := f.getPath(key)
			entry := fileEntry[V]{
				Value:       value,
				Expires:     ttl.GetExpirationTime(ttlDuration, f.ttlConfig),
				CreatedAt:   time.Now(),
				LastAccess:  time.Now(),
				AccessCount: 1,
				Size:        int64(unsafe.Sizeof(value)),
			}

			// Validate TTL
			if err := ttl.Validate(ttlDuration, f.ttlConfig); err != nil {
				return fmt.Errorf("failed to validate TTL for key %v: %w", key, err)
			}

			data, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("failed to marshal entry for key %v: %w", key, err)
			}

			// Create parent directory if it doesn't exist
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create directory for key %v: %w", key, err)
			}

			if err := os.WriteFile(path, data, 0o644); err != nil {
				return fmt.Errorf("failed to write file for key %v: %w", key, err)
			}
		}
	}
	return nil
}

// DeleteMany removes multiple values from the store
func (f *fileStore[K, V]) DeleteMany(ctx context.Context, keys []K) error {
	f.maybeCleanup() // Call before acquiring lock

	if len(keys) == 0 {
		return nil
	}

	if !f.tryLock() {
		return cacheErrors.WrapError("DeleteMany", nil, cacheErrors.ErrInvalidOperation)
	}
	defer func() {
		f.unlock()
	}()

	for _, key := range keys {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			path := f.getPath(key)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to delete file for key %v: %w", key, err)
			}
		}
	}
	return nil
}

// getPath returns the full path for a key
func (f *fileStore[K, V]) getPath(key K) string {
	// Replace slashes with a safe character and URL encode
	keyStr := toString(key)
	keyStr = strings.ReplaceAll(keyStr, "/", "__SLASH__")
	encodedKey := url.QueryEscape(keyStr)
	return filepath.Join(f.config.Directory, encodedKey+f.config.FileExtension)
}

// getKeyFromPath extracts the key from a file path
func (f *fileStore[K, V]) getKeyFromPath(path string) string {
	filename := strings.TrimSuffix(filepath.Base(path), f.config.FileExtension)
	key, _ := url.QueryUnescape(filename)
	// Restore slashes
	key = strings.ReplaceAll(key, "__SLASH__", "/")
	return key
}

// tryLock attempts to acquire the lock without blocking
func (f *fileStore[K, V]) tryLock() bool {
	return f.mu.TryLock()
}

// unlock releases the lock
func (f *fileStore[K, V]) unlock() {
	f.mu.Unlock()
}

// maybeCleanup checks if cleanup is needed and performs it if necessary
func (f *fileStore[K, V]) maybeCleanup() {
	// Lock cleanupMu for reading lastCleanup
	f.cleanupMu.Lock()
	timeSinceLastCleanup := time.Since(f.lastCleanup)
	f.cleanupMu.Unlock()
	if timeSinceLastCleanup < f.config.CleanupInterval {
		return
	}
	f.forceCleanup()
}

// forceCleanup performs a cleanup of expired files
func (f *fileStore[K, V]) forceCleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get all files in the directory
	files, err := os.ReadDir(f.config.Directory)
	if err != nil {
		return
	}

	// Track cleanup statistics
	cleaned := 0
	errors := 0

	// Check each file for expiration
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != f.config.FileExtension {
			continue
		}

		path := filepath.Join(f.config.Directory, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errors++
			continue
		}

		var entry fileEntry[V]
		if err := json.Unmarshal(data, &entry); err != nil {
			// Delete corrupted file
			if err := os.Remove(path); err != nil {
				errors++
			} else {
				cleaned++
			}
			continue
		}

		// Check if entry has expired
		if !entry.Expires.IsZero() && ttl.IsExpired(entry.Expires) {
			if err := os.Remove(path); err != nil {
				errors++
			} else {
				cleaned++
			}
		}
	}

	// Update last cleanup time with lock
	f.cleanupMu.Lock()
	f.lastCleanup = time.Now()
	f.cleanupMu.Unlock()
}

// Close closes the store
func (f *fileStore[K, V]) Close(ctx context.Context) error {
	var err error
	f.closeOnce.Do(func() {
		// Signal cleanup goroutine to stop
		close(f.stop)

		// Wait for cleanup goroutine to finish
		select {
		case <-f.done:
			// Cleanup goroutine finished successfully
		case <-ctx.Done():
			err = cacheErrors.WrapError("Close", nil, cacheErrors.ErrContextCanceled)
		case <-time.After(5 * time.Second):
			err = cacheErrors.WrapError("Close", nil, cacheErrors.ErrStoreTimeout)
		}

		// Perform final cleanup
		f.forceCleanup()
	})
	return err
}

// Note: Compression functions were removed as they were unused.
// If compression is needed in the future, implement it here.

// GetFilePath returns the actual file path for a key
func (f *fileStore[K, V]) GetFilePath(key K) string {
	// This method is used by tests to get the actual file path
	keyStr := toString(key)
	keyStr = strings.ReplaceAll(keyStr, "/", "__SLASH__")
	encodedKey := url.QueryEscape(keyStr)
	return filepath.Join(f.config.Directory, encodedKey+f.config.FileExtension)
}

// toString converts a key to its string representation
func toString[K comparable](key K) string {
	return fmt.Sprintf("%v", key)
}

// fromString converts a string back to the key type
func fromString[K comparable](s string) (K, bool) {
	var zero K
	var result K
	_, err := fmt.Sscanf(s, "%v", &result)
	if err != nil {
		return zero, false
	}
	return result, true
}
