package policy

import (
	"container/heap"
	"sync"
	"time"

	"github.com/gozephyr/gencache/internal"
)

// LFU implements the Policy interface using Least Frequently Used strategy
type LFU[K comparable, V any] struct {
	items    *internal.SafeMap[K, *lfuItem[K, V]]
	queue    *lfuQueue[K, V]
	mu       sync.RWMutex
	capacity int
}

type lfuItem[K comparable, V any] struct {
	key   K
	value V
	entry Entry[V]
	index int
}

type lfuQueue[K comparable, V any] []*lfuItem[K, V]

func (q lfuQueue[K, V]) Len() int { return len(q) }

func (q lfuQueue[K, V]) Less(i, j int) bool {
	return q[i].entry.AccessCount < q[j].entry.AccessCount
}

func (q lfuQueue[K, V]) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *lfuQueue[K, V]) Push(x any) {
	n := len(*q)
	item := x.(*lfuItem[K, V])
	item.index = n
	*q = append(*q, item)
}

func (q *lfuQueue[K, V]) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*q = old[0 : n-1]
	return item
}

// NewLFU creates a new LFU policy
func NewLFU[K comparable, V any](opts ...Option) Policy[K, V] {
	options := &Options{
		MaxSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(options)
	}

	queue := &lfuQueue[K, V]{}
	heap.Init(queue)

	return &LFU[K, V]{
		items:    internal.NewSafeMap[K, *lfuItem[K, V]](),
		queue:    queue,
		capacity: options.MaxSize,
	}
}

// OnGet is called when an item is retrieved from the cache
func (p *LFU[K, V]) OnGet(key K, value V) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if item, exists := p.items.Get(key); exists {
		item.entry.AccessCount++
		heap.Fix(p.queue, item.index)
	}
}

// OnSet is called when an item is added to the cache
func (p *LFU[K, V]) OnSet(key K, value V, ttl time.Duration) {
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

	if item, exists := p.items.Get(key); exists {
		item.value = value
		item.entry = entry
		heap.Fix(p.queue, item.index)
	} else {
		// Check capacity before adding new item
		if p.items.Size() >= p.capacity {
			// Evict the least frequently used item
			if p.queue.Len() > 0 {
				item := heap.Pop(p.queue).(*lfuItem[K, V])
				p.items.Delete(item.key)
			}
		}
		item := &lfuItem[K, V]{
			key:   key,
			value: value,
			entry: entry,
		}
		heap.Push(p.queue, item)
		p.items.Set(key, item)
	}
}

// OnDelete is called when an item is removed from the cache
func (p *LFU[K, V]) OnDelete(key K) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if item, exists := p.items.Get(key); exists {
		heap.Remove(p.queue, item.index)
		p.items.Delete(key)
	}
}

// OnClear is called when the cache is cleared
func (p *LFU[K, V]) OnClear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.queue = &lfuQueue[K, V]{}
	heap.Init(p.queue)
	p.items.Clear()
}

// Evict returns the next key to be evicted from the cache
func (p *LFU[K, V]) Evict() (K, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.queue.Len() == 0 {
		var zero K
		return zero, false
	}

	item := heap.Pop(p.queue).(*lfuItem[K, V])
	p.items.Delete(item.key)

	return item.key, true
}

// Size returns the number of items in the policy
func (p *LFU[K, V]) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.items.Size()
}

// Capacity returns the maximum number of items the policy can hold
func (p *LFU[K, V]) Capacity() int {
	return p.capacity
}
