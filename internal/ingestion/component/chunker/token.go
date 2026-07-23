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

// SCOPE (honest) for token.go:
//
//   - WHITELIST: delimiter_mode ∈ {"token_size","delimiter"} (the
//     single-chunk "one" behaviour moved to OneChunker in one.go).
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
//   - MODE "delimiter" uses the regex-aware delimiter pattern to split
//     text into segments; unlike token_size, these segments are NOT
//     merged — they become standalone chunks.
//
//   - MODE "token_size" implements Python's naive_merge split-then-
//     merge: segments are split by the configured delimiter pattern
//     (chunkFromItem), then greedily merged to chunk_token_size with
//     optional overlap (mergeByTokenSizeFromJSON). The JSON and text
//     payload paths share the same merge after splitting.
//
//   - JSON-STRUCTURED INPUT (output_format == "json", or the default
//     parser-style branch when output_format is unset) is normalized
//     into the same internal chunk shape via a parallel fan-out.
//     Media-context attachment is per-item sequential; merge is
//     index-deterministic.
//
//   - No PDF/outline awareness (Python `restore_pdf_text_previews`).
//     That depends on deepdoc/parser which is out of scope for this
//     phase; the chunker accepts the parser-style structured JSON
//     payload and runs the same logic against it.
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"ragflow/internal/agent/runtime"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
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

// Inputs is exposed so callers can introspect.
func (c *TokenChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

// Outputs is exposed so callers can introspect.
func (c *TokenChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

// Invoke runs the chunker against the input payload.
//
// Concurrency: text payloads are fanned across 4 goroutines by
// primary-delimiter segment; structured JSON/chunks payloads fan
// across items. Merge is by input index (plan §8 R8): the i-th
// goroutine's output occupies slot i, regardless of completion order.
//
// Timeout: honours ctx cancellation only — there is no inner @timeout
// decorator equivalent (plan §8 R1).
func (c *TokenChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *TokenChunkerComponent) invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	// `name` lives in the workflow-wide Globals bag (seeded at pipeline
	// start, published by the File component), not in the upstream output
	// map. decodeChunkerFromUpstream validates it, so carry the resolved
	// name into the decode input.
	name := globals.GlobalOrInput(ctx, inputs, "name", "")
	decInputs := inputs
	if name != "" {
		decInputs = cloneInputs(inputs)
		decInputs["name"] = name
	}
	upstream, err := decodeChunkerFromUpstream(decInputs)
	if err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        fmt.Sprintf("Input error: %v", err),
		}, nil
	}

	delimPattern := compileDelimPattern(c.param.Delimiters)
	childrenPattern := compileChildrenPattern(c.param.ChildrenDelimiters)

	switch upstream.OutputFormat {
	case schema.PayloadFormatMarkdown:
		if upstream.MarkdownResult == nil {
			return emptyOutputs(), nil
		}
		return c.invokeTextPayload(ctx, *upstream.MarkdownResult, delimPattern, childrenPattern), nil
	case schema.PayloadFormatText:
		if upstream.TextResult == nil {
			return emptyOutputs(), nil
		}
		return c.invokeTextPayload(ctx, *upstream.TextResult, delimPattern, childrenPattern), nil
	case schema.PayloadFormatHTML:
		if upstream.HTMLResult == nil {
			return emptyOutputs(), nil
		}
		return c.invokeTextPayload(ctx, *upstream.HTMLResult, delimPattern, childrenPattern), nil
	default:
		// Port of token_chunker.py:347 — when the upstream emitted
		// chunks (output_format == "chunks", e.g. a TitleChunker
		// feeding into this TokenChunker), consume those chunks rather
		// than the raw parser json_result. Otherwise fall back to the
		// structured json_result. This fixes #16812 where a
		// TitleChunker → TokenChunker chain silently discarded the
		// chapter-level chunks and re-chunked the raw parser output.
		var items []schema.ChunkDoc
		if upstream.OutputFormat == schema.PayloadFormatChunks {
			items = upstream.Chunks
		} else {
			items = upstream.JSONResult
		}

		// Re-acquire the source PDF (if the Parser forwarded storage
		// refs) so image/table sections are cropped on demand rather
		// than carried through the wire. Best-effort: a nil engine
		// simply skips cropping.
		engine, engErr := newPDFEngineFromUpstream(ctx, upstream)
		if engErr != nil {
			slog.Warn("TokenChunker: could not open PDF for on-demand cropping", "err", engErr)
		}
		if engine != nil {
			defer engine.Close()
		}
		return c.invokeJSONPayload(ctx, items, delimPattern, childrenPattern, engine), nil
	}
}

