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
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const yahooFinanceToolName = "yahoo_finance"

const yahooFinanceToolDescription = "The Yahoo Finance service provides access to real-time and historical stock market data, company profiles, and financial news."

const yahooFinanceStockCodeDescription = "The stock code or company name."

// yahooFinanceParams keeps the model-emitted stock code separate from the
// Canvas-side data-selection switches. Info exposes only StockCode.
type yahooFinanceParams struct {
	StockCode         string `json:"stock_code"`
	Info              bool   `json:"info"`
	History           bool   `json:"history"`
	Count             bool   `json:"count"`
	Financials        bool   `json:"financials"`
	IncomeStmt        bool   `json:"income_stmt"`
	BalanceSheet      bool   `json:"balance_sheet"`
	CashFlowStatement bool   `json:"cash_flow_statement"`
	News              bool   `json:"news"`
}

// yahooFinanceResponse is the upstream Yahoo Finance /v7/finance/quote
// envelope.
type yahooFinanceResponse struct {
	QuoteResponse struct {
		Result []map[string]any `json:"result"`
		Error  any              `json:"error,omitempty"`
	} `json:"quoteResponse"`
}

// yahooFinanceEnvelope is the tool-to-component transport shape.
type yahooFinanceEnvelope struct {
	Report string `json:"report"`
	Error  string `json:"_ERROR,omitempty"`
}

// yahooFinanceEndpoint is the Yahoo Finance quote URL. Exposed as a
// package var so tests can substitute a httptest.Server URL.
var yahooFinanceEndpoint = "https://query1.finance.yahoo.com/v7/finance/quote"

// YahooFinanceTool is the
// Yahoo Finance quote tool.
// It performs an unauthenticated GET against the public quote API
// via the shared HTTPHelper and returns the parsed quote records.
type YahooFinanceTool struct {
	helper   *HTTPHelper
	defaults yahooFinanceParams
}

var _ ToolComponent = (*YahooFinanceTool)(nil)

// NewYahooFinanceTool returns a YahooFinanceTool using the default
// HTTPHelper.
func NewYahooFinanceTool() *YahooFinanceTool {
	return NewYahooFinanceToolWithDefaults(nil, defaultYahooFinanceParams())
}

// NewYahooFinanceToolWith returns a YahooFinanceTool that uses the
// provided HTTPHelper. Useful for tests.
func NewYahooFinanceToolWith(h *HTTPHelper) *YahooFinanceTool {
	return NewYahooFinanceToolWithDefaults(h, defaultYahooFinanceParams())
}

// NewYahooFinanceToolWithDefaults returns a YahooFinanceTool with Canvas-side
// defaults. This follows the same constructor pattern as GitHubTool.
func NewYahooFinanceToolWithDefaults(h *HTTPHelper, defaults yahooFinanceParams) *YahooFinanceTool {
	if h == nil {
		// ToolParamBase defaults to max_retries=0, so the tool itself performs
		// one request unless a caller injects a differently configured helper.
		h = NewHTTPHelperWithRetry(RetryConfig{MaxAttempts: 1})
	}
	return &YahooFinanceTool{helper: h, defaults: defaults}
}

func defaultYahooFinanceParams() yahooFinanceParams {
	return yahooFinanceParams{Info: true, News: true}
}

// Info returns the tool's metadata for the chat model.
func (y *YahooFinanceTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: yahooFinanceToolName,
		Desc: yahooFinanceToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"stock_code": {
				Type:     schema.String,
				Desc:     yahooFinanceStockCodeDescription,
				Required: true,
			},
		}),
	}, nil
}

func (y *YahooFinanceTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"stock_code": yahooFinanceStockCodeDescription,
		},
		Outputs: map[string]string{"report": "Yahoo Finance data formatted as Markdown."},
		InputForm: map[string]any{
			"stock_code": map[string]any{"type": "line", "name": "Stock code/Company name"},
		},
	}
}

