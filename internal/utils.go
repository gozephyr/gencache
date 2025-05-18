// Package internal provides internal utility functions and types used across the gencache package.
package internal

import (
	"sync"
	"time"
)

// Expired returns true if the given time is in the past
func Expired(t time.Time) bool {
	return time.Now().After(t)
}

// SafeCounter is a thread-safe counter
type SafeCounter struct {
	mu    sync.RWMutex
	count int64
}

// NewSafeCounter creates a new thread-safe counter
func NewSafeCounter() *SafeCounter {
	return &SafeCounter{}
}

// Increment increases the counter by 1
func (c *SafeCounter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
}

// Decrement decreases the counter by 1
func (c *SafeCounter) Decrement() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.count > 0 {
		c.count--
	}
}

// Get returns the current count
func (c *SafeCounter) Get() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.count
}

// Reset sets the counter to 0
func (c *SafeCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count = 0
}

// SafeMap is a thread-safe map
type SafeMap[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

// NewSafeMap creates a new thread-safe map
func NewSafeMap[K comparable, V any]() *SafeMap[K, V] {
	return &SafeMap[K, V]{
		data: make(map[K]V),
	}
}

// Get retrieves a value from the map
func (m *SafeMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.data[key]
	return value, exists
}

// Set stores a value in the map
func (m *SafeMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

// Delete removes a value from the map
func (m *SafeMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

// Clear removes all values from the map
func (m *SafeMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[K]V)
}

// Size returns the number of items in the map
func (m *SafeMap[K, V]) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}
