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
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	searxngToolName        = "searxng_search"
	searxngToolDescription = "SearXNG is a privacy-focused metasearch engine that aggregates results from multiple search engines without tracking users. It provides comprehensive web search capabilities."
	defaultSearXNGTopN     = 10
	searxngRequestTimeout  = 10 * time.Second
)

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