func decodeChunkerFromUpstream(inputs map[string]any) (schema.ChunkerFromUpstream, error) {
	var out schema.ChunkerFromUpstream
	data, err := json.Marshal(stripChunkerRuntimeTimestamps(inputs))
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	return out, nil
}

func stripChunkerRuntimeTimestamps(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		if k == "_created_time" || k == "_elapsed_time" {
			continue
		}
		out[k] = v
	}
	return out
}

// invokeTextPayload handles plain-text input (output_format in
// {markdown,text,html} on the python side).
func (c *TokenChunkerComponent) invokeTextPayload(_ context.Context, text string, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	if text == "" {
		return emptyOutputs()
	}

	if !hasActiveDelimiter(delimPattern) {
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

	// Python's naive_merge: custom (backtick) delimiters produce one
	// chunk per segment — no token-size merge (naive_merge:1194-1213).
	if hasCustomDelim(c.param.Delimiters) {
		return chunkOutputs(docs)
	}

	// Split-then-merge: split on delimiters, then greedily merge to
	// chunk_token_size with optional overlap.
	perItem := [][]schema.ChunkDoc{docs}
	merged := mergeByTokenSizeFromJSON(perItem, c.param.ChunkTokenSize, c.param.OverlappedPercent)
	return chunkOutputs(flatten(merged))
}

// mergeByTokenSize implements exact token-based chunk merging that mirrors
// Python's naive_merge (rag/nlp/__init__.py:1156). It uses
// tokenizeStr (= tokenizer.NumTokensFromString, cl100k_base BPE) for
// precise token counting, splits input into paragraph sections, further
// subdivides oversized sections on sentence delimiters, and greedily
// merges into chunks of approximately chunk_token_size tokens with
// optional overlap from the previous chunk.
func (c *TokenChunkerComponent) mergeByTokenSize(text string, childrenPattern *regexp.Regexp) map[string]any {
	target := c.param.ChunkTokenSize
	overlapPct := c.param.OverlappedPercent

	// Split into paragraph-aligned sections.
	sections := splitIntoSections(text)
	if len(sections) == 0 {
		return emptyOutputs()
	}

	// Sentence/clause-boundary regex for splitting oversized sections.
	// Matches Python's default delimiter "\n。；！？" plus English ". " fallback.
	sentenceDelim := regexp.MustCompile(`(\n|[。；！？]|\.\s)`)

	var cks []string // chunk texts
	var tkns []int   // token counts per chunk

	// mergeOrNew mirrors Python add_chunk in naive_merge:
	//   - If the current chunk is empty or would overflow the
	//     threshold → start a new chunk (with optional overlap prefix).
	//   - Otherwise → merge into the current chunk.
	mergeOrNew := func(segment string, tokens int) {
		threshold := float64(target) * (100 - overlapPct*100) / 100.0
		if len(cks) == 0 || float64(tkns[len(tkns)-1]) > threshold {
			seg := segment
			segTokens := tokens
			if overlapPct > 0 && len(cks) > 0 {
				prev := cks[len(cks)-1]
				// Take the last overlapped_percent of the previous chunk
				// (in runes, matching Python's len(overlapped) * ratio).
				prevRunes := []rune(prev)
				cut := int(float64(len(prevRunes)) * (100 - overlapPct*100) / 100.0)
				if cut < len(prevRunes) {
					suffix := string(prevRunes[cut:])
					seg = suffix + segment
					segTokens = tokenizeStr(seg)
				}
			}
			cks = append(cks, seg)
			tkns = append(tkns, segTokens)
		} else {
			cks[len(cks)-1] += segment
			tkns[len(tkns)-1] += tokens
		}
	}

	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}
		t := "\n" + sec
		tn := tokenizeStr(t)

		if tn < 8 {
			// Tiny section — always merge into the previous chunk.
			if len(cks) > 0 {
				cks[len(cks)-1] += t
				tkns[len(tkns)-1] += tn
			} else {
				cks = append(cks, t)
				tkns = append(tkns, tn)
			}
			continue
		}

		if tn <= target {
			mergeOrNew(t, tn)
			continue
		}

		// Oversized section: split on sentence delimiters, then merge.
		parts := sentenceDelim.Split(sec, -1)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			p := "\n" + part
			pn := tokenizeStr(p)
			if pn < 8 {
				if len(cks) > 0 {
					cks[len(cks)-1] += p
					tkns[len(tkns)-1] += pn
				} else {
					cks = append(cks, p)
					tkns = append(tkns, pn)
				}
				continue
			}
			mergeOrNew(p, pn)
		}
	}

	docs := make([]schema.ChunkDoc, 0, len(cks))
	for _, ch := range cks {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		docs = append(docs, schema.ChunkDoc{Text: ch})
	}
	final := applyChildrenDelimText(docs, childrenPattern)
	return chunkOutputs(final)
}

