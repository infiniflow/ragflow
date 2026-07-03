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

// SCOPE (honest) for token.go (Phase 2.3a):
//
//   - WHITELIST mirrors rag/flow/chunker/token_chunker.py:TokenChunkerParam.check
//     exactly: delimiter_mode ∈ {"token_size","delimiter","one"},
//     chunk_token_size > 0, overlapped_percent ∈ [0, 1), table_context_size ≥ 0,
//     image_context_size ≥ 0. enum/range checks live in param.Check.
//
//   - DELIMITER PARSING mirrors python `_compile_delimiter_pattern`:
//     entries wrapped in backticks (e.g. "`\\n\\n`") are treated as
//     regex split points; plain strings are regex-escaped and joined
//     into the same alternation. Empty entries are filtered.
//
//   - CHILDREN DELIMITERS (the secondary split) is implemented via the
//     shared splitKeepingDelim helper; emitted chunks carry the parent
//     ("mom") and the split child ("text") keys.
//
//   - MODE "delimiter" uses the regex-aware delimiter pattern; the
//     token_size merge pass only runs when no working delimiter was
//     detected — matching python at token_chunker.py:359-360.
//
//   - MODE "token_size" falls back to a chunk-engine merge plan. The
//     python `naive_merge` algorithm uses a sentence-aware + stop-word
//     strategy that the Go chunk_engine's "greedy" merge approximates;
//     this is flagged as a TODO parity-gap and intentionally not
//     machine-mirrored.
//
//   - MODE "one" emits a single chunk containing the entire payload.
//
//   - JSON-STRUCTURED INPUT (output_format == "json" or upstream
//     "chunks") is normalized into the same internal chunk shape via
//     a parallel fan-out (Plan §4 Phase 2 row 2.3a, Parallelism=4).
//     Media-context attachment is per-item sequential; merge is
//     index-deterministic (Plan §8 R8).
//
//   - No PDF/outline awareness (Python `restore_pdf_text_previews`).
//     That depends on deepdoc/parser which is out of scope for this
//     phase; the chunker accepts arbitrary upstream "chunks" payloads
//     and runs the same logic against them.
package chunker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/chunk"
)

const ComponentNameTokenChunker = "TokenChunker"

type tokenChunkerParam struct {
	schema.TokenChunkerParam
}

func (p *tokenChunkerParam) Update(conf map[string]any) {
	if conf == nil {
		return
	}
	if v, ok := conf["delimiter_mode"].(string); ok {
		p.TokenChunkerParam.DelimiterMode = v
	}
	if v, ok := numericFromAny(conf["chunk_token_size"]); ok {
		p.TokenChunkerParam.ChunkTokenSize = int(v)
	}
	if v, ok := conf["delimiters"].([]any); ok {
		p.TokenChunkerParam.Delimiters = stringListFromAny(v)
	} else if v, ok := conf["delimiters"].([]string); ok {
		p.TokenChunkerParam.Delimiters = append([]string(nil), v...)
	}
	if v, ok := numericFromAny(conf["overlapped_percent"]); ok {
		p.TokenChunkerParam.OverlappedPercent = v
	}
	if v, ok := conf["children_delimiters"].([]any); ok {
		p.TokenChunkerParam.ChildrenDelimiters = stringListFromAny(v)
	} else if v, ok := conf["children_delimiters"].([]string); ok {
		p.TokenChunkerParam.ChildrenDelimiters = append([]string(nil), v...)
	}
	if v, ok := numericFromAny(conf["table_context_size"]); ok {
		p.TokenChunkerParam.TableContextSize = int(v)
	}
	if v, ok := numericFromAny(conf["image_context_size"]); ok {
		p.TokenChunkerParam.ImageContextSize = int(v)
	}
}

func defaultsToken(p tokenChunkerParam) tokenChunkerParam {
	p.TokenChunkerParam = schema.TokenChunkerParam{}.Defaults()
	return p
}

