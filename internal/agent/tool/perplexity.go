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
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const perplexityToolName = "perplexity_search"

const perplexityToolDescription = `Perplexity Search API is a web search API. It returns ranked pages with a title, URL, snippet and, when known, a publication date. When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Optionally restrict results by how recently they were published.`

const perplexityEndpoint = "https://api.perplexity.ai/search"

// perplexityMaxResults caps how many results one search returns; the API
// accepts 1 to 20 for the web and fast search types.
const perplexityMaxResults = 20

const perplexityDefaultTopN = 10

const perplexityPromptMaxTokens = 200000

// perplexityMaxDomainFilters is the API's limit on search_domain_filter entries.
const perplexityMaxDomainFilters = 20

// perplexityMaxResponseBytes caps how much of the response body is read: twenty
// hits of JSON is a few KiB, so 4 MiB is far above any legitimate answer while
// still bounding what a misbehaving upstream can make us allocate.
const perplexityMaxResponseBytes = 4 << 20

const (
	perplexitySearchTypeWeb  = "web"
	perplexitySearchTypeFast = "fast"
)

var perplexitySearchTypes = []string{perplexitySearchTypeWeb, perplexitySearchTypeFast}

var perplexityRecencyValues = []string{"any", "hour", "day", "week", "month", "year"}

// perplexityArgs is the JSON shape the model sends into InvokableRun.
// max_results is decoded loosely: a missing, blank or non-numeric value means
// "use the node's Top N" rather than failing the run.
type perplexityArgs struct {
	Query               string `json:"query"`
	SearchRecencyFilter string `json:"search_recency_filter"`
	MaxResults          any    `json:"max_results"`
}

// perplexityDefaults holds the node-level configuration.
type perplexityDefaults struct {
	SearchType          string
	SearchRecencyFilter string
	SearchDomainFilter  []string
	Country             string
	TopN                int
}

type perplexityRequestBody struct {
	Query               string   `json:"query"`
	MaxResults          int      `json:"max_results"`
	SearchType          string   `json:"search_type"`
	SearchRecencyFilter string   `json:"search_recency_filter,omitempty"`
	SearchDomainFilter  []string `json:"search_domain_filter,omitempty"`
	Country             string   `json:"country,omitempty"`
}

// perplexityResponse preserves every upstream result field (title, url,
// snippet, date, last_updated) because the Canvas json output exposes the
// result list unchanged.
type perplexityResponse struct {
	Results []map[string]any `json:"results"`
}

type perplexityEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// PerplexityTool is the Perplexity Search API tool. It POSTs a search to
// https://api.perplexity.ai/search and returns the upstream `results` array.
// The API key is node configuration only and never model-visible.
type PerplexityTool struct {
	helper   *HTTPHelper
	apiKey   string
	defaults perplexityDefaults
}

var _ ToolComponent = (*PerplexityTool)(nil)
var _ ReferenceBuilder = (*PerplexityTool)(nil)

// NewPerplexityTool returns a PerplexityTool with no API key and the default
// HTTPHelper.
func NewPerplexityTool() *PerplexityTool {
	return newPerplexityTool(nil, "", perplexityDefaults{})
}

// NewPerplexityToolWith returns a PerplexityTool that uses the provided
// HTTPHelper and API key. Useful for tests that want to inject a custom
// transport.
func NewPerplexityToolWith(h *HTTPHelper, apiKey string) *PerplexityTool {
	return newPerplexityTool(h, apiKey, perplexityDefaults{})
}

