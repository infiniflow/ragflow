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
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const search1APIToolName = "search1api_search"

const search1APIToolDescription = `Search1API searches the web or the news through one API, using the search service you pick. When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Use the news channel for current events, and the general channel otherwise.
   - Pick a search service that fits the question, e.g. "github" for code, "arxiv" for papers, "reddit" for discussions, "youtube" for videos, "baidu" for Chinese web pages, "hackernews" or "reuters" for news. Leave it out to use the service set on the node.
   - Optionally restrict results by how recently they were published.`

// search1APIMaxResults is the most results Search1API returns for one search.
const search1APIMaxResults = 50

const search1APIPromptMaxTokens = 200000

const (
	search1APIChannelGeneral = "general"
	search1APIChannelNews    = "news"
)

var search1APIChannels = []string{search1APIChannelGeneral, search1APIChannelNews}

// Search services accepted by each channel's endpoint. A service outside the
// channel's list is rejected by Search1API, so it is checked before the call.
var search1APIGeneralServices = []string{
	"google", "bing", "bingcn", "duckduckgo", "yahoo", "yandex", "youtube", "x", "reddit",
	"github", "arxiv", "wechat", "bilibili", "imdb", "wikipedia", "baidu", "360", "quark",
}

var search1APINewsServices = []string{"google", "bing", "duckduckgo", "yahoo", "hackernews", "reuters"}

// search1APIServices is every service the model may pick, across both channels.
var search1APIServices = append(slices.Clone(search1APIGeneralServices), "hackernews", "reuters")

// "any" means no recency limit and is not sent.
var search1APITimeRanges = []string{"any", "day", "week", "month", "year"}

// search1APIEndpoints are the Search1API URLs per channel. Exposed as a
// package var so tests can substitute it through rewriteHostTransport.
var search1APIEndpoints = map[string]string{
	search1APIChannelGeneral: "https://api.search1api.com/search",
	search1APIChannelNews:    "https://api.search1api.com/news",
}

// search1APIArgs is the JSON shape the model sends into InvokableRun.
// max_results is decoded loosely: a missing, blank or non-numeric value means
// "use the node's Top N" rather than failing the run.
type search1APIArgs struct {
	Query         string `json:"query"`
	Channel       string `json:"channel"`
	SearchService string `json:"search_service"`
	TimeRange     string `json:"time_range"`
	MaxResults    any    `json:"max_results"`
}

// search1APIDefaults holds the node-level configuration. The model's
// channel and search_service, when given, override the node's.
type search1APIDefaults struct {
	Channel       string
	SearchService string
	TopN          int
}

type search1APIRequestBody struct {
	Query         string `json:"query"`
	SearchService string `json:"search_service,omitempty"`
	MaxResults    int    `json:"max_results"`
	TimeRange     string `json:"time_range,omitempty"`
}

// search1APIResponse preserves every upstream result field because the Canvas
// json output exposes the result list unchanged.
type search1APIResponse struct {
	Results []map[string]any `json:"results"`
}

type search1APIEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// Search1APITool is the Search1API web and news search tool. It POSTs a
// search to https://api.search1api.com/search or /news and returns the
// upstream `results` array. The API key is node configuration only and never
// model-visible.
type Search1APITool struct {
	helper   *HTTPHelper
	apiKey   string
	defaults search1APIDefaults
}

var _ ToolComponent = (*Search1APITool)(nil)
var _ ReferenceBuilder = (*Search1APITool)(nil)

// NewSearch1APITool returns a Search1APITool with no API key and the default
// HTTPHelper.
func NewSearch1APITool() *Search1APITool {
	return newSearch1APITool(nil, "", search1APIDefaults{})
}

// NewSearch1APIToolWith returns a Search1APITool that uses the provided
// HTTPHelper and API key. Useful for tests that want to inject a custom
// transport.
func NewSearch1APIToolWith(h *HTTPHelper, apiKey string) *Search1APITool {
	return newSearch1APITool(h, apiKey, search1APIDefaults{})
}

