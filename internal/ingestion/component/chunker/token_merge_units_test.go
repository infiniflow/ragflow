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
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// pythonMergeUnitsOracle is the Go port of the UNIFIED merge contract shared by
// the Go TokenChunker, Python naive_merge, and Python token_chunker's JSON
// path. It is the oracle that mergeUnits (the single, unified merge core) must
// match chunk-for-chunk (text and count).
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
func pythonMergeUnitsOracle(texts []string, tkNums []int, kinds []string, target int, overlap float64, joinSep string, strat schema.MergeStrategy) []string {
	threshold := float64(target) * (100.0 - overlap) / 100.0
	type ck struct {
		text string
		tk   int
	}
	merged := []ck{}
	prev := -1
	for i := range texts {
		if kinds[i] != "text" {
			merged = append(merged, ck{texts[i], tkNums[i]})
			prev = -1
			continue
		}
		tk := tkNums[i]
		if prev < 0 {
			merged = append(merged, ck{texts[i], tk})
			prev = len(merged) - 1
			continue
		}
		// #17799: an over-budget unit stands alone — never merged into prev.
		if tk > target {
			text := texts[i]
			cpTk := tk
			if overlap > 0 && merged[prev].text != "" {
				vis := []rune(merged[prev].text)
				cut := int(float64(len(vis)) * (100.0 - overlap) / 100.0)
				if cut < 0 {
					cut = 0
				}
				if cut < len(vis) {
					text = string(vis[cut:]) + texts[i]
				}
				cpTk = tokenizeStr(text)
			}
			merged = append(merged, ck{text, cpTk})
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
				vis := []rune(merged[prev].text)
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
			merged = append(merged, ck{text, cpTk})
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
	out := make([]string, len(merged))
	for i := range merged {
		out[i] = merged[i].text
	}
	return out
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			units := make([]schema.ChunkDoc, len(c.texts))
			for i, tx := range c.texts {
				units[i] = schema.ChunkDoc{Text: tx, TKNums: intPtr(c.tkNums[i]), CKType: c.kinds[i]}
			}
			want := pythonMergeUnitsOracle(c.texts, c.tkNums, c.kinds, c.target, c.overlap, c.joinSep, c.strat)
			got := mergeUnits(units, c.target, c.overlap, c.strat, c.joinSep)
			gotTexts := make([]string, len(got))
			for i := range got {
				gotTexts[i] = got[i].Text
			}
			if len(gotTexts) != len(want) {
				t.Fatalf("chunk count go=%d want=%d\n got=%v\nwant=%v", len(gotTexts), len(want), gotTexts, want)
			}
			for i := range gotTexts {
				if gotTexts[i] != want[i] {
					t.Errorf("chunk[%d] mismatch:\n got=%q\nwant=%q", i, gotTexts[i], want[i])
				}
			}
		})
	}
}
