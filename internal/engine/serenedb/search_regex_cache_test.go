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

package serenedb

import (
	"fmt"
	"reflect"
	"testing"
)

func resetHighlightRegexCache(t *testing.T) {
	t.Helper()
	highlightRegexCache.Clear()
	highlightRegexCount.Store(0)
	t.Cleanup(func() {
		highlightRegexCache.Clear()
		highlightRegexCount.Store(0)
	})
}

func highlightFixture() []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "c1", "content": "hello world and hello again"},
		{"id": "c2", "content": "nothing relevant"},
	}
}

// TestCachedHighlightRegexIsReused is a deterministic guard against per-call
// compilation: the same keyword returns the same compiled pattern, while a
// different keyword gets its own entry.
func TestCachedHighlightRegexIsReused(t *testing.T) {
	first := cachedHighlightRegex("hello")
	again := cachedHighlightRegex("hello")
	other := cachedHighlightRegex("world")
	if first != again {
		t.Error("highlight regex was rebuilt across calls")
	}
	if first == other {
		t.Error("highlight regex cache did not separate distinct keywords")
	}
}

// TestHighlightRegexCacheIsBounded is the adversarial memory case: unique
// query tokens must not grow the memo without limit. The baseline cache kept
// every key forever, so this fails before a rotation policy exists.
func TestHighlightRegexCacheIsBounded(t *testing.T) {
	resetHighlightRegexCache(t)

	const uniqueKeywords = 5000
	const maxEntries = 4096
	for i := range uniqueKeywords {
		cachedHighlightRegex(fmt.Sprintf("term%06d", i))
	}

	entries := 0
	highlightRegexCache.Range(func(_, _ any) bool {
		entries++
		return true
	})
	if entries > maxEntries {
		t.Fatalf("highlight cache entries = %d, want <= %d", entries, maxEntries)
	}
}

// TestHighlightRegexCacheReusesRepeatedKeywords pins that bounding the memo
// does not destroy reuse of hot keywords.
func TestHighlightRegexCacheReusesRepeatedKeywords(t *testing.T) {
	resetHighlightRegexCache(t)

	first := cachedHighlightRegex("hot-term")
	for range 3 {
		if got := cachedHighlightRegex("hot-term"); got != first {
			t.Fatal("repeated keyword did not reuse the cached regex")
		}
	}
}

// TestGetHighlightStableAcrossCalls pins byte-for-byte output stability with
// cached patterns.
func TestGetHighlightStableAcrossCalls(t *testing.T) {
	e := &serenedbEngine{}
	chunks := highlightFixture()
	keywords := []string{"hello", "world"}
	want := e.GetHighlight(chunks, keywords, "content")
	for range 5 {
		if got := e.GetHighlight(chunks, keywords, "content"); !reflect.DeepEqual(got, want) {
			t.Fatalf("highlight changed across calls: %v vs %v", got, want)
		}
	}
}

// BenchmarkGetHighlightCached guards the per-result highlight path against
// reintroducing per-call regex compilation.
func BenchmarkGetHighlightCached(b *testing.B) {
	e := &serenedbEngine{}
	chunks := highlightFixture()
	keywords := []string{"hello", "world"}
	e.GetHighlight(chunks, keywords, "content")
	b.ResetTimer()
	for range b.N {
		e.GetHighlight(chunks, keywords, "content")
	}
}

// TestGetHighlightCacheKeepsOutput pins byte-for-byte equivalence of the
// cached highlighting with the original per-call compilation.
func TestGetHighlightCacheKeepsOutput(t *testing.T) {
	e := &serenedbEngine{}
	got := e.GetHighlight(highlightFixture(), []string{"hello", "world"}, "content")
	want := map[string]string{
		"c1": "<em>hello world</em> and <em>hello</em> again",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight = %v, want %v", got, want)
	}
}

// TestGetHighlightCacheKeepsWordBoundary is the adversarial case: a keyword
// that is a substring of a longer word must still be left alone.
func TestGetHighlightCacheKeepsWordBoundary(t *testing.T) {
	e := &serenedbEngine{}
	chunks := []map[string]interface{}{
		{"id": "c1", "content": "helloworld hello worlds hello"},
	}
	want := map[string]string{"c1": "helloworld <em>hello</em> worlds <em>hello</em>"}
	for range 3 {
		if got := e.GetHighlight(chunks, []string{"hello"}, "content"); !reflect.DeepEqual(got, want) {
			t.Fatalf("GetHighlight = %v, want %v", got, want)
		}
	}
}

// TestGetHighlightCacheSkipsEmptyKeywords pins that blank keywords never
// reach the pattern cache and still contribute no highlight.
func TestGetHighlightCacheSkipsEmptyKeywords(t *testing.T) {
	e := &serenedbEngine{}
	got := e.GetHighlight(highlightFixture(), []string{"", "  ", "hello"}, "content")
	want := map[string]string{"c1": "<em>hello</em> world and <em>hello</em> again"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight = %v, want %v", got, want)
	}
}
