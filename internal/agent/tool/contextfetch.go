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
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const contextFetchToolName = "contextfetch_search"

const contextFetchToolDescription = `ContextFetch is a web search API for AI agents. It returns a title, URL and the most relevant passage of each result page. When searching:
   - Use a focused query of the most important terms (and synonyms).`

// contextFetchMaxResults is the most results one ContextFetch search returns.
const contextFetchMaxResults = 20

// contextFetchEndpoint is ContextFetch's Tavily-compatible search URL. The
// request and response follow Tavily's /search, so results are rendered with
// the Tavily reference helpers. Exposed as a package var so tests can
// substitute it through rewriteHostTransport.
var contextFetchEndpoint = "https://api.contextfetch.dev/tavily/search"

// contextFetchArgs is the JSON shape the model sends into InvokableRun.
// max_results is decoded loosely: a missing, blank or non-numeric value means
// "use the node's Top N" rather than failing the run.
type contextFetchArgs struct {
	Query      string `json:"query"`
	MaxResults any    `json:"max_results"`
}

type contextFetchRequestBody struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type contextFetchResponse struct {
	Results []map[string]any `json:"results"`
}

type contextFetchEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// ContextFetchTool is the ContextFetch web search tool. The API key is node
// configuration only and never model-visible.
type ContextFetchTool struct {
	helper *HTTPHelper
	apiKey string
	topN   int
}

var _ ToolComponent = (*ContextFetchTool)(nil)
var _ ReferenceBuilder = (*ContextFetchTool)(nil)

// NewContextFetchToolWith returns a ContextFetchTool that uses the provided
// HTTPHelper and API key. Useful for tests that want to inject a transport.
func NewContextFetchToolWith(h *HTTPHelper, apiKey string) *ContextFetchTool {
	return newContextFetchTool(h, apiKey, 0)
}

func newContextFetchTool(h *HTTPHelper, apiKey string, topN int) *ContextFetchTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if topN <= 0 {
		topN = 10
	}
	return &ContextFetchTool{helper: h, apiKey: strings.TrimSpace(apiKey), topN: topN}
}

// Info returns the tool's metadata for the chat model.
func (c *ContextFetchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: contextFetchToolName,
		Desc: contextFetchToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with ContextFetch. The most important words/terms (and synonyms) from the original request.",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Number of results to return, 1 to 20. Leave it out to use the Top N set on the node.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun performs the ContextFetch search. Recoverable errors are
// returned as a JSON envelope with an _ERROR field and a nil Go error, so the
// ReAct agent feeds them back to the model, matching TavilyTool.
func (c *ContextFetchTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args contextFetchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return contextFetchErrJSON(fmt.Errorf("contextfetch: parse arguments: %w", err)), nil
	}
	if strings.TrimSpace(args.Query) == "" {
		return contextFetchJSON(contextFetchEnvelope{Results: []map[string]any{}}), nil
	}
	if c.apiKey == "" {
		return contextFetchErrJSON(fmt.Errorf("contextfetch: api_key is required")), nil
	}
	maxResults := contextFetchMaxResultsArg(args.MaxResults, c.topN)

	body, _ := json.Marshal(contextFetchRequestBody{Query: args.Query, MaxResults: maxResults})
	resp, err := c.helper.Do(ctx,
		http.MethodPost, contextFetchEndpoint, string(body), "application/json",
		map[string]string{"Authorization": "Bearer " + c.apiKey},
	)
	if err != nil {
		return contextFetchErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contextFetchErrJSON(fmt.Errorf("contextfetch: upstream returned %d", resp.StatusCode)), nil
	}

	var raw contextFetchResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return contextFetchErrJSON(fmt.Errorf("contextfetch: decode response: %w", err)), nil
	}
	results := raw.Results
	if results == nil {
		results = []map[string]any{}
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return contextFetchJSON(contextFetchEnvelope{Results: results}), nil
}

// contextFetchMaxResultsArg returns the model's result count if it gave a
// usable one, else the node's Top N, clamped to [1, 20].
func contextFetchMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), contextFetchMaxResults)
}

// ComponentSpec returns the ContextFetchSearch Canvas surface.
func (c *ContextFetchTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query": "The search keywords to execute with ContextFetch.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered ContextFetch references for downstream prompts.",
			"json":               "Raw ContextFetch result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
		},
	}
}

func (c *ContextFetchTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildTavilyReferences(envelope)
}

func (c *ContextFetchTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	chunks, _ := buildTavilyReferences(envelope)
	return map[string]any{
		"formalized_content": renderTavilyReferences(chunks, tavilyPromptMaxTokens),
		"json":               envelopeSlice(envelope, "results"),
	}
}

func contextFetchJSON(env contextFetchEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"contextfetch: marshal result: %s"}`, err)
	}
	return string(b)
}

func contextFetchErrJSON(err error) string {
	return contextFetchJSON(contextFetchEnvelope{Error: err.Error()})
}
