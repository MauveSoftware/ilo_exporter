// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package lru

import (
	"testing"
	"time"
)

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	var evicted []string
	c := New(2, WithEvictCallback(func(key string, _ int) { evicted = append(evicted, key) }))

	c.Set("a", 1)
	c.Set("b", 2)
	c.Get("a")
	c.Set("c", 3)

	if _, found := c.Get("b"); found {
		t.Error("expected b to be evicted")
	}
	if v, found := c.Get("a"); !found || v != 1 {
		t.Error("expected a to be kept")
	}
	if len(evicted) != 1 || evicted[0] != "b" {
		t.Errorf("expected evict callback for b, got %v", evicted)
	}
	if c.Len() != 2 {
		t.Errorf("expected 2 entries, got %d", c.Len())
	}
}

func TestExpiry(t *testing.T) {
	now := time.Now()
	c := New(10, WithTTL[string](time.Hour))
	c.now = func() time.Time { return now }

	c.Set("a", "x")

	now = now.Add(50 * time.Minute)
	if _, found := c.Get("a"); !found {
		t.Error("expected entry before TTL - jitter")
	}

	now = now.Add(20 * time.Minute)
	if _, found := c.Get("a"); found {
		t.Error("expected entry to be expired after TTL + jitter")
	}
	if c.Len() != 0 {
		t.Error("expected expired entry to be removed")
	}
}

func TestDeleteAndPurge(t *testing.T) {
	var evicted int
	c := New(10, WithEvictCallback(func(string, int) { evicted++ }))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	c.Delete("a")
	c.Purge()

	if c.Len() != 0 || evicted != 3 {
		t.Errorf("expected empty cache and 3 evictions, got %d entries and %d evictions", c.Len(), evicted)
	}
}
