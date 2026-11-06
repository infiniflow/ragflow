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
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const specstreamExtractToolName = "specstream_extract"

// specstreamExtractEndpoint is the SpecStream Extract API URL. Exposed as a
// package var so tests can substitute it through rewriteHostTransport.
var specstreamExtractEndpoint = "https://api.specstream.ai/v1/extract"

type specstreamExtractArgs struct {
	URL    string `json:"url"`
	Format string `json:"format"`
}

type specstreamExtractRequestBody struct {
	URLs   []string `json:"urls"`
	Format string   `json:"format"`
}

type specstreamExtractEnvelope struct {
	Results map[string]any `json:"results,omitempty"`
	Error   string         `json:"_ERROR,omitempty"`
}

// SpecStreamExtractTool reads a web page or document (PDF, Word, Excel) through
// the SpecStream Extract API (POST /v1/extract) and returns its extracted main
// text and metadata. The API key is node configuration only and never
// model-visible.
type SpecStreamExtractTool struct {
	helper     *HTTPHelper
	apiKey     string
	defaultURL string
}

var _ ToolComponent = (*SpecStreamExtractTool)(nil)

// NewSpecStreamExtractTool returns a SpecStreamExtractTool with no API key and
// the default HTTPHelper.
func NewSpecStreamExtractTool() *SpecStreamExtractTool {
	return newSpecStreamExtractTool(nil, "", "")
}

// NewSpecStreamExtractToolWith returns a SpecStreamExtractTool that uses the
// provided HTTPHelper and API key.
func NewSpecStreamExtractToolWith(h *HTTPHelper, apiKey string) *SpecStreamExtractTool {
	return newSpecStreamExtractTool(h, apiKey, "")
}

func newSpecStreamExtractTool(h *HTTPHelper, apiKey, defaultURL string) *SpecStreamExtractTool {
	if h == nil {
		h = NewHTTPHelper()
		h.client.Timeout = 60 * time.Second
	}
	return &SpecStreamExtractTool{helper: h, apiKey: strings.TrimSpace(apiKey), defaultURL: defaultURL}
}

// Info returns the tool's metadata for the chat model.
func (e *SpecStreamExtractTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: specstreamExtractToolName,
		Desc: "Read one web page or document (PDF, Word, Excel) with SpecStream and return its title and main text. Use it to read a page found by a search, or a URL the user gave.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The absolute HTTP or HTTPS URL of the page or document to read.",
				Required: true,
			},
			"format": {
				Type:     schema.String,
				Desc:     `Output text format: "markdown" (default) or "text".`,
				Enum:     []string{"markdown", "text"},
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun extracts the page via POST /v1/extract. Recoverable errors are
// returned as a JSON envelope with an _ERROR field and a nil Go error.
func (e *SpecStreamExtractTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args specstreamExtractArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: parse arguments: %w", err)), nil
	}
	pageURL := strings.TrimSpace(args.URL)
	if pageURL == "" {
		pageURL = strings.TrimSpace(e.defaultURL)
	}
	if pageURL == "" {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: url is required")), nil
	}
	parsed, err := url.ParseRequestURI(pageURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: url %q must be an absolute HTTP or HTTPS URL", pageURL)), nil
	}
	format := strings.ToLower(strings.TrimSpace(args.Format))
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "text" {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: format %q is not supported, it should be markdown or text", args.Format)), nil
	}
	if e.apiKey == "" {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: api_key is required")), nil
	}

	body, _ := json.Marshal(specstreamExtractRequestBody{
		URLs:   []string{pageURL},
		Format: format,
	})
	resp, err := e.helper.Do(ctx,
		http.MethodPost, specstreamExtractEndpoint, string(body), "application/json",
		map[string]string{
			"Authorization": "Bearer " + e.apiKey,
			"Accept":        "application/json",
			"User-Agent":    "RAGFlow specstream-integration/infiniflow-ragflow",
		},
	)
	if err != nil {
		return specstreamExtractErrJSON(err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: upstream returned %d", resp.StatusCode)), nil
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: decode response: %w", err)), nil
	}
	if dataObj, ok := payload["data"].(map[string]any); ok {
		payload = dataObj
	}
	rawResults, ok := payload["results"].([]any)
	if !ok || len(rawResults) == 0 {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: response has no results")), nil
	}
	firstResult, ok := rawResults[0].(map[string]any)
	if !ok || firstResult == nil {
		return specstreamExtractErrJSON(fmt.Errorf("specstream_extract: response has no results")), nil
	}
	return specstreamExtractJSON(specstreamExtractEnvelope{Results: firstResult}), nil
}

// ComponentSpec returns the SpecStreamExtract Canvas surface.
func (e *SpecStreamExtractTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"url": "The absolute HTTP or HTTPS URL of the page or document to read.",
		},
		Outputs: map[string]string{
			"json": "The extracted page: url, status, title, text, published and read_from.",
		},
		InputForm: map[string]any{
			"url": map[string]any{"name": "URL", "type": "line"},
		},
	}
}

func (e *SpecStreamExtractTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results, _ := envelope["results"].(map[string]any)
	if results == nil {
		results = map[string]any{}
	}
	return map[string]any{"json": results}
}

func specstreamExtractJSON(env specstreamExtractEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"specstream_extract: marshal result: %s"}`, err)
	}
	return string(b)
}

func specstreamExtractErrJSON(err error) string {
	return specstreamExtractJSON(specstreamExtractEnvelope{Error: err.Error()})
}
