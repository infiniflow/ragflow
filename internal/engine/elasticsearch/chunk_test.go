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

package elasticsearch

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"ragflow/internal/common"
)

// makeResponse builds a SearchResponse with `n` synthetic hits whose id
// is the index in the batch ("h-0", "h-1", ...) and whose sort cursor is
// ["h-0", "h-1", ...] so each iteration's cursor advances deterministically.
func makeResponse(n int, startID int, total int64) SearchResponse {
	resp := SearchResponse{}
	resp.Hits.Total.Value = total
	if n == 0 {
		return resp
	}
	hits := make([]struct {
		ID        string                 `json:"_id"`
		Index     string                 `json:"_index"`
		Score     float64                `json:"_score"`
		Source    map[string]interface{} `json:"_source"`
		Fields    map[string]interface{} `json:"fields"`
		Highlight map[string]interface{} `json:"highlight,omitempty"`
		Sort      []interface{}          `json:"sort,omitempty"`
	}, n)
	for i := 0; i < n; i++ {
		id := startID + i
		hits[i].ID = "h-" + itoa(id)
		hits[i].Source = map[string]interface{}{"id": id}
		hits[i].Sort = []interface{}{"h-" + itoa(id)}
	}
	resp.Hits.Hits = hits
	return resp
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// mockFetcher returns searchAfterFetcher implementations that draw
// from a pre-loaded sequence of (batch, hits) responses and record
// every call so tests can assert against them.
//
// The mock honours the `batch` argument the way a real ES client
// would: it returns at most `min(batch, scriptedHitsRemaining)` hits
// per call. This makes the loop's "did we ask for more than ES gave
// us?" branch observable.
type mockFetcher struct {
	// scripted: each entry is the FULL response the fetcher would
	// return for one request. The hits inside it are pre-truncated to
	// the size the test wants the fetcher to deliver.
	scripted      []SearchResponse
	scriptedTotal int64
	idx           int
	// calls records every (batch, cursor, trackTotalHits) tuple the
	// pagination loop sent.
	calls []mockCall
}

type mockCall struct {
	batch          int
	cursor         []interface{}
	trackTotalHits bool
}

func (m *mockFetcher) fetch(_ context.Context, _ map[string]interface{}, batch int, cursor []interface{}, trackTotalHits bool) (SearchResponse, error) {
	m.calls = append(m.calls, mockCall{batch: batch, cursor: cursor, trackTotalHits: trackTotalHits})
	if m.idx >= len(m.scripted) {
		return SearchResponse{}, nil
	}
	resp := m.scripted[m.idx]
	m.idx++
	// Honour `batch` like a real ES: trim the hit list to the
	// requested size so the loop's "short batch" branch is reachable.
	if batch > 0 && len(resp.Hits.Hits) > batch {
		resp.Hits.Hits = resp.Hits.Hits[:batch]
	}
	// Only the first request is asked to track the exact total. The
	// scripted response may already carry a total; the fetcher fills
	// in scriptedTotal only when the response's total is zero AND the
	// caller asked for the exact count.
	if trackTotalHits && resp.Hits.Total.Value == 0 && m.scriptedTotal > 0 {
		resp.Hits.Total.Value = m.scriptedTotal
	}
	return resp, nil
}

// TestSortValuesEqual pins down the cursor-equality helper. The
// pagination loop uses it to detect "ES didn't advance" — when the
// cursor between two consecutive responses is unchanged, the index is
// exhausted and the loop must stop. False negatives here would loop
// forever; false positives would terminate early and miss data.
func TestSortValuesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []interface{}
		want bool
	}{
		{name: "both_nil", a: nil, b: nil, want: true},
		{name: "first_nil", a: nil, b: []interface{}{"x"}, want: false},
		{name: "second_nil", a: []interface{}{"x"}, b: nil, want: false},
		{name: "equal_strings", a: []interface{}{"a", "b"}, b: []interface{}{"a", "b"}, want: true},
		{name: "different_strings", a: []interface{}{"a"}, b: []interface{}{"b"}, want: false},
		{name: "different_lengths", a: []interface{}{"a"}, b: []interface{}{"a", "b"}, want: false},
		{name: "mixed_types_equal", a: []interface{}{"x", float64(1)}, b: []interface{}{"x", float64(1)}, want: true},
		{name: "mixed_types_differ", a: []interface{}{"x", float64(1)}, b: []interface{}{"x", float64(2)}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sortValuesEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("sortValuesEqual(%#v, %#v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestSearchAfterPaginateSimpleFirstPage exercises the trivial case:
// offset=0, limit=N, N hits available in one response. The loop must
// send one request, return the hits, and report the total.
func TestSearchAfterPaginateSimpleFirstPage(t *testing.T) {
	m := &mockFetcher{
		scripted:      []SearchResponse{makeResponse(5, 0, 5)},
		scriptedTotal: 5,
	}
	got, total, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, 0, 5, m.fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(got) != 5 {
		t.Fatalf("len(hits) = %d, want 5", len(got))
	}
	if m.idx != 1 {
		t.Errorf("expected 1 fetch call, got %d", m.idx)
	}
}

// TestSearchAfterPaginateSkipsDeepOffset covers the regression the
// `useSearchAfter` bug report flagged: offset=10_500 (past the
// MAX_RESULT_WINDOW of 10_000) with limit=10 must NOT return the
// first page — it must walk the result set and return the right slice.
//
// We simulate 10,510 total hits, asking for offset=10_500 + limit=10.
// The mock returns full 1000-hit batches with advancing cursors; the
// loop should issue 11 batches (10 to skip + 1 to take) and return
// the last 10 hits (h-10500..h-10509).
func TestSearchAfterPaginateSkipsDeepOffset(t *testing.T) {
	const total = 10510
	const offset = 10500
	const limit = 10

	// 10 full skip batches: each 1000 hits, cursor advances.
	// The 11th batch has 510 hits — the skip phase asks for 500
	// (the remaining 500 to skip), the take phase picks up the
	// leftover 10. So 11 fetches total cover both phases.
	m := &mockFetcher{scriptedTotal: total}
	for i := 0; i < 10; i++ {
		m.scripted = append(m.scripted, makeResponse(common.SearchAfterBatchSize, i*common.SearchAfterBatchSize, 0))
	}
	// Partial batch: 510 hits (10*1000..10*1000+509). Skip uses
	// the first 500, take uses the last 10.
	m.scripted = append(m.scripted, makeResponse(510, 10*common.SearchAfterBatchSize, 0))
	// Defensive: if the loop miscounts and asks for another take
	// batch, this would surface as a 12th fetch.
	m.scripted = append(m.scripted, makeResponse(10, 10500, 0))

	got, totalHits, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, offset, limit, m.fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalHits != total {
		t.Errorf("total = %d, want %d", totalHits, total)
	}
	if len(got) != limit {
		t.Fatalf("len(hits) = %d, want %d", len(got), limit)
	}
	// The first returned hit must be h-10500, not h-0 — that is the
	// entire point of the search_after path.
	if id, _ := got[0]["id"].(int); id != offset {
		t.Errorf("first hit id = %d, want %d (search_after must skip the deep offset)", id, offset)
	}
	if id, _ := got[limit-1]["id"].(int); id != offset+limit-1 {
		t.Errorf("last hit id = %d, want %d", id, offset+limit-1)
	}
	// 12 fetches: 10 full skip + 1 partial-skip (trims scripted 510
	// down to 500) + 1 partial-take (the 10 leftover hits the take
	// phase still needs). The 12th batch in scripted is a defensive
	// sentinel; if the loop miscounts, that fetch would not be hit.
	if m.idx != 12 {
		t.Errorf("expected 12 fetches, got %d", m.idx)
	}
}

// TestSearchAfterPaginateExhaustsIndex: when the skip phase reaches
// the end of the index, the loop must stop, not loop forever waiting
// for non-empty hits. We simulate a small index that runs out mid-skip.
func TestSearchAfterPaginateExhaustsIndex(t *testing.T) {
	// 500 total hits, offset=400, limit=10.
	// Skip phase: one batch of 500 (the whole index). remainingSkip
	// becomes 400-500 = -100, loop ends.
	m := &mockFetcher{
		scripted: []SearchResponse{
			makeResponse(500, 0, 500),
			// We don't expect a second call; if we get one the
			// loop failed to terminate. Return empty to make
			// that visible.
		},
		scriptedTotal: 500,
	}
	got, total, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, 400, 10, m.fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 500 {
		t.Errorf("total = %d, want 500", total)
	}
	// After skipping all 500 hits, the take phase should issue one
	// fetch and find 0 hits, returning empty. We check call count
	// (not m.idx) because the empty response path in the mock
	// fetcher returns early without incrementing idx.
	if len(got) != 0 {
		t.Errorf("len(hits) = %d, want 0 (index exhausted past offset)", len(got))
	}
	if len(m.calls) != 2 {
		t.Errorf("expected 2 fetches (skip+take-empty), got %d", len(m.calls))
	}
}

// TestSearchAfterPaginateEmptyResult: zero total hits means the very
// first response is empty; loop must not loop forever and must
// return total=0.
func TestSearchAfterPaginateEmptyResult(t *testing.T) {
	m := &mockFetcher{
		scripted:      []SearchResponse{makeResponse(0, 0, 0)},
		scriptedTotal: 0,
	}
	got, total, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, 50, 10, m.fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(got) != 0 {
		t.Errorf("len(hits) = %d, want 0", len(got))
	}
	if m.idx != 1 {
		t.Errorf("expected 1 fetch, got %d", m.idx)
	}
}

// TestSearchAfterPaginateStopOnUnchangedCursor: the loop must detect
// the "ES didn't advance" signal (next sort cursor == previous) and
// stop, rather than spinning. This is a defensive break in case
// search_after returns identical hits on consecutive requests.
func TestSearchAfterPaginateStopOnUnchangedCursor(t *testing.T) {
	// First response advances the cursor; second response is the
	// same cursor — loop should stop, NOT call a third time.
	resp1 := makeResponse(1000, 0, 5000)
	resp1.Hits.Hits[999].Sort = []interface{}{"cursor-1"}
	resp2 := makeResponse(1000, 1000, 0)
	resp2.Hits.Hits[999].Sort = []interface{}{"cursor-1"} // same as resp1's last
	m := &mockFetcher{
		scripted:      []SearchResponse{resp1, resp2},
		scriptedTotal: 5000,
	}
	// offset=500, limit=10.
	_, _, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, 500, 10, m.fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 fetches: 1 skip, 1 take that hit the unchanged-cursor
	// termination. A 3rd fetch would mean we failed to stop.
	if m.idx != 2 {
		t.Errorf("expected 2 fetches (loop must stop on unchanged cursor), got %d", m.idx)
	}
}

// TestSearchAfterPaginateLimitLargerThanBatchSize: when limit exceeds
// SearchAfterBatchSize, the take phase must issue multiple iterations.
func TestSearchAfterPaginateLimitLargerThanBatchSize(t *testing.T) {
	const limit = 2500 // > 2 * common.SearchAfterBatchSize
	m := &mockFetcher{
		scripted: []SearchResponse{
			// Skip phase empty (offset=0).
			makeResponse(1000, 0, 10000),
			makeResponse(1000, 1000, 0),
			makeResponse(1000, 2000, 0),
		},
		scriptedTotal: 10000,
	}
	got, total, err := searchAfterPaginate(context.Background(), map[string]interface{}{}, 0, limit, m.fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 10000 {
		t.Errorf("total = %d, want 10000", total)
	}
	if len(got) != limit {
		t.Errorf("len(hits) = %d, want %d", len(got), limit)
	}
	// offset=0 means the skip loop doesn't run; the take loop
	// issues 3 fetches (1000 + 1000 + 500 = 2500). The third fetch
	// is "short" (500 < 1000), which the loop uses to stop early
	// without over-collecting.
	if m.idx != 3 {
		t.Errorf("expected 3 take fetches, got %d", m.idx)
	}
	// Hits should be in order h-0..h-2499.
	wantIDs := make([]int, limit)
	for i := range wantIDs {
		wantIDs[i] = i
	}
	gotIDs := make([]int, len(got))
	for i, h := range got {
		gotIDs[i] = h["id"].(int)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("hit order wrong: got %v, want %v", sortedCopy(gotIDs), wantIDs)
	}
}

func sortedCopy(in []int) []int {
	out := append([]int(nil), in...)
	sort.Ints(out)
	return out
}

// TestBuildBoolQueryFromConditionIDFilter is the regression for the
// "id filter encoded as a nested array inside bool.should" bug.
//
// Each branch of the `k == "id"` clause used to append a
// `[]map[string]interface{}` literal as a single element, so the emitted
// JSON was `should: [[{...}, {...}]]` (a nested array) instead of
// `should: [{...}, {...}]`. ES rejects the nested form as malformed.
// This test pins down the flat shape for the list, string, and int
// branches.
func TestBuildBoolQueryFromConditionIDFilter(t *testing.T) {
	check := func(name string, cond map[string]interface{}, wantFields []string) {
		t.Helper()
		got := buildBoolQueryFromCondition(cond, nil, false, false)
		outer, ok := got["bool"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: missing bool wrapper: %v", name, got)
		}
		should, ok := outer["should"].([]interface{})
		if !ok {
			t.Fatalf("%s: should is not []interface{}: %T", name, outer["should"])
		}
		if len(should) != len(wantFields) {
			t.Fatalf("%s: should length = %d, want %d (raw=%v)", name, len(should), len(wantFields), should)
		}
		// Every element must be a map (not another slice). A nested slice
		// would be the bug we're guarding against.
		seenFields := make(map[string]bool, len(wantFields))
		for i, el := range should {
			m, ok := el.(map[string]interface{})
			if !ok {
				t.Fatalf("%s: should[%d] is %T, want map[string]interface{} (nested array bug) — raw=%v", name, i, el, el)
			}
			if inner, ok := m["term"].(map[string]interface{}); ok {
				for f := range inner {
					seenFields[f] = true
				}
				continue
			}
			if terms, ok := m["terms"].(map[string]interface{}); ok {
				for f := range terms {
					seenFields[f] = true
				}
				continue
			}
			t.Fatalf("%s: should[%d] missing term/terms: %v", name, i, m)
		}
		for _, want := range wantFields {
			if !seenFields[want] {
				t.Errorf("%s: expected field %q in should clauses, got %v", name, want, seenFields)
			}
		}
	}

	check("list_value", map[string]interface{}{
		"id": []interface{}{"a", "b", "c"},
	}, []string{"id", "_id"})

	check("string_value", map[string]interface{}{
		"id": "doc-42",
	}, []string{"id", "_id"})

	check("int_value", map[string]interface{}{
		"id": 42,
	}, []string{"id", "_id"})
}

// paginationGRID mirrors the (page_size, top) grid from
// rag/nlp/search.py::Dealer._rerank_window tests. It covers the common page
// sizes that do NOT divide 64 (the exact case the legacy min(..., 64) clamp
// broke) plus tiny / large / page-aligned tops.
var paginationGRID = func() []struct{ size, topK int } {
	sizes := []int{1, 5, 7, 10, 30, 50, 64}
	tops := []int{0, 5, 30, 50, 55, 64, 100, 1024}
	out := make([]struct{ size, topK int }, 0, len(sizes)*len(tops))
	for _, s := range sizes {
		for _, t := range tops {
			out = append(out, struct{ size, topK int }{s, t})
		}
	}
	return out
}()

// paginate replays the (block-fetch + in-block slice) math that
// calculatePagination's window is consumed by: for every page whose start is
// inside the candidate pool, return the in-block page slice. The block is
// window-aligned, so on the aligned invariant every page is full and the
// concatenation reconstructs [0, cap).
func paginate(total, size, topK int) (window, capN int, surfaced []int) {
	window = rerankWindow(size, topK)
	capN = total
	if topK > 0 && capN > topK {
		capN = topK
	}
	for page := 1; (page-1)*size < capN; page++ {
		globalOffset := (page - 1) * size
		blockIndex := globalOffset / window
		blockStart := blockIndex * window
		block := make([]int, 0, window)
		for i := blockStart; i < blockStart+window && i < capN; i++ {
			block = append(block, i)
		}
		begin := globalOffset % window
		end := begin + size
		if end > len(block) {
			end = len(block)
		}
		surfaced = append(surfaced, block[begin:end]...)
	}
	return window, capN, surfaced
}

func TestRerankWindowIsPageAligned(t *testing.T) {
	for _, g := range paginationGRID {
		window := rerankWindow(g.size, g.topK)
		if window < 1 {
			t.Errorf("rerankWindow(%d, %d) = %d, want >= 1", g.size, g.topK, window)
		}
		if g.size > 1 && window%g.size != 0 {
			t.Errorf("rerankWindow(%d, %d) = %d, want multiple of %d", g.size, g.topK, window, g.size)
		}
	}
}

func TestRerankWindowPaginationReconstructsPool(t *testing.T) {
	// Walking every page reconstructs the candidate pool exactly: in order,
	// no gaps, no duplicates, and no short interior pages.
	const total = 250
	for _, g := range paginationGRID {
		window, capN, surfaced := paginate(total, g.size, g.topK)
		if len(surfaced) != capN {
			t.Errorf("size=%d topK=%d: surfaced %d, want %d (window=%d)",
				g.size, g.topK, len(surfaced), capN, window)
			continue
		}
		for i, v := range surfaced {
			if v != i {
				t.Errorf("size=%d topK=%d: surfaced[%d] = %d, want %d (window=%d)",
					g.size, g.topK, i, v, i, window)
				break
			}
		}
	}
}

func TestCalculatePaginationReportedRegression(t *testing.T) {
	// The reported case: size=10, topK=1024. Legacy min(..., 64) clamped the
	// window to 64 (not a multiple of 10), so page 7 (global offset 60) used
	// to return only 4 of 10 results. With the fix, the window is 70 and
	// page 7 is full and contiguous.
	_, limit := calculatePagination(7, 10, 1024)
	if limit != 70 {
		t.Fatalf("calculatePagination(7, 10, 1024) limit = %d, want 70", limit)
	}
	if limit%10 != 0 {
		t.Fatalf("calculatePagination(7, 10, 1024) limit = %d, want multiple of 10", limit)
	}

	// And the simulated end-to-end page walk covers positions 60..69 fully.
	_, capN, surfaced := paginate(250, 10, 1024)
	if capN < 70 || len(surfaced) < 70 {
		t.Fatalf("paginate(250, 10, 1024) returned cap=%d surfaced=%d, want >= 70", capN, len(surfaced))
	}
	for i := 60; i < 70; i++ {
		if surfaced[i] != i {
			t.Errorf("page 7: surfaced[%d] = %d, want %d", i, surfaced[i], i)
		}
	}
}
