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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTushare_BuildRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		params       tushareParams
		wantToken    string
		wantAPIName  string
		wantFieldKey string
		wantFieldVal string
	}{
		{
			name: "minimal — token + api_name",
			params: tushareParams{
				Token:   "T-abc",
				APIName: "stock_basic",
			},
			wantToken:   "T-abc",
			wantAPIName: "stock_basic",
		},
		{
			name: "with params and fields",
			params: tushareParams{
				Token:   "T-xyz",
				APIName: "daily",
				Params:  map[string]string{"ts_code": "000001.SZ", "start_date": "20240101"},
				Fields:  []string{"ts_code", "trade_date", "close"},
			},
			wantToken:    "T-xyz",
			wantAPIName:  "daily",
			wantFieldKey: "ts_code",
			wantFieldVal: "000001.SZ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, err := buildTushareRequestBody(tc.params)
			if err != nil {
				t.Fatalf("buildTushareRequestBody: %v", err)
			}
			var got tushareRequest
			if jerr := json.Unmarshal(body, &got); jerr != nil {
				t.Fatalf("request body is not valid JSON: %v (raw=%s)", jerr, body)
			}
			if got.Token != tc.wantToken {
				t.Errorf("token = %q, want %q", got.Token, tc.wantToken)
			}
			if got.APIName != tc.wantAPIName {
				t.Errorf("api_name = %q, want %q", got.APIName, tc.wantAPIName)
			}
			if tc.wantFieldKey != "" {
				if v, ok := got.Params[tc.wantFieldKey]; !ok || v != tc.wantFieldVal {
					t.Errorf("params[%q] = %q (present=%v), want %q", tc.wantFieldKey, v, ok, tc.wantFieldVal)
				}
			}
		})
	}
}

func TestTushare_ParseResponse(t *testing.T) {
	t.Parallel()

	var (
		gotMethod  string
		gotCT      string
		gotBodyRaw []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotBodyRaw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"msg":  "",
			"data": {
				"fields": ["ts_code", "name", "industry"],
				"items":  [
					["000001.SZ", "平安银行", "银行"],
					["600519.SH", "贵州茅台", "白酒"]
				]
			}
		}`))
	}))
	defer srv.Close()

	prev := tushareEndpoint
	tushareEndpoint = srv.URL
	t.Cleanup(func() { tushareEndpoint = prev })

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTushareToolWith(helper)

	out, err := tool.InvokableRun(context.Background(),
		`{"token":"T-test","api_name":"stock_basic","params":{"list_status":"L"}}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.Contains(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}

	// Verify the request body had the right shape.
	var sentReq tushareRequest
	if jerr := json.Unmarshal(gotBodyRaw, &sentReq); jerr != nil {
		t.Fatalf("server saw malformed request body: %v (raw=%s)", jerr, gotBodyRaw)
	}
	if sentReq.Token != "T-test" {
		t.Errorf("server saw token = %q, want T-test", sentReq.Token)
	}
	if sentReq.APIName != "stock_basic" {
		t.Errorf("server saw api_name = %q, want stock_basic", sentReq.APIName)
	}
	if v := sentReq.Params["list_status"]; v != "L" {
		t.Errorf("server saw params[list_status] = %q, want L", v)
	}

	var env tushareEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if len(env.Fields) != 3 || env.Fields[0] != "ts_code" {
		t.Errorf("Fields = %v, want [ts_code name industry]", env.Fields)
	}
	if len(env.Items) != 2 {
		t.Fatalf("Items len = %d, want 2", len(env.Items))
	}
	if env.Items[0][1] != "平安银行" {
		t.Errorf("Items[0][1] = %v, want 平安银行", env.Items[0][1])
	}
}

func TestTushare_RejectsMissingToken(t *testing.T) {
	t.Parallel()

	tool := NewTushareTool()
	_, err := tool.InvokableRun(context.Background(),
		`{"api_name":"stock_basic"}`)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("err = %v, want to mention token", err)
	}
}

func TestTushare_RejectsMissingAPIName(t *testing.T) {
	t.Parallel()

	tool := NewTushareTool()
	_, err := tool.InvokableRun(context.Background(),
		`{"token":"T-abc"}`)
	if err == nil {
		t.Fatal("expected error for missing api_name")
	}
	if !strings.Contains(err.Error(), "api_name") {
		t.Errorf("err = %v, want to mention api_name", err)
	}
}

func TestTushare_UpstreamErrorCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code": 40201, "msg": "权限不足"}`))
	}))
	defer srv.Close()

	prev := tushareEndpoint
	tushareEndpoint = srv.URL
	t.Cleanup(func() { tushareEndpoint = prev })

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewTushareToolWith(helper)

	_, err := tool.InvokableRun(context.Background(),
		`{"token":"T-abc","api_name":"premium_only"}`)
	if err == nil {
		t.Fatal("expected error for non-zero code, got nil")
	}
	if !strings.Contains(err.Error(), "权限不足") {
		t.Errorf("err = %v, want to surface upstream msg", err)
	}
}

func TestTushare_Info(t *testing.T) {
	t.Parallel()

	tool := NewTushareTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "tushare" {
		t.Errorf("Name = %q, want tushare", info.Name)
	}
	if !strings.Contains(info.Desc, "Tushare") {
		t.Errorf("Desc = %q, want to mention Tushare", info.Desc)
	}
}
