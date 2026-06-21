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

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const deeplToolName = "deepl"

const deeplToolDescription = "Translate text via the DeepL API. Returns translations[].{text, detected_source_language}."

// deeplParams is the JSON shape the model sends into InvokableRun.
type deeplParams struct {
	APIKey     string `json:"api_key"`
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// deeplTranslation is one element of the upstream `translations` array.
type deeplTranslation struct {
	Text                   string `json:"text"`
	DetectedSourceLanguage string `json:"detected_source_language"`
}

// deeplResponse is the upstream DeepL envelope.
type deeplResponse struct {
	Translations []deeplTranslation `json:"translations"`
}

// deeplEnvelope is what the model sees.
type deeplEnvelope struct {
	Results []deeplTranslation `json:"results"`
	Error   string             `json:"_ERROR,omitempty"`
}

// deeplFreeEndpoint is the DeepL free-plan API host. The pro plan uses
// api.deepl.com; we default to free because it is the public default
// in the Python tool. Override with deeplEndpoint for tests.
var deeplFreeEndpoint = "https://api-free.deepl.com/v2/translate"

// deeplProEndpoint is the DeepL pro-plan API host.
var deeplProEndpoint = "https://api.deepl.com/v2/translate"

// DeepLTool is the DeepL
// translation tool. It POSTs
// a translation request to the DeepL /v2/translate endpoint via the
// shared HTTPHelper.
type DeepLTool struct {
	helper *HTTPHelper
}

// NewDeepLTool returns a DeepLTool using the default HTTPHelper.
func NewDeepLTool() *DeepLTool {
	return NewDeepLToolWith(NewHTTPHelper())
}

// NewDeepLToolWith returns a DeepLTool that uses the provided
// HTTPHelper. Useful for tests.
func NewDeepLToolWith(h *HTTPHelper) *DeepLTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &DeepLTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (d *DeepLTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: deeplToolName,
		Desc: deeplToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"api_key": {
				Type:     schema.String,
				Desc:     "DeepL API authentication key. Free keys end in ':fx'.",
				Required: true,
			},
			"text": {
				Type:     schema.String,
				Desc:     "Text to translate.",
				Required: true,
			},
			"source_lang": {
				Type:     schema.String,
				Desc:     `Source language code (e.g. "EN", "DE"). Defaults to "EN".`,
				Required: false,
			},
			"target_lang": {
				Type:     schema.String,
				Desc:     `Target language code (e.g. "ZH", "EN-US"). Defaults to "ZH".`,
				Required: false,
			},
		}),
	}, nil
}

// buildDeepLFormBody composes the application/x-www-form-urlencoded
// body that the DeepL /v2/translate endpoint expects. Centralized so
// the test suite can verify field encoding.
func buildDeepLFormBody(text, sourceLang, targetLang string) string {
	form := url.Values{}
	form.Set("text", text)
	if sourceLang != "" {
		form.Set("source_lang", strings.ToUpper(sourceLang))
	}
	if targetLang != "" {
		form.Set("target_lang", strings.ToUpper(targetLang))
	}
	return form.Encode()
}

// InvokableRun performs the DeepL translation.
func (d *DeepLTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p deeplParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return deeplErrJSON(fmt.Errorf("deepl: parse arguments: %w", err)),
			fmt.Errorf("deepl: parse arguments: %w", err)
	}
	if p.APIKey == "" {
		return deeplErrJSON(fmt.Errorf("api_key is required")),
			fmt.Errorf("deepl: api_key is required")
	}
	if strings.TrimSpace(p.Text) == "" {
		return deeplErrJSON(fmt.Errorf("text is required")),
			fmt.Errorf("deepl: text is required")
	}
	if p.SourceLang == "" {
		p.SourceLang = "EN"
	}
	if p.TargetLang == "" {
		p.TargetLang = "ZH"
	}

	endpoint := deeplFreeEndpoint
	if !strings.HasSuffix(p.APIKey, ":fx") {
		// non-:fx keys are pro plan keys; route to the pro endpoint.
		endpoint = deeplProEndpoint
	}

	body := buildDeepLFormBody(p.Text, p.SourceLang, p.TargetLang)
	headers := map[string]string{
		"Authorization": "DeepL-Auth-Key " + p.APIKey,
	}

	resp, err := d.helper.Do(ctx, http.MethodPost, endpoint, body,
		"application/x-www-form-urlencoded", headers)
	if err != nil {
		return deeplErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return deeplErrJSON(fmt.Errorf("deepl: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("deepl: upstream returned %d", resp.StatusCode)
	}

	var raw deeplResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return deeplErrJSON(fmt.Errorf("deepl: decode response: %w", err)),
			fmt.Errorf("deepl: decode response: %w", err)
	}
	return deeplJSON(deeplEnvelope{Results: raw.Translations}), nil
}

func deeplJSON(env deeplEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"deepl: marshal result: %s"}`, err)
	}
	return string(b)
}

func deeplErrJSON(err error) string {
	return deeplJSON(deeplEnvelope{Error: err.Error()})
}
