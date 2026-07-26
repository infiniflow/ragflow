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
)

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
}

// Add atomically adds a single LLM call's token counts to the sink.
// Safe to call concurrently from multiple goroutines.
func (u *RunUsage) Add(prompt, completion, total int) {
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
	return context.WithValue(ctx, runUsageKeyType{}, &RunUsage{})
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
	sink := GetRunUsage(ctx)
	if sink == nil {
		return
	}
	sink.Add(promptTokens, completionTokens, totalTokens)
}

// UsageFromMap extracts a token usage split from a raw API response map.
// Handles OpenAI/OpenRouter-style resp["usage"] dicts. Missing fields
// default to 0; total_tokens falls back to prompt+completion when absent.
// Returns nil when no usage found.
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
	pt := getInt(usage, "prompt_tokens", "input_tokens")
	ct := getInt(usage, "completion_tokens", "output_tokens")
	tt := getInt(usage, "total_tokens")
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
