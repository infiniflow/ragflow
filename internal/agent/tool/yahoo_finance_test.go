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
	if parsed.Host != "query1.finance.yahoo.com" {
		t.Fatalf("host = %q", parsed.Host)
	}
	if parsed.Path != "/v7/finance/quote" {
		t.Fatalf("path = %q", parsed.Path)
	}
	if symbols := parsed.Query().Get("symbols"); symbols != "0005.HK" {
		t.Fatalf("symbols = %q", symbols)
	}
	if _, exists := parsed.Query()["fields"]; exists {
		t.Fatalf("unexpected legacy fields query: %s", parsed.RawQuery)
	}
}

func TestYahooFinanceInvokableRunBuildsMarkdownReport(t *testing.T) {
	t.Parallel()

	var gotSymbols, gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotSymbols = request.URL.Query().Get("symbols")
		gotUserAgent = request.Header.Get("User-Agent")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"quoteResponse": {
				"result": [{
					"symbol":"AAPL",
					"regularMarketPrice":189.5,
					"currency":"USD",
					"marketState":"REGULAR",
					"note":"left|right"
				}],
				"error": null
			}
		}`))
	}))
	defer server.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{Transport: rewriteHostTransport(server.URL)})
	raw, err := NewYahooFinanceToolWith(helper).InvokableRun(context.Background(), `{"stock_code":" AAPL "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotSymbols != "AAPL" {
		t.Fatalf("symbols = %q", gotSymbols)
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
	} {
		if !strings.Contains(envelope.Report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, envelope.Report)
		}
	}
	if envelope.Error != "" {
		t.Fatalf("unexpected error = %q", envelope.Error)
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

func TestMergeYahooFinanceParamsKeepsStockCodeAndUsesNodeConfig(t *testing.T) {
	t.Parallel()

	defaults := yahooFinanceParams{Info: true}
	params := yahooFinanceParams{
		StockCode: "AAPL",
		Info:      false,
	}

	got := mergeYahooFinanceParams(defaults, params)
	want := defaults
	want.StockCode = "AAPL"
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
	}{
		{name: "http status", statusCode: http.StatusUnauthorized, body: `denied`, wantError: "upstream returned 401"},
		{name: "invalid json", statusCode: http.StatusOK, body: `{`, wantError: "decode response"},
		{name: "upstream envelope", statusCode: http.StatusOK, body: `{"quoteResponse":{"result":[],"error":{"code":"Not Found"}}}`, wantError: "upstream error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.statusCode)
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

func TestYahooFinanceInfoOnlyExposesStockCode(t *testing.T) {
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
	for _, expected := range []string{`"stock_code"`, `"required":["stock_code"]`} {
		if !strings.Contains(schemaText, expected) {
			t.Fatalf("schema missing %s: %s", expected, schemaText)
		}
	}
	for _, leaked := range []string{`"symbols"`, `"fields"`, `"info"`, `"news"`} {
		if strings.Contains(schemaText, leaked) {
			t.Fatalf("schema leaked node config %s: %s", leaked, schemaText)
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
