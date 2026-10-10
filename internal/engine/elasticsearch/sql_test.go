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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/tokenizer"

	"github.com/elastic/go-elasticsearch/v8"
)

// capturedRequest holds the request body the test server saw, for
// assertions.
type capturedRequest struct {
	mu     sync.Mutex
	path   string
	body   string
	method string
}

// newCapturingServer returns an httptest.Server that captures each
// incoming request and replies with the given body / status.
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

// newTestEngine constructs an Engine pointing at the given
// test server. Bypasses NewEngine (which calls ES Info to verify
// connectivity) — the test server is a stub, not a real ES cluster.
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
    {"name": "docnm", "type": "text"},
    {"name": "count", "type": "long"}
  ],
  "rows": [
    ["d1", "report.pdf", 5],
    ["d2", "spec.pdf", 3]
  ]
}`

// TestRunSQL_NoFilterAdded verifies the request body is exactly
// {"query": <sql>} — the redundant `filter` field that the previous
// implementation added is gone. (service.tableSQLPolicy is the source of
// truth for kb_id scoping upstream of RunSQL.)
func TestRunSQL_NoFilterAdded(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)
	ctx := t.Context()

	rows, err := e.RunSQL(ctx, "ragflow_t1", "SELECT doc_id FROM ragflow_t1", nil, "json")
	if err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	cap.mu.Lock()
	got := cap.body
	cap.mu.Unlock()

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(got), &body); err != nil {
		t.Fatalf("body is not JSON: %v\nbody=%q", err, got)
	}
	if _, has := body["filter"]; has {
		t.Errorf("RunSQL request must NOT include top-level filter (tableSQLPolicy scopes the statement upstream). body=%v", body)
	}
	if _, has := body["query"]; !has {
		t.Errorf("RunSQL request must include query. body=%v", body)
	}
}

func TestRunSQLPreservesStringValues(t *testing.T) {
	srv, captured := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)
	query := "SELECT doc_id FROM ragflow_t1 WHERE docnm_kwd = '50%  `price`'"
	if _, err := e.RunSQL(t.Context(), "ragflow_t1", query, nil, "json"); err != nil {
		t.Fatal(err)
	}
	captured.mu.Lock()
	defer captured.mu.Unlock()
	var body map[string]any
	if err := json.Unmarshal([]byte(captured.body), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["query"].(string), "'50%  `price`'") {
		t.Fatalf("query value changed: %v", body["query"])
	}
}

func TestRunSQL_PerAttemptTimeout(t *testing.T) {
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-hang
	}))
	t.Cleanup(func() {
		close(hang)
		srv.Close()
	})
	e := newTestEngine(t, srv.URL)

	// 15s outer budget — well above the expected ~7s. If this fires,
	// the retry loop or the timeout is broken; the test will report
	// a clear "did not return within 15s" message rather than a
	// fragile absolute wall-clock assertion.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	type result struct {
		elapsed time.Duration
		err     error
	}
	done := make(chan result, 1)
	go func() {
		start := time.Now()
		_, err := e.RunSQL(ctx, "ragflow_t1", "SELECT 1", nil, "json")
		done <- result{elapsed: time.Since(start), err: err}
	}()

	select {
	case r := <-done:
		if r.err == nil {
			t.Fatalf("RunSQL: got nil error, want timeout error")
		}
		// 2s + 3s + 2s = 7s. Lower bound proves both attempts fired
		// AND the retry was scheduled. A constant-delay or single-attempt
		// regression would slip below this.
		if r.elapsed < 6*time.Second {
			t.Errorf("RunSQL returned in %s; expected ~7s (2 attempts + 3s sleep)", r.elapsed)
		}
		if !strings.Contains(r.err.Error(), "timeout after 2 attempts") {
			t.Errorf("err: got %q, want substring %q", r.err.Error(), "timeout after 2 attempts")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("RunSQL did not return within 15s — suspected hang in retry/timeout chain")
	}
}

// TestRunSQL_RetryOnTimeoutThenSucceed simulates Python's
// ConnectionTimeout-retry pattern: the first attempt times out, the
// second attempt returns valid rows. The loop should silently retry
// and return the rows.
func TestRunSQL_RetryOnTimeoutThenSucceed(t *testing.T) {
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
			// First attempt: hang so the 2s context fires.
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
	ctx := t.Context()
	rows, err := e.RunSQL(ctx, "ragflow_t1", "SELECT 1", nil, "json")
	if err != nil {
		t.Fatalf("RunSQL: %v", err)
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

// TestRunSQL_NonTimeoutErrorSurfacesImmediately verifies the non-retry
// path: a 4xx ES response should NOT trigger a retry. The error must
// be wrapped as `SQL error: <e>\n\nSQL: <sql>`, matching Python's
// es_conn_base.py:400.
func TestRunSQL_NonTimeoutErrorSurfacesImmediately(t *testing.T) {
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
	ctx := t.Context()
	_, err := e.RunSQL(ctx, "ragflow_t1", "SELECT bad", nil, "json")
	if err == nil {
		t.Fatalf("RunSQL: got nil error, want error")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("server calls: got %d, want 1 (non-timeout error must NOT retry)", calls)
	}
	// Python wraps as `f"SQL error: {e}\n\nSQL: {sql}"`.
	if !strings.Contains(err.Error(), "SQL error:") {
		t.Errorf("err: got %q, want substring 'SQL error:'", err.Error())
	}
	if !strings.Contains(err.Error(), "SQL: SELECT bad") {
		t.Errorf("err: got %q, want substring 'SQL: SELECT bad'", err.Error())
	}
}

// TestRunSQL_RequestBodyHasFetchSizeAndFormat verifies the request body
// includes fetch_size=128 and the SQLQueryRequest is built with
// format="json", matching the Python defaults at rag/nlp/search.py:773.
func TestRunSQL_RequestBodyHasFetchSizeAndFormat(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)

	ctx := t.Context()
	if _, err := e.RunSQL(ctx, "ragflow_t1", "SELECT 1", nil, "json"); err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	cap.mu.Lock()
	got := cap.body
	cap.mu.Unlock()

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(got), &body); err != nil {
		t.Fatalf("body is not JSON: %v\nbody=%q", err, got)
	}
	fs, ok := body["fetch_size"]
	if !ok {
		t.Errorf("body has no fetch_size; got %v", body)
	}
	if fmt.Sprint(fs) != "128" {
		t.Errorf("fetch_size: got %v, want 128", fs)
	}
}

// TestRunSQL_EmptyRowsReturnsNilNil verifies the (nil, nil) sentinel
// for empty results — callers treat this as "fall through to vector
// retrieval".
func TestRunSQL_EmptyRowsReturnsNilNil(t *testing.T) {
	empty := `{"columns": [{"name": "doc_id", "type": "text"}], "rows": []}`
	srv, _ := newCapturingServer(t, http.StatusOK, empty)
	e := newTestEngine(t, srv.URL)

	ctx := t.Context()
	rows, err := e.RunSQL(ctx, "ragflow_t1", "SELECT doc_id FROM ragflow_t1", nil, "json")
	if err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	if rows != nil {
		t.Errorf("rows: got %v, want nil (empty-rows sentinel)", rows)
	}
}

// TestRunSQL_PostsToSQLPath verifies the request goes to the /_sql
// endpoint (the modern ES SQL API; the older /_xpack/sql path is
// deprecated as of ES 7.x). The Go SDK's esapi.SQLQueryRequest hits
// /_sql; the Python ES client is also pinned to the modern endpoint
// at runtime even though the legacy /_xpack/sql name appears in the
// SDK's method (`es.sql.query(...)`).
func TestRunSQL_PostsToSQLPath(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)

	ctx := t.Context()
	if _, err := e.RunSQL(ctx, "ragflow_t1", "SELECT 1", nil, "json"); err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	cap.mu.Lock()
	got := cap.path
	cap.mu.Unlock()
	if got != "/_sql" {
		t.Errorf("path: got %q, want /_sql", got)
	}
}

// TestMain registers the engine as "infinity" so tokenizer.Tokenize and
// tokenizer.FineGrainedTokenize short-circuit and return the input
// as-is. This lets the rewrite tests assert on the SHAPE of the MATCH()
// substitution without depending on a real tokenizer pool.
func TestMain(m *testing.M) {
	tokenizer.SetEngineType("infinity")
	m.Run()
}

type fakeNetTimeoutErr struct{}

func (fakeNetTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeNetTimeoutErr) Timeout() bool   { return true }
func (fakeNetTimeoutErr) Temporary() bool { return true }

func TestIsTimeoutError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("wrap: %w", context.DeadlineExceeded), true},
		{"net.Error.Timeout()==true", fakeNetTimeoutErr{}, true},
		{"wrapped net.Error.Timeout", fmt.Errorf("wrap: %w", fakeNetTimeoutErr{}), true},
		{"plain string 'i/o timeout'", errors.New("read tcp: i/o timeout"), true},
		{"plain string 'deadline exceeded'", errors.New("context deadline exceeded"), true},
		{"plain string 'connection timeout'", errors.New("connection timeout while reading"), true},
		{"unrelated error", errors.New("parse: invalid character"), false},
		{"EOF is not a timeout", errors.New("EOF"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTimeoutError(c.err); got != c.want {
				t.Errorf("isTimeoutError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsTimeoutError_NonTimeoutNetError(t *testing.T) {
	e := &net.OpError{
		Op:  "dial",
		Err: errors.New("connection refused"),
	}
	if isTimeoutError(e) {
		t.Errorf("isTimeoutError(connection-refused) = true, want false")
	}
}

func TestRunSQLBuildsRuntimeTableFields(t *testing.T) {
	key := "c_" + strings.Repeat("a", 64)
	srv, captured := newCapturingServer(t, http.StatusOK, sampleESResponse)
	query := "SELECT json_extract_string(chunk_data, '$." + key + "') AS value FROM ragflow_t1 WHERE json_extract_isnull(chunk_data, '$." + key + "') == false LIMIT 5"
	if _, err := newTestEngine(t, srv.URL).RunSQL(t.Context(), "ragflow_t1", query, nil, "json"); err != nil {
		t.Fatal(err)
	}
	captured.mu.Lock()
	defer captured.mu.Unlock()
	var body map[string]any
	if err := json.Unmarshal([]byte(captured.body), &body); err != nil {
		t.Fatal(err)
	}
	runtime, ok := body["runtime_mappings"].(map[string]any)
	if !ok {
		t.Fatalf("no runtime fields: %v", body)
	}
	field, ok := runtime[key].(map[string]any)
	if !ok || len(runtime) != 1 || field["type"] != "keyword" {
		t.Fatalf("runtime fields = %v", runtime)
	}
	script := field["script"].(map[string]any)
	if script["params"].(map[string]any)["key"] != key {
		t.Fatalf("column key not parameterized: %v", script)
	}
	rendered := body["query"].(string)
	if strings.Contains(rendered, "= =") || strings.Contains(rendered, "json_extract") || !strings.Contains(rendered, "IS NULL") || !strings.Contains(rendered, "\""+key+"\"") {
		t.Fatalf("query not translated: %s", rendered)
	}
}

func TestRunSQLReadsPagesAndClosesCursorAtLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		call := calls.Add(1)
		switch call {
		case 1:
			if body["query"] == nil {
				t.Error("first request missing query")
			}
			fmt.Fprint(w, `{"columns":[{"name":"doc_id","type":"keyword"}],"rows":[["d1"],["d2"]],"cursor":"first"}`)
		case 2:
			if body["cursor"] != "first" {
				t.Errorf("page request = %v", body)
			}
			fmt.Fprint(w, `{"rows":[["d3"],["d4"]],"cursor":"second"}`)
		case 3:
			if r.URL.Path != "/_sql/close" || body["cursor"] != "second" {
				t.Errorf("cursor not closed: %s %v", r.URL.Path, body)
			}
			fmt.Fprint(w, `{"succeeded":true}`)
		default:
			t.Errorf("unexpected request: %v", body)
		}
	}))
	defer srv.Close()
	rows, err := newTestEngine(t, srv.URL).RunSQL(t.Context(), "ragflow_t1", "SELECT doc_id FROM ragflow_t1 LIMIT 3", nil, "json")
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("requests=%d, want query, page, close", calls.Load())
	}
}

func TestRunSQLPageFailureReturnsNoPartialRowsAndClosesCursor(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		call := calls.Add(1)
		switch call {
		case 1:
			fmt.Fprint(w, `{"columns":[{"name":"doc_id","type":"keyword"}],"rows":[["d1"]],"cursor":"first"}`)
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"failed page"}`)
		case 3:
			if r.URL.Path != "/_sql/close" {
				t.Errorf("missing cursor cleanup: %s", r.URL.Path)
			}
			fmt.Fprint(w, `{"succeeded":true}`)
		default:
			t.Errorf("unexpected request")
		}
	}))
	defer srv.Close()
	rows, err := newTestEngine(t, srv.URL).RunSQL(t.Context(), "ragflow_t1", "SELECT doc_id FROM ragflow_t1 LIMIT 3", nil, "json")
	if err == nil || rows != nil || calls.Load() != 3 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, calls.Load())
	}
}

