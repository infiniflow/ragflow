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
	neturl "net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"ragflow/internal/common"
)

const keenableToolName = "keenable_search"

// keenableToolDescription follows the upstream Python tool's description,
// trimmed for the chat model. The "no API key required" line is the
// differentiator from Tavily/DuckDuckGo/SearXNG.
const keenableToolDescription = `Keenable is a web search API built for AI agents. It returns fresh, relevant web results for a query and works without an API key by default (keyless free tier). When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Optionally restrict to a single site/domain.`

// keenableParams is the JSON shape the model sends into InvokableRun.
// site is an optional single-domain filter. mode is "pro" (default,
// deeper) or "realtime" (requires a server-configured key). top_n caps
// how many results we keep from the upstream `results` array.
type keenableParams struct {
	Query string `json:"query"`
	Site  string `json:"site"`
	Mode  string `json:"mode"`
	TopN  int    `json:"top_n"`
}

// keenableRequestBody is the JSON body POSTed to the Keenable search
// endpoint. Mirrors the upstream Python tool — query, mode, and an
// optional site filter.
type keenableRequestBody struct {
	Query string `json:"query"`
	Mode  string `json:"mode"`
	Site  string `json:"site,omitempty"`
}

// keenableResponse preserves complete upstream result objects because Python
// exposes them unchanged through the Canvas json output.
type keenableResponse struct {
	Results []map[string]any `json:"results"`
}

// keenableEnvelope is the shape the model actually sees, identical to
// the Python tool's output convention.
type keenableEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// KeenableTool is the Keenable web search tool. It POSTs a search
// request to the public keyless endpoint by default and to the keyed
// endpoint (with X-API-Key) when an API key is provided. The upstream
// `results` array is returned as JSON.
type KeenableTool struct {
	helper   *HTTPHelper
	apiKey   string
	defaults keenableParams

	// envBaseURL resolves the Keenable API base URL from the
	// KEENABLE_API_URL env var (HTTPS enforced). Exposed as a
	// function so tests can inject a fake without mutating process
	// state — matches the envKey pattern used by TavilyTool.
	envBaseURL func() string
}

var _ ToolComponent = (*KeenableTool)(nil)
var _ ReferenceBuilder = (*KeenableTool)(nil)

// NewKeenableTool returns a KeenableTool using the default HTTPHelper
// and the KEENABLE_API_URL env var for base-URL resolution.
func NewKeenableTool() *KeenableTool {
	return newKeenableTool(nil, nil, "", keenableParams{})
}

// NewKeenableToolWithAPIKey returns a KeenableTool that uses a
// server-provided API key instead of model-visible runtime args.
func NewKeenableToolWithAPIKey(h *HTTPHelper, apiKey string) *KeenableTool {
	return newKeenableTool(h, nil, apiKey, keenableParams{})
}

// NewKeenableToolWith returns a KeenableTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewKeenableToolWith(h *HTTPHelper) *KeenableTool {
	return newKeenableTool(h, nil, "", keenableParams{})
}

// NewKeenableToolWithEnvBaseURL returns a KeenableTool with a custom
// base-URL resolver. Useful for tests that want to inject a fake env
// without mutating process state.
func NewKeenableToolWithEnvBaseURL(h *HTTPHelper, envBaseURL func() string) *KeenableTool {
	return newKeenableTool(h, envBaseURL, "", keenableParams{})
}

func newKeenableTool(h *HTTPHelper, envBaseURL func() string, apiKey string, defaults keenableParams) *KeenableTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if envBaseURL == nil {
		envBaseURL = defaultKeenableEnvBaseURL
	}
	if strings.TrimSpace(defaults.Mode) == "" {
		defaults.Mode = "pro"
	}
	if defaults.TopN == 0 {
		defaults.TopN = 10
	}
	return &KeenableTool{
		helper:     h,
		apiKey:     strings.TrimSpace(apiKey),
		defaults:   defaults,
		envBaseURL: envBaseURL,
	}
}

// defaultKeenableEnvBaseURL is the production base-URL resolver.
// Pulled out as a named function (not a var) so tests cannot
// accidentally mutate it via package-var assignment.
func defaultKeenableEnvBaseURL() string {
	if v := strings.TrimSpace(common.GetEnv(common.EnvKeenableAPIURL)); v != "" {
		return v
	}
	return "https://api.keenable.ai"
}

// resolveKeenableBaseURL returns the validated Keenable base URL.
// HTTPS is required for any non-loopback host; loopback hosts may
// use plain http for local development. Mirrors the Python tool's
// _base_url() guard — a misconfigured URL fails fast at request time
// rather than silently making a request to the wrong host.
func resolveKeenableBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", fmt.Errorf("keenable: empty base URL")
	}
	u, err := neturl.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("keenable: parse KEENABLE_API_URL %q: %w", raw, err)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("keenable: KEENABLE_API_URL must have a host, got %q", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("keenable: KEENABLE_API_URL must not include query or fragment, got %q", raw)
	}
	switch u.Scheme {
	case "https":
		return raw, nil
	case "http":
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return raw, nil
		}
		return "", fmt.Errorf("keenable: KEENABLE_API_URL must be https://, got %q", raw)
	default:
		return "", fmt.Errorf("keenable: KEENABLE_API_URL scheme %q not allowed (https required)", u.Scheme)
	}
}

// ToolMeta returns the tool's metadata for the chat model.
func (k *KeenableTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        keenableToolName,
		Description: keenableToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "Search keywords to execute with Keenable. The most important words/terms (and synonyms) from the original request.",
				Required:    true,
			},
			"site": {
				Type:        ParamTypeString,
				Description: "Optional. Restrict results to a single domain, e.g. 'techcrunch.com'. Defaults to '' (no filter).",
				Required:    false,
			},
		},
	}
}