func newPerplexityTool(h *HTTPHelper, apiKey string, defaults perplexityDefaults) *PerplexityTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.SearchType == "" {
		defaults.SearchType = perplexitySearchTypeWeb
	}
	if defaults.SearchRecencyFilter == "" {
		defaults.SearchRecencyFilter = "any"
	}
	if defaults.TopN <= 0 {
		defaults.TopN = perplexityDefaultTopN
	}
	return &PerplexityTool{helper: h, apiKey: strings.TrimSpace(apiKey), defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (p *PerplexityTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: perplexityToolName,
		Desc: perplexityToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with the Perplexity Search API. The most important words/terms (and synonyms) from the original request.",
				Required: true,
			},
			"search_recency_filter": {
				Type:     schema.String,
				Desc:     `Restrict results by recency: "hour", "day", "week", "month", "year", or "any" (default) for no limit.`,
				Enum:     perplexityRecencyValues,
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Number of results to return, 1 to 20. Leave it out to use the Top N set on the node.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun performs the Perplexity search.
//
// Recoverable errors (bad arguments, missing API key, API or network failures)
// are returned as a JSON envelope with an _ERROR field and a nil Go error, so
// the ReAct agent feeds them back to the model instead of failing the run,
// matching Search1APITool.
func (p *PerplexityTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args perplexityArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return perplexityErrJSON(fmt.Errorf("perplexity: parse arguments: %w", err)), nil
	}
	if strings.TrimSpace(args.Query) == "" {
		return perplexityJSON(perplexityEnvelope{Results: []map[string]any{}}), nil
	}
	if p.apiKey == "" {
		return perplexityErrJSON(fmt.Errorf("perplexity: api_key is required")), nil
	}
	if !perplexityKeyIsWellFormed(p.apiKey) {
		// Checked before the request is built so the key never reaches
		// net/http, which would reject it as an invalid header value.
		return perplexityErrJSON(fmt.Errorf("perplexity: api_key contains control characters")), nil
	}

	recency, err := perplexityEnumArg("search_recency_filter", args.SearchRecencyFilter, perplexityRecencyValues, p.defaults.SearchRecencyFilter)
	if err != nil {
		return perplexityErrJSON(err), nil
	}
	if recency == "any" {
		recency = ""
	}
	maxResults := perplexityMaxResultsArg(args.MaxResults, p.defaults.TopN)

	body, _ := json.Marshal(perplexityRequestBody{
		Query:               args.Query,
		MaxResults:          maxResults,
		SearchType:          p.defaults.SearchType,
		SearchRecencyFilter: recency,
		SearchDomainFilter:  p.defaults.SearchDomainFilter,
		Country:             p.defaults.Country,
	})
	resp, err := p.helper.Do(ctx,
		http.MethodPost, perplexityEndpoint, string(body), "application/json",
		map[string]string{
			"Authorization": "Bearer " + p.apiKey,
			"Accept":        "application/json",
			// Identifies RAGFlow to Perplexity so API usage can be attributed to
			// the calling product.
			"User-Agent":         "RAGFlow perplexity-integration/infiniflow-ragflow",
			"X-Pplx-Integration": "ragflow",
		},
	)
	if err != nil {
		return perplexityErrJSON(fmt.Errorf("perplexity: %w", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return perplexityErrJSON(perplexityStatusError(resp)), nil
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, perplexityMaxResponseBytes+1))
	if err != nil {
		return perplexityErrJSON(fmt.Errorf("perplexity: read response: %w", err)), nil
	}
	if len(raw) > perplexityMaxResponseBytes {
		return perplexityErrJSON(fmt.Errorf("perplexity: response body exceeds %d bytes", perplexityMaxResponseBytes)), nil
	}
	var decoded perplexityResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return perplexityErrJSON(fmt.Errorf("perplexity: upstream returned a body that is not valid JSON")), nil
	}
	results := decoded.Results
	if results == nil {
		results = []map[string]any{}
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return perplexityJSON(perplexityEnvelope{Results: results}), nil
}

// perplexityStatusError turns a non-2xx response into a fixed, status-only
// message the model can act on. 401 (invalid key), 422 (request rejected) and
// 429 (rate limited) are the documented failure codes. The response body is
// deliberately not read or echoed: it is upstream-controlled text and must not
// reach the model, the canvas, or logs.
func perplexityStatusError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("perplexity: upstream returned 401 (invalid API key)")
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("perplexity: upstream returned 422 (request rejected, check search_domain_filter, country and max_results)")
	case http.StatusTooManyRequests:
		return fmt.Errorf("perplexity: upstream returned 429 (rate limited, retry later)")
	}
	return fmt.Errorf("perplexity: upstream returned %d", resp.StatusCode)
}

