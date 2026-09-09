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
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const yahooFinanceToolName = "yahoo_finance"

const yahooFinanceToolDescription = "The Yahoo Finance service provides access to real-time and historical stock market data, company profiles, and financial news."

const yahooFinanceStockCodeDescription = "The stock code or company name."

const (
	yahooFinanceInfoDescription              = "Fetch stock information."
	yahooFinanceHistoryDescription           = "Fetch historical market data."
	yahooFinanceCountDescription             = "Fetch share count data."
	yahooFinanceFinancialsDescription        = "Fetch financial calendar data."
	yahooFinanceIncomeStatementDescription   = "Fetch income statement data."
	yahooFinanceBalanceSheetDescription      = "Fetch balance sheet data."
	yahooFinanceCashFlowStatementDescription = "Fetch cash flow statement data."
	yahooFinanceNewsDescription              = "Fetch related financial news."
)

// yahooFinanceParams contains the resolved tool input and Canvas-side switches.
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

type yahooFinanceArgs struct {
	StockCode         string `json:"stock_code"`
	Query             string `json:"query"`
	Info              *bool  `json:"info"`
	History           *bool  `json:"history"`
	Count             *bool  `json:"count"`
	Financials        *bool  `json:"financials"`
	IncomeStmt        *bool  `json:"income_stmt"`
	BalanceSheet      *bool  `json:"balance_sheet"`
	CashFlowStatement *bool  `json:"cash_flow_statement"`
	News              *bool  `json:"news"`
}

type yahooFinanceSearchResponse struct {
	Quotes []map[string]any `json:"quotes"`
	News   []map[string]any `json:"news"`
}

