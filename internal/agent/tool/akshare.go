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

const akshareToolName = "akshare"

const akshareToolDescription = "Query Chinese A-share financial data (AkShare). " +
	"STUB: AkShare is a Python library — not available in the Go Canvas. " +
	"Use the Python Canvas for Chinese A-share data."

const akshareUnsupportedMessage = "AkShare is a Python library — not available in Go Canvas. " +
	"Use Python Canvas for Chinese A-share data."

// akshareParams is the JSON shape the model sends into InvokableRun.
//
// AkShare exposes 200+ indicator functions (e.g. stock_zh_a_hist,
// stock_zh_a_spot_em, bond_zh_hs_cov). For a future HTTP shim that routes
// to a Python backend we keep the same shape: a stock/asset symbol plus
// the indicator name. The Go stub validates the shape and rejects all
// invocations with a clear error.
type akshareParams struct {
	Symbol    string `json:"symbol"`
	Indicator string `json:"indicator"`
}

// akshareEnvelope is the model-facing JSON shape. The stub always
// returns a populated Error so the model gets a deterministic, parseable
// failure.
type akshareEnvelope struct {
	Fields []string `json:"fields,omitempty"`
	Items  []any    `json:"items,omitempty"`
	Error  string   `json:"_ERROR,omitempty"`
}

// AkShareTool is a stub implementation of the AkShare Chinese
// financial data tool.
//
// AkShare is a Python library (https://github.com/akfamily/akshare)
// — there is no public HTTP API. The tool is registered with the
// canvas so DSLs that reference "akshare" continue to parse, but
// every invocation fails fast with a clear "use Python Canvas"
// message rather
// than silently no-op'ing.
//
// AkShareTool does not own an HTTPHelper — it never makes network calls.
type AkShareTool struct{}

// NewAkShareTool returns an AkShareTool. No HTTPHelper is allocated;
// the stub never issues network requests.
func NewAkShareTool() *AkShareTool { return &AkShareTool{} }

// Info returns the tool's metadata for the chat model.
func (a *AkShareTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: akshareToolName,
		Desc: akshareToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"symbol": {
				Type:     schema.String,
				Desc:     "Stock or asset symbol (e.g. 000001, sh600000, bj920566).",
				Required: true,
			},
			"indicator": {
				Type:     schema.String,
				Desc:     "AkShare indicator function name (e.g. stock_zh_a_hist, stock_zh_a_spot_em).",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun validates the input shape and returns a clear "use Python
// Canvas" error. The model receives a JSON envelope with the message in
// the `_ERROR` field so it can present a useful message back to the user
// without surfacing a Go stack trace.
func (a *AkShareTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p akshareParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return akshareErrJSON(errors.New(akshareUnsupportedMessage)),
			errors.New(akshareUnsupportedMessage)
	}
	if p.Symbol == "" || p.Indicator == "" {
		return akshareErrJSON(errors.New(akshareUnsupportedMessage)),
			errors.New(akshareUnsupportedMessage)
	}
	return akshareErrJSON(errors.New(akshareUnsupportedMessage)),
		errors.New(akshareUnsupportedMessage)
}

func akshareJSON(env akshareEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"akshare: marshal result: %s"}`, err)
	}
	return string(b)
}

func akshareErrJSON(err error) string {
	return akshareJSON(akshareEnvelope{Error: err.Error()})
}
