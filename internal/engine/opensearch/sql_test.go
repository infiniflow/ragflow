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

package opensearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/tokenizer"
)

func init() {
	tokenizer.SetEngineType("infinity")
}

const sampleOSJDBCResponse = `{
  "schema": [
    {"name": "doc_id", "type": "keyword"},
    {"name": "docnm", "type": "text"},
    {"name": "tag", "type": "keyword"},
    {"name": "count", "type": "integer"}
  ],
  "datarows": [
    ["d1", "report.pdf", "", 5],
    ["d2", null, "active", 3]
  ],
  "total": 2,
  "size": 2,
  "status": 200
}`

func TestPreprocess_WhitespaceAndBackticks(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a  b", "a b"},
		{"a   b   c", "a b c"},
		{"a`b`c", "a b c"},
		{"a `` b", "a b"},
		{"  leading and trailing  ", " leading and trailing "},
	}
	for _, c := range cases {
		if got := preprocessSQL(c.in); got != c.want {
			t.Errorf("preprocessSQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocess_StripsPercent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"count > 0 %", "count > 0 "},
		{"100% match", "100 match"},
		{"%%%", ""},
	}
	for _, c := range cases {
		if got := preprocessSQL(c.in); got != c.want {
			t.Errorf("preprocessSQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocess_LktksRewrite_OpenSearchNamedSyntax(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		field  string
		expect string
	}{
		{
			"like with single-token value (ltks suffix)",
			"select content_ltks like 'weather'",
			"content_ltks",
			"MATCH(content_ltks, 'weather', operator='OR', minimum_should_match='30%')",
		},
		{
			"uppercase LIKE with ltks suffix",
			"select content_ltks LIKE 'financial'",
			"content_ltks",
			"MATCH(content_ltks, 'financial', operator='OR', minimum_should_match='30%')",
		},
		{
			"mixed-case Like with ltks suffix",
			"select content_ltks Like 'report'",
			"content_ltks",
			"MATCH(content_ltks, 'report', operator='OR', minimum_should_match='30%')",
		},
		{
			"= with multi-word value (ltks suffix)",
			"select content_ltks = 'final report'",
			"content_ltks",
			"MATCH(content_ltks, 'final report', operator='OR', minimum_should_match='30%')",
		},
		{
			"tks (no l) suffix",
			"select title_tks like 'climate'",
			"title_tks",
			"MATCH(title_tks, 'climate', operator='OR', minimum_should_match='30%')",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := preprocessSQL(c.in)
			if !strings.Contains(got, c.expect) {
				t.Errorf("preprocessSQL(%q):\ngot:  %q\nwant contains: %q", c.in, got, c.expect)
			}
		})
	}
}

func TestRunSQL_NilClientOrEmptySQL(t *testing.T) {
	ctx := context.Background()
	var nilEngine *Engine
	if _, err := nilEngine.RunSQL(ctx, "t1", "SELECT 1", nil, "json"); err == nil {
		t.Errorf("expected error on nil engine, got nil")
	}

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	if _, err := engine.RunSQL(ctx, "t1", "", nil, "json"); err == nil {
		t.Errorf("expected error on empty SQL, got nil")
	}
}

func TestRunSQL_FormatValidation(t *testing.T) {
	ctx := context.Background()
	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleOSJDBCResponse))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	// "json", "", and case-insensitive "JSON" should succeed
	for _, fmtStr := range []string{"json", "", "JSON", " json "} {
		if _, err := engine.RunSQL(ctx, "t1", "SELECT 1", nil, fmtStr); err != nil {
			t.Errorf("expected format %q to succeed, got error: %v", fmtStr, err)
		}
	}

	// Non-json formats must return an error
	for _, badFmt := range []string{"csv", "jdbc", "xml", "text"} {
		if _, err := engine.RunSQL(ctx, "t1", "SELECT 1", nil, badFmt); err == nil {
			t.Errorf("expected error for unsupported format %q, got nil", badFmt)
		} else if !strings.Contains(err.Error(), "unsupported format") {
			t.Errorf("expected 'unsupported format' in error for %q, got: %v", badFmt, err)
		}
	}
}

