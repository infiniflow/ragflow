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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/service/youcom"
	"ragflow/internal/tokenizer"
)

const youComToolName = "youcom_search"

const youComToolDescription = `You.com is a web search API. It returns the extracted passages of each result page, so hits carry usable context without a separate fetch. When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Optionally restrict results by how recently they were published.`

// youComMaxResults caps how many results one search returns.
const youComMaxResults = 20

const youComDefaultTopN = 10

const youComPromptMaxTokens = 200000

// youComMaxResponseBytes caps how much of You.com's response body is read: a
// handful of hits of JSON is a few KiB, so 4 MiB is far above any legitimate
// answer while still bounding what a misbehaving upstream can make us allocate.
const youComMaxResponseBytes = 4 << 20

var youComFreshnessValues = []string{"any", "day", "week", "month", "year"}

// youComArgs is the JSON shape the model sends into InvokableRun. max_results
// is decoded loosely: a missing, blank or non-numeric value means "use the
// node's Top N" rather than failing the run.
type youComArgs struct {
	Query      string `json:"query"`
	Freshness  string `json:"freshness"`
	MaxResults any    `json:"max_results"`
}

// youComDefaults holds the node-level configuration.
type youComDefaults struct {
	Freshness string
	TopN      int
}

type youComEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// YouComTool is the You.com web search tool. It queries
// https://api.you.com/v1/search (or the keyless endpoint when no API key is
// configured) and returns the merged web + news results. The API key is node
// configuration only and never model-visible.
type YouComTool struct {
	helper   *HTTPHelper
	apiKey   string
	defaults youComDefaults
}

var _ ToolComponent = (*YouComTool)(nil)
var _ ReferenceBuilder = (*YouComTool)(nil)

// NewYouComTool returns a YouComTool with no API key and the default HTTPHelper.
func NewYouComTool() *YouComTool {
	return newYouComTool(nil, "", youComDefaults{})
}

// NewYouComToolWith returns a YouComTool that uses the provided HTTPHelper and
// API key. Useful for tests that want to inject a custom transport.
func NewYouComToolWith(h *HTTPHelper, apiKey string) *YouComTool {
	return newYouComTool(h, apiKey, youComDefaults{})
}

func newYouComTool(h *HTTPHelper, apiKey string, defaults youComDefaults) *YouComTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.Freshness == "" {
		defaults.Freshness = "any"
	}
	if defaults.TopN <= 0 {
		defaults.TopN = youComDefaultTopN
	}
	return &YouComTool{helper: h, apiKey: strings.TrimSpace(apiKey), defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (t *YouComTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: youComToolName,
		Desc: youComToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with You.com. The most important words/terms (and synonyms) from the original request.",
				Required: true,
			},
			"freshness": {
				Type:     schema.String,
				Desc:     `Restrict results by recency: "day", "week", "month", "year", or "any" (default) for no limit.`,
				Enum:     youComFreshnessValues,
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

// InvokableRun performs the You.com search.
//
// Recoverable errors (bad arguments, API or network failures) are returned as a
// JSON envelope with an _ERROR field and a nil Go error, so the ReAct agent
// feeds them back to the model instead of failing the run, matching TavilyTool.
func (t *YouComTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args youComArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return youComErrJSON(fmt.Errorf("youcom: parse arguments: %w", err)), nil
	}
	if strings.TrimSpace(args.Query) == "" {
		return youComJSON(youComEnvelope{Results: []map[string]any{}}), nil
	}

	freshness, err := youComEnumArg("freshness", args.Freshness, youComFreshnessValues, t.defaults.Freshness)
	if err != nil {
		return youComErrJSON(err), nil
	}
	maxResults := youComMaxResultsArg(args.MaxResults, t.defaults.TopN)

	hits, err := youcom.Search(ctx, t.fetch, youcom.Request{
		Endpoint:  youcom.EndpointFor(t.apiKey),
		APIKey:    t.apiKey,
		Query:     args.Query,
		Freshness: freshness,
		Count:     maxResults,
	})
	if err != nil {
		return youComErrJSON(err), nil
	}
	// `count` applies per response section, so the merged web + news list can
	// exceed it; trim back to what the model (or the node's Top N) asked for.
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}

	results := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		results = append(results, map[string]any{
			"title":   hit.Title,
			"url":     hit.URL,
			"content": hit.Content,
		})
	}
	return youComJSON(youComEnvelope{Results: results}), nil
}

// fetch performs the HTTP GET for the youcom leaf package, applying the status
// check and response-body cap that the leaf leaves to its caller.
func (t *YouComTool) fetch(ctx context.Context, endpoint string, headers map[string]string) ([]byte, error) {
	resp, err := t.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, youComMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(body) > youComMaxResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", youComMaxResponseBytes)
	}
	return body, nil
}

// youComEnumArg normalizes a model-supplied enum value. Blank means the
// default; anything else must be a known value, because it is forwarded to
// You.com as-is.
func youComEnumArg(name, value string, allowed []string, fallback string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return fallback, nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", fmt.Errorf("youcom: %s %q is not supported, it should be one of %v", name, value, allowed)
}

// youComMaxResultsArg returns the model's result count if it gave a usable one,
// else the node's Top N, clamped to [1, youComMaxResults].
func youComMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), youComMaxResults)
}

// ComponentSpec returns the YouComSearch Canvas surface.
func (t *YouComTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":     "The search keywords to execute with You.com.",
			"freshness": `Recency limit: "day", "week", "month", "year" or "any".`,
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered You.com references for downstream prompts.",
			"json":               "Raw You.com result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
			"freshness": map[string]any{
				"name":    "Freshness",
				"type":    "options",
				"value":   "any",
				"options": youComFreshnessValues,
			},
		},
	}
}

func (t *YouComTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildYouComReferences(envelope)
}

func (t *YouComTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildYouComReferences(envelope)
	return map[string]any{
		"formalized_content": renderYouComReferences(chunks, youComPromptMaxTokens),
		"json":               results,
	}
}

// buildYouComReferences turns each usable result into the chunk/docAgg shape
// the citation path expects. A result with no text is dropped rather than
// stored as a blank chunk.
func buildYouComReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := truncateYouComRunes(strings.Join(strings.Fields(youComText(result["content"])), " "), 10000)
		if content == "" {
			continue
		}
		title := youComText(result["title"])
		resultURL := youComText(result["url"])
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

func renderYouComReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := youComText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + youComText(chunk["id"]),
			"├── Title: " + strings.Join(strings.Fields(youComText(chunk["document_name"])), " "),
			"├── URL: " + youComText(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func youComText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func truncateYouComRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func youComJSON(env youComEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"youcom: marshal result: %s"}`, err)
	}
	return string(b)
}

func youComErrJSON(err error) string {
	return youComJSON(youComEnvelope{Error: err.Error()})
}
