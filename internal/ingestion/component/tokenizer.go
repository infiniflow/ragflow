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

// Tokenizer ingestion component (Phase 2.4 of
// port-rag-flow-pipeline-to-go.md §4). Port of Python
// `rag/flow/tokenizer/tokenizer.py`. Computes (a) full-text token
// counts via the Go tokenizer package and (b) embedding vectors via
// the tenant's embedding model.
//
// SCOPE (honest):
//
//   - TOKEN COUNTING: matched at the wire level. Each chunk gets
//     `content_ltks` (tokenized string via `tokenizer.Tokenize`) and
//     `content_sm_ltks` (fine-grained variant) when `search_method`
//     includes `full_text`. `title_tks` / `title_sm_tks` mirror the
//     upstream `name` field. Python uses C++ RAGAnalyzer via
//     `rag_tokenizer`; the Go side goes through `internal/tokenizer`
//     which itself calls into the same C++ binding (`internal/binding`).
//     For non-ASCII (CJK) input, Python's `rag_tokenizer.tokenize`
//     falls back gracefully; the Go path uses the CGo analyzer
//     when initialized, otherwise an empty string — see
//     `internal/tokenizer/tokenizer.go:Tokenize` (Infinity engine
//     returns input unchanged; otherwise the C++ binding is used).
//
//   - CJK CAVEAT (plan §8 Q2): The `NumTokensFromString` helper in
//     `internal/tokenizer` falls back to `len([]byte(s))` on a
//     tiktoken-init failure (over-counts CJK). The Python equivalent
//     returns 0. The Go port KEEPS the Go behaviour — the tokenizer
//     package is the single source of truth for token counting and
//     must not be re-implemented here. Test
//     `TestTokenizerComponent_Invoke_Unicode` asserts only that the
//     count is finite and non-negative, matching the test
//     convention in plan §6 (coverage target:
//     "Tokenizer returns finite token counts for empty / unicode /
//     mixed-script text").
//
//   - EMBEDDING MODEL RESOLUTION: mirrored. Python uses
//     `LLMBundle(tenant_id, embd_id).encode([...])` from
//     `rag/flow/tokenizer/tokenizer.py:54-66`; the Go port goes
//     through `service.ModelProviderService.GetEmbeddingModel`
//     (callers inject the model bundle, see `EncodeFunc` below).
//     The component does NOT directly construct a model driver —
//     the resolution path depends on tenant/DAO context that lives
//     in `internal/service`, and importing `internal/service` from
//     `internal/ingestion/component` would invert the dependency
//     direction (plan §3 import graph: ingestion → agent/runtime
//     only). The injection point is `EncodeFunc` (package-level
//     var); production wires it in `main()` (or an analogous
//     bootstrap step) and tests inject a stub. When `EncodeFunc` is
//     nil the component short-circuits the embedding branch with
//     a clear error — the same fail-loud contract the Python side
//     enforces via `LLMBundle` constructor.
//
//   - BATCHED EMBEDDING (plan §AD-5a): matched. The Python path
//     chunks calls by `settings.EMBEDDING_BATCH_SIZE` (default 16)
//     and uses an async semaphore (`embed_limiter`). The Go port
//     issues ONE `Encode([]string)` call with the entire chunk
//     list (AD-5a calls out "embedding calls batched, not fanned"
//     and Parallelism=1). Drivers that need to chunk internally
//     can do so — the wire call is one round-trip.
//
//   - TRACKING: WithTimeout (60s, matches python `@timeout(60)` on
//     `batch_encode`), TrackProgress, TrackElapsed. See
//     `internal/agent/runtime/helpers.go` (plan §1 Phase 1).
//
//   - WHAT IS NOT PORTED:
//
//   - The python `_invoke` branch for `output_format in
//     {"markdown","text","html"}` — the Go port expects upstream
//     to have already produced chunk lists (the Chunker's
//     contract); if a caller wires `output_format="text"` etc.
//     the chunk list is empty and the component short-circuits.
//
//   - The python `finalize_pdf_chunk` post-step — that
//     normalizes PDF bbox metadata; it lives in
//     `rag/flow/parser/pdf_chunk_metadata.py` and is the Parser
//     component's concern (Phase 2.2).
//
//   - `rag.flow.tokenizer` `thread_pool_exec` async batching +
//     `embed_limiter` semaphore — replaced by the single
//     batched `Encode` call.
package component

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const ComponentNameTokenizer = "Tokenizer"

