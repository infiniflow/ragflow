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
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const search1APICrawlToolName = "search1api_crawl"

// search1APICrawlEndpoint is the Search1API crawl URL. Exposed as a package
// var so tests can substitute it through rewriteHostTransport.
var search1APICrawlEndpoint = "https://api.search1api.com/crawl"

type search1APICrawlArgs struct {
	URL string `json:"url"`
}

type search1APICrawlRequestBody struct {
	URL string `json:"url"`
}

// search1APICrawlResponse keeps the upstream page object unchanged: title,
// link, content and any metadata Search1API returns.
type search1APICrawlResponse struct {
	Results map[string]any `json:"results"`
}

type search1APICrawlEnvelope struct {
	Results map[string]any `json:"results,omitempty"`
	Error   string         `json:"_ERROR,omitempty"`
}

// Search1APICrawlTool reads one web page through the Search1API crawl
// endpoint and returns its content. The API key is node configuration only
// and never model-visible.
type Search1APICrawlTool struct {
	helper     *HTTPHelper
	apiKey     string
	defaultURL string
}

var _ ToolComponent = (*Search1APICrawlTool)(nil)

// NewSearch1APICrawlTool returns a Search1APICrawlTool with no API key and
// the default HTTPHelper.
func NewSearch1APICrawlTool() *Search1APICrawlTool {
	return newSearch1APICrawlTool(nil, "", "")
}

// NewSearch1APICrawlToolWith returns a Search1APICrawlTool that uses the
// provided HTTPHelper and API key.
func NewSearch1APICrawlToolWith(h *HTTPHelper, apiKey string) *Search1APICrawlTool {
	return newSearch1APICrawlTool(h, apiKey, "")
}

func newSearch1APICrawlTool(h *HTTPHelper, apiKey, defaultURL string) *Search1APICrawlTool {
	if h == nil {
		h = NewHTTPHelper()
		// Reading a slow page can take longer than the 30s default.
		h.client.Timeout = 60 * time.Second
	}
	return &Search1APICrawlTool{helper: h, apiKey: strings.TrimSpace(apiKey), defaultURL: defaultURL}
}

// Info returns the tool's metadata for the chat model.
func (c *Search1APICrawlTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: search1APICrawlToolName,
		Desc: "Read one web page with Search1API and return its title and full content. Use it to read a page found by a search, or a URL the user gave.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The absolute HTTP or HTTPS URL of the page to read.",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun crawls the page. Recoverable errors are returned as a JSON
// envelope with an _ERROR field and a nil Go error, matching
// Search1APITool.
func (c *Search1APICrawlTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args search1APICrawlArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: parse arguments: %w", err)), nil
	}
	pageURL := strings.TrimSpace(args.URL)
	if pageURL == "" {
		pageURL = strings.TrimSpace(c.defaultURL)
	}
	if pageURL == "" {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: url is required")), nil
	}
	parsed, err := url.ParseRequestURI(pageURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: url %q must be an absolute HTTP or HTTPS URL", pageURL)), nil
	}
	if c.apiKey == "" {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: api_key is required")), nil
	}

	body, _ := json.Marshal(search1APICrawlRequestBody{URL: pageURL})
	resp, err := c.helper.Do(ctx,
		http.MethodPost, search1APICrawlEndpoint, string(body), "application/json",
		map[string]string{
			"Authorization": "Bearer " + c.apiKey,
			"Accept":        "application/json",
			// Identifies RAGFlow to Search1API.
			"User-Agent": "RAGFlow search1api-integration/infiniflow-ragflow",
		},
	)
	if err != nil {
		return search1APICrawlErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: upstream returned %d", resp.StatusCode)), nil
	}

	var raw search1APICrawlResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: decode response: %w", err)), nil
	}
	if raw.Results == nil {
		return search1APICrawlErrJSON(fmt.Errorf("search1api_crawl: response has no results")), nil
	}
	return search1APICrawlJSON(search1APICrawlEnvelope{Results: raw.Results}), nil
}

// ComponentSpec returns the Search1APICrawl Canvas surface.
func (c *Search1APICrawlTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"url": "The absolute HTTP or HTTPS URL of the page to read.",
		},
		Outputs: map[string]string{
			"json": "The crawled page: title, link, content and metadata.",
		},
		InputForm: map[string]any{
			"url": map[string]any{"name": "URL", "type": "line"},
		},
	}
}

func (c *Search1APICrawlTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results, _ := envelope["results"].(map[string]any)
	if results == nil {
		results = map[string]any{}
	}
	return map[string]any{"json": results}
}

func search1APICrawlJSON(env search1APICrawlEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"search1api_crawl: marshal result: %s"}`, err)
	}
	return string(b)
}

func search1APICrawlErrJSON(err error) string {
	return search1APICrawlJSON(search1APICrawlEnvelope{Error: err.Error()})
}
