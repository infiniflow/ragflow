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

func TestAkShare_FetchesStockNews(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/jsonp" {
			t.Errorf("path = %q, want /search/jsonp", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		param := r.URL.Query().Get("param")
		var req akshareSearchRequest
		if err := json.Unmarshal([]byte(param), &req); err != nil {
			t.Errorf("param is not valid request JSON: %v (%s)", err, param)
			http.Error(w, "invalid param", http.StatusBadRequest)
			return
		}
		if req.Keyword != "600519" {
			t.Errorf("keyword = %q, want 600519", req.Keyword)
			http.Error(w, "unexpected keyword", http.StatusBadRequest)
			return
		}

		callback := r.URL.Query().Get("cb")
		if callback == "" {
			callback = "callback"
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(callback + `({
			"result": {
				"cmsArticleWebOld": [
					{"code":"202601010001","title":"<em>贵州茅台</em> title0","content":"content0\r\nmore","date":"2026-01-01","mediaName":"src0"},
					{"code":"202601010002","title":"title1","content":"content1","date":"2026-01-02","mediaName":"src1"},
					{"code":"202601010003","title":"title2","content":"content2","date":"2026-01-03","mediaName":"src2"}
				]
			}
		})`))
	}))
	defer srv.Close()

	oldEndpoint := akshareStockNewsEndpoint
	akshareStockNewsEndpoint = srv.URL + "/search/jsonp"
	defer func() { akshareStockNewsEndpoint = oldEndpoint }()

	tool := NewAkShareToolWithTopN(NewHTTPHelper(), 2)
	out, err := tool.InvokableRun(context.Background(), `{"query":"600519"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v (out=%s)", err, out)
	}

	var env akshareEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", err, out)
	}
	if env.Error != "" {
		t.Fatalf("env.Error = %q, want empty", env.Error)
	}
	if len(env.Articles) != 2 {
		t.Fatalf("len(Articles) = %d, want 2", len(env.Articles))
	}
	if env.Articles[0].Title != "贵州茅台 title0" {
		t.Errorf("first title = %q, want cleaned title", env.Articles[0].Title)
	}
	if !strings.Contains(env.Content, `<a href="http://finance.eastmoney.com/a/202601010001.html">贵州茅台 title0</a>`) {
		t.Errorf("formatted content missing first link/title: %q", env.Content)
	}
	if strings.Count(env.Content, "新闻内容:") != 2 {
		t.Errorf("formatted content count = %d, want 2", strings.Count(env.Content, "新闻内容:"))
	}
}

func TestAkShare_ParseTruncatesToTopN(t *testing.T) {
	t.Parallel()

	articles := parseAkShareFixture(t, 3)
	if len(articles) != 3 {
		t.Fatalf("fixture articles = %d, want 3", len(articles))
	}
}

func TestAkShare_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	tool := NewAkShareTool()
	_, err := tool.InvokableRun(context.Background(), `{not json`)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse arguments") {
		t.Errorf("err = %q, want parse arguments error", err.Error())
	}
}

func TestAkShare_RejectsMissingQuery(t *testing.T) {
	t.Parallel()

	tool := NewAkShareTool()
	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err == nil {
		t.Fatalf("expected error for missing query, got nil (out=%s)", out)
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Errorf("err = %q, want query required", err.Error())
	}
}

func TestAkShare_Info(t *testing.T) {
	t.Parallel()

	tool := NewAkShareTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "akshare_stock_news" {
		t.Errorf("Name = %q, want akshare_stock_news", info.Name)
	}
	if !strings.Contains(info.Desc, "East Money") {
		t.Errorf("Desc = %q, want to mention East Money", info.Desc)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("ParamsOneOf = nil, want schema definition")
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal params schema: %v", err)
	}
	params := string(raw)
	if !strings.Contains(params, `"query"`) {
		t.Fatalf("schema missing query parameter: %s", params)
	}
	if !strings.Contains(params, `"required":["query"]`) {
		t.Fatalf("schema does not require query: %s", params)
	}
}

func TestBuildByName_AkShareRejectsInvalidTopN(t *testing.T) {
	t.Parallel()

	_, err := BuildByName("akshare", map[string]any{"top_n": 0})
	if err == nil {
		t.Fatal("expected top_n validation error")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Fatalf("err = %q, want positive integer validation", err.Error())
	}
}

func parseAkShareFixture(t *testing.T, topN int) []akshareArticle {
	t.Helper()

	callback := "cb"
	body := callback + `({
		"result": {
			"cmsArticleWebOld": [
				{"code":"1","title":"title1","content":"content1","date":"2026-01-01","mediaName":"src1"},
				{"code":"2","title":"title2","content":"content2","date":"2026-01-02","mediaName":"src2"},
				{"code":"3","title":"title3","content":"content3","date":"2026-01-03","mediaName":"src3"},
				{"code":"4","title":"title4","content":"content4","date":"2026-01-04","mediaName":"src4"}
			]
		}
	})`
	articles, err := parseAkShareStockNews([]byte(body), topN)
	if err != nil {
		t.Fatalf("parseAkShareStockNews: %v", err)
	}
	return articles
}

func TestBuildAkShareStockNewsURL(t *testing.T) {
	oldEndpoint := akshareStockNewsEndpoint
	akshareStockNewsEndpoint = "https://example.com/search/jsonp"
	defer func() { akshareStockNewsEndpoint = oldEndpoint }()

	rawURL, err := buildAkShareStockNewsURL("600519", 25)
	if err != nil {
		t.Fatalf("buildAkShareStockNewsURL: %v", err)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse built URL: %v", err)
	}
	if u.Host != "example.com" {
		t.Fatalf("host = %q, want example.com", u.Host)
	}
	var req akshareSearchRequest
	if err := json.Unmarshal([]byte(u.Query().Get("param")), &req); err != nil {
		t.Fatalf("param JSON: %v", err)
	}
	if req.Keyword != "600519" {
		t.Fatalf("keyword = %q, want 600519", req.Keyword)
	}
	cmsParam, ok := req.Param["cmsArticleWebOld"].(map[string]any)
	if !ok {
		t.Fatalf("cmsArticleWebOld param = %T, want object", req.Param["cmsArticleWebOld"])
	}
	if cmsParam["pageSize"] != float64(25) {
		t.Fatalf("pageSize = %v, want 25", cmsParam["pageSize"])
	}
}
