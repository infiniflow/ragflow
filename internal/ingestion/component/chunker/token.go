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
//   - WHITELIST: delimiter_mode ∈ {"delimiter"} (the
//     single-chunk "one" behaviour moved to OneChunker in one.go).
//     chunk_token_size > 0, overlapped_percent accepts a [0,1) fraction or a
//     [0,90] percentage (normalized to [0,90] by normalizeOverlappedPercent,
//     mirroring Python's normalize_overlapped_percent), table_context_size ≥ 0,
//     image_context_size ≥ 0. enum/range checks live in param.Check.
//
//   - DELIMITER PARSING for the TokenChunker list API mirrors Python
//     token_chunker: only entries wrapped in backticks (e.g. "`\\n\\n`")
//     produce an active split pattern. Plain list entries are not
//     compiled into the pattern. (The single-string parser_config.delimiter
//     field is not parsed in Go; only the []string list API is consumed.)
//
//   - CHILDREN DELIMITERS (the secondary split) is implemented via the
//     splitDroppingDelim helper; emitted chunks carry the parent
//     ("mom") and the split child ("text") keys, with the delimiter dropped.
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
//   - PDF text previews (Python `restore_pdf_text_previews`) are
//     generated on demand for text chunks that carry PDF positions:
//     cropImageChunks crops the text region and writes a preview image,
//     then imageUploadDecorator uploads it to img_id. See pdfcrop_cgo.go.
//
//   - OVER-BUDGET UNITS (contract #17799): a single item that exceeds
//     chunk_token_size is KEPT WHOLE as its own chunk and is NOT
//     atom-split; the embedding/rerank layer truncates it later. The
//     TokenChunker must never sub-split a single item (the naive_merge
//     invariant). If oversized-unit handling is ever needed to avoid a
//     single mega-chunk, it belongs at the CHUNKER side (or a dedicated
//     PreSplitter stage between Parser and Chunker), fed by an EXPLICIT
//     token budget + tokenizer — NOT in the parser, and NOT as a
//     char-window atom-split. The earlier splitOversizedUnit /
//     splitAtomByTokenBudget helpers were a misplaced (parser-layer logic
//     wrongly living in the chunker) and unwired vestige; they were
//     removed to align with this contract.
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"

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
	if v, ok := schema.NumericFromAny(conf["chunk_token_size"]); ok {
		p.TokenChunkerParam.ChunkTokenSize = int(v)
	}
	if v, ok := conf["delimiters"].([]any); ok {
		p.TokenChunkerParam.Delimiters = stringListFromAny(v)
	} else if v, ok := conf["delimiters"].([]string); ok {
		p.TokenChunkerParam.Delimiters = append([]string(nil), v...)
	}
	if v, ok := conf["overlapped_percent"]; ok {
		p.TokenChunkerParam.OverlappedPercent = schema.NormalizeOverlappedPercent(v)
	}
	if v, ok := conf["children_delimiters"].([]any); ok {
		p.TokenChunkerParam.ChildrenDelimiters = stringListFromAny(v)
	} else if v, ok := conf["children_delimiters"].([]string); ok {
		p.TokenChunkerParam.ChildrenDelimiters = append([]string(nil), v...)
	}
	if v, ok := schema.NumericFromAny(conf["table_context_size"]); ok {
		p.TokenChunkerParam.TableContextSize = int(v)
	}
	if v, ok := schema.NumericFromAny(conf["image_context_size"]); ok {
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
func (c *TokenChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, db, inputs)
}

func (c *TokenChunkerComponent) invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
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
		engine, engErr := newPDFEngineFromUpstream(ctx, db, upstream)
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

// cropTitleChunks crops image/table/text previews for chunks produced by
// the Title/Group/Hierarchy chunkers, mirroring the TokenChunker JSON path
// (cropImageChunks at token.go:513). A nil engine — or an
// empty chunk list — leaves chunks unchanged (best-effort, matching the
// on-demand PDF crop contract used by the TokenChunker path).
func cropTitleChunks(ctx context.Context, engine deepdoctype.PDFEngine, chunks []map[string]any) []map[string]any {
	if engine == nil || len(chunks) == 0 {
		return chunks
	}
	docs, _, err := schema.ChunkDocsFromAny(chunks)
	if err != nil || len(docs) == 0 {
		return chunks
	}
	// The Title/Group/Hierarchy chunkers emit doc_type_kwd but not the
	// ck_type field that cropImageChunks' needsCrop consults
	// (pdfcrop_cgo.go:151). Derive ck_type from doc_type_kwd so the crop
	// decision matches the TokenChunker path. The derived ck_type is
	// stripped from the returned maps so the downstream chunk shape is
	// unchanged (setting ck_type in the real output would also change
	// how a downstream TokenChunker merges these chunks — a separate
	// concern, out of scope here).
	for i := range docs {
		if docs[i].CKType == "" {
			switch docs[i].DocType {
			case "image", "table":
				docs[i].CKType = docs[i].DocType
			default:
				docs[i].CKType = "text"
			}
		}
	}
	cropped := cropImageChunks(ctx, engine, docs)
	out := schema.ChunkDocsToMaps(cropped)
	for _, m := range out {
		delete(m, "ck_type")
	}
	return out
}