// splitIntoSections partitions text into paragraph-level sections by
// splitting on double-newline boundaries. This mirrors the caller side
// in Python's naive_merge where sections are pre-split before merging.
func splitIntoSections(text string) []string {
	if text == "" {
		return nil
	}
	// Split on consecutive newlines (paragraph boundaries).
	re := regexp.MustCompile(`\n\s*\n`)
	parts := re.Split(text, -1)
	return parts
}

// invokeJSONPayload handles structured upstream input. Items fan
// across 4 goroutines; merge is by input index.
func (c *TokenChunkerComponent) invokeJSONPayload(ctx context.Context, items []schema.ChunkDoc, delimPattern, childrenPattern *regexp.Regexp, engine deepdoctype.PDFEngine) map[string]any {
	if len(items) == 0 {
		return emptyOutputs()
	}
	workers := 4
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) {
		workers = len(items)
	}
	lanes := partition(len(items), workers)
	perItem := make([][]schema.ChunkDoc, len(items))

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
	if err := ctx.Err(); err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        fmt.Sprintf("TokenChunker: %v", err),
		}
	}

	// Attach surrounding media context (token_chunker.py:358).
	attached := attachMediaContext(perItem, c.param.TableContextSize, c.param.ImageContextSize)

	// Python's naive_merge: custom (backtick) delimiters produce one
	// chunk per segment — no token-size merge (naive_merge:1194-1213).
	// Otherwise split-then-merge: delimiter-split segments are greedily
	// merged to chunk_token_size with optional overlap.
	if !hasCustomDelim(c.param.Delimiters) {
		attached = mergeByTokenSizeFromJSON(attached, c.param.ChunkTokenSize, c.param.OverlappedPercent)
	}

	flat := flatten(attached)
	if childrenPattern != nil {
		flat = splitByChildren(flat, childrenPattern)
	}

	// Crop image/table chunks on demand when a PDF engine is available.
	flat = cropImageChunks(ctx, engine, flat)

	out := make([]schema.ChunkDoc, 0, len(flat))
	for _, m := range flat {
		if strings.TrimSpace(m.Text) == "" {
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return emptyOutputs()
	}
	return chunkOutputs(out)
}

// ---------------------------------------------------------------------------
// JSON-payload internals
// ---------------------------------------------------------------------------