func TestRunSQL_PostsToPluginsSQLJDBC(t *testing.T) {
	var capturedPath string
	var capturedBody string
	var capturedMethod string

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.RequestURI()
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleOSJDBCResponse))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	rows, err := engine.RunSQL(ctx, "ragflow_t1", "SELECT doc_id, docnm, count FROM `ragflow_t1` WHERE count > 0", nil, "json")
	if err != nil {
		t.Fatalf("RunSQL failed: %v", err)
	}

	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST method, got %s", capturedMethod)
	}
	if capturedPath != "/_plugins/_sql?format=jdbc" {
		t.Errorf("expected path /_plugins/_sql?format=jdbc, got %s", capturedPath)
	}

	var parsedBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &parsedBody); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}
	if fmt.Sprint(parsedBody["fetch_size"]) != "128" {
		t.Errorf("expected fetch_size=128, got %v", parsedBody["fetch_size"])
	}
	if _, ok := parsedBody["query"]; !ok {
		t.Errorf("expected query in request body")
	}

	// Verify JDBC decoding: null stays nil, empty string stays empty string
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["doc_id"] != "d1" || rows[0]["docnm"] != "report.pdf" {
		t.Errorf("row 0 mismatch: %v", rows[0])
	}
	if val, ok := rows[0]["tag"]; !ok || val != "" {
		t.Errorf("expected tag in row 0 to be empty string \"\", got %v (exists: %v)", val, ok)
	}

	// Row 1: docnm was JSON null -> must be nil in Go, not empty string
	docnmVal, ok := rows[1]["docnm"]
	if !ok {
		t.Errorf("expected docnm key to exist in row 1")
	}
	if docnmVal != nil {
		t.Errorf("expected null docnm to be nil, got %v (%T)", docnmVal, docnmVal)
	}
	if rows[1]["tag"] != "active" {
		t.Errorf("row 1 tag mismatch: %v", rows[1]["tag"])
	}
}

func TestRunSQL_EmptyRowsSentinel(t *testing.T) {
	emptyResp := `{"schema": [{"name": "doc_id", "type": "keyword"}], "datarows": []}`
	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(emptyResp))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	rows, err := engine.RunSQL(context.Background(), "t1", "SELECT doc_id FROM t1", nil, "json")
	if err != nil {
		t.Fatalf("RunSQL failed: %v", err)
	}
	if rows != nil {
		t.Errorf("expected empty rows sentinel (nil, nil), got %v", rows)
	}
}

func TestRunSQL_NonTimeoutErrorSurfacesImmediately(t *testing.T) {
	var mu sync.Mutex
	calls := 0

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"type": "ParsingException", "reason": "syntax error near line 1"}}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	_, err = engine.RunSQL(context.Background(), "t1", "SELECT bad syntax", nil, "json")
	if err == nil {
		t.Fatalf("expected error on 400 Bad Request, got nil")
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("expected exactly 1 call without retrying non-timeout error, got %d", calls)
	}
	if !strings.Contains(err.Error(), "SQL error:") {
		t.Errorf("expected 'SQL error:' in error message, got %v", err)
	}
}

func TestRunSQL_RetryOnTimeoutThenSucceed(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_plugins/_sql" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}

		mu.Lock()
		calls++
		attempt := calls
		mu.Unlock()

		if attempt == 1 {
			// Simulate timeout on attempt 1 by blocking until context expires
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleOSJDBCResponse))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer func() {
		close(release)
		server.Close()
	}()

	rows, err := engine.RunSQL(context.Background(), "t1", "SELECT doc_id FROM t1", nil, "json")
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows on successful retry, got %d", len(rows))
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("expected 2 calls (1 timeout + 1 success), got %d", calls)
	}
}

func TestRunSQL_ClosesCursorWhenPresent(t *testing.T) {
	cursorClosed := false
	responseWithCursor := `{
	  "cursor": "cursor12345",
	  "schema": [
	    {"name": "doc_id", "type": "keyword"}
	  ],
	  "datarows": [
	    ["d1"]
	  ]
	}`

	engine, server, err := newMockSearchEngine(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_plugins/_sql/close" {
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["cursor"] == "cursor12345" {
				cursorClosed = true
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"acknowledged": true}`))
			return
		}
		if r.URL.Path == "/_plugins/_sql" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(responseWithCursor))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}, "2.19.0")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer server.Close()

	rows, err := engine.RunSQL(context.Background(), "t1", "SELECT doc_id FROM t1", nil, "json")
	if err != nil {
		t.Fatalf("expected query to succeed, got error: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
	if !cursorClosed {
		t.Errorf("expected cursor to be closed via /_plugins/_sql/close")
	}
}