func newSearch1APITool(h *HTTPHelper, apiKey string, defaults search1APIDefaults) *Search1APITool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.Channel == "" {
		defaults.Channel = search1APIChannelGeneral
	}
	if defaults.TopN <= 0 {
		defaults.TopN = 10
	}
	return &Search1APITool{helper: h, apiKey: strings.TrimSpace(apiKey), defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (s *Search1APITool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: search1APIToolName,
		Desc: search1APIToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with Search1API. The most important words/terms (and synonyms) from the original request.",
				Required: true,
			},
			"channel": {
				Type:     schema.String,
				Desc:     `Search channel: "news" for current events, "general" for a broader web search. Leave it out to use the channel set on the node.`,
				Enum:     search1APIChannels,
				Required: false,
			},
			"search_service": {
				Type: schema.String,
				Desc: fmt.Sprintf(
					"The search service to query. The general channel supports: %s. The news channel supports: %s. Leave it out to use the service set on the node.",
					strings.Join(search1APIGeneralServices, ", "),
					strings.Join(search1APINewsServices, ", "),
				),
				Enum:     search1APIServices,
				Required: false,
			},
			"time_range": {
				Type:     schema.String,
				Desc:     `Restrict results by recency: "day", "week", "month", "year", or "any" (default) for no limit.`,
				Enum:     search1APITimeRanges,
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Number of results to return, 1 to 50. Leave it out to use the Top N set on the node.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun performs the Search1API search.
//
// Recoverable errors (bad arguments, missing API key, API or network failures)
// are returned as a JSON envelope with an _ERROR field and a nil Go error, so
// the ReAct agent feeds them back to the model instead of failing the run,
// matching SofyaTool.
func (s *Search1APITool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args search1APIArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return search1APIErrJSON(fmt.Errorf("search1api: parse arguments: %w", err)), nil
	}
	if strings.TrimSpace(args.Query) == "" {
		return search1APIJSON(search1APIEnvelope{Results: []map[string]any{}}), nil
	}
	if s.apiKey == "" {
		return search1APIErrJSON(fmt.Errorf("search1api: api_key is required")), nil
	}

	channel, err := search1APIEnumArg("channel", args.Channel, search1APIChannels, s.defaults.Channel)
	if err != nil {
		return search1APIErrJSON(err), nil
	}
	searchService, err := s.searchServiceArg(channel, args.SearchService)
	if err != nil {
		return search1APIErrJSON(err), nil
	}
	timeRange, err := search1APIEnumArg("time_range", args.TimeRange, search1APITimeRanges, "any")
	if err != nil {
		return search1APIErrJSON(err), nil
	}
	if timeRange == "any" {
		timeRange = ""
	}
	maxResults := search1APIMaxResultsArg(args.MaxResults, s.defaults.TopN)

	body, _ := json.Marshal(search1APIRequestBody{
		Query:         args.Query,
		SearchService: searchService,
		MaxResults:    maxResults,
		TimeRange:     timeRange,
	})
	resp, err := s.helper.Do(ctx,
		http.MethodPost, search1APIEndpoints[channel], string(body), "application/json",
		map[string]string{
			"Authorization": "Bearer " + s.apiKey,
			"Accept":        "application/json",
			// Identifies RAGFlow to Search1API.
			"User-Agent": "RAGFlow search1api-integration/infiniflow-ragflow",
		},
	)
	if err != nil {
		return search1APIErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return search1APIErrJSON(fmt.Errorf("search1api: upstream returned %d", resp.StatusCode)), nil
	}

	var raw search1APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return search1APIErrJSON(fmt.Errorf("search1api: decode response: %w", err)), nil
	}
	results := raw.Results
	if results == nil {
		results = []map[string]any{}
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return search1APIJSON(search1APIEnvelope{Results: results}), nil
}

// searchServiceArg picks the search service for the channel. A service the
// model names must belong to the channel. The node's service is used when the
// model names none; if the model switched to a channel that does not offer
// it, no service is sent and Search1API uses its own default.
func (s *Search1APITool) searchServiceArg(channel, value string) (string, error) {
	allowed := search1APIChannelServices(channel)
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		if slices.Contains(allowed, s.defaults.SearchService) {
			return s.defaults.SearchService, nil
		}
		return "", nil
	}
	if !slices.Contains(allowed, v) {
		return "", fmt.Errorf("search1api: search_service %q is not supported by the %s channel, it should be one of %v", value, channel, allowed)
	}
	return v, nil
}

func search1APIChannelServices(channel string) []string {
	if channel == search1APIChannelNews {
		return search1APINewsServices
	}
	return search1APIGeneralServices
}

// search1APIEnumArg normalizes a model-supplied enum value. Blank means the
// default; anything else must be a known value, because it is forwarded to
// Search1API as-is.
func search1APIEnumArg(name, value string, allowed []string, fallback string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return fallback, nil
	}
	if slices.Contains(allowed, v) {
		return v, nil
	}
	return "", fmt.Errorf("search1api: %s %q is not supported, it should be one of %v", name, value, allowed)
}

// search1APIMaxResultsArg returns the model's result count if it gave a
// usable one, else the node's Top N, clamped to [1, 50].
func search1APIMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), search1APIMaxResults)
}

// ComponentSpec returns the Search1APISearch Canvas surface.
func (s *Search1APITool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":          "The search keywords to execute with Search1API.",
			"channel":        `Search channel: "general" or "news".`,
			"search_service": "The search service to query.",
			"time_range":     `Recency limit: "day", "week", "month", "year" or "any".`,
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Search1API references for downstream prompts.",
			"json":               "Raw Search1API result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
			"channel": map[string]any{
				"name":    "Channel",
				"type":    "options",
				"value":   search1APIChannelGeneral,
				"options": search1APIChannels,
			},
			"time_range": map[string]any{
				"name":    "Time range",
				"type":    "options",
				"value":   "any",
				"options": search1APITimeRanges,
			},
		},
	}
}

func (s *Search1APITool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildSearch1APIReferences(envelope)
}

func (s *Search1APITool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildSearch1APIReferences(envelope)
	return map[string]any{
		"formalized_content": renderSearch1APIReferences(chunks, search1APIPromptMaxTokens),
		"json":               results,
	}
}

// buildSearch1APIReferences prefers a result's page content and falls back to
// its search snippet. `content` is only present when Search1API read the page.
// A result with neither is dropped rather than stored as a blank chunk.
func buildSearch1APIReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := search1APIText(result["content"])
		if content == "" {
			content = search1APIText(result["snippet"])
		}
		content = truncateSearch1APIRunes(strings.Join(strings.Fields(content), " "), 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(search1APIHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(search1APIHashInt(documentID, 500), 10)
		title := search1APIText(result["title"])
		resultURL := search1APIText(result["link"])
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

func renderSearch1APIReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := search1APIText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + search1APIText(chunk["id"]),
			"├── Title: " + strings.Join(strings.Fields(search1APIText(chunk["document_name"])), " "),
			"├── URL: " + search1APIText(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func search1APIText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func search1APIHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateSearch1APIRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func search1APIJSON(env search1APIEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"search1api: marshal result: %s"}`, err)
	}
	return string(b)
}

func search1APIErrJSON(err error) string {
	return search1APIJSON(search1APIEnvelope{Error: err.Error()})
}
