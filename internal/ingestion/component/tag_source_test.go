//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import "testing"

func TestBoundedTagCacheEvictsOldest(t *testing.T) {
	c := newBoundedTagCache(2)
	a, b, d := &MemoryTagIndex{}, &MemoryTagIndex{}, &MemoryTagIndex{}
	c.store("a", a)
	c.store("b", b)
	c.store("c", d) // pushes "a" out

	if _, ok := c.load("a"); ok {
		t.Fatal("the oldest entry should have been evicted")
	}
	for key, want := range map[string]*MemoryTagIndex{"b": b, "c": d} {
		got, ok := c.load(key)
		if !ok || got != want {
			t.Errorf("load(%q) = %v, %v; want %v, true", key, got, ok, want)
		}
	}
	if len(c.items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(c.items))
	}
}

// A non-positive cap used to run the eviction loop once per insert, dropping
// the entry that had just been stored; it must simply retain nothing.
func TestBoundedTagCacheZeroCapRetainsNothing(t *testing.T) {
	c := newBoundedTagCache(0)
	idx := &MemoryTagIndex{}
	if got := c.store("a", idx); got != idx {
		t.Fatalf("store must still return the caller's index, got %v", got)
	}
	if _, ok := c.load("a"); ok {
		t.Fatal("cap <= 0 must disable caching")
	}
	if len(c.items) != 0 || len(c.recent) != 0 {
		t.Errorf("items=%d recent=%d, want 0/0", len(c.items), len(c.recent))
	}
}

// load() reports a stored key as a hit, so caching a nil index would hand
// callers a nil *MemoryTagIndex that looks valid.
func TestBoundedTagCacheSkipsNilIndex(t *testing.T) {
	c := newBoundedTagCache(2)
	c.store("a", nil)
	if _, ok := c.load("a"); ok {
		t.Fatal("a nil index must not be cached")
	}
	if len(c.items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(c.items))
	}
}

// The eviction loop indexes recent[0], so it panics when items and recent
// drift apart (items holds keys recent does not track). It must stay bounded
// and terminate instead, and leave the cache usable afterwards - a stale entry
// that recent cannot name would otherwise wedge the only slot forever.
func TestBoundedTagCacheEvictionSurvivesRecentDrift(t *testing.T) {
	c := newBoundedTagCache(1)
	c.items["stale-a"] = &MemoryTagIndex{}
	c.items["stale-b"] = &MemoryTagIndex{}
	c.recent = nil

	c.store("fresh", &MemoryTagIndex{})
	if len(c.items) > c.cap {
		t.Fatalf("cache exceeded its cap: %d > %d", len(c.items), c.cap)
	}

	idx := &MemoryTagIndex{}
	c.store("next", idx)
	if got, ok := c.load("next"); !ok || got != idx {
		t.Fatalf("load(%q) = %v, %v; want %v, true", "next", got, ok, idx)
	}
	if len(c.items) > c.cap {
		t.Fatalf("cache exceeded its cap: %d > %d", len(c.items), c.cap)
	}
}
