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

package harness

import (
	"fmt"
	"strings"
	"unicode"
)

// Field accessors for chunk maps.
//
// Mirrors Python harness/chunk_utils.py. Maintenance-helper data — doc ids,
// dataset ids, titles — has historically been stored under different field
// names depending on the backend and indexer, so every read goes through a
// helper that tolerates all known aliases.
//
// NOTE: tools/ keeps its own accessors rather than importing these, mirroring
// Python, where tools/navigation.py defines its own _chunk_id. That duplication
// is what keeps the dependency graph acyclic (tools must not import the root
// package the root package imports tools from).

// ChunkAttr returns the first non-empty value among keys.
// Mirrors Python _chunk_attr: truthiness is `v not in (None, "")`.
func ChunkAttr(c map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := c[k]; ok && v != nil {
			if s := fmt.Sprint(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// ChunkTextOf mirrors Python _chunk_text. The root package already provides
// chunkText with identical semantics; this exported form exists so callers
// outside the package share one implementation.
func ChunkTextOf(c map[string]any) string { return chunkText(c) }

// DocIDOf mirrors Python _doc_id: doc_id / docid / document_id.
func DocIDOf(c map[string]any) string {
	return ChunkAttr(c, "doc_id", "docid", "document_id")
}

// DatasetIDOf mirrors Python _dataset_id: dataset_id / kb_id / knowledgebase_id.
func DatasetIDOf(c map[string]any) string {
	return ChunkAttr(c, "dataset_id", "kb_id", "knowledgebase_id")
}

// DocTitleOf mirrors Python _doc_title exactly: docnm_kwd / doc_title / title /
// document_name (the same four keys, in the same order). Go chunk retrieval
// carries the title under docnm_kwd, so no extra alias is needed.
func DocTitleOf(c map[string]any) string {
	return ChunkAttr(c, "docnm_kwd", "doc_title", "title", "document_name")
}

// ChunkIDOf mirrors Python _chunk_id: chunk_id / id.
func ChunkIDOf(c map[string]any) string { return ChunkAttr(c, "chunk_id", "id") }

// Snippet mirrors Python _snippet: trim both ends, cut to limit, right-trim ALL
// trailing whitespace (not just spaces), then add an ellipsis marker when the
// value was actually truncated. Python's .rstrip() with no argument strips any
// Unicode whitespace (space, tab, newline, ...), so a cut that ends mid-run of
// whitespace collapses to the same trailing slice before "...".
//
// Python indexes strings by Unicode code point, so len(s) and s[:n] are
// character-based. Go's len/[:] are byte-based and would split a multibyte
// (e.g. CJK) rune and emit invalid UTF-8. We therefore convert to []rune so the
// limit and the cut operate on code points exactly like Python.
func Snippet(s string, limit int) string {
	t := strings.TrimSpace(s)
	r := []rune(t)
	if len(r) <= limit {
		return t
	}
	return strings.TrimRightFunc(string(r[:limit]), unicode.IsSpace) + "..."
}

// IsTableChunk mirrors Python _is_table_chunk / _is_table_text: a corpus-neutral
// table detector — HTML table markup, or >=3 pipe rows. Exported so the
// orchestrator and the bridge share one implementation.
func IsTableChunk(c map[string]any) bool {
	return isTableText(ChunkTextOf(c))
}

// isTableText mirrors Python _is_table_text: table detection from raw text.
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

// XMLEscape mirrors Python _xml_escape: the four XML entities (&, <, >, ").
// Python intentionally does NOT escape the apostrophe — values are only ever
// embedded inside double-quoted XML/Markdown attributes, where a literal '
// is valid, so escaping it to &apos; would diverge from the Python output.
func XMLEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

// MergeChunks deduplicates incoming chunks against an existing slice by
// chunkKey, appending only unseen ones. Mirrors Python's
// `seen = {_chunk_key(c) for c in kbinfos["chunks"]}` merge pattern used by
// direct.py and compiled_expansion.py.
//
// Returns the merged slice and the global indices of the newly appended chunks.
func MergeChunks(existing, incoming []map[string]any) ([]map[string]any, []int) {
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
