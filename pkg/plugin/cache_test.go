// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestCache(ttl time.Duration, maxEntries int) (*cache[string], *fakeClock) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	c := newCache[string](ttl, maxEntries)
	c.now = clock.now
	return c, clock
}

func TestCacheExpiresAfterTTL(t *testing.T) {
	c, clock := newTestCache(10*time.Second, 4)
	c.set("a", "1")
	clock.advance(9 * time.Second)
	if v, ok := c.get("a"); !ok || v != "1" {
		t.Fatalf("before ttl: got %q %v", v, ok)
	}
	clock.advance(time.Second)
	if _, ok := c.get("a"); ok {
		t.Fatal("value still served at ttl")
	}
}

func TestCacheKeysAreIndependent(t *testing.T) {
	c, _ := newTestCache(time.Minute, 4)
	c.set("a", "1")
	c.set("b", "2")
	if v, _ := c.get("a"); v != "1" {
		t.Fatalf("a = %q", v)
	}
	if v, _ := c.get("b"); v != "2" {
		t.Fatalf("b = %q", v)
	}
	if _, ok := c.get("c"); ok {
		t.Fatal("unknown key found")
	}
}

func TestCacheSetDropsExpiredEntriesWhileNotFull(t *testing.T) {
	c, clock := newTestCache(10*time.Second, 8)
	c.set("a", "1")
	c.set("b", "2")
	clock.advance(10 * time.Second)
	c.set("c", "3")
	if n := c.len(); n != 1 {
		t.Fatalf("%d entries, want only the new one", n)
	}
}

func TestCacheFullKeepsLiveEntriesWhenExpiredOnesFreeRoom(t *testing.T) {
	c, clock := newTestCache(10*time.Second, 2)
	c.set("old", "1")
	clock.advance(6 * time.Second)
	c.set("mid", "2")
	clock.advance(5 * time.Second) // old has expired, mid has not
	c.set("new", "3")
	if _, ok := c.get("mid"); !ok {
		t.Fatal("live entry dropped while an expired one existed")
	}
	if _, ok := c.get("new"); !ok {
		t.Fatal("new entry missing")
	}
}

func TestCacheFullDropsOldestWhenNothingExpired(t *testing.T) {
	c, clock := newTestCache(time.Minute, 2)
	c.set("first", "1")
	clock.advance(time.Second)
	c.set("second", "2")
	clock.advance(time.Second)
	c.set("third", "3")
	if _, ok := c.get("first"); ok {
		t.Fatal("oldest entry kept")
	}
	for _, k := range []string{"second", "third"} {
		if _, ok := c.get(k); !ok {
			t.Fatalf("%s dropped", k)
		}
	}
}

func TestCacheOverwriteDoesNotEvict(t *testing.T) {
	c, _ := newTestCache(time.Minute, 2)
	c.set("a", "1")
	c.set("b", "2")
	c.set("a", "3")
	if v, ok := c.get("a"); !ok || v != "3" {
		t.Fatalf("a = %q %v", v, ok)
	}
	if _, ok := c.get("b"); !ok {
		t.Fatal("b evicted by an overwrite")
	}
}

func TestCacheNeverExceedsMaxEntries(t *testing.T) {
	c, _ := newTestCache(time.Minute, 3)
	for i := range 10 {
		c.set(strconv.Itoa(i), "v")
	}
	if n := c.len(); n != 3 {
		t.Fatalf("%d entries, want 3", n)
	}
}

func TestCacheConcurrentUse(t *testing.T) {
	c := newCache[int](time.Minute, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 200 {
				k := strconv.Itoa(j % 16)
				c.set(k, i)
				c.get(k)
			}
		}()
	}
	wg.Wait()
}

func (c *cache[V]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
