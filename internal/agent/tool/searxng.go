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
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const (
	searxngToolName         = "searxng_search"
	searxngToolDescription  = "SearXNG is a privacy-focused metasearch engine that aggregates results from multiple search engines without tracking users. It provides comprehensive web search capabilities."
	defaultSearXNGTopN      = 10
	searxngRequestTimeout   = 10 * time.Second
	searxngPromptTokenLimit = 200000
)

var searxngDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var searxngNewlinePattern = regexp.MustCompile(`\n+`)

// searxngParams mirrors the SearXNG-specific Python parameters. Query and
// searxng_url are model inputs; searxng_url may also be node configuration,
// while top_n is Canvas-only configuration.
type searxngParams struct {
	Query      string `json:"query"`
	SearXNGURL string `json:"searxng_url"`
	TopN       int    `json:"top_n"`
}

type searxngEnvelope struct {
	Results []any  `json:"results"`
	Error   string `json:"_ERROR,omitempty"`
}

type searxngResolver func(string) (string, net.IP, error)

// SearXNGTool queries a caller-configured SearXNG instance.
type SearXNGTool struct {
	helper   *HTTPHelper
	defaults searxngParams
	resolve  searxngResolver
}

var _ ToolComponent = (*SearXNGTool)(nil)
var _ ReferenceBuilder = (*SearXNGTool)(nil)

func defaultSearXNGParams() searxngParams {
	return searxngParams{TopN: defaultSearXNGTopN}
}

// NewSearXNGTool returns a SearXNG tool using Python's defaults.
func NewSearXNGTool() *SearXNGTool {
	return newSearXNGToolWithDefaults(nil, defaultSearXNGParams())
}

func newSearXNGToolWithDefaults(helper *HTTPHelper, defaults searxngParams) *SearXNGTool {
	if helper == nil {
		helper = NewHTTPHelper()
	}
	if defaults.TopN == 0 {
		defaults.TopN = defaultSearXNGTopN
	}
	// SearXNG performs one request. Keep the shared helper from adding its own
	// retry loop so request count stays explicit and predictable.
	helper.retry.MaxAttempts = 1
	if helper.client != nil {
		helper.client.Timeout = searxngRequestTimeout
	}
	return &SearXNGTool{
		helper:   helper,
		defaults: defaults,
		resolve:  ResolveAndValidate,
	}
}

// Info exposes the Python model-call schema, not Canvas-only configuration.
func (s *SearXNGTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: searxngToolName,
		Desc: searxngToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with SearXNG. The keywords should be the most important words/terms(includes synonyms) from the original request.",
				Required: true,
			},
			"searxng_url": {
				Type:     schema.String,
				Desc:     "The base URL of your SearXNG instance (e.g., http://localhost:4000). This is required to connect to your SearXNG server.",
				Required: false,
			},
		}),
	}, nil
}

func buildSearXNGURL(baseURL, query string) string {
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("categories", "general")
	params.Set("language", "auto")
	params.Set("safesearch", "1")
	params.Set("pageno", "1")
	return baseURL + "/search?" + params.Encode()
}

func mergeSearXNGDefaults(defaults, params searxngParams) searxngParams {
	if defaults.SearXNGURL != "" {
		params.SearXNGURL = defaults.SearXNGURL
	}
	params.TopN = defaults.TopN
	return params
}

// InvokableRun performs the same request and result slicing as Python
// SearXNG._invoke. Empty try-run inputs return an empty result without I/O.
func (s *SearXNGTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var params searxngParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		err = fmt.Errorf("searxng: parse arguments: %w", err)
		return searxngErrJSON(err), err
	}
	params = mergeSearXNGDefaults(s.defaults, params)
	if strings.TrimSpace(params.Query) == "" {
		return searxngJSON(searxngEnvelope{Results: []any{}}), nil
	}
	params.SearXNGURL = strings.TrimSpace(params.SearXNGURL)
	if params.SearXNGURL == "" {
		return searxngJSON(searxngEnvelope{Results: []any{}}), nil
	}

	host, pinnedIP, err := s.resolve(params.SearXNGURL)
	if err != nil {
		return searxngErrJSON(err), err
	}

	endpoint := buildSearXNGURL(params.SearXNGURL, params.Query)
	if ctx.Err() != nil {
		return searxngJSON(searxngEnvelope{Results: []any{}}), nil
	}
	results, requestErr := s.fetch(ctx, endpoint, host, pinnedIP)
	if requestErr != nil {
		if ctx.Err() != nil {
			return searxngJSON(searxngEnvelope{Results: []any{}}), nil
		}
		return searxngErrJSON(requestErr), requestErr
	}
	if len(results) > params.TopN {
		results = results[:params.TopN]
	}
	return searxngJSON(searxngEnvelope{Results: results}), nil
}

