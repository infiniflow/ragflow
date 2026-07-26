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
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

const tavilyToolName = "tavily_search"

const tavilyExtractToolName = "tavily_extract"

const tavilyToolDescription = "Search the web via the Tavily API. Returns a list of {url, title, content} results."

const tavilyPromptMaxTokens = 200000

var tavilyDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var tavilyNewlinePattern = regexp.MustCompile(`\n+`)

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

// tavilyResponse preserves every upstream result field because Python exposes
// response["results"] directly through the Canvas json output.
type tavilyResponse struct {
	Results []map[string]any `json:"results"`
}

// tavilyEnvelope is the shape the model actually sees, identical to the
// Python tool's output convention.
type tavilyEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
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

type tavilyExtractResponse struct {
	Results []map[string]any `json:"results"`
}

type tavilyExtractEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// TavilyTool is the Tavily search
// tool. It POSTs a search request
// to https://api.tavily.com/search using the shared HTTPHelper and returns
// the upstream `results` array as JSON.
type TavilyTool struct {
	helper   *HTTPHelper
	envKey   func() string
	defaults tavilyParams
}

// TavilyExtractTool is the Tavily Extract tool. It POSTs URLs to
// https://api.tavily.com/extract and returns the upstream results array.
type TavilyExtractTool struct {
	helper   *HTTPHelper
	envKey   func() string
	defaults tavilyExtractParams
}

var _ ToolComponent = (*TavilyTool)(nil)
var _ ReferenceBuilder = (*TavilyTool)(nil)
var _ ToolComponent = (*TavilyExtractTool)(nil)

// NewTavilyTool returns a TavilyTool using the default HTTPHelper and
// the TAVILY_API_KEY env var for credential resolution.
func NewTavilyTool() *TavilyTool {
	return newTavilyTool(nil, nil, tavilyParams{})
}

// NewTavilyToolWith returns a TavilyTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewTavilyToolWith(h *HTTPHelper) *TavilyTool {
	return newTavilyTool(h, nil, tavilyParams{})
}

// NewTavilyToolWithEnvKey returns a TavilyTool with a custom env-key
// resolver. Useful for tests that want to inject a fake credential
// without mutating process state.
func NewTavilyToolWithEnvKey(h *HTTPHelper, envKey func() string) *TavilyTool {
	return newTavilyTool(h, envKey, tavilyParams{})
}

func newTavilyTool(h *HTTPHelper, envKey func() string, defaults tavilyParams) *TavilyTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if envKey == nil {
		envKey = defaultTavilyEnvKey
	}
	if defaults.MaxResults == 0 {
		defaults.MaxResults = 6
	}
	if defaults.SearchDepth == "" {
		defaults.SearchDepth = "basic"
	}
	if defaults.Topic == "" {
		defaults.Topic = "general"
	}
	if defaults.Days == 0 {
		defaults.Days = 14
	}
	return &TavilyTool{helper: h, envKey: envKey, defaults: defaults}
}

// NewTavilyExtractTool returns a TavilyExtractTool using the default HTTPHelper.
func NewTavilyExtractTool() *TavilyExtractTool {
	return newTavilyExtractTool(nil, nil, tavilyExtractParams{})
}

// NewTavilyExtractToolWith returns a TavilyExtractTool using the provided helper.
func NewTavilyExtractToolWith(h *HTTPHelper) *TavilyExtractTool {
	return newTavilyExtractTool(h, nil, tavilyExtractParams{})
}

// NewTavilyExtractToolWithEnvKey returns a TavilyExtractTool with a custom env-key resolver.
func NewTavilyExtractToolWithEnvKey(h *HTTPHelper, envKey func() string) *TavilyExtractTool {
	return newTavilyExtractTool(h, envKey, tavilyExtractParams{})
}

func newTavilyExtractTool(h *HTTPHelper, envKey func() string, defaults tavilyExtractParams) *TavilyExtractTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if envKey == nil {
		envKey = defaultTavilyEnvKey
	}
	if defaults.ExtractDepth == "" {
		defaults.ExtractDepth = "basic"
	}
	if defaults.Format == "" {
		defaults.Format = "markdown"
	}
	return &TavilyExtractTool{helper: h, envKey: envKey, defaults: defaults}
}