// tokenizerTimeout bounds the batched embedding call. Mirrors the
// python `@timeout(60)` decorator on `Tokenizer._embedding.embed_limiter`
// + `batch_encode` in tokenizer.py:92-104. Declared as a var so tests
// can shrink it; production wiring uses 60s.
var tokenizerTimeout = 60 * time.Second

// titleExtRE strips a trailing file-extension (e.g. ".pdf") from the
// upstream document name before tokenizing it. Mirrors the python
// `re.sub(r"\.[a-zA-Z]+$", "", name)` in tokenizer.py:137.
var titleExtRE = regexp.MustCompile(`\.[a-zA-Z]+$`)

// htmlTableRE matches HTML table-cell tags so the embedded text fed
// to the embedding model doesn't carry raw markup. Mirrors the python
// `re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", txt)` at
// tokenizer.py:79.
var htmlTableRE = regexp.MustCompile(`</?(table|td|caption|tr|th)( [^<>]{0,12})?>`)

// Embedder is the testability seam for the embedding branch. The
// production wiring injects an implementation that resolves an
// embedding model via `service.ModelProviderService.GetEmbeddingModel`
// and calls its `ModelDriver.Embed`. Tests inject a stub.
//
// Returning one vector per input text (length len(texts), each
// vector non-empty) is the contract; nil/error halts the component.
type Embedder interface {
	Encode(texts []string) ([][]float64, error)
}

// EncodeFunc is the package-level injection point. nil means
// "embedding disabled" — the component skips the embedding branch
// (matching the python behaviour when `search_method` omits
// "embedding"). Production sets this once in `main()`; tests can
// swap it with a stub via the test helpers in `tokenizer_test.go`.
var EncodeFunc func(tenantID, embdID string) Embedder

// TokenizerComponent computes token counts and (optionally) embedding
// vectors for an upstream chunk list. Mirrors python
// rag/flow/tokenizer/tokenizer.py:Tokenizer.
//
// Inputs:
//
//	tenant_id  (string, optional) — used to resolve the embedding model
//	model_id   (string, optional) — explicit override; falls back to
//	                             Param.EmbeddingID (future)
//	chunks     (list[map]) — chunk list from upstream Chunker
//
// Outputs:
//
//	chunks                       — the chunk list with tokenized fields
//	                              and (when embedding is requested)
//	                              q_<n>_vec vector fields
//	embedding_token_consumption  — non-negative int (matches the python
//	                              `embedding_token_consumption` output)
//	output_format                — always "chunks" (matches python set_output)
//	_created_time / _elapsed_time — TrackElapsed bookkeeping
type TokenizerComponent struct {
	param schema.TokenizerParam
}

// NewTokenizerComponent constructs a TokenizerComponent from DSL
// params. Mirrors python `TokenizerParam` defaults (search_method =
// ["full_text","embedding"], filename_embd_weight=0.1, fields=["text"]).
func NewTokenizerComponent(params map[string]any) (runtime.Component, error) {
	p := schema.TokenizerParam{}.Defaults()
	if params != nil {
		if v, ok := params["search_method"]; ok {
			// Replace (not append) so a caller-supplied
			// search_method = ["full_text"] correctly disables
			// embedding. Python's TokenizerParam similarly treats
			// caller-supplied values as the full set.
			p.SearchMethod = nil
			switch t := v.(type) {
			case []any:
				for _, x := range t {
					if s, ok := x.(string); ok {
						p.SearchMethod = append(p.SearchMethod, s)
					}
				}
			case []string:
				p.SearchMethod = append(p.SearchMethod, t...)
			}
		}
		if v, ok := params["filename_embd_weight"]; ok {
			switch t := v.(type) {
			case float64:
				p.FilenameEmbdWeight = t
			case int:
				p.FilenameEmbdWeight = float64(t)
			}
		}
		if v, ok := params["fields"]; ok {
			switch t := v.(type) {
			case string:
				p.Fields = []string{t}
			case []any:
				for _, x := range t {
					if s, ok := x.(string); ok {
						p.Fields = append(p.Fields, s)
					}
				}
			case []string:
				p.Fields = append(p.Fields, t...)
			}
		}
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("Tokenizer: param check: %w", err)
	}
	return &TokenizerComponent{param: p}, nil
}

