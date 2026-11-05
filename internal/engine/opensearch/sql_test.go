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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"

	"github.com/opensearch-project/opensearch-go/v4"
)

// capturedRequest holds the request body the test server saw, for
// assertions.
type capturedRequest struct {
	mu       sync.Mutex
	path     string
	body     string
	method   string
	rawQuery string
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
		capRequest.rawQuery = r.URL.RawQuery
		capRequest.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(replyStatus)
		_, _ = w.Write([]byte(replyBody))
	}))
	t.Cleanup(srv.Close)
	return srv, capRequest
}

// newTestEngine constructs an Engine pointing at the given test server.
// Bypasses NewEngine (which calls the cluster Info endpoint to verify
// connectivity) — the test server is a stub, not a real cluster.
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
    {"name": "docnm", "type": "text"},
    {"name": "count", "type": "long"}
  ],
  "datarows": [
    ["d1", "report.pdf", 5],
    ["d2", "spec.pdf", 3]
  ],
  "total": 2,
  "size": 2
}`

// TestRunSQL_NoFilterAdded verifies the request body is exactly
// {"query": <sql>, "fetch_size": 128} — no `filter` field is added.
// (service.addKBFilter is the source of truth for kb_id scoping
// upstream of RunSQL.)
func TestRunSQL_NoFilterAdded(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleOSResponse)
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
		t.Errorf("RunSQL request must NOT include top-level filter (addKBFilter is the source of truth upstream). body=%v", body)
	}
	if _, has := body["query"]; !has {
		t.Errorf("RunSQL request must include query. body=%v", body)
	}
}

// TestRunSQL_WhitespaceNormalizedAndPercentStripped verifies the Python
// preprocessing step `re.sub(r"[ `]+", " ", sql)` + `sql.replace("%", "")`
// is applied.
func TestRunSQL_WhitespaceNormalizedAndPercentStripped(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleOSResponse)
	e := newTestEngine(t, srv.URL)
	ctx := t.Context()

	in := "SELECT   doc_id  FROM  `ragflow_t1`  WHERE  count  >  0  %"
	_, err := e.RunSQL(ctx, "ragflow_t1", in, nil, "json")
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

// TestRunSQL_PerAttemptTimeout verifies each attempt gets a 2s deadline
// and a timeout triggers one retry: 2s (first attempt) + 3s (sleep) +
// 2s (second attempt) ≈ 7s.
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

// TestRunSQL_RetryOnTimeoutThenSucceed simulates the Python
// ConnectionTimeout-retry pattern: the first attempt times out, the
// second attempt returns valid rows.
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
// path: a 4xx response must NOT trigger a retry and must be wrapped as
// `SQL error: <e>\n\nSQL: <sql>`.
func TestRunSQL_NonTimeoutErrorSurfacesImmediately(t *testing.T) {
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
	if !strings.Contains(err.Error(), "SQL error:") {
		t.Errorf("err: got %q, want substring 'SQL error:'", err.Error())
	}
	if !strings.Contains(err.Error(), "SQL: SELECT bad") {
		t.Errorf("err: got %q, want substring 'SQL: SELECT bad'", err.Error())
	}
}

// TestRunSQL_RequestBodyHasFetchSizeAndFormat verifies the request body
// includes fetch_size=128 and that the caller's ES-style "json" format is
// replaced with the JDBC format OpenSearch actually understands.
func TestRunSQL_RequestBodyHasFetchSizeAndFormat(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleOSResponse)
	e := newTestEngine(t, srv.URL)

	ctx := t.Context()
	if _, err := e.RunSQL(ctx, "ragflow_t1", "SELECT 1", nil, "json"); err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	cap.mu.Lock()
	got := cap.body
	query := cap.rawQuery
	cap.mu.Unlock()

	if query != "format=jdbc" {
		t.Errorf("query: got %q, want %q", query, "format=jdbc")
	}

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
	empty := `{"schema": [{"name": "doc_id", "type": "text"}], "datarows": []}`
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

// TestRunSQL_PostsToSQLPath verifies the request goes to the OpenSearch
// SQL plugin endpoint /_plugins/_sql, not Elasticsearch's /_sql.
func TestRunSQL_PostsToSQLPath(t *testing.T) {
	srv, cap := newCapturingServer(t, http.StatusOK, sampleOSResponse)
	e := newTestEngine(t, srv.URL)

	ctx := t.Context()
	if _, err := e.RunSQL(ctx, "ragflow_t1", "SELECT 1", nil, "json"); err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	cap.mu.Lock()
	got := cap.path
	cap.mu.Unlock()
	if got != "/_plugins/_sql" {
		t.Errorf("path: got %q, want /_plugins/_sql", got)
	}
}

// TestRunSQL_FollowsCursorPagination verifies the first response's
// cursor is sent back as {"cursor": ...} and rows from every page are
// concatenated.
func TestRunSQL_FollowsCursorPagination(t *testing.T) {
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
	rows, err := e.RunSQL(t.Context(), "ragflow_t1", "SELECT doc_id FROM ragflow_t1", nil, "json")
	if err != nil {
		t.Fatalf("RunSQL: %v", err)
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

// TestMain registers the engine as "infinity" so tokenizer.Tokenize and
// tokenizer.FineGrainedTokenize short-circuit and return the input
// as-is. This lets the rewrite tests assert on the SHAPE of the MATCH()
// substitution without depending on a real tokenizer pool.
func TestMain(m *testing.M) {
	// Suppress the client's background node-stats polling so the httptest
	// servers only observe requests issued by the code under test.
	_ = os.Setenv("OPENSEARCH_GO_DISCOVERY_CONFIG", "-node_stats")
	_ = common.InitLogger("info", common.FileOutput{}, "opensearch_test")
	tokenizer.SetEngineType("infinity")
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