func (s *SearXNGTool) fetch(ctx context.Context, endpoint, host string, pinnedIP net.IP) ([]any, error) {
	requestCtx, cancel := context.WithTimeout(ctx, searxngRequestTimeout)
	defer cancel()

	resp, err := s.helper.DoPinned(requestCtx, http.MethodGet, endpoint, "", "", nil, host, pinnedIP)
	if err != nil {
		return nil, fmt.Errorf("Network error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Network error: SearXNG returned %d", resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("Invalid response from SearXNG: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("Invalid response from SearXNG")
	}
	rawResults, ok := data["results"]
	if !ok {
		return []any{}, nil
	}
	results, ok := rawResults.([]any)
	if !ok {
		return nil, fmt.Errorf("Invalid results format from SearXNG")
	}
	for _, result := range results {
		if _, ok := result.(map[string]any); !ok {
			return nil, fmt.Errorf("Invalid results format from SearXNG")
		}
	}
	return results, nil
}

func parseSearXNGTopN(value any) (int, bool) {
	if text, ok := value.(string); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(text))
		return parsed, err == nil
	}
	return strictInt(value)
}

// ComponentSpec returns the Python-compatible SearXNG Canvas surface.
func (s *SearXNGTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":       "The search keywords to execute with SearXNG.",
			"searxng_url": "The base URL of the SearXNG instance.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered SearXNG references for downstream LLM prompts.",
			"json":               "Raw SearXNG result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{
				"name": "Query",
				"type": "line",
			},
			"searxng_url": map[string]any{
				"name":        "SearXNG URL",
				"type":        "line",
				"placeholder": "http://localhost:4000",
			},
		},
	}
}

// BuildReferences builds the same references as Python ToolBase._retrieve_chunks.
func (s *SearXNGTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildSearXNGReferences(envelope)
}

// BuildComponentOutputs converts SearXNG's complete tool envelope into its
// public Canvas outputs.
func (s *SearXNGTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildSearXNGReferences(envelope)
	return map[string]any{
		"formalized_content": renderSearXNGReferences(chunks, searxngPromptTokenLimit),
		"json":               results,
	}
}

func buildSearXNGReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content, _ := item["content"].(string)
		if content == "" {
			continue
		}
		content = searxngDataImagePattern.ReplaceAllString(content, "")
		runes := []rune(content)
		if len(runes) > 10000 {
			content = string(runes[:10000])
		}
		if content == "" {
			continue
		}

		documentID := strconv.Itoa(hashSearXNGString(content, 100000000))
		displayID := strconv.Itoa(hashSearXNGString(documentID, 500))
		title := searxngText(item["title"])
		resultURL := searxngText(item["url"])
		chunks = append(chunks, map[string]any{
			"id":                displayID,
			"chunk_id":          documentID,
			"content":           content,
			"doc_id":            documentID,
			"docnm_kwd":         title,
			"document_id":       documentID,
			"document_name":     title,
			"dataset_id":        nil,
			"image_id":          nil,
			"positions":         nil,
			"url":               resultURL,
			"similarity":        1,
			"vector_similarity": nil,
			"term_similarity":   nil,
			"row_id":            nil,
			"doc_type":          nil,
			"document_metadata": nil,
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

func renderSearXNGReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := searxngText(chunk["content"])
		if content == "" {
			continue
		}
		var block strings.Builder
		fmt.Fprintf(&block, "\nID: %s", searxngText(chunk["id"]))
		if title := searxngPromptField(chunk["document_name"]); title != "" {
			fmt.Fprintf(&block, "\n├── Title: %s", title)
		}
		if resultURL := searxngPromptField(chunk["url"]); resultURL != "" {
			fmt.Fprintf(&block, "\n├── URL: %s", resultURL)
		}
		block.WriteString("\n└── Content:\n")
		block.WriteString(content)
		completeBlock := block.String()
		blockTokens := tokenizer.NumTokensFromString(completeBlock)
		if maxTokens > 0 && float64(usedTokens+blockTokens) > float64(maxTokens)*0.97 {
			break
		}
		usedTokens += blockTokens
		blocks = append(blocks, completeBlock)
	}
	return strings.Join(blocks, "\n")
}

func searxngText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func searxngPromptField(value any) string {
	return searxngNewlinePattern.ReplaceAllString(searxngText(value), " ")
}

func hashSearXNGString(value string, modulus int) int {
	digest := sha1.Sum([]byte(value))
	result := 0
	for _, part := range digest {
		result = (result*256 + int(part)) % modulus
	}
	return result
}

func searxngJSON(env searxngEnvelope) string {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"searxng: marshal result: %s"}`, err)
	}
	return string(data)
}

func searxngErrJSON(err error) string {
	if err == nil {
		err = fmt.Errorf("SearXNG error")
	}
	return searxngJSON(searxngEnvelope{Results: []any{}, Error: err.Error()})
}
