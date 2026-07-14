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

// Package component Universe A delegation wrappers. Canvas-facing components that
// delegate to their corresponding Universe B eino tool
// implementations. The delegation pattern keeps the canvas
// scheduler's Component contract thin and the eino tool's
// InvokableRun interface as the actual implementation seam.
//
// Primary registration: TavilySearch, Retrieval (incl. the
// Python-typo SearchMyDataset alias), and ExeSQL all delegate to
// the real Universe B tools. fixture_stubs.go's init() wires the
// registry to these wrappers; the legacy stub-only path is
// preserved as NewRetrievalStub / NewExeSQLStub for unit tests
// that want to assert the "no service wired" state directly.
package component

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	einotool "github.com/cloudwego/eino/components/tool"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// tavilySearchComponent delegates to internal/agent/tool/TavilyTool.
// The underlying tool makes a real HTTP call; the wrapper is the
// canvas-facing surface.
type tavilySearchComponent struct {
	inner  *agenttool.TavilyTool
	apiKey string
}

func newTavilySearchComponent(params map[string]any) (Component, error) {
	return &tavilySearchComponent{
		inner:  agenttool.NewTavilyTool(),
		apiKey: stringParam(params["api_key"]),
	}, nil
}

func (c *tavilySearchComponent) Name() string { return "TavilySearch" }

func (c *tavilySearchComponent) Inputs() map[string]string {
	return map[string]string{
		"query":        "Search query.",
		"api_key":      "Tavily API key (overrides TAVILY_API_KEY env var).",
		"max_results":  "Maximum results to return (default 5).",
		"search_depth": "\"basic\" (default) or \"advanced\".",
	}
}

func (c *tavilySearchComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"json":               "Raw result list (url, title, content, score).",
	}
}

func (c *tavilySearchComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *tavilySearchComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if c.apiKey != "" && stringParam(inputs["api_key"]) == "" {
		inputs["api_key"] = c.apiKey
	}
	if strings.TrimSpace(stringParam(inputs["query"])) == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, _ := json.Marshal(inputs)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: TavilySearch: %w", err)
	}
	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderTavilySearchResults(results),
		"json":               results,
	}, nil
}

func (c *tavilySearchComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// googleComponent wraps internal/agent/tool/GoogleTool for canvas execution and
// adapts the tool envelope to the Google component outputs.
type googleComponent struct {
	inner  *agenttool.GoogleTool
	params map[string]any
}

func newGoogleComponent(params map[string]any) (Component, error) {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &googleComponent{inner: agenttool.NewGoogleTool(), params: cloned}, nil
}

func (c *googleComponent) Name() string { return "Google" }

func (c *googleComponent) Inputs() map[string]string {
	return map[string]string{
		"q":        "Search query.",
		"api_key":  "SerpApi API key.",
		"start":    "Result offset.",
		"num":      "Maximum number of results.",
		"country":  "Google country code.",
		"language": "Google language code.",
	}
}

func (c *googleComponent) GetInputForm() map[string]any {
	return agenttool.NewGoogleTool().InputForm()
}

func (c *googleComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"json":               "Raw Google organic result list.",
	}
}

func (c *googleComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := make(map[string]any, len(c.params)+len(inputs)+2)
	for k, v := range c.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}
	if _, ok := merged["q"]; !ok {
		if query, ok := merged["query"]; ok {
			merged["q"] = query
		}
	}
	if _, ok := merged["num"]; !ok {
		if maxResults, ok := merged["max_results"]; ok {
			merged["num"] = maxResults
		}
	}

	argsJSON, _ := json.Marshal(merged)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	results := anySlice(decoded["organic_results"])
	if len(results) == 0 {
		results = anySlice(decoded["results"])
	}
	formalized := renderGoogleResults(results)
	if existing, _ := decoded["_ERROR"].(string); strings.TrimSpace(existing) != "" {
		return map[string]any{"formalized_content": formalized, "json": results, "_ERROR": existing}, nil
	}
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{"formalized_content": formalized, "json": results, "_ERROR": decoded["_ERROR"]}, nil
		}
		return nil, fmt.Errorf("canvas: Google: %w", err)
	}
	return map[string]any{"formalized_content": formalized, "json": results}, nil
}

func (c *googleComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderGoogleResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title := strings.TrimSpace(stringParam(m["title"]))
		link := strings.TrimSpace(stringParam(m["link"]))
		content := strings.TrimSpace(stringParam(m["snippet"]))
		if content == "" {
			content = strings.TrimSpace(googleAboutDescription(m["about_this_result"]))
		}
		if content == "" {
			continue
		}
		lines := []string{}
		if title != "" {
			lines = append(lines, "Title: "+title)
		}
		if link != "" {
			lines = append(lines, "URL: "+link)
		}
		lines = append(lines, "Content: "+content)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func googleAboutDescription(v any) string {
	about, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	source, ok := about["source"].(map[string]any)
	if !ok {
		return ""
	}
	return stringParam(source["description"])
}

// tavilyExtractComponent delegates to internal/agent/tool/TavilyExtractTool.
type tavilyExtractComponent struct {
	inner  *agenttool.TavilyExtractTool
	apiKey string
}

func newTavilyExtractComponent(params map[string]any) (Component, error) {
	return &tavilyExtractComponent{
		inner:  agenttool.NewTavilyExtractTool(),
		apiKey: stringParam(params["api_key"]),
	}, nil
}

func (c *tavilyExtractComponent) Name() string { return "TavilyExtract" }

func (c *tavilyExtractComponent) Inputs() map[string]string {
	return map[string]string{
		"urls":          "URLs to extract content from. Accepts a comma-separated string or array.",
		"api_key":       "Tavily API key (overrides TAVILY_API_KEY env var).",
		"extract_depth": "\"basic\" (default) or \"advanced\".",
		"format":        "\"markdown\" (default) or \"text\".",
	}
}

func (c *tavilyExtractComponent) GetInputForm() map[string]any {
	return map[string]any{
		"urls": map[string]any{
			"name": "URLs",
			"type": "line",
		},
	}
}

func (c *tavilyExtractComponent) Outputs() map[string]string {
	return map[string]string{
		"json": "Raw Tavily Extract results.",
	}
}

func (c *tavilyExtractComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if c.apiKey != "" && stringParam(inputs["api_key"]) == "" {
		inputs["api_key"] = c.apiKey
	}

	argsJSON, _ := json.Marshal(inputs)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{"json": []any{}, "_ERROR": decoded["_ERROR"]}, nil
		}
		return nil, fmt.Errorf("canvas: TavilyExtract: %w", err)
	}
	return map[string]any{"json": anySlice(decoded["results"])}, nil
}