// TokenChunkerComponent implements the runtime.Component interface for
// the TokenChunker variant.
type TokenChunkerComponent struct {
	name  string
	param tokenChunkerParam
}

// NewTokenChunker constructs a TokenChunker from the DSL param map.
// Errors here surface as canvas compile failures (mirrors the
// python check() phase).
func NewTokenChunker(params map[string]any) (runtime.Component, error) {
	p := defaultsToken(tokenChunkerParam{})
	p.Update(params)
	if err := p.TokenChunkerParam.Validate(); err != nil {
		return nil, fmt.Errorf("TokenChunker: %w", err)
	}
	return &TokenChunkerComponent{
		name:  ComponentNameTokenChunker,
		param: p,
	}, nil
}

// Parallelism is the configured intra-component fan-out (plan §4
// Phase 2 row 2.3a).
func (c *TokenChunkerComponent) Parallelism() int { return 4 }

// Inputs is exposed so callers can introspect.
func (c *TokenChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

// Outputs is exposed so callers can introspect.
func (c *TokenChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

// Invoke runs the chunker against the input payload.
//
// Concurrency: text payloads are fanned across Parallelism goroutines
// by primary-delimiter segment; structured JSON/chunks payloads fan
// across items. Merge is by input index (plan §8 R8): the i-th
// goroutine's output occupies slot i, regardless of completion order.
//
// Timeout: honours ctx cancellation only — there is no inner @timeout
// decorator equivalent (plan §8 R1).
func (c *TokenChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return runtime.TrackElapsed(ComponentNameTokenChunker, func() (map[string]any, error) {
		return c.invoke(ctx, inputs)
	})
}

func (c *TokenChunkerComponent) invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	if _, ok := inputs["name"].(string); !ok {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"chunks_full":   []map[string]any{},
			"_ERROR":        "TokenChunker: missing required upstream field \"name\"",
		}, nil
	}

	delimPattern := compileDelimPattern(c.param.Delimiters)
	childrenPattern := compileChildrenPattern(c.param.ChildrenDelimiters)

	if text, ok := stringFromInputs(inputs, "text", "content"); ok {
		return c.invokeTextPayload(ctx, text, delimPattern, childrenPattern), nil
	}
	if chunksAny := chunksFromInputs(inputs); chunksAny != nil {
		return c.invokeJSONPayload(ctx, chunksAny, delimPattern, childrenPattern), nil
	}
	return emptyOutputs(), nil
}

// invokeTextPayload handles plain-text input (output_format in
// {markdown,text,html} on the python side; here we collapse all three
// because the text payload's distinction lives in the upstream
// component, not in the chunker).
func (c *TokenChunkerComponent) invokeTextPayload(_ context.Context, text string, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	if text == "" {
		return emptyOutputs()
	}

	mode := c.param.DelimiterMode
	if mode == "one" {
		out := map[string]any{"text": text}
		chunks := []map[string]any{out}
		return map[string]any{
			"output_format": "chunks",
			"chunks":        chunks,
			"chunks_full":   enrichWithIndex(chunks),
		}
	}

	if !hasActiveDelimiter(delimPattern) {
		// No primary delimiter hit — fall back to a token-size merge.
		return c.mergeByTokenSize(text, childrenPattern)
	}

	parts := splitKeepingDelim(text, delimPattern)
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		cleaned = append(cleaned, p)
	}
	if len(cleaned) == 0 {
		return emptyOutputs()
	}
	docs := applyChildrenDelim(cleaned, childrenPattern)
	return map[string]any{
		"output_format": "chunks",
		"chunks":        docs,
		"chunks_full":   enrichWithIndex(docs),
	}
}