// perplexityKeyIsWellFormed reports whether the key can be placed in an HTTP
// header: printable, no control characters (CR, LF, NUL, tabs and friends).
func perplexityKeyIsWellFormed(key string) bool {
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// perplexityEnumArg normalizes a model-supplied enum value. Blank means the
// default; anything else must be a known value, because it is forwarded to
// Perplexity as-is.
func perplexityEnumArg(name, value string, allowed []string, fallback string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return fallback, nil
	}
	if slices.Contains(allowed, v) {
		return v, nil
	}
	return "", fmt.Errorf("perplexity: %s %q is not supported, it should be one of %v", name, value, allowed)
}

// perplexityMaxResultsArg returns the model's result count if it gave a usable
// one, else the node's Top N, clamped to [1, 20].
func perplexityMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), perplexityMaxResults)
}

// perplexityDomainFilterArg normalizes the node-level domain filter. It accepts
// a comma or newline separated string or a list of strings, trims each entry,
// drops blanks and keeps the API's maximum of 20 entries. A leading "-"
// excludes a domain, as documented by the API, and is passed through.
func perplexityDomainFilterArg(value any) ([]string, error) {
	var entries []string
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		entries = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' })
	case []string:
		entries = v
	case []any:
		entries = make([]string, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("perplexity: search_domain_filter entries must be strings")
			}
			entries = append(entries, text)
		}
	default:
		return nil, fmt.Errorf("perplexity: search_domain_filter must be a string or a list of strings")
	}
	domains := make([]string, 0, len(entries))
	for _, entry := range entries {
		if domain := strings.TrimSpace(entry); domain != "" {
			domains = append(domains, domain)
		}
	}
	if len(domains) > perplexityMaxDomainFilters {
		return nil, fmt.Errorf("perplexity: search_domain_filter accepts at most %d domains", perplexityMaxDomainFilters)
	}
	if len(domains) == 0 {
		return nil, nil
	}
	// The API runs in allowlist mode (no prefix) or denylist mode ("-"
	// prefix), never both in one request. Reject a mix here with a clear
	// message instead of letting upstream answer 422.
	excluded := 0
	for _, domain := range domains {
		if strings.HasPrefix(domain, "-") {
			excluded++
		}
	}
	if excluded != 0 && excluded != len(domains) {
		return nil, fmt.Errorf("perplexity: search_domain_filter must list either domains to include or domains to exclude (prefixed with -), not both")
	}
	return domains, nil
}

// ComponentSpec returns the PerplexitySearch Canvas surface.
func (p *PerplexityTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":                 "The search keywords to execute with the Perplexity Search API.",
			"search_recency_filter": `Recency limit: "hour", "day", "week", "month", "year" or "any".`,
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Perplexity references for downstream prompts.",
			"json":               "Raw Perplexity result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
			"search_type": map[string]any{
				"name":    "Search type",
				"type":    "options",
				"value":   perplexitySearchTypeWeb,
				"options": perplexitySearchTypes,
			},
			"search_recency_filter": map[string]any{
				"name":    "Recency",
				"type":    "options",
				"value":   "any",
				"options": perplexityRecencyValues,
			},
		},
	}
}

func (p *PerplexityTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildPerplexityReferences(envelope)
}

func (p *PerplexityTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildPerplexityReferences(envelope)
	return map[string]any{
		"formalized_content": renderPerplexityReferences(chunks, perplexityPromptMaxTokens),
		"json":               results,
	}
}

// buildPerplexityReferences turns each usable result into the chunk/docAgg
// shape the citation path expects. A result with no snippet is dropped rather
// than stored as a blank chunk.
func buildPerplexityReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := truncatePerplexityRunes(strings.Join(strings.Fields(perplexityText(result["snippet"])), " "), 10000)
		if content == "" {
			continue
		}
		title := perplexityText(result["title"])
		resultURL := perplexityText(result["url"])
		documentID, displayID := referenceIDs(resultURL, content)
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

func renderPerplexityReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := perplexityText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + perplexityText(chunk["id"]),
			"├── Title: " + strings.Join(strings.Fields(perplexityText(chunk["document_name"])), " "),
			"├── URL: " + perplexityText(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func perplexityText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func truncatePerplexityRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func perplexityJSON(env perplexityEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"perplexity: marshal result: %s"}`, err)
	}
	return string(b)
}

func perplexityErrJSON(err error) string {
	return perplexityJSON(perplexityEnvelope{Error: err.Error()})
}
