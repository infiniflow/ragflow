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
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

const googleToolName = "google_search"

const googleToolDescription = "Search the web via Google using SerpApi. Returns organic_results[].{title, link, snippet}."

const googlePromptMaxTokens = 200000

var googleDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var googleNewlinePattern = regexp.MustCompile(`\n+`)

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

var _ ToolComponent = (*GoogleTool)(nil)
var _ ReferenceBuilder = (*GoogleTool)(nil)

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

func (g *GoogleTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"q":        "Search query.",
			"start":    "Result offset.",
			"num":      "Maximum number of results.",
			"api_key":  "SerpApi API key.",
			"country":  "Google country code.",
			"language": "Google language code.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Google references for downstream prompts.",
			"json":               "Raw Google organic result list.",
		},
		InputForm: g.InputForm(),
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

func (g *GoogleTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildGoogleReferences(envelope)
}

func (g *GoogleTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildGoogleReferences(envelope)
	return map[string]any{
		"formalized_content": renderGoogleReferences(chunks, googlePromptMaxTokens),
		"json":               results,
	}
}

func buildGoogleReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := strings.TrimSpace(googleText(item["snippet"]))
		if content == "" {
			content = strings.TrimSpace(googleAboutDescription(item["about_this_result"]))
		}
		content = googleDataImagePattern.ReplaceAllString(content, "")
		content = truncateGoogleRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(googleHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(googleHashInt(documentID, 500), 10)
		title := strings.TrimSpace(googleText(item["title"]))
		resultURL := strings.TrimSpace(googleText(item["link"]))
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

func renderGoogleReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := googleText(chunk["content"])
		if content == "" {
			continue
		}
		block := strings.Join([]string{
			"\nID: " + googleText(chunk["id"]),
			"├── Title: " + googlePromptField(chunk["document_name"]),
			"├── URL: " + googlePromptField(chunk["url"]),
			"└── Content:\n" + content,
		}, "\n")
		blockTokens := tokenizer.NumTokensFromString(block)
		if maxTokens > 0 && float64(usedTokens+blockTokens) > float64(maxTokens)*0.97 {
			break
		}
		usedTokens += blockTokens
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n")
}

func googleAboutDescription(value any) string {
	about, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	source, ok := about["source"].(map[string]any)
	if !ok {
		return ""
	}
	return googleText(source["description"])
}

func googleText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func googlePromptField(value any) string {
	return googleNewlinePattern.ReplaceAllString(googleText(value), " ")
}

func googleHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateGoogleRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
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
