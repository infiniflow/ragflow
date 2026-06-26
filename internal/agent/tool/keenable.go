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
	neturl "net/url"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const keenableToolName = "keenable"

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

// keenableResult mirrors one element of the upstream `results` array.
// The Python tool's _retrieve_chunks reads `title`, `url`, `description`,
// so we model those fields and pass everything else through verbatim
// when serializing to the model.
type keenableResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// keenableResponse is the envelope returned by Keenable. We only model
// the fields we care about; the upstream API has more, but they are
// ignored.
type keenableResponse struct {
	Results []keenableResult `json:"results"`
}

// keenableEnvelope is the shape the model actually sees, identical to
// the Python tool's output convention.
type keenableEnvelope struct {
	Results []keenableResult `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// KeenableTool is the Keenable web search tool. It POSTs a search
// request to the public keyless endpoint by default and to the keyed
// endpoint (with X-API-Key) when an API key is provided. The upstream
// `results` array is returned as JSON.
type KeenableTool struct {
	helper *HTTPHelper
	apiKey string

	// envBaseURL resolves the Keenable API base URL from the
	// KEENABLE_API_URL env var (HTTPS enforced). Exposed as a
	// function so tests can inject a fake without mutating process
	// state — matches the envKey pattern used by TavilyTool.
	envBaseURL func() string
}

// NewKeenableTool returns a KeenableTool using the default HTTPHelper
// and the KEENABLE_API_URL env var for base-URL resolution.
func NewKeenableTool() *KeenableTool {
	return NewKeenableToolWith(NewHTTPHelper())
}

// NewKeenableToolWithAPIKey returns a KeenableTool that uses a
// server-provided API key instead of model-visible runtime args.
func NewKeenableToolWithAPIKey(h *HTTPHelper, apiKey string) *KeenableTool {
	t := NewKeenableToolWith(h)
	t.apiKey = strings.TrimSpace(apiKey)
	return t
}

// NewKeenableToolWith returns a KeenableTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewKeenableToolWith(h *HTTPHelper) *KeenableTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &KeenableTool{helper: h, envBaseURL: defaultKeenableEnvBaseURL}
}

// NewKeenableToolWithEnvBaseURL returns a KeenableTool with a custom
// base-URL resolver. Useful for tests that want to inject a fake env
// without mutating process state.
func NewKeenableToolWithEnvBaseURL(h *HTTPHelper, envBaseURL func() string) *KeenableTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if envBaseURL == nil {
		envBaseURL = defaultKeenableEnvBaseURL
	}
	return &KeenableTool{helper: h, envBaseURL: envBaseURL}
}

// defaultKeenableEnvBaseURL is the production base-URL resolver.
// Pulled out as a named function (not a var) so tests cannot
// accidentally mutate it via package-var assignment.
func defaultKeenableEnvBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("KEENABLE_API_URL")); v != "" {
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

// Info returns the tool's metadata for the chat model. The description
// is the short prose above; the parameter schema lists the model-emitted
// fields with sane defaults documented inline.
func (k *KeenableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: keenableToolName,
		Desc: keenableToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search keywords to execute with Keenable. The most important words/terms (and synonyms) from the original request.",
				Required: true,
			},
			"site": {
				Type:     schema.String,
				Desc:     "Optional. Restrict results to a single domain, e.g. 'techcrunch.com'. Defaults to '' (no filter).",
				Required: false,
			},
			"mode": {
				Type:     schema.String,
				Desc:     `Search mode: "pro" (default, deeper) or "realtime" (low latency; requires a server-configured API key).`,
				Required: false,
			},
			"top_n": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 10.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun performs the Keenable search.
func (k *KeenableTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p keenableParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return keenableErrJSON(fmt.Errorf("keenable: parse arguments: %w", err)),
			fmt.Errorf("keenable: parse arguments: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return keenableErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("keenable: query is required")
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