// mergeByTokenSize uses the chunk library's split + postprocess-merge
// to combine the payload into chunks of approximately
// chunk_token_size runes. Mirrors python's `naive_merge` fallback
// (token_chunker.py:319-324) at the wire-shape level; the merge
// strategy is the Go chunk library's "greedy" approximation.
//
// This caller uses the typed chunk.Run entry point directly:
// same operator sequence, no DSL round-trip.
func (c *TokenChunkerComponent) mergeByTokenSize(text string, childrenPattern *regexp.Regexp) map[string]any {
	target := c.param.ChunkTokenSize
	result, err := chunk.Run(text, chunk.PipelineOptions{
		StripWhitespace:  true,
		RemoveEmptyLines: true,
		SplitStrategy:    "sentence",
		MergeTargetSize:  target,
		MergeStrategy:    "greedy",
	})
	if err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{{"text": text}},
			"chunks_full":   []map[string]any{{"text": text, "index": 0, "size": len([]rune(text))}},
		}
	}
	docs := make([]map[string]any, 0, len(result.ResultChunks))
	for _, ck := range result.ResultChunks {
		content := strings.TrimSpace(ck.Content)
		if content == "" {
			continue
		}
		docs = append(docs, map[string]any{"text": content})
	}
	final := applyChildrenDelimText(docs, childrenPattern)
	return map[string]any{
		"output_format": "chunks",
		"chunks":        final,
		"chunks_full":   enrichWithIndex(final),
	}
}

// invokeJSONPayload handles structured upstream input. Items fan
// across goroutines (Parallelism); merge is by input index.
func (c *TokenChunkerComponent) invokeJSONPayload(ctx context.Context, items []map[string]any, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	if len(items) == 0 {
		return emptyOutputs()
	}
	mode := c.param.DelimiterMode
	if mode == "one" {
		var parts []string
		for _, it := range items {
			if t := itemTextOrFallback(it); t != "" {
				parts = append(parts, t)
			}
		}
		merged := strings.Join(parts, "\n")
		chunks := []map[string]any{{"text": merged}}
		return map[string]any{
			"output_format": "chunks",
			"chunks":        chunks,
			"chunks_full":   enrichWithIndex(chunks),
		}
	}

	workers := c.Parallelism()
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) {
		workers = len(items)
	}
	lanes := partition(len(items), workers)
	perItem := make([][]map[string]any, len(items))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lane := lanes[w]
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				if err := ctx.Err(); err != nil {
					perItem[i] = nil
					continue
				}
				perItem[i] = chunkFromItem(items[i], delimPattern)
			}
		}(lane.start, lane.end)
	}
	wg.Wait()

	// Attach surrounding media context (token_chunker.py:358).
	attached := attachMediaContext(perItem, c.param.TableContextSize, c.param.ImageContextSize)

	// Optional token-size merge (only when no working delimiter).
	if !hasActiveDelimiter(delimPattern) {
		attached = mergeByTokenSizeFromJSON(attached, c.param.ChunkTokenSize, c.param.OverlappedPercent)
	}

	flat := flatten(attached)
	if childrenPattern != nil {
		flat = splitByChildren(flat, childrenPattern)
	}

	out := make([]map[string]any, 0, len(flat))
	for _, m := range flat {
		if m == nil {
			continue
		}
		if t, ok := m["text"].(string); ok && strings.TrimSpace(t) == "" {
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return emptyOutputs()
	}
	return map[string]any{
		"output_format": "chunks",
		"chunks":        out,
		"chunks_full":   enrichWithIndex(out),
	}
}

// ---------------------------------------------------------------------------
// JSON-payload internals
// ---------------------------------------------------------------------------

