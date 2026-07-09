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

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const googleToolName = "google"

const googleToolDescription = "Search the web via Google using SerpApi. Returns organic_results[].{title, link, snippet}."

// googleParams is the JSON shape the model or canvas sends into InvokableRun.
// The primary fields are api_key, q, start, num, country, and language;
// query and max_results are accepted as Agent-tool aliases.
type googleParams struct {
	APIKey     string `json:"api_key"`
	Q          string `json:"q"`
	Query      string `json:"query"`
	Start      int    `json:"start"`
	Num        int    `json:"num"`
	MaxResults int    `json:"max_results"`
	Country    string `json:"country"`
	Language   string `json:"language"`
}

type googleResult map[string]any

type googleResponse struct {
	OrganicResults []googleResult `json:"organic_results"`
}

type googleEnvelope struct {
	Results []googleResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

type GoogleTool struct {
	helper   *HTTPHelper
	defaults googleParams
}

func NewGoogleTool() *GoogleTool {
	return NewGoogleToolWith(NewHTTPHelper())
}

func NewGoogleToolWith(h *HTTPHelper) *GoogleTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleTool{helper: h}
}

func NewGoogleToolWithDefaults(h *HTTPHelper, defaults googleParams) *GoogleTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleTool{helper: h, defaults: defaults}
}

func (g *GoogleTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: googleToolName,
		Desc: googleToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"q": {
				Type:     schema.String,
				Desc:     "Search query.",
				Required: true,
			},
			"api_key": {
				Type:     schema.String,
				Desc:     "SerpApi API key.",
				Required: true,
			},
			"start": {
				Type:     schema.Integer,
				Desc:     "Result offset. Defaults to 0.",
				Required: false,
			},
			"num": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 6.",
				Required: false,
			},
			"country": {
				Type:     schema.String,
				Desc:     "Google country code. Defaults to us.",
				Required: false,
			},
			"language": {
				Type:     schema.String,
				Desc:     "Google language code. Defaults to en.",
				Required: false,
			},
		}),
	}, nil
}

// InputForm returns the Google fields exposed through Agent tool aggregation.
func (g *GoogleTool) InputForm() map[string]any {
	return map[string]any{
		"q": map[string]any{
			"name": "Query",
			"type": "line",
		},
		"start": map[string]any{
			"name":  "From",
			"type":  "integer",
			"value": 0,
		},
		"num": map[string]any{
			"name":  "Limit",
			"type":  "integer",
			"value": 12,
		},
	}
}

var googleEndpoint = "https://serpapi.com/search.json"

func buildGoogleURL(p googleParams) string {
	query := strings.TrimSpace(p.Q)
	if query == "" {
		query = strings.TrimSpace(p.Query)
	}
	num := p.Num
	if num <= 0 {
		num = p.MaxResults
	}
	if num <= 0 {
		num = 6
	}
	country := strings.TrimSpace(p.Country)
	if country == "" {
		country = "us"
	}
	language := strings.TrimSpace(p.Language)
	if language == "" {
		language = "en"
	}

	q := url.Values{}
	q.Set("api_key", p.APIKey)
	q.Set("engine", "google")
	q.Set("q", query)
	q.Set("google_domain", "google.com")
	q.Set("gl", country)
	q.Set("hl", language)
	q.Set("start", strconv.Itoa(p.Start))
	q.Set("num", strconv.Itoa(num))
	return googleEndpoint + "?" + q.Encode()
}

func (g *GoogleTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p googleParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return googleErrJSON(fmt.Errorf("google: parse arguments: %w", err)),
			fmt.Errorf("google: parse arguments: %w", err)
	}
	p = mergeGoogleDefaults(g.defaults, p)
	if strings.TrimSpace(p.Q) == "" && strings.TrimSpace(p.Query) == "" {
		return googleErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("google: query is required")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return googleErrJSON(fmt.Errorf("google: api_key is required")),
			fmt.Errorf("google: api_key is required")
	}

	resp, err := g.helper.Do(ctx, http.MethodGet, buildGoogleURL(p), "", "", nil)
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
	return googleJSON(googleEnvelope{Results: raw.OrganicResults}), nil
}

func mergeGoogleDefaults(defaults, p googleParams) googleParams {
	if strings.TrimSpace(p.Query) != "" {
		p.Q = p.Query
	}
	if p.MaxResults > 0 {
		p.Num = p.MaxResults
	}

	if p.APIKey == "" {
		p.APIKey = defaults.APIKey
	}
	if p.Q == "" {
		p.Q = defaults.Q
	}
	if p.Q == "" {
		p.Q = defaults.Query
	}
	if p.Start == 0 {
		p.Start = defaults.Start
	}
	if p.Num == 0 {
		p.Num = defaults.Num
	}
	if p.Num == 0 {
		p.Num = defaults.MaxResults
	}
	if p.Country == "" {
		p.Country = defaults.Country
	}
	if p.Language == "" {
		p.Language = defaults.Language
	}
	return p
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