func (c *tavilyExtractComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// bgptInvoker is the subset of BGPTTool used by the canvas wrapper.
type bgptInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

type duckDuckGoInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

type keenableInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

// bgptComponent delegates to internal/agent/tool/BGPTTool and adapts
// the tool envelope to the BGPT canvas output contract.
type bgptComponent struct {
	inner  bgptInvoker
	apiKey string
}

func newBGPTComponent(params map[string]any) (Component, error) {
	return newBGPTComponentWithInvoker(agenttool.NewBGPTTool(), stringParam(params["api_key"])), nil
}

func newBGPTComponentWithInvoker(inner bgptInvoker, apiKey ...string) Component {
	c := &bgptComponent{inner: inner}
	if len(apiKey) > 0 {
		c.apiKey = apiKey[0]
	}
	return c
}

func (c *bgptComponent) Name() string { return "BGPT" }

func (c *bgptComponent) Inputs() map[string]string {
	return map[string]string{
		"query":     "Scientific search query.",
		"api_key":   "Optional BGPT API key.",
		"days_back": "Optional recency filter in days.",
		"top_n":     "Maximum number of results.",
	}
}

func (c *bgptComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *bgptComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered scientific paper evidence for downstream LLM prompts.",
		"json":               "Raw BGPT result list.",
	}
}

func (c *bgptComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if c.apiKey != "" && stringParam(inputs["api_key"]) == "" {
		inputs["api_key"] = c.apiKey
	}

	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	args := map[string]any{
		"query": query,
	}
	if apiKey := strings.TrimSpace(stringParam(inputs["api_key"])); apiKey != "" {
		args["api_key"] = apiKey
	}
	if daysBack := toIntParam(inputs["days_back"]); daysBack > 0 {
		args["days_back"] = daysBack
	}
	if topN := toIntParam(inputs["top_n"]); topN > 0 {
		args["num_results"] = topN
	}

	argsJSON, _ := json.Marshal(args)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: BGPT: %w", err)
	}

	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderBGPTResults(results),
		"json":               results,
	}, nil
}

func (c *bgptComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// wikipediaComponent delegates to internal/agent/tool/WikipediaTool.
// Python's canvas component is named "Wikipedia" and stores top_n/language
// on the node params while accepting query at runtime.
type wikipediaComponent struct {
	inner *agenttool.WikipediaTool
}

func newWikipediaComponent(params map[string]any) (Component, error) {
	topN := 10
	if v, ok := params["top_n"]; ok {
		topN = toIntParam(v)
	}
	if topN <= 0 {
		return nil, fmt.Errorf("canvas: Wikipedia: top_n must be a positive integer")
	}
	language := "en"
	if v, ok := params["language"].(string); ok && strings.TrimSpace(v) != "" {
		language = strings.TrimSpace(v)
	}
	if !agenttool.WikipediaLanguageSupported(language) {
		return nil, fmt.Errorf("canvas: Wikipedia: unsupported language %q", language)
	}
	return &wikipediaComponent{inner: agenttool.NewWikipediaToolWithParams(nil, topN, language)}, nil
}

func (c *wikipediaComponent) Name() string { return "Wikipedia" }

func (c *wikipediaComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "The search keyword to execute with wikipedia. The keyword MUST be a specific subject that can match the title.",
	}
}

func (c *wikipediaComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *wikipediaComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered Wikipedia article summaries for downstream LLM prompts.",
		"json":               "Raw Wikipedia result list.",
	}
}

func (c *wikipediaComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, _ := json.Marshal(map[string]any{"query": query})
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if results, ok := decoded["results"]; ok {
		decoded["json"] = results
	}
	if err != nil {
		if len(decoded) > 0 {
			if _, ok := decoded["formalized_content"]; !ok {
				decoded["formalized_content"] = ""
			}
			if _, ok := decoded["json"]; !ok {
				decoded["json"] = []any{}
			}
			return decoded, nil
		}
		return nil, fmt.Errorf("canvas: Wikipedia: %w", err)
	}
	return decoded, nil
}

func (c *wikipediaComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// duckDuckGoComponent delegates to internal/agent/tool/DuckDuckGoTool.
type duckDuckGoComponent struct {
	inner duckDuckGoInvoker
}

func newDuckDuckGoComponent(_ map[string]any) (Component, error) {
	return newDuckDuckGoComponentWithInvoker(agenttool.NewDuckDuckGoTool()), nil
}

func newDuckDuckGoComponentWithInvoker(inner duckDuckGoInvoker) Component {
	return &duckDuckGoComponent{inner: inner}
}

func (c *duckDuckGoComponent) Name() string { return "DuckDuckGo" }

func (c *duckDuckGoComponent) Inputs() map[string]string {
	return map[string]string{
		"query":   "Search query.",
		"channel": "Search channel: general or news.",
		"top_n":   "Maximum number of results.",
	}
}

func (c *duckDuckGoComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
		"channel": map[string]any{
			"name":    "Channel",
			"type":    "options",
			"value":   "general",
			"options": []string{"general", "news"},
		},
	}
}

func (c *duckDuckGoComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"json":               "Raw result list.",
	}
}

func (c *duckDuckGoComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	args := map[string]any{
		"query": query,
	}
	if channel := strings.TrimSpace(stringParam(inputs["channel"])); channel != "" {
		args["channel"] = channel
	}
	if topN := toIntParam(inputs["top_n"]); topN > 0 {
		args["top_n"] = topN
	}

	argsJSON, _ := json.Marshal(args)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: DuckDuckGo: %w", err)
	}

	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderDuckDuckGoResults(results),
		"json":               results,
	}, nil
}

func (c *duckDuckGoComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// keenableSearchComponent delegates to internal/agent/tool/KeenableTool.
type keenableSearchComponent struct {
	inner  keenableInvoker
	apiKey string
	mode   string
	topN   int
	site   string
}

func newKeenableSearchComponent(params map[string]any) (Component, error) {
	apiKey := stringParam(params["api_key"])
	return newKeenableSearchComponentWithInvoker(agenttool.NewKeenableToolWithAPIKey(nil, apiKey), params), nil
}

func newKeenableSearchComponentWithInvoker(inner keenableInvoker, params map[string]any) Component {
	if inner == nil {
		inner = agenttool.NewKeenableTool()
	}
	mode := strings.TrimSpace(stringParam(params["mode"]))
	if mode == "" {
		mode = "pro"
	}
	topN := toIntParam(params["top_n"])
	if topN <= 0 {
		topN = 10
	}
	return &keenableSearchComponent{
		inner:  inner,
		apiKey: stringParam(params["api_key"]),
		mode:   mode,
		topN:   topN,
		site:   stringParam(params["site"]),
	}
}

func (c *keenableSearchComponent) Name() string { return "KeenableSearch" }

func (c *keenableSearchComponent) Inputs() map[string]string {
	return map[string]string{
		"query":   "Search query.",
		"site":    "Optional single-domain filter.",
		"api_key": "Optional Keenable API key.",
		"mode":    "Search mode: pro or realtime.",
		"top_n":   "Maximum number of results.",
	}
}

func (c *keenableSearchComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
		"site": map[string]any{
			"name": "Site",
			"type": "line",
		},
	}
}

func (c *keenableSearchComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"json":               "Raw Keenable result list.",
	}
}

