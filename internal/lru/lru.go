// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

// Package lru provides a size bounded cache with optional expiry
package lru

import (
	"container/list"
	"math/rand/v2"
	"sync"
	"time"
)

// Cache is a concurrency safe LRU cache with string keys
type Cache[V any] struct {
	mu      sync.Mutex
	max     int
	ttl     time.Duration
	ll      *list.List
	items   map[string]*list.Element
	onEvict func(key string, v V)
	now     func() time.Time
}

type entry[V any] struct {
	key     string
	value   V
	expires time.Time
}

// Option configures a Cache
type Option[V any] func(*Cache[V])

// WithTTL lets entries expire after ttl (+/- 10% jitter, so entries added at the same time do not expire at once)
func WithTTL[V any](ttl time.Duration) Option[V] {
	return func(c *Cache[V]) {
		c.ttl = ttl
	}
}

// WithEvictCallback calls f when an entry is removed or replaced, e.g. to release resources.
// f is called while the cache is locked, so it must not use the cache and should not block.
func WithEvictCallback[V any](f func(key string, v V)) Option[V] {
	return func(c *Cache[V]) {
		c.onEvict = f
	}
}

// New creates a cache holding at most max entries
func New[V any](max int, opts ...Option[V]) *Cache[V] {
	c := &Cache[V]{
		max:   max,
		ll:    list.New(),
		items: make(map[string]*list.Element),
		now:   time.Now,
	}

	for _, o := range opts {
		o(c)
	}

	return c
}

// Get returns the value for key if present and not expired
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero V
	el, found := c.items[key]
	if !found {
		return zero, false
	}

	e := el.Value.(*entry[V])
	if !e.expires.IsZero() && c.now().After(e.expires) {
		c.remove(el)
		return zero, false
	}

	c.ll.MoveToFront(el)
	return e.value, true
}

// Set adds or replaces the value for key
func (c *Cache[V]) Set(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expires time.Time
	if c.ttl > 0 {
		jitter := time.Duration((rand.Float64()*0.2 - 0.1) * float64(c.ttl)) // #nosec G404 -- jitter does not need a secure random source
		expires = c.now().Add(c.ttl + jitter)
	}

	if el, found := c.items[key]; found {
		e := el.Value.(*entry[V])
		old := e.value
		e.value = v
		e.expires = expires
		c.ll.MoveToFront(el)
		if c.onEvict != nil {
			c.onEvict(key, old)
		}
		return
	}

	c.items[key] = c.ll.PushFront(&entry[V]{key: key, value: v, expires: expires})

	for c.max > 0 && c.ll.Len() > c.max {
		c.remove(c.ll.Back())
	}
}

// Delete removes key from the cache
func (c *Cache[V]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, found := c.items[key]; found {
		c.remove(el)
	}
}

// Len returns the number of entries (incl. expired ones not yet removed)
func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ll.Len()
}

// Purge removes all entries
func (c *Cache[V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for c.ll.Len() > 0 {
		c.remove(c.ll.Back())
	}
}

func (c *Cache[V]) remove(el *list.Element) {
	e := el.Value.(*entry[V])
	c.ll.Remove(el)
	delete(c.items, e.key)

	if c.onEvict != nil {
		c.onEvict(e.key, e.value)
	}
}
