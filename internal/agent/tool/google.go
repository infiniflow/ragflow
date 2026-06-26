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

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const googleToolName = "google"

const googleToolDescription = "Search the web via Google Programmable Search (CSE). Returns items[].{title, link, snippet}."

// googleParams is the JSON shape the model sends into InvokableRun. The
// api_key (CX search-engine id) and cx are both required by the upstream
// API; api_key is the Programmable Search API key and cx is the
// search-engine ID.
type googleParams struct {
	APIKey     string `json:"api_key"`
	CX         string `json:"cx"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// googleResult mirrors one element of the upstream `items` array.
type googleResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

// googleResponse is the upstream Programmable Search envelope. We only
// model the fields we care about.
type googleResponse struct {
	Items []googleResult `json:"items"`
}

// googleEnvelope is what the model sees.
type googleEnvelope struct {
	Results []googleResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

// GoogleTool is the Google
// Programmable Search tool.
// It performs a GET against the CSE endpoint using the shared
// HTTPHelper.
type GoogleTool struct {
	helper *HTTPHelper
}

// NewGoogleTool returns a GoogleTool using the default HTTPHelper.
func NewGoogleTool() *GoogleTool {
	return NewGoogleToolWith(NewHTTPHelper())
}

// NewGoogleToolWith returns a GoogleTool that uses the provided
// HTTPHelper. Useful for tests.
func NewGoogleToolWith(h *HTTPHelper) *GoogleTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (g *GoogleTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: googleToolName,
		Desc: googleToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query",
				Required: true,
			},
			"api_key": {
				Type:     schema.String,
				Desc:     "Google Programmable Search API key.",
				Required: true,
			},
			"cx": {
				Type:     schema.String,
				Desc:     "Google Programmable Search engine ID (cx).",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5 (max 10 per request).",
				Required: false,
			},
		}),
	}, nil
}

// buildGoogleURL constructs the CSE query URL. The Programmable Search
// API caps `num` at 10; we clamp to that range to avoid upstream errors.
func buildGoogleURL(apiKey, cx, query string, maxResults int) string {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}
	q := url.Values{}
	q.Set("key", apiKey)
	q.Set("cx", cx)
	q.Set("q", query)
	q.Set("num", fmt.Sprintf("%d", maxResults))
	return "https://www.googleapis.com/customsearch/v1?" + q.Encode()
}

// InvokableRun performs the Google Programmable Search.
func (g *GoogleTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p googleParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return googleErrJSON(fmt.Errorf("google: parse arguments: %w", err)),
			fmt.Errorf("google: parse arguments: %w", err)
	}
	if p.Query == "" {
		return googleErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("google: query is required")
	}
	if p.APIKey == "" || p.CX == "" {
		return googleErrJSON(fmt.Errorf("google: api_key and cx are required")),
			fmt.Errorf("google: api_key and cx are required")
	}

	endpoint := buildGoogleURL(p.APIKey, p.CX, p.Query, p.MaxResults)
	resp, err := g.helper.Do(ctx, http.MethodGet, endpoint, "", "", nil)
	if err != nil {
		return googleErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleErrJSON(fmt.Errorf("google: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("google: upstream returned %d", resp.StatusCode)
	}

	var raw googleResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return googleErrJSON(fmt.Errorf("google: decode response: %w", err)),
			fmt.Errorf("google: decode response: %w", err)
	}
	return googleJSON(googleEnvelope{Results: raw.Items}), nil
}

func googleJSON(env googleEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"google: marshal result: %s"}`, err)
	}
	return string(b)
}

func googleErrJSON(err error) string {
	return googleJSON(googleEnvelope{Error: err.Error()})
}