func (c *keenableSearchComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}

	args := map[string]any{
		"query": query,
		"mode":  c.mode,
		"top_n": c.topN,
	}
	if mode := strings.TrimSpace(stringParam(inputs["mode"])); mode != "" {
		args["mode"] = mode
	}
	if topN := toIntParam(inputs["top_n"]); topN > 0 {
		args["top_n"] = topN
	}
	site := strings.TrimSpace(stringParam(inputs["site"]))
	if site == "" {
		site = strings.TrimSpace(c.site)
	}
	if site != "" {
		args["site"] = site
	}

	invoker := c.inner
	if apiKey := strings.TrimSpace(stringParam(inputs["api_key"])); apiKey != "" && apiKey != strings.TrimSpace(c.apiKey) {
		invoker = agenttool.NewKeenableToolWithAPIKey(nil, apiKey)
	}

	argsJSON, _ := json.Marshal(args)
	out, err := invoker.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	results := anySlice(decoded["results"])
	if existing, _ := decoded["_ERROR"].(string); strings.TrimSpace(existing) != "" {
		return map[string]any{
			"formalized_content": "",
			"json":               results,
			"_ERROR":             existing,
		}, nil
	}
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               results,
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: KeenableSearch: %w", err)
	}
	chunks, docAggs := buildKeenableReferences(results)
	if state, _, stateErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); stateErr == nil && state != nil {
		state.SetRetrievalReferences(chunks, docAggs)
	}
	return map[string]any{
		"formalized_content": renderKeenableReferences(chunks),
		"json":               results,
	}, nil
}

func (c *keenableSearchComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func buildKeenableReferences(results []any) ([]map[string]any, []map[string]any) {
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := truncateKeenableRunes(strings.TrimSpace(keenableValueString(m["description"])), 10000)
		if content == "" || content == "None" {
			continue
		}
		documentID := strconv.FormatInt(keenableHashInt(content, 100000000), 10)
		title := keenableValueString(m["title"])
		url := keenableValueString(m["url"])
		displayID := strconv.FormatInt(keenableHashInt(documentID, 500), 10)
		chunks = append(chunks, map[string]any{
			"id":            displayID,
			"chunk_id":      documentID,
			"content":       content,
			"doc_id":        documentID,
			"document_id":   documentID,
			"docnm_kwd":     title,
			"document_name": title,
			"similarity":    1,
			"score":         1,
			"url":           url,
		})
		docAggs = append(docAggs, map[string]any{
			"doc_name": title,
			"doc_id":   documentID,
			"count":    1,
			"url":      url,
		})
	}
	return chunks, docAggs
}

func renderKeenableReferences(chunks []map[string]any) string {
	if len(chunks) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + keenableValueString(chunk["id"]),
			"├── Title: " + keenableValueString(chunk["docnm_kwd"]),
			"├── URL: " + keenableValueString(chunk["url"]),
			"└── Content:\n" + keenableValueString(chunk["content"]),
		}, "\n"))
	}
	return strings.Join(blocks, "\n")
}

