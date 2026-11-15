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
	gen        uint64
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
	c.store(key, value)
}

// generation counts the calls to clear.
func (c *cache[V]) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// setIfGeneration stores value like set only when clear has not run since gen was read, and reports whether it stored.
func (c *cache[V]) setIfGeneration(key string, value V, gen uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return false
	}
	c.store(key, value)
	return true
}

// store is set without the lock.
func (c *cache[V]) store(key string, value V) {
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

// clear drops every entry and starts a new generation.
func (c *cache[V]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
	c.gen++
}
