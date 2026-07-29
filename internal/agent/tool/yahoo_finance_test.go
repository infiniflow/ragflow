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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestYahooFinanceBuildURL(t *testing.T) {
	t.Parallel()

	got := buildYahooFinanceURL("0005.HK")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if parsed.Host != "query2.finance.yahoo.com" {
		t.Fatalf("host = %q", parsed.Host)
	}
	if parsed.Path != "/v1/finance/search" {
		t.Fatalf("path = %q", parsed.Path)
	}
	if q := parsed.Query().Get("q"); q != "0005.HK" {
		t.Fatalf("q = %q", q)
	}
	if quotesCount := parsed.Query().Get("quotesCount"); quotesCount != "1" {
		t.Fatalf("quotesCount = %q", quotesCount)
	}
}

func TestYahooFinanceInvokableRunBuildsMarkdownReport(t *testing.T) {
	t.Parallel()

	var gotQuery, gotUserAgent string
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPaths = append(gotPaths, request.URL.Path)
		gotUserAgent = request.Header.Get("User-Agent")
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/finance/search":
			gotQuery = request.URL.Query().Get("q")
			_, _ = writer.Write([]byte(`{
				"quotes": [{
					"symbol":"AAPL",
					"currency":"USD",
					"note":"left|right"
				}],
				"news": [{
					"title":"Apple market update",
					"publisher":"Example"
				}]
			}`))
		case "/v8/finance/chart/AAPL":
			_, _ = writer.Write([]byte(`{
				"chart": {
					"result": [{
						"meta": {
							"regularMarketPrice":189.5,
							"marketState":"REGULAR"
						},
						"timestamp": [1710000000],
						"indicators": {"quote": [{"close": [189.5]}]}
					}],
					"error": null
				}
			}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})
	raw, err := NewYahooFinanceToolWith(helper).InvokableRun(context.Background(), `{"stock_code":" AAPL "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotQuery != "AAPL" {
		t.Fatalf("q = %q", gotQuery)
	}
	if strings.Join(gotPaths, ",") != "/v1/finance/search,/v8/finance/chart/AAPL" {
		t.Fatalf("paths = %#v", gotPaths)
	}
	if !strings.Contains(gotUserAgent, "ragflow") {
		t.Fatalf("User-Agent = %q", gotUserAgent)
	}

	var envelope yahooFinanceEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal output %q: %v", raw, err)
	}
	for _, expected := range []string{
		"# Information:",
		"| currency | USD |",
		"| marketState | REGULAR |",
		`| note | left\|right |`,
		"| regularMarketPrice | 189.5 |",
		"| symbol | AAPL |",
		"# News:",
		"| publisher | title |",
		"| Example | Apple market update |",
	} {
		if !strings.Contains(envelope.Report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, envelope.Report)
		}
	}
	if envelope.Error != "" {
		t.Fatalf("unexpected error = %q", envelope.Error)
	}
}

func TestYahooFinanceInvokableRunAcceptsQueryAlias(t *testing.T) {
	t.Parallel()

	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/finance/search":
			gotQuery = request.URL.Query().Get("q")
			_, _ = writer.Write([]byte(`{"quotes":[{"symbol":"3800.HK"}],"news":[]}`))
		case "/v8/finance/chart/3800.HK":
			_, _ = writer.Write([]byte(`{
				"chart": {
					"result": [{
						"meta": {"regularMarketPrice": 0.6},
						"timestamp": [],
						"indicators": {"quote": [{}]}
					}],
					"error": null
				}
			}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})
	raw, err := NewYahooFinanceToolWith(helper).InvokableRun(context.Background(), `{"query":"3800.HK"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotQuery != "3800.HK" {
		t.Fatalf("q = %q", gotQuery)
	}
	var envelope yahooFinanceEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal output %q: %v", raw, err)
	}
	if !strings.Contains(envelope.Report, "| symbol | 3800.HK |") ||
		!strings.Contains(envelope.Report, "| regularMarketPrice | 0.6 |") {
		t.Fatalf("report = %s", envelope.Report)
	}
}

func TestYahooFinanceUsesSearchResolvedSymbolForDownstreamRequests(t *testing.T) {
	t.Parallel()

	var paths []string
	var gotSearchQuery string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/finance/search":
			gotSearchQuery = request.URL.Query().Get("q")
			_, _ = writer.Write([]byte(`{"quotes":[{"symbol":"AAPL","longname":"Apple Inc."}],"news":[]}`))
		case "/v8/finance/chart/AAPL":
			_, _ = writer.Write([]byte(`{
				"chart": {
					"result": [{
						"meta": {"regularMarketPrice": 189.5},
						"timestamp": [],
						"indicators": {"quote": [{}]}
					}],
					"error": null
				}
			}`))
		case "/":
			http.SetCookie(writer, &http.Cookie{Name: "A1", Value: "cookie-value"})
			writer.WriteHeader(http.StatusNotFound)
		case "/v1/test/getcrumb":
			if cookie := request.Header.Get("Cookie"); cookie != "A1=cookie-value" {
				t.Fatalf("crumb Cookie = %q", cookie)
			}
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte("crumb-value"))
		case "/v10/finance/quoteSummary/AAPL":
			if cookie := request.Header.Get("Cookie"); cookie != "A1=cookie-value" {
				t.Fatalf("summary Cookie = %q", cookie)
			}
			if crumb := request.URL.Query().Get("crumb"); crumb != "crumb-value" {
				t.Fatalf("crumb = %q", crumb)
			}
			_, _ = writer.Write([]byte(`{
				"quoteSummary": {
					"result": [{
						"defaultKeyStatistics": {
							"sharesOutstanding": {"raw": 14687356000, "fmt": "14.69B"}
						}
					}],
					"error": null
				}
			}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})
	yahoo := NewYahooFinanceToolWithDefaults(helper, yahooFinanceParams{Info: true, Count: true})
	raw, err := yahoo.InvokableRun(context.Background(), `{"stock_code":"Apple"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotSearchQuery != "Apple" {
		t.Fatalf("search q = %q", gotSearchQuery)
	}
	gotPaths := strings.Join(paths, ",")
	if strings.Contains(gotPaths, "/Apple") {
		t.Fatalf("downstream paths used unresolved company name: %v", paths)
	}
	for _, expected := range []string{"/v1/finance/search", "/v8/finance/chart/AAPL", "/v10/finance/quoteSummary/AAPL"} {
		if !strings.Contains(gotPaths, expected) {
			t.Fatalf("paths missing %s: %v", expected, paths)
		}
	}
	var envelope yahooFinanceEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal output %q: %v", raw, err)
	}
	if !strings.Contains(envelope.Report, "| longname | Apple Inc. |") ||
		!strings.Contains(envelope.Report, "| sharesOutstanding | 14.69B |") {
		t.Fatalf("report = %s", envelope.Report)
	}
}

func TestYahooFinanceEmptyStockCodeSkipsRequest(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})

	for _, args := range []string{`{"stock_code":""}`, `{"stock_code":"   "}`} {
		raw, err := NewYahooFinanceToolWith(helper).InvokableRun(context.Background(), args)
		if err != nil {
			t.Fatalf("InvokableRun(%s): %v", args, err)
		}
		var envelope yahooFinanceEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if envelope.Report != "" || envelope.Error != "" {
			t.Fatalf("empty input output = %#v", envelope)
		}
	}
	if calls != 0 {
		t.Fatalf("server calls = %d, want 0", calls)
	}
}

func TestYahooFinanceAllSectionsDisabledSkipsRequest(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})
	yahoo := NewYahooFinanceToolWithDefaults(helper, yahooFinanceParams{})

	raw, err := yahoo.InvokableRun(context.Background(), `{"stock_code":"AAPL"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var envelope yahooFinanceEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if envelope.Report != "" || calls != 0 {
		t.Fatalf("output = %#v, server calls = %d", envelope, calls)
	}
}

func TestYahooFinanceSummaryUsesCrumbAndCookie(t *testing.T) {
	t.Parallel()

	var sawCookieOnCrumb, sawCookieOnSummary bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/finance/search":
			_, _ = writer.Write([]byte(`{"quotes":[{"symbol":"AAPL"}],"news":[]}`))
		case "/":
			http.SetCookie(writer, &http.Cookie{Name: "A1", Value: "cookie-value"})
			writer.WriteHeader(http.StatusNotFound)
		case "/v1/test/getcrumb":
			sawCookieOnCrumb = request.Header.Get("Cookie") == "A1=cookie-value"
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte("crumb-value"))
		case "/v10/finance/quoteSummary/AAPL":
			sawCookieOnSummary = request.Header.Get("Cookie") == "A1=cookie-value"
			if crumb := request.URL.Query().Get("crumb"); crumb != "crumb-value" {
				t.Fatalf("crumb = %q", crumb)
			}
			if modules := request.URL.Query().Get("modules"); modules != "defaultKeyStatistics" {
				t.Fatalf("modules = %q", modules)
			}
			_, _ = writer.Write([]byte(`{
				"quoteSummary": {
					"result": [{
						"defaultKeyStatistics": {
							"sharesOutstanding": {"raw": 14687356000, "fmt": "14.69B"}
						}
					}],
					"error": null
				}
			}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})
	yahoo := NewYahooFinanceToolWithDefaults(helper, yahooFinanceParams{Count: true})
	raw, err := yahoo.InvokableRun(context.Background(), `{"stock_code":"AAPL"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !sawCookieOnCrumb || !sawCookieOnSummary {
		t.Fatalf("cookie propagation crumb=%v summary=%v", sawCookieOnCrumb, sawCookieOnSummary)
	}
	var envelope yahooFinanceEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !strings.Contains(envelope.Report, "# Count:") ||
		!strings.Contains(envelope.Report, "| sharesOutstanding | 14.69B |") {
		t.Fatalf("report = %s", envelope.Report)
	}
}

func TestRenderYahooFinanceMapFormatsNestedRows(t *testing.T) {
	t.Parallel()

	report := renderYahooFinanceMap(map[string]any{
		"maxAge": float64(86400),
		"cashflowStatements": []any{
			map[string]any{
				"endDate":   map[string]any{"fmt": "2025-12-31", "raw": float64(1767139200)},
				"maxAge":    float64(1),
				"netIncome": map[string]any{"fmt": "-2.87B", "longFmt": "-2,867,891,000", "raw": float64(-2867891000)},
			},
			map[string]any{
				"endDate":   map[string]any{"fmt": "2024-12-31", "raw": float64(1735603200)},
				"maxAge":    float64(1),
				"netIncome": map[string]any{"fmt": "-4.75B", "longFmt": "-4,750,396,000", "raw": float64(-4750396000)},
			},
		},
	})

	for _, expected := range []string{
		"| maxAge | 86400 |",
		"### cashflowStatements",
		"| endDate | maxAge | netIncome |",
		"| 2025-12-31 | 1 | -2.87B |",
		"| 2024-12-31 | 1 | -4.75B |",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, report)
		}
	}
	if strings.Contains(report, `"raw"`) || strings.Contains(report, `"fmt"`) {
		t.Fatalf("report leaked JSON internals:\n%s", report)
	}
}

func TestYahooFinanceValueFormatsNestedObjectsWithoutJSON(t *testing.T) {
	t.Parallel()

	report := renderYahooFinanceMap(map[string]any{
		"currentTradingPeriod": map[string]any{
			"regular": map[string]any{"start": float64(1784251800), "end": float64(1784275800), "timezone": "HKT"},
		},
		"validRanges": []any{"1d", "5d", "1mo"},
		"thumbnail": map[string]any{
			"resolutions": []any{
				map[string]any{"tag": "original", "url": "https://example.test/original.png", "width": float64(768)},
			},
		},
	})

	for _, expected := range []string{
		"| currentTradingPeriod | regular: end: 1784275800; start: 1784251800; timezone: HKT |",
		"| validRanges | 1d, 5d, 1mo |",
		"| thumbnail | resolutions: tag: original; url: https://example.test/original.png; width: 768 |",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, report)
		}
	}
	for _, leaked := range []string{`{"`, `":`, `["`, `"]`} {
		if strings.Contains(report, leaked) {
			t.Fatalf("report leaked JSON token %q:\n%s", leaked, report)
		}
	}
}

func TestMergeYahooFinanceParamsKeepsStockCodeAndUsesNodeConfig(t *testing.T) {
	t.Parallel()

	defaults := yahooFinanceParams{Info: true, News: true}
	info := false
	history := true
	args := yahooFinanceArgs{
		StockCode: "AAPL",
		Info:      &info,
		History:   &history,
	}

	got := mergeYahooFinanceParams(defaults, args)
	want := defaults
	want.StockCode = "AAPL"
	want.Info = false
	want.History = true
	if got != want {
		t.Fatalf("merged params = %#v, want %#v", got, want)
	}
}

func TestYahooFinanceErrorsReturnEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
		chartError bool
	}{
		{name: "http status", statusCode: http.StatusUnauthorized, body: `denied`, wantError: "upstream returned 401"},
		{name: "invalid json", statusCode: http.StatusOK, body: `{`, wantError: "decode response"},
		{name: "upstream envelope", statusCode: http.StatusOK, body: `{"chart":{"result":[],"error":{"code":"Not Found"}}}`, wantError: "upstream chart error", chartError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(test.statusCode)
				if test.chartError && request.URL.Path == "/v1/finance/search" {
					_, _ = writer.Write([]byte(`{"quotes":[{"symbol":"AAPL"}],"news":[]}`))
					return
				}
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			helper := NewHTTPHelperWithRetry(RetryConfig{MaxAttempts: 1}).WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})

			raw, err := NewYahooFinanceToolWith(helper).InvokableRun(context.Background(), `{"stock_code":"AAPL"}`)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("err = %v, want %q", err, test.wantError)
			}
			var envelope yahooFinanceEnvelope
			if decodeErr := json.Unmarshal([]byte(raw), &envelope); decodeErr != nil {
				t.Fatalf("error result is not JSON: %s: %v", raw, decodeErr)
			}
			if !strings.Contains(envelope.Error, test.wantError) || envelope.Report != "" {
				t.Fatalf("envelope = %#v", envelope)
			}
		})
	}
}

func TestYahooFinanceMalformedArguments(t *testing.T) {
	t.Parallel()

	raw, err := NewYahooFinanceTool().InvokableRun(context.Background(), `{`)
	if err == nil || !strings.Contains(err.Error(), "parse arguments") {
		t.Fatalf("err = %v", err)
	}
	var envelope yahooFinanceEnvelope
	if decodeErr := json.Unmarshal([]byte(raw), &envelope); decodeErr != nil {
		t.Fatalf("error result is not JSON: %s: %v", raw, decodeErr)
	}
	if !strings.Contains(envelope.Error, "parse arguments") {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestYahooFinanceComponentContract(t *testing.T) {
	t.Parallel()

	yahoo := NewYahooFinanceTool()
	spec := yahoo.ComponentSpec()
	stockCode, ok := spec.InputForm["stock_code"].(map[string]any)
	if !ok || stockCode["type"] != "line" || stockCode["name"] != "Stock code/Company name" {
		t.Fatalf("stock_code input form = %#v", spec.InputForm["stock_code"])
	}
	if _, exists := spec.Outputs["report"]; !exists {
		t.Fatalf("outputs = %#v", spec.Outputs)
	}
	outputs := yahoo.BuildComponentOutputs(map[string]any{"report": "# Information:\nAAPL"})
	if report, ok := outputs["report"].(string); !ok || report != "# Information:\nAAPL" {
		t.Fatalf("report = %#v", outputs["report"])
	}
}

func TestYahooFinanceInfoExposesPythonCompatibleParams(t *testing.T) {
	t.Parallel()

	info, err := NewYahooFinanceTool().Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "yahoo_finance" || info.Desc != yahooFinanceToolDescription {
		t.Fatalf("Info = name %q, desc %q", info.Name, info.Desc)
	}
	jsonSchema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, err := json.Marshal(jsonSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	schemaText := string(raw)
	for _, expected := range []string{
		`"stock_code"`,
		`"required":["stock_code"]`,
		`"info"`,
		`"history"`,
		`"count"`,
		`"financials"`,
		`"income_stmt"`,
		`"balance_sheet"`,
		`"cash_flow_statement"`,
		`"news"`,
	} {
		if !strings.Contains(schemaText, expected) {
			t.Fatalf("schema missing %s: %s", expected, schemaText)
		}
	}
	for _, leaked := range []string{`"symbols"`, `"fields"`} {
		if strings.Contains(schemaText, leaked) {
			t.Fatalf("schema leaked upstream field %s: %s", leaked, schemaText)
		}
	}
}

func TestBuildYahooFinanceToolUsesNodeDefaults(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("yahoo_finance", map[string]any{
		"info": false, "history": true, "balance_sheet": true, "news": true,
		"stock_code": "sys.query", "outputs": map[string]any{"report": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	yahoo, ok := built.(*YahooFinanceTool)
	if !ok {
		t.Fatalf("tool type = %T", built)
	}
	if yahoo.defaults.Info {
		t.Fatalf("defaults = %#v", yahoo.defaults)
	}
	if !yahoo.defaults.History || !yahoo.defaults.BalanceSheet || !yahoo.defaults.News {
		t.Fatalf("defaults = %#v", yahoo.defaults)
	}
	if yahoo.defaults.StockCode != "" {
		t.Fatalf("runtime stock_code leaked into defaults: %#v", yahoo.defaults)
	}
}

func TestBuildYahooFinanceToolRejectsInvalidInfoParam(t *testing.T) {
	t.Parallel()

	_, err := BuildByName("yahoo_finance", map[string]any{"info": "true"})
	if err == nil || !strings.Contains(err.Error(), "requires boolean node-level param info") {
		t.Fatalf("err = %v", err)
	}
}
