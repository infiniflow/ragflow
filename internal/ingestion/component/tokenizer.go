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
//     (callers inject the resolver, see `DefaultEmbedderResolver`).
//     The component does NOT directly construct a model driver —
//     the resolution path depends on tenant/DAO context that lives
//     in `internal/service`, and importing `internal/service` from
//     `internal/ingestion/component` would invert the dependency
//     direction (plan §3 import graph: ingestion → agent/runtime
//     only). The injection point is `DefaultEmbedderResolver`
//     (package-level var); the ingestion task package wires it in
//     its init() and tests inject a stub via the test-only
//     NewTokenizerComponentWithResolver. When no resolver is
//     available the component short-circuits the embedding branch
//     with a clear error — the same fail-loud contract the Python
//     side enforces via `LLMBundle` constructor.
//
//   - BATCHED EMBEDDING (plan §AD-5a): matched. The Python path
//     chunks calls by `settings.EMBEDDING_BATCH_SIZE` (default 16)
//     and uses an async semaphore (`embed_limiter`). The Go port
//     issues ONE `Encode([]string)` call with the entire chunk
//     list (AD-5a calls out "embedding calls batched, not fanned").
//     Drivers that need to chunk internally can do so — the wire
//     call is one round-trip.
//
//   - TRACKING: WithTimeout (60s, matches python `@timeout(60)` on
//     `batch_encode`), TrackProgress, TrackElapsed. See
//     `internal/agent/runtime/helpers.go` (plan §1 Phase 1).
//
//   - WHAT IS NOT PORTED:
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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

const ComponentNameTokenizer = "Tokenizer"

// tokenizerTimeout returns the per-batch timeout for embedding API calls.
// Reads COMPONENT_EXEC_TIMEOUT_TOKENIZER env var (seconds); defaults to 600s
// (10 min) to match the canvas-level component timeout default.
// Invalid / non-positive values fall back to the default.
func tokenizerTimeout() time.Duration {
	if v := os.Getenv("COMPONENT_EXEC_TIMEOUT_TOKENIZER"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultTokenizerTimeout
}

var defaultTokenizerTimeout = 600 * time.Second

// tokenizerEmbeddingBatchSize mirrors Python's
// settings.EMBEDDING_BATCH_SIZE default.
var tokenizerEmbeddingBatchSize = 16

// titleExtRE strips a trailing file-extension (e.g. ".pdf") from the
// upstream document name before tokenizing it. Mirrors the python
// `re.sub(r"\.[a-zA-Z]+$", "", name)` in tokenizer.py:137.
var titleExtRE = regexp.MustCompile(`\.[a-zA-Z]+$`)

// htmlTableRE matches HTML table-cell tags so the embedded text fed
// to the embedding model doesn't carry raw markup. Mirrors the python
// `re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", txt)` at
// tokenizer.py:79.
var htmlTableRE = regexp.MustCompile(`</?(table|td|caption|tr|th)( [^<>]{0,12})?>`)

// EmbeddingResult carries a vector plus the model-reported token usage
// for that input batch entry.
type EmbeddingResult struct {
	Vector     []float64
	TokenCount int
}

// Embedder is the testability seam for the embedding branch.
type Embedder interface {
	MaxTokens() int
	Encode(texts []string) ([]EmbeddingResult, error)
}

// EmbedderResolver resolves the embedder for one tokenizer invocation.
// embeddingModel is the Tokenizer-scoped embedding-model identifier (from the
// component's setups); an empty value tells the resolver to fall back to the
// dataset's configured model.
type EmbedderResolver func(tenantID, kbID, embeddingModel string) (Embedder, error)

// DefaultEmbedderResolver is the production embedder resolver. It is nil in
// this leaf package — which must not import internal/service (see the
// EMBEDDING MODEL RESOLUTION note above) — and is injected by the composition
// root: the ingestion task package wires a resolver backed by the model
// provider in its init(). NewTokenizerComponent falls back to this resolver
// when no explicit (test-only) resolver is supplied.
var DefaultEmbedderResolver EmbedderResolver

// TokenizerComponent computes token counts and (optionally) embedding
// vectors for an upstream chunk list. Mirrors python
// rag/flow/tokenizer/tokenizer.py:Tokenizer.
//
// Inputs:
//
//	tenant_id  (string, optional) — used to resolve the embedding model
//	kb_id      (string, optional) — dataset whose embd_id is used when the
//	                             setups embedding_model is unset
//	output_format (string) — one of json/markdown/text/html/chunks
//	chunks        (list[map]) — chunk list when output_format == "chunks"
//	json          (list[map]) — structured parser payload when output_format == "json" or unset
//	markdown/text/html        — scalar payload matching output_format
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
	param          schema.TokenizerParam
	resolver       EmbedderResolver
	embeddingModel string
}