func keenableValueString(value any) string {
	if value == nil {
		return "None"
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func keenableHashInt(value string, modulus int64) int64 {
	sum := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(sum[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateKeenableRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func renderDuckDuckGoResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field := func(key string) string {
			v, ok := m[key]
			if !ok || v == nil {
				return "-"
			}
			text := strings.TrimSpace(fmt.Sprintf("%v", v))
			if text == "" {
				return "-"
			}
			return text
		}
		blocks = append(blocks, strings.Join([]string{
			fmt.Sprintf("Title: %s", field("title")),
			fmt.Sprintf("URL: %s", field("url")),
			fmt.Sprintf("Body: %s", field("body")),
		}, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func stringParam(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func anySlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, item)
		}
		return out
	default:
		return []any{}
	}
}

func renderTavilySearchResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title := strings.TrimSpace(fmt.Sprint(m["title"]))
		url := strings.TrimSpace(fmt.Sprint(m["url"]))
		content := strings.TrimSpace(fmt.Sprint(m["raw_content"]))
		if content == "" || content == "<nil>" {
			content = strings.TrimSpace(fmt.Sprint(m["content"]))
		}
		if content == "" || content == "<nil>" {
			continue
		}
		lines := []string{}
		if title != "" && title != "<nil>" {
			lines = append(lines, fmt.Sprintf("Title: %s", title))
		}
		if url != "" && url != "<nil>" {
			lines = append(lines, fmt.Sprintf("URL: %s", url))
		}
		lines = append(lines, content)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func renderBGPTResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		bgptField := func(key string) string {
			v, ok := m[key]
			if !ok || v == nil {
				return "-"
			}
			switch vv := v.(type) {
			case string:
				if text := strings.TrimSpace(vv); text != "" {
					return text
				}
			default:
				if text := strings.TrimSpace(fmt.Sprintf("%v", vv)); text != "" {
					return text
				}
			}
			return "-"
		}
		lines := []string{
			fmt.Sprintf("Title: %s", bgptField("title")),
			fmt.Sprintf("Authors: %s", bgptField("authors")),
			fmt.Sprintf("Journal: %s", bgptField("journal")),
			fmt.Sprintf("Year: %s", bgptField("year")),
			fmt.Sprintf("DOI: %s", bgptField("doi")),
			fmt.Sprintf("Abstract: %s", bgptField("abstract")),
			fmt.Sprintf("Methods: %s", bgptField("methods")),
			fmt.Sprintf("Sample size / population: %s", bgptField("sample_size")),
			fmt.Sprintf("Results: %s", bgptField("results")),
			fmt.Sprintf("Limitations: %s", bgptField("limitations")),
			fmt.Sprintf("Conflicts of interest: %s", bgptField("conflict_of_interest")),
			fmt.Sprintf("Data availability: %s", bgptField("data_availability")),
			fmt.Sprintf("Blind spots: %s", bgptField("blind_spots")),
			fmt.Sprintf("How to falsify: %s", bgptField("falsify")),
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// retrievalParams mirrors the Python RetrievalParam shape: the
// values the canvas node declares at build time, applied as
// defaults to the per-invocation RetrievalRequest. The fields are
// the same the Python agent/component/retrieval.py exposes.
type retrievalParams struct {
	KbIDs                    []string
	TopN                     int
	TopK                     int
	SimilarityThreshold      float64
	KeywordsSimilarityWeight float64
	RerankID                 string
	EmptyResponse            string
}

// parseRetrievalParams reads the v1 DSL node params for Retrieval.
// Unknown keys are ignored; nil/empty/missing-key inputs yield a
// zero-value retrievalParams which the tool layer treats as
// "default everything". This matches Python's
// component.retrieval.RetrievalParam.__init__ tolerance.
func parseRetrievalParams(params map[string]any) retrievalParams {
	out := retrievalParams{
		EmptyResponse: "Sorry, no relevant content was found in the knowledge base.",
	}
	if params == nil {
		return out
	}
	if v, ok := params["kb_ids"].([]any); ok {
		for _, x := range v {
			if s, ok := x.(string); ok {
				out.KbIDs = append(out.KbIDs, s)
			}
		}
	}
	if v, ok := params["kb_ids"].([]string); ok {
		out.KbIDs = append(out.KbIDs, v...)
	}
	if v, ok := params["top_n"]; ok {
		out.TopN = toIntParam(v)
	}
	if v, ok := params["top_k"]; ok {
		out.TopK = toIntParam(v)
	}
	if v, ok := params["similarity_threshold"]; ok {
		out.SimilarityThreshold = toFloatParam(v)
	}
	if v, ok := params["keywords_similarity_weight"]; ok {
		out.KeywordsSimilarityWeight = toFloatParam(v)
	}
	if v, ok := params["rerank_id"].(string); ok {
		out.RerankID = v
	}
	if v, ok := params["empty_response"].(string); ok {
		out.EmptyResponse = v
	}
	return out
}

// retrievalComponent delegates to internal/agent/tool/RetrievalTool.
// The wrapper captures the v1 DSL node params (kb_ids, top_n,
// top_k, similarity_threshold, keywords_similarity_weight,
// rerank_id, empty_response) at build time and applies them as
// defaults to each invocation. Per-call inputs override the
// defaults.
type retrievalComponent struct {
	inner  *agenttool.RetrievalTool
	params retrievalParams
}

var legacyRetrievalQueryPattern = regexp.MustCompile(`(?s)^\s*UserFillUp:\s*(.*?)\s+Input\s+(.*?)\s*$`)

func newRetrievalComponent(params map[string]any) (Component, error) {
	return &retrievalComponent{
		inner:  agenttool.NewRetrievalTool(),
		params: parseRetrievalParams(params),
	}, nil
}

func (c *retrievalComponent) Name() string { return "Retrieval" }

func (c *retrievalComponent) Inputs() map[string]string {
	return map[string]string{
		"query":       "Natural-language search query.",
		"dataset_ids": "Optional list of dataset IDs to restrict the search to (overrides node-level kb_ids).",
		"top_n":       "Maximum chunks to return (default 8, overrides node-level top_n).",
		"use_kg":      "GraphRAG toggle (returns ErrKGRetrievalServiceMissing until a kg adapter is registered).",
	}
}

func (c *retrievalComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered chunks for downstream LLM prompts.",
		"chunks":             "Raw chunk payloads (id, document_id, content, score).",
	}
}

func (c *retrievalComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := c.applyDefaults(inputs)
	normalizeLegacyRetrievalInputs(ctx, merged)
	common.Debug("agent retrieval component: invoke",
		zap.Any("inputs", inputs),
		zap.Any("merged", merged),
	)
	argsJSON, _ := json.Marshal(merged)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: Retrieval: %w", err)
	}
	common.Debug("agent retrieval component: output",
		zap.String("tool_output", out),
	)
	return parseToolEnvelope(out), nil
}

func (c *retrievalComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	// V1: retrieval is a non-streaming node (the Python
	// Retrieval component also blocks on Dealer.search). A
	// streaming retrieval lands with the streaming-dealer
	// follow-up phase. The single-chunk fallback convention
	// (one {"_raw": "..."} frame) is intentionally NOT
	// emitted here because the canvas scheduler treats
	// nil-stream as "non-streaming node, read Invoke() output"
	// — a fallback frame would confuse downstream cpn wiring.
	return nil, nil
}

// applyDefaults folds the node-level params into the per-call
// input map. Per-call values always win; node-level values fill
// the gaps. This mirrors Python's RetrievalParam semantics where
// the canvas DSL declares the defaults and the runtime input
// overrides them.
//
// v1 → tool field name: the wrapper writes node-level kb_ids
// (the v1 DSL / Python surface) but the tool's retrievalArgs
// JSON struct only declares `dataset_ids`. Passing kb_ids
// through unchanged would silently drop the filter (the tool's
// json.Unmarshal would never see the key). To avoid that
// regression, we normalise the merged map here: any `kb_ids`
// present (whether from defaults or inputs) is copied into
// `dataset_ids` (unless dataset_ids is already set), and the
// stale `kb_ids` key is removed so the marshalled payload to
// the tool carries exactly one canonical name.
func (c *retrievalComponent) applyDefaults(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs)+8)
	for k, v := range inputs {
		out[k] = v
	}
	if _, ok := out["kb_ids"]; !ok && len(c.params.KbIDs) > 0 {
		ids := make([]any, len(c.params.KbIDs))
		for i, s := range c.params.KbIDs {
			ids[i] = s
		}
		out["kb_ids"] = ids
	}
	if _, ok := out["top_n"]; !ok && c.params.TopN > 0 {
		out["top_n"] = c.params.TopN
	}
	if _, ok := out["top_k"]; !ok && c.params.TopK > 0 {
		out["top_k"] = c.params.TopK
	}
	if _, ok := out["similarity_threshold"]; !ok && c.params.SimilarityThreshold > 0 {
		out["similarity_threshold"] = c.params.SimilarityThreshold
	}
	if _, ok := out["keywords_similarity_weight"]; !ok && c.params.KeywordsSimilarityWeight > 0 {
		out["keywords_similarity_weight"] = c.params.KeywordsSimilarityWeight
	}
	if _, ok := out["rerank_id"]; !ok && c.params.RerankID != "" {
		out["rerank_id"] = c.params.RerankID
	}
	if _, ok := out["empty_response"]; !ok && c.params.EmptyResponse != "" {
		out["empty_response"] = c.params.EmptyResponse
	}
	// Translate v1 DSL name `kb_ids` to the tool's expected
	// name `dataset_ids`. dataset_ids already-set wins; kb_ids
	// is consumed and removed so the marshalled JSON carries a
	// single canonical key. Without this step, the tool's
	// retrievalArgs.DatasetIDs would be empty even when the
	// caller supplied kb_ids at build or call time.
	if kbIDs, ok := out["kb_ids"]; ok {
		if _, hasDatasetIDs := out["dataset_ids"]; !hasDatasetIDs {
			out["dataset_ids"] = kbIDs
		}
		delete(out, "kb_ids")
	}
	return out
}

func normalizeLegacyRetrievalInputs(ctx context.Context, out map[string]any) {
	if normalizeStructuredRetrievalInputs(ctx, out) {
		return
	}
	rawQuery, _ := out["query"].(string)
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return
	}
	matches := legacyRetrievalQueryPattern.FindStringSubmatch(rawQuery)
	if len(matches) != 3 {
		return
	}
	kbName := strings.TrimSpace(matches[1])
	queryText := strings.TrimSpace(matches[2])
	if queryText != "" {
		out["query"] = queryText
	}
	if _, hasDatasetIDs := out["dataset_ids"]; hasDatasetIDs {
		return
	}
	if kbName == "" {
		return
	}
	if datasetID := resolveRetrievalDatasetID(ctx, kbName); datasetID != "" {
		out["dataset_ids"] = []string{datasetID}
	}
}

