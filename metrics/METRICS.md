# GenCache Metrics Guide

This guide explains how to use the metrics functionality in GenCache to monitor and analyze cache performance.

## Overview

GenCache provides a comprehensive metrics system through the `metrics` package that allows you to track various aspects of your cache's performance and resource usage.

## Basic Usage

```go
import (
    "fmt"
    "github.com/gozephyr/gencache/metrics"
)

func main() {
    // Create metrics instance
    cacheMetrics := metrics.NewCacheMetrics()

    // Record cache operations
    cacheMetrics.RecordHit()     // When a cache hit occurs
    cacheMetrics.RecordMiss()    // When a cache miss occurs
    cacheMetrics.RecordEviction() // When an item is evicted
    cacheMetrics.UpdateSize(100) // Update current cache size

    // Get current metrics
    snapshot := cacheMetrics.GetSnapshot()
    
    // Print metrics
    fmt.Printf("Cache Size: %d\n", snapshot.Size)
    fmt.Printf("Hits: %d\n", snapshot.Hits)
    fmt.Printf("Misses: %d\n", snapshot.Misses)
}
```

## Configuring Metrics with Cache

### Basic Configuration

```go
import (
    "github.com/gozephyr/gencache"
    "github.com/gozephyr/gencache/metrics"
    "github.com/gozephyr/gencache/store"
)

func main() {
    // Create metrics instance
    cacheMetrics := metrics.NewCacheMetrics()

    // Create a memory store with metrics
    memStore, err := store.NewMemoryStore[string, string](
        store.WithMetricsConfig[string, string](metrics.MetricsConfig{
            ExporterType: metrics.StandardExporter,
            CacheName: "my-cache",
            Labels: map[string]string{
                "environment": "production",
            },
        }),
    )
    if err != nil {
        // Handle error
    }

    // Create cache with metrics enabled
    cache := gencache.New[string, string](
        gencache.WithStore[string, string](memStore),
        gencache.WithMetricsConfig[string, string](gencache.MetricsConfig{
            ExporterType: metrics.StandardExporter,
            CacheName: "my-cache",
            Labels: map[string]string{
                "environment": "production",
            },
        }),
    )
}
```

### Prometheus Metrics Configuration

To use Prometheus metrics, you need to configure the metrics exporter with `PrometheusExporterType`. Here's how:

```go
import (
    "github.com/gozephyr/gencache"
    "github.com/gozephyr/gencache/metrics"
    "github.com/gozephyr/gencache/store"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    // Create cache with Prometheus metrics
    cache := gencache.New[string, string](
        gencache.WithMetricsConfig[string, string](gencache.MetricsConfig{
            ExporterType: metrics.PrometheusExporterType,
            CacheName: "my-cache",
            Labels: map[string]string{
                "environment": "production",
                "service": "api",
                "region": "us-west",
            },
        }),
    )

    // The following Prometheus metrics will be automatically collected:
    // - cache_hits_total: Total number of cache hits
    // - cache_misses_total: Total number of cache misses
    // - cache_evictions_total: Total number of cache evictions
    // - cache_size: Current number of items in the cache
    // - cache_memory_bytes: Current memory usage of the cache
}
```

### Prometheus Metrics with Multiple Caches

When using multiple caches, you can distinguish them using labels:

```go
// Cache for user data
userCache := gencache.New[string, string](
    gencache.WithMetricsConfig[string, string](gencache.MetricsConfig{
        ExporterType: metrics.PrometheusExporterType,
        CacheName: "user-cache",
        Labels: map[string]string{
            "cache_type": "user",
            "environment": "production",
        },
    }),
)

// Cache for session data
sessionCache := gencache.New[string, string](
    gencache.WithMetricsConfig[string, string](gencache.MetricsConfig{
        ExporterType: metrics.PrometheusExporterType,
        CacheName: "session-cache",
        Labels: map[string]string{
            "cache_type": "session",
            "environment": "production",
        },
    }),
)
```

### Prometheus Query Examples

Once metrics are being collected, you can query them in Prometheus:

```promql
# Get hit ratio for a specific cache
rate(cache_hits_total{cache="my-cache"}[5m]) / 
(rate(cache_hits_total{cache="my-cache"}[5m]) + rate(cache_misses_total{cache="my-cache"}[5m]))

# Get memory usage across all caches
cache_memory_bytes

# Get eviction rate for production caches
rate(cache_evictions_total[5m])

# Get cache size by type
cache_size{cache_type=~"user|session"}
```

### Prometheus Alert Rules

Example alert rules for monitoring cache health:

