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

const yahooFinanceToolName = "yahoo_finance"

const yahooFinanceToolDescription = "Fetch stock quote snapshots from Yahoo Finance. Returns quoteResponse.result[].{symbol, regularMarketPrice, currency, regularMarketChangePercent}."

// yahooFinanceParams is the JSON shape the model sends into InvokableRun.
type yahooFinanceParams struct {
	Symbols []string `json:"symbols"`
	Fields  []string `json:"fields"`
}

// yahooFinanceQuote is one element of the upstream result array.
type yahooFinanceQuote struct {
	Symbol                     string  `json:"symbol"`
	RegularMarketPrice         float64 `json:"regularMarketPrice"`
	Currency                   string  `json:"currency"`
	RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
}

// yahooFinanceResponse is the upstream Yahoo Finance /v7/finance/quote
// envelope.
type yahooFinanceResponse struct {
	QuoteResponse struct {
		Result []yahooFinanceQuote `json:"result"`
		Error  any                 `json:"error,omitempty"`
	} `json:"quoteResponse"`
}

// yahooFinanceEnvelope is what the model sees.
type yahooFinanceEnvelope struct {
	Results []yahooFinanceQuote `json:"results"`
	Error   string              `json:"_ERROR,omitempty"`
}

// yahooFinanceEndpoint is the Yahoo Finance quote URL. Exposed as a
// package var so tests can substitute a httptest.Server URL.
var yahooFinanceEndpoint = "https://query1.finance.yahoo.com/v7/finance/quote"

// YahooFinanceTool is the
// Yahoo Finance quote tool.
// It performs an unauthenticated GET against the public quote API
// via the shared HTTPHelper and returns the parsed quote records.
type YahooFinanceTool struct {
	helper *HTTPHelper
}

// NewYahooFinanceTool returns a YahooFinanceTool using the default
// HTTPHelper.
func NewYahooFinanceTool() *YahooFinanceTool {
	return NewYahooFinanceToolWith(NewHTTPHelper())
}

// NewYahooFinanceToolWith returns a YahooFinanceTool that uses the
// provided HTTPHelper. Useful for tests.
func NewYahooFinanceToolWith(h *HTTPHelper) *YahooFinanceTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &YahooFinanceTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (y *YahooFinanceTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: yahooFinanceToolName,
		Desc: yahooFinanceToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"symbols": {
				Type:     schema.Array,
				Desc:     "Stock symbols to look up (e.g. AAPL, MSFT, 0005.HK).",
				Required: true,
			},
			"fields": {
				Type:     schema.Array,
				Desc:     "Optional list of fields to request via the `fields` query parameter.",
				Required: false,
			},
		}),
	}, nil
}

// buildYahooFinanceURL composes the quote URL with the symbol list
// and an optional `fields` parameter. Centralized for testability.
func buildYahooFinanceURL(symbols []string, fields []string) string {
	q := url.Values{}
	q.Set("symbols", strings.Join(symbols, ","))
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	return yahooFinanceEndpoint + "?" + q.Encode()
}

// InvokableRun performs the Yahoo Finance quote lookup.
func (y *YahooFinanceTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p yahooFinanceParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return yahooFinanceErrJSON(fmt.Errorf("yahoo_finance: parse arguments: %w", err)),
			fmt.Errorf("yahoo_finance: parse arguments: %w", err)
	}
	if len(p.Symbols) == 0 {
		return yahooFinanceErrJSON(fmt.Errorf("symbols is required and must be non-empty")),
			fmt.Errorf("yahoo_finance: symbols is required and must be non-empty")
	}

	endpoint := buildYahooFinanceURL(p.Symbols, p.Fields)
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
	return yahooFinanceJSON(yahooFinanceEnvelope{Results: raw.QuoteResponse.Result}), nil
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
