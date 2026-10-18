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

package tokenizer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// A full memo has to rotate, not freeze. Freezing lets the first
// termLookupCacheMax distinct terms occupy the budget permanently, so a term
// that only became hot later misses on every lookup with no way back; rotating
// keeps the memo representing recent traffic.
//
// This test drives admitTerm directly with a small limit so it needs no
// dictionary, and therefore runs without the "manual" build tag.
func TestAdmitTermRotatesWhenFull(t *testing.T) {
	const limit = 4
	var (
		cache   sync.Map
		counter atomic.Int64
	)

	for i, term := range []string{"a", "b", "c", "d"} {
		admitTerm(&cache, &counter, limit, term, int32(i))
	}
	if got := counter.Load(); got != limit {
		t.Fatalf("counter = %d after filling the memo, want %d", got, limit)
	}

	// The term that triggers the rotation was just looked up, so it has to end
	// up in the new generation rather than being recomputed.
	admitTerm(&cache, &counter, limit, "e", int32(9))
	got, ok := cache.Load("e")
	if !ok || got.(int32) != 9 {
		t.Fatalf("term admitted at the cap = %v, present = %v; want 9, true", got, ok)
	}
	if n := counter.Load(); n != 1 {
		t.Fatalf("counter = %d after rotation, want 1", n)
	}
	if _, ok := cache.Load("a"); ok {
		t.Error("rotation kept an entry from the previous generation")
	}

	// Capacity must be usable again: the whole point is that terms arriving
	// after the cap was reached still get memoized.
	for _, term := range []string{"f", "g", "h"} {
		admitTerm(&cache, &counter, limit, term, int32(0))
	}
	if n := counter.Load(); n != limit {
		t.Fatalf("counter = %d after refilling, want %d", n, limit)
	}
	for _, term := range []string{"e", "f", "g", "h"} {
		if _, ok := cache.Load(term); !ok {
			t.Errorf("%q is missing after refill", term)
		}
	}
}

// A term that arrived before the rotation must be re-admitted afterwards: that
// is what self-healing means. If this regressed to "insert only until full",
// "old" would stay out forever.
func TestAdmitTermReadmitsAfterRotation(t *testing.T) {
	const limit = 2
	var (
		cache   sync.Map
		counter atomic.Int64
	)

	admitTerm(&cache, &counter, limit, "old", int32(1))
	admitTerm(&cache, &counter, limit, "filler", int32(2))

	// Fill past the cap so the generation containing "old" is dropped.
	admitTerm(&cache, &counter, limit, "trigger", int32(3))
	if _, ok := cache.Load("old"); ok {
		t.Fatal("expected the previous generation to be gone")
	}

	// "old" comes back: the rotated memo must accept it again.
	admitTerm(&cache, &counter, limit, "old", int32(1))
	if v, ok := cache.Load("old"); !ok || v.(int32) != 1 {
		t.Fatalf("re-admitted term = %v, present = %v; want 1, true", v, ok)
	}
}

// Rotation clears the memo and resets the counter on the same path that
// concurrent inserts use, so exercise it under contention. The value of this
// test is being run with -race; the bound below also pins the documented
// "limit plus writers in flight" overshoot.
func TestAdmitTermIsRaceFreeUnderContention(t *testing.T) {
	const (
		limit     = 4
		workers   = 8
		perWorker = 64
	)

	var (
		cache   sync.Map
		counter atomic.Int64
		wg      sync.WaitGroup
	)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				admitTerm(&cache, &counter, limit, fmt.Sprintf("w%d-%d", w, i), int32(i))
			}
		}(w)
	}
	wg.Wait()

	entries := 0
	cache.Range(func(_, _ any) bool {
		entries++
		return true
	})
	if entries == 0 {
		t.Fatal("memo is empty after concurrent admission")
	}
	if entries > limit+workers {
		t.Fatalf("memo holds %d entries, want at most limit+workers = %d", entries, limit+workers)
	}
}
