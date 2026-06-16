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

// Package component — Phase 1 e2e stubs for v1 DSL components.
//
// The v1 fixture set under internal/agent/dsl/testdata/v1_examples
// references seven component names that the production Phase 1
// registry does not yet implement: Retrieval, TavilySearch, ExeSQL,
// Generate, Answer, Iteration, IterationItem. Real bodies for these
// require network / DB / iteration-engine work that is out of scope
// for the canvas compile + invoke e2e path. Without registrations,
// the canvas builder errors out at buildNodeBody with "factory:
// component: unknown component", which makes the fixture suite
// useless as a regression check on topology wiring.
//
// The seven stubs in this file give the e2e tests a registered
// factory for each name. Their bodies are deliberately trivial — they
// echo a stable, template-friendly output shape and never call the
// network or DB. They are NOT a substitute for the real
// implementations; the contract is "registered, non-panicking, and
// produces outputs downstream templates can resolve", not "do
// something useful". Real Retrieval, TavilySearch, ExeSQL, Generate,
// Answer, Iteration, IterationItem bodies land in subsequent phases
// (see plan §2.11.3 + §2.11.6) and will replace these stubs file
// for file.
//
// The seven names were chosen by enumerating the component_name
// values in the v1_examples fixtures (see dsl.v1Examples). Keeping
// the list in sync with the fixture set is a single-source-of-truth
// discipline: if a new fixture references a name not in this file,
// the e2e test's compile+invoke loop will surface the gap with a
// clear factory error.
package component

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
)

// ----- Retrieval -----

const componentNameRetrieval = "Retrieval"

// RetrievalStub is a Phase 1 placeholder for the v1 Retrieval
// component. It returns an empty `formalized_content` so downstream
// templates that reference `{retrieval:0@formalized_content}` resolve
// to an empty string. The real component (Dealer / KGSearch path,
// plan §2.11.3 row 9) replaces this stub when the port lands.
type RetrievalStub struct{}

// NewRetrievalStub constructs a Retrieval stub. params is accepted
// for API parity but unused at this stage (the real component will
// parse kb_ids / similarity_threshold / top_n from it).
func NewRetrievalStub(_ map[string]any) (Component, error) {
	return &RetrievalStub{}, nil
}

// Name returns the registered component name.
func (r *RetrievalStub) Name() string { return componentNameRetrieval }

// Invoke returns a stub result that downstream templates can
// resolve. `formalized_content` is the field the v1 fixtures
// reference; empty string is the safe Phase 1 value.
func (r *RetrievalStub) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"formalized_content": ""}, nil
}

// Stream mirrors Invoke as a single-chunk SSE stream.
func (r *RetrievalStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := r.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the v1 DSL param surface.
func (r *RetrievalStub) Inputs() map[string]string {
	return map[string]string{
		"kb_ids":                    "Knowledge base IDs to search over.",
		"similarity_threshold":      "Minimum vector similarity to include a chunk.",
		"keywords_similarity_weight": "BM25 vs vector blend factor (0 = pure vector, 1 = pure BM25).",
		"top_n":                     "Number of top chunks to keep after rerank.",
		"top_k":                     "Number of candidates to retrieve before rerank.",
		"rerank_id":                 "Optional rerank model identifier.",
		"empty_response":            "Fallback message when no chunks pass the threshold.",
	}
}

// Outputs returns the public output surface.
func (r *RetrievalStub) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered chunks for downstream LLM prompts.",
	}
}

// ----- TavilySearch -----

const componentNameTavilySearch = "TavilySearch"

// TavilySearchStub is a Phase 1 placeholder for the v1 TavilySearch
// tool. The real implementation (plan §2.11.6) calls the Tavily
// HTTP API; this stub returns an empty result so the canvas e2e
// flow runs without network access.
type TavilySearchStub struct{}

// NewTavilySearchStub constructs a TavilySearch stub.
func NewTavilySearchStub(_ map[string]any) (Component, error) {
	return &TavilySearchStub{}, nil
}

// Name returns the registered component name.
func (t *TavilySearchStub) Name() string { return componentNameTavilySearch }

// Invoke returns an empty `formalized_content` so downstream
// templates resolve.
func (t *TavilySearchStub) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"formalized_content": ""}, nil
}

// Stream mirrors Invoke.
func (t *TavilySearchStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := t.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the v1 DSL param surface.
func (t *TavilySearchStub) Inputs() map[string]string {
	return map[string]string{
		"api_key": "Tavily API key.",
		"query":   "Search query template (may reference {iterationitem:0@result}).",
	}
}

// Outputs returns the public output surface.
func (t *TavilySearchStub) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
	}
}

