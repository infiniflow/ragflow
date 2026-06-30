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
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const wencaiToolName = "wencai"

const wencaiToolDescription = "Query 同花顺 Wencai (问财) for natural-language stock screening " +
	"(e.g. \"近期涨停股\", \"高股息低估值\"). " +
	"STUB: Wencai has no public API; the Python implementation scrapes the " +
	"10jqka.com.cn web app. Not yet implemented in the Go Canvas. " +
	"Use the Python Canvas for Wencai queries."

const wencaiUnsupportedMessage = "Wencai requires web scraping of 同花顺 — not yet implemented in Go Canvas. " +
	"Use Python Canvas."

// wencaiParams is the JSON shape the model sends into InvokableRun.
// The Python implementation accepts a free-form natural-language query
// and an optional page/per-page limit. The Go stub preserves the shape
// but rejects every invocation.
type wencaiParams struct {
	Query   string `json:"query"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

// wencaiEnvelope is the model-facing JSON shape. The stub always
// returns a populated Error.
type wencaiEnvelope struct {
	Items []any  `json:"items,omitempty"`
	Error string `json:"_ERROR,omitempty"`
}

// WencaiTool is a stub for the
// 同花顺 Wencai (问财) natural-language stock screening tool
// ( .
//
// Wencai (https://www.iwencai.com) has no public API. The Python
// implementation scrapes 10jqka.com.cn using session cookies and
// reverse-engineered POST endpoints, which is fragile and legally
// grey. A Go port would have to repeat the scraping work and the
// reverse-engineering, and is deferred. For P3-B4 the tool is
// registered so DSLs that reference "wencai" keep parsing, but every
// invocation fails fast with a clear "use Python Canvas" message.
//
// WencaiTool does not own an HTTPHelper — it never makes network calls.
type WencaiTool struct{}

// NewWencaiTool returns a WencaiTool. No HTTPHelper is allocated; the
// stub never issues network requests.
func NewWencaiTool() *WencaiTool { return &WencaiTool{} }

// Info returns the tool's metadata for the chat model.
func (w *WencaiTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: wencaiToolName,
		Desc: wencaiToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Natural-language Wencai query (e.g. \"近期涨停股\", \"高股息低估值\").",
				Required: true,
			},
			"page": {
				Type:     schema.Integer,
				Desc:     "Optional 1-based page number. Defaults to 1.",
				Required: false,
			},
			"per_page": {
				Type:     schema.Integer,
				Desc:     "Optional results per page. Defaults to 20.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun validates the input shape (query is required) and
// returns a clear "use Python Canvas" error. The model receives a
// JSON envelope with the message in the `_ERROR` field.
func (w *WencaiTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p wencaiParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return wencaiErrJSON(errors.New(wencaiUnsupportedMessage)),
			errors.New(wencaiUnsupportedMessage)
	}
	if p.Query == "" {
		return wencaiErrJSON(errors.New(wencaiUnsupportedMessage)),
			errors.New(wencaiUnsupportedMessage)
	}
	return wencaiErrJSON(errors.New(wencaiUnsupportedMessage)),
		errors.New(wencaiUnsupportedMessage)
}

func wencaiJSON(env wencaiEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"wencai: marshal result: %s"}`, err)
	}
	return string(b)
}

func wencaiErrJSON(err error) string {
	return wencaiJSON(wencaiEnvelope{Error: err.Error()})
}
