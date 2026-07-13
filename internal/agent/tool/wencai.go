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
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	wencaiToolName         = "iwencai"
	defaultWencaiTopN      = 10
	defaultWencaiQueryType = "stock"
)

const wencaiToolDescription = `
iwencai search: search platform is committed to providing hundreds of millions of investors with the most timely, accurate and comprehensive information, covering news, announcements, research reports, blogs, forums, Weibo, characters, etc.
robo-advisor intelligent stock selection platform: through AI technology, is committed to providing investors with intelligent stock selection, quantitative investment, main force tracking, value investment, technical analysis and other types of stock selection technologies.
fund selection platform: through AI technology, is committed to providing excellent fund, value investment, quantitative analysis and other fund selection technologies for foundation citizens.
`

var wencaiQueryTypes = map[string]struct{}{
	"stock":            {},
	"zhishu":           {},
	"fund":             {},
	"hkstock":          {},
	"usstock":          {},
	"threeboard":       {},
	"conbond":          {},
	"insurance":        {},
	"futures":          {},
	"lccp":             {},
	"foreign_exchange": {},
}

// wencaiParams mirrors Python WenCaiParam. Query is model-provided runtime
// input; top_n and query_type are Canvas node configuration.
type wencaiParams struct {
	Query     string `json:"query"`
	TopN      int    `json:"top_n"`
	QueryType string `json:"query_type"`
}

type wencaiEnvelope struct {
	Report string `json:"report"`
	Error  string `json:"_ERROR,omitempty"`
}

// WencaiTool implements the behavior currently exposed by Python WenCai.
// The Python integration has its upstream request disabled and returns an
// empty report for both empty and non-empty queries, so the Go tool does the
// same without reporting a false unsupported error.
type WencaiTool struct {
	defaults wencaiParams
}

func NewWencaiTool() *WencaiTool {
	return newWencaiTool(wencaiParams{})
}

func newWencaiTool(defaults wencaiParams) *WencaiTool {
	if defaults.TopN == 0 {
		defaults.TopN = defaultWencaiTopN
	}
	if strings.TrimSpace(defaults.QueryType) == "" {
		defaults.QueryType = defaultWencaiQueryType
	}
	return &WencaiTool{defaults: defaults}
}

// Info exposes only Python meta.parameters. Node configuration does not
// belong in the model-emitted function-call schema.
func (w *WencaiTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: wencaiToolName,
		Desc: strings.TrimSpace(wencaiToolDescription),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The question/conditions to select stocks.",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun matches the current Python invocation result: valid arguments
// produce an empty report and no error because the upstream call is disabled.
func (w *WencaiTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	select {
	case <-ctx.Done():
		err := ctx.Err()
		return wencaiErrJSON(err), err
	default:
	}

	params := w.defaults
	if strings.TrimSpace(argsJSON) != "" {
		var runtimeParams wencaiParams
		if err := json.Unmarshal([]byte(argsJSON), &runtimeParams); err != nil {
			err = fmt.Errorf("wencai: parse arguments: %w", err)
			return wencaiErrJSON(err), err
		}
		params = mergeWencaiParams(params, runtimeParams)
	}
	_ = params
	return wencaiJSON(wencaiEnvelope{Report: ""}), nil
}

func mergeWencaiParams(defaults, params wencaiParams) wencaiParams {
	if params.Query == "" {
		params.Query = defaults.Query
	}
	if params.TopN == 0 {
		params.TopN = defaults.TopN
	}
	if strings.TrimSpace(params.QueryType) == "" {
		params.QueryType = defaults.QueryType
	}
	return params
}

func isWencaiQueryTypeSupported(queryType string) bool {
	_, ok := wencaiQueryTypes[queryType]
	return ok
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
