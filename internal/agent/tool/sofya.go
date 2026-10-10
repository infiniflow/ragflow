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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const sofyaToolName = "sofya_search"

const sofyaToolDescription = `Sofya is a web search API for AI agents. It returns the content of the result pages, not just a snippet, so results carry usable context without a separate fetch. When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Use the news topic for current events, and the general topic otherwise.
   - Optionally restrict results by how recently they were published.`

// sofyaMaxResults is the most results Sofya returns for one search.
const sofyaMaxResults = 20

const sofyaPromptMaxTokens = 200000

// "basic" reads the result pages and returns their content; "snippets"
// returns the search snippets only, which is faster and cheaper.
var sofyaSearchDepths = []string{"basic", "snippets"}

var sofyaTopics = []string{"general", "news"}

// Sofya also accepts a date range for freshness. Only the named windows are
// offered, so the value can be checked against a fixed list. "any" means no
// limit and is not sent.
var sofyaFreshnessValues = []string{"any", "day", "week", "month", "year"}

// sofyaEndpoint is the Sofya search URL. Exposed as a package var so tests
// can substitute it through rewriteHostTransport.
var sofyaEndpoint = "https://sofya.co/v1/search"

// sofyaArgs is the JSON shape the model sends into InvokableRun.
// max_results is decoded loosely: a missing, blank or non-numeric value means
// "use the node's Top N" rather than failing the run.
type sofyaArgs struct {
	Query      string `json:"query"`
	Topic      string `json:"topic"`
	Freshness  string `json:"freshness"`
	MaxResults any    `json:"max_results"`
}

// sofyaDefaults holds the node-level configuration.
type sofyaDefaults struct {
	SearchDepth string
	TopN        int
}

type sofyaRequestBody struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth"`
	MaxResults  int    `json:"max_results"`
	Topic       string `json:"topic"`
	Freshness   string `json:"freshness,omitempty"`
}

// sofyaResponse preserves every upstream result field because the Canvas json
// output exposes the result list unchanged.
type sofyaResponse struct {
	Results []map[string]any `json:"results"`
}

type sofyaEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// SofyaTool is the Sofya web search tool. It POSTs a search to
// https://sofya.co/v1/search and returns the upstream `results` array.
// The API key is node configuration only and never model-visible.
type SofyaTool struct {
	helper   *HTTPHelper
	apiKey   string
	defaults sofyaDefaults
}

var _ ToolComponent = (*SofyaTool)(nil)
var _ ReferenceBuilder = (*SofyaTool)(nil)

// NewSofyaTool returns a SofyaTool with no API key and the default HTTPHelper.
func NewSofyaTool() *SofyaTool {
	return newSofyaTool(nil, "", sofyaDefaults{})
}

// NewSofyaToolWith returns a SofyaTool that uses the provided HTTPHelper and
// API key. Useful for tests that want to inject a custom transport.
func NewSofyaToolWith(h *HTTPHelper, apiKey string) *SofyaTool {
	return newSofyaTool(h, apiKey, sofyaDefaults{})
}

