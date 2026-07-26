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

// Package component — Browser (T3).
//
// Browser is an LLM-driven single-shot web extraction canvas node
// built on `github.com/browserbase/stagehand-go/v3` in local mode.
// It uses `RunExtract` (not the multi-step agent `RunTask`) to
// navigate to a page and extract structured content against a
// `{"type": "string"}` JSON schema.
//
// It mirrors the Python `agent/component/browser.py` param surface
// (`llm_id`, `prompts`, `max_steps`, `headless`, `persist_session`,
// `upload_sources`, etc.) so the v1 fixture
// (`internal/agent/dsl/testdata/browser.json`) loads without
// fixture-side changes.
//
// LLM dispatch is delegated to `StagehandInvoker` (see
// `stagehand_runtime.go`), which owns the stagehand-server child
// process and the session lifecycle. The component itself is a thin
// orchestrator: parse → resolve template → look up tenant model
// config → call runtime.RunExtract → emit Python-shaped outputs.
//
// File upload / download and persistent session management are
// not supported; see [`.claude/plans/tingly-weaving-orbit.md`]
// for the full deferral list.
package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const componentNameBrowser = "Browser"

var stagehandNativeProviders = map[string]string{
	"anthropic": "anthropic",
	"google":    "google",
	"openai":    "openai",
}

