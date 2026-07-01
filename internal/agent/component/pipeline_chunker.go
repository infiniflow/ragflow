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

// PipelineChunker component (T3) — partial port of python
// `agent/component/pipeline_chunker.py` (PR #15068).
//
// SCOPE (honest):
//
//   - WHITELIST: every parser_id in the python
//     `_PARSER_MODULES` table is accepted (general/naive/paper/
//     book/presentation/manual/laws/qa/table/resume/picture/one/
//     audio/email/tag). Check() rejects anything else.
//
//   - DISPATCH: parser_id drives the chunk-engine split
//     strategy. The Go chunk engine exposes sentence / paragraph /
//     char / paragraph splits; we map the python parser_ids to
//     those strategies (see parserToSplitStrategy). Per-parser
//     knobs (paper's double-column handling, table's HTML
//     reconstruction, laws' article-aware boundaries, …) are
//     not ported yet — a canvas author who picks `paper` gets a
//     paragraph split, not the python paper parser's column-aware
//     behaviour.
//
//   - TEXT INPUT: "text" / "content" / "file_bytes" (interpreted
//     as UTF-8 text) flow through the chunk engine. This matches
//     the python "text file" path.
//
//     "file_ref" is the bytes-container form used by the other Go
//     agent components (ExcelProcessor, etc.). It accepts
//     []byte OR a base64-encoded string. Raw text MUST go in
//     "text" / "content" — a string file_ref that is not valid
//     base64 is rejected with a clear error so a caller that
//     mistakenly hands plain text doesn't see it silently
//     reinterpreted (a "try base64, fall back" policy would
//     rewrite any plain text that happens to satisfy the
//     base64 alphabet, e.g. "Zm9v" → "foo"). The contract is
//     therefore: file_ref is always raw bytes (or base64 of
//     them); text/content is always UTF-8 text.
//
//   - FILE-FORMAT EXTRACTION (PDF, DOCX, XLSX, PPT, images, …):
//     NOT PORTED. The python side uses `rag.app.<parser>.chunk
//     (filename)` which extracts text from the file format
//     AND chunks in one step. The Go side does not yet have a
//     unified `rag.app.*.chunk` dispatch — `deepdoc/parser` is
//     still Python-only. A caller that supplies binary
//     (non-UTF-8) bytes is rejected with an explicit error so
//     the canvas author knows to add text extraction upstream.
//     This is a follow-up that lands with the deepdoc/parser
//     port.
//
//   - NO EMBEDDING / NO PERSISTENCE: chunks live only in
//     canvas variables for the run, exactly as in the python
//     fix.
package component

import (
	"context"
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion"
	"ragflow/internal/ingestion/chunk"
)

const componentNamePipelineChunker = "PipelineChunker"

// supportedParserIDs mirrors the python `_PARSER_MODULES` keys.
// The whitelist is enforced at component-construction time
// so a misspelled parser_id is caught at canvas-build time,
// not at run time.
var supportedPipelineParserIDs = map[string]struct{}{
	"general":      {},
	"naive":        {},
	"paper":        {},
	"book":         {},
	"presentation": {},
	"manual":       {},
	"laws":         {},
	"qa":           {},
	"table":        {},
	"resume":       {},
	"picture":      {},
	"one":          {},
	"audio":        {},
	"email":        {},
	"tag":          {},
}

// pipelineChunkerParam is the static DSL configuration for a
// PipelineChunker node. Fields mirror python
// PipelineChunkerParam.__init__ defaults.
type pipelineChunkerParam struct {
	ParserID     string         `json:"parser_id"`     // whitelisted; drives split strategy
	Lang         string         `json:"lang"`          // python-only knob, surfaced for DSL parity
	FromPage     int            `json:"from_page"`     // python-only knob, surfaced for DSL parity
	ToPage       int            `json:"to_page"`       // python-only knob, surfaced for DSL parity
	ParserConfig map[string]any `json:"parser_config"` // python-only knob, surfaced for DSL parity
}

// Update copies a fresh param map into the receiver.
func (p *pipelineChunkerParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	p.ParserID, _ = conf["parser_id"].(string)
	if p.ParserID == "" {
		p.ParserID = "naive"
	}
	p.Lang, _ = conf["lang"].(string)
	if p.Lang == "" {
		p.Lang = "English"
	}
	if v, ok := conf["from_page"].(float64); ok {
		p.FromPage = int(v)
	} else if v, ok := conf["from_page"].(int); ok {
		p.FromPage = v
	}
	if v, ok := conf["to_page"].(float64); ok {
		p.ToPage = int(v)
	} else if v, ok := conf["to_page"].(int); ok {
		p.ToPage = v
	}
	if cfg, ok := conf["parser_config"].(map[string]any); ok {
		p.ParserConfig = cfg
	} else {
		p.ParserConfig = map[string]any{}
	}
	return nil
}

