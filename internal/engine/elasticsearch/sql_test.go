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

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/engine/tablequery"
	"ragflow/internal/tokenizer"

	"github.com/elastic/go-elasticsearch/v8"
)

const testKBID = "11111111-1111-1111-1111-111111111111"

func sampleValidatedQuery(t *testing.T) *tablequery.ValidatedQuery {
	t.Helper()
	scope := tablequery.Scope{
		TenantID:  "t1",
		TableName: "ragflow_t1",
		KBIDs:     []string{testKBID},
		Fields: tablequery.FieldsFromFieldMap(map[string]interface{}{
			"title_tks": "Title",
			"count":     "Count (int)",
		}),
	}
	vq, err := tablequery.Validate(scope, tablequery.Query{
		Select: []string{"title_tks", "count"},
		Where:  &tablequery.FilterNode{Field: "count", Op: tablequery.OpGt, Value: 0},
	}, true)
	if err != nil {
		t.Fatalf("tablequery.Validate: %v", err)
	}
	return vq
}

// capturedRequest holds the request body the test server saw, for assertions.
type capturedRequest struct {
	mu     sync.Mutex
	path   string
	body   string
	method string
}

func newCapturingServer(t *testing.T, replyStatus int, replyBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	capRequest := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capRequest.mu.Lock()
		capRequest.method = r.Method
		capRequest.path = r.URL.Path
		capRequest.body = string(body)
		capRequest.mu.Unlock()
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(replyStatus)
		_, _ = w.Write([]byte(replyBody))
	}))
	t.Cleanup(srv.Close)
	return srv, capRequest
}

func newTestEngine(t *testing.T, srvURL string) *Engine {
	t.Helper()
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{srvURL},
	})
	if err != nil {
		t.Fatalf("elasticsearch.NewClient: %v", err)
	}
	return &Engine{client: client}
}

const sampleESResponse = `{
  "columns": [
    {"name": "doc_id", "type": "text"},
    {"name": "docnm_kwd", "type": "text"},
    {"name": "count", "type": "long"}
  ],
  "rows": [
    ["d1", "report.pdf", 5],
    ["d2", "spec.pdf", 3]
  ]
}`

func TestExecuteTableQuery_CompilesScopedQueryAndPostsToSQL(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)
	ctx := t.Context()

	rows, err := e.ExecuteTableQuery(ctx, sampleValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	cap.mu.Lock()
	gotPath := cap.path
	gotBody := cap.body
	cap.mu.Unlock()

	if gotPath != "/_sql" {
		t.Errorf("path: got %q, want /_sql", gotPath)
	}

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body is not JSON: %v\nbody=%q", err, gotBody)
	}
	if _, has := body["filter"]; has {
		t.Errorf("request must not include top-level filter; body=%v", body)
	}
	if fmt.Sprint(body["fetch_size"]) != "128" {
		t.Errorf("fetch_size: got %v, want 128", body["fetch_size"])
	}
	q, _ := body["query"].(string)
	if !strings.Contains(q, "FROM ragflow_t1 WHERE (count > 0) AND (kb_id = '"+testKBID+"')") {
		t.Errorf("unexpected compiled query: %q", q)
	}
}

func TestExecuteTableQuery_RejectsNilQuery(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)
	if _, err := e.ExecuteTableQuery(t.Context(), nil); err == nil {
		t.Fatal("expected error for nil ValidatedQuery")
	}
}

func TestExecuteTableQuery_PerAttemptTimeout(t *testing.T) {
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-hang
	}))
	t.Cleanup(func() {
		close(hang)
		srv.Close()
	})
	e := newTestEngine(t, srv.URL)

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	type result struct {
		elapsed time.Duration
		err     error
	}
	done := make(chan result, 1)
	go func() {
		start := time.Now()
		_, err := e.ExecuteTableQuery(ctx, sampleValidatedQuery(t))
		done <- result{elapsed: time.Since(start), err: err}
	}()

	select {
	case r := <-done:
		if r.err == nil {
			t.Fatalf("ExecuteTableQuery: got nil error, want timeout error")
		}
		if r.elapsed < 6*time.Second {
			t.Errorf("ExecuteTableQuery returned in %s; expected ~7s (2 attempts + 3s sleep)", r.elapsed)
		}
		if !strings.Contains(r.err.Error(), "timeout after 2 attempts") {
			t.Errorf("err: got %q, want substring %q", r.err.Error(), "timeout after 2 attempts")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("ExecuteTableQuery did not return within 15s — suspected hang in retry/timeout chain")
	}
}

func TestExecuteTableQuery_RetryOnTimeoutThenSucceed(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		attempt := calls
		mu.Unlock()
		if attempt == 1 {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleESResponse))
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	e := newTestEngine(t, srv.URL)
	rows, err := e.ExecuteTableQuery(t.Context(), sampleValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("rows: got %d, want 2 (second attempt should succeed)", len(rows))
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("server calls: got %d, want 2 (initial + one retry)", calls)
	}
}

func TestExecuteTableQuery_NonTimeoutErrorSurfacesImmediately(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "syntax error"}`))
	}))
	t.Cleanup(srv.Close)

	e := newTestEngine(t, srv.URL)
	_, err := e.ExecuteTableQuery(t.Context(), sampleValidatedQuery(t))
	if err == nil {
		t.Fatalf("ExecuteTableQuery: got nil error, want error")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("server calls: got %d, want 1 (non-timeout error must NOT retry)", calls)
	}
	if !strings.Contains(err.Error(), "SQL error:") {
		t.Errorf("err: got %q, want substring 'SQL error:'", err.Error())
	}
}

func TestExecuteTableQuery_EmptyRowsReturnsNilNil(t *testing.T) {
	empty := `{"columns": [{"name": "doc_id", "type": "text"}], "rows": []}`
	srv, _ := newCapturingServer(t, http.StatusOK, empty)
	e := newTestEngine(t, srv.URL)

	rows, err := e.ExecuteTableQuery(t.Context(), sampleValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if rows != nil {
		t.Errorf("rows: got %v, want nil (empty-rows sentinel)", rows)
	}
}

func TestMain(m *testing.M) {
	tokenizer.SetEngineType("infinity")
	m.Run()
}