// invokeTextPayload handles plain-text input (output_format in
// {Markdown,text,html} on the python side).
func (c *TokenChunkerComponent) invokeTextPayload(_ context.Context, text string, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	if text == "" {
		return emptyOutputs()
	}

	// No active delimiter at all: single-section merge that only re-splits
	// oversized sections on sentence boundaries.
	if !hasActiveDelimiter(delimPattern) {
		return c.mergeByTokenSize(text, nil, childrenPattern)
	}

	// Custom (backtick-wrapped) delimiter: one chunk per segment, no token
	// merge — mirrors Python naive_merge's has_custom branch
	// (token_chunker.py:1194-1213).
	if hasCustomDelim(c.param.Delimiters) {
		return c.chunkPerSegment(text, delimPattern, childrenPattern)
	}

	// Bare delimiter: split into paragraphs (delimiter dropped), then merge
	// by token size — mirrors Python naive_merge's default branch. This is
	// the #17723 fix: bare delimiters were previously ignored entirely.
	return c.mergeByTokenSize(text, delimPattern, childrenPattern)
}

// chunkPerSegment splits text on a custom (backtick) delimiter and emits one
// chunk per segment with no token-size merge. Mirrors Python naive_merge's
// has_custom branch (token_chunker.py:1194-1213).
func (c *TokenChunkerComponent) chunkPerSegment(text string, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	parts := splitDroppingDelim(text, delimPattern)
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		// Python's text path keeps only the even-index (text) parts from
		// _split_text_by_pattern and then .strip()s each one
		// (token_chunker.py:316-338), so the delimiter is dropped and
		// surrounding whitespace is trimmed.
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) == 0 {
		return emptyOutputs()
	}
	textDocs := make([]schema.ChunkDoc, 0, len(cleaned))
	for _, s := range cleaned {
		textDocs = append(textDocs, schema.ChunkDoc{Text: s, DocType: "text", CKType: "text"})
	}
	docs := applyChildrenDelimText(textDocs, childrenPattern)
	return chunkOutputs(docs)
}

// sentenceDelimiter is the token-chunker TEXT-path sentence delimiter (the
// naive_merge port). It mirrors the delimiter Python's naive_merge receives in
// production: rag/app/naive.py passes DEFAULT_DELIMITER ("\n!?;。；！？", defined
// once in rag/nlp/delim.py), which includes ASCII "!", "?" and ";" plus the CJK
// punctuation "。；！？" but NOT an English ". " boundary.
//
// The Title chunker and the token chunker's JSON path use the shared
// sentenceBoundaryRe instead (Python _sentence_boundary.py SENTENCE_BOUNDARY_RE,
// which adds ". "). The two constants mirror two distinct Python delimiters and
// differ only in that English boundary, exactly as Python does.
var sentenceDelimiter = regexp.MustCompile(`(\n|[!?;。；！？])`)

// overlapCut returns the visible-text rune offset where the overlap prefix
// begins, mirroring the cut computed inside computeOverlapPrefix. It is split
// out so the coordinate-carrying path reuses the exact same cut as the text
// path (#18148).
func overlapCut(prevText string, overlappedPct float64) int {
	visible := removeTag(prevText)
	runes := []rune(visible)
	cut := int(float64(len(runes)) * (100.0 - overlappedPct) / 100.0)
	if cut < 0 {
		cut = 0
	}
	if cut >= len(runes) {
		return len(runes)
	}
	return cut
}

// computeOverlapPrefix returns (overlapText, overlapTokenCount) carved from
// the tail of prevText after stripping parser tags. overlappedPct is a
// percentage in [0, 100]. Mirrors Python rag/nlp._compute_overlap_prefix.
func computeOverlapPrefix(prevText string, overlappedPct float64) (string, int) {
	visible := removeTag(prevText)
	if visible == "" {
		return "", 0
	}
	runes := []rune(visible)
	cut := overlapCut(prevText, overlappedPct)
	if cut >= len(runes) {
		return "", 0
	}
	overlap := string(runes[cut:])
	return overlap, tokenizeStr(overlap)
}

