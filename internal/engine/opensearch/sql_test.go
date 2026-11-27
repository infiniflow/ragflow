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
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/tablequery"
	"ragflow/internal/tokenizer"

	"github.com/opensearch-project/opensearch-go/v4"
)

const testOSKBID = "11111111-1111-1111-1111-111111111111"

func sampleOSValidatedQuery(t *testing.T) *tablequery.ValidatedQuery {
	t.Helper()
	scope := tablequery.Scope{
		TenantID:  "t1",
		TableName: "ragflow_t1",
		KBIDs:     []string{testOSKBID},
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

type capturedRequest struct {
	mu       sync.Mutex
	path     string
	body     string
	method   string
	rawQuery string
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
		capRequest.rawQuery = r.URL.RawQuery
		capRequest.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(replyStatus)
		_, _ = w.Write([]byte(replyBody))
	}))
	t.Cleanup(srv.Close)
	return srv, capRequest
}

func newTestEngine(t *testing.T, srvURL string) *Engine {
	t.Helper()
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses:    []string{srvURL},
		DisableRetry: true,
	})
	if err != nil {
		t.Fatalf("opensearch.NewClient: %v", err)
	}
	return &Engine{client: client}
}

const sampleOSResponse = `{
  "schema": [
    {"name": "doc_id", "type": "text"},
    {"name": "docnm_kwd", "type": "text"},
    {"name": "count", "type": "long"}
  ],
  "datarows": [
    ["d1", "report.pdf", 5],
    ["d2", "spec.pdf", 3]
  ],
  "total": 2,
  "size": 2
}`

func TestExecuteTableQuery_CompilesScopedQueryAndPostsToPluginSQL(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleOSResponse)
	e := newTestEngine(t, srv.URL)

	rows, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	cap.mu.Lock()
	gotPath := cap.path
	gotBody := cap.body
	gotQuery := cap.rawQuery
	cap.mu.Unlock()

	if gotPath != "/_plugins/_sql" {
		t.Errorf("path: got %q, want /_plugins/_sql", gotPath)
	}
	if gotQuery != "format=jdbc" {
		t.Errorf("rawQuery: got %q, want format=jdbc", gotQuery)
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
	if !strings.Contains(q, "FROM ragflow_t1 WHERE (count > 0) AND (kb_id = '"+testOSKBID+"')") {
		t.Errorf("unexpected compiled query: %q", q)
	}
}

