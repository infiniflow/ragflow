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
// production eino-based invoker) and internal/agent/tool / internal/agent/harness
// (which need to call the LLM for routing/selection) can depend on it without
// forming an import cycle. The production invoker is registered at boot via
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
}

// Response is the result of a chat call.
type Response struct {
	Content  string
	Thinking string
	Model    string
	Stopped  bool
	Tokens   int
}

// ErrNotConfigured is returned by GetDefaultInvoker when no production invoker
// has been installed. Callers should treat it as "no chat model available".
var ErrNotConfigured = &configError{"chat: default invoker not configured"}

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

var (
	mu           sync.RWMutex
	inst         Invoker
	defaultModel string
)

// SetDefaultInvoker installs the (production or test) invoker. Pass nil to
// restore the "not configured" state.
func SetDefaultInvoker(inv Invoker) {
	mu.Lock()
	defer mu.Unlock()
	inst = inv
}

// SetDefaultModelName records the tenant-default chat model name. The production
// invoker uses it when a Request omits ModelName, so harness/agentic-search LLM
// calls work without threading model config through every node.
func SetDefaultModelName(name string) {
	mu.Lock()
	defer mu.Unlock()
	defaultModel = name
}

// GetDefaultModelName returns the configured default model name ("" if unset).
func GetDefaultModelName() string {
	mu.RLock()
	defer mu.RUnlock()
	return defaultModel
}

// GetDefaultInvoker returns the installed invoker. It returns nil when no
// invoker has been installed (e.g. before server bootstrap or in tests that do
// not need the LLM).
func GetDefaultInvoker() Invoker {
	mu.RLock()
	defer mu.RUnlock()
	return inst
}