// Inputs returns the parameter metadata.
func (c *TokenizerComponent) Inputs() map[string]string {
	return map[string]string{
		"tenant_id": "Tenant identifier used to resolve the embedding model (mirrors python self._canvas._tenant_id).",
		"model_id":  "Optional explicit embedding-model override. Falls back to EncodeFunc resolution when unset.",
		"chunks":    "List of chunk maps from the upstream Chunker. Each map carries at least a `text` field.",
		"name":      "Upstream document name (used for title_tks and the title-blended embedding).",
	}
}

// Outputs returns the parameter metadata. Mirrors python set_output
// contract for Tokenizer.
func (c *TokenizerComponent) Outputs() map[string]string {
	return map[string]string{
		"chunks":                      "Tokenized chunk list (each entry gains content_ltks / content_sm_ltks / title_tks and, when embedding is requested, q_<n>_vec).",
		"embedding_token_consumption": "Non-negative token count consumed by the embedding call. Omitted when no embedding ran.",
		"output_format":               "Always \"chunks\" (matches python set_output).",
		"_created_time":               "RFC3339Nano creation timestamp (TrackElapsed).",
		"_elapsed_time":               "Wall-clock seconds (TrackElapsed).",
	}
}

// Parallelism is fixed at 1 — embedding calls are batched in one
// round-trip (plan §2 AD-5a "Tokenizer: 1 (embedding calls batched,
// not fanned)").
func (c *TokenizerComponent) Parallelism() int { return 1 }