var browserFactoryDefaultBaseURL = map[string]string{
	"302.ai":         "https://api.302.ai/v1",
	"anthropic":      "https://api.anthropic.com/",
	"astraflow":      "https://api-us-ca.umodelverse.ai/v1",
	"astraflow-cn":   "https://api.modelverse.cn/v1",
	"avian":          "https://api.avian.io/v1",
	"cometapi":       "https://api.cometapi.com/v1",
	"dashscope":      "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"deepseek":       "https://api.deepseek.com/v1",
	"deerapi":        "https://api.deerapi.com/v1",
	"futurmix":       "https://futurmix.ai/v1",
	"giteeai":        "https://ai.gitee.com/v1/",
	"hunyuan":        "https://api.hunyuan.cloud.tencent.com/v1",
	"jiekouai":       "https://api.jiekou.ai/openai",
	"lingyi-ai":      "https://api.lingyiwanwu.com/v1",
	"longcat":        "https://api.longcat.chat/openai",
	"minimax":        "https://api.minimaxi.com/v1",
	"moonshot":       "https://api.moonshot.cn/v1",
	"n1n":            "https://api.n1n.ai/v1",
	"novitaai":       "https://api.novita.ai/v3/openai",
	"openai":         "https://api.openai.com/v1",
	"openrouter":     "https://openrouter.ai/api/v1",
	"perfxcloud":     "https://cloud.perfxlab.cn/v1",
	"ppio":           "https://api.ppinfra.com/v3/openai",
	"siliconflow":    "https://api.siliconflow.cn/v1",
	"stepfun":        "https://api.stepfun.com/v1",
	"tongyi-qianwen": "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"upstage":        "https://api.upstage.ai/v1/solar",
	"zhipu-ai":       "https://open.bigmodel.cn/api/paas/v4",
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

// llmIDPattern matches `ModelName@Factory`. The factory part is
// optional; when absent, the caller's tenant lookup will be
// `GetByTenantAndModelName` instead of
// `GetByTenantFactoryAndModelName`.
var llmIDPattern = regexp.MustCompile(`^(.+)@(.+)$`)

// resolveLLMID splits `llm_id` (e.g. "deepseek-v4-pro@DeepSeek") into
// `(modelName, factory)`. When no `@` is present, factory is empty
// and the caller must use a single-key lookup.
//
// Mirrors the contract of `dao.splitModelNameAndFactory` (private);
// re-implemented here to keep the component free of an import
// dependency on a DB-validating private helper.
func resolveLLMID(llmID string) (modelName, factory string) {
	m := llmIDPattern.FindStringSubmatch(strings.TrimSpace(llmID))
	if m == nil {
		return strings.TrimSpace(llmID), ""
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
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
// param; delegates the multi-step agent run to StagehandInvoker.
type BrowserComponent struct {
	name  string
	param browserParam
}

// NewBrowserComponent constructs a Browser from the DSL param map.
func NewBrowserComponent(params map[string]any) (Component, error) {
	p := &browserParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("Browser: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("Browser: param check: %w", err)
	}
	return &BrowserComponent{
		name:  componentNameBrowser,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (b *BrowserComponent) Name() string { return b.name }

// Invoke dispatches a single-shot extraction task via
// StagehandInvoker.RunExtract with a `{"type": "string"}` schema.
// The flow:
//
//  1. Pull tenant_id from `state.Sys["tenant_id"]`.
//  2. Resolve the `prompts` template via `runtime.ResolveTemplate`.
//  3. Resolve `llm_id` as a tenant_model id or legacy model@factory
//     reference and look up the tenant LLM config (apiKey, baseURL).
//  4. Build `RunExtractRequest` with `ModelName = "openai/<model>"`,
//     the resolved apiKey/baseURL/instruction, and
//     `Schema = {"type": "string"}`.
//  5. Call `DefaultRuntime.RunExtract` → raw JSON string.
//  6. Unmarshal the JSON string to get the plain text content.
//  7. Emit the Python-shaped outputs (`content`,
//     `downloaded_files`) plus Go-native compat keys.
//
// File upload/download and session persistence are not supported
// in this component; they are v1-deferred.
func (b *BrowserComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("Browser: %w", err)
	}
	if state == nil {
		return nil, errors.New("Browser: nil canvas state")
	}

	tenantID, _ := state.Sys["tenant_id"].(string)
	if tenantID == "" {
		return nil, errors.New("Browser: tenant_id missing from canvas state (state.Sys[\"tenant_id\"])")
	}

	// 1. Resolve prompts template.
	prompts := b.param.Prompts
	if v, ok := inputs["prompts"].(string); ok && strings.TrimSpace(v) != "" {
		prompts = v
	}
	resolvedPrompts, err := runtime.ResolveTemplate(prompts, state)
	if err != nil {
		return nil, fmt.Errorf("Browser: resolve prompts template: %w", err)
	}

	// 2. Look up tenant model config.
	providerName, modelName, apiKey, baseURL, err := resolveBrowserLLM(tenantID, b.param.LLMID)
	if err != nil {
		return nil, fmt.Errorf("Browser: tenant llm lookup (%q): %w", b.param.LLMID, err)
	}
	baseURL = strings.TrimSpace(baseURL)

	// 3. Build RunExtractRequest with single-string schema.
	req := RunExtractRequest{
		TenantID:    tenantID,
		LLMID:       b.param.LLMID,
		ModelName:   stagehandModelName(providerName, modelName),
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Headless:    b.param.Headless,
		Instruction: resolvedPrompts,
		Schema:      map[string]any{"type": "string"},
	}

	// 4. Dispatch via the runtime's RunExtract.
	invoker := getDefaultStagehandInvoker()
	rawJSON, err := invoker.RunExtract(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Browser: stagehand extract (model=%q, base_url=%s): %w",
			req.ModelName, browserBaseURLForLog(req.BaseURL), err)
	}

	// 5. Unmarshal the JSON-string result to get the plain text.
	var content string
	if err := json.Unmarshal([]byte(rawJSON), &content); err != nil {
		return nil, fmt.Errorf("Browser: unmarshal extract result: %w", err)
	}

	// 6. Build the output map.
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

func stagehandModelName(providerName, modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ""
	}
	if strings.Contains(modelName, "/") {
		prefix, _, _ := strings.Cut(modelName, "/")
		if stagehandNativeProviders[strings.ToLower(strings.TrimSpace(prefix))] != "" {
			return modelName
		}
		return "openai/" + modelName
	}
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if nativeProvider := stagehandNativeProviders[providerName]; nativeProvider != "" {
		return nativeProvider + "/" + modelName
	}
	return "openai/" + modelName
}

// Stream mirrors Invoke; Browser is a single-shot generator.
func (b *BrowserComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := b.Invoke(ctx, inputs)
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
		"prompts":                   "Required: natural-language extraction task. {sys.query} and other canvas refs are resolved. prompt accepted as alias.",
		"max_steps":                 "Accepted for fixture compat; ignored at Invoke.",
		"headless":                  "Browser launch mode (default true).",
		"enable_default_extensions": "Accepted for fixture compat; ignored at Invoke.",
		"chromium_sandbox":          "Accepted for fixture compat; ignored at Invoke.",
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
		"content":          "Extracted plain text (Sessions.Extract result with schema {\"type\":\"string\"}).",
		"downloaded_files": "Always [] (file download not supported).",
		"url":              "Go-native compat key; always \"\".",
		"status":           "Go-native compat key; always 0.",
		"size":             "Bytes in content.",
		"model_id":         "Resolved llm_id (echoed back).",
		"prompt":           "Resolved prompts (echoed back).",
	}
}

// resolveBrowserLLM resolves the Browser's selected model into the model name
// and credentials required by the stagehand runtime. It first tries a
// tenant_model.id lookup, then model@factory parsing via resolveTenantLLM.
//
// Tests override the lookup via `tenantLLMLookupForTest` (a
// package-level function variable) so they don't need a real DB.
// Production code leaves the variable unset.
func resolveBrowserLLM(tenantID, llmID string) (providerName, modelName, apiKey, baseURL string, err error) {
	if tenantLLMLookupForTest != nil {
		oldModelName, factory := resolveLLMID(llmID)
		apiKey, baseURL, err = tenantLLMLookupForTest(tenantID, oldModelName, factory)
		baseURL = browserOpenAICompatibleBaseURL(baseURL, factory)
		return factory, oldModelName, apiKey, baseURL, err
	}

	providerName, modelName, apiKey, baseURL, err = resolveTenantModelBrowserLLM(tenantID, llmID)
	if err == nil {
		baseURL = browserOpenAICompatibleBaseURL(baseURL, providerName)
		return providerName, modelName, apiKey, baseURL, nil
	}
	modelErr := err

	oldModelName, factory := resolveLLMID(llmID)
	apiKey, baseURL, oldErr := resolveTenantLLM(tenantID, oldModelName, factory)
	if oldErr == nil {
		baseURL = browserOpenAICompatibleBaseURL(baseURL, factory)
		return factory, oldModelName, apiKey, baseURL, nil
	}
	return "", "", "", "", fmt.Errorf("tenant_model lookup: %v; tenant_llm fallback: %w", modelErr, oldErr)
}

func resolveTenantModelBrowserLLM(tenantID, modelID string) (providerName, modelName, apiKey, baseURL string, err error) {
	modelRow, err := dao.NewTenantModelDAO().GetByID(modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", "", "", err
		}
		return "", "", "", "", fmt.Errorf("tenant model id=%s lookup failed: %w", modelID, err)
	}
	if modelRow.Status != "active" {
		return "", "", "", "", fmt.Errorf("tenant model id=%s is disabled", modelID)
	}
	if !entity.ModelType(modelRow.ModelType).Has(entity.ModelTypeChat) {
		return "", "", "", "", fmt.Errorf("tenant model id=%s cannot be used as %s model", modelID, entity.ModelTypeChat.String())
	}

	provider, err := dao.NewTenantModelProviderDAO().GetByID(modelRow.ProviderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", "", "", fmt.Errorf("provider id=%s not found for model id=%s", modelRow.ProviderID, modelID)
		}
		return "", "", "", "", err
	}
	if provider == nil {
		return "", "", "", "", fmt.Errorf("provider id=%s not found for model id=%s", modelRow.ProviderID, modelID)
	}
	if provider.TenantID != tenantID {
		return "", "", "", "", fmt.Errorf("tenant %s has no access to provider owned by tenant %s", tenantID, provider.TenantID)
	}

	instance, err := dao.NewTenantModelInstanceDAO().GetByID(modelRow.InstanceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", "", "", fmt.Errorf("instance id=%s not found for model id=%s", modelRow.InstanceID, modelID)
		}
		return "", "", "", "", err
	}
	if instance == nil {
		return "", "", "", "", fmt.Errorf("instance id=%s not found for model id=%s", modelRow.InstanceID, modelID)
	}

	apiKey = instance.APIKey
	if strings.TrimSpace(instance.Extra) != "" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
			return "", "", "", "", err
		}
		baseURL = extra["base_url"]
	}
	return provider.ProviderName, modelRow.ModelName, apiKey, baseURL, nil
}