// buildYahooFinanceURL composes the quote URL for one Python-compatible
// stock_code input. Centralized for testability.
func buildYahooFinanceURL(stockCode string) string {
	q := url.Values{}
	q.Set("symbols", stockCode)
	return yahooFinanceEndpoint + "?" + q.Encode()
}

// InvokableRun performs the Yahoo Finance quote lookup.
func (y *YahooFinanceTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p yahooFinanceParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return yahooFinanceErrJSON(fmt.Errorf("yahoo_finance: parse arguments: %w", err)),
			fmt.Errorf("yahoo_finance: parse arguments: %w", err)
	}
	p = mergeYahooFinanceDefaults(y.defaults, p)
	p.StockCode = strings.TrimSpace(p.StockCode)
	if p.StockCode == "" || !p.anySectionEnabled() {
		return yahooFinanceJSON(yahooFinanceEnvelope{Report: ""}), nil
	}

	endpoint := buildYahooFinanceURL(p.StockCode)
	// Yahoo Finance returns 401 unless we send a User-Agent that
	// looks like a real browser. curl-style UA is the conventional
	// workaround for the public (unauthenticated) endpoint.
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "application/json",
	}

	resp, err := y.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return yahooFinanceErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return yahooFinanceErrJSON(fmt.Errorf("yahoo_finance: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("yahoo_finance: upstream returned %d", resp.StatusCode)
	}

	var raw yahooFinanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return yahooFinanceErrJSON(fmt.Errorf("yahoo_finance: decode response: %w", err)),
			fmt.Errorf("yahoo_finance: decode response: %w", err)
	}
	if raw.QuoteResponse.Error != nil {
		err := fmt.Errorf("yahoo_finance: upstream error: %v", raw.QuoteResponse.Error)
		return yahooFinanceErrJSON(err), err
	}
	return yahooFinanceJSON(yahooFinanceEnvelope{Report: renderYahooFinanceReport(raw.QuoteResponse.Result)}), nil
}

func (y *YahooFinanceTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	report, _ := envelope["report"].(string)
	return map[string]any{"report": report}
}

func mergeYahooFinanceDefaults(defaults, params yahooFinanceParams) yahooFinanceParams {
	params.Info = defaults.Info
	params.History = defaults.History
	params.Count = defaults.Count
	params.Financials = defaults.Financials
	params.IncomeStmt = defaults.IncomeStmt
	params.BalanceSheet = defaults.BalanceSheet
	params.CashFlowStatement = defaults.CashFlowStatement
	params.News = defaults.News
	return params
}

func (p yahooFinanceParams) anySectionEnabled() bool {
	return p.Info || p.History || p.Count || p.Financials || p.IncomeStmt ||
		p.BalanceSheet || p.CashFlowStatement || p.News
}

// renderYahooFinanceReport keeps the existing public quote endpoint while
// returning the string report expected by the Canvas component.
func renderYahooFinanceReport(quotes []map[string]any) string {
	if len(quotes) == 0 {
		return ""
	}
	sections := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		keys := make([]string, 0, len(quote))
		for key := range quote {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		rows := []string{"# Information:", "| | 0 |", "|:---|:---|"}
		for _, key := range keys {
			rows = append(rows, fmt.Sprintf("| %s | %s |", markdownCell(key), markdownCell(yahooFinanceValue(quote[key]))))
		}
		sections = append(sections, strings.Join(rows, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func yahooFinanceValue(value any) string {
	if value == nil {
		return "None"
	}
	if text, ok := value.(string); ok {
		return text
	}
	if encoded, err := json.Marshal(value); err == nil {
		return string(encoded)
	}
	return fmt.Sprint(value)
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", `\|`)
	return strings.ReplaceAll(value, "\n", "<br>")
}

func yahooFinanceJSON(env yahooFinanceEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"yahoo_finance: marshal result: %s"}`, err)
	}
	return string(b)
}

func yahooFinanceErrJSON(err error) string {
	return yahooFinanceJSON(yahooFinanceEnvelope{Error: err.Error()})
}
