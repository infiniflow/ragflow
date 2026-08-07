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

	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/globals"
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
	if v, ok := conf["under_cap"].(bool); ok {
		p.TokenChunkerParam.UnderCap = v
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
// {markdown,text,html} on the python side).
func (c *TokenChunkerComponent) invokeTextPayload(_ context.Context, text string, delimPattern, childrenPattern *regexp.Regexp) map[string]any {
	if text == "" {
		return emptyOutputs()
	}

	if !hasActiveDelimiter(delimPattern) {
		return c.mergeByTokenSize(text, childrenPattern)
	}

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

	// Python's naive_merge: a custom (backtick) delimiter yields one chunk
	// per segment and no token-size merge (naive_merge:1194-1213). A
	// non-custom active delimiter cannot reach here — delimPattern is
	// non-nil only when a backtick delimiter exists, so the split-then-
	// merge branch was unreachable and has been removed.
	return chunkOutputs(docs)
}

// sentenceDelimiter is the sentence/clause-boundary regex used to split
// oversized sections. It mirrors the delimiter Python's chunker actually
// uses in production: rag/app/naive.py:1285 passes "\n!?。；！？" to
// naive_merge, which includes ASCII "!" and "?" as well as the CJK
// punctuation "。；！？". It deliberately does NOT include an English
// ". " fallback: Python's production delimiter has no "\.\s", so adding
// it would diverge from Python's chunk boundaries.
var sentenceDelimiter = regexp.MustCompile(`(\n|[!?。；！？])`)

// computeOverlapPrefix returns (overlapText, overlapTokenCount) carved from
// the tail of prevText after stripping parser tags. overlappedPct is a
// percentage in [0, 100]. Mirrors Python rag/nlp._compute_overlap_prefix.
func computeOverlapPrefix(prevText string, overlappedPct float64) (string, int) {
	visible := removeTag(prevText)
	if visible == "" {
		return "", 0
	}
	runes := []rune(visible)
	cut := int(float64(len(runes)) * (100 - overlappedPct) / 100.0)
	if cut < 0 {
		cut = 0
	}
	if cut >= len(runes) {
		return "", 0
	}
	overlap := string(runes[cut:])
	return overlap, tokenizeStr(overlap)
}

// mergeAction is the decision returned by mergeDecision for one incoming unit.
type mergeAction int

const (
	// mergeIntoPrev overrides the previous chunk's text with the joined text.
	mergeIntoPrev mergeAction = iota
	// startNewChunk appends a new chunk, optionally prefixed with an overlap
	// slice carved from the previous chunk.
	startNewChunk
	// mergeThenClose overrides the previous chunk with the joined text and
	// forces the NEXT incoming unit to start a brand-new chunk (OVER_CAP
	// boundary overflow: a chunk may exceed target by at most one unit).
	mergeThenClose
)

// mergeDecision computes the merge decision shared by the text and JSON merge
// paths. prevText is the current chunk, incoming is the next unit, and joinSep
// is the separator used to project the joined text ("" for the text path, "\n"
// for the JSON path). target is the token cap. prevTokens is the running sum of
// per-unit token counts already accumulated into prevText; incomingTokens is
// the token count of incoming, counted the same way the calling path counts a
// unit.
//
// strategy selects the merge strategy (schema.MergeStrategy), mirroring
// Python's MergeStrategy:
//   - MergeOverCap (default, Python OVER_CAP): when the joined text exceeds
//     target but the incoming unit still fits target, it is merged into the
//     previous chunk and that chunk is then closed (mergeThenClose), forcing
//     the next unit to start a new chunk. An incoming unit that already exceeds
//     target is never merged — it stands alone as its own chunk (Python
//     OVER_CAP: an oversized paragraph is never combined with the previous
//     chunk).
//   - MergeUnderCap (Python UNDER_CAP, strict no-overflow): an overflowing
//     joined text starts a new chunk instead.
//
// The merge decision is made on the RUNNING SUM of per-unit token counts
// (prevTokens + incomingTokens), NEVER on tokenizeStr(joined). BPE
// tokenization is non-additive across joinSep ("\n"), so re-tokenizing the
// joined string disagreed with Python's per-paragraph size() sum and shifted
// every downstream boundary by a line — and, once the budget is small enough,
// changed the chunk count. Both Python references accumulate a running sum of
// per-unit counts: rag/nlp/__init__.py:_merge_paragraph_groups
// (size("\n" + sub_sec)) and rag/flow/chunker/token_chunker.py:
// _merge_text_chunks_by_token_size (tk_nums += current["tk_nums"]). The joined
// text is still returned as the merged content.
//
// JSON-only metadata (PDFPositions/Positions/TKNums) is the caller's
// responsibility; this helper only returns the merged/new text and the action.
//
// Note on the JSON path: this helper decides on the running sum, but it still
// uses the OVER_CAP strategy (merge-then-close on overflow). The Python JSON
// reference (rag/flow/chunker/token_chunker.py:_merge_text_chunks_by_token_size)
// decides on prev_tk_nums > threshold instead, so JSON parity is only partially
// addressed here: the join-string re-tokenization is gone but the merge
// STRATEGY difference remains. The TKNums accounting contract is pinned by
// TestMergeByTokenSizeFromJSON_TKNumsConsistency.
func mergeDecision(prevText, incoming, joinSep string, target int, overlapPct float64, strategy schema.MergeStrategy, prevTokens, incomingTokens int) (string, mergeAction) {
	// An incoming unit that already exceeds target can never be merged; it
	// stands alone as its own chunk.
	if incomingTokens > target {
		return newChunkText(prevText, incoming, target, overlapPct, incomingTokens), startNewChunk
	}
	joined := prevText + joinSep + incoming
	// Faithful to Python: decide on the running sum of per-unit token counts,
	// never on the BPE count of the re-joined string, which is non-additive
	// across joinSep ("\n") and therefore disagrees with Python's per-paragraph
	// size() sum, shifting every downstream boundary by a line (and, on a small
	// enough budget, changing the chunk count). See the function doc.
	if prevTokens+incomingTokens <= target {
		return joined, mergeIntoPrev
	}
	if strategy == schema.MergeOverCap {
		// OVER_CAP: merge the overflowing unit but close the chunk so the
		// next unit starts fresh.
		return joined, mergeThenClose
	}
	return newChunkText(prevText, incoming, target, overlapPct, incomingTokens), startNewChunk
}

// newChunkText returns the text for a fresh chunk started after prevText,
// prefixing an overlap slice from prevText when one fits within target. It is
// used both by the UNDER_CAP overflow branch of mergeDecision and by the
// caller when a previous chunk was closed by an OVER_CAP boundary overflow.
func newChunkText(prevText, incoming string, target int, overlapPct float64, incomingTokens int) string {
	if overlapPct > 0 {
		if overlapText, overlapTokens := computeOverlapPrefix(prevText, overlapPct); overlapTokens > 0 {
			if overlapTokens+incomingTokens <= target {
				return overlapText + incoming
			}
		}
	}
	return incoming
}

// mergeByTokenSize implements exact token-based chunk merging that mirrors
// Python's naive_merge (rag/nlp/__init__.py) after the strict chunk_token_num
// hard-cap fix. It uses tokenizeStr for precise token counting, treats the
// payload as a single section, and splits oversized sections on production
// sentence delimiters. An oversize unit (a single paragraph larger than the
// token budget) is kept whole as a standalone chunk — matching Python OVER_CAP,
// where the model layer truncates it later — instead of being atom-split.
// Sections are merged only when the projected total stays within
// chunk_token_size. Overlap is applied only when the resulting chunk still
// fits the budget.
func (c *TokenChunkerComponent) mergeByTokenSize(text string, childrenPattern *regexp.Regexp) map[string]any {
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
	sections := []string{text}
	if len(sections) == 0 {
		return emptyOutputs()
	}

	var cks []string
	var tkns []int

	// addChunk applies the projected-total merge and optional-overlap decision
	// to one unit that already fits target.
	var prevClosed bool
	addChunk := func(segment string) {
		tnum := tokenizeStr(segment)
		if len(cks) == 0 {
			cks = append(cks, segment)
			tkns = append(tkns, tnum)
			return
		}
		// Previous chunk was closed by an OVER_CAP boundary overflow: the
		// next unit must start a fresh chunk (with overlap when it fits).
		if prevClosed {
			prevClosed = false
			out := newChunkText(cks[len(cks)-1], segment, target, overlapPct, tnum)
			cks = append(cks, out)
			tkns = append(tkns, tokenizeStr(out))
			return
		}
		out, act := mergeDecision(cks[len(cks)-1], segment, "", target, overlapPct, c.param.MergeStrategy(), tkns[len(tkns)-1], tnum)
		switch act {
		case mergeIntoPrev, mergeThenClose:
			cks[len(cks)-1] = out
			// Maintain the running sum of per-paragraph token counts so the
			// next merge decision matches Python's size() sum.
			tkns[len(tkns)-1] += tnum
			prevClosed = act == mergeThenClose
		case startNewChunk:
			cks = append(cks, out)
			tkns = append(tkns, tokenizeStr(out))
		}
	}

	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}
		t := "\n" + sec
		if tokenizeStr(t) <= target {
			addChunk(t)
			continue
		}
		// Oversized section: split on production sentence delimiters into
		// units. An oversize unit (still exceeds the budget) is passed through
		// addChunk and kept whole — no atom-split, matching Python
		// naive_merge. mergeDecision forces an oversize incoming unit to
		// startNewChunk, so it stands alone as its own chunk.
		parts := sentenceDelimiter.Split(sec, -1)
		hadPart := false
		for _, part := range parts {
			// Keep the raw split fragment, including any inter-line trailing
			// whitespace. Python's naive_merge builds each unit from
			// "\n" + sub_sec (naive_merge:1357) where sub_sec retains its
			// trailing space and is never TrimSpaced — the only post-processing
			// is dropping the leading empty placeholder (naive_merge:1370-1375),
			// never per-unit trimming. Trimming here drops that space, so the
			// overlap prefix carved from the previous chunk (which runs over the
			// untrimmed segment) loses a character and diverges from Python.
			// Only genuinely empty fragments are skipped, mirroring naive_merge's
			// `if not sub_sec` guard. (The final-output TrimSpace only strips a
			// chunk's own leading/trailing whitespace, not the inter-line space
			// preserved here.)
			if part == "" {
				continue
			}
			hadPart = true
			addChunk("\n" + part)
		}
		if !hadPart {
			addChunk(t)
		}
	}

	docs := make([]schema.ChunkDoc, 0, len(cks))
	for _, ch := range cks {
		// Strip parser position tags from the final text:
		// the merge paths may carry @@...## markers that must not leak into
		// indexed/embedded chunk text.
		ch = removeTag(strings.TrimSpace(ch))
		if ch == "" {
			continue
		}
		docs = append(docs, schema.ChunkDoc{Text: ch})
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
		attached = mergeByTokenSizeFromJSON([][]schema.ChunkDoc{flatten(attached)}, c.param.ChunkTokenSize, c.param.OverlappedPercent, c.param.MergeStrategy())
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

// mergeByTokenSizeFromJSON mirrors Python naive_merge's projected-total
// hard cap (rag/nlp/__init__.py after the strict chunk_token_num fix).
// Over-budget units are never atom-split: each one stands alone as its own
// chunk (Python naive_merge behavior, #17808 OVER_CAP contract). Overlap is
// applied only when overlap+segment still fits the budget.
//
// strategy selects the merge strategy (schema.MergeStrategy): MergeOverCap =
// OVER_CAP (Python's canonical default, a chunk may exceed the target by at
// most one incoming unit), MergeUnderCap = UNDER_CAP (never exceed the target;
// a projected overflow starts a fresh chunk). The TokenChunker threads its
// MergeStrategy() here.
func mergeByTokenSizeFromJSON(perItem [][]schema.ChunkDoc, chunkTokens int, overlappedPct float64, strategy schema.MergeStrategy) [][]schema.ChunkDoc {
	// overlappedPct is a [0,100] percentage. Clamp defensively because this
	// helper is also exercised directly by tests.
	if overlappedPct < 0 {
		overlappedPct = 0
	} else if overlappedPct > 100 {
		overlappedPct = 100
	}
	for idx := range perItem {
		chunks := perItem[idx]
		if len(chunks) == 0 {
			continue
		}
		var merged []schema.ChunkDoc

		// addTextChunk applies the projected-total merge / overlap-drop
		// decision for one text unit that already fits chunkTokens.
		var prevClosed bool
		addTextChunk := func(ck schema.ChunkDoc) {
			tk := intValue(ck.TKNums)
			if tk <= 0 {
				tk = tokenizeStr(ck.Text)
				ck.TKNums = intPtr(tk)
			}
			if len(merged) == 0 || merged[len(merged)-1].CKType != "text" {
				// First text chunk, or first text after a non-text chunk:
				// no prior text to overlap with. A stale OVER_CAP boundary
				// overflow (prevClosed) from a previous text chunk must be
				// cleared here, otherwise the next text chunk would be wrongly
				// forced into a fresh chunk instead of merging with this one.
				prevClosed = false
				merged = append(merged, cloneChunkDoc(ck))
				return
			}
			prev := &merged[len(merged)-1]
			// Empty previous text: assign incoming text directly
			// (diff Chunker-2.11 / token_chunker.py:236-239).
			if prev.Text == "" {
				// Invariant: every path that emits a chunk WITHOUT consuming
				// the OVER_CAP boundary-overflow flag (prevClosed) must clear
				// it, so a stale flag can never leak into a later chunk. The
				// early-return above already does this for the first-text /
				// after-non-text case; do the same here. (Today this branch is
				// only reached with an empty prev, and mergeThenClose always
				// leaves a non-empty prev, so prevClosed cannot actually be
				// true here — the reset is defensive and keeps the invariant
				// explicit against future refactors.)
				prevClosed = false
				prev.Text = ck.Text
				prev.TKNums = intPtr(tk)
				prev.PDFPositions = extendRawJSONArray(prev.PDFPositions, ck.PDFPositions)
				prev.Positions = extendRawJSONArray(prev.Positions, ck.Positions)
				return
			}
			// Previous chunk was closed by an OVER_CAP boundary overflow:
			// the next unit must start a fresh chunk (with overlap when it
			// fits). Coordinates stay on the new chunk only.
			if prevClosed {
				prevClosed = false
				cp := cloneChunkDoc(ck)
				cp.Text = newChunkText(prev.Text, ck.Text, chunkTokens, overlappedPct, tk)
				// Boundary path: TKNums is the RE-TOKENIZED new chunk text (which
				// may include an overlap prefix and the "\n" joins), NOT the
				// running sum used on the merge path above. With overlap > 0 this
				// mixes the actual text count into the otherwise-running-sum
				// accounting, so it can diverge from Python; pinned by
				// TestMergeByTokenSizeFromJSON_TKNumsConsistency.
				cp.TKNums = intPtr(tokenizeStr(cp.Text))
				merged = append(merged, cp)
				return
			}
			// Proactive projected-total merge (joined with "\n").
			out, act := mergeDecision(prev.Text, ck.Text, "\n", chunkTokens, overlappedPct, strategy, intValue(prev.TKNums), tk)
			switch act {
			case mergeIntoPrev, mergeThenClose:
				prev.Text = out
				// Maintain the running sum of upstream tk_nums so the next
				// merge decision matches Python's tk_nums += current["tk_nums"].
				prev.TKNums = intPtr(intValue(prev.TKNums) + tk)
				prev.PDFPositions = extendRawJSONArray(prev.PDFPositions, ck.PDFPositions)
				prev.Positions = extendRawJSONArray(prev.Positions, ck.Positions)
				prevClosed = act == mergeThenClose
			case startNewChunk:
				cp := cloneChunkDoc(ck)
				cp.Text = out
				// Boundary path: TKNums is the RE-TOKENIZED new chunk text, not the
				// running sum (see the prevClosed branch above for why this is a
				// deliberate, divergence-prone accounting choice).
				cp.TKNums = intPtr(tokenizeStr(out))
				merged = append(merged, cp)
			}
		}

		for _, ck := range chunks {
			if ck.CKType != "text" {
				merged = append(merged, cloneChunkDoc(ck))
				continue
			}
			tk := intValue(ck.TKNums)
			if tk <= 0 {
				tk = tokenizeStr(ck.Text)
			}
			if tk <= chunkTokens {
				addTextChunk(ck)
				continue
			}
			// Over-budget unit: keep it whole. Python's naive_merge never
			// sub-splits a single item, so emit it as one chunk and let the
			// model layer truncate it later.
			addTextChunk(ck)
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
		mom := ck.Text
		parts := splitDroppingDelim(mom, pattern)
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
			out = append(out, schema.ChunkDoc{Text: child, Mom: t})
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