func normalizeStructuredRetrievalInputs(ctx context.Context, out map[string]any) bool {
	_, hasDatasetIDs := out["dataset_ids"]
	candidateMaps := []map[string]any{}
	if stateMap, ok := out["state"].(map[string]any); ok {
		if raw, ok := stateMap["UserFillUp:KBInput"].(map[string]any); ok {
			candidateMaps = append(candidateMaps, raw)
		}
	}
	candidateMaps = append(candidateMaps, out)

	consumed := false
	for _, candidate := range candidateMaps {
		kbName, _ := candidate["kb"].(string)
		queryText, _ := candidate["query"].(string)
		if kbName == "" && legacyRetrievalQueryPattern.MatchString(strings.TrimSpace(queryText)) {
			continue
		}
		if kbName == "" && queryText == "" {
			continue
		}
		consumed = true
		if queryText != "" {
			out["query"] = queryText
		}
		if kbName != "" && !hasDatasetIDs {
			if datasetID := resolveRetrievalDatasetID(ctx, strings.TrimSpace(kbName)); datasetID != "" {
				out["dataset_ids"] = []string{datasetID}
				common.Debug("agent retrieval component: resolved dataset id",
					zap.String("kb", strings.TrimSpace(kbName)),
					zap.String("dataset_id", datasetID))
			}
		}
		if queryText != "" {
			return true
		}
		if kbName != "" && out["dataset_ids"] != nil {
			return true
		}
	}
	return consumed
}

func resolveRetrievalDatasetID(ctx context.Context, kbName string) string {
	if kbName == "" {
		return ""
	}
	if kb, err := dao.NewKnowledgebaseDAO().GetByID(kbName); err == nil && kb != nil {
		common.Debug("agent retrieval component: resolved dataset id by direct id",
			zap.String("kb", kbName),
			zap.String("dataset_id", kb.ID))
		return kb.ID
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.Warn("agent retrieval component: resolve dataset id by id failed",
			zap.String("kb", kbName),
			zap.Error(err))
	}
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		common.Debug("agent retrieval component: resolve dataset id context",
			zap.String("kb", kbName),
			zap.Any("sys_query", state.Sys["query"]),
			zap.Any("tenant_id", state.Sys["tenant_id"]),
			zap.Any("user_id", state.Sys["user_id"]))
		if tenantID, _ := state.Sys["tenant_id"].(string); tenantID != "" {
			if kb, lookupErr := dao.NewKnowledgebaseDAO().GetByName(kbName, tenantID); lookupErr == nil && kb != nil {
				common.Debug("agent retrieval component: resolved dataset id by tenant",
					zap.String("kb", kbName),
					zap.String("tenant_id", tenantID),
					zap.String("dataset_id", kb.ID))
				return kb.ID
			} else if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				common.Warn("agent retrieval component: resolve dataset id by tenant failed",
					zap.String("kb", kbName),
					zap.String("tenant_id", tenantID),
					zap.Error(lookupErr))
			} else {
				common.Debug("agent retrieval component: tenant lookup missed",
					zap.String("kb", kbName),
					zap.String("tenant_id", tenantID))
			}
		}
		if userID, _ := state.Sys["user_id"].(string); userID != "" {
			if kbs, lookupErr := dao.NewKnowledgebaseDAO().GetKBByNameAndUserID(kbName, userID); lookupErr == nil && len(kbs) > 0 {
				for _, kb := range kbs {
					if kb == nil || kb.Status == nil || *kb.Status != string(entity.StatusValid) {
						continue
					}
					common.Debug("agent retrieval component: resolved dataset id by user visibility",
						zap.String("kb", kbName),
						zap.String("user_id", userID),
						zap.String("dataset_id", kb.ID))
					return kb.ID
				}
			} else if lookupErr != nil {
				common.Warn("agent retrieval component: resolve dataset id by name failed",
					zap.String("kb", kbName),
					zap.String("user_id", userID),
					zap.Error(lookupErr))
			} else {
				common.Debug("agent retrieval component: user visibility lookup missed",
					zap.String("kb", kbName),
					zap.String("user_id", userID))
			}
		}
	} else {
		common.Debug("agent retrieval component: resolve dataset id missing canvas state",
			zap.String("kb", kbName),
			zap.Error(err))
	}
	common.Debug("agent retrieval component: dataset id unresolved",
		zap.String("kb", kbName))
	return ""
}

// exesqlComponent delegates to internal/agent/tool/ExeSQLTool. The
// connection params (db_type, host, port, database, username,
// password) are passed via the canvas node's params map at build
// time, matching Python's ExeSQLParam semantics.
//
// v1 → tool param translation: the legacy v1 ExeSQL canvas node
// surface used (database, username, host, port, password, top_n)
// and did NOT declare db_type. The tool, by contrast, REQUIRES
// db_type (and uses max_records for the row cap, not top_n). A
// naive passthrough would turn every v1 canvas into a build-time
// error (NewExeSQLConnParams returns "missing required connection
// params (db_type/host/database/username)"). The adapter below
// bridges the two surfaces so existing v1 DSLs keep compiling.
//
// Defaults applied: db_type defaults to "mysql" (matches the v1
// Python default); top_n is mapped to max_records; port is coerced
// from JSON-decoded float64 to int. See TestExeSQL_V1DSLParamsAccepted.
func newExeSQLComponent(params map[string]any) (Component, error) {
	toolParams := translateExeSQLParamsToToolShape(params)
	conn, err := agenttool.NewExeSQLConnParams(toolParams)
	if err != nil {
		return nil, fmt.Errorf("canvas: ExeSQL: %w", err)
	}
	return &exesqlComponent{
		inner: agenttool.NewExeSQLTool(conn),
		sql:   conn.SQL,
	}, nil
}

// translateExeSQLParamsToToolShape adapts a v1 DSL ExeSQL params
// map into the tool's expected param surface. Idempotent: callers
// that already supply db_type / max_records / int-typed port pass
// through unchanged.
//
// Field map:
//
//	v1 surface          → tool surface
//	-------------------   --------------
//	db_type (optional)  → db_type        (defaults to "mysql")
//	database            → database
//	username            → username
//	host                → host
//	port (float64)      → port           (coerced to int)
//	password            → password
//	top_n (numeric)     → max_records    (and dropped from out)
//
// Returns a fresh map; the input is not mutated.
func translateExeSQLParamsToToolShape(v1Params map[string]any) map[string]any {
	out := make(map[string]any, len(v1Params)+2)
	for k, v := range v1Params {
		out[k] = v
	}
	// db_type: required by the tool, absent in v1 DSL — default
	// to mysql to match the v1 Python default and most legacy
	// canvases. Operators wanting a different engine can set
	// db_type explicitly in the params map.
	if _, ok := out["db_type"]; !ok {
		out["db_type"] = "mysql"
	}
	// port: JSON-decoded numeric comes through as float64, but
	// NewExeSQLConnParams asserts on int via type-switch. Coerce.
	if v, ok := out["port"]; ok {
		switch x := v.(type) {
		case float64:
			out["port"] = int(x)
		case int64:
			out["port"] = int(x)
		}
	}
	// top_n: v1's row-limit param. Map to max_records (the tool's
	// equivalent). If both keys are present, max_records wins — the
	// tool's name is the canonical one.
	if v, ok := out["top_n"]; ok {
		if _, hasMaxRecords := out["max_records"]; !hasMaxRecords {
			switch x := v.(type) {
			case float64:
				out["max_records"] = int(x)
			case int:
				out["max_records"] = x
			case int64:
				out["max_records"] = int(x)
			}
		}
		delete(out, "top_n")
	}
	return out
}

type exeSQLInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

