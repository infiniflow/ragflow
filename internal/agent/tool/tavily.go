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
	"os"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const tavilyToolName = "tavily"

const tavilyToolDescription = "Search the web via the Tavily API. Returns a list of {url, title, content} results."

// tavilyParams is the JSON shape the model sends into InvokableRun. The
// api_key may be omitted when the env var TAVILY_API_KEY is set; the tool
// resolves it from the environment in that case.
type tavilyParams struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	MaxResults  int    `json:"max_results"`
	SearchDepth string `json:"search_depth"`
}

// tavilyRequestBody is the JSON body POSTed to the Tavily /search endpoint.
// The struct matches the upstream API (https://docs.tavily.com).
type tavilyRequestBody struct {
	Query       string `json:"query"`
	MaxResults  int    `json:"max_results"`
	SearchDepth string `json:"search_depth"`
}

// tavilyResult mirrors one element of the upstream `results` array. We
// return these verbatim to the model.
type tavilyResult struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// tavilyResponse is the envelope returned by Tavily. We only model the
// fields we care about; the upstream API has more, but they are ignored.
type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

// tavilyEnvelope is the shape the model actually sees, identical to the
// Python tool's output convention.
type tavilyEnvelope struct {
	Results []tavilyResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

// TavilyTool is the Tavily search
// tool. It POSTs a search request
// to https://api.tavily.com/search using the shared HTTPHelper and returns
// the upstream `results` array as JSON.
type TavilyTool struct {
	helper *HTTPHelper
	envKey func() string
}

// NewTavilyTool returns a TavilyTool using the default HTTPHelper and
// the TAVILY_API_KEY env var for credential resolution.
func NewTavilyTool() *TavilyTool {
	return NewTavilyToolWith(NewHTTPHelper())
}

// NewTavilyToolWith returns a TavilyTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewTavilyToolWith(h *HTTPHelper) *TavilyTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &TavilyTool{helper: h, envKey: defaultTavilyEnvKey}
}

// NewTavilyToolWithEnvKey returns a TavilyTool with a custom env-key
// resolver. Useful for tests that want to inject a fake credential
// without mutating process state.
func NewTavilyToolWithEnvKey(h *HTTPHelper, envKey func() string) *TavilyTool {
	if envKey == nil {
		envKey = defaultTavilyEnvKey
	}
	return &TavilyTool{helper: h, envKey: envKey}
}

// defaultTavilyEnvKey is the production env-key resolver. Pulled out
// as a named function (not a var) so tests cannot accidentally
// mutate it via package-var assignment.
func defaultTavilyEnvKey() string { return os.Getenv("TAVILY_API_KEY") }

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
			"api_key": {
				Type:     schema.String,
				Desc:     "Tavily API key. Falls back to TAVILY_API_KEY env var.",
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5.",
				Required: false,
			},
			"search_depth": {
				Type:     schema.String,
				Desc:     `Tavily search depth: "basic" (default) or "advanced".`,
				Required: false,
			},
		}),
	}, nil
}

// tavilyEndpoint is the Tavily /search URL. Exposed as a package var so
// tests can substitute a httptest.Server URL. Tests must serialize
// access with tavilyEndpointMu if running in parallel.
var tavilyEndpoint = "https://api.tavily.com/search"

// InvokableRun performs the Tavily search. The api_key may come from the
// argument or the TAVILY_API_KEY env var.
func (t *TavilyTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p tavilyParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return tavilyErrJSON(fmt.Errorf("tavily: parse arguments: %w", err)),
			fmt.Errorf("tavily: parse arguments: %w", err)
	}
	if p.Query == "" {
		return tavilyErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("tavily: query is required")
	}
	if p.MaxResults <= 0 {
		p.MaxResults = 5
	}
	if p.SearchDepth == "" {
		p.SearchDepth = "basic"
	}

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = t.envKey()
	}
	if apiKey == "" {
		return tavilyErrJSON(fmt.Errorf("tavily: api_key is required (or set TAVILY_API_KEY)")),
			fmt.Errorf("tavily: api_key is required (or set TAVILY_API_KEY)")
	}

	body, _ := json.Marshal(tavilyRequestBody{
		Query:       p.Query,
		MaxResults:  p.MaxResults,
		SearchDepth: p.SearchDepth,
	})

	resp, err := t.helper.Do(ctx,
		http.MethodPost, tavilyEndpoint, string(body), "application/json",
		map[string]string{"Authorization": "Bearer " + apiKey},
	)
	if err != nil {
		return tavilyErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tavilyErrJSON(fmt.Errorf("tavily: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("tavily: upstream returned %d", resp.StatusCode)
	}

	var raw tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return tavilyErrJSON(fmt.Errorf("tavily: decode response: %w", err)),
			fmt.Errorf("tavily: decode response: %w", err)
	}
	return tavilyJSON(tavilyEnvelope{Results: raw.Results}), nil
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
