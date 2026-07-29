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
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

const (
	queritToolName        = "querit_search"
	queritToolDescription = "Search the web via the Querit API and return the complete search response."
	queritEndpoint        = "https://api.querit.ai/v1/search"
	queritMaxAttempts     = 3
	queritPromptMaxTokens = 200000
)

var (
	queritRelativeTimeRangePattern = regexp.MustCompile(`^[dwmy][1-9][0-9]*$`)
	queritDateRangePattern         = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}to\d{4}-\d{2}-\d{2}$`)
	queritDataImagePattern         = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)
	queritNewlinePattern           = regexp.MustCompile(`\n+`)
)

type queritParams struct {
	APIKey          string   `json:"api_key"`
	Query           string   `json:"query"`
	Count           int      `json:"count"`
	ChunksPerDoc    *int     `json:"chunks_per_doc"`
	SiteInclude     []string `json:"site_include"`
	SiteExclude     []string `json:"site_exclude"`
	TimeRange       string   `json:"time_range"`
	CountryInclude  []string `json:"country_include"`
	LanguageInclude []string `json:"language_include"`
}

type queritRequest struct {
	Query        string         `json:"query"`
	Count        int            `json:"count"`
	ChunksPerDoc *int           `json:"chunksPerDoc,omitempty"`
	Filters      *queritFilters `json:"filters,omitempty"`
}

type queritFilters struct {
	Sites     *queritIncludeExcludeFilter `json:"sites,omitempty"`
	TimeRange *queritDateFilter           `json:"timeRange,omitempty"`
	Geo       *queritGeoFilter            `json:"geo,omitempty"`
	Languages *queritIncludeFilter        `json:"languages,omitempty"`
}

type queritIncludeExcludeFilter struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type queritIncludeFilter struct {
	Include []string `json:"include,omitempty"`
}

type queritDateFilter struct {
	Date string `json:"date"`
}

type queritGeoFilter struct {
	Countries *queritIncludeFilter `json:"countries,omitempty"`
}

// QueritTool searches the web through Querit. Node-level parameters are
// retained as defaults and model-emitted parameters override them per call.
type QueritTool struct {
	helper    *HTTPHelper
	envKey    func() string
	defaults  queritParams
	retryWait func(context.Context, int) bool
}

var _ ToolComponent = (*QueritTool)(nil)
var _ ReferenceBuilder = (*QueritTool)(nil)

// NewQueritTool returns a QueritTool using the shared HTTP helper and the
// QUERIT_API_KEY environment variable.
func NewQueritTool() *QueritTool {
	return newQueritTool(nil, nil, queritParams{}, nil)
}

// NewQueritToolWith returns a QueritTool using the supplied HTTP helper.
func NewQueritToolWith(helper *HTTPHelper) *QueritTool {
	return newQueritTool(helper, nil, queritParams{}, nil)
}

// NewQueritToolWithEnvKey returns a QueritTool with an injectable API-key
// resolver. It is intended for tests that must not depend on process state.
func NewQueritToolWithEnvKey(helper *HTTPHelper, envKey func() string) *QueritTool {
	return newQueritTool(helper, envKey, queritParams{}, nil)
}

func newQueritTool(
	helper *HTTPHelper,
	envKey func() string,
	defaults queritParams,
	retryWait func(context.Context, int) bool,
) *QueritTool {
	if helper == nil {
		helper = NewHTTPHelper()
	}
	if envKey == nil {
		envKey = defaultQueritEnvKey
	}
	if defaults.Count == 0 {
		defaults.Count = 10
	}
	if defaults.ChunksPerDoc == nil {
		defaults.ChunksPerDoc = queritInt(3)
	}
	if retryWait == nil {
		retryWait = waitForQueritRetry
	}
	return &QueritTool{helper: helper, envKey: envKey, defaults: defaults, retryWait: retryWait}
}

func defaultQueritEnvKey() string { return common.GetEnv(common.EnvQueritAPIKey) }

// Info returns the arguments that a chat model may provide. Credentials stay
// in node configuration or the environment and are never exposed to the model.
func (q *QueritTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: queritToolName,
		Desc: queritToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query.",
				Required: true,
			},
			"count": {
				Type:     schema.Integer,
				Desc:     "Maximum number of documents to return. Defaults to 10 and must be at least 1.",
				Required: false,
			},
			"chunks_per_doc": {
				Type:     schema.Integer,
				Desc:     "Number of snippets per document. Defaults to 3 and must be between 1 and 3.",
				Required: false,
			},
			"site_include": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "Sites that search results must include.",
				Required: false,
			},
			"site_exclude": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "Sites that search results must exclude.",
				Required: false,
			},
			"time_range": {
				Type:     schema.String,
				Desc:     "Relative time range such as d7, w1, m3, or y1, or YYYY-MM-DDtoYYYY-MM-DD.",
				Required: false,
			},
			"country_include": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "Countries that search results must include.",
				Required: false,
			},
			"language_include": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "Languages that search results must include.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun performs a Querit search. All expected operational failures are
// returned as soft-error JSON with a nil Go error so an Agent run can continue.
func (q *QueritTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	provided := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(argsJSON), &provided); err != nil {
		return queritErrorJSON(fmt.Errorf("parse arguments: %w", err)), nil
	}
	queryJSON, hasQuery := provided["query"]
	if hasQuery {
		if strings.TrimSpace(string(queryJSON)) == "null" {
			return queritErrorJSON(fmt.Errorf("query must be provided as a string")), nil
		}
		var query string
		if err := json.Unmarshal(queryJSON, &query); err != nil {
			return queritErrorJSON(fmt.Errorf("query must be provided as a string")), nil
		}
	}
	var runtimeParams queritParams
	if err := json.Unmarshal([]byte(argsJSON), &runtimeParams); err != nil {
		return queritErrorJSON(fmt.Errorf("parse arguments: %w", err)), nil
	}
	params := mergeQueritParams(q.defaults, runtimeParams, provided)
	if !hasQuery && strings.TrimSpace(params.Query) == "" {
		return queritErrorJSON(fmt.Errorf("query must be provided as a string")), nil
	}
	if params.Query == "" {
		return `{}`, nil
	}
	if err := validateQueritParams(params); err != nil {
		return queritErrorJSON(err, params.APIKey), nil
	}

	apiKey := strings.TrimSpace(params.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(q.envKey())
	}
	if apiKey == "" {
		return queritErrorJSON(fmt.Errorf("api_key is required (or set QUERIT_API_KEY)")), nil
	}

	body, err := json.Marshal(buildQueritRequest(params))
	if err != nil {
		return queritErrorJSON(fmt.Errorf("encode request: %w", err), apiKey), nil
	}
	for attempt := 1; attempt <= queritMaxAttempts; attempt++ {
		resp, requestErr := q.helper.Do(
			ctx,
			http.MethodPost,
			queritEndpoint,
			string(body),
			"application/json",
			map[string]string{"Authorization": "Bearer " + apiKey},
		)
		if requestErr != nil {
			return queritErrorJSON(fmt.Errorf("request failed: %w", requestErr), apiKey), nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if attempt == queritMaxAttempts || !q.retryWait(ctx, attempt) {
				return queritErrorJSON(fmt.Errorf("upstream returned %d after %d attempts", resp.StatusCode, attempt), apiKey), nil
			}
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return queritErrorJSON(fmt.Errorf("upstream returned %d", resp.StatusCode), apiKey), nil
		}

		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return queritErrorJSON(fmt.Errorf("read response: %w", readErr), apiKey), nil
		}
		if _, err := decodeQueritResponse(raw); err != nil {
			return queritErrorJSON(err, apiKey), nil
		}
		return string(raw), nil
	}

	return queritErrorJSON(fmt.Errorf("request exhausted retries"), apiKey), nil
}

func mergeQueritParams(defaults, runtimeParams queritParams, provided map[string]json.RawMessage) queritParams {
	merged := defaults
	if _, ok := provided["query"]; ok {
		merged.Query = runtimeParams.Query
	}
	if _, ok := provided["count"]; ok {
		merged.Count = runtimeParams.Count
	}
	if _, ok := provided["chunks_per_doc"]; ok {
		merged.ChunksPerDoc = runtimeParams.ChunksPerDoc
	}
	if _, ok := provided["site_include"]; ok {
		merged.SiteInclude = runtimeParams.SiteInclude
	}
	if _, ok := provided["site_exclude"]; ok {
		merged.SiteExclude = runtimeParams.SiteExclude
	}
	if _, ok := provided["time_range"]; ok {
		merged.TimeRange = runtimeParams.TimeRange
	}
	if _, ok := provided["country_include"]; ok {
		merged.CountryInclude = runtimeParams.CountryInclude
	}
	if _, ok := provided["language_include"]; ok {
		merged.LanguageInclude = runtimeParams.LanguageInclude
	}
	return merged
}

func validateQueritParams(params queritParams) error {
	if params.Count < 1 {
		return fmt.Errorf("count must be at least 1")
	}
	if params.ChunksPerDoc != nil && (*params.ChunksPerDoc < 1 || *params.ChunksPerDoc > 3) {
		return fmt.Errorf("chunks_per_doc must be between 1 and 3")
	}
	if !isValidQueritTimeRange(strings.TrimSpace(params.TimeRange)) {
		return fmt.Errorf("time_range must use dN, wN, mN, yN, or YYYY-MM-DDtoYYYY-MM-DD")
	}
	return nil
}

func isValidQueritTimeRange(value string) bool {
	return value == "" || queritRelativeTimeRangePattern.MatchString(value) || queritDateRangePattern.MatchString(value)
}

func buildQueritRequest(params queritParams) queritRequest {
	request := queritRequest{
		Query:        params.Query,
		Count:        params.Count,
		ChunksPerDoc: params.ChunksPerDoc,
	}
	filters := &queritFilters{}
	if len(params.SiteInclude) > 0 || len(params.SiteExclude) > 0 {
		filters.Sites = &queritIncludeExcludeFilter{Include: params.SiteInclude, Exclude: params.SiteExclude}
	}
	if strings.TrimSpace(params.TimeRange) != "" {
		filters.TimeRange = &queritDateFilter{Date: params.TimeRange}
	}
	if len(params.CountryInclude) > 0 {
		filters.Geo = &queritGeoFilter{Countries: &queritIncludeFilter{Include: params.CountryInclude}}
	}
	if len(params.LanguageInclude) > 0 {
		filters.Languages = &queritIncludeFilter{Include: params.LanguageInclude}
	}
	if filters.Sites != nil || filters.TimeRange != nil || filters.Geo != nil || filters.Languages != nil {
		request.Filters = filters
	}
	return request
}

func decodeQueritResponse(raw []byte) (map[string]any, error) {
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode response: expected exactly one JSON value")
		}
		return nil, fmt.Errorf("decode response: trailing content: %w", err)
	}
	response, ok := decoded.(map[string]any)
	if !ok || response == nil {
		return nil, fmt.Errorf("decode response: expected a JSON object")
	}
	resultsValue, exists := response["results"]
	if !exists {
		return response, nil
	}
	results, ok := resultsValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode response: results must be a JSON object")
	}
	resultValue, exists := results["result"]
	if !exists {
		return response, nil
	}
	if _, ok := resultValue.([]any); !ok {
		return nil, fmt.Errorf("decode response: results.result must be a JSON array")
	}
	return response, nil
}

func waitForQueritRetry(ctx context.Context, attempt int) bool {
	delay := 200 * time.Millisecond
	for current := 1; current < attempt; current++ {
		delay *= 2
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// ComponentSpec returns the QueritSearch Canvas-facing metadata.
func (q *QueritTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		PreserveJSONNumbers: true,
		Inputs: map[string]string{
			"api_key":          "Querit API key. Uses QUERIT_API_KEY when empty.",
			"query":            "Search query.",
			"count":            "Maximum number of documents.",
			"chunks_per_doc":   "Number of snippets per document.",
			"site_include":     "Sites that search results must include.",
			"site_exclude":     "Sites that search results must exclude.",
			"time_range":       "dN, wN, mN, yN, or YYYY-MM-DDtoYYYY-MM-DD.",
			"country_include":  "Countries that search results must include.",
			"language_include": "Languages that search results must include.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Querit references for downstream prompts.",
			"json":               "Complete raw Querit JSON response.",
		},
		InputForm: map[string]any{
			"api_key":          map[string]any{"name": "API key", "type": "password"},
			"query":            map[string]any{"name": "Query", "type": "line"},
			"count":            map[string]any{"name": "Count", "type": "line"},
			"chunks_per_doc":   map[string]any{"name": "Chunks per document", "type": "line"},
			"site_include":     map[string]any{"name": "Site include", "type": "line"},
			"site_exclude":     map[string]any{"name": "Site exclude", "type": "line"},
			"time_range":       map[string]any{"name": "Time range", "type": "line"},
			"country_include":  map[string]any{"name": "Country include", "type": "line"},
			"language_include": map[string]any{"name": "Language include", "type": "line"},
		},
	}
}

// BuildReferences converts results.result into RAGFlow retrieval references.
func (q *QueritTool) BuildReferences(_ context.Context, response map[string]any) ([]map[string]any, []map[string]any) {
	results := queritResults(response)
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		title := queritText(result["title"])
		resultURL := queritText(result["url"])
		content := queritText(result["snippet"])
		if content == "" {
			continue
		}
		content = queritDataImagePattern.ReplaceAllString(content, "")
		content = truncateQueritRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(queritHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(queritHashInt(documentID, 500), 10)
		chunks = append(chunks, map[string]any{
			"id":            displayID,
			"chunk_id":      documentID,
			"content":       content,
			"doc_id":        documentID,
			"document_id":   documentID,
			"docnm_kwd":     title,
			"document_name": title,
			"similarity":    1,
			"score":         1,
			"url":           resultURL,
		})
		docAggs = append(docAggs, map[string]any{
			"doc_name": title,
			"doc_id":   documentID,
			"count":    1,
			"url":      resultURL,
		})
	}
	return chunks, docAggs
}

// BuildComponentOutputs keeps the complete upstream object in json and adds a
// reference-formatted string for downstream Canvas components.
func (q *QueritTool) BuildComponentOutputs(response map[string]any) map[string]any {
	chunks, _ := q.BuildReferences(context.Background(), response)
	return map[string]any{
		"formalized_content": renderQueritReferences(chunks, queritPromptMaxTokens),
		"json":               response,
	}
}

func queritResults(response map[string]any) []map[string]any {
	results, ok := response["results"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := results["result"].([]any)
	if !ok {
		if typed, typedOK := results["result"].([]map[string]any); typedOK {
			return typed
		}
		return nil
	}
	items := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		if item, itemOK := value.(map[string]any); itemOK {
			items = append(items, item)
		}
	}
	return items
}

func renderQueritReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := queritText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + queritText(chunk["id"]),
			"├── Title: " + queritPromptField(chunk["document_name"]),
			"├── URL: " + queritPromptField(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func queritText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func queritPromptField(value any) string {
	return queritNewlinePattern.ReplaceAllString(queritText(value), " ")
}

func queritHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateQueritRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func queritInt(value int) *int { return &value }

func queritStringSlice(value any) ([]string, bool) {
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...), true
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			result = append(result, text)
		}
		return result, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}

func queritErrorJSON(err error, apiKeys ...string) string {
	message := "querit_search: unknown error"
	if err != nil {
		message = "querit_search: " + err.Error()
	}
	for _, apiKey := range apiKeys {
		if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
			message = strings.ReplaceAll(message, apiKey, "[REDACTED]")
		}
	}
	raw, marshalErr := json.Marshal(map[string]any{"_ERROR": message})
	if marshalErr != nil {
		return `{"_ERROR":"querit_search: marshal error"}`
	}
	return string(raw)
}