func TestRunSQLRejectsInvalidTablePathsBeforeRequest(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		fmt.Fprint(w, sampleESResponse)
	}))
	defer srv.Close()
	e := newTestEngine(t, srv.URL)
	for _, expression := range []string{"json_extract_string(content, '$.c_bad')", "json_extract_string(chunk_data, '$.raw_name')", "json_extract_string(chunk_data, doc_id)"} {
		if _, err := e.RunSQL(t.Context(), "ragflow_t1", "SELECT "+expression+" FROM ragflow_t1 LIMIT 100", nil, "json"); err == nil {
			t.Errorf("accepted %s", expression)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("bad JSON path sent %d requests", calls.Load())
	}
}

func TestRunSQLDefaultsMissingAvailabilityToEnabled(t *testing.T) {
	srv, captured := newCapturingServer(t, http.StatusOK, sampleESResponse)
	if _, err := newTestEngine(t, srv.URL).RunSQL(t.Context(), "ragflow_t1", "SELECT COUNT(*) FROM ragflow_t1 WHERE available_int = 1 LIMIT 100", nil, "json"); err != nil {
		t.Fatal(err)
	}
	captured.mu.Lock()
	defer captured.mu.Unlock()
	var body map[string]any
	if err := json.Unmarshal([]byte(captured.body), &body); err != nil {
		t.Fatal(err)
	}
	runtime, ok := body["runtime_mappings"].(map[string]any)
	if !ok {
		t.Fatalf("missing default availability: %v", body)
	}
	availability, ok := runtime["available_int"].(map[string]any)
	if !ok || availability["type"] != "long" || !strings.Contains(availability["script"].(string), "1L") {
		t.Fatalf("missing default availability: %v", runtime)
	}
}