// InvokableRun performs the Keenable search.
func (k *KeenableTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var runtimeParams keenableParams
	if err := json.Unmarshal([]byte(argsJSON), &runtimeParams); err != nil {
		return keenableErrJSON(fmt.Errorf("keenable: parse arguments: %w", err)),
			fmt.Errorf("keenable: parse arguments: %w", err)
	}
	p := mergeKeenableParams(k.defaults, runtimeParams)
	if strings.TrimSpace(p.Query) == "" {
		return keenableJSON(keenableEnvelope{Results: []map[string]any{}}), nil
	}

	mode := strings.TrimSpace(p.Mode)
	if mode == "" {
		mode = "pro"
	}
	if mode != "pro" && mode != "realtime" {
		return keenableErrJSON(fmt.Errorf("keenable: mode %q must be one of [pro realtime]", p.Mode)),
			fmt.Errorf("keenable: mode %q must be one of [pro realtime]", p.Mode)
	}
	// 'realtime' is only available on the keyed endpoint. Reject the
	// invalid combination up front instead of letting the upstream
	// return a confusing error — matches the Python tool's check().
	if mode == "realtime" && strings.TrimSpace(k.apiKey) == "" {
		return keenableErrJSON(fmt.Errorf("keenable: 'realtime' mode requires a configured api_key")),
			fmt.Errorf("keenable: 'realtime' mode requires a configured api_key")
	}

	topN := p.TopN
	if topN <= 0 {
		topN = 10
	}

	baseURL, err := resolveKeenableBaseURL(k.envBaseURL())
	if err != nil {
		// Config/local error — won't be fixed by retrying, so fail fast
		// (matches the Python tool's behavior for ValueError).
		return keenableErrJSON(err), err
	}

	apiKey := strings.TrimSpace(k.apiKey)
	path := "/v1/search/public"
	headers := map[string]string{
		"User-Agent":       "keenable-ragflow",
		"X-Keenable-Title": "RAGFlow",
	}
	if apiKey != "" {
		path = "/v1/search"
		headers["X-API-Key"] = apiKey
	}

	body := keenableRequestBody{
		Query: p.Query,
		Mode:  mode,
	}
	if site := strings.TrimSpace(p.Site); site != "" {
		body.Site = site
	}

	bodyJSON, _ := json.Marshal(body)

	resp, err := k.helper.Do(ctx,
		http.MethodPost, baseURL+path, string(bodyJSON), "application/json", headers,
	)
	if err != nil {
		return keenableErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return keenableErrJSON(fmt.Errorf("keenable: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("keenable: upstream returned %d", resp.StatusCode)
	}

	var raw keenableResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return keenableErrJSON(fmt.Errorf("keenable: decode response: %w", err)),
			fmt.Errorf("keenable: decode response: %w", err)
	}

	results := raw.Results
	if len(results) > topN {
		results = results[:topN]
	}

	return keenableJSON(keenableEnvelope{Results: results}), nil
}

func mergeKeenableParams(defaults, params keenableParams) keenableParams {
	if strings.TrimSpace(params.Site) == "" {
		params.Site = defaults.Site
	}
	if strings.TrimSpace(params.Mode) == "" {
		params.Mode = defaults.Mode
	}
	if params.TopN == 0 {
		params.TopN = defaults.TopN
	}
	return params
}

// ComponentSpec returns the Python-compatible KeenableSearch Canvas surface.
func (k *KeenableTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query": "The search keywords to execute with Keenable.",
			"site":  "Optional single-domain filter.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered search results for downstream LLM prompts.",
			"json":               "Raw Keenable result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{
				"name": "Query",
				"type": "line",
			},
			"site": map[string]any{
				"name": "Site",
				"type": "line",
			},
		},
	}
}

// BuildReferences builds the same references as Python ToolBase._retrieve_chunks.
func (k *KeenableTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildKeenableReferences(envelope)
}

// BuildComponentOutputs converts Keenable's complete tool envelope into its
// public Canvas outputs.
func (k *KeenableTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildKeenableReferences(envelope)
	return map[string]any{
		"formalized_content": renderKeenableReferences(chunks),
		"json":               results,
	}
}

func buildKeenableReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := truncateKeenableRunes(strings.TrimSpace(keenableValueString(result["description"])), 10000)
		if content == "" || content == "None" {
			continue
		}
		documentID := strconv.FormatInt(keenableHashInt(content, 100000000), 10)
		title := keenableValueString(result["title"])
		resultURL := keenableValueString(result["url"])
		displayID := strconv.FormatInt(keenableHashInt(documentID, 500), 10)
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

func renderKeenableReferences(chunks []map[string]any) string {
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + keenableValueString(chunk["id"]),
			"├── Title: " + keenableValueString(chunk["docnm_kwd"]),
			"├── URL: " + keenableValueString(chunk["url"]),
			"└── Content:\n" + keenableValueString(chunk["content"]),
		}, "\n"))
	}
	return strings.Join(blocks, "\n")
}

func keenableValueString(value any) string {
	if value == nil {
		return "None"
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func keenableHashInt(value string, modulus int64) int64 {
	sum := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(sum[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateKeenableRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// keenableJSON marshals the envelope to a JSON string for the model.
func keenableJSON(env keenableEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"keenable: marshal result: %s"}`, err)
	}
	return string(b)
}

// keenableErrJSON wraps an error in the standard envelope.
func keenableErrJSON(err error) string {
	return keenableJSON(keenableEnvelope{Error: err.Error()})
}
