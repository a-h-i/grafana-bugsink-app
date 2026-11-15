// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"sync"
	"time"
)

const (
	cacheTTL        = 15 * time.Second
	cacheMaxEntries = 32
)

type cacheEntry[V any] struct {
	value   V
	expires time.Time
}

// cache is a small time-limited map safe for concurrent use.
type cache[V any] struct {
	mu         sync.Mutex
	entries    map[string]cacheEntry[V]
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func newCache[V any](ttl time.Duration, maxEntries int) *cache[V] {
	return &cache[V]{entries: make(map[string]cacheEntry[V]), ttl: ttl, maxEntries: maxEntries, now: time.Now}
}

func (c *cache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expires) {
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	return e.value, true
}

// set stores value after dropping expired entries; when the cache is still full it drops the oldest.
func (c *cache[V]) set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		c.dropOldest()
	}
	c.entries[key] = cacheEntry[V]{value: value, expires: now.Add(c.ttl)}
}

func (c *cache[V]) dropOldest() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, e := range c.entries {
		if first || e.expires.Before(oldest) {
			oldestKey, oldest, first = k, e.expires, false
		}
	}
	delete(c.entries, oldestKey)
}