// NewTokenizerComponent constructs a production TokenizerComponent from DSL
// params. Mirrors python `TokenizerParam` defaults (search_method =
// ["full_text","embedding"], filename_embd_weight=0.1, fields=["text"]). The
// embedding branch resolves its embedder via the injected
// DefaultEmbedderResolver (wired by the ingestion task package).
func NewTokenizerComponent(params map[string]any) (runtime.Component, error) {
	return newTokenizerComponent(params, nil)
}

// NewTokenizerComponentWithResolver is TEST-ONLY. It injects an explicit
// embedder resolver so unit/integration tests can stub the embedding backend
// without touching the model provider. Production code MUST use
// NewTokenizerComponent and rely on DefaultEmbedderResolver instead.
func NewTokenizerComponentWithResolver(params map[string]any, resolver EmbedderResolver) (runtime.Component, error) {
	return newTokenizerComponent(params, resolver)
}

func newTokenizerComponent(params map[string]any, resolver EmbedderResolver) (runtime.Component, error) {
	p := schema.TokenizerParam{}.Defaults()
	embeddingModel := ""
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
		embeddingModel = embeddingModelFromSetups(params)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("Tokenizer: param check: %w", err)
	}
	return &TokenizerComponent{param: p, resolver: resolver, embeddingModel: embeddingModel}, nil
}