// ----- ExeSQL -----

const componentNameExeSQL = "ExeSQL"

// ExeSQLStub is a Phase 1 placeholder for the v1 ExeSQL component.
// The real implementation (plan §2.11.3 row 10) opens a MySQL
// connection and runs the user's SQL; this stub returns a fixed
// two-column schema so the e2e flow runs without a database.
type ExeSQLStub struct{}

// NewExeSQLStub constructs an ExeSQL stub.
func NewExeSQLStub(_ map[string]any) (Component, error) {
	return &ExeSQLStub{}, nil
}

// Name returns the registered component name.
func (e *ExeSQLStub) Name() string { return componentNameExeSQL }

// Invoke returns a stable two-column stub result. Downstream
// templates that render SQL output will see headers + an empty row
// — enough for the message surface to format a string.
func (e *ExeSQLStub) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{
		"columns": []string{"col1", "col2"},
		"rows":    [][]any{{"", ""}},
		"sql":     "",
	}, nil
}

// Stream mirrors Invoke.
func (e *ExeSQLStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := e.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the v1 DSL param surface.
func (e *ExeSQLStub) Inputs() map[string]string {
	return map[string]string{
		"database": "Database / schema name.",
		"username": "DB user.",
		"host":     "DB host.",
		"port":     "DB port.",
		"password": "DB password.",
		"top_n":    "Limit on rows returned.",
	}
}

// Outputs returns the public output surface.
func (e *ExeSQLStub) Outputs() map[string]string {
	return map[string]string{
		"columns": "Result-set column names.",
		"rows":    "Result-set rows (matrix form).",
		"sql":     "Resolved SQL string.",
	}
}

// ----- Generate -----

const componentNameGenerate = "Generate"

// GenerateStub is a Phase 1 placeholder for the v1 "Generate"
// component. The Python DSL used "Generate" for a non-tool-using
// chat call; the Go port renamed the canonical name to "LLM" (see
// llm.go) and registers "Generate" here as a thin alias that routes
// to the LLM factory. This way the v1 fixtures that still reference
// the old name compile and run identically to LLM-backed flows.
type GenerateStub struct {
	inner *LLMComponent
}

// NewGenerateStub constructs a Generate stub. params is forwarded to
// the LLM factory so Generate and LLM share the same param surface
// (llm_id, prompt, temperature, message_history_window_size, cite).
func NewGenerateStub(params map[string]any) (Component, error) {
	llmParams, err := buildLLMParamFromV1Params(params)
	if err != nil {
		return nil, fmt.Errorf("Generate: %w", err)
	}
	return &GenerateStub{inner: NewLLMComponent(llmParams)}, nil
}

// Name returns the registered component name.
func (g *GenerateStub) Name() string { return componentNameGenerate }

// Invoke delegates to the LLM component.
func (g *GenerateStub) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return g.inner.Invoke(ctx, inputs)
}

// Stream delegates to the LLM component.
func (g *GenerateStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	return g.inner.Stream(ctx, inputs)
}

// Inputs returns the v1 DSL param surface. Matches LLM's surface
// plus the v1-only message_history_window_size and cite.
func (g *GenerateStub) Inputs() map[string]string {
	return map[string]string{
		"llm_id":                    "LLM model identifier.",
		"prompt":                    "System / user prompt template.",
		"temperature":               "Sampling temperature (0 = greedy).",
		"message_history_window_size": "How many prior turns to include.",
		"cite":                      "Whether to include source citations in the output.",
	}
}

// Outputs returns the public output surface.
func (g *GenerateStub) Outputs() map[string]string {
	return map[string]string{
		"content": "Assistant text response.",
		"model":   "Resolved model identifier.",
		"tokens":  "Token count for the call.",
	}
}

// buildLLMParamFromV1Params converts the v1 Generate params shape
// into the LLMParam shape. v1 stores the user prompt under "prompt"
// (not "user_prompt") and the system prompt is sometimes empty (the
// system role is often folded into "prompt"). We map: prompt →
// UserPrompt, llm_id → ModelID, temperature → Temperature,
// base_url → BaseURL, api_key → APIKey.
func buildLLMParamFromV1Params(p map[string]any) (LLMParam, error) {
	out := LLMParam{}
	if v, ok := p["llm_id"].(string); ok {
		out.ModelID = v
	}
	if v, ok := p["prompt"].(string); ok {
		out.UserPrompt = v
	}
	if v, ok := p["temperature"].(float64); ok {
		out.Temperature = &v
	}
	if v, ok := p["max_tokens"].(float64); ok {
		i := int(v)
		out.MaxTokens = &i
	}
	if v, ok := p["api_key"].(string); ok {
		out.APIKey = v
	}
	if v, ok := p["base_url"].(string); ok {
		out.BaseURL = v
	}
	return out, nil
}