// Check validates the parser_id whitelist, page range, and
// parser_config shape. Mirrors python
// PipelineChunkerParam.check().
func (p *pipelineChunkerParam) Check() error {
	if _, ok := supportedPipelineParserIDs[p.ParserID]; !ok {
		return fmt.Errorf("PipelineChunker: parser_id %q is not supported (allowed: %v)",
			p.ParserID, pipelineChunkerWhitelistOrdered())
	}
	if p.FromPage < 0 {
		return fmt.Errorf("PipelineChunker: from_page must be non-negative (got %d)", p.FromPage)
	}
	if p.ToPage < 0 {
		return fmt.Errorf("PipelineChunker: to_page must be non-negative (got %d)", p.ToPage)
	}
	if p.FromPage > p.ToPage {
		return fmt.Errorf("PipelineChunker: from_page (%d) must be <= to_page (%d)",
			p.FromPage, p.ToPage)
	}
	if p.ParserConfig == nil {
		return fmt.Errorf("PipelineChunker: parser_config must be a dict")
	}
	return nil
}

func pipelineChunkerWhitelistOrdered() []string {
	out := make([]string, 0, len(supportedPipelineParserIDs))
	for k := range supportedPipelineParserIDs {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// parserToSplitStrategy maps a python parser_id to the Go
// chunk engine's split strategy. The Go chunk engine exposes
// sentence / paragraph / char / paragraph splits — these
// correspond loosely to the python "naive" / "paper" /
// "table" strategies but do not implement the parser-specific
// extraction (PDF column detection, table HTML reconstruction,
// etc.). The mapping below is the conservative best-effort
// port: pick the split granularity that most matches the
// python parser's intent. A canvas author who needs the full
// parser-specific behaviour must wait for the deepdoc/parser
// port to land.
func parserToSplitStrategy(parserID string) string {
	switch parserID {
	case "general", "naive", "book", "presentation",
		"manual", "qa", "resume", "email", "tag":
		return "paragraph"
	case "paper", "laws":
		// python paper/laws chunk on sentence boundaries
		// within article/column scopes; the Go split engine
		// does not have a "scoped sentence" split, so we
		// fall back to sentence-level. The structural loss
		// is documented in the package comment.
		return "sentence"
	case "table":
		// python table chunker emits one chunk per row.
		// The Go side has no row-aware split; char-split
		// at a 1024 rune size approximates it. The
		// mismatch is documented.
		return "char"
	case "picture", "one", "audio":
		// picture/one/audio produce a single chunk per
		// file. The Go side has no "single-chunk" split;
		// paragraph split on a single-paragraph input
		// collapses to one chunk.
		return "paragraph"
	default:
		return "paragraph"
	}
}

// PipelineChunkerComponent runs the configured chunker against
// the input text and returns the chunks as plain text (no
// embedding, no persistence) for downstream Agent nodes.
//
// Output shape:
//
//	chunks       — []string of plain-text chunks
//	chunks_full  — []map[string]any with at minimum {text, ...}
//	summary      — short human-readable summary (parser_id + chunk count)
type PipelineChunkerComponent struct {
	name  string
	param pipelineChunkerParam
}

// NewPipelineChunkerComponent constructs a PipelineChunker from
// the DSL param map. Errors here surface as canvas compile
// failures so a bad parser_id is caught at canvas-build time
// rather than mid-run.
func NewPipelineChunkerComponent(params map[string]any) (Component, error) {
	p := &pipelineChunkerParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("PipelineChunker: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("PipelineChunker: param check: %w", err)
	}
	return &PipelineChunkerComponent{
		name:  componentNamePipelineChunker,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (c *PipelineChunkerComponent) Name() string { return c.name }

// Stream is a synchronous facade over Invoke.
func (c *PipelineChunkerComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := c.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the parameter metadata. The component reads
// any of the following from the inputs map, in order:
//
//	text       (string)              — raw UTF-8 text (primary)
//	content    (string)              — alias for "text"
//	file_ref   ([]byte | base64 str) — file bytes container.
//	                                   Mirrors the ExcelProcessor
//	                                   contract. The []byte form
//	                                   is the in-process caller's
//	                                   normal form; the string
//	                                   form is HTTP / JSON
//	                                   callers' normal form
//	                                   (base64). Raw text MUST
//	                                   go in "text" / "content".
//	file_bytes ([]byte | base64 str) — alias for "file_ref"
//	                                   under a more honest
//	                                   name. Same contract.
//
// Binary file bytes must be text-extracted upstream until the
// deepdoc/parser port lands; non-UTF-8 bytes are rejected
// with a clear "not yet ported" error.
func (c *PipelineChunkerComponent) Inputs() map[string]string {
	return map[string]string{
		"text":       "Plain-text input. The chunker slices this into downstream chunks.",
		"content":    "Alias for \"text\".",
		"file_ref":   "File bytes ([]byte) or base64-encoded string. NOT a state ref name. Raw text goes in \"text\" / \"content\".",
		"file_bytes": "Alias for \"file_ref\" ([]byte or base64-encoded string). Same encoding contract.",
	}
}

// Outputs returns the public surface that downstream Agent
// nodes can wire into.
func (c *PipelineChunkerComponent) Outputs() map[string]string {
	return map[string]string{
		"chunks":      "list[string]: plain-text chunks.",
		"chunks_full": "list[object]: per-chunk metadata (text + size + index).",
		"summary":     "string: short human-readable summary.",
	}
}

// Invoke runs the chunker against the input.
//
// Inputs contract:
//
//	"text"       — (preferred) the already-extracted plain text
//	"content"    — alias for "text"
//	"file_bytes" — raw bytes, MUST be valid UTF-8 (text formats
//	               only; binary formats return an explicit
//	               "deepdoc/parser not yet ported" error)
//
// Empty input returns the no-chunks sentinel.
//
// Non-UTF-8 bytes are rejected with an explicit "file-format
// extraction not ported" error so the canvas author is told
// to add text extraction upstream (or use the python canvas).
func (c *PipelineChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if _, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err != nil {
		return nil, fmt.Errorf("PipelineChunker: %w", err)
	}

	text, err := readPipelineInputText(inputs)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return map[string]any{
			"chunks":      []string{},
			"chunks_full": []map[string]any{},
			"summary":     "no input text",
		}, nil
	}

	strategy := parserToSplitStrategy(c.param.ParserID)
	dsl := buildPipelineChunkerDSL(strategy)
	plan, err := ingestion.NewChunkEngine().Compile(dsl)
	if err != nil {
		return nil, fmt.Errorf("PipelineChunker: compile (strategy=%s): %w", strategy, err)
	}
	result, err := ingestion.NewChunkEngine().Execute(plan, text)
	if err != nil {
		return nil, fmt.Errorf("PipelineChunker: execute: %w", err)
	}
	return chunkerOutputs(result, c.param.ParserID), nil
}

// readPipelineInputText returns the input text from any of the
// supported keys. Non-UTF-8 binary inputs are rejected with an
// explicit "extraction not ported" error so the canvas author
// gets a clear signal instead of a silent garbled chunk.
//
// Supported keys (first match wins):
//
//	text       (string)              — raw UTF-8 text (primary)
//	content    (string)              — alias for "text"
//	file_ref   ([]byte | base64 str) — file bytes container.
//	                                  Mirrors the ExcelProcessor
//	                                  contract. The []byte form
//	                                  is the in-process caller's
//	                                  normal form; the string
//	                                  form is HTTP / JSON callers'
//	                                  normal form (base64). Raw
//	                                  text MUST go in "text" /
//	                                  "content" — see
//	                                  decodeFileRefString for
//	                                  the rationale.
//	file_bytes ([]byte | base64 str) — alias for file_ref under a
//	                                  more honest name. Same
//	                                  encoding contract.
func readPipelineInputText(inputs map[string]any) (string, error) {
	if inputs == nil {
		return "", nil
	}
	if v, ok := inputs["text"].(string); ok {
		return v, nil
	}
	if v, ok := inputs["content"].(string); ok {
		return v, nil
	}
	// file_ref accepts []byte or a base64-encoded string,
	// matching the ExcelProcessor contract. The orchestrator
	// is responsible for any state-ref → bytes resolution.
	if b, ok := inputs["file_ref"].([]byte); ok {
		return validateAndDecodeBytes(b)
	}
	if s, ok := inputs["file_ref"].(string); ok && s != "" {
		return decodeFileRefString(s)
	}
	// file_bytes is the same bytes contract under a more
	// honest name; both keys map to the same handler.
	if b, ok := inputs["file_bytes"].([]byte); ok {
		return validateAndDecodeBytes(b)
	}
	// file_bytes also accepts a base64-encoded string, matching
	// file_ref's contract — JSON callers hand the orchestrator a
	// base64 blob under whichever key the upstream component
	// happens to emit, so we accept both forms rather than
	// silently dropping a perfectly-valid payload that used
	// "file_bytes" instead of "file_ref". Same strict-base64 rule
	// as decodeFileRefString: no fall-through to raw text.
	if s, ok := inputs["file_bytes"].(string); ok && s != "" {
		return decodeFileRefString(s)
	}
	return "", nil
}

// decodeFileRefString treats a file_ref string as STRICTLY
// base64-encoded raw bytes. There is no "fall back to raw
// text" path: a "try base64, fall back" policy would silently
// rewrite any plain-text input that happens to satisfy the
// base64 alphabet (e.g. "Zm9v" → "foo", "Q29kZUNvbnZlcnQ"
// → "CodeConvert"). The contract is unambiguous: file_ref
// string = base64 of the raw bytes. Callers that have raw
// text should use the "text" / "content" keys.
//
// The decoded bytes still flow through validateAndDecodeBytes
// so a PDF / DOCX with no upstream extraction surfaces a
// loud "not yet ported" error rather than garbled chunks.
func decodeFileRefString(s string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf(
			"PipelineChunker: file_ref string is not valid base64. " +
				"file_ref carries base64-encoded raw bytes; if you have plain text, " +
				"use the \"text\" or \"content\" input key instead. " +
				"(plain-text under the file_ref key was deliberately rejected to avoid " +
				"silent misinterpretation of strings that happen to satisfy the " +
				"base64 alphabet).")
	}
	if len(decoded) == 0 {
		return "", fmt.Errorf(
			"PipelineChunker: file_ref base64 string decoded to zero bytes. " +
				"Empty file_ref is not a valid input — use the \"text\" key for empty text " +
				"if that is the intent.")
	}
	return validateAndDecodeBytes(decoded)
}

// validateAndDecodeBytes is the central gate for byte inputs:
// non-UTF-8 bytes are rejected with a clear error so a caller
// that mistakenly hands a PDF without extraction sees a loud
// failure instead of a silent garbled chunk.
func validateAndDecodeBytes(b []byte) (string, error) {
	if !utf8.Valid(b) {
		return "", fmt.Errorf(
			"PipelineChunker: input bytes are not valid UTF-8. " +
				"File-format extraction (PDF/DOCX/...) is not yet ported to the Go side; " +
				"extract text upstream or use the python canvas.")
	}
	return string(b), nil
}

func chunkerOutputs(result *chunk.ChunkContext, parserID string) map[string]any {
	if result == nil {
		return map[string]any{
			"chunks":      []string{},
			"chunks_full": []map[string]any{},
			"summary":     "no chunks",
		}
	}
	chunks := make([]string, 0, len(result.ResultChunks))
	full := make([]map[string]any, 0, len(result.ResultChunks))
	for _, c := range result.ResultChunks {
		chunks = append(chunks, c.Content)
		full = append(full, map[string]any{
			"text":  c.Content,
			"size":  c.Size,
			"index": c.Index,
			"meta":  c.Metadata,
		})
	}
	summary := fmt.Sprintf("parser_id=%s chunks=%d", parserID, len(chunks))
	return map[string]any{
		"chunks":      chunks,
		"chunks_full": full,
		"summary":     summary,
	}
}

// buildPipelineChunkerDSL returns the chunk pipeline DSL the Go
// chunk engine consumes. Strategy is the split strategy
// (sentence/paragraph/char) selected from the parser_id. We
// deliberately do NOT pass `params.boundaries` here — the
// chunk engine's split operators have strategy-appropriate
// defaults (sentence: {。, ！, ？, \n}; paragraph: \n;
// char: rune count), and overriding them with a single hard-
// coded \n\n boundary would silence the per-strategy
// behaviour we are porting. The DSL shape matches
// ingestion.ChunkEngine.Compile (see internal/ingestion/
// chunk_engine_test.go:minimalDSL for the reference shape).
func buildPipelineChunkerDSL(strategy string) string {
	if strategy == "" {
		strategy = "paragraph"
	}
	return fmt.Sprintf(`{
  "pipeline": [
    {"operator": "preprocess", "normalize_newlines": true},
    {"operator": "split", "strategy": %q},
    {"operator": "postprocess", "filter": {"min_length": 1}}
  ]
}`, strategy)
}

func init() {
	Register(componentNamePipelineChunker, NewPipelineChunkerComponent)
}
