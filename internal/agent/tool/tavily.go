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

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const tavilyToolName = "tavily"

const tavilyExtractToolName = "tavily_extract"

const tavilyToolDescription = "Search the web via the Tavily API. Returns a list of {url, title, content} results."

// tavilyParams is the JSON shape the model sends into InvokableRun. The
// api_key may be omitted when the env var TAVILY_API_KEY is set; the tool
// resolves it from the environment in that case.
type tavilyParams struct {
	APIKey                   string   `json:"api_key"`
	Query                    string   `json:"query"`
	MaxResults               int      `json:"max_results"`
	SearchDepth              string   `json:"search_depth"`
	Topic                    string   `json:"topic"`
	Days                     int      `json:"days"`
	IncludeAnswer            bool     `json:"include_answer"`
	IncludeRawContent        bool     `json:"include_raw_content"`
	IncludeImages            bool     `json:"include_images"`
	IncludeImageDescriptions bool     `json:"include_image_descriptions"`
	IncludeDomains           []string `json:"include_domains"`
	ExcludeDomains           []string `json:"exclude_domains"`
}

// tavilyRequestBody is the JSON body POSTed to the Tavily /search endpoint.
// The struct matches the upstream API (https://docs.tavily.com).
type tavilyRequestBody struct {
	Query                    string   `json:"query"`
	MaxResults               int      `json:"max_results"`
	SearchDepth              string   `json:"search_depth"`
	Topic                    string   `json:"topic,omitempty"`
	Days                     int      `json:"days"`
	IncludeAnswer            bool     `json:"include_answer"`
	IncludeRawContent        bool     `json:"include_raw_content"`
	IncludeImages            bool     `json:"include_images"`
	IncludeImageDescriptions bool     `json:"include_image_descriptions"`
	IncludeDomains           []string `json:"include_domains,omitempty"`
	ExcludeDomains           []string `json:"exclude_domains,omitempty"`
}

