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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const bgptToolName = "bgpt_search"

const bgptToolDescription = "Search scientific papers via BGPT (bgpt.pro) and return structured evidence " +
	"extracted from full-text studies: methods, sample sizes, results, limitations, conflicts of interest, " +
	"data/code availability, study blind spots, quality scores, and falsification prompts."

// bgptEndpoint is the BGPT search URL. Exposed as a package var for tests.
var bgptEndpoint = "https://bgpt.pro/api/mcp-search"

// bgptParams is the JSON shape the model sends into InvokableRun.
type bgptParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"num_results"`
	APIKey     string `json:"api_key,omitempty"`
	DaysBack   int    `json:"days_back,omitempty"`
}

// bgptResult is one paper in the result list.
type bgptResult struct {
	Title              string `json:"title"`
	Authors            string `json:"authors"`
	Journal            string `json:"journal"`
	Year               string `json:"year"`
	DOI                string `json:"doi"`
	URL                string `json:"url"`
	Abstract           string `json:"abstract"`
	Methods            string `json:"methods"`
	SampleSize         string `json:"sample_size"`
	Results            string `json:"results"`
	Limitations        string `json:"limitations"`
	ConflictOfInterest string `json:"conflict_of_interest"`
	DataAvailability   string `json:"data_availability"`
	BlindSpots         string `json:"blind_spots"`
	Falsify            string `json:"falsify"`
}

// bgptEnv is what the model sees.
type bgptEnv struct {
	Results []bgptResult `json:"results"`
	Error   string       `json:"_ERROR,omitempty"`
}

// bgptResponse is the upstream BGPT API envelope.
type bgptResponse struct {
	Results []map[string]interface{} `json:"results"`
	Error   string                   `json:"error,omitempty"`
}

// BGPTTool searches scientific papers via bgpt.pro.
type BGPTTool struct {
	helper *HTTPHelper
}

// NewBGPTTool returns a BGPTTool using the default HTTPHelper.
func NewBGPTTool() *BGPTTool {
	return NewBGPTToolWith(NewHTTPHelper())
}

// NewBGPTToolWith returns a BGPTTool with the given helper.
func NewBGPTToolWith(h *HTTPHelper) *BGPTTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &BGPTTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (b *BGPTTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: bgptToolName,
		Desc: bgptToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Natural-language scientific search query.",
				Required: true,
			},
			"num_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results. Defaults to 10.",
				Required: false,
			},
			"api_key": {
				Type:     schema.String,
				Desc:     "Optional BGPT API key. Leave blank for the free tier.",
				Required: false,
			},
			"days_back": {
				Type:     schema.Integer,
				Desc:     "Optional recency filter (e.g. 365 for last year).",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun performs the BGPT search.
func (b *BGPTTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p bgptParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: parse arguments: %w", err)),
			fmt.Errorf("bgpt: parse arguments: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return bgptErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("bgpt: query is required")
	}
	if p.MaxResults <= 0 {
		p.MaxResults = 10
	}

	reqBody := map[string]interface{}{
		"query":       strings.TrimSpace(p.Query),
		"num_results": p.MaxResults,
	}
	if p.APIKey != "" {
		reqBody["api_key"] = p.APIKey
	}
	if p.DaysBack > 0 {
		reqBody["days_back"] = p.DaysBack
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return bgptErrJSON(err), err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bgptEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return bgptErrJSON(err), err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: request failed: %w", err)),
			fmt.Errorf("bgpt: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: read response: %w", err)),
			fmt.Errorf("bgpt: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bgptErrJSON(fmt.Errorf("bgpt: upstream returned %d: %s", resp.StatusCode, string(body))),
			fmt.Errorf("bgpt: upstream returned %d: %s", resp.StatusCode, string(body))
	}

	var raw bgptResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return bgptErrJSON(fmt.Errorf("bgpt: parse response: %w", err)),
			fmt.Errorf("bgpt: parse response: %w", err)
	}
	if strings.TrimSpace(raw.Error) != "" {
		err := fmt.Errorf("bgpt: %s", strings.TrimSpace(raw.Error))
		return bgptErrJSON(err), err
	}

	results := make([]bgptResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		results = append(results, bgptResult{
			Title:              strVal(r["title"]),
			Authors:            strVal(r["authors"]),
			Journal:            strVal(r["journal"]),
			Year:               strVal(r["year"]),
			DOI:                strVal(r["doi"]),
			URL:                strVal(r["url"]),
			Abstract:           strVal(r["abstract"]),
			Methods:            firstStr(r, "methods_and_experimental_techniques", "methods"),
			SampleSize:         firstStr(r, "sample_size_and_population_characteristics", "sample_size_and_population"),
			Results:            firstStr(r, "results_and_conclusions", "results"),
			Limitations:        firstStr(r, "paper_limitations_and_biases", "limitations"),
			ConflictOfInterest: firstStr(r, "conflict_of_interest_statements", "conflict_of_interest"),
			DataAvailability:   firstStr(r, "data_availability_statements", "data_availability"),
			BlindSpots:         strVal(r["study_blindspots"]),
			Falsify:            strVal(r["how_to_falsify"]),
		})
	}

	env := bgptEnv{Results: results}
	out, _ := json.Marshal(env)
	return string(out), nil
}

func bgptErrJSON(err error) string {
	env := bgptEnv{Error: err.Error()}
	b, _ := json.Marshal(env)
	return string(b)
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func firstStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s := strVal(m[k]); s != "" {
			return s
		}
	}
	return ""
}
