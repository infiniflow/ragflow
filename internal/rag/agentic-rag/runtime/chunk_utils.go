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

package runtime

import (
	"fmt"
	"strings"
	"unicode"
)

// Field accessors for chunk maps.
//
// Maintenance-helper data — doc ids, dataset ids, titles — has historically been stored under
// different field names depending on the backend and indexer, so every read goes through a
// helper that tolerates all known aliases.
//
// NOTE: tools/ keeps its own accessors rather than importing these. That duplication is what
// keeps the dependency graph acyclic (tools must not import the root package the root package
// imports tools from).

// chunkAttr returns the first non-empty value among keys.
// Truthiness is "not nil and not empty".
func chunkAttr(c map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := c[k]; ok && v != nil {
			if s := fmt.Sprint(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// ChunkTextOf: The root package already provides
// chunkText with identical semantics; this exported form exists so callers
// outside the package share one implementation.
func ChunkTextOf(c map[string]any) string { return chunkText(c) }

// DocIDOf: doc_id / docid / document_id.
func DocIDOf(c map[string]any) string {
	return chunkAttr(c, "doc_id", "docid", "document_id")
}

// shingleRunes is the shingle size of the near-duplicate test below. Four characters is short enough to
// survive a re-cut boundary (a passage the windowing sliced at a different place) and long enough that
// unrelated Chinese prose does not collide on its own.
const shingleRunes = 4

// duplicateOverlap is where two passages count as saying the SAME thing. It is 0.98, NOT the 0.85 a
// same-kind pipeline uses for its partial-overlap pass, and the difference is the whole point: this
// runtime's questions are enumerations, whose member passages differ by a NAME and agree in everything
// else — "关羽手起一刀，斩颜良于马下" against "…斩文丑于马下" is two members by any reader, and a 0.85
// containment test would call them one passage and drop a member (measured on this repo's own fixtures:
// forty passages differing in one digit collapsed to ten at 0.85). What this test is FOR is the same
// passage delivered twice — overlapping windows of adjacent chunks, and parent/child duplicates the
// retrieval layer already produces (see RetrievalByChildren) — and those agree to within a cut boundary.
const duplicateOverlap = 0.98

// shingles is the 4-rune shingle set of a normalized text (case- and whitespace-insensitive), or nil.
func shingles(s string) map[string]struct{} {
	r := []rune(spaceLess(strings.ToLower(s)))
	switch {
	case len(r) == 0:
		return nil
	case len(r) < shingleRunes:
		return map[string]struct{}{string(r): {}}
	}
	out := make(map[string]struct{}, len(r)-shingleRunes+1)
	for i := 0; i+shingleRunes <= len(r); i++ {
		out[string(r[i:i+shingleRunes])] = struct{}{}
	}
	return out
}

// TextOverlap is the share of the SHORTER text's shingles that also appear in the longer one: 1.0 when
// one passage contains the other, 0 when they share nothing.
//
// It is a containment-flavoured measure on purpose. The passages this runtime compares are windows cut
// out of adjacent chunks of the SAME document (overlapping retrieval windows), so a long passage and the
// short window inside it are the same evidence — and a symmetric measure would call them different.
func TextOverlap(a, b string) float64 {
	sa, sb := shingles(a), shingles(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	small, large := sa, sb
	if len(sb) < len(sa) {
		small, large = sb, sa
	}
	hit := 0
	for g := range small {
		if _, ok := large[g]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(small))
}

// NearDuplicate reports whether two passages say the same thing (see duplicateOverlap).
//
// A cheap length guard runs first: a passage five times the length of another cannot be 85% contained in
// it, and the guard keeps the O(n·m) comparison off the long tail of a scan's hit set.
func NearDuplicate(a, b string) bool {
	la, lb := len([]rune(a)), len([]rune(b))
	if la == 0 || lb == 0 {
		return false
	}
	if la > lb*5 || lb > la*5 {
		return false
	}
	return TextOverlap(a, b) >= duplicateOverlap
}

// datasetIDOf: dataset_id / kb_id / knowledgebase_id.
func datasetIDOf(c map[string]any) string {
	return chunkAttr(c, "dataset_id", "kb_id", "knowledgebase_id")
}

// DocTitleOf: exactly: docnm_kwd / doc_title / title /
// document_name (the same four keys, in the same order). Go chunk retrieval
// carries the title under docnm_kwd, so no extra alias is needed.
func DocTitleOf(c map[string]any) string {
	return chunkAttr(c, "docnm_kwd", "doc_title", "title", "document_name")
}

// ChunkIDOf: chunk_id / id.
func ChunkIDOf(c map[string]any) string { return chunkAttr(c, "chunk_id", "id") }

// Snippet: trim both ends, cut to limit, right-trim ALL
// trailing whitespace (not just spaces), then add an ellipsis marker when the
// value was actually truncated. The right-trim strips ANY Unicode whitespace (space, tab,
// newline, ...), so a cut that ends mid-run of whitespace collapses to the same trailing slice
// before "...".
//
// Indexing is by Unicode code point, so the limit and the cut are character-based. Go's
// len/[:] are byte-based and would split a multibyte (e.g. CJK) rune and emit invalid UTF-8,
// hence the []rune conversion.
func Snippet(s string, limit int) string {
	t := strings.TrimSpace(s)
	r := []rune(t)
	if len(r) <= limit {
		return t
	}
	return strings.TrimRightFunc(string(r[:limit]), unicode.IsSpace) + "..."
}

// isTableChunk: / _is_table_text: a corpus-neutral table detector — HTML table markup, or
// >=3 pipe rows.
func isTableChunk(c map[string]any) bool {
	return isTableText(ChunkTextOf(c))
}

// isTableText: table detection from raw text.
func isTableText(text string) bool {
	t := strings.ToLower(text)
	if strings.Contains(t, "<table") || strings.Contains(t, "<tr") {
		return true
	}
	pipeRows := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Count(line, "|") >= 2 {
			pipeRows++
		}
	}
	return pipeRows >= 3
}

// xmlEscape: the four XML entities (&, <, >, ").
// The apostrophe is intentionally NOT escaped — values are only ever embedded inside
// double-quoted XML/Markdown attributes, where a literal ' is valid, so escaping it to &apos;
// would be wrong.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

// ChunkDocIDs lists the distinct documents a chunk set belongs to, in first-seen
// order. It is what a reasoning step reports as "from N documents".
func ChunkDocIDs(chunks []map[string]any) []string {
	seen := make(map[string]struct{}, len(chunks))
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		id := DocIDOf(c)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ChunkEvidenceIDs lists a chunk set's chunk ids, deduplicated, capped at limit
// (limit <= 0 means no cap). These are the anchors a reasoning step points at.
func ChunkEvidenceIDs(chunks []map[string]any, limit int) []string {
	seen := make(map[string]struct{}, len(chunks))
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		id := ChunkIDOf(c)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// mergeChunks deduplicates incoming chunks against an existing slice by chunkKey, appending
// only unseen ones — the merge pattern used by the direct and compiled-expansion paths.
//
// Returns the merged slice and the global indices of the newly appended chunks.
func mergeChunks(existing, incoming []map[string]any) ([]map[string]any, []int) {
	seen := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		seen[chunkKey(c)] = struct{}{}
	}
	out := existing
	var idx []int
	for _, c := range incoming {
		if c == nil {
			continue
		}
		k := chunkKey(c)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		idx = append(idx, len(out))
		out = append(out, c)
	}
	return out, idx
}

// MergeDocAggs deduplicates doc aggregations by doc id, mirroring
// direct.py:_merge_kbinfos. Returns the merged slice and the number added.
func MergeDocAggs(existing, incoming []map[string]any) ([]map[string]any, int) {
	seen := make(map[any]struct{}, len(existing))
	for _, d := range existing {
		seen[d["doc_id"]] = struct{}{}
	}
	out := existing
	added := 0
	for _, d := range incoming {
		if d == nil {
			continue
		}
		key := d["doc_id"]
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, d)
		added++
	}
	return out, added
}
