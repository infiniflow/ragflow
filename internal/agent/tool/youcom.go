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

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	youComToolName        = "youcom_search"
	youComKeylessEndpoint = "https://api.you.com/v1/agents/search"
	youComKeyedEndpoint   = "https://api.you.com/v1/search"
	youComUserAgent       = "RAGFlow youdotcom-integration/infiniflow-ragflow"
	youComMaxCount        = 100
)

type youComParams struct {
	Query     string `json:"query"`
	Freshness string `json:"freshness"`
	TopN      int    `json:"top_n"`
}

type youComResponse struct {
	Results struct {
		Web  []map[string]any `json:"web"`
		News []map[string]any `json:"news"`
	} `json:"results"`
}

type YouComTool struct {
	helper          *HTTPHelper
	apiKey          string
	defaults        youComParams
	keylessEndpoint string
	keyedEndpoint   string
}

var _ ToolComponent = (*YouComTool)(nil)
var _ ReferenceBuilder = (*YouComTool)(nil)

func newYouComTool(helper *HTTPHelper, apiKey string, defaults youComParams) *YouComTool {
	if helper == nil {
		helper = NewHTTPHelper()
	}
	if defaults.TopN == 0 {
		defaults.TopN = 10
	}
	return &YouComTool{
		helper:          helper,
		apiKey:          strings.TrimSpace(apiKey),
		defaults:        defaults,
		keylessEndpoint: youComKeylessEndpoint,
		keyedEndpoint:   youComKeyedEndpoint,
	}
}

func (y *YouComTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: youComToolName,
		Desc: "Search the web with You.com and return page passages with titles and URLs.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query":     {Type: schema.String, Desc: "Search query.", Required: true},
			"freshness": {Type: schema.String, Desc: "Optional recency window: any, day, week, month or year.", Required: false},
		}),
	}, nil
}

func (y *YouComTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		PreserveJSONNumbers: true,
		Inputs:              map[string]string{"query": "Search query.", "freshness": "Optional recency window."},
		Outputs: map[string]string{
			"formalized_content": "Rendered You.com references for downstream prompts.",
			"json":               "You.com result list.",
		},
		InputForm: map[string]any{
			"query":     map[string]any{"name": "Query", "type": "line"},
			"freshness": map[string]any{"name": "Freshness", "type": "options", "value": "any", "options": []string{"any", "day", "week", "month", "year"}},
		},
	}
}

func (y *YouComTool) InvokableRun(ctx context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	var args youComParams
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("youcom: invalid arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return `{"results":[]}`, nil
	}

	topN := y.defaults.TopN
	if args.TopN > 0 {
		topN = args.TopN
	}
	freshness := y.defaults.Freshness
	if args.Freshness != "" {
		freshness = args.Freshness
	}
	freshness = strings.ToLower(strings.TrimSpace(freshness))
	if freshness != "" && freshness != "any" && freshness != "day" && freshness != "week" && freshness != "month" && freshness != "year" {
		return "", fmt.Errorf("youcom: unsupported freshness %q", freshness)
	}
	params := url.Values{"query": {query}, "count": {strconv.Itoa(min(topN, youComMaxCount))}}
	if freshness != "" && freshness != "any" {
		params.Set("freshness", freshness)
	}

	endpoint := y.keylessEndpoint
	headers := map[string]string{"Accept": "application/json", "User-Agent": youComUserAgent}
	if y.apiKey != "" {
		endpoint = y.keyedEndpoint
		headers["X-API-Key"] = y.apiKey
	}
	response, err := y.helper.Do(ctx, http.MethodGet, endpoint+"?"+params.Encode(), "", "", headers)
	if err != nil {
		// The transport error can contain the query URL or request headers.
		return "", fmt.Errorf("youcom: search request failed (%T)", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("youcom: upstream returned HTTP %d", response.StatusCode)
	}
	var payload youComResponse
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("youcom: invalid response (%T)", err)
	}
	merged := append(payload.Results.Web, payload.Results.News...)
	if len(merged) > topN {
		merged = merged[:topN]
	}
	if merged == nil {
		merged = []map[string]any{}
	}
	encoded, err := json.Marshal(map[string]any{"results": merged})
	if err != nil {
		return "", fmt.Errorf("youcom: encode results: %w", err)
	}
	return string(encoded), nil
}

func youComContent(result map[string]any) string {
	passages := make([]string, 0)
	if snippets, ok := result["snippets"].([]any); ok {
		for _, snippet := range snippets {
			text, ok := snippet.(string)
			if !ok {
				continue
			}
			if trimmed := strings.Join(strings.Fields(text), " "); trimmed != "" {
				passages = append(passages, trimmed)
			}
		}
	}
	if len(passages) > 0 {
		return strings.Join(passages, " ")
	}
	description, _ := result["description"].(string)
	return strings.Join(strings.Fields(description), " ")
}

func (y *YouComTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildYouComReferences(envelope)
}

func buildYouComReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	// You.com hits have the same title, URL and text shape as web results
	// already rendered by the DuckDuckGo reference path.
	converted := make([]any, 0, len(envelopeSlice(envelope, "results")))
	for _, value := range envelopeSlice(envelope, "results") {
		if hit, ok := value.(map[string]any); ok {
			converted = append(converted, map[string]any{"title": hit["title"], "url": hit["url"], "body": youComContent(hit)})
		}
	}
	return buildDuckDuckGoReferences(map[string]any{"results": converted})
}

func (y *YouComTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	chunks, _ := buildYouComReferences(envelope)
	return map[string]any{
		"formalized_content": renderDuckDuckGoReferences(chunks, duckduckgoPromptMaxTokens),
		"json":               envelopeSlice(envelope, "results"),
	}
}
