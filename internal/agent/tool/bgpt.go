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
	"time"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

const bgptToolName = "bgpt_search"

const bgptToolDescription = "Search scientific papers via BGPT (bgpt.pro) and return structured evidence " +
	"extracted from full-text studies: methods, sample sizes, results, limitations, conflicts of interest, " +
	"data/code availability, study blind spots, quality scores, and falsification prompts."

// bgptEndpoint is the BGPT search URL. Exposed as a package var for tests.
var bgptEndpoint = "https://bgpt.pro/api/mcp-search"

const bgptPromptMaxTokens = 200000

var bgptDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var bgptNewlinePattern = regexp.MustCompile(`\n+`)

// bgptParams holds the Canvas node configuration and the model-emitted
// runtime input. Info exposes only query; api_key / top_n / days_back
// come from canvas DSL params and live in BGPTTool.defaults.
type bgptParams struct {
	Query    string `json:"query"`
	TopN     int    `json:"top_n"`
	APIKey   string `json:"api_key,omitempty"`
	DaysBack int    `json:"days_back,omitempty"`
}

// bgptEnv is what the model sees.
type bgptEnv struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// bgptResponse is the upstream BGPT API envelope.
type bgptResponse struct {
	Results []map[string]interface{} `json:"results"`
	Error   string                   `json:"error,omitempty"`
}

// BGPTTool searches scientific papers via bgpt.pro.
type BGPTTool struct {
	helper   *HTTPHelper
	defaults bgptParams
}

var _ ToolComponent = (*BGPTTool)(nil)
var _ ReferenceBuilder = (*BGPTTool)(nil)

// NewBGPTTool returns a BGPTTool using the default HTTPHelper.
func NewBGPTTool() *BGPTTool {
	return newBGPTTool(nil, bgptParams{})
}

// NewBGPTToolWith returns a BGPTTool with the given helper.
func NewBGPTToolWith(h *HTTPHelper) *BGPTTool {
	return newBGPTTool(h, bgptParams{})
}

func newBGPTTool(h *HTTPHelper, defaults bgptParams) *BGPTTool {
	if h == nil {
		h = NewHTTPHelperWithRetry(RetryConfig{MaxAttempts: 1})
		h.client.Timeout = 25 * time.Second
	}
	if defaults.TopN == 0 {
		defaults.TopN = 10
	}
	return &BGPTTool{helper: h, defaults: defaults}
}

// ToolMeta returns the tool's metadata for the chat model.
func (b *BGPTTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        bgptToolName,
		Description: bgptToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "Natural-language scientific search query.",
				Required:    true,
			},
			"num_results": {
				Type:        ParamTypeInteger,
				Description: "Maximum number of results. Defaults to 10.",
				Required:    false,
			},
			"api_key": {
				Type:        ParamTypeString,
				Description: "Optional BGPT API key. Leave blank for the free tier.",
				Required:    false,
			},
			"days_back": {
				Type:        ParamTypeInteger,
				Description: "Optional recency filter (e.g. 365 for last year).",
				Required:    false,
			},
		},
	}
}

func (b *BGPTTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":     "Scientific search query.",
			"api_key":   "Optional BGPT API key.",
			"days_back": "Optional recency filter in days.",
			"top_n":     "Maximum number of results.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered scientific paper references for downstream prompts.",
			"json":               "Raw BGPT result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
		},
	}
}

// InvokableRun performs the BGPT search.
func (b *BGPTTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p bgptParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: parse arguments: %w", err)),
			fmt.Errorf("bgpt: parse arguments: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return bgptJSON(bgptEnv{Results: []map[string]any{}}), nil
	}
	p = mergeBGPTParams(b.defaults, p)
	if p.TopN <= 0 {
		p.TopN = 10
	}

	reqBody := map[string]interface{}{
		"query":       strings.TrimSpace(p.Query),
		"num_results": p.TopN,
	}
	if p.APIKey != "" {
		reqBody["api_key"] = p.APIKey
	}
	if p.DaysBack > 0 {
		reqBody["days_back"] = p.DaysBack
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return bgptErrJSON(err), err
	}

	resp, err := b.helper.Do(ctx, http.MethodPost, bgptEndpoint, string(bodyBytes), "application/json", map[string]string{"Accept": "application/json"})
	if err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: request failed: %w", err)),
			fmt.Errorf("bgpt: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bgptErrJSON(fmt.Errorf("bgpt: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("bgpt: upstream returned %d", resp.StatusCode)
	}

	var raw bgptResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: parse response: %w", err)),
			fmt.Errorf("bgpt: parse response: %w", err)
	}
	if strings.TrimSpace(raw.Error) != "" {
		err := fmt.Errorf("bgpt: %s", strings.TrimSpace(raw.Error))
		return bgptErrJSON(err), err
	}

	return bgptJSON(bgptEnv{Results: raw.Results}), nil
}