// mergeByTokenSize implements exact token-based chunk merging that mirrors
// Python naive_merge (rag/nlp/__init__.py) after the strict chunk_token_num
// hard-cap fix.
// It uses tokenizeStr for precise token counting, treats the payload as a
// single section, and splits oversized sections on production sentence
// delimiters. Sections are merged with the unified core (hard-cap expansion +
// UNDER_CAP merge): oversized units are re-split (sentence boundaries first,
// hard token-split fallback) so no text chunk exceeds chunk_token_size. When
// overlap>0 the previous chunk's tail is prepended and trimmed to fit, so the
// hard cap still holds.
func (c *TokenChunkerComponent) mergeByTokenSize(text string, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	target := c.param.ChunkTokenSize
	overlapPct := c.param.OverlappedPercent
	// Clamp to [0,100] so the merge math below never produces a
	// negative/inverted threshold for an out-of-range value (review:
	// yuzhichang, PR #17396). c.param.OverlappedPercent is already in
	// [0,90] via Update/Validate, so this is a defensive no-op in
	// normal operation.
	if overlapPct < 0 {
		overlapPct = 0
	} else if overlapPct > 100 {
		overlapPct = 100
	}

	// Normalize line endings to LF before any splitting. Python's
	// naive_merge runs text.replace("\r\n", "\n").replace("\r", "\n"),
	// then treats the input string as one section.
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")

	// Build the merge units.
	//
	// When a bare (non-custom) delimiter is active we split the text into
	// paragraphs (the delimiter is DROPPED, mirroring Python naive_merge) and
	// use each paragraph VERBATIM as a unit. This preserves the original
	// inter-paragraph whitespace, so the merged text matches the source (and
	// Python naive_merge). An oversized paragraph is re-split by the hard-cap
	// expansion in mergeUnits.
	//
	// Otherwise (no active delimiter) the whole text is a single section and
	// oversized sections are re-split on production sentence delimiters, which
	// injects "\n" to mirror Python's sentence-boundary handling. This is the
	// historical behavior and must stay byte-for-byte identical.
	useDelimSplit := hasActiveDelimiter(delimPattern) && !hasCustomDelim(c.param.Delimiters)
	var units []schema.ChunkDoc
	if useDelimSplit {
		// Mirror Python naive_merge's default path (rag/nlp/__init__.py:1406-1415):
		// split on the delimiter, DROP the delimiter text, and prepend "\n" to
		// each kept paragraph. The sub-sec keeps its original surrounding
		// whitespace (e.g. the trailing space before the delimiter); only the
		// delimiter itself is removed. No sentence re-split is performed here —
		// an oversized paragraph is re-split later by the hard-cap expansion in
		// mergeUnits.
		for _, sub := range splitDroppingDelim(text, delimPattern) {
			if sub == "" {
				continue
			}
			t := "\n" + sub
			units = append(units, schema.ChunkDoc{Text: t, TKNums: intPtr(tokenizeStr(t)), CKType: "text"})
		}
	} else {
		sections := []string{text}
		for _, sec := range sections {
			sec = strings.TrimSpace(sec)
			if sec == "" {
				continue
			}
			t := "\n" + sec
			tk := tokenizeStr(t)
			if tk <= target {
				units = append(units, schema.ChunkDoc{Text: t, TKNums: intPtr(tk), CKType: "text"})
				continue
			}
			// Oversized section: split on production sentence delimiters into
			// units. A unit that still exceeds the budget is re-split by the
			// hard-cap expansion in mergeUnits (sentence boundaries first,
			// hard token-split fallback), so no chunk ever exceeds the target.
			parts := sentenceDelimiter.Split(sec, -1)
			hadPart := false
			for _, part := range parts {
				// Keep the raw split fragment, including any inter-line
				// trailing whitespace (Python's naive_merge builds each unit
				// from "\n" + sub_sec with its trailing space, never TrimSpaced).
				// Only genuinely empty fragments are skipped.
				if part == "" {
					continue
				}
				hadPart = true
				seg := "\n" + part
				units = append(units, schema.ChunkDoc{Text: seg, TKNums: intPtr(tokenizeStr(seg)), CKType: "text"})
			}
			if !hadPart {
				units = append(units, schema.ChunkDoc{Text: t, TKNums: intPtr(tokenizeStr(t)), CKType: "text"})
			}
		}
	}

	if len(units) == 0 {
		return emptyOutputs()
	}

	// Merge with the unified hard-cap core (oversized-unit expansion + UNDER_CAP
	// merge + overlap prefix trimmed to fit). joinSep is "" for the text path,
	// mirroring Python naive_merge's concatenation of adjacent paragraphs.
	merged := mergeUnits(units, target, overlapPct, "")
	docs := make([]schema.ChunkDoc, 0, len(merged))
	for _, ch := range merged {
		// Strip parser position tags from the final text:
		// the merge paths may carry @@...## markers that must not leak into
		// indexed/embedded chunk text.
		ch.Text = removeTag(strings.TrimSpace(ch.Text))
		if ch.Text == "" {
			continue
		}
		docs = append(docs, ch)
	}
	final := applyChildrenDelimText(docs, childrenPattern)
	return chunkOutputs(final)
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
		// Python _merge_text_chunks_by_token_size merges adjacent text
		// chunks across JSON items into one global token budget. Flatten the
		// per-item structure into a single sequence first so the merge is
		// global; non-text chunks still break the merge via their CKType.
		attached = mergeByTokenSizeFromJSON([][]schema.ChunkDoc{flatten(attached)}, c.param.ChunkTokenSize, c.param.OverlappedPercent)
	}

	flat := flatten(attached)
	if childrenPattern != nil {
		flat = splitByChildren(flat, childrenPattern)
	}

	// Crop image/table chunks on demand when a PDF engine is available.
	flat = cropImageChunks(ctx, engine, flat)

	out := make([]schema.ChunkDoc, 0, len(flat))
	for _, m := range flat {
		// Strip parser position tags from the final text:
		// the merge paths may carry @@...## markers that must not leak into
		// indexed/embedded chunk text. Crop above reads positions, not text,
		// so the ordering is safe.
		m.Text = removeTag(m.Text)
		if m.Text == "" {
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
	parts := splitDroppingDelim(txt, delimPattern)
	if !delimPattern.MatchString(txt) {
		return []schema.ChunkDoc{buildChunkDoc(it, "text", txt, "", "")}
	}
	// Collect non-empty parts first so we can slice positions proportionally.
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		return []schema.ChunkDoc{buildChunkDoc(it, "text", txt, "", "")}
	}
	// If the item carries PDF positions, slice them proportionally so each
	// delimiter-split piece's screenshot crops only its own region instead of
	// the whole item bbox (fix for chunk-screenshot mismatch).
	if len(it.PDFPositions) == 0 && len(it.Positions) == 0 {
		out := make([]schema.ChunkDoc, 0, len(kept))
		for _, p := range kept {
			out = append(out, buildChunkDoc(it, "text", p, "", ""))
		}
		return out
	}
	// Visible-text ratio: parser tags (@@...##) carry no visual height
	// and must not shift crop boundaries. Mirrors the overlap logic
	// which already counts on removeTag'd text.
	runCount := 0
	for _, p := range kept {
		runCount += utf8.RuneCountInString(removeTag(p))
	}
	if runCount == 0 {
		out := make([]schema.ChunkDoc, 0, len(kept))
		for _, p := range kept {
			out = append(out, buildChunkDoc(it, "text", p, "", ""))
		}
		return out
	}
	out := make([]schema.ChunkDoc, 0, len(kept))
	cumRunes := 0
	for _, p := range kept {
		pcRunes := utf8.RuneCountInString(removeTag(p))
		startRatio := float64(cumRunes) / float64(runCount)
		endRatio := float64(cumRunes+pcRunes) / float64(runCount)
		ck := buildChunkDoc(it, "text", p, "", "")
		ck.PDFPositions = slicePositionsByTextRatio(it.PDFPositions, startRatio, endRatio)
		ck.Positions = slicePositionsByTextRatio(it.Positions, startRatio, endRatio)
		if len(ck.PDFPositions) == 0 && len(it.PDFPositions) > 0 {
			ck.PDFPositions = it.PDFPositions
		}
		if len(ck.Positions) == 0 && len(it.Positions) > 0 {
			ck.Positions = it.Positions
		}
		out = append(out, ck)
		cumRunes += pcRunes
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

// takeFromEnd returns the smallest tail of text whose token count is >=
// tokens, counted exactly via tokenizeStr  The previous
// 4-bytes-per-token heuristic over-counted for CJK text.
func takeFromEnd(text string, tokens int) string {
	runes := []rune(text)
	// The tail runes[i:] grows as i decreases, so the first (largest i,
	// i.e. smallest tail) that meets the budget is the answer.
	for i := len(runes); i > 0; i-- {
		cand := string(runes[i:])
		if tokenizeStr(cand) >= tokens {
			return cand
		}
	}
	return text
}

// takeFromStart returns the smallest prefix of text whose token count is >=
// tokens, counted exactly via tokenizeStr
func takeFromStart(text string, tokens int) string {
	runes := []rune(text)
	best := text
	// Prefix grows as i increases; the first (smallest) qualifying prefix
	// is the answer.
	for i := 1; i <= len(runes); i++ {
		cand := string(runes[:i])
		if tokenizeStr(cand) >= tokens {
			best = cand
			break
		}
	}
	return best
}

// mergeUnits is the single, unified token-merge core shared by BOTH the text
// path (mergeByTokenSize) and the JSON path (mergeByTokenSizeFromJSON). It is
// a faithful port of Python rag/flow/chunker/token_chunker.py:
// _merge_text_chunks_by_token_size (the JSON strategy) and is also the target
// the Python text path (rag/nlp naive_merge) is migrating to, so the two
// languages and the two paths converge on ONE algorithm.
//
// Hard-cap contract:
//   - Every emitted text chunk is <= target tokens. Oversized units (a single
//     unit whose token count exceeds target) are expanded FIRST: sentence
//     boundaries are tried, then a hard token-split fallback guarantees each
//     piece fits the target. The merge itself is UNDER_CAP: a projected join
//     that would push the running sum over target starts a fresh chunk, so no
//     chunk ever exceeds target.
//   - The merge threshold is SCALED to reserve room for overlap:
//     threshold = target * (100 - overlap) / 100. A chunk keeps receiving
//     units while its running token sum stays <= threshold; once it exceeds
//     threshold the next unit starts a fresh chunk.
//   - When a fresh chunk starts and overlap>0, the tail of the previous chunk
//     is UNCONDITIONALLY prepended (computeOverlapPrefix already strips parser
//     tags). If the overlap prefix would push the fresh chunk over target it is
//     trimmed to fit, so the hard cap still holds with overlap enabled.
//
// Token counts use the RUNNING SUM of per-unit counts (never re-tokenizing the
// joined string), matching Python's tk_nums += current["tk_nums"] (#17948).
// As a hard-cap safety net the merge ALSO re-checks the actual joined text via
// tokenizeStr before accepting a merge: joinSep (e.g. "\n" on the JSON path)
// can add tokens beyond the running sum, so a candidate whose joined text
// would exceed target starts a fresh chunk. The running-sum bookkeeping (and
// therefore chunk TKNums) stays as-is — it matches Python and can under-report
// the joinSep tokens by a few — while the emitted text never exceeds target.
// Non-text units pass through unchanged and reset the merge run. joinSep is
// "\n" for the JSON path and "" for the text path.
// mergeItem records one source unit that contributed to a merged chunk: its
// visible text and its coordinate box groups. Tracking items (not just the
// flattened position list) lets the overlap path map a visible-text offset
// range back to the exact boxes that belong to it, so the overlap prefix
// carries only the previous chunk's tail coordinates (#18148).
type mergeItem struct {
	Text         string
	PDFPositions json.RawMessage
	Positions    json.RawMessage
}

// overlapTailPositions returns the coordinate boxes of the previous chunk's
// source items whose visible span intersects the overlap tail
// [overlapStart, total). PDF positions are per-item (coarse), so an item is
// included wholesale once any part of it falls in the overlap tail. This keeps
// the overlap prefix highlighted without over-inflating the box set with the
// previous chunk's non-overlap (head) coordinates (#18148). Offsets are in
// rune units to match computeOverlapPrefix's visible-text indexing.
func overlapTailPositions(prevItems []mergeItem, overlapStart int, joinSep string) (json.RawMessage, json.RawMessage) {
	if len(prevItems) == 0 {
		return nil, nil
	}
	// Items are concatenated with joinSep (a single "\n" for the JSON path),
	// matching the merge join at mergeUnits. Offsets are measured on the
	// TAG-FREE visible text so they line up with overlapCut/overlapFitPrefix,
	// which carve the overlap from removeTag'd text; a coordinate tag in an
	// earlier item must not shift the boundaries of later items.
	visible := make([]string, len(prevItems))
	total := 0
	for i, it := range prevItems {
		visible[i] = removeTag(it.Text)
		total += utf8.RuneCountInString(visible[i]) + utf8.RuneCountInString(joinSep)
	}
	total -= utf8.RuneCountInString(joinSep)
	var pdfAcc, posAcc json.RawMessage
	offset := 0
	for i, it := range prevItems {
		start := offset
		end := offset + utf8.RuneCountInString(visible[i])
		if start < total && end > overlapStart {
			pdfAcc = extendRawJSONArray(pdfAcc, it.PDFPositions)
			posAcc = extendRawJSONArray(posAcc, it.Positions)
		}
		offset = end + utf8.RuneCountInString(joinSep)
	}
	return pdfAcc, posAcc
}

func mergeUnits(units []schema.ChunkDoc, target int, overlapPct float64, joinSep string) []schema.ChunkDoc {
	if overlapPct < 0 {
		overlapPct = 0
	} else if overlapPct > 100 {
		overlapPct = 100
	}
	// Expand oversized text units into <= target pieces BEFORE merging so the
	// UNDER_CAP merge can never exceed target (hard cap). Non-text units pass
	// through unchanged.
	units = expandOversizedUnits(units, target)
	// Scaled threshold reserves room for the unconditional overlap prefix.
	threshold := float64(target) * (100.0 - overlapPct) / 100.0

	merged := make([]schema.ChunkDoc, 0, len(units))
	// mergedItems parallels merged: for each merged chunk, the source items it
	// was built from. Tracking items (not just the flattened position list)
	// lets the overlap path map a visible-text offset range back to the exact
	// boxes that belong to it, so the overlap prefix carries only the previous
	// chunk's tail coordinates (#18148).
	mergedItems := make([][]mergeItem, 0, len(units))
	prevIdx := -1
	for i := range units {
		ck := units[i]
		if ck.CKType != "text" {
			merged = append(merged, cloneChunkDoc(ck))
			mergedItems = append(mergedItems, nil)
			prevIdx = -1
			continue
		}
		tk := intValue(ck.TKNums)
		if tk <= 0 {
			tk = tokenizeStr(ck.Text)
		}
		cur := mergeItem{Text: ck.Text, PDFPositions: ck.PDFPositions, Positions: ck.Positions}
		if prevIdx < 0 {
			// First text chunk (or first after a non-text chunk): no prior
			// text to overlap with.
			cp := cloneChunkDoc(ck)
			cp.TKNums = intPtr(tk)
			merged = append(merged, cp)
			mergedItems = append(mergedItems, []mergeItem{cur})
			prevIdx = len(merged) - 1
			continue
		}
		prev := &merged[prevIdx]
		// UNDER_CAP: a projected join that would exceed target starts a fresh
		// chunk. The scaled threshold reserves room for the overlap prefix.
		startNew := float64(intValue(prev.TKNums)) > threshold || intValue(prev.TKNums)+tk > target
		if !startNew {
			// The hard cap must hold on the ACTUAL joined text: joinSep (e.g.
			// "\n" on the JSON path) can add tokens beyond the running sum, so
			// re-check the candidate. The running-sum decision and bookkeeping
			// stay unchanged (Python parity #17948); this guard only splits
			// when the joined text itself would exceed target.
			candidate := prev.Text + ck.Text
			if prev.Text != "" && ck.Text != "" {
				candidate = prev.Text + joinSep + ck.Text
			}
			if tokenizeStr(candidate) > target {
				startNew = true
			}
		}
		if startNew {
			cp := cloneChunkDoc(ck)
			if overlapPct > 0 && prev.Text != "" {
				// Unconditional overlap prefix (mirrors Python JSON). The
				// prefix is a duplicate of prev's tail; carry prev's tail
				// coordinates so the overlap region is highlighted (#18148),
				// then keep only cur's coordinates for the non-overlap part.
				// If the prefix would push the fresh chunk over target it is
				// trimmed to fit so the hard cap still holds.
				overlap, _ := computeOverlapPrefix(prev.Text, overlapPct)
				overlap, trimRunes := overlapFitPrefix(overlap, cp.Text, target)
				pdfTail, posTail := overlapTailPositions(mergedItems[prevIdx], overlapCut(prev.Text, overlapPct)+trimRunes, joinSep)
				cp.Text = overlap + cp.Text
				cp.PDFPositions = extendRawJSONArray(pdfTail, cp.PDFPositions)
				cp.Positions = extendRawJSONArray(posTail, cp.Positions)
				cp.TKNums = intPtr(tokenizeStr(cp.Text))
				merged = append(merged, cp)
				mergedItems = append(mergedItems, []mergeItem{{Text: cp.Text, PDFPositions: cp.PDFPositions, Positions: cp.Positions}})
				prevIdx = len(merged) - 1
				continue
			}
			cp.TKNums = intPtr(tk)
			merged = append(merged, cp)
			mergedItems = append(mergedItems, []mergeItem{cur})
			prevIdx = len(merged) - 1
			continue
		}
		// Merge into the previous chunk, maintaining the running token sum.
		if prev.Text != "" && ck.Text != "" {
			prev.Text = prev.Text + joinSep + ck.Text
		} else {
			prev.Text = prev.Text + ck.Text
		}
		prev.TKNums = intPtr(intValue(prev.TKNums) + tk)
		prev.PDFPositions = extendRawJSONArray(prev.PDFPositions, ck.PDFPositions)
		prev.Positions = extendRawJSONArray(prev.Positions, ck.Positions)
		mergedItems[prevIdx] = append(mergedItems[prevIdx], cur)
	}
	return merged
}

// mergeByTokenSizeFromJSON merges the text units of each upstream item using
// the unified mergeUnits core (hard-cap expansion + UNDER_CAP merge),
// mirroring Python token_chunker.py:_merge_text_chunks_by_token_size. Non-text
// units pass through and reset the merge run.
func mergeByTokenSizeFromJSON(perItem [][]schema.ChunkDoc, chunkTokens int, overlappedPct float64) [][]schema.ChunkDoc {
	// overlappedPct is a [0,100] percentage. Clamp defensively because this
	// helper is also exercised directly by tests.
	if overlappedPct < 0 {
		overlappedPct = 0
	} else if overlappedPct > 100 {
		overlappedPct = 100
	}
	for idx := range perItem {
		if len(perItem[idx]) == 0 {
			continue
		}
		// All text units in the sequence are merged with the unified
		// JSON-strategy core. Non-text units pass through and reset the merge
		// run (see mergeUnits). Join separator is "\n" to mirror
		// token_chunker.py:_merge_text_chunks_by_token_size, which joins
		// adjacent item text with "\n".
		perItem[idx] = mergeUnits(perItem[idx], chunkTokens, overlappedPct, "\n")
	}
	return perItem
}

// ---------------------------------------------------------------------------
// Hard-cap expansion
// ---------------------------------------------------------------------------

// expandOversizedUnits re-splits text units whose token count exceeds target
// so every returned text unit is <= target tokens. Sentence boundaries are
// tried first (delimiters preserved, lossless); a piece that still exceeds
// target is hard-split on token count (char-prefix via TrimContentToTokenLimit,
// also lossless). Every sub-piece keeps the source unit's (coarse) PDF
// positions so each piece still gets its page-region preview image and
// highlight. Non-text units pass through unchanged.
func expandOversizedUnits(units []schema.ChunkDoc, target int) []schema.ChunkDoc {
	if target <= 0 {
		return units
	}
	out := make([]schema.ChunkDoc, 0, len(units))
	for _, ck := range units {
		if ck.CKType != "text" {
			out = append(out, ck)
			continue
		}
		tk := intValue(ck.TKNums)
		if tk <= 0 {
			tk = tokenizeStr(ck.Text)
		}
		if tk <= target {
			if ck.TKNums == nil {
				ck.TKNums = intPtr(tk)
			}
			out = append(out, ck)
			continue
		}
		out = append(out, splitOversizedText(ck, target)...)
	}
	return out
}

// splitOversizedText splits one oversized text unit into <= target pieces.
// Sentence boundaries are preferred; a boundary-less run that still exceeds
// target is hard token-split. Delimiters are preserved so the concatenated
// pieces reproduce the unit text exactly (lossless). Every piece keeps the
// source unit's DocType and (coarse) PDF positions, built from a clone of the
// source so all ChunkDoc fields survive the split.
func splitOversizedText(ck schema.ChunkDoc, target int) []schema.ChunkDoc {
	text := ck.Text
	// Preserve a leading "\n" glue (text-path units are built as "\n"+part).
	lead := ""
	if strings.HasPrefix(text, "\n") {
		lead = "\n"
		text = strings.TrimPrefix(text, "\n")
	}
	// Split into sentences (delimiters attached). A whitespace-only fragment
	// (a bare delimiter from a repeated run, e.g. "\n\n") is prepended to the
	// NEXT non-whitespace piece so the concatenated pieces reproduce the unit
	// text exactly; trailing whitespace attaches to the last piece. If
	// attaching whitespace pushes a piece over the target, that piece is
	// hard-split so the hard cap still holds.
	var pieces []schema.ChunkDoc
	var wsPending string
	for _, part := range splitSentencesLossless(text) {
		if strings.TrimSpace(part) == "" {
			wsPending += part
			continue
		}
		p := schema.ChunkDoc{Text: wsPending + part, DocType: ck.DocType, CKType: "text"}
		wsPending = ""
		if n := tokenizeStr(p.Text); n > target {
			pieces = append(pieces, hardSplitPiece(p.Text, ck.DocType, target)...)
		} else {
			p.TKNums = intPtr(n)
			pieces = append(pieces, p)
		}
	}
	if wsPending != "" {
		if len(pieces) > 0 {
			last := &pieces[len(pieces)-1]
			last.Text += wsPending
			if n := tokenizeStr(last.Text); n > target {
				replaced := hardSplitPiece(last.Text, ck.DocType, target)
				pieces = append(pieces[:len(pieces)-1], replaced...)
			} else {
				last.TKNums = intPtr(n)
			}
		} else {
			// All-whitespace unit: keep it lossless but still within the cap —
			// hard-split like any other oversized text so a whitespace-only run
			// whose token count exceeds the target cannot emit an over-cap chunk.
			if n := tokenizeStr(wsPending); n > target {
				pieces = append(pieces, hardSplitPiece(wsPending, ck.DocType, target)...)
			} else {
				pieces = append(pieces, schema.ChunkDoc{Text: wsPending, DocType: ck.DocType, TKNums: intPtr(n), CKType: "text"})
			}
		}
	}
	if len(pieces) == 0 {
		return nil
	}
	if lead != "" {
		pieces[0].Text = lead + pieces[0].Text
		// The leading "\n" adds a token; recompute so the running sum stays
		// faithful. Re-split if it pushes the piece over the target.
		if n := tokenizeStr(pieces[0].Text); n > target {
			replaced := hardSplitPiece(pieces[0].Text, ck.DocType, target)
			pieces = append(replaced, pieces[1:]...)
		} else {
			pieces[0].TKNums = intPtr(n)
		}
	}
	// Every sub-piece inherits the source unit's metadata (coarse positions +
	// item attributes); each is built from a clone so every ChunkDoc field
	// survives the split. PDF positions are proportionally sliced vertically so
	// each piece's screenshot crops only its own region instead of the whole
	// paragraph (fix for chunk-screenshot mismatch).
	if len(ck.PDFPositions) == 0 && len(ck.Positions) == 0 {
		for i := range pieces {
			inherited := cloneChunkDoc(ck)
			inherited.Text = pieces[i].Text
			inherited.TKNums = pieces[i].TKNums
			inherited.CKType = "text"
			pieces[i] = inherited
		}
		return pieces
	}
	// Ratio basis: rune counts of the real source content. The synthetic
	// leading "\n" glue (text-path units are built as "\n"+paragraph) carries
	// no visual height, so it is excluded from the first piece's count to keep
	// the slice proportions aligned with the original text. Parser tags
	// (@@...##) also carry no visual height and are stripped before counting.
	counts := make([]int, len(pieces))
	runCount := 0
	for i, p := range pieces {
		n := utf8.RuneCountInString(removeTag(p.Text))
		if i == 0 && lead != "" {
			n -= utf8.RuneCountInString(lead)
			if n < 0 {
				n = 0
			}
		}
		counts[i] = n
		runCount += n
	}
	if runCount == 0 {
		for i := range pieces {
			inherited := cloneChunkDoc(ck)
			inherited.Text = pieces[i].Text
			inherited.TKNums = pieces[i].TKNums
			inherited.CKType = "text"
			pieces[i] = inherited
		}
		return pieces
	}
	cumRunes := 0
	for i, p := range pieces {
		startRatio := float64(cumRunes) / float64(runCount)
		endRatio := float64(cumRunes+counts[i]) / float64(runCount)
		inherited := cloneChunkDoc(ck)
		inherited.Text = p.Text
		inherited.TKNums = p.TKNums
		inherited.CKType = "text"
		inherited.PDFPositions = slicePositionsByTextRatio(ck.PDFPositions, startRatio, endRatio)
		inherited.Positions = slicePositionsByTextRatio(ck.Positions, startRatio, endRatio)
		// Fall back to original positions if slicing produced nothing (e.g.
		// malformed matrix) so the piece still gets a preview rather than none.
		if len(inherited.PDFPositions) == 0 && len(ck.PDFPositions) > 0 {
			inherited.PDFPositions = ck.PDFPositions
		}
		if len(inherited.Positions) == 0 && len(ck.Positions) > 0 {
			inherited.Positions = ck.Positions
		}
		pieces[i] = inherited
		cumRunes += counts[i]
	}
	return pieces
}

// splitSentencesLossless splits text on sentenceDelimiter, keeping each
// delimiter attached to its preceding sentence, so the concatenated parts
// reproduce the input exactly (lossless). Mirrors Python's
// _split_text_by_sentences (rag/flow/chunker/title_chunker/common.py).
func splitSentencesLossless(text string) []string {
	if text == "" {
		return nil
	}
	idxs := sentenceDelimiter.FindAllStringIndex(text, -1)
	var out []string
	prev := 0
	for _, idx := range idxs {
		if idx[0] > prev {
			out = append(out, text[prev:idx[1]])
		} else {
			out = append(out, text[idx[0]:idx[1]])
		}
		prev = idx[1]
	}
	if prev < len(text) {
		out = append(out, text[prev:])
	}
	return out
}

// hardSplitPiece hard token-splits a boundary-less run into <= target pieces.
// Each piece is a true character prefix of the remaining text (U+FFFD tail
// trimmed), so the concatenated pieces reproduce the input exactly. A token
// cut never lands inside a @@...## coordinate tag: the cut is extended past
// the closing "##" when the budget allows, otherwise backed off to before the
// "@@" — so a tag is never split across two pieces.
func hardSplitPiece(text string, docType string, target int) []schema.ChunkDoc {
	var out []schema.ChunkDoc
	rest := text
	for tokenizeStr(rest) > target {
		head := tokenizer.TrimContentToTokenLimit(rest, target)
		if head == "" || head == rest {
			break // cannot shrink further; avoid an infinite loop
		}
		// TrimContentToTokenLimit decodes a token prefix; if that lands on a
		// mid-multibyte boundary it can emit a trailing U+FFFD which is not a
		// true prefix of rest. Trim it so the rest[len(head):] advance stays
		// lossless.
		if !strings.HasPrefix(rest, head) {
			head = strings.TrimRight(head, "\uFFFD")
			if head == "" {
				break
			}
		}
		cut := adjustCutPastTag(rest, len(head), target)
		if cut <= 0 || cut >= len(rest) {
			// Safety: never stall. An empty cut or a whole-text cut means no
			// progress is possible; emit the remaining text as the last piece.
			if rest != "" {
				out = append(out, schema.ChunkDoc{Text: rest, DocType: docType, TKNums: intPtr(tokenizeStr(rest)), CKType: "text"})
			}
			rest = ""
			break
		}
		head = rest[:cut]
		out = append(out, schema.ChunkDoc{Text: head, DocType: docType, TKNums: intPtr(tokenizeStr(head)), CKType: "text"})
		rest = rest[cut:]
	}
	if rest != "" {
		out = append(out, schema.ChunkDoc{Text: rest, DocType: docType, TKNums: intPtr(tokenizeStr(rest)), CKType: "text"})
	}
	return out
}

// adjustCutPastTag shifts a character cut so it never lands inside a
// @@...## coordinate tag: extending past the closing "##" is preferred when
// the token budget allows; otherwise the cut backs off to just before the
// "@@". Tags with no closing "##" (unterminated) are ignored — they have no
// paired span to protect.
func adjustCutPastTag(text string, cut, target int) int {
	searchFrom := 0
	for {
		s := strings.Index(text[searchFrom:], "@@")
		if s < 0 {
			return cut
		}
		s += searchFrom
		eRel := strings.Index(text[s+2:], "##")
		if eRel < 0 {
			return cut // unterminated tag; nothing to protect
		}
		e := s + 2 + eRel + 2
		if cut > s && cut < e {
			// The cut lands inside a tag: prefer extending past the tag if it
			// stays within the token budget, else back off to before it. A
			// leading tag (s == 0) cannot back off to an empty piece, so it is
			// kept whole by extending past it.
			if e <= len(text) && tokenizeStr(text[:e]) <= target {
				return e
			}
			if s > 0 {
				return s
			}
			return e
		}
		searchFrom = e
	}
}

// overlapFitPrefix trims the overlap prefix so overlap+current fits within
// target tokens (hard cap with overlap enabled). It returns the trimmed
// overlap and the number of runes removed from its front, so callers can
// adjust the position-tail cut to the actual overlap region.
func overlapFitPrefix(overlap, current string, target int) (string, int) {
	if tokenizeStr(overlap+current) <= target {
		return overlap, 0
	}
	runes := []rune(overlap)
	for c := 1; c <= len(runes); c++ {
		suffix := string(runes[c:])
		if tokenizeStr(suffix+current) <= target {
			return suffix, c
		}
	}
	// current alone should never exceed target (units are pre-expanded); if it
	// somehow does, drop the overlap entirely.
	return "", len(runes)
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
	// Deep-copy the coordinate byte slices so the clone does not alias
	// the source's backing array (diff 2.5 defensive fix).
	if in.PDFPositions != nil {
		out.PDFPositions = append(json.RawMessage(nil), in.PDFPositions...)
	}
	if in.Positions != nil {
		out.Positions = append(json.RawMessage(nil), in.Positions...)
	}
	if in.Extra != nil {
		out.Extra = make(map[string]json.RawMessage, len(in.Extra))
		for k, v := range in.Extra {
			out.Extra[k] = append(json.RawMessage(nil), v...)
		}
	}
	return out
}

// extendRawJSONArray concatenates two JSON array payloads, mirroring
// Python's `merged[prev][KEY].extend(current[KEY])`. Either operand may be
// empty; the result is always a valid JSON array (or an empty raw message).
// It is used to accumulate PDF coordinate lists (`_pdf_positions`,
// `positions`) when text chunks are merged (diffs 2.5 / 2.3).
func extendRawJSONArray(a, b json.RawMessage) json.RawMessage {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
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
		mom := strings.TrimPrefix(ck.Text, "\n")
		parts := splitDroppingDelim(ck.Text, pattern)
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

// hasActiveDelimiter reports whether a compiled delimiter pattern is
// present (non-nil). compileDelimPattern returns nil when no active
// pattern exists, so a nil check is sufficient.
func hasActiveDelimiter(p *regexp.Regexp) bool {
	return p != nil
}

// hasCustomDelim reports whether any delimiter uses backtick syntax
// (`pattern`). Python's naive_merge skips token-size merging when
// custom delimiters are present. Delegates to the canonical helper.
func hasCustomDelim(delims []string) bool {
	return chunk.HasCustomDelimiterList(delims)
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
		for _, child := range splitDroppingDelim(t, pattern) {
			if strings.TrimSpace(child) == "" {
				continue
			}
			out = append(out, schema.ChunkDoc{Text: child, Mom: strings.TrimPrefix(t, "\n")})
		}
	}
	return out
}

// compileChildrenPattern is the children_delimiters version of
// compileDelimPattern. Returns nil when no delimiters exist.
// compileChildrenPattern builds the children-split regex from a
// `children_delimiters` list. Every non-empty entry is active (including bare
// ones), and backtick-wrapped entries contribute their inner content — see
// chunk.CompileDelimiterPatternList. Delegating keeps children splitting
// consistent with the main delimiter list (backtick stripping + rune-descending
// order) instead of re-implementing a divergent copy.
func compileChildrenPattern(delims []string) *regexp.Regexp {
	return chunk.CompileDelimiterPatternList(delims, true)
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
