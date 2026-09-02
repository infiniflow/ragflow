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
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	queritContentsToolName = "querit_contents"
	queritContentsEndpoint = "https://api.querit.ai/v1/contents"
)

type queritContentsParams struct {
	APIKey       string `json:"api_key"`
	URLs         any    `json:"urls"`
	Format       string `json:"format"`
	CrawlTimeout int    `json:"crawl_timeout"`
	ExtrasMeta   bool   `json:"extras_meta"`
}

type queritContentsRequest struct {
	URLs         []string `json:"urls"`
	Format       string   `json:"format"`
	CrawlTimeout int      `json:"crawlTimeout"`
	ExtrasMeta   bool     `json:"extrasMeta"`
}

// QueritContentsTool crawls public web pages through the Querit Contents API.
type QueritContentsTool struct {
	helper    *HTTPHelper
	envKey    func() string
	defaults  queritContentsParams
	retryWait func(context.Context, int) bool
}

var _ ToolComponent = (*QueritContentsTool)(nil)

func NewQueritContentsTool() *QueritContentsTool {
	return newQueritContentsTool(nil, nil, queritContentsParams{}, nil)
}

func NewQueritContentsToolWith(helper *HTTPHelper) *QueritContentsTool {
	return newQueritContentsTool(helper, nil, queritContentsParams{}, nil)
}

func NewQueritContentsToolWithEnvKey(helper *HTTPHelper, envKey func() string) *QueritContentsTool {
	return newQueritContentsTool(helper, envKey, queritContentsParams{}, nil)
}

func newQueritContentsTool(
	helper *HTTPHelper,
	envKey func() string,
	defaults queritContentsParams,
	retryWait func(context.Context, int) bool,
) *QueritContentsTool {
	if helper == nil {
		helper = NewHTTPHelper()
		helper.client.Timeout = 65 * time.Second
	}
	if envKey == nil {
		envKey = defaultQueritEnvKey
	}
	if defaults.Format == "" {
		defaults.Format = "markdown"
	}
	if defaults.CrawlTimeout == 0 {
		defaults.CrawlTimeout = 10
	}
	if retryWait == nil {
		retryWait = waitForQueritRetry
	}
	return &QueritContentsTool{helper: helper, envKey: envKey, defaults: defaults, retryWait: retryWait}
}

func (q *QueritContentsTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: queritContentsToolName,
		Desc: "Crawl one or more web pages with Querit and return their contents.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"urls": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "One to ten absolute HTTP or HTTPS URLs to crawl.",
				Required: true,
			},
			"format": {
				Type:     schema.String,
				Desc:     `Content format: "text", "markdown" (default), or "html".`,
				Required: false,
			},
			"crawl_timeout": {
				Type:     schema.Integer,
				Desc:     "Per-page crawl timeout in seconds. Defaults to 10 and must be between 1 and 60.",
				Required: false,
			},
			"extras_meta": {
				Type:     schema.Boolean,
				Desc:     "Whether to include page metadata in each result.",
				Required: false,
			},
		}),
	}, nil
}

func (q *QueritContentsTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	provided := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(argsJSON), &provided); err != nil {
		return queritContentsErrorJSON(fmt.Errorf("parse arguments: %w", err)), nil
	}
	var runtimeParams queritContentsParams
	if err := json.Unmarshal([]byte(argsJSON), &runtimeParams); err != nil {
		return queritContentsErrorJSON(fmt.Errorf("parse arguments: %w", err)), nil
	}
	params := mergeQueritContentsParams(q.defaults, runtimeParams, provided)
	urls := normalizeQueritContentURLs(params.URLs)
	if err := validateQueritContentsParams(params, urls); err != nil {
		return queritContentsErrorJSON(err, params.APIKey), nil
	}

	apiKey := strings.TrimSpace(params.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(q.envKey())
	}
	if apiKey == "" {
		return queritContentsErrorJSON(fmt.Errorf("api_key is required (or set QUERIT_API_KEY)")), nil
	}

	body, err := json.Marshal(queritContentsRequest{
		URLs:         urls,
		Format:       params.Format,
		CrawlTimeout: params.CrawlTimeout,
		ExtrasMeta:   params.ExtrasMeta,
	})
	if err != nil {
		return queritContentsErrorJSON(fmt.Errorf("encode request: %w", err), apiKey), nil
	}
	raw, requestErr := doQueritRequest(ctx, q.helper, q.retryWait, queritContentsEndpoint, body, apiKey)
	if requestErr != nil {
		return queritContentsErrorJSON(requestErr, apiKey), nil
	}
	if _, err := decodeQueritContentsResponse(raw); err != nil {
		return queritContentsErrorJSON(err, apiKey), nil
	}
	return string(raw), nil
}

