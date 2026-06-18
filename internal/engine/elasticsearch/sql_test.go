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
	"regexp"
	"strings"
	"sync"
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
	cap := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap.mu.Lock()
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.body = string(body)
		cap.mu.Unlock()
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(replyStatus)
		_, _ = w.Write([]byte(replyBody))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

// newTestEngine constructs an elasticsearchEngine pointing at the given
// test server. Bypasses NewEngine (which calls ES Info to verify
// connectivity) — the test server is a stub, not a real ES cluster.
func newTestEngine(t *testing.T, srvURL string) *elasticsearchEngine {
	t.Helper()
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{srvURL},
	})
	if err != nil {
		t.Fatalf("elasticsearch.NewClient: %v", err)
	}
	return &elasticsearchEngine{client: client}
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
// implementation added is gone. (service.addKBFilter is the source of
// truth for kb_id scoping upstream of RunSQL.)
func TestRunSQL_NoFilterAdded(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)

	rows, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT doc_id FROM ragflow_t1", nil, "json")
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
		t.Errorf("RunSQL request must NOT include top-level filter (addKBFilter is the source of truth upstream). body=%v", body)
	}
	if _, has := body["query"]; !has {
		t.Errorf("RunSQL request must include query. body=%v", body)
	}
}

// TestRunSQL_WhitespaceNormalizedAndPercentStripped verifies the Python
// preprocessing step `re.sub(r"[ `]+", " ", sql)` + `sql.replace("%", "")`
// is applied. Without these, the LLM-generated SQL with stray backticks
// or `%` characters (e.g. from JSON decoding glitches) would fail to
// parse in ES.
func TestRunSQL_WhitespaceNormalizedAndPercentStripped(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleESResponse)
	e := newTestEngine(t, srv.URL)

	// Input SQL has multiple backticks/spaces and trailing % characters.
	in := "SELECT   doc_id  FROM  `ragflow_t1`  WHERE  count  >  0  %"
	_, err := e.RunSQL(context.Background(), "ragflow_t1", in, nil, "json")
	if err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	cap.mu.Lock()
	got := cap.body
	cap.mu.Unlock()

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(got), &body); err != nil {
		t.Fatalf("body is not JSON: %v\nbody=%q", err, got)
	}
	q, _ := body["query"].(string)
	if strings.Contains(q, "  ") {
		t.Errorf("query still has multiple spaces (whitespace not normalized): %q", q)
	}
	if strings.Contains(q, "`") {
		t.Errorf("query still has backticks (whitespace+backtick regex not applied): %q", q)
	}
	if strings.Contains(q, "%") {
		t.Errorf("query still has %% (percent strip not applied): %q", q)
	}
}

// TestRunSQL_PerAttemptTimeout verifies the derived context has a 2s
// deadline. We send a hanging response from the test server and assert
// the call returns well before 30s (the Go ES client's default
// transport-level timeout). With the retry loop in place, the total
// time is 2s (first attempt) + 3s (sleep) + 2s (second attempt) = ~7s.
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

	start := time.Now()
	_, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT 1", nil, "json")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("RunSQL: got nil error, want timeout error")
	}
	// 2s + 3s + 2s = 7s; allow generous upper bound for the test runner.
	if elapsed < 6*time.Second {
		t.Errorf("RunSQL returned in %s; expected ~7s (2 attempts + 3s sleep)", elapsed)
	}
	if elapsed > 10*time.Second {
		t.Errorf("RunSQL took %s; expected ~7s, looks like the retry didn't fire", elapsed)
	}
	// The final error should mention timeout + 2 attempts.
	if !strings.Contains(err.Error(), "timeout after 2 attempts") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "timeout after 2 attempts")
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
	rows, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT 1", nil, "json")
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
	_, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT bad", nil, "json")
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

	if _, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT 1", nil, "json"); err != nil {
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

	rows, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT doc_id FROM ragflow_t1", nil, "json")
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

	if _, err := e.RunSQL(context.Background(), "ragflow_t1", "SELECT 1", nil, "json"); err != nil {
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
	tokenizer.RegisterEngineType(func() string { return "infinity" })
	m.Run()
}

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
		if got := Preprocess(c.in); got != c.want {
			t.Errorf("Preprocess(%q) = %q, want %q", c.in, got, c.want)
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
		if got := Preprocess(c.in); got != c.want {
			t.Errorf("Preprocess(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocess_LktksRewrite(t *testing.T) {
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
			"MATCH(content_ltks,",
		},
		{
			"= with multi-word value (ltks suffix)",
			"select content_ltks = 'final report'",
			"content_ltks",
			"MATCH(content_ltks,",
		},
		{
			"tks (no l) suffix",
			"select title_tks = 'hello'",
			"title_tks",
			"MATCH(title_tks,",
		},
		{
			"leading-space anchor: no leading space means no match (mirrors Python regex)",
			"content_ltks like 'weather'",
			"content_ltks",
			"content_ltks like 'weather'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Preprocess(c.in)
			isAnchorTest := c.expect == c.in
			if isAnchorTest {
				if got != c.in {
					t.Errorf("Preprocess(%q) = %q, want unchanged (leading-space anchor should prevent match)", c.in, got)
				}
				return
			}
			if strings.Contains(got, c.field+" ") {
				pattern := regexp.MustCompile(c.field + `( like | ?= ?)`)
				if pattern.MatchString(got) {
					t.Errorf("Preprocess(%q) = %q, still contains the original `<field> like/=` pattern", c.in, got)
				}
			}
			if !strings.Contains(got, c.expect) {
				t.Errorf("Preprocess(%q) = %q, want substring %q", c.in, got, c.expect)
			}
			if !strings.Contains(got, "minimum_should_match=30") {
				t.Errorf("Preprocess(%q) = %q, want substring minimum_should_match=30", c.in, got)
			}
		})
	}
}

func TestPreprocess_NoMatchLeavesSQLAlone(t *testing.T) {
	in := "SELECT doc_id FROM ragflow_t1"
	got := Preprocess(in)
	if got != in {
		t.Errorf("Preprocess(%q) = %q, want unchanged", in, got)
	}
}

// fakeNetTimeoutErr implements net.Error with Timeout()==true.
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
