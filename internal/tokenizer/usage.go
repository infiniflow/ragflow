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

// Package tokenizer — per-run token usage tracking.
//
// An agent run installs a mutable token usage accumulator on the context
// (via WithRunUsage) at the start of each turn. Every LLM call inside
// that run adds its usage (prompt/completion/total tokens) to the sink
// via RecordRunTokenUsage. At the end of the run, the service layer
// reads the accumulated totals and emits them in the workflow_finished
// SSE event.
//
// This mirrors Python's common.token_utils:
//   - token_usage_sink ContextVar → context.Context + runUsageKey
//   - langfuse_run_attrs ContextVar → context.Context + runAttrsKey
//   - record_run_token_usage() → RecordRunTokenUsage(ctx, ...)
//   - usage_from_response() → UsageFromMap(raw)
package tokenizer

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// CallUsage is ONE LLM call's token split inside a run. The aggregate (RunUsage)
// answers "what did this question cost"; this answers "where did the cost go" —
// which ReAct iteration, repair turn or auditor pass burned the tokens, and how
// the input/output mix moved as the context grew.
type CallUsage struct {
	// Seq is the 1-based call index within the run.
	Seq int `json:"seq"`
	// AtSeconds is the call's start relative to the run sink. Rounded to
	// milliseconds: enough to order calls, not enough to bloat the artefact.
	AtSeconds float64 `json:"at_seconds"`
	// Model is the model the call went to, when the caller knows it (the
	// explorer, the auditor and the synthesis fallback can use different ones).
	Model string `json:"model,omitempty"`
	// PromptTokens / CompletionTokens are the call's input and output split.
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Context key types — unexported to prevent direct external access.
type runUsageKeyType struct{}
type runAttrsKeyType struct{}

// RunUsage is the mutable per-run token usage accumulator installed on
// the context by the service layer at the start of a canvas turn.
// All fields are guarded by the embedded mutex because concurrent
// tool-calling goroutines (run_in_executor copies the context, so
// workers share the same sink) can race on read/modify/write.
type RunUsage struct {
	mu               sync.Mutex
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Calls            int
	// startedAt anchors CallUsage.AtSeconds; calls keeps one record per LLM
	// call so a per-question cost can be broken down turn by turn.
	startedAt time.Time
	calls     []CallUsage
}

// Add atomically adds a single LLM call's token counts to the sink.
// Safe to call concurrently from multiple goroutines.
func (u *RunUsage) Add(prompt, completion, total int) {
	u.AddFor("", prompt, completion, total)
}

// AddFor is Add plus the model that served the call, and records the call's own
// token split. Callers that know the model (the chat-model wrapper does) get a
// per-turn breakdown for free; the rest still get a record with an empty model.
func (u *RunUsage) AddFor(model string, prompt, completion, total int) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if prompt > 0 {
		u.PromptTokens += prompt
	}
	if completion > 0 {
		u.CompletionTokens += completion
	}
	if total > 0 {
		u.TotalTokens += total
	}
	u.Calls++
	at := 0.0
	if !u.startedAt.IsZero() {
		at = time.Since(u.startedAt).Seconds()
	}
	u.calls = append(u.calls, CallUsage{
		Seq:              u.Calls,
		AtSeconds:        float64(int(at*1000)) / 1000,
		Model:            model,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	})
}

// CallSnapshot returns a copy of the per-call records, oldest first.
func (u *RunUsage) CallSnapshot() []CallUsage {
	if u == nil {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.calls) == 0 {
		return nil
	}
	out := make([]CallUsage, len(u.calls))
	copy(out, u.calls)
	return out
}

// Snapshot returns a copy of the current cumulative counts.
func (u *RunUsage) Snapshot() (prompt, completion, total, calls int) {
	if u == nil {
		return 0, 0, 0, 0
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.Calls
}

// RunAttrs holds per-run Langfuse correlating attributes (session_id,
// user_id) installed on the context by the service layer.
type RunAttrs struct {
	SessionID string
	UserID    string
}

// WithRunUsage installs a fresh RunUsage sink on ctx. Should be called
// once at the start of a canvas turn.
func WithRunUsage(ctx context.Context) context.Context {
	return context.WithValue(ctx, runUsageKeyType{}, &RunUsage{startedAt: time.Now()})
}

// GetRunUsage retrieves the per-run token usage sink from ctx.
// Returns nil when no sink is installed (e.g. outside a canvas run).
func GetRunUsage(ctx context.Context) *RunUsage {
	if v := ctx.Value(runUsageKeyType{}); v != nil {
		if sink, ok := v.(*RunUsage); ok {
			return sink
		}
	}
	return nil
}

// WithRunAttrs installs Langfuse correlation attributes on ctx.
func WithRunAttrs(ctx context.Context, attrs *RunAttrs) context.Context {
	if attrs == nil {
		return ctx
	}
	return context.WithValue(ctx, runAttrsKeyType{}, attrs)
}

// GetRunAttrs retrieves the per-run Langfuse attributes from ctx.
func GetRunAttrs(ctx context.Context) *RunAttrs {
	if v := ctx.Value(runAttrsKeyType{}); v != nil {
		if attrs, ok := v.(*RunAttrs); ok {
			return attrs
		}
	}
	return nil
}

// RecordRunTokenUsage adds a single LLM call's token usage to the
// active run sink on ctx. Safe to call from anywhere; when no run sink
// is installed it is a no-op.
func RecordRunTokenUsage(ctx context.Context, promptTokens, completionTokens, totalTokens int) {
	RecordRunTokenUsageFor(ctx, "", promptTokens, completionTokens, totalTokens)
}

// RecordRunTokenUsageFor is RecordRunTokenUsage plus the model that served the
// call, so the per-call breakdown can attribute tokens to the explorer, the
// auditor or the synthesis fallback.
func RecordRunTokenUsageFor(ctx context.Context, model string, promptTokens, completionTokens, totalTokens int) {
	sink := GetRunUsage(ctx)
	if sink == nil {
		return
	}
	sink.AddFor(model, promptTokens, completionTokens, totalTokens)
}

// UsageFromMap extracts a token usage split from a raw API response map.
// Handles OpenAI/OpenRouter-style resp["usage"] dicts, including the camelCase
// spelling (promptTokens/completionTokens/totalTokens) that some providers
// return. Missing fields default to 0; total_tokens falls back to
// prompt+completion when absent. Returns zeros when no usage found.
// Mirrors Python's common.token_utils.usage_from_response().
func UsageFromMap(raw map[string]interface{}) (promptTokens, completionTokens, totalTokens int) {
	if raw == nil {
		return 0, 0, 0
	}
	usageRaw, ok := raw["usage"]
	if !ok {
		return 0, 0, 0
	}
	usage, ok := usageRaw.(map[string]interface{})
	if !ok {
		return 0, 0, 0
	}
	pt := getInt(usage, "prompt_tokens", "input_tokens", "promptTokens")
	ct := getInt(usage, "completion_tokens", "output_tokens", "completionTokens")
	tt := getInt(usage, "total_tokens", "totalTokens")
	if tt == 0 {
		tt = pt + ct
	}
	return pt, ct, tt
}

func getInt(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case json.Number:
			n, err := val.Int64()
			if err == nil {
				return int(n)
			}
		}
	}
	return 0
}