func mergeBGPTParams(defaults, params bgptParams) bgptParams {
	if params.APIKey == "" {
		params.APIKey = defaults.APIKey
	}
	if params.DaysBack == 0 {
		params.DaysBack = defaults.DaysBack
	}
	if params.TopN == 0 {
		params.TopN = defaults.TopN
	}
	return params
}

func (b *BGPTTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildBGPTReferences(envelope)
}

func (b *BGPTTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildBGPTReferences(envelope)
	return map[string]any{
		"formalized_content": renderBGPTReferences(chunks, bgptPromptMaxTokens),
		"json":               results,
	}
}

func buildBGPTReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		paper, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := bgptDataImagePattern.ReplaceAllString(formatBGPTPaper(paper), "")
		content = truncateBGPTRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(bgptHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(bgptHashInt(documentID, 500), 10)
		title := bgptFirstField(paper, "title")
		if title == "-" {
			title = "Untitled"
		}
		resultURL := bgptFirstField(paper, "url", "doi")
		if resultURL == "-" {
			resultURL = ""
		}
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
		docAggs = append(docAggs, map[string]any{"doc_name": title, "doc_id": documentID, "count": 1, "url": resultURL})
	}
	return chunks, docAggs
}

func formatBGPTPaper(paper map[string]any) string {
	lines := []string{
		"Title: " + bgptFirstField(paper, "title"),
		"Authors: " + bgptFirstField(paper, "authors"),
		"Journal: " + bgptFirstField(paper, "journal"),
		"Year: " + bgptFirstField(paper, "year"),
		"DOI: " + bgptFirstField(paper, "doi"),
		"Abstract: " + bgptFirstField(paper, "abstract"),
		"Methods: " + bgptFirstField(paper, "methods_and_experimental_techniques", "methods"),
		"Sample size / population: " + bgptFirstField(paper, "sample_size_and_population_characteristics", "sample_size_and_population", "sample_size"),
		"Results: " + bgptFirstField(paper, "results_and_conclusions", "results"),
		"Limitations: " + bgptFirstField(paper, "paper_limitations_and_biases", "limitations"),
		"Conflicts of interest: " + bgptFirstField(paper, "conflict_of_interest_statements", "conflict_of_interest"),
		"Data availability: " + bgptFirstField(paper, "data_availability_statements", "data_availability"),
		"Blind spots: " + bgptFirstField(paper, "study_blindspots", "blind_spots"),
		"How to falsify: " + bgptFirstField(paper, "how_to_falsify", "falsify"),
	}
	return strings.Join(lines, "\n")
}

func bgptFirstField(paper map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := paper[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	return "-"
}

func renderBGPTReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := fmt.Sprint(chunk["content"])
		block := strings.Join([]string{
			"\nID: " + fmt.Sprint(chunk["id"]),
			"├── Title: " + bgptNewlinePattern.ReplaceAllString(fmt.Sprint(chunk["document_name"]), " "),
			"├── URL: " + bgptNewlinePattern.ReplaceAllString(fmt.Sprint(chunk["url"]), " "),
			"└── Content:\n" + content,
		}, "\n")
		blockTokens := tokenizer.NumTokensFromString(block)
		if maxTokens > 0 && float64(usedTokens+blockTokens) > float64(maxTokens)*0.97 {
			break
		}
		usedTokens += blockTokens
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n")
}

func bgptHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateBGPTRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func bgptJSON(env bgptEnv) string {
	b, _ := json.Marshal(env)
	return string(b)
}

func bgptErrJSON(err error) string {
	return bgptJSON(bgptEnv{Error: err.Error()})
}
