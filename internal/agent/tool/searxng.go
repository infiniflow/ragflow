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
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const searxngToolName = "searxng"

const searxngToolDescription = "Search a self-hosted SearXNG instance. Returns results[].{title, url, content}."

// searxngParams is the JSON shape the model sends into InvokableRun.
// base_url is the SearXNG root (no trailing slash); the default points
// at a local instance on port 8888.
type searxngParams struct {
	BaseURL    string `json:"base_url"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// searxngResult mirrors one element of the upstream `results` array.
type searxngResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// searxngResponse is the upstream SearXNG JSON envelope.
type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

// searxngEnvelope is the JSON shape the model sees.
type searxngEnvelope struct {
	Results []searxngResult `json:"results"`
	Error   string          `json:"_ERROR,omitempty"`
}

// SearXNGTool is the SearXNG
// meta-search tool. It calls
// a self-hosted SearXNG instance via the shared HTTPHelper.
type SearXNGTool struct {
	helper *HTTPHelper
}

// NewSearXNGTool returns a SearXNGTool using the default HTTPHelper.
func NewSearXNGTool() *SearXNGTool {
	return NewSearXNGToolWith(NewHTTPHelper())
}

// NewSearXNGToolWith returns a SearXNGTool that uses the provided
// HTTPHelper. Useful for tests.
func NewSearXNGToolWith(h *HTTPHelper) *SearXNGTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &SearXNGTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (s *SearXNGTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: searxngToolName,
		Desc: searxngToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query",
				Required: true,
			},
			"base_url": {
				Type:     schema.String,
				Desc:     `SearXNG base URL. Defaults to "http://localhost:8888".`,
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5.",
				Required: false,
			},
		}),
	}, nil
}

// buildSearXNGURL constructs the SearXNG /search URL. The base URL is
// normalized to drop trailing slashes. We use url.JoinPath-style
// construction (manual) so the function works on Go 1.19 as well.
func buildSearXNGURL(baseURL, query string, maxResults int) string {
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if maxResults <= 0 {
		maxResults = 5
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("language", "all")
	return baseURL + "/search?" + q.Encode()
}

// InvokableRun performs the SearXNG search.
func (s *SearXNGTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p searxngParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return searxngErrJSON(fmt.Errorf("searxng: parse arguments: %w", err)),
			fmt.Errorf("searxng: parse arguments: %w", err)
	}
	if p.Query == "" {
		return searxngErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("searxng: query is required")
	}

	endpoint := buildSearXNGURL(p.BaseURL, p.Query, p.MaxResults)
	resp, err := s.helper.Do(ctx, http.MethodGet, endpoint, "", "", nil)
	if err != nil {
		return searxngErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return searxngErrJSON(fmt.Errorf("searxng: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("searxng: upstream returned %d", resp.StatusCode)
	}

	var raw searxngResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return searxngErrJSON(fmt.Errorf("searxng: decode response: %w", err)),
			fmt.Errorf("searxng: decode response: %w", err)
	}
	return searxngJSON(searxngEnvelope{Results: raw.Results}), nil
}

func searxngJSON(env searxngEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"searxng: marshal result: %s"}`, err)
	}
	return string(b)
}

func searxngErrJSON(err error) string {
	return searxngJSON(searxngEnvelope{Error: err.Error()})
}
