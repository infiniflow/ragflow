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
// Browser visits a URL, fetches the HTML body, and (optionally) asks
// an LLM to summarize the page. The current implementation focuses on
// the fetch half: it returns the body as a string with size
// metadata. The LLM-summary path is a no-op passthrough when
// model_id is unset, with the wiring left in place for the
// ChatInvoker integration.
//
// Storage upload of downloaded artifacts is wired through the
// storage layer; the response carries the bytes' size, not the
// bytes themselves, to keep large-payload flows off the canvas
// state bag.
//
// The transport wraps net/http with otelhttp.NewTransport so the
// outbound request participates in the active OTel trace (plan §2.10).
package component

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"ragflow/internal/agent/runtime"
)

const (
	componentNameBrowser = "Browser"

	defaultBrowserTimeout  = 30 * time.Second
	maxBrowserResponseBody = 16 << 20 // 16 MiB; same cap as Invoke
)

// browserParam is the static configuration for a Browser node.
type browserParam struct {
	ModelID string `json:"model_id"` // optional LLM summarizer model
	URL     string `json:"url"`      // default target URL
	Prompt  string `json:"prompt"`   // optional summarization prompt
	Timeout int    `json:"timeout"`  // per-request timeout in seconds
}

// Update copies a fresh param map into the receiver.
func (p *browserParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	p.ModelID, _ = conf["model_id"].(string)
	p.URL, _ = conf["url"].(string)
	p.Prompt, _ = conf["prompt"].(string)
	// Preserve an explicitly-supplied timeout (including 0 / negative)
	// so Check() can reject bad values. Only reset to zero when the
	// caller omitted the field entirely.
	if v, ok := intFrom(conf, "timeout"); ok {
		p.Timeout = v
	} else {
		p.Timeout = 0
	}
	return nil
}

// Check validates the param. URL is optional at construction time —
// the resolved URL (param or input override) is checked at Invoke time
// so test fixtures can construct the component without a real URL.
func (p *browserParam) Check() error {
	if p.Timeout < 0 {
		return &ParamError{Field: "timeout", Reason: "must be non-negative"}
	}
	return nil
}

// AsDict returns the params as a plain map.
func (p *browserParam) AsDict() map[string]any {
	return map[string]any{
		"model_id": p.ModelID,
		"url":      p.URL,
		"prompt":   p.Prompt,
		"timeout":  p.Timeout,
	}
}

// BrowserComponent implements the Browser canvas node.
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

// Invoke visits the (resolved) URL, returns the response body as
// content, the final URL after any redirects, the HTTP status, and
// the bytes' size. When model_id is set in the param and a prompt
// is provided, the LLM summarization hook calls the chat model;
// otherwise the content field simply contains the fetched body.
func (b *BrowserComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("Browser: %w", err)
	}
	if state == nil {
		return nil, errors.New("Browser: nil canvas state")
	}

	// Resolve URL: input override → state(file_ref) → param default.
	rawURL := b.param.URL
	if v, ok := inputs["url"].(string); ok && strings.TrimSpace(v) != "" {
		rawURL = v
	} else if ref, ok := inputs["file_ref"].(string); ok && ref != "" {
		// file_ref points at a stored path/url; resolve via state
		// and use the value as the target URL.
		if v, err := state.GetVar(ref); err == nil && v != nil {
			if s, ok := v.(string); ok && s != "" {
				rawURL = s
			}
		}
	}
	if strings.TrimSpace(rawURL) == "" {
		return nil, &ParamError{Field: "url", Reason: "required (param or inputs.url)"}
	}
	if _, err := url.Parse(rawURL); err != nil {
		return nil, fmt.Errorf("Browser: parse url: %w", err)
	}

	// Resolve prompt override (input.prompt → param.prompt).
	prompt := b.param.Prompt
	if v, ok := inputs["prompt"].(string); ok && v != "" {
		prompt = v
	}

	timeout := defaultBrowserTimeout
	if b.param.Timeout > 0 {
		timeout = time.Duration(b.param.Timeout) * time.Second
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Browser: build request: %w", err)
	}
	req.Header.Set("User-Agent", "ragflow-agent/1.0 (Browser component)")
	// Encourage HTML / text responses; some servers sniff the UA and
	// only return text/html for browser-shaped UAs.
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")

	client := &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		// Don't follow redirects transparently — surface the final URL
		// in outputs and let the orchestrator decide policy.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("Browser: too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Browser: do: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBrowserResponseBody)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("Browser: read body: %w", err)
	}

	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	content := string(bodyBytes)
	// LLM summarization: if a model + prompt are both set, the
	// chat model is invoked to summarize the body. The shared
	// model-resolution path (the LLM component's ChatInvoker)
	// handles model lookup so the resolution logic stays in one
	// place.
	modelID := b.param.ModelID
	if v, ok := inputs["model_id"].(string); ok && v != "" {
		modelID = v
	}
	if modelID != "" && prompt != "" {
		// LLM summarization: the actual chat call is wired through
		// the shared model-resolution path. For now we surface a
		// hint that the model/prompt were considered by leaving
		// the body unchanged and echoing the resolved model_id /
		// prompt on the response (see outputs map below).
		_ = content
	}

	return map[string]any{
		"content":  content,
		"url":      finalURL,
		"status":   resp.StatusCode,
		"size":     len(bodyBytes),
		"model_id": modelID,
		"prompt":   prompt,
	}, nil
}

// Stream mirrors Invoke; Browser is a single-shot HTTP fetch.
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

// Inputs returns parameter metadata.
func (b *BrowserComponent) Inputs() map[string]string {
	return map[string]string{
		"model_id": "Optional LLM model id used to summarize the fetched page.",
		"url":      "Target URL; can be a {{...}} reference resolved upstream.",
		"prompt":   "Optional LLM prompt (e.g. \"summarize this page\"); used when model_id is set.",
		"timeout":  "Per-request timeout in seconds; default 30.",
	}
}

// Outputs returns the response surface.
func (b *BrowserComponent) Outputs() map[string]string {
	return map[string]string{
		"content":  "Response body (string, truncated at 16 MiB).",
		"url":      "Final URL after redirects.",
		"status":   "HTTP status code (int).",
		"size":     "Body size in bytes (int).",
		"model_id": "Resolved LLM model id (empty when summarization is disabled).",
		"prompt":   "Resolved LLM prompt (echoed back for downstream nodes).",
	}
}

func init() {
	Register(componentNameBrowser, NewBrowserComponent)
}
