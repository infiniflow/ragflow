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

// Browser runs natural-language browser tasks through Stagehand in local mode.
// StagehandInvoker.RunTask navigates and interacts with pages before returning
// the agent's final result. The runtime owns the server and session lifecycle.
// File upload/download and persistent sessions are not supported.
package component

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity/models"

	"gorm.io/gorm"
)

const componentNameBrowser = "Browser"

var browserModelFactory func(context.Context, string, string) (*models.ChatModel, error)

// SetBrowserModelFactory binds Browser to the shared tenant-scoped model factory.
func SetBrowserModelFactory(factory func(context.Context, string, string) (*models.ChatModel, error)) {
	browserModelFactory = factory
}

// browserParam is the static DSL param surface for the Browser
// component. Mirrors Python `browser.py:LLMParam + browser knobs`:
//
//	llm_id, model_id (alias), prompts, prompt (alias),
//	max_steps, headless, enable_default_extensions,
//	chromium_sandbox, persist_session, upload_sources.
//
// Go-only fields kept for backward compat with the existing test
// file and the optional-URL form some operators still wire up:
//
//	url, timeout.
//
// v1 does not act on the v1-deferred params; Update accepts them so
// the v1 fixture loads.
type browserParam struct {
	LLMID            string `json:"llm_id"`
	ModelID          string `json:"model_id"` // alias for llm_id
	Prompts          string `json:"prompts"`
	Prompt           string `json:"prompt"` // alias for prompts
	MaxSteps         int    `json:"max_steps"`
	Headless         *bool  `json:"headless"`
	EnableDefaultExt *bool  `json:"enable_default_extensions"`
	ChromiumSandbox  *bool  `json:"chromium_sandbox"`
	PersistSession   *bool  `json:"persist_session"`

	// Go-only fields (kept for backward compat with the existing
	// test file; not used by the stagehand path).
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
}

// Update copies a fresh param map into the receiver. The
// `llm_id`/`model_id` and `prompts`/`prompt` alias pairs collapse
// onto the same field; the first non-empty value wins.
func (p *browserParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	if v, ok := stringFrom(conf, "llm_id"); ok && v != "" {
		p.LLMID = v
	}
	if v, ok := stringFrom(conf, "model_id"); ok && v != "" && p.LLMID == "" {
		p.LLMID = v
	}
	if v, ok := stringFrom(conf, "prompts"); ok && v != "" {
		p.Prompts = v
	}
	if v, ok := stringFrom(conf, "prompt"); ok && v != "" && p.Prompts == "" {
		p.Prompts = v
	}
	if v, ok := intFrom(conf, "max_steps"); ok {
		p.MaxSteps = v
	} else {
		p.MaxSteps = 0
	}
	if v, ok := boolFrom(conf, "headless"); ok {
		p.Headless = &v
	}
	if v, ok := boolFrom(conf, "enable_default_extensions"); ok {
		p.EnableDefaultExt = &v
	}
	if v, ok := boolFrom(conf, "chromium_sandbox"); ok {
		p.ChromiumSandbox = &v
	}
	if v, ok := boolFrom(conf, "persist_session"); ok {
		p.PersistSession = &v
	}
	if v, ok := stringFrom(conf, "url"); ok {
		p.URL = v
	}
	if v, ok := intFrom(conf, "timeout"); ok {
		p.Timeout = v
	} else {
		p.Timeout = 0
	}
	return nil
}

// Check validates the param. The accepted-but-ignored Python
// fields are NOT validated here — the v1 fixture is allowed to set
// them; we only reject structurally invalid values for fields we
// actually use (`llm_id`, `prompts`).
func (p *browserParam) Check() error {
	if p.LLMID == "" {
		return &ParamError{Field: "llm_id", Reason: "required (or model_id alias)"}
	}
	if p.Prompts == "" {
		return &ParamError{Field: "prompts", Reason: "required (or prompt alias)"}
	}
	if p.MaxSteps < 0 {
		return &ParamError{Field: "max_steps", Reason: "must be non-negative"}
	}
	if p.Timeout < 0 {
		return &ParamError{Field: "timeout", Reason: "must be non-negative"}
	}
	return nil
}

// AsDict returns the param as a plain map (for serialization / debug).
func (p *browserParam) AsDict() map[string]any {
	out := map[string]any{
		"llm_id":    p.LLMID,
		"model_id":  p.LLMID, // alias echoed
		"prompts":   p.Prompts,
		"prompt":    p.Prompts, // alias echoed
		"max_steps": p.MaxSteps,
		"url":       p.URL,
		"timeout":   p.Timeout,
	}
	if p.Headless != nil {
		out["headless"] = *p.Headless
	}
	if p.EnableDefaultExt != nil {
		out["enable_default_extensions"] = *p.EnableDefaultExt
	}
	if p.ChromiumSandbox != nil {
		out["chromium_sandbox"] = *p.ChromiumSandbox
	}
	if p.PersistSession != nil {
		out["persist_session"] = *p.PersistSession
	}
	return out
}