type exesqlComponent struct {
	inner exeSQLInvoker
	sql   string
}

func (c *exesqlComponent) Name() string { return "ExeSQL" }

func (c *exesqlComponent) Inputs() map[string]string {
	return map[string]string{
		"sql": "SQL statement to execute (SELECT-only; DML/DDL rejected).",
	}
}

func (c *exesqlComponent) GetInputForm() map[string]any {
	return map[string]any{
		"sql": map[string]any{
			"name": "SQL",
			"type": "line",
		},
	}
}

func (c *exesqlComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "SQL result rendered as Markdown.",
		"json":               "Raw SQL statement results.",
	}
}

func (c *exesqlComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	sqlText := c.sql
	if value, ok := inputs["sql"].(string); ok && strings.TrimSpace(value) != "" {
		sqlText = value
	}
	if state, _, stateErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); stateErr == nil && state != nil {
		resolved, resolveErr := runtime.ResolveTemplate(sqlText, state)
		if resolveErr != nil {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             resolveErr.Error(),
			}, nil
		}
		sqlText = resolved
	}

	argsJSON, err := json.Marshal(map[string]any{"sql": sqlText})
	if err != nil {
		return nil, fmt.Errorf("canvas: ExeSQL: encode SQL: %w", err)
	}
	out, invokeErr := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	result := formatExeSQLCanvasOutput(decoded)
	if invokeErr != nil {
		if message := stringParam(decoded["_ERROR"]); message != "" {
			result["_ERROR"] = message
		} else {
			result["_ERROR"] = invokeErr.Error()
		}
	}
	return result, nil
}

func formatExeSQLCanvasOutput(decoded map[string]any) map[string]any {
	rows := anySlice(decoded["rows"])
	columns := anySlice(decoded["columns"])
	jsonResult := make([]any, 0, 1)
	if len(rows) == 1 {
		if row, ok := rows[0].(map[string]any); ok && len(row) == 1 {
			if _, hasContent := row["content"]; hasContent {
				jsonResult = append(jsonResult, row)
			}
		}
	}
	if len(jsonResult) == 0 && len(rows) > 0 {
		jsonResult = append(jsonResult, rows)
	}
	result := map[string]any{
		"formalized_content": renderExeSQLMarkdown(columns, rows),
		"json":               jsonResult,
	}
	if message := stringParam(decoded["_ERROR"]); message != "" {
		result["_ERROR"] = message
	}
	return result
}

func renderExeSQLMarkdown(columns, rows []any) string {
	if len(rows) == 0 {
		return ""
	}
	for _, value := range rows {
		if row, ok := value.(map[string]any); ok && len(row) == 1 {
			if message, exists := row["content"]; exists {
				return stringParam(message)
			}
		}
	}
	columnNames := make([]string, 0, len(columns))
	for _, column := range columns {
		columnNames = append(columnNames, fmt.Sprint(column))
	}
	if len(columnNames) == 0 {
		if first, ok := rows[0].(map[string]any); ok {
			for column := range first {
				columnNames = append(columnNames, column)
			}
			sort.Strings(columnNames)
		}
	}
	if len(columnNames) == 0 {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(columnNames, " | "))
	separators := make([]string, len(columnNames))
	for i := range separators {
		separators[i] = "---"
	}
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(separators, " | "))
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		cells := make([]string, len(columnNames))
		for i, column := range columnNames {
			cells[i] = strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(row[column]), "|", "\\|"), "\n", "<br>")
		}
		fmt.Fprintf(&builder, "| %s |\n", strings.Join(cells, " | "))
	}
	return strings.TrimSuffix(builder.String(), "\n")
}

func (c *exesqlComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// codeExecComponent delegates to internal/agent/tool/CodeExecTool.
// The node-level params map carries the legacy v1 DSL surface
// (`lang`, `script`, `arguments`, optional `timeout`). Per-call inputs
// override those defaults so resolved canvas refs win at invocation
// time, while static DSL-provided literals still flow through.
type codeExecComponent struct {
	inner   *agenttool.CodeExecTool
	params  map[string]any
	outputs map[string]any
}

func newCodeExecComponent(params map[string]any) (Component, error) {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &codeExecComponent{
		inner:   agenttool.NewCodeExecTool(),
		params:  cloned,
		outputs: cloneAnyMap(asAnyMap(params["outputs"])),
	}, nil
}

func (c *codeExecComponent) Name() string { return "CodeExec" }

func (c *codeExecComponent) Inputs() map[string]string {
	return map[string]string{
		"lang":      "Programming language: python/python3/javascript/nodejs.",
		"script":    "Code to execute. Should define main(...).",
		"arguments": "Arguments passed to main(...) as keyword args / object fields.",
		"timeout":   "Optional per-execution timeout in seconds.",
	}
}

func (c *codeExecComponent) GetInputForm() map[string]any {
	res := make(map[string]any, len(c.params))
	for k, _ := range c.params {
		res[k] = map[string]any{
			"type": "line",
			"name": k,
		}
	}
	return res
}

func (c *codeExecComponent) Outputs() map[string]string {
	return map[string]string{
		"result":      "The main(...) return value rendered as the legacy CodeExec result field.",
		"content":     "Raw CodeExec tool content field.",
		"_ERROR":      "Execution or sandbox error message.",
		"actual_type": "Runtime type inferred by the sandbox bridge.",
		"stdout":      "Captured stdout.",
		"stderr":      "Captured stderr.",
		"exit_code":   "Process exit code.",
	}
}

func (c *codeExecComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := make(map[string]any, len(c.params)+len(inputs))
	for k, v := range c.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}
	if rawArgs, ok := merged["arguments"].(map[string]any); ok {
		state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
		merged["arguments"] = resolveCodeExecArguments(rawArgs, merged, state)
	}
	common.Debug("CodeExec wrapper invoke",
		zap.Int("params_keys", len(c.params)),
		zap.Int("inputs_keys", len(inputs)),
		zap.Int("merged_keys", len(merged)),
		zap.Bool("has_arguments", merged["arguments"] != nil))
	argsJSON, _ := json.Marshal(merged)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if c.outputs != nil {
		applyCodeExecBusinessOutputs(decoded, c.outputs)
	} else if rawResult, ok := decoded["raw_result"]; ok {
		decoded["result"] = rawResult
		if _, ok := decoded["_ERROR"]; !ok {
			decoded["_ERROR"] = ""
		}
	} else if content, ok := decoded["content"]; ok {
		decoded["result"] = content
		if _, ok := decoded["_ERROR"]; !ok {
			decoded["_ERROR"] = ""
		}
	}
	if err != nil {
		return decoded, fmt.Errorf("canvas: CodeExec: %w", err)
	}
	return decoded, nil
}

func (c *codeExecComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// parseToolEnvelope decodes the JSON envelope returned by eino tool
// InvokableRun into a map[string]any. The result has whatever keys
// the tool's result type carries (rows/columns/chunks/etc.).
func parseToolEnvelope(jsonStr string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		// Tool returned non-JSON; surface the raw string under a
		// known key so the caller can still see something.
		return map[string]any{"_raw": jsonStr}
	}
	return out
}