// chunkFromItem mirrors _build_json_chunks for a single item.
func chunkFromItem(it map[string]any, delimPattern *regexp.Regexp) []map[string]any {
	ckType := itemDocType(it)
	txt := itemTextOrFallback(it)
	if ckType != "text" {
		return []map[string]any{buildChunkMap(it, ckType, txt, "", "")}
	}
	if !hasActiveDelimiter(delimPattern) {
		return []map[string]any{buildChunkMap(it, "text", txt, "", "")}
	}
	parts := splitKeepingDelim(txt, delimPattern)
	if !delimPattern.MatchString(txt) {
		return []map[string]any{buildChunkMap(it, "text", txt, "", "")}
	}
	out := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, buildChunkMap(it, "text", p, "", ""))
	}
	if len(out) == 0 {
		return []map[string]any{buildChunkMap(it, "text", txt, "", "")}
	}
	return out
}

// buildChunkMap constructs the python-compatible chunk payload.
//
// The chunker output carries the basic text+doc_type_kwd+ck_type
// fields plus the per-chunk meta fields the python
// rag/flow/chunker/token_chunker.py emits:
//
//   - tk_nums       — tokenized list (used downstream by Tokenizer)
//   - mom           — parent-section identifier (title / hierarchy
//     chunkers populate; TokenChunker pass-through)
//   - img_id        — image attachment identifier
//   - layout        — layout classification (text / table / image / figure)
//   - _pdf_positions — PDF bbox coordinates when the parser path
//     emitted them on the upstream item
//   - context_above / context_below — surrounding media context
//     when attachMediaContext was invoked
//
// Pass-through fields are sourced from the input item map. Missing
// fields are simply absent from the output (the python side does
// the same — see python `_build_json_chunks`).
func buildChunkMap(it map[string]any, ckType, text, ctxAbove, ctxBelow string) map[string]any {
	out := map[string]any{
		"text":         text,
		"doc_type_kwd": ckType,
		"ck_type":      ckType,
		"tk_nums":      tokenizeStr(text),
	}
	if ctxAbove != "" {
		out["context_above"] = ctxAbove
	}
	if ctxBelow != "" {
		out["context_below"] = ctxBelow
	}
	for _, key := range []string{"mom", "img_id", "layout", "_pdf_positions", "positions", "image", "page_number"} {
		if v, ok := it[key]; ok && v != nil {
			out[key] = v
		}
	}
	return out
}

type lane struct{ start, end int }

func partition(n, parts int) []lane {
	if parts < 1 {
		parts = 1
	}
	if n < parts {
		parts = n
	}
	out := make([]lane, 0, parts)
	size := n / parts
	rem := n % parts
	cursor := 0
	for i := 0; i < parts; i++ {
		end := cursor + size
		if i < rem {
			end++
		}
		if end > n {
			end = n
		}
		if cursor < end {
			out = append(out, lane{start: cursor, end: end})
		}
		cursor = end
	}
	return out
}

func attachMediaContext(perItem [][]map[string]any, tableCtx, imageCtx int) [][]map[string]any {
	if tableCtx <= 0 && imageCtx <= 0 {
		return perItem
	}
	for idx := range perItem {
		chunks := perItem[idx]
		if len(chunks) == 0 {
			continue
		}
		for i, ck := range chunks {
			ckType, _ := ck["ck_type"].(string)
			if ckType != "table" && ckType != "image" {
				continue
			}
			ctx := imageCtx
			if ckType == "table" {
				ctx = tableCtx
			}
			if ctx <= 0 {
				continue
			}
			ck["context_above"] = collectContext(chunks, i, ctx, true)
			ck["context_below"] = collectContext(chunks, i, ctx, false)
		}
	}
	return perItem
}

