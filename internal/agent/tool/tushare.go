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
	"net/http"
	"net/url"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const tushareToolName = "tushare"

const tushareToolDescription = "Call Tushare Pro (api.tushare.pro) for Chinese financial data. " +
	"Returns the upstream data.fields and data.items arrays."

// tushareEndpoint is the Tushare Pro REST endpoint. Exposed as a
// package var so tests can substitute a httptest.Server URL.
var tushareEndpoint = "http://api.tushare.pro"

// tushareParams is the JSON shape the model sends into InvokableRun.
//
//   - Token (required): the Tushare Pro API token (积分账户).
//   - APIName (required): the Tushare interface name, e.g. "stock_basic",
//     "daily", "fund_basic", "index_daily", "fut_basic".
//   - Params (optional): a free-form map of Tushare query parameters,
//     e.g. {"ts_code":"000001.SZ","start_date":"20240101"}.
type tushareParams struct {
	Token   string            `json:"token"`
	APIName string            `json:"api_name"`
	Params  map[string]string `json:"params,omitempty"`
	Fields  []string          `json:"fields,omitempty"`
}

// tushareRequest is the JSON envelope Tushare Pro expects on POST.
type tushareRequest struct {
	Token   string            `json:"token"`
	APIName string            `json:"api_name"`
	Params  map[string]string `json:"params,omitempty"`
	Fields  []string          `json:"fields,omitempty"`
}

// tushareData is the upstream `data` field shape. Tushare returns a
// column-major record: `fields` lists the column names in order, and
// `items` is a slice of rows where each row is a slice aligned with
// `fields`.
type tushareData struct {
	Fields []string `json:"fields"`
	Items  [][]any  `json:"items"`
}

// tushareResponse is the upstream Tushare Pro envelope.
//
//	{
//	  "code": 0,           // 0 = OK
//	  "msg":  "...",
//	  "data": {...}        // optional
//	}
type tushareResponse struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg,omitempty"`
	Data *tushareData `json:"data,omitempty"`
}

// tushareEnvelope is what the model sees. We pass through fields/items
// verbatim so the model can index by column name. _ERROR captures
// non-zero `code` responses from Tushare (e.g. "权限不足" / 40201) and
// transport-level failures.
type tushareEnvelope struct {
	Fields []string `json:"fields,omitempty"`
	Items  [][]any  `json:"items,omitempty"`
	Error  string   `json:"_ERROR,omitempty"`
}

// TushareTool is the Tushare Pro
// Chinese financial data tool (plan §2.11.4 row 19 [numbered 19, also
// mentioned as row 18 in the matrix header], ). It
// performs a POST against api.tushare.pro with the token in the body
// and returns the data.fields + data.items arrays.
//
// TushareTool uses the shared HTTPHelper for retry/timeout/OTel
// propagation.
type TushareTool struct {
	helper *HTTPHelper
}

// NewTushareTool returns a TushareTool using the default HTTPHelper.
func NewTushareTool() *TushareTool {
	return NewTushareToolWith(NewHTTPHelper())
}

// NewTushareToolWith returns a TushareTool that uses the provided
// HTTPHelper. Useful for tests.
func NewTushareToolWith(h *HTTPHelper) *TushareTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &TushareTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (t *TushareTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: tushareToolName,
		Desc: tushareToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"token": {
				Type:     schema.String,
				Desc:     "Tushare Pro API token (积分账户).",
				Required: true,
			},
			"api_name": {
				Type:     schema.String,
				Desc:     "Tushare interface name, e.g. stock_basic, daily, fund_basic, index_daily, fut_basic.",
				Required: true,
			},
			"params": {
				Type:     schema.Object,
				Desc:     "Optional query parameters, e.g. {\"ts_code\":\"000001.SZ\",\"start_date\":\"20240101\"}.",
				Required: false,
			},
			"fields": {
				Type:     schema.Array,
				Desc:     "Optional subset of columns to return (Tushare `fields` parameter).",
				Required: false,
			},
		}),
	}, nil
}

// buildTushareRequestBody marshals the request envelope to JSON. The
// Tushare Pro server expects a flat object — no URL-encoding — so we
// POST raw JSON. Exposed for unit testing.
func buildTushareRequestBody(p tushareParams) ([]byte, error) {
	req := tushareRequest{
		Token:   p.Token,
		APIName: p.APIName,
		Params:  p.Params,
		Fields:  p.Fields,
	}
	return json.Marshal(req)
}

// buildTushareURL returns the POST URL. Tushare's API only reads the
// body, not the query string, but the helper requires a URL.
func buildTushareURL() string {
	u, _ := url.Parse(tushareEndpoint)
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	return u.String()
}

// InvokableRun performs the Tushare Pro POST call.
func (t *TushareTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p tushareParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return tushareErrJSON(fmt.Errorf("tushare: parse arguments: %w", err)),
			fmt.Errorf("tushare: parse arguments: %w", err)
	}
	if p.Token == "" {
		return tushareErrJSON(fmt.Errorf("tushare: token is required")),
			fmt.Errorf("tushare: token is required")
	}
	if p.APIName == "" {
		return tushareErrJSON(fmt.Errorf("tushare: api_name is required")),
			fmt.Errorf("tushare: api_name is required")
	}

	body, err := buildTushareRequestBody(p)
	if err != nil {
		return tushareErrJSON(fmt.Errorf("tushare: build request: %w", err)),
			fmt.Errorf("tushare: build request: %w", err)
	}

	headers := map[string]string{
		"Accept": "application/json",
	}

	resp, err := t.helper.Do(ctx, http.MethodPost, buildTushareURL(), string(body), "application/json", headers)
	if err != nil {
		return tushareErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tushareErrJSON(fmt.Errorf("tushare: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("tushare: upstream returned %d", resp.StatusCode)
	}

	var raw tushareResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return tushareErrJSON(fmt.Errorf("tushare: decode response: %w", err)),
			fmt.Errorf("tushare: decode response: %w", err)
	}
	if raw.Code != 0 {
		msg := strings.TrimSpace(raw.Msg)
		if msg == "" {
			msg = fmt.Sprintf("tushare: upstream returned code %d", raw.Code)
		}
		return tushareErrJSON(fmt.Errorf("tushare: %s", msg)),
			fmt.Errorf("tushare: %s", msg)
	}

	env := tushareEnvelope{Error: ""}
	if raw.Data != nil {
		env.Fields = raw.Data.Fields
		env.Items = raw.Data.Items
	}
	return tushareJSON(env), nil
}

func tushareJSON(env tushareEnvelope) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Error string is set via _ERROR, but if Error is empty we want the
	// key omitted. Use a typed marshal to honour the omitempty tag.
	if err := enc.Encode(env); err != nil {
		return fmt.Sprintf(`{"_ERROR":"tushare: marshal result: %s"}`, err)
	}
	// json.Encoder always appends a newline — strip it.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return string(out)
}

func tushareErrJSON(err error) string {
	return tushareJSON(tushareEnvelope{Error: err.Error()})
}
