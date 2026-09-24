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

package chunker

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// slowConcat mirrors the PRE-FIX extendRawJSONArray (unmarshal + marshal per
// step) so the fast-path rewrite can be locked to byte-identical output. It is
// the reference the production function must stay equal to.
func slowConcat(a, b json.RawMessage) json.RawMessage {
	if len(a) == 0 {
		return append(json.RawMessage(nil), b...)
	}
	if len(b) == 0 {
		return append(json.RawMessage(nil), a...)
	}
	var arrA, arrB []json.RawMessage
	if err := json.Unmarshal(a, &arrA); err != nil {
		return b
	}
	if err := json.Unmarshal(b, &arrB); err != nil {
		return a
	}
	arrA = append(arrA, arrB...)
	out, err := json.Marshal(arrA)
	if err != nil {
		return a
	}
	return out
}

// TestExtendRawJSONArray_BehaviorIdenticalToSlow locks the O(n^2)->O(n) rewrite
// of extendRawJSONArray to byte-identical output versus the old unmarshal/marshal
// path across nil/empty/nested-array inputs. This is what keeps the merged
// chunk's extended _pdf_positions / positions byte-for-byte identical to before
// (parity-safe).
func TestExtendRawJSONArray_BehaviorIdenticalToSlow(t *testing.T) {
	cases := []struct{ a, b string }{
		{"", "[[1,2]]"},
		{"[[1,2]]", ""},
		{"[]", "[[1,2]]"},
		{"[[1,2]]", "[]"},
		{"[[1,2]]", "[[3,4]]"},
		{"[[1,2],[3,4]]", "[[5,6]]"},
		{"[[[1]]]", "[[[2]]]"},
		{"[[1,2]]", "[[3,4],[5,6]]"},
		{"[[1,2,3]]", "[[4,5,6],[7,8,9]]"},
	}
	for _, c := range cases {
		// Compute the mutation-free reference FIRST: extendRawJSONArray's
		// fast path grows its accumulator `a` in place (append contract), so
		// `a` must not be reused after the call. slowConcat never mutates its
		// inputs, so evaluating it before fast keeps the comparison valid.
		a := json.RawMessage(c.a)
		b := json.RawMessage(c.b)
		slow := slowConcat(a, b)
		fast := extendRawJSONArray(a, b)
		if string(fast) != string(slow) {
			t.Errorf("extendRawJSONArray(%q,%q): fast=%q slow=%q", c.a, c.b, fast, slow)
		}
	}
}

// TestExtendRawJSONArray_NonArrayFallsBackToSlow locks the defensive fallback:
// when an operand is not a compact JSON array, the function must still behave
// exactly like the old path (which tolerated malformed payloads).
func TestExtendRawJSONArray_NonArrayFallsBackToSlow(t *testing.T) {
	cases := []struct{ a, b string }{
		{"[1,2", "[[3,4]]"},
		{"[[1,2]]", "3,4]"},
		{"notjson", "[[3,4]]"},
	}
	for _, c := range cases {
		// Compute the mutation-free reference FIRST (see
		// TestExtendRawJSONArray_BehaviorIdenticalToSlow for why).
		a := json.RawMessage(c.a)
		b := json.RawMessage(c.b)
		slow := slowConcat(a, b)
		fast := extendRawJSONArray(a, b)
		if string(fast) != string(slow) {
			t.Errorf("non-array fallback mismatch (%q,%q): fast=%q slow=%q", c.a, c.b, fast, slow)
		}
	}
}

// benchMergeUnitsJSON builds n text units each carrying a NON-TRIVIAL
// _pdf_positions array (~20 boxes, ~0.5 KB) and merges them with the real
// JSON-path default chunk budget (chunkTokens=512). That budget caps each merge
// run at ~100 units, so the only quadratic cost left in the hot path is the
// per-step coordinate-list extension in extendRawJSONArray — which the fast
// path turns from O(n^2) (unmarshal+marshal the whole growing array every step)
// into O(n). The text path (prev.Text concat + re-tokenize guard) is bounded by
// chunkTokens and is identical before/after the fix, so it does not mask the
// improvement.
func benchMergeUnitsJSON(b *testing.B, n int) {
	pos := make([]byte, 0, 512)
	pos = append(pos, '[')
	for i := 0; i < 20; i++ {
		if i > 0 {
			pos = append(pos, ',')
		}
		pos = append(pos, fmt.Sprintf("[%d,0,%d,0,%d]", i, i*10, i*5)...)
	}
	pos = append(pos, ']')
	units := make([]schema.ChunkDoc, n)
	for i := 0; i < n; i++ {
		units[i] = schema.ChunkDoc{
			Text:         strings.Repeat("word ", 5),
			CKType:       "text",
			TKNums:       intPtr(5),
			PDFPositions: append(json.RawMessage(nil), pos...),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mergeByTokenSizeFromJSON([][]schema.ChunkDoc{units}, 512, 0)
	}
}

func BenchmarkMergeUnitsJSON_1k(b *testing.B)  { benchMergeUnitsJSON(b, 1000) }
func BenchmarkMergeUnitsJSON_3k(b *testing.B)  { benchMergeUnitsJSON(b, 3000) }
func BenchmarkMergeUnitsJSON_10k(b *testing.B) { benchMergeUnitsJSON(b, 10000) }

// benchExtendRawJSONArrayChain isolates extendRawJSONArray from the rest of the
// merge path: it chains N extensions of a ~0.5 KB coordinate array. Before the
// fix each step unmarshal+marshal'd the entire growing array (O(n^2) bytes +
// reflection); the fast path appends into the buffer (O(n)). It is the direct,
// noise-free proof of the fix.
func benchExtendRawJSONArrayChain(b *testing.B, n int) {
	pos := make([]byte, 0, 512)
	pos = append(pos, '[')
	for i := 0; i < 20; i++ {
		if i > 0 {
			pos = append(pos, ',')
		}
		pos = append(pos, fmt.Sprintf("[%d,0,%d,0,%d]", i, i*10, i*5)...)
	}
	pos = append(pos, ']')
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var acc json.RawMessage
		for j := 0; j < n; j++ {
			acc = extendRawJSONArray(acc, pos)
		}
	}
}

func BenchmarkExtendRawJSONArrayChain_1k(b *testing.B)  { benchExtendRawJSONArrayChain(b, 1000) }
func BenchmarkExtendRawJSONArrayChain_10k(b *testing.B) { benchExtendRawJSONArrayChain(b, 10000) }