type yahooFinanceChartResponse struct {
	Chart struct {
		Result []struct {
			Meta       map[string]any `json:"meta"`
			Timestamp  []int64        `json:"timestamp"`
			Indicators struct {
				Quote []map[string][]any `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	} `json:"chart"`
}

type yahooFinanceSummaryResponse struct {
	QuoteSummary struct {
		Result []map[string]any `json:"result"`
		Error  any              `json:"error,omitempty"`
	} `json:"quoteSummary"`
}

// yahooFinanceEnvelope is the tool-to-component transport shape.
type yahooFinanceEnvelope struct {
	Report string `json:"report"`
	Error  string `json:"_ERROR,omitempty"`
}

// yahooFinanceSearchEndpoint is the Yahoo Finance search URL

var (
	yahooFinanceSearchEndpoint  = "https://query2.finance.yahoo.com/v1/finance/search"
	yahooFinanceChartEndpoint   = "https://query1.finance.yahoo.com/v8/finance/chart"
	yahooFinanceSummaryEndpoint = "https://query2.finance.yahoo.com/v10/finance/quoteSummary"
	yahooFinanceCookieEndpoint  = "https://fc.yahoo.com"
	yahooFinanceCrumbEndpoint   = "https://query1.finance.yahoo.com/v1/test/getcrumb"
)

// YahooFinanceTool fetches Yahoo Finance stock data via public Yahoo endpoints.
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
			"info": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceInfoDescription,
				Required: false,
			},
			"history": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceHistoryDescription,
				Required: false,
			},
			"count": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceCountDescription,
				Required: false,
			},
			"financials": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceFinancialsDescription,
				Required: false,
			},
			"income_stmt": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceIncomeStatementDescription,
				Required: false,
			},
			"balance_sheet": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceBalanceSheetDescription,
				Required: false,
			},
			"cash_flow_statement": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceCashFlowStatementDescription,
				Required: false,
			},
			"news": {
				Type:     schema.Boolean,
				Desc:     yahooFinanceNewsDescription,
				Required: false,
			},
		}),
	}, nil
}

func (y *YahooFinanceTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"stock_code":          yahooFinanceStockCodeDescription,
			"info":                yahooFinanceInfoDescription,
			"history":             yahooFinanceHistoryDescription,
			"count":               yahooFinanceCountDescription,
			"financials":          yahooFinanceFinancialsDescription,
			"income_stmt":         yahooFinanceIncomeStatementDescription,
			"balance_sheet":       yahooFinanceBalanceSheetDescription,
			"cash_flow_statement": yahooFinanceCashFlowStatementDescription,
			"news":                yahooFinanceNewsDescription,
		},
		Outputs: map[string]string{"report": "Yahoo Finance data formatted as Markdown."},
		InputForm: map[string]any{
			"stock_code": map[string]any{"type": "line", "name": "Stock code/Company name"},
		},
	}
}

// buildYahooFinanceURL composes the search URL
func buildYahooFinanceURL(stockCode string) string {
	q := url.Values{}
	q.Set("q", stockCode)
	q.Set("quotesCount", "1")
	q.Set("newsCount", "10")
	return yahooFinanceSearchEndpoint + "?" + q.Encode()
}

func buildYahooFinanceChartURL(stockCode string) string {
	q := url.Values{}
	q.Set("range", "1mo")
	q.Set("interval", "1d")
	return yahooFinanceChartEndpoint + "/" + url.PathEscape(stockCode) + "?" + q.Encode()
}

func buildYahooFinanceSummaryURL(stockCode string, modules []string, crumb string) string {
	q := url.Values{}
	q.Set("modules", strings.Join(modules, ","))
	q.Set("crumb", crumb)
	return yahooFinanceSummaryEndpoint + "/" + url.PathEscape(stockCode) + "?" + q.Encode()
}

// InvokableRun performs the Yahoo Finance lookup.
func (y *YahooFinanceTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args yahooFinanceArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return yahooFinanceErrJSON(fmt.Errorf("yahoo_finance: parse arguments: %w", err)),
			fmt.Errorf("yahoo_finance: parse arguments: %w", err)
	}
	p := mergeYahooFinanceParams(y.defaults, args)
	p.StockCode = strings.TrimSpace(p.StockCode)
	if p.StockCode == "" || !p.anySectionEnabled() {
		return yahooFinanceJSON(yahooFinanceEnvelope{Report: ""}), nil
	}

	report, err := y.fetchYahooFinanceReport(ctx, p)
	if err != nil {
		return yahooFinanceErrJSON(err), err
	}
	return yahooFinanceJSON(yahooFinanceEnvelope{Report: report}), nil
}

func (y *YahooFinanceTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	report, _ := envelope["report"].(string)
	return map[string]any{"report": report}
}

func mergeYahooFinanceParams(defaults yahooFinanceParams, args yahooFinanceArgs) yahooFinanceParams {
	params := defaults
	params.StockCode = args.StockCode
	if strings.TrimSpace(params.StockCode) == "" {
		params.StockCode = args.Query
	}
	applyBool := func(value *bool, target *bool) {
		if value != nil {
			*target = *value
		}
	}
	applyBool(args.Info, &params.Info)
	applyBool(args.History, &params.History)
	applyBool(args.Count, &params.Count)
	applyBool(args.Financials, &params.Financials)
	applyBool(args.IncomeStmt, &params.IncomeStmt)
	applyBool(args.BalanceSheet, &params.BalanceSheet)
	applyBool(args.CashFlowStatement, &params.CashFlowStatement)
	applyBool(args.News, &params.News)
	return params
}

func (p yahooFinanceParams) anySectionEnabled() bool {
	return p.Info || p.History || p.Count || p.Financials || p.IncomeStmt ||
		p.BalanceSheet || p.CashFlowStatement || p.News
}

func (y *YahooFinanceTool) fetchYahooFinanceReport(ctx context.Context, p yahooFinanceParams) (string, error) {
	var sections []string
	var search *yahooFinanceSearchResponse
	if p.Info || p.News || p.History || len(yahooFinanceSummaryModules(p)) > 0 {
		raw, err := y.fetchYahooFinanceSearch(ctx, p.StockCode)
		if err != nil {
			return "", err
		}
		search = raw
	}
	symbol := yahooFinanceResolvedSymbol(search, p.StockCode)

	var chart *yahooFinanceChartResponse
	if p.Info || p.History {
		raw, err := y.fetchYahooFinanceChart(ctx, symbol)
		if err != nil {
			return "", err
		}
		chart = raw
		if chart.Chart.Error != nil {
			return "", fmt.Errorf("yahoo_finance: upstream chart error: %v", chart.Chart.Error)
		}
	}

	if p.Info {
		sections = append(sections, "# Information:\n"+renderYahooFinanceMap(yahooFinanceInfo(search, chart)))
	}
	if p.History {
		sections = append(sections, "# History:\n"+renderYahooFinanceHistory(chart))
	}
	if p.News {
		sections = append(sections, "# News:\n"+renderYahooFinanceRows(yahooFinanceNews(search)))
	}

	modules := yahooFinanceSummaryModules(p)
	if len(modules) > 0 {
		summary, err := y.fetchYahooFinanceSummary(ctx, symbol, modules)
		if err != nil {
			return "", err
		}
		if summary.QuoteSummary.Error != nil {
			return "", fmt.Errorf("yahoo_finance: upstream summary error: %v", summary.QuoteSummary.Error)
		}
		if len(summary.QuoteSummary.Result) > 0 {
			result := summary.QuoteSummary.Result[0]
			if p.Count {
				sections = append(sections, "# Count:\n"+renderYahooFinanceMap(summaryMap(result, "defaultKeyStatistics")))
			}
			if p.Financials {
				sections = append(sections, "# Calendar:\n"+renderYahooFinanceMap(summaryMap(result, "calendarEvents")))
			}
			if p.IncomeStmt {
				sections = append(sections, "# Income statement:\n"+renderYahooFinanceMap(summaryMap(result, "incomeStatementHistory")))
				sections = append(sections, "# Quarterly income statement:\n"+renderYahooFinanceMap(summaryMap(result, "incomeStatementHistoryQuarterly")))
			}
			if p.BalanceSheet {
				sections = append(sections, "# Balance sheet:\n"+renderYahooFinanceMap(summaryMap(result, "balanceSheetHistory")))
				sections = append(sections, "# Quarterly balance sheet:\n"+renderYahooFinanceMap(summaryMap(result, "balanceSheetHistoryQuarterly")))
			}
			if p.CashFlowStatement {
				sections = append(sections, "# Cash flow statement:\n"+renderYahooFinanceMap(summaryMap(result, "cashflowStatementHistory")))
				sections = append(sections, "# Quarterly cash flow statement:\n"+renderYahooFinanceMap(summaryMap(result, "cashflowStatementHistoryQuarterly")))
			}
		}
	}

	return strings.Join(sections, "\n\n"), nil
}

func yahooFinanceResolvedSymbol(search *yahooFinanceSearchResponse, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if search == nil || len(search.Quotes) == 0 {
		return fallback
	}
	symbol, _ := search.Quotes[0]["symbol"].(string)
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return fallback
	}
	return symbol
}

func (y *YahooFinanceTool) fetchYahooFinanceSearch(ctx context.Context, stockCode string) (*yahooFinanceSearchResponse, error) {
	var raw yahooFinanceSearchResponse
	if err := y.fetchYahooFinanceJSON(ctx, buildYahooFinanceURL(stockCode), &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func (y *YahooFinanceTool) fetchYahooFinanceChart(ctx context.Context, stockCode string) (*yahooFinanceChartResponse, error) {
	var raw yahooFinanceChartResponse
	if err := y.fetchYahooFinanceJSON(ctx, buildYahooFinanceChartURL(stockCode), &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func (y *YahooFinanceTool) fetchYahooFinanceSummary(ctx context.Context, stockCode string, modules []string) (*yahooFinanceSummaryResponse, error) {
	cookieHeader, crumb, err := y.fetchYahooFinanceCrumb(ctx)
	if err != nil {
		return nil, err
	}
	var raw yahooFinanceSummaryResponse
	if err := y.fetchYahooFinanceJSONWithHeaders(ctx, buildYahooFinanceSummaryURL(stockCode, modules, crumb), &raw, map[string]string{
		"Cookie": cookieHeader,
	}); err != nil {
		return nil, err
	}
	return &raw, nil
}

func (y *YahooFinanceTool) fetchYahooFinanceJSON(ctx context.Context, endpoint string, target any) error {
	return y.fetchYahooFinanceJSONWithHeaders(ctx, endpoint, target, nil)
}

func (y *YahooFinanceTool) fetchYahooFinanceJSONWithHeaders(ctx context.Context, endpoint string, target any, extraHeaders map[string]string) error {
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "application/json",
	}
	for key, value := range extraHeaders {
		if value != "" {
			headers[key] = value
		}
	}
	resp, err := y.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("yahoo_finance: upstream returned %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("yahoo_finance: decode response: %w", err)
	}
	return nil
}

func (y *YahooFinanceTool) fetchYahooFinanceCrumb(ctx context.Context) (string, string, error) {
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "text/plain,*/*",
	}
	resp, err := y.helper.Do(ctx, http.MethodGet, yahooFinanceCookieEndpoint, "", "", headers)
	if err != nil {
		return "", "", err
	}
	cookies := resp.Cookies()
	_ = resp.Body.Close()
	cookieHeader := yahooFinanceCookieHeader(cookies)
	if cookieHeader == "" {
		return "", "", fmt.Errorf("yahoo_finance: missing Yahoo cookie")
	}

	headers["Cookie"] = cookieHeader
	resp, err = y.helper.Do(ctx, http.MethodGet, yahooFinanceCrumbEndpoint, "", "", headers)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("yahoo_finance: crumb endpoint returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("yahoo_finance: read crumb: %w", err)
	}
	crumb := strings.TrimSpace(string(body))
	if crumb == "" {
		return "", "", fmt.Errorf("yahoo_finance: empty crumb")
	}
	return cookieHeader, crumb, nil
}

func yahooFinanceCookieHeader(cookies []*http.Cookie) string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		values = append(values, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(values, "; ")
}

func yahooFinanceSummaryModules(p yahooFinanceParams) []string {
	var modules []string
	if p.Count {
		modules = append(modules, "defaultKeyStatistics")
	}
	if p.Financials {
		modules = append(modules, "calendarEvents")
	}
	if p.IncomeStmt {
		modules = append(modules, "incomeStatementHistory", "incomeStatementHistoryQuarterly")
	}
	if p.BalanceSheet {
		modules = append(modules, "balanceSheetHistory", "balanceSheetHistoryQuarterly")
	}
	if p.CashFlowStatement {
		modules = append(modules, "cashflowStatementHistory", "cashflowStatementHistoryQuarterly")
	}
	return modules
}

func yahooFinanceInfo(search *yahooFinanceSearchResponse, chart *yahooFinanceChartResponse) map[string]any {
	info := map[string]any{}
	if search != nil && len(search.Quotes) > 0 {
		for key, value := range search.Quotes[0] {
			info[key] = value
		}
	}
	if chart != nil && len(chart.Chart.Result) > 0 {
		for key, value := range chart.Chart.Result[0].Meta {
			info[key] = value
		}
	}
	return info
}

func yahooFinanceNews(search *yahooFinanceSearchResponse) []map[string]any {
	if search == nil {
		return nil
	}
	return search.News
}

func summaryMap(result map[string]any, key string) map[string]any {
	value, ok := result[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func renderYahooFinanceMap(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	scalars := make([]string, 0, len(keys))
	tables := make([]string, 0)
	for _, key := range keys {
		if rows := yahooFinanceRows(values[key]); len(rows) > 0 {
			tables = append(tables, "### "+markdownCell(key)+"\n"+renderYahooFinanceTable(rows))
			continue
		}
		scalars = append(scalars, fmt.Sprintf("| %s | %s |", markdownCell(key), markdownCell(yahooFinanceValue(values[key]))))
	}

	sections := make([]string, 0, 1+len(tables))
	if len(scalars) > 0 {
		rows := []string{"| | 0 |", "|:---|:---|"}
		rows = append(rows, scalars...)
		sections = append(sections, strings.Join(rows, "\n"))
	}
	sections = append(sections, tables...)
	return strings.Join(sections, "\n\n")
}

func renderYahooFinanceRows(values []map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	return renderYahooFinanceTable(values)
}

func yahooFinanceRows(value any) []map[string]any {
	switch rows := value.(type) {
	case []map[string]any:
		return rows
	case []any:
		result := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			asMap, ok := row.(map[string]any)
			if !ok {
				return nil
			}
			result = append(result, asMap)
		}
		return result
	default:
		return nil
	}
}

func renderYahooFinanceTable(values []map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	keys := yahooFinanceTableKeys(values)
	if len(keys) == 0 {
		return ""
	}

	rows := []string{"| " + strings.Join(keys, " | ") + " |"}
	align := make([]string, len(keys))
	for i := range align {
		align[i] = ":---"
	}
	rows = append(rows, "|"+strings.Join(align, "|")+"|")
	for _, value := range values {
		cells := make([]string, 0, len(keys))
		for _, key := range keys {
			cells = append(cells, markdownCell(yahooFinanceValue(value[key])))
		}
		rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(rows, "\n")
}

func yahooFinanceTableKeys(values []map[string]any) []string {
	seen := map[string]bool{}
	keys := make([]string, 0)
	for _, value := range values {
		rowKeys := make([]string, 0, len(value))
		for key := range value {
			if !seen[key] {
				rowKeys = append(rowKeys, key)
			}
		}
		sort.Strings(rowKeys)
		for _, key := range rowKeys {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

func renderYahooFinanceHistory(chart *yahooFinanceChartResponse) string {
	if chart == nil || len(chart.Chart.Result) == 0 || len(chart.Chart.Result[0].Indicators.Quote) == 0 {
		return ""
	}
	result := chart.Chart.Result[0]
	quotes := result.Indicators.Quote[0]
	keys := make([]string, 0, len(quotes))
	for key := range quotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := []string{"| timestamp | " + strings.Join(keys, " | ") + " |"}
	align := []string{":---"}
	for range keys {
		align = append(align, ":---")
	}
	rows = append(rows, "|"+strings.Join(align, "|")+"|")
	for i, ts := range result.Timestamp {
		cells := []string{fmt.Sprint(ts)}
		for _, key := range keys {
			values := quotes[key]
			if i >= len(values) {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, yahooFinanceValue(values[i]))
		}
		rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(rows, "\n")
}

func yahooFinanceValue(value any) string {
	if value == nil {
		return "None"
	}
	if text, ok := value.(string); ok {
		return text
	}
	if formatted, ok := yahooFinanceFormattedValue(value); ok {
		return formatted
	}
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case map[string]any:
		return yahooFinanceMapValue(typed)
	case []any:
		return yahooFinanceArrayValue(typed)
	case []string:
		return strings.Join(typed, ", ")
	}
	return fmt.Sprint(value)
}

func yahooFinanceFormattedValue(value any) (string, bool) {
	fields, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	if text, ok := fields["fmt"].(string); ok {
		return text, true
	}
	if text, ok := fields["longFmt"].(string); ok {
		return text, true
	}
	raw, ok := fields["raw"]
	if !ok || len(fields) > 3 {
		return "", false
	}
	return yahooFinanceValue(raw), true
}

func yahooFinanceMapValue(fields map[string]any) string {
	if len(fields) == 0 {
		return "None"
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+yahooFinanceValue(fields[key]))
	}
	return strings.Join(parts, "; ")
}

func yahooFinanceArrayValue(values []any) string {
	if len(values) == 0 {
		return "None"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, yahooFinanceValue(value))
	}
	return strings.Join(parts, ", ")
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
