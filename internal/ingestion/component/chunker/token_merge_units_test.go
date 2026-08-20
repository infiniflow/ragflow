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

// hardCapMergeUnitsOracle is the Go reference for the hard-cap merge contract
// (方案 B) that mergeUnits (the single, unified merge core) must match
// chunk-for-chunk (text AND count). It mirrors the same algorithm as the Go
// TokenChunker, so the comparison pins the merge behavior, not Python's
// OVER_CAP contract (which this code no longer follows).
//
// Contract mirrored here:
//   - token counts are the RUNNING SUM of per-unit counts (never re-tokenizing
//     the joined text), but the merge ALSO re-checks the actual joined text
//     against the target — joinSep (e.g. "\n" on the JSON path) can add tokens
//     beyond the running sum, so a candidate whose joined text exceeds target
//     starts a fresh chunk (hard-cap safety net, matching mergeUnits);
//   - overlap>0 uses a SCALED threshold target*(100-overlap)/100 and prepends
//     the previous chunk's tail, trimmed to fit the target (computeOverlapPrefix
//   - overlapFitPrefix, matching mergeUnits);
//   - a non-text unit passes through and resets the merge run;
//   - the merge is UNDER_CAP: a projected join that would push the running sum
//     over target starts a fresh chunk (never merge-then-close);
//   - oversized units are expanded by mergeUnits BEFORE merging (sentence split
//   - hard token-split fallback); this oracle takes pre-expanded units, so
//     every tkNums it sees is already <= target.
//
// Token counts: the initial chunk and merged running sum use the EXPLICIT
// tkNums (so merge decisions are deterministic and offline-independent), but
// an overlap-prefixed chunk recomputes its count via tokenizeStr — exactly as
// mergeUnits does — so the two stay in lock-step.
func hardCapMergeUnitsOracle(texts []string, tkNums []int, kinds []string, target int, overlap float64, joinSep string) []mergeUnitsOracleRow {
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
		// UNDER_CAP: a projected join that would exceed target starts a fresh
		// chunk. The scaled threshold reserves room for the overlap prefix.
		startNew := float64(merged[prev].tk) > threshold || merged[prev].tk+tk > target
		if !startNew {
			// Hard-cap safety net: re-check the actual joined text (joinSep can
			// add tokens beyond the running sum), mirroring mergeUnits.
			cand := merged[prev].text + texts[i]
			if merged[prev].text != "" && texts[i] != "" {
				cand = merged[prev].text + joinSep + texts[i]
			}
			if tokenizeStr(cand) > target {
				startNew = true
			}
		}
		if startNew {
			text := texts[i]
			cpTk := tk
			if overlap > 0 && merged[prev].text != "" {
				// Mirror mergeUnits: carve the overlap prefix from the previous
				// chunk's tag-free tail, then trim it to fit the target.
				prefix, _ := computeOverlapPrefix(merged[prev].text, overlap)
				prefix, _ = overlapFitPrefix(prefix, texts[i], target)
				text = prefix + texts[i]
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

func TestMergeUnitsMatchesHardCapOracle(t *testing.T) {
	cases := []struct {
		name    string
		texts   []string
		tkNums  []int
		kinds   []string
		target  int
		overlap float64
		joinSep string
	}{
		{
			name:   "json overlap0",
			texts:  []string{"a", "b", "c", "d"},
			tkNums: []int{5, 5, 5, 5},
			kinds:  []string{"text", "text", "text", "text"},
			target: 8, overlap: 0, joinSep: "\n",
		},
		{
			name:   "json overlap20 unconditional prefix",
			texts:  []string{"a", "b", "c", "d"},
			tkNums: []int{5, 5, 5, 5},
			kinds:  []string{"text", "text", "text", "text"},
			target: 8, overlap: 20, joinSep: "\n",
		},
		{
			name:   "text overlap0 joinsep empty",
			texts:  []string{"a", "b", "c"},
			tkNums: []int{5, 5, 5},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 0, joinSep: "",
		},
		{
			name:   "text overlap20 unconditional prefix",
			texts:  []string{"a", "b", "c"},
			tkNums: []int{5, 5, 5},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 20, joinSep: "",
		},
		{
			name:   "nontext breaks merge run",
			texts:  []string{"a", "IMG", "b"},
			tkNums: []int{5, 0, 5},
			kinds:  []string{"text", "image", "text"},
			target: 8, overlap: 20, joinSep: "\n",
		},
		{
			name:   "three units under budget no overflow",
			texts:  []string{"a", "b", "c"},
			tkNums: []int{5, 5, 5},
			kinds:  []string{"text", "text", "text"},
			target: 8, overlap: 0, joinSep: "\n",
		},
		{
			name:   "overlap20 still no overflow but carries overlap",
			texts:  []string{"a", "b", "c", "d"},
			tkNums: []int{5, 5, 5, 5},
			kinds:  []string{"text", "text", "text", "text"},
			target: 8, overlap: 20, joinSep: "\n",
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
			target: 6, overlap: 20, joinSep: "\n",
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
			target: 12, overlap: 20, joinSep: "\n",
		},
		{
			// Current unit near the cap: the overlap prefix carved from the
			// previous chunk would push the fresh chunk over the target, so
			// overlapFitPrefix trims it. The oracle must mirror the trim.
			name:   "overlap prefix trimmed to fit hard cap",
			texts:  []string{strings.Repeat("word ", 10), strings.Repeat("pad ", 10)},
			tkNums: []int{tokenizeStr(strings.Repeat("word ", 10)), tokenizeStr(strings.Repeat("pad ", 10))},
			kinds:  []string{"text", "text"},
			target: 22, overlap: 20, joinSep: "\n",
		},
		{
			// The re-tokenize guard (hard-cap safety net): the joined text
			// (with joinSep "\n") exceeds target even though the running sum
			// fits, so the merge must start a fresh chunk. Both mergeUnits and
			// the oracle must fire the guard identically.
			name:   "joinsep guard fires on actual joined text",
			texts:  []string{"word", "pad"},
			tkNums: []int{1, 1},
			kinds:  []string{"text", "text"},
			target: 2, overlap: 0, joinSep: "\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			units := make([]schema.ChunkDoc, len(c.texts))
			for i, tx := range c.texts {
				units[i] = schema.ChunkDoc{Text: tx, TKNums: intPtr(c.tkNums[i]), CKType: c.kinds[i]}
			}
			want := hardCapMergeUnitsOracle(c.texts, c.tkNums, c.kinds, c.target, c.overlap, c.joinSep)
			got := mergeUnits(units, c.target, c.overlap, c.joinSep)
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
		{"cut inside tag", "abcd@@100\t200\t300\t400##", "wxyz", 10, 20},
		{"tag at end", "hello world@@1\t2\t3\t4##", "next", 10, 20},
		{"tag at start", "@@1\t2\t3\t4##leading text", "next", 10, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			units := []schema.ChunkDoc{
				{Text: c.prev, TKNums: intPtr(10), CKType: "text"},
				{Text: c.cur, TKNums: intPtr(4), CKType: "text"},
			}
			got := mergeUnits(units, c.target, c.overlap, "\n")
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