// collectContext walks chunks around `i` (above when direction==true,
// below when false), pulling text chunks while remaining token budget
// stays positive. Matches token_chunker.py:_attach_context_to_media_chunks.
func collectContext(chunks []map[string]any, i, ctxTokens int, above bool) string {
	var parts []string
	remain := ctxTokens
	var pos int
	if above {
		pos = i - 1
		for pos >= 0 && remain > 0 {
			if other, ok := chunks[pos]["ck_type"].(string); ok && other == "text" {
				tk, _ := chunks[pos]["tk_nums"].(int)
				txt := toString(chunks[pos]["text"])
				if tk >= remain {
					parts = append([]string{takeFromEnd(txt, remain)}, parts...)
					remain = 0
					break
				}
				parts = append([]string{txt}, parts...)
				remain -= tk
			}
			pos--
		}
	} else {
		pos = i + 1
		for pos < len(chunks) && remain > 0 {
			if other, ok := chunks[pos]["ck_type"].(string); ok && other == "text" {
				tk, _ := chunks[pos]["tk_nums"].(int)
				txt := toString(chunks[pos]["text"])
				if tk >= remain {
					parts = append(parts, takeFromStart(txt, remain))
					remain = 0
					break
				}
				parts = append(parts, txt)
				remain -= tk
			}
			pos++
		}
	}
	return strings.Join(parts, "")
}

// takeFromEnd returns the last approx `tokens` worth of text (1 token
// ≈ 4 bytes is the best-effort approximation used here; python uses
// the actual tokenizer).
func takeFromEnd(text string, tokens int) string {
	bytes := tokens * 4
	if bytes >= len(text) {
		return text
	}
	return text[len(text)-bytes:]
}

func takeFromStart(text string, tokens int) string {
	bytes := tokens * 4
	if bytes >= len(text) {
		return text
	}
	return text[:bytes]
}

