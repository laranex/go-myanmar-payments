package myanmarpayments

import (
	"sync"
	"time"
)

// TokenCache stores access tokens between calls (used by Yoma MMQR).
// Implement it on top of Redis or similar to share tokens between processes.
type TokenCache interface {
	Get(key string) (string, bool)
	Set(key, value string, ttl time.Duration)
	Delete(key string)
}

// MemoryTokenCache is an in-memory TokenCache that is safe for concurrent use.
// It only lives as long as the process.
type MemoryTokenCache struct {
	mu    sync.Mutex
	items map[string]memoryItem
	now   func() time.Time
}

type memoryItem struct {
	value     string
	expiresAt time.Time
}

// NewMemoryTokenCache returns an empty MemoryTokenCache.
func NewMemoryTokenCache() *MemoryTokenCache {
	return &MemoryTokenCache{items: map[string]memoryItem{}, now: time.Now}
}

// Get returns the cached value if it has not expired.
func (c *MemoryTokenCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		return "", false
	}
	if !item.expiresAt.IsZero() && !c.now().Before(item.expiresAt) {
		delete(c.items, key)
		return "", false
	}

	return item.value, true
}

// Set stores value for ttl; a ttl of zero or less never expires.
func (c *MemoryTokenCache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item := memoryItem{value: value}
	if ttl > 0 {
		item.expiresAt = c.now().Add(ttl)
	}
	c.items[key] = item
}

// Delete removes key.
func (c *MemoryTokenCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}
