package policy

import (
	"container/list"
	"sync"
	"time"
)

// LRU implements the Policy interface using Least Recently Used strategy
type LRU[K comparable, V any] struct {
	items    map[K]*list.Element
	list     *list.List
	mu       sync.RWMutex
	capacity int
}

type lruItem[K comparable, V any] struct {
	key   K
	value V
	entry Entry[V]
}

// NewLRU creates a new LRU policy
func NewLRU[K comparable, V any](opts ...Option) Policy[K, V] {
	options := &Options{
		MaxSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(options)
	}

	return &LRU[K, V]{
		items:    make(map[K]*list.Element),
		list:     list.New(),
		capacity: options.MaxSize,
	}
}

// OnGet is called when an item is retrieved from the cache
func (p *LRU[K, V]) OnGet(key K, value V) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if element, exists := p.items[key]; exists {
		item := element.Value.(*lruItem[K, V])
		item.entry.LastAccess = time.Now()
		item.entry.AccessCount++
		p.list.MoveToFront(element)
	}
}

// OnSet is called when an item is added to the cache
func (p *LRU[K, V]) OnSet(key K, value V, ttl time.Duration) {
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

	if element, exists := p.items[key]; exists {
		item := element.Value.(*lruItem[K, V])
		item.value = value
		item.entry = entry
		p.list.MoveToFront(element)
	} else {
		// Check capacity before adding new item
		if len(p.items) >= p.capacity {
			// Evict the least recently used item
			if element := p.list.Back(); element != nil {
				item := element.Value.(*lruItem[K, V])
				p.list.Remove(element)
				delete(p.items, item.key)
			}
		}
		item := &lruItem[K, V]{
			key:   key,
			value: value,
			entry: entry,
		}
		element := p.list.PushFront(item)
		p.items[key] = element
	}
}

// OnDelete is called when an item is removed from the cache
func (p *LRU[K, V]) OnDelete(key K) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if element, exists := p.items[key]; exists {
		p.list.Remove(element)
		delete(p.items, key)
	}
}

// OnClear is called when the cache is cleared
func (p *LRU[K, V]) OnClear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.list = list.New()
	p.items = make(map[K]*list.Element)
}

// Evict returns the next key to be evicted from the cache
func (p *LRU[K, V]) Evict() (K, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.list.Len() == 0 {
		var zero K
		return zero, false
	}

	element := p.list.Back()
	item := element.Value.(*lruItem[K, V])
	p.list.Remove(element)
	delete(p.items, item.key)

	return item.key, true
}

// Size returns the number of items in the policy
func (p *LRU[K, V]) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.list.Len()
}

// Capacity returns the maximum number of items the policy can hold
func (p *LRU[K, V]) Capacity() int {
	return p.capacity
}