func TestExecuteTableQuery_RejectsNilQuery(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusOK, sampleOSResponse)
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
		_, err := e.ExecuteTableQuery(ctx, sampleOSValidatedQuery(t))
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleOSResponse))
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	e := newTestEngine(t, srv.URL)
	rows, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "syntax error"}`))
	}))
	t.Cleanup(srv.Close)

	e := newTestEngine(t, srv.URL)
	_, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
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
	empty := `{"schema": [{"name": "doc_id", "type": "text"}], "datarows": [], "total": 0, "size": 0}`
	srv, _ := newCapturingServer(t, http.StatusOK, empty)
	e := newTestEngine(t, srv.URL)

	rows, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if rows != nil {
		t.Errorf("rows: got %v, want nil (empty-rows sentinel)", rows)
	}
}

func TestExecuteTableQuery_FollowsCursorPagination(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		page := len(bodies)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if page == 1 {
			_, _ = w.Write([]byte(`{
				"schema": [{"name": "doc_id", "type": "text"}],
				"datarows": [["d1"]],
				"cursor": "cursor-2"
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"schema": [{"name": "doc_id", "type": "text"}],
			"datarows": [["d2"]]
		}`))
	}))
	t.Cleanup(srv.Close)

	e := newTestEngine(t, srv.URL)
	rows, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2 (both pages)", len(rows))
	}
	if rows[0]["doc_id"] != "d1" || rows[1]["doc_id"] != "d2" {
		t.Errorf("rows: got %v, want d1 then d2", rows)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("requests: got %d, want 2", len(bodies))
	}
	var first, second map[string]interface{}
	if err := json.Unmarshal([]byte(bodies[0]), &first); err != nil {
		t.Fatalf("first body is not JSON: %v", err)
	}
	if _, has := first["query"]; !has {
		t.Errorf("first request must carry query, got %v", first)
	}
	if err := json.Unmarshal([]byte(bodies[1]), &second); err != nil {
		t.Fatalf("second body is not JSON: %v", err)
	}
	if _, has := second["query"]; has {
		t.Errorf("cursor request must NOT carry query, got %v", second)
	}
	if second["cursor"] != "cursor-2" {
		t.Errorf("cursor: got %v, want cursor-2", second["cursor"])
	}
}

func TestExecuteTableQuery_UsesColumnAliasIfPresent(t *testing.T) {
	aliasResponse := `{
		"schema": [
			{"name": "COUNT(1)", "type": "long", "alias": "total_count"},
			{"name": "doc_id", "type": "text"}
		],
		"datarows": [
			[42, "doc-1"]
		],
		"total": 1,
		"size": 1
	}`
	srv, _ := newCapturingServer(t, http.StatusOK, aliasResponse)
	e := newTestEngine(t, srv.URL)

	rows, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if v, ok := rows[0]["total_count"]; !ok || v != float64(42) {
		t.Errorf("expected row['total_count'] = 42, got %v", rows[0]["total_count"])
	}
	if v, ok := rows[0]["doc_id"]; !ok || v != "doc-1" {
		t.Errorf("expected row['doc_id'] = 'doc-1', got %v", rows[0]["doc_id"])
	}
	if _, ok := rows[0]["COUNT(1)"]; ok {
		t.Errorf("row['COUNT(1)'] should not be present when alias is defined")
	}
}

func TestExecuteTableQuery_ClosesCursorOnPrematureExit(t *testing.T) {
	var (
		mu           sync.Mutex
		closeCalled  bool
		closedCursor string
		requests     int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.URL.Path == "/_plugins/_sql/close" {
			closeCalled = true
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			closedCursor = body["cursor"]
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"succeeded": true}`))
			return
		}

		if requests == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"schema": [{"name": "doc_id", "type": "text"}],
				"datarows": [["d1"]],
				"cursor": "cursor-exit-early"
			}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "node failure"}`))
	}))
	t.Cleanup(srv.Close)

	e := newTestEngine(t, srv.URL)
	_, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t))
	if err == nil {
		t.Fatal("expected ExecuteTableQuery to fail on 500 error")
	}

	mu.Lock()
	defer mu.Unlock()
	if !closeCalled {
		t.Errorf("expected /_plugins/_sql/close to be called on error")
	}
	if closedCursor != "cursor-exit-early" {
		t.Errorf("expected closed cursor 'cursor-exit-early', got %q", closedCursor)
	}
}

func TestMain(m *testing.M) {
	_ = os.Setenv("OPENSEARCH_GO_DISCOVERY_CONFIG", "-node_stats")
	common.InitLogger("info", common.FileOutput{}, "opensearch_test")
	tokenizer.SetEngineType("infinity")
	m.Run()
}

func TestExecuteTableQuery_ErrorsOnStuckCursor(t *testing.T) {
	var queryRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_plugins/_sql/close" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"succeeded": true}`))
			return
		}
		queryRequests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"schema": [{"name": "doc_id", "type": "text"}],
			"datarows": [["d1"]],
			"cursor": "same-cursor"
		}`))
	}))
	t.Cleanup(srv.Close)

	e := newTestEngine(t, srv.URL)
	if _, err := e.ExecuteTableQuery(t.Context(), sampleOSValidatedQuery(t)); err == nil {
		t.Fatal("ExecuteTableQuery: got nil error for a cursor that never advances")
	}
	if queryRequests > 2 {
		t.Errorf("queryRequests: got %d, want at most 2 before aborting", queryRequests)
	}
}