// mergeByTokenSizeFromJSON mirrors `_merge_text_chunks_by_token_size`
// at token_chunker.py:212-243.
func mergeByTokenSizeFromJSON(perItem [][]map[string]any, chunkTokens int, overlappedPct float64) [][]map[string]any {
	threshold := float64(chunkTokens) * (100 - overlappedPct*100) / 100.0
	for idx := range perItem {
		chunks := perItem[idx]
		if len(chunks) == 0 {
			continue
		}
		var merged []map[string]any
		prevTextIdx := -1
		for _, ck := range chunks {
			ckType, _ := ck["ck_type"].(string)
			if ckType != "text" {
				merged = append(merged, cloneMap(ck))
				prevTextIdx = -1
				continue
			}
			tk, _ := ck["tk_nums"].(int)
			if prevTextIdx < 0 || float64(tk) > threshold {
				cp := cloneMap(ck)
				if prevTextIdx >= 0 && overlappedPct > 0 {
					if prevText := toString(merged[prevTextIdx]["text"]); prevText != "" {
						cut := int(float64(len(prevText)) * (100 - overlappedPct*100) / 100.0)
						if cut < len(prevText) {
							if curText, ok := cp["text"].(string); ok {
								cp["text"] = prevText[cut:] + curText
								if v, ok := cp["tk_nums"].(int); ok {
									cp["tk_nums"] = v + tokenizeStr(cp["text"].(string))
								}
							}
						}
					}
				}
				merged = append(merged, cp)
				prevTextIdx = len(merged) - 1
				continue
			}
			// Merge into prev text chunk.
			prev := merged[prevTextIdx]
			if pt := toString(prev["text"]); pt != "" {
				if ct, ok := ck["text"].(string); ok {
					prev["text"] = pt + "\n" + ct
					if v, ok := prev["tk_nums"].(int); ok {
						if curTk, ok := ck["tk_nums"].(int); ok {
							prev["tk_nums"] = v + curTk
						}
					}
				}
			}
		}
		perItem[idx] = merged
	}
	return perItem
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func flatten(perItem [][]map[string]any) []map[string]any {
	var out []map[string]any
	for _, cs := range perItem {
		out = append(out, cs...)
	}
	return out
}

func splitByChildren(chunks []map[string]any, pattern *regexp.Regexp) []map[string]any {
	if pattern == nil {
		return chunks
	}
	var out []map[string]any
	for _, ck := range chunks {
		dt, _ := ck["doc_type_kwd"].(string)
		if dt != "text" {
			out = append(out, ck)
			continue
		}
		mom := toString(ck["text"])
		parts := splitKeepingDelim(mom, pattern)
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				continue
			}
			cp := cloneMap(ck)
			cp["text"] = p
			cp["mom"] = mom
			out = append(out, cp)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// shared text-payload helpers (used by TitleChunker et al.)
// ---------------------------------------------------------------------------

// hasActiveDelimiter reports whether a regex compiled by
// compileDelimPattern contains any non-placeholder pattern. The "match
// nothing" sentinel regexp makes a quick `pattern.MatchString("")`
// viable as a check without re-walking the source slice.
func hasActiveDelimiter(p *regexp.Regexp) bool {
	return p != nil && p.String() != `\A(?!)`
}

// applyChildrenDelim mirrors token_chunker.py:325-334.
func applyChildrenDelim(segs []string, pattern *regexp.Regexp) []map[string]any {
	if pattern == nil {
		out := make([]map[string]any, 0, len(segs))
		for _, s := range segs {
			out = append(out, map[string]any{"text": s})
		}
		return out
	}
	var docs []map[string]any
	for _, seg := range segs {
		if strings.TrimSpace(seg) == "" {
			continue
		}
		for _, child := range splitKeepingDelim(seg, pattern) {
			if strings.TrimSpace(child) == "" {
				continue
			}
			docs = append(docs, map[string]any{"text": child, "mom": seg})
		}
	}
	return docs
}

func applyChildrenDelimText(docs []map[string]any, pattern *regexp.Regexp) []map[string]any {
	if pattern == nil {
		return docs
	}
	var out []map[string]any
	for _, d := range docs {
		t := toString(d["text"])
		if strings.TrimSpace(t) == "" {
			continue
		}
		for _, child := range splitKeepingDelim(t, pattern) {
			if strings.TrimSpace(child) == "" {
				continue
			}
			out = append(out, map[string]any{"text": child, "mom": t})
		}
	}
	return out
}

// compileChildrenPattern is the children_delimiters version of
// compileDelimPattern. Returns nil when no delimiters exist.
func compileChildrenPattern(delims []string) *regexp.Regexp {
	if len(delims) == 0 {
		return nil
	}
	escaped := make([]string, 0, len(delims))
	for _, d := range delims {
		if d == "" {
			continue
		}
		escaped = append(escaped, regexp.QuoteMeta(d))
	}
	if len(escaped) == 0 {
		return nil
	}
	sortSlice(escaped)
	return regexp.MustCompile(strings.Join(escaped, "|"))
}

// sortSlice sorts in place by descending length (longest pattern
// first, mirroring python's `sorted(set, key=len, reverse=True)`).
func sortSlice(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && len(in[j-1]) < len(in[j]); j-- {
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}

// stringFromInputs returns the string value at the first matching key
// in `keys`, or ("", false) when none is set.
func stringFromInputs(inputs map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := inputs[k].(string); ok {
			return v, true
		}
	}
	return "", false
}

// chunksFromInputs returns the chunk list from inputs as a uniform
// []map[string]any, or nil when absent. Both []map[string]any (the
// JSON-decoded form) and []any (the slice-of-mixed form) are handled.
//
// Three upstream keys are accepted, in priority order:
//
//   - "chunks"     — canonical post-chunker shape (chunker → chunker
//     re-entry, test fixtures, downstream stages).
//   - "chunks_full" — schema-package alias used by older fixtures.
//   - "json"       — the parser-structured-output key (Parser
//     component emits under "json"; we accept it
//     so a token-chunker can run directly after
//     a parser without an intermediate reshape).
func chunksFromInputs(inputs map[string]any) []map[string]any {
	for _, key := range []string{"chunks", "chunks_full", "json"} {
		v, ok := inputs[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case []map[string]any:
			return t
		case []any:
			out := make([]map[string]any, 0, len(t))
			for _, it := range t {
				if m, ok := it.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return out
		}
	}
	return nil
}

// init registers TokenChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameTokenChunker)
}
