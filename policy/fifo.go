// Package policy provides different cache eviction policies like FIFO, LRU, and LFU.
package policy

import (
	"container/list"
	"sync"
	"time"

	"github.com/gozephyr/gencache/internal"
)

// FIFO implements the Policy interface using First In First Out strategy
type FIFO[K comparable, V any] struct {
	items    *internal.SafeMap[K, *list.Element]
	list     *list.List
	mu       sync.RWMutex
	capacity int
}

type fifoItem[K comparable, V any] struct {
	key   K
	value V
	entry Entry[V]
}

// NewFIFO creates a new FIFO policy
func NewFIFO[K comparable, V any](opts ...Option) Policy[K, V] {
	options := &Options{
		MaxSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(options)
	}

	return &FIFO[K, V]{
		items:    internal.NewSafeMap[K, *list.Element](),
		list:     list.New(),
		capacity: options.MaxSize,
	}
}

// OnGet is called when an item is retrieved from the cache
func (p *FIFO[K, V]) OnGet(key K, value V) {
	// FIFO doesn't need to do anything on get
}

// OnSet is called when an item is added to the cache
func (p *FIFO[K, V]) OnSet(key K, value V, ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	entry := Entry[V]{
		Value:       value,
		Expires:     now.Add(ttl),
		CreatedAt:   now,
		LastAccess:  now,
		AccessCount: 1,
	}

	if element, exists := p.items.Get(key); exists {
		element.Value.(*fifoItem[K, V]).value = value
		element.Value.(*fifoItem[K, V]).entry = entry
	} else {
		// Check capacity before adding new item
		if p.items.Size() >= p.capacity {
			// Evict the oldest item
			if element := p.list.Front(); element != nil {
				item := element.Value.(*fifoItem[K, V])
				p.list.Remove(element)
				p.items.Delete(item.key)
			}
		}
		item := &fifoItem[K, V]{
			key:   key,
			value: value,
			entry: entry,
		}
		element := p.list.PushBack(item)
		p.items.Set(key, element)
	}
}

// OnDelete is called when an item is removed from the cache
func (p *FIFO[K, V]) OnDelete(key K) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if element, exists := p.items.Get(key); exists {
		p.list.Remove(element)
		p.items.Delete(key)
	}
}

// OnClear is called when the cache is cleared
func (p *FIFO[K, V]) OnClear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.list = list.New()
	p.items.Clear()
}

// Evict returns the next key to be evicted from the cache
func (p *FIFO[K, V]) Evict() (K, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.list.Len() == 0 {
		var zero K
		return zero, false
	}

	element := p.list.Front()
	item := element.Value.(*fifoItem[K, V])
	p.list.Remove(element)
	p.items.Delete(item.key)

	return item.key, true
}

// Size returns the number of items in the policy
func (p *FIFO[K, V]) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.items.Size()
}

// Capacity returns the maximum number of items the policy can hold
func (p *FIFO[K, V]) Capacity() int {
	return p.capacity
}