// embeddingModelFromSetups extracts the embedding-model identifier from the
// component's setups map (params["setups"]["embedding_model"]). The embedding
// model id is a Tokenizer-scoped setup rather than a run-level global so it is
// never mistaken for, e.g., a chat model id shared across components. Empty
// when unset — the resolver then falls back to the dataset's configured model.
func embeddingModelFromSetups(params map[string]any) string {
	setups, ok := params["setups"].(map[string]any)
	if !ok {
		return ""
	}
	if v, ok := setups["embedding_model"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// Inputs returns the parameter metadata.
func (c *TokenizerComponent) Inputs() map[string]string {
	return map[string]string{
		"tenant_id":     "Tenant identifier used to resolve the embedding model (mirrors python self._canvas._tenant_id).",
		"kb_id":         "Optional knowledgebase identifier used to resolve the bound embedding model when the setups embedding_model is unset.",
		"output_format": "Upstream payload discriminator: json / markdown / text / html / chunks.",
		"chunks":        "List of chunk maps when output_format == \"chunks\".",
		"json":          "Structured parser payload when output_format == \"json\" or unset.",
		"text":          "Plain-text payload when output_format == \"text\".",
		"markdown":      "Markdown payload when output_format == \"markdown\".",
		"html":          "HTML payload when output_format == \"html\".",
		"name":          "Upstream document name (used for title_tks and the title-blended embedding).",
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

// Invoke computes tokens + embeddings for the upstream chunks.
//
// Failure modes:
//
//   - "embedding" requested but resolver is nil → returns an
//     error (fail-loud: same contract as python when LLMBundle is
//     unconstructable).
//   - Empty chunks list → returns an empty chunks output without
//     panicking (python tokenizer.py:121 treats this as valid).
//   - Per-chunk empty cleaned text → chunk is skipped from the
//     embedding batch (python tokenizer.py:80-82 `if not cleaned_txt:
//     continue`), but the chunk still carries tokenized fields if
//     `full_text` is in `search_method`.
func (c *TokenizerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// Run-level metadata lives in the workflow-wide CanvasState.Globals
	// bag (seeded at pipeline start, published by the File component),
	// not in the upstream output map — see GlobalOrInput.
	name := globals.GlobalOrInput(ctx, inputs, "name", "")
	tenantID := globals.GlobalOrInput(ctx, inputs, "tenant_id", "")
	kbID := globals.GlobalOrInput(ctx, inputs, "kb_id", "")
	// The embedding-model id is a Tokenizer-scoped setup (params["setups"]),
	// resolved at construction, not a run-level global — see
	// embeddingModelFromSetups.
	embeddingModel := c.embeddingModel

	// decodeTokenizerFromUpstream validates `name`; carry the resolved
	// name into the decode input so both a Globals-backed run and a
	// headless run (no Globals attached) satisfy it.
	decInputs := inputs
	if name != "" {
		decInputs = cloneInputs(inputs)
		decInputs["name"] = name
	}
	upstream, err := decodeTokenizerFromUpstream(decInputs)
	if err != nil {
		return nil, err
	}
	chunks := chunksFromTokenizerUpstream(upstream)
	common.Debug("tokenizer stage",
		zap.String("component", "Tokenizer"),
		zap.Int("input_chunks", len(chunks)),
	)
	titleStem := titleExtRE.ReplaceAllString(name, "")

	normalizeChunkTextFallback(chunks)

	if contains(c.param.SearchMethod, "full_text") {
		if err := tokenizeChunks(chunks, titleStem); err != nil {
			return nil, err
		}
	}

	out := map[string]any{
		"output_format": "chunks",
		"chunks":        schema.ChunkDocsToMaps(chunks),
	}

	if contains(c.param.SearchMethod, "embedding") {
		chunks, tokenCount, err := c.embedChunks(ctx, tenantID, kbID, embeddingModel, name, chunks)
		if err != nil {
			return nil, err
		}
		out["embedding_token_consumption"] = tokenCount
		out["chunks"] = schema.ChunkDocsToMaps(chunks)
	}
	if err := validateTokenizerOutputs(chunks, c.param.SearchMethod, c.param.Fields); err != nil {
		return nil, err
	}

	common.Debug("tokenizer stage",
		zap.String("component", "Tokenizer"),
		zap.Int("output_chunks", len(chunks)),
	)
	return out, nil
}

func (c *TokenizerComponent) embedChunks(ctx context.Context, tenantID, kbID, embeddingModel, name string, chunks []schema.ChunkDoc) ([]schema.ChunkDoc, int, error) {
	if len(chunks) == 0 {
		return chunks, 0, nil
	}
	// An explicit (test-only) resolver wins; production wiring leaves it nil
	// and falls back to the injected DefaultEmbedderResolver.
	resolver := c.resolver
	if resolver == nil {
		resolver = DefaultEmbedderResolver
	}
	if resolver == nil {
		return nil, 0, fmt.Errorf("Tokenizer: embedding requested but no embedder resolver configured")
	}
	embedder, err := resolver(tenantID, kbID, embeddingModel)
	if err != nil {
		return nil, 0, fmt.Errorf("Tokenizer: resolve embedder: %w", err)
	}
	if embedder == nil {
		return nil, 0, fmt.Errorf("Tokenizer: embedding requested but encoder resolution returned nil")
	}

	texts := make([]string, 0, len(chunks))
	pairs := make([]int, 0, len(chunks))
	for i, ck := range chunks {
		txt := concatFields(ck, c.param.Fields)
		txt = htmlTableRE.ReplaceAllString(txt, " ")
		txt = strings.TrimSpace(txt)
		if txt == "" {
			continue
		}
		texts = append(texts, truncateForEmbedding(txt, embedder.MaxTokens()))
		pairs = append(pairs, i)
	}
	if len(texts) == 0 {
		return chunks, 0, nil
	}

	trimmedName := strings.TrimSpace(name)
	var (
		titleVec    []float64
		tokenCount  int
		hasTitleVec bool
	)
	if trimmedName == "" {
		log.Printf("Tokenizer: empty name provided from upstream, embedding will skip title weighting")
	} else {
		titleResults, err := encodeWithTimeout(ctx, embedder, []string{trimmedName})
		if err != nil {
			return nil, 0, fmt.Errorf("Tokenizer: encode title: %w", err)
		}
		if len(titleResults) != 1 {
			return nil, 0, fmt.Errorf("Tokenizer: encode title returned %d vectors for 1 chunk", len(titleResults))
		}
		titleVec = titleResults[0].Vector
		tokenCount = titleResults[0].TokenCount
		hasTitleVec = true
	}

	contentResults := make([]EmbeddingResult, 0, len(texts))
	for start := 0; start < len(texts); start += tokenizerEmbeddingBatchSize {
		end := start + tokenizerEmbeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchResults, err := encodeWithTimeout(ctx, embedder, texts[start:end])
		if err != nil {
			return nil, 0, fmt.Errorf("Tokenizer: encode: %w", err)
		}
		if len(batchResults) != end-start {
			return nil, 0, fmt.Errorf("Tokenizer: encode returned %d vectors for %d chunks", len(batchResults), end-start)
		}
		for _, result := range batchResults {
			tokenCount += result.TokenCount
		}
		contentResults = append(contentResults, batchResults...)
	}

	titleWeight := c.param.FilenameEmbdWeight
	for i, idx := range pairs {
		merged := append([]float64(nil), contentResults[i].Vector...)
		if hasTitleVec {
			merged, err = mergeEmbeddingVectors(titleVec, contentResults[i].Vector, titleWeight)
			if err != nil {
				return nil, 0, fmt.Errorf("Tokenizer: merge vectors: %w", err)
			}
		}
		if err := chunks[idx].SetExtraValue(fmt.Sprintf("q_%d_vec", len(merged)), merged); err != nil {
			return nil, 0, fmt.Errorf("Tokenizer: vector marshal: %w", err)
		}
	}
	return chunks, tokenCount, nil
}

func encodeWithTimeout(ctx context.Context, embedder Embedder, texts []string) ([]EmbeddingResult, error) {
	var (
		results []EmbeddingResult
		encErr  error
	)
	timeoutErr := runtime.WithTimeout(ctx, tokenizerTimeout(), func(timeoutCtx context.Context) error {
		results, encErr = embedder.Encode(texts)
		return encErr
	})
	if timeoutErr != nil {
		return nil, timeoutErr
	}
	return results, nil
}

func truncateForEmbedding(text string, maxTokens int) string {
	if maxTokens <= 10 {
		return text
	}
	return tokenizer.TrimContentToTokenLimit(text, maxTokens-10)
}

func mergeEmbeddingVectors(titleVec, contentVec []float64, titleWeight float64) ([]float64, error) {
	if len(titleVec) == 0 || len(contentVec) == 0 {
		return nil, fmt.Errorf("empty embedding vector")
	}
	if len(titleVec) != len(contentVec) {
		return nil, fmt.Errorf("unexpected embedding dimensions")
	}
	merged := make([]float64, len(titleVec))
	for i := range titleVec {
		merged[i] = titleWeight*titleVec[i] + (1-titleWeight)*contentVec[i]
	}
	return merged, nil
}

func decodeTokenizerFromUpstream(inputs map[string]any) (schema.TokenizerFromUpstream, error) {
	var out schema.TokenizerFromUpstream
	if inputs == nil {
		return out, fmt.Errorf("Tokenizer: inputs map is nil")
	}
	data, err := json.Marshal(stripRuntimeTimestamps(inputs))
	if err != nil {
		return out, fmt.Errorf("Tokenizer: encode inputs: %w", err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("Tokenizer: decode inputs: %w", err)
	}
	if err := out.Validate(); err != nil {
		return out, fmt.Errorf("Tokenizer: input error: %w", err)
	}
	return out, nil
}

func stripRuntimeTimestamps(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		if k == "_created_time" || k == "_elapsed_time" {
			continue
		}
		out[k] = v
	}
	return out
}

func chunksFromTokenizerUpstream(in schema.TokenizerFromUpstream) []schema.ChunkDoc {
	switch in.OutputFormat {
	case schema.PayloadFormatChunks:
		return cloneChunkDocs(in.Chunks)
	case schema.PayloadFormatMarkdown:
		return textPayloadToChunks(in.MarkdownResult)
	case schema.PayloadFormatText:
		return textPayloadToChunks(in.TextResult)
	case schema.PayloadFormatHTML:
		return textPayloadToChunks(in.HTMLResult)
	default:
		return cloneChunkDocs(in.JSONResult)
	}
}

func textPayloadToChunks(payload *string) []schema.ChunkDoc {
	if payload == nil || strings.TrimSpace(*payload) == "" {
		return []schema.ChunkDoc{}
	}
	return []schema.ChunkDoc{{Text: *payload}}
}

func cloneChunkDocs(in []schema.ChunkDoc) []schema.ChunkDoc {
	if len(in) == 0 {
		return []schema.ChunkDoc{}
	}
	out := make([]schema.ChunkDoc, len(in))
	for i := range in {
		out[i] = cloneTokenizerChunkDoc(in[i])
	}
	return out
}

func cloneTokenizerChunkDoc(in schema.ChunkDoc) schema.ChunkDoc {
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
	if len(in.PDFPositions) > 0 {
		out.PDFPositions = append(json.RawMessage(nil), in.PDFPositions...)
	}
	if len(in.Positions) > 0 {
		out.Positions = append(json.RawMessage(nil), in.Positions...)
	}
	return out
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
func normalizeChunkTextFallback(chunks []schema.ChunkDoc) {
	for i := range chunks {
		if chunks[i].Text != "" {
			continue
		}
		if chunks[i].ContentWithWeight != "" {
			chunks[i].Text = chunks[i].ContentWithWeight
		}
	}
}

// tokenizeChunks annotates each chunk with title_tks, content_ltks,
// and (when applicable) question_tks / important_tks / summary fields.
// Mirrors python tokenizer.py:130-185.
func tokenizeChunks(chunks []schema.ChunkDoc, titleStem string) error {
	for i := range chunks {
		ck := &chunks[i]
		ck.ChunkOrderInt = intPtr(i)
		titleTk, err := tokenizer.Tokenize(titleStem)
		if err != nil {
			return fmt.Errorf("Tokenizer: title tokenize: %w", err)
		}
		titleSmTk, err := tokenizer.FineGrainedTokenize(titleTk)
		if err != nil {
			return fmt.Errorf("Tokenizer: title fine-grain: %w", err)
		}
		ck.TitleTks = titleTk
		ck.TitleSmTks = titleSmTk

		// Question / keyword / summary fields are optional. The python
		// path branches on each independently.
		if q := ck.Questions; q != "" {
			if err := ck.SetExtraValue("question_kwd", strings.Split(q, "\n")); err != nil {
				return fmt.Errorf("Tokenizer: question keywords marshal: %w", err)
			}
			qt, err := tokenizer.Tokenize(q)
			if err != nil {
				return fmt.Errorf("Tokenizer: question tokenize: %w", err)
			}
			if err := ck.SetExtraValue("question_tks", qt); err != nil {
				return fmt.Errorf("Tokenizer: question tokens marshal: %w", err)
			}
		}
		if kw := ck.Keywords; kw != "" {
			if err := ck.SetExtraValue("important_kwd", utility.SplitKeywords(kw)); err != nil {
				return fmt.Errorf("Tokenizer: keyword list marshal: %w", err)
			}
			it, err := tokenizer.Tokenize(kw)
			if err != nil {
				return fmt.Errorf("Tokenizer: keyword tokenize: %w", err)
			}
			if err := ck.SetExtraValue("important_tks", it); err != nil {
				return fmt.Errorf("Tokenizer: keyword tokens marshal: %w", err)
			}
		}
		if s := ck.Summary; strings.TrimSpace(s) != "" {
			st, err := tokenizer.Tokenize(s)
			if err != nil {
				return fmt.Errorf("Tokenizer: summary tokenize: %w", err)
			}
			if st == "" {
				st = s
			}
			ck.ContentLtks = st
			smt, err := tokenizer.FineGrainedTokenize(st)
			if err != nil {
				return fmt.Errorf("Tokenizer: summary fine-grain: %w", err)
			}
			if smt == "" {
				smt = st
			}
			ck.ContentSmLtks = smt
		} else if t := ck.Text; strings.TrimSpace(t) != "" {
			tt, err := tokenizer.Tokenize(t)
			if err != nil {
				return fmt.Errorf("Tokenizer: text tokenize: %w", err)
			}
			if tt == "" {
				tt = t
			}
			ck.ContentLtks = tt
			smt, err := tokenizer.FineGrainedTokenize(tt)
			if err != nil {
				return fmt.Errorf("Tokenizer: text fine-grain: %w", err)
			}
			if smt == "" {
				smt = tt
			}
			ck.ContentSmLtks = smt
		}
	}
	return nil
}

// concatFields concatenates the configured fields of a chunk into
// a single string. Mirrors python tokenizer.py:69-79 which
// concatenates `param.fields` (string or list-of-strings per chunk).
func concatFields(ck schema.ChunkDoc, fields []string) string {
	var b strings.Builder
	for _, f := range fields {
		switch f {
		case "text":
			b.WriteString(ck.Text)
		case "content_with_weight":
			b.WriteString(ck.ContentWithWeight)
		case "questions":
			b.WriteString(ck.Questions)
		case "keywords":
			b.WriteString(ck.Keywords)
		case "summary":
			b.WriteString(ck.Summary)
		default:
			if s, ok := ck.GetExtraString(f); ok {
				b.WriteString(s)
				continue
			}
			if values, ok := ck.GetExtraStringSlice(f); ok {
				b.WriteString(strings.Join(values, "\n"))
			}
		}
	}
	return b.String()
}

func validateTokenizerOutputs(chunks []schema.ChunkDoc, searchMethods, fields []string) error {
	needFullText := contains(searchMethods, "full_text")
	needEmbedding := contains(searchMethods, "embedding")
	if !needFullText && !needEmbedding {
		return nil
	}
	for i := range chunks {
		if needFullText && requiresFullTextTokens(chunks[i]) {
			if strings.TrimSpace(chunks[i].ContentLtks) == "" || strings.TrimSpace(chunks[i].ContentSmLtks) == "" {
				return fmt.Errorf("Tokenizer: chunk[%d] missing full_text tokens", i)
			}
		}
		if needEmbedding && requiresEmbeddingVector(chunks[i], fields) {
			if !hasEmbeddingVector(chunks[i]) {
				return fmt.Errorf("Tokenizer: chunk[%d] missing embedding vector", i)
			}
		}
	}
	return nil
}

func requiresFullTextTokens(ck schema.ChunkDoc) bool {
	return strings.TrimSpace(ck.Summary) != "" || strings.TrimSpace(ck.Text) != ""
}

func requiresEmbeddingVector(ck schema.ChunkDoc, fields []string) bool {
	return strings.TrimSpace(cleanEmbeddingText(concatFields(ck, fields))) != ""
}

func cleanEmbeddingText(text string) string {
	return strings.TrimSpace(htmlTableRE.ReplaceAllString(text, " "))
}

func hasEmbeddingVector(ck schema.ChunkDoc) bool {
	if len(ck.Extra) == 0 {
		return false
	}
	for key, raw := range ck.Extra {
		if !strings.HasPrefix(key, "q_") || !strings.HasSuffix(key, "_vec") {
			continue
		}
		var vec []float64
		if err := json.Unmarshal(raw, &vec); err != nil {
			continue
		}
		if len(vec) > 0 {
			return true
		}
	}
	return false
}

func getStringOr(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

// cloneInputs returns a shallow copy of m with room for one extra key.
// Used to inject the Globals-resolved `name` into the decode input without
// mutating the caller's input snapshot.
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

func contains(s []string, v string) bool {
	return slices.Contains(s, v)
}

func intPtr(v int) *int { return &v }

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
