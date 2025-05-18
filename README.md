# GenCache

[![Go Report Card](https://goreportcard.com/badge/github.com/gozephyr/gencache)](https://goreportcard.com/report/github.com/gozephyr/gencache)
[![GoDoc](https://godoc.org/github.com/gozephyr/gencache?status.svg)](https://godoc.org/github.com/gozephyr/gencache)
[![License](https://img.shields.io/github/license/gozephyr/gencache)](https://github.com/gozephyr/gencache/blob/main/LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.18%2B-blue)](https://golang.org)
[![Build Status](https://github.com/gozephyr/gencache/actions/workflows/ci.yml/badge.svg)](https://github.com/gozephyr/gencache/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/gozephyr/gencache/branch/main/graph/badge.svg)](https://codecov.io/gh/gozephyr/gencache)
[![Release](https://img.shields.io/github/v/release/gozephyr/gencache?include_prereleases)](https://github.com/gozephyr/gencache/releases)

A high-performance, generic caching library with support for multiple storage backends, batch operations, compression, and object pooling.

## Summary

GenCache is a robust, thread-safe caching library designed for high-performance applications. It provides a unified interface for caching with support for in-memory and file-based storage, batch operations, compression, and object pooling. Built with Go's generics, it offers type safety and flexibility for various use cases.

## Features

- **Generic Caching**: Type-safe caching with Go generics.
- **Multiple Storage Backends**:
  - In-memory store for fastest access.
  - File-based store for persistence.
- **Batch Operations**: Efficient bulk processing for high throughput.
- **Compression Support**: Memory optimization with configurable compression levels.
- **Object Pooling**: Reduced GC pressure and improved performance.
- **Unified Metrics and Monitoring**: Track hit/miss rates, memory usage, and operation latencies. See [Metrics Guide](metrics/METRICS.md) for detailed usage.
- **Resource Coordination and Cleanup**: Automatic cleanup of expired items and resources.
- **Configurable Components**: Tune cleanup intervals, size limits, and compression settings.
- **TTL (Time-To-Live) Support**: Automatic expiration of cached items.
- **Thread-Safe Operations**: Concurrent access without data races.

## Installation

```bash
go get github.com/gozephyr/gencache
```

## Basic Usage

### Memory Store Example

```go
import (
    "context"
    "time"
    "github.com/gozephyr/gencache"
    "github.com/gozephyr/gencache/store"
    "github.com/gozephyr/gencache/ttl"
)

func main() {
    ctx := context.Background()

    // Create a memory store
    memStore, err := store.NewMemoryStore[string, string](ctx)
    if err != nil {
        // Handle error
    }
    defer memStore.Close(ctx)

    // Create a cache with memory store
    cache := gencache.New[string, string](
        gencache.WithStore[string, string](memStore),
        gencache.WithTTLConfig[string, string](ttl.DefaultConfig()),
    )
    defer cache.Close()

    // Set and get values
    err = cache.Set("key", "value", time.Minute)
    if err != nil {
        // Handle error
    }

    value, err := cache.Get("key")
    if err != nil {
        // Handle error
    }
}
```

### Memory Store Capabilities

- Fastest access with in-memory storage.
- Configurable size limits.
- Automatic cleanup of expired items.
- Thread-safe operations.
- Memory usage tracking.

### Memory Store Performance

1. **Memory Usage**:
   - Proportional to number of items.
   - Each item stores key, value, and metadata.
   - Overhead: ~100 bytes per item.

2. **CPU Usage**:
   - Minimal overhead for operations.
   - Fast access and updates.
   - Efficient cleanup of expired items.

### File Store

```go
import (
    "context"
    "time"
    "github.com/gozephyr/gencache"
    "github.com/gozephyr/gencache/store"
    "github.com/gozephyr/gencache/ttl"
)

func main() {
    ctx := context.Background()

    // Create a file store
    fileStore, err := store.NewFileStore[string, string](ctx, &store.FileConfig{
        Directory:          "/tmp/cache",
        FileExtension:      ".cache",
        CompressionEnabled: true,
        CompressionLevel:   6,
        CleanupInterval:    time.Hour,
    })
    if err != nil {
        // Handle error
    }
    defer fileStore.Close(ctx)

    // Create a cache with file store
    cache := gencache.New[string, string](
        gencache.WithStore[string, string](fileStore),
        gencache.WithTTLConfig[string, string](ttl.DefaultConfig()),
    )
    defer cache.Close()

    // Set and get values
    err = cache.Set("key", "value", time.Minute)
    if err != nil {
        // Handle error
    }

    value, err := cache.Get("key")
    if err != nil {
        // Handle error
    }
}
```

## Advanced Features

### Batch Operations

```go
import (
    "context"
    "time"
    "github.com/gozephyr/gencache"
)

func main() {
    ctx := context.Background()

    // Create a cache with batch operations
    cache := gencache.New[string, string](
        gencache.WithBatchConfig[string, string](gencache.BatchConfig{
            MaxBatchSize:     1000,
            OperationTimeout: 5 * time.Second,
            MaxConcurrent:    10,
        }),
    )
    defer cache.Close()

    // Create batch cache
    batchCache := gencache.NewBatchCache(cache, gencache.DefaultBatchConfig())

    // Batch get operation
    values := batchCache.GetMany(ctx, []string{"key1", "key2", "key3"})

    // Batch set operation
    err := batchCache.SetMany(ctx, map[string]string{
        "key1": "value1",
        "key2": "value2",
        "key3": "value3",
    }, time.Minute)
    if err != nil {
        // Handle error
    }
}
```

### Object Pooling

```go
import (
    "context"
    "time"
    "github.com/gozephyr/gencache"
)

type MyStruct struct {
    ID   int
    Name string
}

func main() {
    ctx := context.Background()

    // Create a cache with object pooling
    cache := gencache.New[string, *MyStruct](
        gencache.WithPoolConfig[string, *MyStruct](gencache.PoolConfig{
            MaxSize:       1000,
            MinSize:       10,
            CleanupPeriod: 5 * time.Minute,
            MaxIdleTime:   10 * time.Minute,
        }),
    )
    defer cache.Close()

    // Create pooled cache
    pooledCache := gencache.NewPooledCache(cache, gencache.DefaultPoolConfig())

    // Use pooled objects
    obj := &MyStruct{ID: 1, Name: "test"}
    err := pooledCache.Set("key", obj, time.Minute)
    if err != nil {
        // Handle error
    }

    value, err := pooledCache.Get("key")
    if err != nil {
        // Handle error
    }
}
```

## Storage Backends

### Memory Store

- Fastest access with in-memory storage.
- Configurable size limits.
- Automatic cleanup of expired items.
- Thread-safe operations.
- Memory usage tracking.

### File Store Overview

- Persistent storage using filesystem.
- Configurable directory and file extension.
- Automatic cleanup of expired files.
- Compression support.
- Thread-safe operations.
- File-based locking.

## Performance Considerations

### Memory Store Metrics

1. **Memory Usage**:
   - Proportional to number of items.
   - Each item stores key, value, and metadata.
   - Overhead: ~100 bytes per item.

2. **CPU Usage**:
   - Minimal overhead for operations.
   - Fast access and updates.
   - Efficient cleanup of expired items.

### File Store Performance

1. **Memory Usage**:
   - Minimal memory overhead.
   - Values stored on disk.
   - Memory usage for file handles and buffers.

2. **CPU Usage**:
   - File I/O operations.
   - Compression/decompression overhead.
   - Periodic cleanup of expired files.

3. **Disk Usage**:
   - Proportional to stored data size.
   - Additional space for metadata.
   - Compression can reduce disk usage.

## Benchmarks

- **Memory Store**: ~100,000 operations per second for small values.
- **File Store**: ~10,000 operations per second for small values, depending on disk speed and compression settings.

## Best Practices

1. **Storage Selection**:
   - Use memory store for fastest access.
   - Use file store for persistence.
   - Consider data size and access patterns.

2. **Configuration**:
   - Tune cleanup intervals.
   - Set appropriate size limits.
   - Configure compression settings.

3. **Error Handling**:
   - Handle storage errors.
   - Implement retry logic.
   - Monitor error rates.

4. **Monitoring**:
   - Track hit/miss rates.
   - Monitor memory usage.
   - Watch disk usage (file store).
   - Track operation latencies.

## Error Handling

The library provides comprehensive error handling:

```go
if err := cache.Set("key", "value", time.Minute); err != nil {
    switch {
    case errors.Is(err, errors.ErrKeyNotFound):
        // Handle key not found
    case errors.Is(err, errors.ErrCapacityLimit):
        // Handle capacity limit
    case errors.Is(err, errors.ErrContextCanceled):
        // Handle context cancellation
    default:
        // Handle other errors
    }
}
```

## Metrics Example

For detailed metrics usage and examples, please refer to the [Metrics Guide](metrics/METRICS.md).

## Eviction Policies

GenCache supports pluggable cache eviction policies via the `policy` package. You can choose from several built-in policies:

- **LRU (Least Recently Used)**: Evicts the least recently accessed items first.
- **LFU (Least Frequently Used)**: Evicts the least frequently accessed items first.
- **FIFO (First In First Out)**: Evicts the oldest items first.

### Policy Package

The `policy` package provides a flexible framework for implementing custom cache eviction policies. It defines a generic `Policy` interface that can be implemented for any key-value type combination.

#### Policy Interface

```go
type Policy[K comparable, V any] interface {
    // OnGet is called when an item is retrieved from the cache
    OnGet(key K, value V)

    // OnSet is called when an item is added to the cache
    OnSet(key K, value V, ttl time.Duration)

    // OnDelete is called when an item is removed from the cache
    OnDelete(key K)

    // OnClear is called when the cache is cleared
    OnClear()

    // Evict returns the next key to be evicted from the cache
    Evict() (K, bool)

    // Size returns the number of items in the policy
    Size() int

    // Capacity returns the maximum number of items the policy can hold
    Capacity() int
}
```

#### Entry Metadata

Each cache entry maintains metadata to help with policy decisions:

```go
type Entry[V any] struct {
    Value       V         // The cached value
    Expires     time.Time // When the entry expires
    CreatedAt   time.Time // When the entry was created
    LastAccess  time.Time // When the entry was last accessed
    AccessCount int64     // Number of times the entry was accessed
}
```