func applyCodeExecBusinessOutputs(decoded map[string]any, outputs map[string]any) {
	if decoded == nil {
		return
	}
	rawResult := resolveCodeExecBusinessResult(decoded)
	common.Debug("CodeExec wrapper",
		zap.Int("decoded_keys", len(decoded)),
		zap.Bool("has_raw_result", rawResult != nil),
		zap.Bool("has_content", decoded["content"] != nil),
		zap.Int("outputs_keys", len(outputs)))
	if existingErr, _ := decoded["_ERROR"].(string); strings.TrimSpace(existingErr) != "" {
		for name := range outputs {
			if isCodeExecSystemOutput(name) {
				continue
			}
			decoded[name] = nil
		}
		if _, ok := decoded["actual_type"]; !ok {
			decoded["actual_type"] = agenttool.InferCodeExecActualType(rawResult)
		}
		if _, ok := decoded["content"]; !ok {
			decoded["content"] = agenttool.RenderCodeExecCanonicalContent(rawResult)
		}
		return
	}
	contract, err := agenttool.BuildCodeExecContract(outputs, rawResult)
	if err != nil {
		for name := range outputs {
			if isCodeExecSystemOutput(name) {
				continue
			}
			decoded[name] = nil
		}
		decoded["actual_type"] = agenttool.InferCodeExecActualType(rawResult)
		decoded["_ERROR"] = err.Error()
		if _, ok := decoded["content"]; !ok {
			decoded["content"] = agenttool.RenderCodeExecCanonicalContent(rawResult)
		}
		return
	}

	decoded["_ERROR"] = ""
	decoded["actual_type"] = contract.ActualType
	decoded["content"] = contract.Content
	decoded[contract.BusinessOutput] = contract.Value
}

func resolveCodeExecBusinessResult(decoded map[string]any) any {
	if decoded == nil {
		return nil
	}
	if rawResult, ok := decoded["raw_result"]; ok {
		return rawResult
	}
	content, _ := decoded["content"].(string)
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		return parsed
	}
	return content
}

func isCodeExecSystemOutput(name string) bool {
	switch name {
	case "content", "actual_type", "attachments", "_ERROR", "_ARTIFACTS", "_ATTACHMENT_CONTENT", "raw_result", "_created_time", "_elapsed_time":
		return true
	default:
		return false
	}
}

func asAnyMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func resolveCodeExecArguments(args map[string]any, merged map[string]any, state *runtime.CanvasState) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = resolveCodeExecArgumentValue(v, merged, state)
	}
	return out
}

func resolveCodeExecArgumentValue(v any, merged map[string]any, state *runtime.CanvasState) any {
	switch x := v.(type) {
	case map[string]any:
		return resolveCodeExecArguments(x, merged, state)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, resolveCodeExecArgumentValue(item, merged, state))
		}
		return out
	case string:
		if resolved, ok := lookupCodeExecArgumentRef(x, merged, state); ok {
			return resolved
		}
		return x
	default:
		return v
	}
}

func lookupCodeExecArgumentRef(ref string, merged map[string]any, state *runtime.CanvasState) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, false
	}
	if state != nil {
		if v, err := state.GetVar(ref); err == nil && v != nil {
			return v, true
		}
	}
	at := strings.Index(ref, "@")
	if at <= 0 || at >= len(ref)-1 {
		return nil, false
	}
	cpnID := ref[:at]
	param := ref[at+1:]

	stateByNode, _ := merged["state"].(map[string]map[string]any)
	if bucket, ok := stateByNode[cpnID]; ok {
		if v, ok := bucket[param]; ok {
			return v, true
		}
	}
	return nil, false
}

// toIntParam coerces a node-param int value to int. JSON-decoded
// values come in as float64 when numeric, so we tolerate that
// case explicitly. Strings that parse as int also work.
// (Renamed from toInt to avoid colliding with
// list_operations.go's same-name helper.)
func toIntParam(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

// toFloatParam coerces a node-param float value to float64. Same
// JSON-float64 / numeric-string tolerance as toIntParam.
func toFloatParam(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// yahooFinanceComponent delegates to internal/agent/tool/YahooFinanceTool.
type yahooFinanceComponent struct {
	inner *agenttool.YahooFinanceTool
}

func newYahooFinanceComponent(_ map[string]any) (Component, error) {
	return &yahooFinanceComponent{inner: agenttool.NewYahooFinanceTool()}, nil
}

func (c *yahooFinanceComponent) Name() string { return "YahooFinance" }

func (c *yahooFinanceComponent) Inputs() map[string]string {
	return map[string]string{
		"stock_code": "Stock symbol to look up (e.g. AAPL, MSFT, 0005.HK).",
	}
}

func (c *yahooFinanceComponent) Outputs() map[string]string {
	return map[string]string{
		"report": "Stock quote data (JSON).",
	}
}

func (c *yahooFinanceComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	stockCode, _ := inputs["stock_code"].(string)
	if strings.TrimSpace(stockCode) == "" {
		return map[string]any{"_ERROR": "stock_code is required"}, nil
	}
	toolInput := map[string]any{
		"symbols": []string{stockCode},
	}
	argsJSON, _ := json.Marshal(toolInput)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		if out != "" {
			return parseToolEnvelope(out), nil
		}
		return nil, fmt.Errorf("canvas: YahooFinance: %w", err)
	}
	result := parseToolEnvelope(out)
	return map[string]any{"report": result["results"]}, nil
}

func (c *yahooFinanceComponent) GetInputForm() map[string]any {
	return map[string]any{
		"stock_code": map[string]any{
			"type": "line",
			"name": "Stock code/Company name",
		},
	}
}

func (c *yahooFinanceComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// arxivComponent delegates to internal/agent/tool/ArxivTool. Query is the
// runtime input; top_n and sort_by are validated node parameters.
type arxivComponent struct {
	inner *agenttool.ArxivTool
}

func newArxivComponent(params map[string]any) (Component, error) {
	topN := 12
	if v, ok := params["top_n"]; ok {
		topN = toIntParam(v)
	}
	if topN <= 0 {
		return nil, fmt.Errorf("canvas: ArXiv: top_n must be a positive integer")
	}
	sortBy := "submittedDate"
	if v, ok := params["sort_by"].(string); ok && strings.TrimSpace(v) != "" {
		sortBy = strings.TrimSpace(v)
	}
	if !agenttool.ArxivSortBySupported(sortBy) {
		return nil, fmt.Errorf("canvas: ArXiv: unsupported sort_by %q", sortBy)
	}
	return &arxivComponent{inner: agenttool.NewArxivToolWithParams(nil, topN, sortBy)}, nil
}

func (c *arxivComponent) Name() string { return "ArXiv" }

func (c *arxivComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "Search query.",
	}
}

func (c *arxivComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered arXiv papers for downstream LLM prompts.",
		"json":               "Raw arXiv paper list.",
	}
}

func (c *arxivComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *arxivComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, _ := json.Marshal(map[string]any{"query": query})
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: ArXiv: %w", err)
	}
	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderArxivResults(results),
		"json":               results,
	}, nil
}

