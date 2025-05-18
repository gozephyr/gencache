package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/gozephyr/gencache"
	"github.com/gozephyr/gencache/policy"
	"github.com/gozephyr/gencache/store"
	"github.com/gozephyr/gencache/ttl"
)

func BenchmarkCacheOperations(b *testing.B) {
	ctx := context.Background()

	// Create cache with different configurations
	configs := []struct {
		name string
		opts []gencache.Option[string, string]
	}{
		{
			name: "Memory_Only",
			opts: []gencache.Option[string, string]{
				gencache.WithMaxSize[string, string](1000),
				gencache.WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(1000))),
			},
		},
		{
			name: "With_Memory_Store",
			opts: []gencache.Option[string, string]{
				gencache.WithStore[string, string](func() store.Store[string, string] {
					s, _ := store.NewMemoryStore[string, string](ctx)
					return s
				}()),
				gencache.WithMaxSize[string, string](1000),
			},
		},
		{
			name: "With_TTL",
			opts: []gencache.Option[string, string]{
				gencache.WithTTLConfig[string, string](ttl.Config{
					MinTTL: time.Millisecond * 10,
					MaxTTL: time.Hour,
				}),
				gencache.WithCleanupInterval[string, string](time.Second),
			},
		},
	}

	for _, cfg := range configs {
		b.Run(cfg.name, func(b *testing.B) {
			cache := gencache.New[string, string](cfg.opts...)
			defer cache.Close()

			// Benchmark Set operation
			b.Run("Set", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i))
					_ = cache.Set(key, "value", time.Minute)
				}
			})

			// Benchmark Get operation
			b.Run("Get", func(b *testing.B) {
				// Pre-populate cache
				for i := 0; i < 1000; i++ {
					key := "key" + string(rune(i))
					_ = cache.Set(key, "value", time.Minute)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i%1000))
					_, _ = cache.Get(key)
				}
			})

			// Benchmark Delete operation
			b.Run("Delete", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i))
					_ = cache.Delete(key)
				}
			})
		})
	}
}

func BenchmarkCacheConcurrent(b *testing.B) {
	cache := gencache.New[string, string](
		gencache.WithMaxSize[string, string](1000),
		gencache.WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(1000))),
	)
	defer cache.Close()

	b.Run("Concurrent_Set", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := "key" + string(rune(i))
				_ = cache.Set(key, "value", time.Minute)
				i++
			}
		})
	})

	b.Run("Concurrent_Get", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < 1000; i++ {
			key := "key" + string(rune(i))
			_ = cache.Set(key, "value", time.Minute)
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := "key" + string(rune(i%1000))
				_, _ = cache.Get(key)
				i++
			}
		})
	})
}

func BenchmarkCacheWithDifferentSizes(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run("Size_"+string(rune(size)), func(b *testing.B) {
			cache := gencache.New[string, string](
				gencache.WithMaxSize[string, string](size),
				gencache.WithPolicy[string, string](policy.NewLRU[string, string](policy.WithMaxSize(size))),
			)
			defer cache.Close()

			// Pre-populate cache
			for i := 0; i < size; i++ {
				key := "key" + string(rune(i))
				_ = cache.Set(key, "value", time.Minute)
			}

			b.Run("Get", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i%size))
					_, _ = cache.Get(key)
				}
			})

			b.Run("Set", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i))
					_ = cache.Set(key, "value", time.Minute)
				}
			})
		})
	}
}

func BenchmarkCacheWithDifferentValueSizes(b *testing.B) {
	valueSizes := []int{10, 100, 1000, 10000}

	for _, size := range valueSizes {
		b.Run("ValueSize_"+string(rune(size)), func(b *testing.B) {
			cache := gencache.New[string, string](
				gencache.WithMaxSize[string, string](1000),
			)
			defer cache.Close()

			value := string(make([]byte, size))

			b.Run("Set", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i))
					_ = cache.Set(key, value, time.Minute)
				}
			})

			b.Run("Get", func(b *testing.B) {
				// Pre-populate cache
				for i := 0; i < 1000; i++ {
					key := "key" + string(rune(i))
					_ = cache.Set(key, value, time.Minute)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := "key" + string(rune(i%1000))
					_, _ = cache.Get(key)
				}
			})
		})
	}
}