// tavilyResult mirrors one element of the upstream `results` array. We
// return these verbatim to the model.
type tavilyResult struct {
	URL        string  `json:"url"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	RawContent string  `json:"raw_content,omitempty"`
	Score      float64 `json:"score,omitempty"`
}

// tavilyResponse is the envelope returned by Tavily. We only model the
// fields we care about; the upstream API has more, but they are ignored.
type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

// tavilyEnvelope is the shape the model actually sees, identical to the
// Python tool's output convention.
type tavilyEnvelope struct {
	Results []tavilyResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

type tavilyExtractParams struct {
	APIKey       string `json:"api_key"`
	URLs         any    `json:"urls"`
	ExtractDepth string `json:"extract_depth"`
	Format       string `json:"format"`
}

type tavilyExtractRequestBody struct {
	URLs          []string `json:"urls"`
	ExtractDepth  string   `json:"extract_depth"`
	Format        string   `json:"format"`
	IncludeImages bool     `json:"include_images"`
}

type tavilyExtractResult struct {
	URL        string `json:"url"`
	RawContent string `json:"raw_content,omitempty"`
	Content    string `json:"content,omitempty"`
	Error      string `json:"error,omitempty"`
}

type tavilyExtractResponse struct {
	Results []tavilyExtractResult `json:"results"`
}

type tavilyExtractEnvelope struct {
	Results []tavilyExtractResult `json:"results"`
	Error   string                `json:"_ERROR,omitempty"`
}

// TavilyTool is the Tavily search
// tool. It POSTs a search request
// to https://api.tavily.com/search using the shared HTTPHelper and returns
// the upstream `results` array as JSON.
type TavilyTool struct {
	helper *HTTPHelper
	envKey func() string
}

// TavilyExtractTool is the Tavily Extract tool. It POSTs URLs to
// https://api.tavily.com/extract and returns the upstream results array.
type TavilyExtractTool struct {
	helper *HTTPHelper
	envKey func() string
}

// NewTavilyTool returns a TavilyTool using the default HTTPHelper and
// the TAVILY_API_KEY env var for credential resolution.
func NewTavilyTool() *TavilyTool {
	return NewTavilyToolWith(NewHTTPHelper())
}

// NewTavilyToolWith returns a TavilyTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewTavilyToolWith(h *HTTPHelper) *TavilyTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &TavilyTool{helper: h, envKey: defaultTavilyEnvKey}
}

// NewTavilyToolWithEnvKey returns a TavilyTool with a custom env-key
// resolver. Useful for tests that want to inject a fake credential
// without mutating process state.
func NewTavilyToolWithEnvKey(h *HTTPHelper, envKey func() string) *TavilyTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if envKey == nil {
		envKey = defaultTavilyEnvKey
	}
	return &TavilyTool{helper: h, envKey: envKey}
}

// NewTavilyExtractTool returns a TavilyExtractTool using the default HTTPHelper.
func NewTavilyExtractTool() *TavilyExtractTool {
	return NewTavilyExtractToolWith(NewHTTPHelper())
}

// NewTavilyExtractToolWith returns a TavilyExtractTool using the provided helper.
func NewTavilyExtractToolWith(h *HTTPHelper) *TavilyExtractTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &TavilyExtractTool{helper: h, envKey: defaultTavilyEnvKey}
}

// NewTavilyExtractToolWithEnvKey returns a TavilyExtractTool with a custom env-key resolver.
func NewTavilyExtractToolWithEnvKey(h *HTTPHelper, envKey func() string) *TavilyExtractTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if envKey == nil {
		envKey = defaultTavilyEnvKey
	}
	return &TavilyExtractTool{helper: h, envKey: envKey}
}

// defaultTavilyEnvKey is the production env-key resolver. Pulled out
// as a named function (not a var) so tests cannot accidentally
// mutate it via package-var assignment.
func defaultTavilyEnvKey() string { return os.Getenv("TAVILY_API_KEY") }

// Info returns the tool's metadata for the chat model.
func (t *TavilyTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: tavilyToolName,
		Desc: tavilyToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query",
				Required: true,
			},
			"api_key": {
				Type:     schema.String,
				Desc:     "Tavily API key. Falls back to TAVILY_API_KEY env var.",
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5.",
				Required: false,
			},
			"search_depth": {
				Type:     schema.String,
				Desc:     `Tavily search depth: "basic" (default) or "advanced".`,
				Required: false,
			},
			"topic": {
				Type:     schema.String,
				Desc:     `Search topic: "general" (default) or "news".`,
				Required: false,
			},
			"include_domains": {
				Type:     schema.Array,
				Desc:     "Domains that search results must include.",
				Required: false,
			},
			"exclude_domains": {
				Type:     schema.Array,
				Desc:     "Domains that search results must exclude.",
				Required: false,
			},
		}),
	}, nil
}

// Info returns the Tavily Extract tool metadata for the chat model.
func (t *TavilyExtractTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: tavilyExtractToolName,
		Desc: "Extract web page content from one or more specified URLs using Tavily Extract.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"urls": {
				Type:     schema.Array,
				Desc:     "The URLs to extract content from.",
				Required: true,
			},
			"extract_depth": {
				Type:     schema.String,
				Desc:     `Extraction depth: "basic" (default) or "advanced".`,
				Required: false,
			},
			"format": {
				Type:     schema.String,
				Desc:     `Output format: "markdown" (default) or "text".`,
				Required: false,
			},
		}),
	}, nil
}

// tavilyEndpoint is the Tavily /search URL. Exposed as a package var so
// tests can substitute a httptest.Server URL. Tests must serialize
// access with tavilyEndpointMu if running in parallel.
var tavilyEndpoint = "https://api.tavily.com/search"

// tavilyExtractEndpoint is the Tavily /extract URL. Exposed as a package var
// so tests can substitute it through rewriteHostTransport.
var tavilyExtractEndpoint = "https://api.tavily.com/extract"

// InvokableRun performs the Tavily search. The api_key may come from the
// argument or the TAVILY_API_KEY env var.
func (t *TavilyTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p tavilyParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return tavilyErrJSON(fmt.Errorf("tavily: parse arguments: %w", err)),
			fmt.Errorf("tavily: parse arguments: %w", err)
	}
	if p.Query == "" {
		return tavilyErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("tavily: query is required")
	}
	if p.MaxResults <= 0 {
		p.MaxResults = 6
	}
	if p.SearchDepth == "" {
		p.SearchDepth = "basic"
	}

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = t.envKey()
	}
	if apiKey == "" {
		return tavilyErrJSON(fmt.Errorf("tavily: api_key is required (or set TAVILY_API_KEY)")),
			fmt.Errorf("tavily: api_key is required (or set TAVILY_API_KEY)")
	}

	body, _ := json.Marshal(tavilyRequestBody{
		Query:                    p.Query,
		MaxResults:               p.MaxResults,
		SearchDepth:              p.SearchDepth,
		Topic:                    defaultString(p.Topic, "general"),
		Days:                     p.Days,
		IncludeAnswer:            p.IncludeAnswer,
		IncludeRawContent:        false,
		IncludeImages:            false,
		IncludeImageDescriptions: p.IncludeImageDescriptions,
		IncludeDomains:           p.IncludeDomains,
		ExcludeDomains:           p.ExcludeDomains,
	})

	resp, err := t.helper.Do(ctx,
		http.MethodPost, tavilyEndpoint, string(body), "application/json",
		map[string]string{"Authorization": "Bearer " + apiKey},
	)
	if err != nil {
		return tavilyErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tavilyErrJSON(fmt.Errorf("tavily: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("tavily: upstream returned %d", resp.StatusCode)
	}

	var raw tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return tavilyErrJSON(fmt.Errorf("tavily: decode response: %w", err)),
			fmt.Errorf("tavily: decode response: %w", err)
	}
	return tavilyJSON(tavilyEnvelope{Results: raw.Results}), nil
}

// InvokableRun performs the Tavily Extract request. The api_key may come from
// the argument or the TAVILY_API_KEY env var.
func (t *TavilyExtractTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p tavilyExtractParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: parse arguments: %w", err)),
			fmt.Errorf("tavily_extract: parse arguments: %w", err)
	}
	urls := normalizeTavilyURLs(p.URLs)
	if len(urls) == 0 {
		return tavilyExtractErrJSON(fmt.Errorf("urls is required")),
			fmt.Errorf("tavily_extract: urls is required")
	}
	if p.ExtractDepth == "" {
		p.ExtractDepth = "basic"
	}
	if p.Format == "" {
		p.Format = "markdown"
	}

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = t.envKey()
	}
	if apiKey == "" {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: api_key is required (or set TAVILY_API_KEY)")),
			fmt.Errorf("tavily_extract: api_key is required (or set TAVILY_API_KEY)")
	}

	body, _ := json.Marshal(tavilyExtractRequestBody{
		URLs:          urls,
		ExtractDepth:  p.ExtractDepth,
		Format:        p.Format,
		IncludeImages: false,
	})
	resp, err := t.helper.Do(ctx,
		http.MethodPost, tavilyExtractEndpoint, string(body), "application/json",
		map[string]string{"Authorization": "Bearer " + apiKey},
	)
	if err != nil {
		return tavilyExtractErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("tavily_extract: upstream returned %d", resp.StatusCode)
	}

	var raw tavilyExtractResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: decode response: %w", err)),
			fmt.Errorf("tavily_extract: decode response: %w", err)
	}
	return tavilyExtractJSON(tavilyExtractEnvelope{Results: raw.Results}), nil
}

// tavilyJSON marshals the envelope to a JSON string for the model.
func tavilyJSON(env tavilyEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"tavily: marshal result: %s"}`, err)
	}
	return string(b)
}

// tavilyErrJSON wraps an error in the standard envelope.
func tavilyErrJSON(err error) string {
	return tavilyJSON(tavilyEnvelope{Error: err.Error()})
}

func tavilyExtractJSON(env tavilyExtractEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"tavily_extract: marshal result: %s"}`, err)
	}
	return string(b)
}

func tavilyExtractErrJSON(err error) string {
	return tavilyExtractJSON(tavilyExtractEnvelope{Error: err.Error()})
}

func normalizeTavilyURLs(raw any) []string {
	switch v := raw.(type) {
	case string:
		return splitTavilyURLs(v)
	case []string:
		return compactStrings(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return compactStrings(out)
	default:
		return nil
	}
}

func splitTavilyURLs(s string) []string {
	parts := strings.Split(s, ",")
	return compactStrings(parts)
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