// BrowserComponent is the canvas Browser node. Owns its static
// param; delegates the multistep agent run to StagehandInvoker.
type BrowserComponent struct {
	name  string
	param browserParam
}

// NewBrowserComponent constructs a Browser from the DSL param map.
func NewBrowserComponent(params map[string]any) (Component, error) {
	p := &browserParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("browser: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("browser: param check: %w", err)
	}
	return &BrowserComponent{
		name:  componentNameBrowser,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (b *BrowserComponent) Name() string { return b.name }

// Invoke runs the resolved browser instruction through Stagehand's agent so
// navigation and interaction happen before the final result is returned.
func (b *BrowserComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}
	if state == nil {
		return nil, errors.New("browser: nil canvas state")
	}

	tenantID, _ := state.Sys["tenant_id"].(string)
	if tenantID == "" {
		return nil, errors.New("browser: tenant_id missing from canvas state (state.Sys[\"tenant_id\"])")
	}

	// 1. Resolve prompts template.
	prompts := b.param.Prompts
	if v, ok := inputs["prompts"].(string); ok && strings.TrimSpace(v) != "" {
		prompts = v
	}
	resolvedPrompts, err := runtime.ResolveTemplate(prompts, state)
	if err != nil {
		return nil, fmt.Errorf("browser: resolve prompts template: %w", err)
	}

	if browserModelFactory == nil {
		return nil, errors.New("browser: model factory is not configured")
	}
	chatModel, err := browserModelFactory(ctx, tenantID, b.param.LLMID)
	if err != nil {
		return nil, fmt.Errorf("browser: resolve model (%q): %w", b.param.LLMID, err)
	}
	if chatModel == nil {
		return nil, errors.New("browser: model factory returned nil model")
	}
	req := RunTaskRequest{
		TenantID:        tenantID,
		LLMID:           b.param.LLMID,
		ChatModel:       chatModel,
		Headless:        b.param.Headless,
		ChromiumSandbox: b.param.ChromiumSandbox != nil && *b.param.ChromiumSandbox,
		Instruction:     resolvedPrompts,
		MaxSteps:        b.param.MaxSteps,
	}
	if chatModel.ModelName != nil {
		req.ModelName = *chatModel.ModelName
	}
	if chatModel.APIConfig != nil {
		if chatModel.APIConfig.ApiKey != nil {
			req.APIKey = *chatModel.APIConfig.ApiKey
		}
		if chatModel.APIConfig.BaseURL != nil {
			req.BaseURL = *chatModel.APIConfig.BaseURL
		}
	}

	invoker := getDefaultStagehandInvoker()
	content, err := invoker.RunTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("browser: stagehand task (model=%q, base_url=%s): %w",
			req.ModelName, browserBaseURLForLog(req.BaseURL), err)
	}

	// 4. Build the output map.
	out := map[string]any{
		"content":          content,
		"downloaded_files": []map[string]any{},
		"url":              "",
		"status":           0,
		"size":             len(content),
		"model_id":         b.param.LLMID,
		"prompt":           prompts,
	}
	return out, nil
}

func browserBaseURLForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "<provider default>"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	u.User = nil
	return u.String()
}

// Stream emits the final result after the browser task completes.
func (b *BrowserComponent) Stream(ctx context.Context, db *gorm.DB, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := b.Invoke(ctx, db, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the parameter metadata for tooling.
func (b *BrowserComponent) Inputs() map[string]string {
	return map[string]string{
		"llm_id":                    "Required: tenant model id, e.g. \"deepseek-v4-pro@DeepSeek\". model_id accepted as alias.",
		"prompts":                   "Required: natural-language browser task. {sys.query} and other canvas refs are resolved. prompt accepted as alias.",
		"max_steps":                 "Maximum browser agent steps (default 30).",
		"headless":                  "Browser launch mode (default true).",
		"enable_default_extensions": "Accepted for fixture compat; ignored at Invoke.",
		"chromium_sandbox":          "Whether to enable Chromium's sandbox (default false).",
		"persist_session":           "Accepted for fixture compat; ignored at Invoke.",
		"url":                       "Go-only; not used (kept for backward compat).",
		"timeout":                   "Go-only; not used (kept for backward compat).",
	}
}

func (b *BrowserComponent) GetInputForm() map[string]any {
	return map[string]any{
		"prompts": map[string]any{
			"type": "paragraph",
			"name": "Prompts",
		},
		"upload_sources": map[string]any{
			"type": "line",
			"name": "Upload sources",
		},
	}
}

// Outputs returns the response surface.
func (b *BrowserComponent) Outputs() map[string]string {
	return map[string]string{
		"content":          "Browser agent final result.",
		"downloaded_files": "Always [] (file download not supported).",
		"url":              "Go-native compat key; always \"\".",
		"status":           "Go-native compat key; always 0.",
		"size":             "Bytes in content.",
		"model_id":         "Resolved llm_id (echoed back).",
		"prompt":           "Resolved prompts (echoed back).",
	}
}

func init() {
	Register(componentNameBrowser, NewBrowserComponent)
}