// defaultTavilyEnvKey is the production env-key resolver. Pulled out
// as a named function (not a var) so tests cannot accidentally
// mutate it via package-var assignment.
func defaultTavilyEnvKey() string { return common.GetEnv(common.EnvTavilyApiKey) }

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
//
// All recoverable errors (API failures, network errors, missing API key) are
// returned as a JSON envelope with an _ERROR field and a nil Go error. This
// lets the ReAct agent's eino framework feed the error back to the LLM instead
// of discarding the result and crashing the entire agent run. (eino's
// wrapToolCall discards the result string when error is non-nil; returning nil
// ensures the error context reaches the model.)
func (t *TavilyTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p tavilyParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return tavilyErrJSON(fmt.Errorf("tavily: parse arguments: %w", err)), nil
	}
	if p.Query == "" {
		return tavilyJSON(tavilyEnvelope{Results: []map[string]any{}}), nil
	}
	var provided map[string]json.RawMessage
	_ = json.Unmarshal([]byte(argsJSON), &provided)
	p = mergeTavilyParams(t.defaults, p, provided)

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = t.envKey()
	}
	if apiKey == "" {
		return tavilyErrJSON(fmt.Errorf("tavily: api_key is required (or set TAVILY_API_KEY)")), nil
	}

	// Match the Python implementation: always disable images and raw content.
	p.IncludeImages = false
	p.IncludeRawContent = false

	body, _ := json.Marshal(tavilyRequestBody{
		Query:                    p.Query,
		MaxResults:               p.MaxResults,
		SearchDepth:              p.SearchDepth,
		Topic:                    defaultString(p.Topic, "general"),
		Days:                     p.Days,
		IncludeAnswer:            p.IncludeAnswer,
		IncludeRawContent:        p.IncludeRawContent,
		IncludeImages:            p.IncludeImages,
		IncludeImageDescriptions: p.IncludeImageDescriptions,
		IncludeDomains:           p.IncludeDomains,
		ExcludeDomains:           p.ExcludeDomains,
	})

	resp, err := t.helper.Do(ctx,
		http.MethodPost, tavilyEndpoint, string(body), "application/json",
		map[string]string{"Authorization": "Bearer " + apiKey},
	)
	if err != nil {
		return tavilyErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tavilyErrJSON(fmt.Errorf("tavily: upstream returned %d", resp.StatusCode)), nil
	}

	var raw tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return tavilyErrJSON(fmt.Errorf("tavily: decode response: %w", err)), nil
	}
	return tavilyJSON(tavilyEnvelope{Results: raw.Results}), nil
}

func mergeTavilyParams(defaults, params tavilyParams, provided map[string]json.RawMessage) tavilyParams {
	if params.APIKey == "" {
		params.APIKey = defaults.APIKey
	}
	if params.MaxResults == 0 {
		params.MaxResults = defaults.MaxResults
	}
	if params.SearchDepth == "" {
		params.SearchDepth = defaults.SearchDepth
	}
	if params.Topic == "" {
		params.Topic = defaults.Topic
	}
	if params.Days == 0 {
		params.Days = defaults.Days
	}
	if params.IncludeDomains == nil {
		params.IncludeDomains = defaults.IncludeDomains
	}
	if params.ExcludeDomains == nil {
		params.ExcludeDomains = defaults.ExcludeDomains
	}
	if _, ok := provided["include_answer"]; !ok {
		params.IncludeAnswer = defaults.IncludeAnswer
	}
	if _, ok := provided["include_raw_content"]; !ok {
		params.IncludeRawContent = defaults.IncludeRawContent
	}
	if _, ok := provided["include_images"]; !ok {
		params.IncludeImages = defaults.IncludeImages
	}
	if _, ok := provided["include_image_descriptions"]; !ok {
		params.IncludeImageDescriptions = defaults.IncludeImageDescriptions
	}
	return params
}

// ComponentSpec returns TavilySearch's Canvas-facing metadata.
func (t *TavilyTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":           "Search query.",
			"topic":           `Search topic: "general" or "news".`,
			"include_domains": "Domains that search results must include.",
			"exclude_domains": "Domains that search results must exclude.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Tavily references for downstream prompts.",
			"json":               "Raw Tavily result list.",
		},
		InputForm: map[string]any{
			"query":           map[string]any{"name": "Query", "type": "line"},
			"topic":           map[string]any{"name": "Topic", "type": "line"},
			"include_domains": map[string]any{"name": "Include domains", "type": "line"},
			"exclude_domains": map[string]any{"name": "Exclude domains", "type": "line"},
		},
	}
}