func browserOpenAICompatibleBaseURL(baseURL, provider string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		return baseURL
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return ""
	}
	return browserFactoryDefaultBaseURL[strings.ToLower(provider)]
}

// resolveTenantLLM looks up the legacy tenant_llm config and returns
// (apiKey, baseURL). baseURL may be empty when the tenant's provider doesn't
// configure a custom endpoint.
//
// TODO(v2): this helper can move to `internal/dao` so the LLM
// component (`llm.go`) and other future components can share it.
func resolveTenantLLM(tenantID, modelName, factory string) (apiKey, baseURL string, err error) {
	dao := dao.NewTenantLLMDAO()
	var (
		row *entity.TenantLLM
	)
	if factory != "" {
		row, err = dao.GetByTenantFactoryAndModelName(tenantID, factory, modelName)
	} else {
		// No factory suffix on llm_id; fall back to a single-key
		// lookup (errors if the model is registered under multiple
		// factories — caller must use the explicit form).
		row, err = dao.GetByTenantAndModelName(tenantID, "", modelName)
	}
	if err != nil {
		return "", "", err
	}
	if row == nil {
		return "", "", fmt.Errorf("tenant LLM not found")
	}
	if row.APIKey != nil {
		apiKey = *row.APIKey
	}
	if row.APIBase != nil {
		baseURL = *row.APIBase
	}
	return apiKey, baseURL, nil
}

// tenantLLMLookupForTest is the test seam for `resolveTenantLLM`.
// When non-nil, it's called instead of the real DAO lookup.
// Production leaves this nil; tests set it via `defer ... = nil`.
var tenantLLMLookupForTest func(tenantID, modelName, factory string) (apiKey, baseURL string, err error)

func init() {
	Register(componentNameBrowser, NewBrowserComponent)
}