// ----- Answer -----

const componentNameAnswer = "Answer"

// AnswerStub is a Phase 1 placeholder for the v1 Answer component.
// Answer is the agent's "wait for user" node (it pairs with ExeSQL
// or Message in conversational flows). The real implementation
// pauses the run and resumes on user input; the stub returns an
// empty answer immediately so the e2e flow can complete.
type AnswerStub struct{}

// NewAnswerStub constructs an Answer stub.
func NewAnswerStub(_ map[string]any) (Component, error) {
	return &AnswerStub{}, nil
}

// Name returns the registered component name.
func (a *AnswerStub) Name() string { return componentNameAnswer }

// Invoke returns an empty answer. Real implementation will block
// until the user provides input; the stub is fire-and-forget so
// the e2e flow doesn't deadlock.
func (a *AnswerStub) Invoke(ctx context.Context, _ map[string]any) (map[string]any, error) {
	// Mirror the no-state-check pattern of Message/Retrieval: we
	// don't read state, but the signature must match.
	if _, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err != nil {
		return nil, fmt.Errorf("Answer: %w", err)
	}
	return map[string]any{"answer": ""}, nil
}

// Stream mirrors Invoke.
func (a *AnswerStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := a.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the v1 DSL param surface.
func (a *AnswerStub) Inputs() map[string]string {
	return map[string]string{
		"question": "Optional clarification question to surface to the user.",
	}
}

// Outputs returns the public output surface.
func (a *AnswerStub) Outputs() map[string]string {
	return map[string]string{
		"answer": "User's response text.",
	}
}

// ----- Iteration / IterationItem -----

const (
	componentNameIteration     = "Iteration"
	componentNameIterationItem = "IterationItem"
)

// IterationStub is a Phase 1 placeholder for the v1 Iteration
// parent. The real implementation lives in canvas/loop_subgraph.go
// and runs the body once per item. The stub returns a single empty
// item list so the body never fires, which is a safe Phase 1
// default for the e2e flow.
type IterationStub struct{}

// NewIterationStub constructs an Iteration stub.
func NewIterationStub(_ map[string]any) (Component, error) {
	return &IterationStub{}, nil
}

// Name returns the registered component name.
func (i *IterationStub) Name() string { return componentNameIteration }

// Invoke returns an empty iteration payload.
func (i *IterationStub) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"items": []any{}}, nil
}

// Stream mirrors Invoke.
func (i *IterationStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := i.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the v1 DSL param surface.
func (i *IterationStub) Inputs() map[string]string {
	return map[string]string{
		"items_ref": "Reference to the items source (e.g. \"{generate:0@structured_content}\").",
	}
}

// Outputs returns the public output surface.
func (i *IterationStub) Outputs() map[string]string {
	return map[string]string{
		"items": "Items to iterate over (resolved at run time).",
	}
}

// IterationItemStub is a Phase 1 placeholder for the body node of
// an Iteration. The real wiring (parent_id → child routing) is
// engine-side; the stub itself is a passthrough.
type IterationItemStub struct{}

// NewIterationItemStub constructs an IterationItem stub.
func NewIterationItemStub(_ map[string]any) (Component, error) {
	return &IterationItemStub{}, nil
}

// Name returns the registered component name.
func (it *IterationItemStub) Name() string { return componentNameIterationItem }

// Invoke returns a passthrough empty map.
func (it *IterationItemStub) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"result": ""}, nil
}

// Stream mirrors Invoke.
func (it *IterationItemStub) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := it.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the v1 DSL param surface.
func (it *IterationItemStub) Inputs() map[string]string {
	return map[string]string{
		"item": "The current iteration item, injected by the Iteration parent.",
	}
}

// Outputs returns the public output surface.
func (it *IterationItemStub) Outputs() map[string]string {
	return map[string]string{
		"result": "Body result for the current item.",
	}
}

// ----- registrations -----

// One init per file keeps the registrations grouped and visible.
// Each Register call panics on a duplicate (the registry enforces
// uniqueness), so accidental double-registration in a later refactor
// surfaces as a panic at init time, not as a silent override.
func init() {
	Register(componentNameRetrieval, NewRetrievalStub)
	Register(componentNameTavilySearch, NewTavilySearchStub)
	Register(componentNameExeSQL, NewExeSQLStub)
	Register(componentNameGenerate, NewGenerateStub)
	Register(componentNameAnswer, NewAnswerStub)
	Register(componentNameIteration, NewIterationStub)
	Register(componentNameIterationItem, NewIterationItemStub)
}