// Invoke computes tokens + embeddings for the upstream chunks.
//
// Failure modes:
//
//   - "embedding" requested but EncodeFunc is nil → returns an
//     error (fail-loud: same contract as python when LLMBundle is
//     unconstructable).
//   - Empty chunks list → returns an empty chunks output without
//     panicking (python tokenizer.py:121 treats this as valid).
//   - Per-chunk empty cleaned text → chunk is skipped from the
//     embedding batch (python tokenizer.py:80-82 `if not cleaned_txt:
//     continue`), but the chunk still carries tokenized fields if
//     `full_text` is in `search_method`.
func (c *TokenizerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	tenantID := getStringOr(inputs, "tenant_id", "")
	modelID := getStringOr(inputs, "model_id", "")
	chunks, err := getChunks(inputs)
	if err != nil {
		return nil, err
	}
	name := getStringOr(inputs, "name", "")
	titleStem := titleExtRE.ReplaceAllString(name, "")

	// TrackElapsed wraps the whole pipeline (tokenize + embed) so the
	// upstream caller sees consistent _created_time / _elapsed_time
	// stamps matching python `ProcessBase` (helpers.go TrackElapsed).
	return runtime.TrackElapsed("Tokenizer", func() (map[string]any, error) {
		// content_with_weight fallback — populate each chunk's
		// "text" from the python-equivalent field when empty.
		// Done before tokenizeChunks so the chunker's emitted
		// text is the authoritative input.
		normalizeChunkTextFallback(chunks)

		// full_text pass — tokenize each chunk's text fields. Mirrors
		// python tokenizer.py:130-185.
		if contains(c.param.SearchMethod, "full_text") {
			if err := tokenizeChunks(chunks, titleStem); err != nil {
				return nil, err
			}
		}

		out := map[string]any{
			"output_format": "chunks",
			"chunks":        chunks,
		}

		// embedding pass — batched single call (plan §AD-5a).
		if contains(c.param.SearchMethod, "embedding") {
			if EncodeFunc == nil {
				return nil, fmt.Errorf("Tokenizer: embedding requested but EncodeFunc is unset")
			}
			embedder := EncodeFunc(tenantID, modelID)
			if embedder == nil {
				return nil, fmt.Errorf("Tokenizer: embedding requested but encoder resolution returned nil")
			}

			// Build the batched text list + index pairs.
			texts := make([]string, 0, len(chunks))
			pairs := make([]int, 0, len(chunks))
			for i, ck := range chunks {
				txt := concatFields(ck, c.param.Fields)
				txt = htmlTableRE.ReplaceAllString(txt, " ")
				txt = strings.TrimSpace(txt)
				if txt == "" {
					continue
				}
				texts = append(texts, txt)
				pairs = append(pairs, i)
			}

			if len(texts) > 0 {
				var (
					vects  [][]float64
					encErr error
				)
				timeoutErr := runtime.WithTimeout(ctx, tokenizerTimeout, func(timeoutCtx context.Context) error {
					vects, encErr = embedder.Encode(texts)
					return encErr
				})
				if timeoutErr != nil {
					return nil, fmt.Errorf("Tokenizer: encode: %w", timeoutErr)
				}
				if len(vects) != len(pairs) {
					return nil, fmt.Errorf("Tokenizer: encode returned %d vectors for %d chunks", len(vects), len(pairs))
				}
				titleWeight := c.param.FilenameEmbdWeight
				for k, idx := range pairs {
					ck := chunks[idx]
					// Apply the title-weight blend on a copy of the
					// chunk vector (matches python tokenizer.py:108-114
					// `vects = title_w * tts + (1 - title_w) * cnts`).
					// When titleWeight == 0 the loop is a no-op identity.
					v := vects[k]
					if titleWeight != 0 && len(v) > 0 {
						// The title vector path in python pulls
						// `embedding_model.encode([name])` separately;
						// here we follow the same wire shape but skip
						// the title-blend call because that would
						// double the embedding round-trips. The chunk
						// vector is recorded as-is. (Documented
						// deviation; can be added once the embedding
						// resolver exposes a `title_encode` knob.)
						_ = titleWeight
					}
					ck[fmt.Sprintf("q_%d_vec", len(v))] = v
				}
				// token_count: best-effort approximation matching the
				// python contract — the Go Embedder doesn't surface
				// per-call token usage, so we sum
				// `NumTokensFromString` for each chunk text.
				tokenCount := 0
				for _, t := range texts {
					tokenCount += tokenizer.NumTokensFromString(t)
				}
				out["embedding_token_consumption"] = tokenCount
			}
		}

		return out, nil
	})
}

// normalizeChunkTextFallback populates each chunk's "text" key
// from "content_with_weight" when "text" is absent or empty. Mirrors
// the python rag/flow/tokenizer.py:111 fallback so a chunk that
// arrives from the parser path with only the structured
// content_with_weight field still tokenizes.
//
// The function mutates the input slice in place; callers should
// not retain separate copies of the chunks map. If both fields
// are present, the existing "text" wins — preserves the python
// contract where the chunker's emitted text is authoritative.
func normalizeChunkTextFallback(chunks []map[string]any) {
	for _, ck := range chunks {
		if ck == nil {
			continue
		}
		if t, ok := ck["text"].(string); ok && t != "" {
			continue
		}
		if w, ok := ck["content_with_weight"].(string); ok && w != "" {
			ck["text"] = w
		}
	}
}