```yaml
groups:
- name: cache_alerts
  rules:
  - alert: HighCacheMissRate
    expr: rate(cache_misses_total[5m]) / (rate(cache_hits_total[5m]) + rate(cache_misses_total[5m])) > 0.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: High cache miss rate detected
      description: Cache miss rate is above 50% for the last 5 minutes

  - alert: HighMemoryUsage
    expr: cache_memory_bytes > 1e9  # 1GB
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: High cache memory usage
      description: Cache memory usage is above 1GB

  - alert: HighEvictionRate
    expr: rate(cache_evictions_total[5m]) > 100
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: High cache eviction rate
      description: Cache is evicting more than 100 items per second
```

## Available Metrics

### 1. Basic Cache Metrics

- `Size`: Current number of items in cache
- `Hits`: Number of successful cache lookups
- `Misses`: Number of failed cache lookups
- `Evictions`: Number of items evicted from cache
- `MemoryUsage`: Current memory consumption in bytes
- `LastOperationTime`: Timestamp of last cache operation

### 2. Batch Operation Metrics

- `BatchOperations`: Total number of batch operations
- `BatchItems`: Total items processed in batches
- `BatchSuccess`: Number of successful batch operations
- `BatchErrors`: Number of failed batch operations
- `BatchTimeouts`: Number of batch operation timeouts
- `LastBatchOperation`: Timestamp of last batch operation

### 3. Compression Metrics

- `CompressedItems`: Number of compressed items
- `DecompressedItems`: Number of decompressed items
- `CompressedBytes`: Total size of compressed data
- `DecompressedBytes`: Total size of decompressed data
- `CompressionRatio`: Current compression ratio
- `LastCompression`: Timestamp of last compression operation

### 4. Pool Metrics

- `PoolCreated`: Number of objects created in pool
- `PoolDestroyed`: Number of objects destroyed from pool
- `PoolCurrentSize`: Current size of object pool
- `LastPoolCleanup`: Timestamp of last pool cleanup

### 5. Resource Usage

- `GoRoutines`: Current number of active goroutines
- `LastCleanup`: Timestamp of last resource cleanup

## Advanced Usage

### Getting Metrics Snapshots

```go
// Get a thread-safe snapshot of all metrics
snapshot := cacheMetrics.GetSnapshot()

// Access specific metrics
fmt.Printf("Hit Ratio: %.2f%%\n", 
    float64(snapshot.Hits)/(float64(snapshot.Hits+snapshot.Misses))*100)
fmt.Printf("Compression Ratio: %.2f\n", snapshot.CompressionRatio)
```

### Resetting Metrics

```go
// Reset all metrics to zero
cacheMetrics.Reset()
```

### Monitoring Best Practices

1. **Regular Monitoring**
   - Take regular snapshots of metrics
   - Monitor hit/miss ratios
   - Track memory usage trends
   - Watch for unusual patterns

2. **Performance Analysis**
   - Use metrics to identify bottlenecks
   - Monitor compression effectiveness
   - Track batch operation efficiency
   - Analyze resource utilization

3. **Alerting**
   - Set thresholds for critical metrics
   - Monitor error rates
   - Track resource exhaustion
   - Watch for performance degradation

## Example: Complete Monitoring Setup

```go
import (
    "time"
    "github.com/gozephyr/gencache/metrics"
)

func monitorCache(cacheMetrics *metrics.CacheMetrics) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        snapshot := cacheMetrics.GetSnapshot()
        
        // Calculate key metrics
        hitRatio := float64(snapshot.Hits) / float64(snapshot.Hits+snapshot.Misses) * 100
        compressionRatio := snapshot.CompressionRatio
        
        // Log metrics
        log.Printf("Cache Stats:")
        log.Printf("- Size: %d items", snapshot.Size)
        log.Printf("- Hit Ratio: %.2f%%", hitRatio)
        log.Printf("- Memory Usage: %d bytes", snapshot.MemoryUsage)
        log.Printf("- Compression Ratio: %.2f", compressionRatio)
        
        // Alert on critical conditions
        if hitRatio < 50 {
            log.Printf("WARNING: Low hit ratio detected")
        }
        if snapshot.MemoryUsage > 1e9 { // 1GB
            log.Printf("WARNING: High memory usage detected")
        }
    }
}
```

## Integration with Monitoring Systems

The metrics package can be easily integrated with monitoring systems like Prometheus or custom monitoring solutions. Use the `GetSnapshot()` method to periodically collect metrics and send them to your monitoring system.

## Troubleshooting

Common issues and solutions:

1. **High Memory Usage**
   - Check `MemoryUsage` metric
   - Monitor `Size` vs `Capacity`
   - Review compression settings

2. **Low Hit Ratio**
   - Analyze access patterns
   - Review cache size settings
   - Check eviction policies

3. **High Error Rates**
   - Monitor `BatchErrors`
   - Check resource constraints
   - Review timeout settings