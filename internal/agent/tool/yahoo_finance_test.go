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

func TestYahooFinance_BuildURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		symbols     []string
		fields      []string
		wantSymbols string
		wantFields  string
		wantHost    string
	}{
		{
			name:        "single symbol, no fields",
			symbols:     []string{"AAPL"},
			fields:      nil,
			wantSymbols: "AAPL",
			wantHost:    "query1.finance.yahoo.com",
		},
		{
			name:        "multi symbol, with fields",
			symbols:     []string{"AAPL", "MSFT", "0005.HK"},
			fields:      []string{"symbol", "regularMarketPrice"},
			wantSymbols: "AAPL,MSFT,0005.HK",
			wantFields:  "symbol,regularMarketPrice",
			wantHost:    "query1.finance.yahoo.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildYahooFinanceURL(tc.symbols, tc.fields)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "/v7/finance/quote" {
				t.Errorf("path = %q, want /v7/finance/quote", u.Path)
			}
			q := u.Query()
			if q.Get("symbols") != tc.wantSymbols {
				t.Errorf("symbols = %q, want %q", q.Get("symbols"), tc.wantSymbols)
			}
			if tc.wantFields != "" && q.Get("fields") != tc.wantFields {
				t.Errorf("fields = %q, want %q", q.Get("fields"), tc.wantFields)
			}
		})
	}
}

func TestYahooFinance_ParseQuote(t *testing.T) {
	t.Parallel()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"quoteResponse": {
				"result": [
					{"symbol":"AAPL","regularMarketPrice":189.5,"currency":"USD","regularMarketChangePercent":1.23},
					{"symbol":"MSFT","regularMarketPrice":421.0,"currency":"USD","regularMarketChangePercent":-0.5}
				]
			}
		}`))
	}))
	defer srv.Close()

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewYahooFinanceToolWith(helper)
	out, err := tool.InvokableRun(context.Background(),
		`{"symbols":["AAPL","MSFT"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(gotUA, "ragflow") {
		t.Errorf("User-Agent = %q, want to contain ragflow", gotUA)
	}

	var env yahooFinanceEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(env.Results))
	}
	if env.Results[0].Symbol != "AAPL" {
		t.Errorf("Results[0].Symbol = %q, want AAPL", env.Results[0].Symbol)
	}
	if env.Results[0].RegularMarketPrice != 189.5 {
		t.Errorf("Results[0].Price = %v, want 189.5", env.Results[0].RegularMarketPrice)
	}
	if env.Results[1].RegularMarketChangePercent != -0.5 {
		t.Errorf("Results[1].ChangePct = %v, want -0.5", env.Results[1].RegularMarketChangePercent)
	}
}

func TestYahooFinance_RequiresSymbols(t *testing.T) {
	t.Parallel()

	tool := NewYahooFinanceTool()
	_, err := tool.InvokableRun(context.Background(), `{"symbols":[]}`)
	if err == nil {
		t.Fatal("expected error for empty symbols")
	}
	if !strings.Contains(err.Error(), "symbols") {
		t.Errorf("err = %v, want to mention symbols", err)
	}
}

func TestYahooFinance_Info(t *testing.T) {
	t.Parallel()

	tool := NewYahooFinanceTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "yahoo_finance" {
		t.Errorf("Name = %q, want yahoo_finance", info.Name)
	}
	if !strings.Contains(info.Desc, "Yahoo") {
		t.Errorf("Desc = %q, want to mention Yahoo", info.Desc)
	}
}