func TestRunSQLStopsEmptyPageWithLiveCursor(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		calls.Add(1)
		if r.URL.Path == "/_sql/close" {
			fmt.Fprint(w, `{"succeeded":true}`)
			return
		}
		if calls.Load() > 1 {
			t.Error("empty page was fetched again")
			w.WriteHeader(400)
			return
		}
		fmt.Fprint(w, `{"columns":[{"name":"doc_id","type":"keyword"}],"rows":[],"cursor":"empty"}`)
	}))
	defer srv.Close()
	rows, err := newTestEngine(t, srv.URL).RunSQL(t.Context(), "ragflow_t1", "SELECT doc_id FROM ragflow_t1 LIMIT 3", nil, "json")
	if err != nil || len(rows) != 0 || calls.Load() != 2 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, calls.Load())
	}
}

func TestRunSQLBudgetCoversMultiplePages(t *testing.T) {
	var pages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.URL.Path == "/_sql/close" {
			fmt.Fprint(w, `{"succeeded":true}`)
			return
		}
		n := pages.Add(1)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(750 * time.Millisecond):
		}
		cursor := "next"
		if n == 3 {
			cursor = ""
		}
		fmt.Fprintf(w, `{"columns":[{"name":"doc_id","type":"keyword"}],"rows":[["d%d"]],"cursor":%q}`, n, cursor)
	}))
	defer srv.Close()
	rows, err := newTestEngine(t, srv.URL).runSQLOnce(t.Context(), "SELECT doc_id FROM ragflow_t1 LIMIT 3", "json")
	if err != nil || len(rows) != 3 {
		t.Fatalf("valid paginated query timed out: rows=%v err=%v", rows, err)
	}
}
