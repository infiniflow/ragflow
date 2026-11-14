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
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const specstreamToolName = "specstream_search"

const specstreamToolDescription = `SpecStream searches the live web and returns ranked results with titles, URLs, snippets, and optional page text passages. When searching:
   - Write a focused question or query in everyday words (1 to 2,000 characters).
   - Quoted phrases are searched as exact phrases.
   - Set passages to true when you want extracted text passages from the top matching pages without a separate fetch.`

const specstreamMaxResults = 20

const specstreamPromptMaxTokens = 200000

// specstreamEndpoint is the SpecStream Search API URL. Exposed as a package
// var so tests can substitute it through rewriteHostTransport.
var specstreamEndpoint = "https://api.specstream.ai/v1/web-search"

type specstreamArgs struct {
	Query      string `json:"query"`
	Passages   *bool  `json:"passages"`
	MaxResults any    `json:"max_results"`
}

type specstreamDefaults struct {
	Passages bool
	TopN     int
}

type specstreamEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// SpecStreamTool is the SpecStream Search API tool. It sends GET requests to
// https://api.specstream.ai/v1/web-search and returns the ranked sources list.
// The API key is node configuration only and never model-visible.
type SpecStreamTool struct {
	helper   *HTTPHelper
	apiKey   string
	defaults specstreamDefaults
}

var _ ToolComponent = (*SpecStreamTool)(nil)
var _ ReferenceBuilder = (*SpecStreamTool)(nil)

// NewSpecStreamTool returns a SpecStreamTool with no API key and the default
// HTTPHelper.
func NewSpecStreamTool() *SpecStreamTool {
	return newSpecStreamTool(nil, "", specstreamDefaults{})
}

// NewSpecStreamToolWith returns a SpecStreamTool that uses the provided
// HTTPHelper and API key.
func NewSpecStreamToolWith(h *HTTPHelper, apiKey string) *SpecStreamTool {
	return newSpecStreamTool(h, apiKey, specstreamDefaults{})
}

func newSpecStreamTool(h *HTTPHelper, apiKey string, defaults specstreamDefaults) *SpecStreamTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.TopN <= 0 {
		defaults.TopN = 10
	}
	return &SpecStreamTool{helper: h, apiKey: strings.TrimSpace(apiKey), defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (s *SpecStreamTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: specstreamToolName,
		Desc: specstreamToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search question or keywords to execute with SpecStream (1 to 2,000 characters).",
				Required: true,
			},
			"passages": {
				Type:     schema.Boolean,
				Desc:     "When true, include extracted text passages from the top matching pages.",
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

// InvokableRun performs the SpecStream search.
func (s *SpecStreamTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args specstreamArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return specstreamErrJSON(fmt.Errorf("specstream: parse arguments: %w", err)), nil
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return specstreamJSON(specstreamEnvelope{Results: []map[string]any{}}), nil
	}
	if utf8.RuneCountInString(query) > 2000 {
		return specstreamErrJSON(fmt.Errorf("specstream: query must be at most 2,000 characters")), nil
	}
	if s.apiKey == "" {
		return specstreamErrJSON(fmt.Errorf("specstream: api_key is required")), nil
	}

	includePassages := s.defaults.Passages
	if args.Passages != nil {
		includePassages = *args.Passages
	}
	maxResults := specstreamMaxResultsArg(args.MaxResults, s.defaults.TopN)

	params := url.Values{}
	params.Set("q", query)
	if includePassages {
		params.Set("passages", "true")
	}

	resp, err := s.helper.Do(ctx,
		http.MethodGet, specstreamEndpoint+"?"+params.Encode(), "", "",
		map[string]string{
			"Authorization": "Bearer " + s.apiKey,
			"Accept":        "application/json",
			"User-Agent":    "RAGFlow specstream-integration/infiniflow-ragflow",
		},
	)
	if err != nil {
		return specstreamErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return specstreamErrJSON(fmt.Errorf("specstream: upstream returned %d", resp.StatusCode)), nil
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return specstreamErrJSON(fmt.Errorf("specstream: decode response: %w", err)), nil
	}
	if dataObj, ok := payload["data"].(map[string]any); ok {
		payload = dataObj
	}

	passageByURL := map[string]string{}
	if rawPassages, ok := payload["passages"].([]any); ok {
		for _, item := range rawPassages {
			p, ok := item.(map[string]any)
			if !ok {
				continue
			}
			pURL := specstreamText(p["url"])
			pText := specstreamText(p["text"])
			if pURL == "" || pText == "" {
				continue
			}
			if prev := passageByURL[pURL]; prev != "" {
				passageByURL[pURL] = prev + "\n\n" + pText
			} else {
				passageByURL[pURL] = pText
			}
		}
	}

	rawSources, _ := payload["sources"].([]any)
	results := make([]map[string]any, 0, len(rawSources))
	for _, item := range rawSources {
		src, ok := item.(map[string]any)
		if !ok {
			continue
		}
		copied := make(map[string]any, len(src)+1)
		for k, v := range src {
			copied[k] = v
		}
		srcURL := specstreamText(src["url"])
		if passage := passageByURL[srcURL]; passage != "" && specstreamText(copied["content"]) == "" {
			copied["content"] = passage
		}
		results = append(results, copied)
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return specstreamJSON(specstreamEnvelope{Results: results}), nil
}

func specstreamMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), specstreamMaxResults)
}

// ComponentSpec returns the SpecStreamSearch Canvas surface.
func (s *SpecStreamTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query": "The search question or keywords to execute with SpecStream.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered SpecStream references for downstream prompts.",
			"json":               "Raw SpecStream result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
		},
	}
}

func (s *SpecStreamTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildSpecStreamReferences(envelope)
}

func (s *SpecStreamTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildSpecStreamReferences(envelope)
	return map[string]any{
		"formalized_content": renderSpecStreamReferences(chunks, specstreamPromptMaxTokens),
		"json":               results,
	}
}

func buildSpecStreamReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := specstreamText(result["content"])
		if content == "" {
			content = specstreamText(result["snippet"])
		}
		content = truncateSpecStreamRunes(strings.Join(strings.Fields(content), " "), 10000)
		if content == "" {
			continue
		}
		title := specstreamText(result["title"])
		resultURL := specstreamText(result["url"])
		if resultURL == "" {
			resultURL = specstreamText(result["link"])
		}
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

func renderSpecStreamReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := specstreamText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + specstreamText(chunk["id"]),
			"├── Title: " + strings.Join(strings.Fields(specstreamText(chunk["document_name"])), " "),
			"├── URL: " + specstreamText(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func specstreamText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func truncateSpecStreamRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func specstreamJSON(env specstreamEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"specstream: marshal result: %s"}`, err)
	}
	return string(b)
}

func specstreamErrJSON(err error) string {
	return specstreamJSON(specstreamEnvelope{Error: err.Error()})
}