func (c *arxivComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderArxivResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		paper, ok := item.(map[string]any)
		if !ok {
			continue
		}
		summary := strings.TrimSpace(stringParam(paper["summary"]))
		if summary == "" {
			continue
		}
		title := strings.TrimSpace(stringParam(paper["title"]))
		url := strings.TrimSpace(stringParam(paper["pdf_url"]))
		lines := make([]string, 0, 3)
		if title != "" {
			lines = append(lines, fmt.Sprintf("Title: %s", title))
		}
		if url != "" {
			lines = append(lines, fmt.Sprintf("URL: %s", url))
		}
		lines = append(lines, summary)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// googleScholarComponent delegates to internal/agent/tool/GoogleScholarTool.
type googleScholarComponent struct {
	inner  googleScholarInvoker
	params map[string]any
}

type googleScholarInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

type pubmedInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

func newGoogleScholarComponent(params map[string]any) (Component, error) {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &googleScholarComponent{
		inner:  agenttool.NewGoogleScholarTool(),
		params: cloned,
	}, nil
}

func newGoogleScholarComponentWithInvoker(inner googleScholarInvoker, params map[string]any) Component {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &googleScholarComponent{inner: inner, params: cloned}
}

func (c *googleScholarComponent) Name() string { return "GoogleScholar" }

func (c *googleScholarComponent) Inputs() map[string]string {
	return map[string]string{
		"query":     "Search query.",
		"top_n":     "Maximum number of results (default 12).",
		"sort_by":   "Sort order: relevance or date.",
		"year_low":  "Earliest publication year to include.",
		"year_high": "Latest publication year to include.",
		"patents":   "Whether to include patents, defaults to true.",
	}
}

func (c *googleScholarComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"json":               "Raw result list.",
	}
}

func (c *googleScholarComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *googleScholarComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := make(map[string]any, len(c.params)+len(inputs))
	for k, v := range c.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}

	query := strings.TrimSpace(stringParam(merged["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	args := map[string]any{
		"query": query,
	}
	if topN := toIntParam(merged["top_n"]); topN > 0 {
		args["top_n"] = topN
	}
	if sortBy := strings.TrimSpace(stringParam(merged["sort_by"])); sortBy != "" {
		args["sort_by"] = sortBy
	}
	if yearLow := toIntParam(merged["year_low"]); yearLow > 0 {
		args["year_low"] = yearLow
	}
	if yearHigh := toIntParam(merged["year_high"]); yearHigh > 0 {
		args["year_high"] = yearHigh
	}
	if patents, ok := merged["patents"].(bool); ok {
		args["patents"] = patents
	} else {
		args["patents"] = true
	}

	argsJSON, _ := json.Marshal(args)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: GoogleScholar: %w", err)
	}

	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderGoogleScholarResults(results),
		"json":               results,
	}, nil
}

func (c *googleScholarComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderGoogleScholarResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field := func(key string) string {
			v, ok := m[key]
			if !ok || v == nil {
				return "-"
			}
			text := strings.TrimSpace(fmt.Sprintf("%v", v))
			if text == "" {
				return "-"
			}
			return text
		}
		blocks = append(blocks, strings.Join([]string{
			fmt.Sprintf("Title: %s", field("title")),
			fmt.Sprintf("URL: %s", field("link")),
			fmt.Sprintf("Authors: %s", field("authors")),
			fmt.Sprintf("Year: %s", field("year")),
			fmt.Sprintf("Snippet: %s", field("snippet")),
		}, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// pubMedComponent delegates to the PubMed tool. Its node parameters are
// consumed at construction time, leaving query as the sole runtime input.
type pubMedComponent struct {
	inner pubmedInvoker
}

func newPubMedComponent(params map[string]any) (Component, error) {
	toolParams := make(map[string]any, 2)
	for _, key := range []string{"top_n", "email"} {
		if value, ok := params[key]; ok {
			toolParams[key] = value
		}
	}
	inner, err := agenttool.BuildByName("pubmed", toolParams)
	if err != nil {
		return nil, err
	}
	invoker, ok := inner.(pubmedInvoker)
	if !ok {
		return nil, fmt.Errorf("PubMed: tool does not implement InvokableRun")
	}
	return newPubMedComponentWithInvoker(invoker), nil
}

func newPubMedComponentWithInvoker(inner pubmedInvoker) Component {
	return &pubMedComponent{inner: inner}
}

func (c *pubMedComponent) Name() string { return "PubMed" }

func (c *pubMedComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "PubMed search query.",
	}
}

func (c *pubMedComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered PubMed references for downstream LLM prompts.",
		"json":               "Raw PubMed result list.",
	}
}

func (c *pubMedComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *pubMedComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, fmt.Errorf("canvas: PubMed: encode query: %w", err)
	}
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	results := anySlice(decoded["results"])
	if existing, _ := decoded["_ERROR"].(string); strings.TrimSpace(existing) != "" {
		return map[string]any{
			"formalized_content": "",
			"json":               results,
			"_ERROR":             existing,
		}, nil
	}
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               results,
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: PubMed: %w", err)
	}
	return map[string]any{
		"formalized_content": renderPubMedResults(results),
		"json":               results,
	}, nil
}

func (c *pubMedComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderPubMedResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for i, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := strings.TrimSpace(stringParam(result["content"]))
		if content == "" {
			continue
		}
		lines := []string{fmt.Sprintf("ID: %d", i)}
		if title := strings.TrimSpace(stringParam(result["title"])); title != "" {
			lines = append(lines, "Title: "+title)
		}
		if link := strings.TrimSpace(stringParam(result["url"])); link != "" {
			lines = append(lines, "URL: "+link)
		}
		lines = append(lines, "Content:", content)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// Compile-time interface checks.
var (
	_ Component = (*retrievalComponent)(nil)
	_ Component = (*tavilySearchComponent)(nil)
	_ Component = (*googleComponent)(nil)
	_ Component = (*tavilyExtractComponent)(nil)
	_ Component = (*duckDuckGoComponent)(nil)
	_ Component = (*exesqlComponent)(nil)
	_ Component = (*codeExecComponent)(nil)
	_ Component = (*arxivComponent)(nil)
	_ Component = (*wikipediaComponent)(nil)
	_ Component = (*googleScholarComponent)(nil)
	_ Component = (*pubMedComponent)(nil)
	_ Component = (*yahooFinanceComponent)(nil)
)

// Compile-time check that the eino InvokableTool methods we call
// are reachable (catches a future refactor that renames them).
var _ einotool.InvokableTool = (*agenttool.TavilyTool)(nil)
var _ einotool.InvokableTool = (*agenttool.WikipediaTool)(nil)
var _ einotool.InvokableTool = (*agenttool.GoogleTool)(nil)
var _ einotool.InvokableTool = (*agenttool.TavilyExtractTool)(nil)
var _ einotool.InvokableTool = (*agenttool.DuckDuckGoTool)(nil)
var _ einotool.InvokableTool = (*agenttool.YahooFinanceTool)(nil)
var _ einotool.InvokableTool = (*agenttool.GoogleScholarTool)(nil)
var _ einotool.InvokableTool = (*agenttool.PubMedTool)(nil)