func (t *TavilyTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildTavilyReferences(envelope)
}

func (t *TavilyTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildTavilyReferences(envelope)
	return map[string]any{
		"formalized_content": renderTavilyReferences(chunks, tavilyPromptMaxTokens),
		"json":               results,
	}
}

func buildTavilyReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := tavilyText(item["raw_content"])
		if content == "" {
			content = tavilyText(item["content"])
		}
		content = tavilyDataImagePattern.ReplaceAllString(content, "")
		content = truncateTavilyRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(tavilyHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(tavilyHashInt(documentID, 500), 10)
		title := tavilyText(item["title"])
		resultURL := tavilyText(item["url"])
		score := item["score"]
		chunks = append(chunks, map[string]any{
			"id":            displayID,
			"chunk_id":      documentID,
			"content":       content,
			"doc_id":        documentID,
			"document_id":   documentID,
			"docnm_kwd":     title,
			"document_name": title,
			"similarity":    score,
			"score":         score,
			"url":           resultURL,
		})
		docAggs = append(docAggs, map[string]any{
			"doc_name": title,
			"doc_id":   documentID,
			"count":    1,
			"url":      resultURL,
		})
	}
	return chunks, docAggs
}

func renderTavilyReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := tavilyText(chunk["content"])
		if content == "" {
			continue
		}
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + tavilyText(chunk["id"]),
			"├── Title: " + tavilyPromptField(chunk["document_name"]),
			"├── URL: " + tavilyPromptField(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func tavilyText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func tavilyPromptField(value any) string {
	return tavilyNewlinePattern.ReplaceAllString(tavilyText(value), " ")
}

func tavilyHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateTavilyRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// InvokableRun performs the Tavily Extract request. The api_key may come from
// the argument or the TAVILY_API_KEY env var.
//
// All recoverable errors are returned as a JSON envelope with an _ERROR field
// and a nil Go error, so the ReAct agent can feed the error back to the LLM
// instead of discarding the result and crashing the agent run.
func (t *TavilyExtractTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var runtimeParams tavilyExtractParams
	if err := json.Unmarshal([]byte(argsJSON), &runtimeParams); err != nil {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: parse arguments: %w", err)), nil
	}
	p := mergeTavilyExtractParams(t.defaults, runtimeParams)
	urls := normalizeTavilyURLs(p.URLs)
	if len(urls) == 0 {
		return tavilyExtractErrJSON(fmt.Errorf("urls is required")), nil
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
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: api_key is required (or set TAVILY_API_KEY)")), nil
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
		return tavilyExtractErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: upstream returned %d", resp.StatusCode)), nil
	}

	var raw tavilyExtractResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return tavilyExtractErrJSON(fmt.Errorf("tavily_extract: decode response: %w", err)), nil
	}
	return tavilyExtractJSON(tavilyExtractEnvelope{Results: raw.Results}), nil
}

func mergeTavilyExtractParams(defaults, params tavilyExtractParams) tavilyExtractParams {
	if params.APIKey == "" {
		params.APIKey = defaults.APIKey
	}
	if params.URLs == nil {
		params.URLs = defaults.URLs
	}
	if params.ExtractDepth == "" {
		params.ExtractDepth = defaults.ExtractDepth
	}
	if params.Format == "" {
		params.Format = defaults.Format
	}
	return params
}

// ComponentSpec returns the Python-compatible TavilyExtract Canvas surface.
func (t *TavilyExtractTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"urls":          "The URLs to extract content from.",
			"extract_depth": `Extraction depth: "basic" or "advanced".`,
			"format":        `Output format: "markdown" or "text".`,
		},
		Outputs: map[string]string{
			"json": "Raw Tavily Extract results.",
		},
		InputForm: map[string]any{
			"urls":          map[string]any{"name": "URLs", "type": "line"},
			"extract_depth": map[string]any{"name": "Extract depth", "type": "line"},
			"format":        map[string]any{"name": "Format", "type": "line"},
		},
	}
}

// BuildComponentOutputs converts TavilyExtract's complete tool envelope into
// its public Canvas outputs.
func (t *TavilyExtractTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	return map[string]any{"json": envelopeSlice(envelope, "results")}
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
