// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunker

import (
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// mergeUnitsOracleRow is one row of the oracle output: the merged chunk text
// plus the token count mergeUnits must assign to it.
type mergeUnitsOracleRow struct {
	text string
	tk   int
}

// pythonMergeUnitsOracle is the Go port of the UNIFIED merge contract shared by
// the Go TokenChunker, Python naive_merge, and Python token_chunker's JSON
// path. It is the oracle that mergeUnits (the single, unified merge core) must
// match chunk-for-chunk (text AND count).
//
// Contract mirrored here (the unified "hybrid" algorithm):
//   - token counts are the RUNNING SUM of per-unit counts (never re-tokenizing
//     the joined text);
//   - overlap>0 uses a SCALED threshold target*(100-overlap)/100 and prepends
//     the previous chunk's tail UNCONDITIONALLY (no fit-check);
//   - a non-text unit passes through and resets the merge run;
//   - an over-budget unit (tk > target) STANDS ALONE — never merged into the
//     previous chunk (retains the #17799 contract; diverges from Python JSON's
//     native merge-into-prev on purpose, so the product-wide behaviour change
//     is avoided);
//   - MergeUnderCap additionally forbids a projected overflow.
//
// Token counts: the initial chunk and merged running sum use the EXPLICIT
// tkNums (so merge decisions are deterministic and offline-independent), but
// an overlap-prefixed chunk recomputes its count via tokenizeStr — exactly as
// mergeUnits does — so the two stay in lock-step.
func pythonMergeUnitsOracle(texts []string, tkNums []int, kinds []string, target int, overlap float64, joinSep string, strat schema.MergeStrategy) []mergeUnitsOracleRow {
	threshold := float64(target) * (100.0 - overlap) / 100.0
	merged := []mergeUnitsOracleRow{}
	prev := -1
	for i := range texts {
		if kinds[i] != "text" {
			merged = append(merged, mergeUnitsOracleRow{texts[i], tkNums[i]})
			prev = -1
			continue
		}
		tk := tkNums[i]
		if prev < 0 {
			merged = append(merged, mergeUnitsOracleRow{texts[i], tk})
			prev = len(merged) - 1
			continue
		}
		// #17799: an over-budget unit stands alone — never merged into prev.
		if tk > target {
			text := texts[i]
			cpTk := tk
			if overlap > 0 && merged[prev].text != "" {
				vis := []rune(removeTag(merged[prev].text))
				cut := int(float64(len(vis)) * (100.0 - overlap) / 100.0)
				if cut < 0 {
					cut = 0
				}
				if cut < len(vis) {
					text = string(vis[cut:]) + texts[i]
				}
				cpTk = tokenizeStr(text)
			}
			merged = append(merged, mergeUnitsOracleRow{text, cpTk})
			prev = len(merged) - 1
			continue
		}
		startNew := float64(merged[prev].tk) > threshold
		if !startNew && strat == schema.MergeUnderCap && merged[prev].tk+tk > target {
			startNew = true
		}
		if startNew {
			text := texts[i]
			cpTk := tk
			if overlap > 0 && merged[prev].text != "" {
				vis := []rune(removeTag(merged[prev].text))
				cut := int(float64(len(vis)) * (100.0 - overlap) / 100.0)
				if cut < 0 {
					cut = 0
				}
				if cut < len(vis) {
					text = string(vis[cut:]) + texts[i]
				}
				// Mirror mergeUnits: recompute the overlap chunk's token
				// count from its (prefix+cur) text. Only when a prefix was
				// actually prepended; otherwise keep the explicit count.
				cpTk = tokenizeStr(text)
			}
			merged = append(merged, mergeUnitsOracleRow{text, cpTk})
			prev = len(merged) - 1
			continue
		}
		if merged[prev].text != "" && texts[i] != "" {
			merged[prev].text = merged[prev].text + joinSep + texts[i]
		} else {
			merged[prev].text = merged[prev].text + texts[i]
		}
		merged[prev].tk += tk
	}
	return merged
}

func TestMergeUnitsMatchesPythonOracle(t *testing.T) {
	cases := []struct {
		name    string
		texts   []string
		tkNums  []int
		kinds   []string
		target  int
		overlap float64
		joinSep string
		strat   schema.MergeStrategy
	}{
		{
			name:   "json overlap0",
			texts:  []string{"a", "b", "c", "d"},
			tkNums: []int{5, 5, 5, 5},
			kinds:  []string{"text", "text", "text", "text"},
			target: 8, overlap: 0, joinSep: "\n", strat: schema.MergeOverCap,
		},
		{
			name:   "json overlap20 unconditional prefix",
			texts:  []string{"a", "b", "c", "d"},
			tkNums: []int{5, 5, 5, 5},
			kinds:  []string{"text", "text", "text", "text"},
			target: 8, overlap: 20, joinSep: "\n", strat: schema.MergeOverCap,
		},
		{
			name:   "text overlap0 joinsep empty",
			texts:  []string{"a", "b", "c"},
			tkNums: []int{5, 5, 5},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 0, joinSep: "", strat: schema.MergeOverCap,
		},
		{
			name:   "text overlap20 unconditional prefix",
			texts:  []string{"a", "b", "c"},
			tkNums: []int{5, 5, 5},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 20, joinSep: "", strat: schema.MergeOverCap,
		},
		{
			name:   "nontext breaks merge run",
			texts:  []string{"a", "IMG", "b"},
			tkNums: []int{5, 0, 5},
			kinds:  []string{"text", "image", "text"},
			target: 8, overlap: 20, joinSep: "\n", strat: schema.MergeOverCap,
		},
		{
			name:   "oversized stands alone (#17799, not merged into prev)",
			texts:  []string{"small", "verylongunitthat exceedsbudgetbyalot", "tiny"},
			tkNums: []int{3, 50, 3},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 0, joinSep: "\n", strat: schema.MergeOverCap,
		},
		{
			name:   "under_cap no overflow",
			texts:  []string{"a", "b", "c"},
			tkNums: []int{5, 5, 5},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 0, joinSep: "\n", strat: schema.MergeUnderCap,
		},
		{
			name:   "under_cap overlap20 still no overflow but carries overlap",
			texts:  []string{"a", "b", "c", "d"},
			tkNums: []int{5, 5, 5, 5},
			kinds:  []string{"text", "text", "text", "text"},
			target: 8, overlap: 20, joinSep: "\n", strat: schema.MergeUnderCap,
		},
		{
			// Tag-bearing overlap source: the previous chunk's text carries a
			// coordinate tag. computeOverlapPrefix strips the tag BEFORE
			// measuring the cut, so the prefix is carved from the visible text
			// only. The oracle must do the same (A2).
			name:   "overlap cuts on tag-stripped visible text",
			texts:  []string{"aa@@1\t2\t3\t4##bb", "cc"},
			tkNums: []int{5, 5},
			kinds:  []string{"text", "text"},
			target: 4, overlap: 20, joinSep: "\n", strat: schema.MergeOverCap,
		},
		{
			// Longer tag so the RAW-text cut would land INSIDE the tag. The
			// overlap prefix must still be carved from the tag-free visible
			// text, so a partial "@@...##" fragment can never leak into the
			// next chunk (A3 / Python test_overlap_prefix_never_leaks_partial_tag).
			name:   "overlap never leaks partial tag when cut lands inside tag",
			texts:  []string{"abcd@@100\t200\t300\t400##", "wxyz"},
			tkNums: []int{10, 4},
			kinds:  []string{"text", "text"},
			target: 5, overlap: 20, joinSep: "\n", strat: schema.MergeOverCap,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			units := make([]schema.ChunkDoc, len(c.texts))
			for i, tx := range c.texts {
				units[i] = schema.ChunkDoc{Text: tx, TKNums: intPtr(c.tkNums[i]), CKType: c.kinds[i]}
			}
			want := pythonMergeUnitsOracle(c.texts, c.tkNums, c.kinds, c.target, c.overlap, c.joinSep, c.strat)
			got := mergeUnits(units, c.target, c.overlap, c.strat, c.joinSep)
			if len(got) != len(want) {
				t.Fatalf("chunk count go=%d want=%d\n got=%v\nwant=%v", len(got), len(want), got, want)
			}
			for i := range got {
				if got[i].Text != want[i].text {
					t.Errorf("chunk[%d] text mismatch:\n got=%q\nwant=%q", i, got[i].Text, want[i].text)
				}
				if intValue(got[i].TKNums) != want[i].tk {
					t.Errorf("chunk[%d] TKNums mismatch: got=%d want=%d (text=%q)", i, intValue(got[i].TKNums), want[i].tk, got[i].Text)
				}
			}
		})
	}
}