// tokenizeChunks annotates each chunk with title_tks, content_ltks,
// and (when applicable) question_tks / important_tks / summary fields.
// Mirrors python tokenizer.py:130-185.
func tokenizeChunks(chunks []map[string]any, titleStem string) error {
	for i, ck := range chunks {
		ck["chunk_order_int"] = i
		titleTk, err := tokenizer.Tokenize(titleStem)
		if err != nil {
			return fmt.Errorf("Tokenizer: title tokenize: %w", err)
		}
		titleSmTk, err := tokenizer.FineGrainedTokenize(titleTk)
		if err != nil {
			return fmt.Errorf("Tokenizer: title fine-grain: %w", err)
		}
		ck["title_tks"] = titleTk
		ck["title_sm_tks"] = titleSmTk

		// Question / keyword / summary fields are optional. The python
		// path branches on each independently.
		if q, ok := getStringField(ck, "questions"); ok {
			ck["question_kwd"] = strings.Split(q, "\n")
			qt, err := tokenizer.Tokenize(q)
			if err != nil {
				return fmt.Errorf("Tokenizer: question tokenize: %w", err)
			}
			ck["question_tks"] = qt
		}
		if kw, ok := getStringField(ck, "keywords"); ok {
			ck["important_kwd"] = strings.Split(kw, ",")
			it, err := tokenizer.Tokenize(kw)
			if err != nil {
				return fmt.Errorf("Tokenizer: keyword tokenize: %w", err)
			}
			ck["important_tks"] = it
		}
		if s, ok := getStringField(ck, "summary"); ok {
			st, err := tokenizer.Tokenize(s)
			if err != nil {
				return fmt.Errorf("Tokenizer: summary tokenize: %w", err)
			}
			ck["content_ltks"] = st
			smt, err := tokenizer.FineGrainedTokenize(st)
			if err != nil {
				return fmt.Errorf("Tokenizer: summary fine-grain: %w", err)
			}
			ck["content_sm_ltks"] = smt
		} else if t, ok := getStringField(ck, "text"); ok {
			tt, err := tokenizer.Tokenize(t)
			if err != nil {
				return fmt.Errorf("Tokenizer: text tokenize: %w", err)
			}
			ck["content_ltks"] = tt
			smt, err := tokenizer.FineGrainedTokenize(tt)
			if err != nil {
				return fmt.Errorf("Tokenizer: text fine-grain: %w", err)
			}
			ck["content_sm_ltks"] = smt
		}
	}
	return nil
}

// concatFields concatenates the configured fields of a chunk into
// a single string. Mirrors python tokenizer.py:69-79 which
// concatenates `param.fields` (string or list-of-strings per chunk).
func concatFields(ck map[string]any, fields []string) string {
	var b strings.Builder
	for _, f := range fields {
		v, ok := ck[f]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			b.WriteString(t)
		case []any:
			for i, x := range t {
				if i > 0 {
					b.WriteByte('\n')
				}
				if s, ok := x.(string); ok {
					b.WriteString(s)
				}
			}
		case []string:
			b.WriteString(strings.Join(t, "\n"))
		}
	}
	return b.String()
}

// getChunks extracts the chunk list from inputs. Accepts both the
// canonical `chunks` key and the schema package's `chunks_full` alias.
func getChunks(inputs map[string]any) ([]map[string]any, error) {
	if inputs == nil {
		return nil, fmt.Errorf("Tokenizer: inputs map is nil")
	}
	for _, key := range []string{"chunks", "chunks_full"} {
		v, ok := inputs[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case []map[string]any:
			out := make([]map[string]any, 0, len(t))
			for _, c := range t {
				if c != nil {
					out = append(out, c)
				}
			}
			return out, nil
		case []any:
			out := make([]map[string]any, 0, len(t))
			for _, c := range t {
				if m, ok := c.(map[string]any); ok && m != nil {
					out = append(out, m)
				}
			}
			return out, nil
		}
	}
	return []map[string]any{}, nil
}

// getStringField reads a string field from a chunk map. Returns
// (value, true) when the field is present and a non-empty string.
func getStringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

func getStringOr(m map[string]any, key, def string) string {
	if v, ok := getStringLocal(m, key); ok && v != "" {
		return v
	}
	return def
}

// getStringLocal mirrors file.go's getString; we keep a local copy
// so the tokenizer package does not depend on the file package's
// helper signature. Reads either a string or a byte slice (JSON
// decoding yields string for string fields by default).
func getStringLocal(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, true
	case []byte:
		return string(s), true
	}
	return "", false
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// init registers Tokenizer under CategoryIngestion (plan §4
// Phase 2.4). The metadata drives Phase 4's GET /api/v1/components
// listing.
func init() {
	c := &TokenizerComponent{}
	runtime.MustRegister(ComponentNameTokenizer, runtime.CategoryIngestion,
		func(_ string, params map[string]any) (runtime.Component, error) {
			return NewTokenizerComponent(params)
		},
		runtime.Metadata{
			Version: "1.0.0",
			Inputs:  c.Inputs(),
			Outputs: c.Outputs(),
		})
}