func mergeQueritContentsParams(
	defaults queritContentsParams,
	runtimeParams queritContentsParams,
	provided map[string]json.RawMessage,
) queritContentsParams {
	merged := defaults
	if _, ok := provided["urls"]; ok {
		merged.URLs = runtimeParams.URLs
	}
	if _, ok := provided["format"]; ok {
		merged.Format = runtimeParams.Format
	}
	if _, ok := provided["crawl_timeout"]; ok {
		merged.CrawlTimeout = runtimeParams.CrawlTimeout
	}
	if _, ok := provided["extras_meta"]; ok {
		merged.ExtrasMeta = runtimeParams.ExtrasMeta
	}
	return merged
}

func validateQueritContentsParams(params queritContentsParams, urls []string) error {
	if len(urls) < 1 || len(urls) > 10 {
		return fmt.Errorf("urls must contain between 1 and 10 values")
	}
	for _, rawURL := range urls {
		parsed, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("urls must be absolute HTTP or HTTPS URLs")
		}
	}
	if params.Format != "text" && params.Format != "markdown" && params.Format != "html" {
		return fmt.Errorf("format must be text, markdown, or html")
	}
	if params.CrawlTimeout < 1 || params.CrawlTimeout > 60 {
		return fmt.Errorf("crawl_timeout must be between 1 and 60")
	}
	return nil
}

func normalizeQueritContentURLs(raw any) []string {
	switch value := raw.(type) {
	case string:
		return compactStrings(strings.Split(value, ","))
	case []string:
		return compactStrings(value)
	case []any:
		urls := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			urls = append(urls, text)
		}
		return compactStrings(urls)
	default:
		return nil
	}
}

func decodeQueritContentsResponse(raw []byte) (map[string]any, error) {
	response, err := decodeQueritJSONObject(raw)
	if err != nil {
		return nil, err
	}
	for _, field := range []string{"results", "statuses"} {
		if value, exists := response[field]; exists {
			if _, ok := value.([]any); !ok {
				return nil, fmt.Errorf("decode response: %s must be a JSON array", field)
			}
		}
	}
	return response, nil
}

func (q *QueritContentsTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		PreserveJSONNumbers: true,
		Inputs: map[string]string{
			"api_key":       "Querit API key. Uses QUERIT_API_KEY when empty.",
			"urls":          "One to ten absolute HTTP or HTTPS URLs to crawl.",
			"format":        `Content format: "text", "markdown", or "html".`,
			"crawl_timeout": "Per-page crawl timeout in seconds.",
			"extras_meta":   "Whether to include page metadata.",
		},
		Outputs: map[string]string{
			"json": "Complete raw Querit Contents JSON response.",
		},
		InputForm: map[string]any{
			"urls": map[string]any{"name": "URLs", "type": "line"},
		},
	}
}

func (q *QueritContentsTool) BuildComponentOutputs(response map[string]any) map[string]any {
	return map[string]any{"json": response}
}

func queritContentsErrorJSON(err error, apiKeys ...string) string {
	return queritToolErrorJSON(queritContentsToolName, err, apiKeys...)
}
