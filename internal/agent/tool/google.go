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
)

const googleToolName = "google"

const googleToolDescription = "Search the web via Google using SerpApi. Returns organic_results[].{title, link, snippet}."

// googleParams is the JSON shape the model or canvas sends into InvokableRun.
type googleParams struct {
	APIKey   string `json:"api_key"`
	Q        string `json:"q"`
	Start    int    `json:"start"`
	Num      int    `json:"num"`
	Country  string `json:"country"`
	Language string `json:"language"`
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

// ToolMeta returns the tool's metadata for the chat model.
func (g *GoogleTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        googleToolName,
		Description: googleToolDescription,
		Parameters: map[string]ParameterInfo{
			"q": {
				Type:        ParamTypeString,
				Description: "The search keywords to execute with Google. The keywords should be the most important words/terms(includes synonyms) from the original request.",
				Required:    true,
			},
			"start": {
				Type:        ParamTypeInteger,
				Description: "Parameter defines the result offset. It skips the given number of results. It's used for pagination. (e.g., 0 (default) is the first page of results, 10 is the 2nd page of results, 20 is the 3rd page of results, etc.). Google Local Results only accepts multiples of 20(e.g. 20 for the second page results, 40 for the third page results, etc.) as the start value.",
				Required:    false,
			},
			"num": {
				Type:        ParamTypeInteger,
				Description: "Parameter defines the maximum number of results to return. (e.g., 10 (default) returns 10 results, 40 returns 40 results, and 100 returns 100 results). The use of num may introduce latency, and/or prevent the inclusion of specialized result types. It is better to omit this parameter unless it is strictly necessary to increase the number of results per page. Results are not guaranteed to have the number of results specified in num.",
				Required:    false,
			},
		},
	}
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
	num := p.Num
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

// InvokableRun performs the Google search via SerpApi. The api_key, country,
// language, q, start, and num may come from the call args or the tool's
// node-level defaults (NewGoogleToolWithDefaults).
func (g *GoogleTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p googleParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return googleErrJSON(fmt.Errorf("google: parse arguments: %w", err)),
			fmt.Errorf("google: parse arguments: %w", err)
	}
	p = mergeGoogleDefaults(g.defaults, p)
	if strings.TrimSpace(p.Q) == "" {
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
	if p.APIKey == "" {
		p.APIKey = defaults.APIKey
	}
	if p.Q == "" {
		p.Q = defaults.Q
	}
	if p.Start == 0 {
		p.Start = defaults.Start
	}
	if p.Num == 0 {
		p.Num = defaults.Num
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