// chunkFromItem mirrors _build_json_chunks for a single item.
func chunkFromItem(it schema.ChunkDoc, delimPattern *regexp.Regexp) []schema.ChunkDoc {
	ckType := itemDocType(it)
	txt := itemTextOrFallback(it)
	if ckType != "text" {
		return []schema.ChunkDoc{buildChunkDoc(it, ckType, txt, "", "")}
	}
	if !hasActiveDelimiter(delimPattern) {
		return []schema.ChunkDoc{buildChunkDoc(it, "text", txt, "", "")}
	}
	parts := splitKeepingDelim(txt, delimPattern)
	if !delimPattern.MatchString(txt) {
		return []schema.ChunkDoc{buildChunkDoc(it, "text", txt, "", "")}
	}
	out := make([]schema.ChunkDoc, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, buildChunkDoc(it, "text", p, "", ""))
	}
	if len(out) == 0 {
		return []schema.ChunkDoc{buildChunkDoc(it, "text", txt, "", "")}
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
func buildChunkDoc(it schema.ChunkDoc, ckType, text, ctxAbove, ctxBelow string) schema.ChunkDoc {
	out := schema.ChunkDoc{
		Text:         text,
		DocType:      ckType,
		CKType:       ckType,
		TKNums:       intPtr(tokenizeStr(text)),
		Mom:          it.Mom,
		ImgID:        it.ImgID,
		Layout:       it.Layout,
		PDFPositions: it.PDFPositions,
		Positions:    it.Positions,
		Image:        it.Image,
		PageNumber:   it.PageNumber,
	}
	if ctxAbove != "" {
		out.ContextAbove = ctxAbove
	}
	if ctxBelow != "" {
		out.ContextBelow = ctxBelow
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

func attachMediaContext(perItem [][]schema.ChunkDoc, tableCtx, imageCtx int) [][]schema.ChunkDoc {
	if tableCtx <= 0 && imageCtx <= 0 {
		return perItem
	}
	for idx := range perItem {
		chunks := perItem[idx]
		if len(chunks) == 0 {
			continue
		}
		for i, ck := range chunks {
			ckType := ck.CKType
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
			chunks[i].ContextAbove = collectContext(chunks, i, ctx, true)
			chunks[i].ContextBelow = collectContext(chunks, i, ctx, false)
		}
	}
	return perItem
}

// collectContext walks chunks around `i` (above when direction==true,
// below when false), pulling text chunks while remaining token budget
// stays positive. Matches token_chunker.py:_attach_context_to_media_chunks.
func collectContext(chunks []schema.ChunkDoc, i, ctxTokens int, above bool) string {
	var parts []string
	remain := ctxTokens
	var pos int
	if above {
		pos = i - 1
		for pos >= 0 && remain > 0 {
			if chunks[pos].CKType == "text" {
				tk := intValue(chunks[pos].TKNums)
				txt := chunks[pos].Text
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
			if chunks[pos].CKType == "text" {
				tk := intValue(chunks[pos].TKNums)
				txt := chunks[pos].Text
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

// mergeByTokenSizeFromJSON mirrors `naive_merge` at
// rag/nlp/__init__.py:1156.
func mergeByTokenSizeFromJSON(perItem [][]schema.ChunkDoc, chunkTokens int, overlappedPct float64) [][]schema.ChunkDoc {
	threshold := float64(chunkTokens) * (100 - overlappedPct*100) / 100.0
	for idx := range perItem {
		chunks := perItem[idx]
		if len(chunks) == 0 {
			continue
		}
		var merged []schema.ChunkDoc
		for _, ck := range chunks {
			ckType := ck.CKType
			if ckType != "text" {
				merged = append(merged, cloneChunkDoc(ck))
				continue
			}
			tk := intValue(ck.TKNums)
			// Mirror Python's naive_merge.add_chunk: start a new chunk
			// when either (a) this is the first text chunk, or
			// (b) the currently accumulated chunk already exceeds the
			// threshold (not the incoming segment).
			if len(merged) == 0 || merged[len(merged)-1].CKType != "text" ||
				float64(intValue(merged[len(merged)-1].TKNums)) > threshold {
				cp := cloneChunkDoc(ck)
				// Overlap: prepend tail of previous chunk onto the new
				// chunk, matching Python's
				//   t = overlapped[overlap_cut:] + t
				//   tnum = num_tokens_from_string(t)
				if len(merged) > 0 && merged[len(merged)-1].CKType == "text" && overlappedPct > 0 {
					if prevText := merged[len(merged)-1].Text; prevText != "" {
						runes := []rune(prevText)
						cut := int(float64(len(runes)) * (100 - overlappedPct*100) / 100.0)
						if cut < len(runes) {
							cp.Text = string(runes[cut:]) + cp.Text
							cp.TKNums = intPtr(tokenizeStr(cp.Text))
						}
					}
				}
				merged = append(merged, cp)
				continue
			}
			// Merge into the accumulated text chunk.
			prev := &merged[len(merged)-1]
			if prev.Text != "" {
				prev.Text = prev.Text + "\n" + ck.Text
				prev.TKNums = intPtr(intValue(prev.TKNums) + tk)
			}
		}
		perItem[idx] = merged
	}
	return perItem
}

func cloneChunkDoc(in schema.ChunkDoc) schema.ChunkDoc {
	out := in
	if in.TKNums != nil {
		v := *in.TKNums
		out.TKNums = &v
	}
	if in.ChunkOrderInt != nil {
		v := *in.ChunkOrderInt
		out.ChunkOrderInt = &v
	}
	if in.PageNumber != nil {
		v := *in.PageNumber
		out.PageNumber = &v
	}
	if in.Extra != nil {
		out.Extra = make(map[string]json.RawMessage, len(in.Extra))
		for k, v := range in.Extra {
			out.Extra[k] = append(json.RawMessage(nil), v...)
		}
	}
	return out
}

func flatten(perItem [][]schema.ChunkDoc) []schema.ChunkDoc {
	var out []schema.ChunkDoc
	for _, cs := range perItem {
		out = append(out, cs...)
	}
	return out
}

func splitByChildren(chunks []schema.ChunkDoc, pattern *regexp.Regexp) []schema.ChunkDoc {
	if pattern == nil {
		return chunks
	}
	var out []schema.ChunkDoc
	for _, ck := range chunks {
		if ck.DocType != "text" {
			out = append(out, ck)
			continue
		}
		mom := ck.Text
		parts := splitKeepingDelim(mom, pattern)
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				continue
			}
			cp := cloneChunkDoc(ck)
			cp.Text = p
			cp.Mom = mom
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

// hasCustomDelim reports whether any delimiter uses backtick syntax
// (`pattern`). Python's naive_merge skips token-size merging when
// custom delimiters are present (naive_merge:1194-1213).
func hasCustomDelim(delims []string) bool {
	for _, d := range delims {
		if strings.HasPrefix(d, "`") && strings.HasSuffix(d, "`") && len(d) >= 2 {
			return true
		}
	}
	return false
}

// applyChildrenDelim mirrors token_chunker.py:325-334.
func applyChildrenDelim(segs []string, pattern *regexp.Regexp) []schema.ChunkDoc {
	if pattern == nil {
		out := make([]schema.ChunkDoc, 0, len(segs))
		for _, s := range segs {
			out = append(out, schema.ChunkDoc{
				Text:    s,
				DocType: "text",
				CKType:  "text",
			})
		}
		return out
	}
	var docs []schema.ChunkDoc
	for _, seg := range segs {
		if strings.TrimSpace(seg) == "" {
			continue
		}
		for _, child := range splitKeepingDelim(seg, pattern) {
			if strings.TrimSpace(child) == "" {
				continue
			}
			docs = append(docs, schema.ChunkDoc{Text: child, Mom: seg})
		}
	}
	return docs
}

func applyChildrenDelimText(docs []schema.ChunkDoc, pattern *regexp.Regexp) []schema.ChunkDoc {
	if pattern == nil {
		return docs
	}
	var out []schema.ChunkDoc
	for _, d := range docs {
		t := d.Text
		if strings.TrimSpace(t) == "" {
			continue
		}
		for _, child := range splitKeepingDelim(t, pattern) {
			if strings.TrimSpace(child) == "" {
				continue
			}
			out = append(out, schema.ChunkDoc{Text: child, Mom: t})
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
// Two upstream keys are accepted, in priority order:
//
//   - "chunks" — canonical post-chunker shape (chunker → chunker
//     re-entry, test fixtures, downstream stages).
//   - "json"   — the parser-structured-output key (Parser
//     component emits under "json"; we accept it
//     so a token-chunker can run directly after
//     a parser without an intermediate reshape).
func chunksFromInputs(inputs map[string]any) []schema.ChunkDoc {
	for _, key := range []string{"chunks", "json"} {
		v, ok := inputs[key]
		if !ok {
			continue
		}
		chunks, found, err := schema.ChunkDocsFromAny(v)
		if err == nil && found {
			return chunks
		}
	}
	return nil
}

func intValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func intPtr(v int) *int { return &v }

// init registers TokenChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameTokenChunker)
}