// TestMergeUnitsOverlapPrefixIsTagFree locks the cross-language overlap parity
// on TAG-BEARING input: every overlap-prefixed chunk must be carved from the
// previous chunk's tag-free visible text, so a dangling "@@...##" coordinate
// fragment can never leak into a chunk (A3). It also asserts the prefix equals
// the visible-cut tail of the previous chunk, mirroring computeOverlapPrefix.
func TestMergeUnitsOverlapPrefixIsTagFree(t *testing.T) {
	cases := []struct {
		name    string
		prev    string
		cur     string
		target  int
		overlap float64
	}{
		{"cut inside tag", "abcd@@100\t200\t300\t400##", "wxyz", 5, 20},
		{"tag at end", "hello world@@1\t2\t3\t4##", "next", 5, 20},
		{"tag at start", "@@1\t2\t3\t4##leading text", "next", 5, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			units := []schema.ChunkDoc{
				{Text: c.prev, TKNums: intPtr(10), CKType: "text"},
				{Text: c.cur, TKNums: intPtr(4), CKType: "text"},
			}
			got := mergeUnits(units, c.target, c.overlap, schema.MergeOverCap, "\n")
			if len(got) < 2 {
				t.Fatalf("expected >=2 chunks, got %d: %v", len(got), got)
			}
			if strings.Contains(got[1].Text, "@@") || strings.Contains(got[1].Text, "##") {
				t.Errorf("overlap-prefixed chunk leaks coord tag: %q", got[1].Text)
			}
			// The prefix must equal the visible-cut tail of prev (mirror of
			// computeOverlapPrefix): strip tags, cut, prepend.
			vis := []rune(removeTag(c.prev))
			cut := int(float64(len(vis)) * (100.0 - c.overlap) / 100.0)
			if cut < 0 {
				cut = 0
			}
			if cut >= len(vis) {
				cut = len(vis) - 1
			}
			wantPrefix := string(vis[cut:])
			if !strings.HasPrefix(got[1].Text, wantPrefix) {
				t.Errorf("overlap prefix mismatch: got=%q wantPrefix=%q", got[1].Text, wantPrefix)
			}
		})
	}
}
