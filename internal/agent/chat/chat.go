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

// Package chat is a dependency-light home for the LLM chat-invoker interface
// and its package-level singleton. It lives here (leaf, importing only eino
// schema + gorm) so that both internal/agent/component (which owns the
// production eino-based invoker) and internal/agent/tool /
// internal/rag/advanced_rag/harness (which need to call the LLM for
// routing/selection) can depend on it without forming an import cycle. The production invoker is registered at boot via
// SetDefaultInvoker.
package chat

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// Invoker abstracts a chat-model call so callers can inject a stub in tests and
// production flows through the eino bridge. This is the shared seam for the LLM
// component and the agentic-search harness/tools.
type Invoker interface {
	Invoke(ctx context.Context, db *gorm.DB, req Request) (*Response, error)
}

// StreamingInvoker is implemented by invokers whose provider can emit the reply
// incrementally. Callers type-assert for it and fall back to Invoke when it is
// absent, so streaming stays an enhancement rather than a requirement.
//
// onDelta receives each piece as it arrives; isThink marks pieces of a hidden
// reasoning block, which must not be shown as part of the answer.
type StreamingInvoker interface {
	Invoker
	Stream(ctx context.Context, db *gorm.DB, req Request, onDelta func(delta string, isThink bool) error) (*Response, error)
}

// Tool is one callable declared to the provider for native tool calling. It
// mirrors the OpenAI tool object (type + function) so the harness can forward
// its existing ToolSpec surface without reshaping it.
type Tool struct {
	Type     string       `json:"type"` // "function" (only value providers accept today)
	Function ToolFunction `json:"function"`
}

// ToolFunction is the callable's name, description, and JSON-schema parameters.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolCall is one invocation the provider requested via native tool calling.
// It carries the arguments as a parsed map so the harness can dispatch without
// re-parsing a fenced block.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ToolChoice controls whether/how the provider may call tools. "" (unset) lets
// the provider decide; "auto" mirrors OpenAI tool_choice="auto"; "none" forbids
// calls; "required" forces at least one call.
type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = "auto"
	ToolChoiceNone     ToolChoice = "none"
	ToolChoiceRequired ToolChoice = "required"
)

// Request is the minimal surface needed to dispatch a chat call.
type Request struct {
	Driver           string
	ModelName        string
	APIKey           string
	BaseURL          string
	Messages         []schema.Message
	Temperature      *float64
	TopP             *float64
	PresencePenalty  *float64
	FrequencyPenalty *float64
	MaxTokens        *int
	Thinking         string // "enabled" | "disabled" | ""
	// Tools, when non-nil, switches the call from prompt-based tool parsing to
	// native provider tool calling. ToolChoice constrains the provider's use of
	// them (see ToolChoice). Nil means no tools are declared.
	Tools      []Tool
	ToolChoice ToolChoice
}

// Usage is the token usage split for one chat call, mirroring the provider's
// prompt/completion/total breakdown (Python token_utils.usage_from_response).
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Response is the result of a chat call.
type Response struct {
	Content  string
	Thinking string
	Model    string
	Stopped  bool
	// Tokens is the total token count (TotalTokens). Deprecated in favour of
	// Usage, which carries the prompt/completion split; kept so existing callers
	// that only total keep working unchanged.
	Tokens int
	// Usage carries the prompt/completion/total split when the provider reports
	// it. Nil when the provider reported nothing.
	Usage *Usage
	// ToolCalls carries native provider tool invocations when the request
	// declared Tools. Empty when the provider returned no calls (or the request
	// used prompt-based tooling). The harness falls back to prompt parsing when
	// this is empty and tools were requested.
	ToolCalls []ToolCall
}

// ErrNotConfigured is returned by GetDefaultInvoker when no production invoker
// has been installed. Callers should treat it as "no chat model available".
var ErrNotConfigured = &configError{"chat: default invoker not configured"}

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

var (
	mu   sync.RWMutex
	inst Invoker
)

// SetDefaultInvoker installs the (production or test) invoker. Pass nil to
// restore the "not configured" state.
func SetDefaultInvoker(inv Invoker) {
	mu.Lock()
	defer mu.Unlock()
	inst = inv
}

// GetDefaultInvoker returns the installed invoker. It returns nil when no
// invoker has been installed (e.g. before server bootstrap or in tests that do
// not need the LLM).
func GetDefaultInvoker() Invoker {
	mu.RLock()
	defer mu.RUnlock()
	return inst
}
