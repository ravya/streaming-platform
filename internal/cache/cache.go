// Package cache is a TTL cache with request coalescing: N concurrent
// identical misses trigger ONE backend call (protects the store during spikes).
package cache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	v   V
	exp time.Time
}

type call[V any] struct {
	wg  sync.WaitGroup
	v   V
	err error
}

type TTL[V any] struct {
	max int
	now func() time.Time

	mu       sync.Mutex
	items    map[string]entry[V]
	inflight map[string]*call[V]
}

func New[V any](maxEntries int) *TTL[V] {
	return &TTL[V]{max: maxEntries, now: time.Now, items: map[string]entry[V]{}, inflight: map[string]*call[V]{}}
}

// Do returns the cached value for key, or runs fn once for all concurrent callers.
// The bool reports whether the value came from cache.
func (c *TTL[V]) Do(key string, ttl time.Duration, fn func() (V, error)) (V, bool, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok && c.now().Before(e.exp) {
		c.mu.Unlock()
		return e.v, true, nil
	}
	if cl, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		cl.wg.Wait()
		return cl.v, false, cl.err
	}
	cl := &call[V]{}
	cl.wg.Add(1)
	c.inflight[key] = cl
	c.mu.Unlock()

	cl.v, cl.err = fn()

	c.mu.Lock()
	delete(c.inflight, key)
	if cl.err == nil {
		if len(c.items) >= c.max { // crude bound: drop expired, else reset
			c.evictLocked()
		}
		c.items[key] = entry[V]{cl.v, c.now().Add(ttl)}
	}
	c.mu.Unlock()
	cl.wg.Done()
	return cl.v, false, cl.err
}

func (c *TTL[V]) evictLocked() {
	now := c.now()
	for k, e := range c.items {
		if now.After(e.exp) {
			delete(c.items, k)
		}
	}
	if len(c.items) >= c.max {
		c.items = map[string]entry[V]{}
	}
}
