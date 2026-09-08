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
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/chunk"
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
		// The DSL contract (shared by the web UI and the Python runtime)
		// expresses single-chunk mode as TokenChunker delimiter_mode "one";
		// in Go that behaviour lives in the OneChunker component.
		if mode, _ := params["delimiter_mode"].(string); mode == "one" {
			return NewOneChunker(params)
		}
		return NewTokenChunker(params)
	case ComponentNameTitleChunker:
		return NewTitleChunker(params)
	case ComponentNameGroupTitleChunker:
		return NewGroupTitleChunker(params)
	case ComponentNameManualChunker:
		return NewManualChunker(params)
	case ComponentNameHierarchyTitleChunker:
		return NewHierarchyTitleChunker(params)
	case ComponentNameQAChunker:
		return NewQAChunker(params)
	case ComponentNameOneChunker:
		return NewOneChunker(params)
	case ComponentNameTableChunker:
		return NewTableChunker(params)
	case ComponentNamePageChunker:
		return NewPageChunker(params)
	default:
		return nil, fmt.Errorf("chunker: unknown component %q", name)
	}
}

// ---------------------------------------------------------------------------
// numeric / list conversion helpers (shared across chunker variants)
// ---------------------------------------------------------------------------

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

// compileDelimPattern compiles a TokenChunker-style []string delimiter list.
// Every non-empty entry is active, including bare (non-backtick) delimiters,
// mirroring Python naive_merge / rag/nlp/delim where bare single-character
// delimiters still split. Backtick-wrapped entries contribute their inner
// content. invokeTextPayload decides whether an active delimiter yields one
// chunk per segment (custom/backtick, no merge) or splits into paragraphs that
// are merged by token size (bare). Canonical single-string parser_config.delimiter
// parsing lives in ragflow/internal/parser/chunk (ParseDelimiterField).
func compileDelimPattern(delims []string) *regexp.Regexp {
	return chunk.CompileDelimiterPatternList(delims, true)
}

// splitDroppingDelim mirrors Python's _split_text_by_pattern
// (token_chunker.py:79-90). The captured delimiter is DISCARDED rather than
// glued to a segment: re.split with a captured group keeps delimiters at odd
// indices, and only the even-index (text) parts are kept. This is the
// behavior every delimiter path (primary and children, text/markdown/html
// and JSON) must reproduce so a split chunk reads "first sentence here"
// without the trailing delimiter.
func splitDroppingDelim(text string, pattern *regexp.Regexp) []string {
	if pattern == nil {
		return []string{text}
	}
	idxs := pattern.FindAllStringIndex(text, -1)
	if len(idxs) == 0 {
		return []string{text}
	}
	var out []string
	cursor := 0
	for _, idx := range idxs {
		start, end := idx[0], idx[1]
		if start == cursor {
			cursor = end
			continue
		}
		out = append(out, text[cursor:start])
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

// chunkOutputs builds the canonical chunker output (output_format="chunks" +
// chunks). The Go runtime passes only this explicit output to the next node,
// so the run-level metadata that downstream components still need (e.g.
// `name` for Tokenizer title embedding, or tenant_id/kb_id for embedding
// model resolution) is NOT re-emitted here — it lives in the workflow-wide
// CanvasState.Globals bag (seeded at pipeline start, published by the File
// component) and read directly from ctx. See runtime.CanvasState.Globals.
func chunkOutputs(chunks []schema.ChunkDoc) map[string]any {
	return map[string]any{
		"output_format": "chunks",
		"chunks":        schema.ChunkDocsToMaps(chunks),
	}
}

// withName returns a shallow copy of inputs with name set, so a component can
// guarantee `name` is present on the map it forwards to a decode step without
// mutating the caller's snapshot.
func withName(inputs map[string]any, name string) map[string]any {
	cp := make(map[string]any, len(inputs)+1)
	for k, v := range inputs {
		cp[k] = v
	}
	cp["name"] = name
	return cp
}

// cloneInputs returns a shallow copy of m with room for one extra key. Used to
// inject the Globals-resolved `name` into the decode input without mutating
// the caller's input snapshot.
func cloneInputs(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	cp := make(map[string]any, len(m)+1)
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