func newSofyaTool(h *HTTPHelper, apiKey string, defaults sofyaDefaults) *SofyaTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.SearchDepth == "" {
		defaults.SearchDepth = "basic"
	}
	if defaults.TopN <= 0 {
		defaults.TopN = 10
	}
	return &SofyaTool{helper: h, apiKey: strings.TrimSpace(apiKey), defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (s *SofyaTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: sofyaToolName,
		Desc: sofyaToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with Sofya. The most important words/terms (and synonyms) from the original request.",
				Required: true,
			},
			"topic": {
				Type:     schema.String,
				Desc:     `Search category: "news" for current events, "general" (default) for a broader web search.`,
				Enum:     sofyaTopics,
				Required: false,
			},
			"freshness": {
				Type:     schema.String,
				Desc:     `Restrict results by recency: "day", "week", "month", "year", or "any" (default) for no limit.`,
				Enum:     sofyaFreshnessValues,
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

// InvokableRun performs the Sofya search.
//
// Recoverable errors (bad arguments, missing API key, API or network failures)
// are returned as a JSON envelope with an _ERROR field and a nil Go error, so
// the ReAct agent feeds them back to the model instead of failing the run,
// matching TavilyTool.
func (s *SofyaTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args sofyaArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return sofyaErrJSON(fmt.Errorf("sofya: parse arguments: %w", err)), nil
	}
	if strings.TrimSpace(args.Query) == "" {
		return sofyaJSON(sofyaEnvelope{Results: []map[string]any{}}), nil
	}
	if s.apiKey == "" {
		return sofyaErrJSON(fmt.Errorf("sofya: api_key is required")), nil
	}

	topic, err := sofyaEnumArg("topic", args.Topic, sofyaTopics, "general")
	if err != nil {
		return sofyaErrJSON(err), nil
	}
	freshness, err := sofyaEnumArg("freshness", args.Freshness, sofyaFreshnessValues, "any")
	if err != nil {
		return sofyaErrJSON(err), nil
	}
	if freshness == "any" {
		freshness = ""
	}
	maxResults := sofyaMaxResultsArg(args.MaxResults, s.defaults.TopN)

	body, _ := json.Marshal(sofyaRequestBody{
		Query:       args.Query,
		SearchDepth: s.defaults.SearchDepth,
		MaxResults:  maxResults,
		Topic:       topic,
		Freshness:   freshness,
	})
	resp, err := s.helper.Do(ctx,
		http.MethodPost, sofyaEndpoint, string(body), "application/json",
		map[string]string{
			"Authorization": "Bearer " + s.apiKey,
			"Accept":        "application/json",
			// Identifies RAGFlow to Sofya.
			"User-Agent": "RAGFlow sofya-integration/infiniflow-ragflow",
		},
	)
	if err != nil {
		return sofyaErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return sofyaErrJSON(fmt.Errorf("sofya: upstream returned %d", resp.StatusCode)), nil
	}

	var raw sofyaResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return sofyaErrJSON(fmt.Errorf("sofya: decode response: %w", err)), nil
	}
	results := raw.Results
	if results == nil {
		results = []map[string]any{}
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return sofyaJSON(sofyaEnvelope{Results: results}), nil
}

// sofyaEnumArg normalizes a model-supplied enum value. Blank means the
// default; anything else must be a known value, because it is forwarded to
// Sofya as-is.
func sofyaEnumArg(name, value string, allowed []string, fallback string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return fallback, nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", fmt.Errorf("sofya: %s %q is not supported, it should be one of %v", name, value, allowed)
}

// sofyaMaxResultsArg returns the model's result count if it gave a usable
// one, else the node's Top N, clamped to [1, 20].
func sofyaMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), sofyaMaxResults)
}

// ComponentSpec returns the SofyaSearch Canvas surface.
func (s *SofyaTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":     "The search keywords to execute with Sofya.",
			"topic":     `Search topic: "general" or "news".`,
			"freshness": `Recency limit: "day", "week", "month", "year" or "any".`,
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Sofya references for downstream prompts.",
			"json":               "Raw Sofya result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
			"topic": map[string]any{
				"name":    "Topic",
				"type":    "options",
				"value":   "general",
				"options": sofyaTopics,
			},
			"freshness": map[string]any{
				"name":    "Freshness",
				"type":    "options",
				"value":   "any",
				"options": sofyaFreshnessValues,
			},
		},
	}
}

func (s *SofyaTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildSofyaReferences(envelope)
}

func (s *SofyaTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildSofyaReferences(envelope)
	return map[string]any{
		"formalized_content": renderSofyaReferences(chunks, sofyaPromptMaxTokens),
		"json":               results,
	}
}

// buildSofyaReferences prefers a result's page content and falls back to its
// search snippet. `content` is empty when the page was not read (and always in
// snippets mode); `description` is the snippet. A result with neither is
// dropped rather than stored as a blank chunk.
func buildSofyaReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := sofyaText(result["content"])
		if content == "" {
			content = sofyaText(result["description"])
		}
		content = truncateSofyaRunes(strings.Join(strings.Fields(content), " "), 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(sofyaHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(sofyaHashInt(documentID, 500), 10)
		title := sofyaText(result["title"])
		resultURL := sofyaText(result["url"])
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

func renderSofyaReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := sofyaText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + sofyaText(chunk["id"]),
			"├── Title: " + strings.Join(strings.Fields(sofyaText(chunk["document_name"])), " "),
			"├── URL: " + sofyaText(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func sofyaText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func sofyaHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateSofyaRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func sofyaJSON(env sofyaEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"sofya: marshal result: %s"}`, err)
	}
	return string(b)
}

func sofyaErrJSON(err error) string {
	return sofyaJSON(sofyaEnvelope{Error: err.Error()})
}
