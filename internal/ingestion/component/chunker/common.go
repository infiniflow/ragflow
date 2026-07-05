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

package chunker

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

// newChunkerByName dispatches the DSL name to a typed constructor.
// Centralised here so each chunker file only needs an init() that
// declares its registered name (see register.go). The returned
// runtime.Component interface is satisfied directly by each
// constructor (NewTokenChunker etc.) — no intermediate assertion
// is needed.
func newChunkerByName(name string, params map[string]any) (runtime.Component, error) {
	switch name {
	case ComponentNameTokenChunker:
		return NewTokenChunker(params)
	case ComponentNameTitleChunker:
		return NewTitleChunker(params)
	case ComponentNameGroupTitleChunker:
		return NewGroupTitleChunker(params)
	case ComponentNameHierarchyTitleChunker:
		return NewHierarchyTitleChunker(params)
	default:
		return nil, fmt.Errorf("chunker: unknown component %q", name)
	}
}

// ---------------------------------------------------------------------------
// numeric / list conversion helpers (shared across chunker variants)
// ---------------------------------------------------------------------------

// numericFromAny normalises JSON-decoded ints to float64 so the
// schema-defaults-vs-Param-Update convention doesn't depend on the
// encoding source (yaml/toml/json all behave the same).
func numericFromAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	}
	return 0, false
}

func stringListFromAny(in []any) []string {
	out := make([]string, 0, len(in))
	for _, x := range in {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// regex / split helpers
// ---------------------------------------------------------------------------

// compileDelimPattern joins all delimiter entries into a single
// alternation. Entries wrapped in backticks are treated as regex
// literals and regex-escaped; plain strings are simply regex-escaped.
// Longer patterns win (matches python `sorted(set, key=len, reverse=True)`).
func compileDelimPattern(delims []string) *regexp.Regexp {
	var custom []string
	var plain []string
	for _, d := range delims {
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "`") && strings.HasSuffix(d, "`") && len(d) >= 2 {
			custom = append(custom, regexp.QuoteMeta(d[1:len(d)-1]))
		} else {
			plain = append(plain, regexp.QuoteMeta(d))
		}
	}
	all := append(plain, custom...)
	if len(all) == 0 {
		return nil
	}
	sort.SliceStable(all, func(i, j int) bool { return len(all[i]) > len(all[j]) })
	return regexp.MustCompile(strings.Join(all, "|"))
}

// splitKeepingDelim is the Go equivalent of python's
// `re.split((pattern), text, flags=re.DOTALL)` with the matched
// delimiter preserved (alternation keeps the original delimiter text
// in the output stream so the rebuilding at token_chunker.py:88-93
// stays lossy-free).
func splitKeepingDelim(text string, pattern *regexp.Regexp) []string {
	if pattern == nil {
		return []string{text}
	}
	idxs := pattern.FindAllStringSubmatchIndex(text, -1)
	if len(idxs) == 0 {
		return []string{text}
	}
	var out []string
	cursor := 0
	for _, idx := range idxs {
		start, end := idx[0], idx[1]
		if start > cursor {
			out = append(out, text[cursor:start])
		}
		out = append(out, text[start:end])
		cursor = end
	}
	if cursor < len(text) {
		out = append(out, text[cursor:])
	}
	return out
}

// ---------------------------------------------------------------------------
// chunk-doc helpers
// ---------------------------------------------------------------------------

// itemText returns the text payload from a JSON-style chunk item,
// preferring "text", then "content_with_weight".
func itemText(it schema.ChunkDoc) (string, bool) {
	if it.Text != "" {
		return it.Text, true
	}
	if it.ContentWithWeight != "" {
		return it.ContentWithWeight, true
	}
	return "", false
}

// itemDocType mirrors _build_json_chunks's type derivation.
func itemDocType(it schema.ChunkDoc) string {
	switch strings.ToLower(strings.TrimSpace(it.DocType)) {
	case "table":
		return "table"
	case "image":
		return "image"
	}
	return "text"
}

// itemTextOrFallback returns the item's preferred text, or "".
func itemTextOrFallback(it schema.ChunkDoc) string {
	if t, ok := itemText(it); ok {
		return t
	}
	return ""
}

// tokenizeStr is the shared NumTokensFromString wrapper used by
// Table/Image context attachment. Lives here so we can centrally
// swizzle the count strategy in one place if needed.
func tokenizeStr(s string) int { return tokenizer.NumTokensFromString(s) }

// toString normalises a chunk-map field to a string. Empty strings
// for missing fields.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// emptyOutputs returns the canonical no-chunks payload.
func emptyOutputs() map[string]any {
	return map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{},
	}
}

func emptyChunkDocs() []schema.ChunkDoc { return []schema.ChunkDoc{} }

func chunkOutputs(chunks []schema.ChunkDoc) map[string]any {
	return map[string]any{
		"output_format": "chunks",
		"chunks":        schema.ChunkDocsToMaps(chunks),
	}
}
