package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJiraConnectorOpenSyncCloud(t *testing.T) {
	server := jiraFixtureServer(t)
	defer server.Close()

	connector, err := NewJiraConnector(map[string]any{
		"base_url":            server.URL,
		"project_key":         "RAG",
		"batch_size":          10,
		"include_comments":    true,
		"include_attachments": true,
		"timezone_offset":     0,
		"credentials": map[string]any{
			"jira_user_email":  "alice@example.com",
			"jira_api_token":   "token",
			"rest_api_version": "3",
		},
	})
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}

	start := mustTime(t, "2026-01-02T12:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(batch.Documents))
	}
	issue := batch.Documents[0]
	if issue.SourceID != server.URL+"/browse/RAG-7" {
		t.Fatalf("issue source id = %q", issue.SourceID)
	}
	if issue.Extension != ".md" {
		t.Fatalf("issue extension = %q", issue.Extension)
	}
	body := string(issue.Blob)
	for _, want := range []string{"key: RAG-7", "## Description", "Implement Jira sync", "## Comments", "Looks good"} {
		if !strings.Contains(body, want) {
			t.Fatalf("issue body missing %q:\n%s", want, body)
		}
	}
	attachment := batch.Documents[1]
	if attachment.SourceID != "RAG-7::attachment::10001" {
		t.Fatalf("attachment source id = %q", attachment.SourceID)
	}
	if string(attachment.Blob) != "attachment body" {
		t.Fatalf("attachment body = %q", attachment.Blob)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestJiraConnectorOpenSyncResumeSkipsCommittedSource(t *testing.T) {
	server := jiraResumeFixtureServer(t, []map[string]any{
		{"id": "10000", "key": "RAG-7"},
		{"id": "10001", "key": "RAG-8"},
	})
	defer server.Close()

	connector, err := NewJiraConnector(map[string]any{
		"base_url":        server.URL,
		"project_key":     "RAG",
		"batch_size":      10,
		"timezone_offset": 0,
		"credentials": map[string]any{
			"jira_api_token":   "token",
			"rest_api_version": "3",
		},
	})
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}
	cursorData, err := json.Marshal(jiraSyncCursor{
		StartAt:  0,
		SourceID: server.URL + "/browse/RAG-7",
	})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   string(cursorData),
			SourceID: server.URL + "/browse/RAG-7",
		},
	})
	if err != nil {
		t.Fatalf("resumed OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != server.URL+"/browse/RAG-8" {
		t.Fatalf("resumed documents = %+v, want RAG-8 only", batch.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resumed final NextBatch = %v, want io.EOF", err)
	}
}

func TestJiraConnectorOpenSyncResumeRejectsInvalidCheckpoint(t *testing.T) {
	server := jiraFixtureServer(t)
	defer server.Close()
	connector := mustJiraResumeConnector(t, server.URL)
	cases := map[string]*SyncCheckpoint{
		"missing":   {},
		"malformed": {Cursor: "not-json"},
		"no-anchor": {Cursor: `{"start_at":1}`},
		"foreign":   {Cursor: `{"start_at":1,"source_id":"https://other.example/browse/RAG-7"}`},
	}
	for name, checkpoint := range cases {
		t.Run(name, func(t *testing.T) {
			session, err := connector.OpenSync(context.Background(), SyncRequest{
				FromBeginning: true,
				Resume:        checkpoint,
			})
			if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
			}
		})
	}
}

func TestJiraConnectorOpenSyncResumeRejectsMissingIssue(t *testing.T) {
	server := jiraResumeFixtureServer(t, []map[string]any{})
	defer server.Close()
	connector := mustJiraResumeConnector(t, server.URL)
	cursorData, err := json.Marshal(jiraSyncCursor{
		StartAt:  0,
		SourceID: server.URL + "/browse/RAG-999",
	})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   string(cursorData),
			SourceID: server.URL + "/browse/RAG-999",
		},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestJiraConnectorOpenPrune(t *testing.T) {
	server := jiraFixtureServer(t)
	defer server.Close()

	connector, err := NewJiraConnector(map[string]any{
		"base_url":        server.URL,
		"project_key":     "RAG",
		"timezone_offset": 0,
		"credentials": map[string]any{
			"jira_api_token":   "token",
			"rest_api_version": "3",
		},
	})
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != server.URL+"/browse/RAG-7" {
		t.Fatalf("prune docs = %+v", batch.Documents)
	}
}

func TestJiraConnectorRegisteredBuiltIn(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("jira", map[string]any{
		"base_url":    "https://jira.example.com",
		"project_key": "RAG",
		"credentials": map[string]any{
			"jira_api_token": "token",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*JiraConnector); !ok {
		t.Fatalf("connector type = %T, want *JiraConnector", connector)
	}
}

func TestJiraConnectorBuildJQLMovesUserOrderAfterFilters(t *testing.T) {
	start := mustTime(t, "2026-01-02T12:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	connector := &JiraConnector{
		jqlQuery:       `status = "Done" ORDER BY priority DESC, updated ASC`,
		timezoneOffset: 0,
	}

	got := connector.buildJQL(&start, end)
	want := `(status = "Done") AND updated >= "2026-01-02 12:00" AND updated < "2026-01-04 00:01" ORDER BY priority DESC, updated ASC`
	if got != want {
		t.Fatalf("buildJQL() = %q, want %q", got, want)
	}
}

// TestJiraConnectorBuildJQLUpperBoundCoversWholeMinute pins the exclusive upper
// bound. JQL cannot express the seconds of the window end, so an inclusive `<=`
// on the truncated value drops the rest of that minute and the next window's
// lower bound skips those issues forever.
func TestJiraConnectorBuildJQLUpperBoundCoversWholeMinute(t *testing.T) {
	end := mustTime(t, "2026-01-04T00:00:30Z")
	connector := &JiraConnector{
		jqlQuery:       `status = "Done"`,
		timezoneOffset: 0,
	}

	got := connector.buildJQL(nil, end)
	if !strings.Contains(got, `updated < "2026-01-04 00:01"`) {
		t.Fatalf("buildJQL() = %q, want an exclusive bound at the next minute tick", got)
	}
	if strings.Contains(got, `updated <=`) {
		t.Fatalf("buildJQL() = %q, want no inclusive upper bound", got)
	}
}

// TestJiraSyncSessionDropsIssuesPastWindowEnd covers the client half: the JQL
// upper bound spans the whole minute holding the window end, so issues updated
// after that end come back from the server and must be dropped locally. The next
// window re-queries them from its own lower bound.
func TestJiraSyncSessionDropsIssuesPastWindowEnd(t *testing.T) {
	withConnectorLoopbackTestHook(t)
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/project/RAG", func(w http.ResponseWriter, r *http.Request) {
		jiraWriteJSON(t, w, map[string]any{"key": "RAG"})
	})
	mux.HandleFunc("/rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		jiraWriteJSON(t, w, map[string]any{"issues": []map[string]any{
			{"id": "10000"},
			{"id": "10001"},
		}})
	})
	mux.HandleFunc("/rest/api/3/issue/bulkfetch", func(w http.ResponseWriter, r *http.Request) {
		inWindow := jiraFixtureIssue(server.URL, "2026-01-03T10:00:00.000+0000")
		inWindow["id"] = "10000"
		inWindow["key"] = "RAG-7"
		inWindow["fields"].(map[string]any)["attachment"] = []map[string]any{}

		pastWindow := jiraFixtureIssue(server.URL, "2026-01-03T10:00:45.000+0000")
		pastWindow["id"] = "10001"
		pastWindow["key"] = "RAG-8"
		pastWindow["fields"].(map[string]any)["attachment"] = []map[string]any{}

		jiraWriteJSON(t, w, map[string]any{"issues": []map[string]any{inWindow, pastWindow}})
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	connector := mustJiraResumeConnector(t, server.URL)
	start := mustTime(t, "2026-01-03T09:59:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-03T10:00:30Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents = %d, want 1 (only the issue updated at or before the window end)", len(batch.Documents))
	}
	if got := batch.Documents[0].SourceID; got != server.URL+"/browse/RAG-7" {
		t.Fatalf("document source = %q, want the in-window issue", got)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("second NextBatch = %v, want io.EOF", err)
	}
}

func TestJiraConnectorBuildJQLKeepsDefaultOrder(t *testing.T) {
	connector := &JiraConnector{
		jqlQuery:       `status = "Done"`,
		timezoneOffset: 0,
	}

	got := connector.buildJQL(nil, time.Time{})
	want := `(status = "Done") ORDER BY updated ASC`
	if got != want {
		t.Fatalf("buildJQL() = %q, want %q", got, want)
	}
}

func TestJiraConnectorBuildJQLIgnoresQuotedOrderBy(t *testing.T) {
	connector := &JiraConnector{
		jqlQuery:       `summary ~ "order by updated"`,
		timezoneOffset: 0,
	}

	got := connector.buildJQL(nil, time.Time{})
	want := `(summary ~ "order by updated") ORDER BY updated ASC`
	if got != want {
		t.Fatalf("buildJQL() = %q, want %q", got, want)
	}
}

func TestJiraConnectorDownloadRejectsSchemeMismatch(t *testing.T) {
	withConnectorLoopbackTestHook(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be sent for mismatched scheme")
	}))
	defer server.Close()

	connector, err := NewJiraConnector(map[string]any{
		"base_url":    server.URL,
		"project_key": "RAG",
		"credentials": map[string]any{
			"jira_api_token": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}
	attachmentURL := "https://" + strings.TrimPrefix(server.URL, "http://") + "/attachment"

	if _, err = connector.downloadURL(context.Background(), attachmentURL); err == nil {
		t.Fatalf("downloadURL should reject attachment URL scheme mismatch")
	}
}

func TestJiraConnectorDownloadRejectsCrossOriginRedirect(t *testing.T) {
	withConnectorLoopbackTestHook(t)
	destinationHit := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationHit = true
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/attachment", http.StatusFound)
	}))
	defer source.Close()

	connector, err := NewJiraConnector(map[string]any{
		"base_url":    source.URL,
		"project_key": "RAG",
		"credentials": map[string]any{
			"jira_api_token": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}

	if _, err = connector.downloadURL(context.Background(), source.URL+"/redirect"); err == nil {
		t.Fatalf("downloadURL should reject cross-origin redirect")
	}
	if destinationHit {
		t.Fatalf("cross-origin redirect target was requested")
	}
}

func TestJiraConnectorDownloadKeepsAuthorizationOnSameOriginRedirect(t *testing.T) {
	withConnectorLoopbackTestHook(t)
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/attachment", http.StatusFound)
	})
	mux.HandleFunc("/attachment", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization header = %q, want Bearer token", got)
		}
		_, _ = w.Write([]byte("attachment body"))
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	connector, err := NewJiraConnector(map[string]any{
		"base_url":    server.URL,
		"project_key": "RAG",
		"credentials": map[string]any{
			"jira_api_token": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}

	blob, err := connector.downloadURL(context.Background(), server.URL+"/redirect")
	if err != nil {
		t.Fatalf("downloadURL failed: %v", err)
	}
	if string(blob) != "attachment body" {
		t.Fatalf("downloaded body = %q", blob)
	}
}

const jiraFixtureUpdatedAt = "2026-01-03T10:00:00.000+0000"

func jiraFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return jiraFixtureServerWithUpdated(t, jiraFixtureUpdatedAt, "")
}

// jiraFixtureServerWithUpdated serves the issue regardless of the JQL filter, so
// wantJQL lets a test pin the lower bound the connector actually requested.
func jiraFixtureServerWithUpdated(t *testing.T, updated, wantJQL string) *httptest.Server {
	t.Helper()
	withConnectorLoopbackTestHook(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/project/RAG", func(w http.ResponseWriter, r *http.Request) {
		jiraWriteJSON(t, w, map[string]any{"key": "RAG"})
	})
	mux.HandleFunc("/rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("jql")
		if !strings.Contains(got, `project = "RAG"`) {
			t.Errorf("jql = %q", got)
			return
		}
		if wantJQL != "" && !strings.Contains(got, wantJQL) {
			t.Errorf("jql = %q, want it to contain %q", got, wantJQL)
			return
		}
		jiraWriteJSON(t, w, map[string]any{
			"issues": []map[string]any{{"id": "10000"}},
		})
	})
	var server *httptest.Server
	mux.HandleFunc("/rest/api/3/issue/bulkfetch", func(w http.ResponseWriter, r *http.Request) {
		jiraWriteJSON(t, w, map[string]any{"issues": []map[string]any{jiraFixtureIssue(server.URL, updated)}})
	})
	mux.HandleFunc("/attachment/10001", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("attachment body"))
	})
	server = httptest.NewServer(mux)
	return server
}

func jiraResumeFixtureServer(t *testing.T, keys []map[string]any) *httptest.Server {
	t.Helper()
	withConnectorLoopbackTestHook(t)
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/project/RAG", func(w http.ResponseWriter, r *http.Request) {
		jiraWriteJSON(t, w, map[string]any{"key": "RAG"})
	})
	mux.HandleFunc("/rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		jiraWriteJSON(t, w, map[string]any{"issues": keys})
	})
	mux.HandleFunc("/rest/api/3/issue/bulkfetch", func(w http.ResponseWriter, r *http.Request) {
		issues := make([]map[string]any, 0, len(keys))
		for _, meta := range keys {
			issue := jiraFixtureIssue(server.URL, jiraFixtureUpdatedAt)
			issue["id"] = meta["id"]
			issue["key"] = meta["key"]
			issues = append(issues, issue)
		}
		jiraWriteJSON(t, w, map[string]any{"issues": issues})
	})
	server = httptest.NewServer(mux)
	return server
}

func mustJiraResumeConnector(t *testing.T, baseURL string, overrides ...map[string]any) *JiraConnector {
	t.Helper()
	config := map[string]any{
		"base_url":        baseURL,
		"project_key":     "RAG",
		"batch_size":      10,
		"timezone_offset": 0,
		"credentials": map[string]any{
			"jira_api_token":   "token",
			"rest_api_version": "3",
		},
	}
	for _, override := range overrides {
		for key, value := range override {
			config[key] = value
		}
	}
	connector, err := NewJiraConnector(config)
	if err != nil {
		t.Fatalf("NewJiraConnector failed: %v", err)
	}
	return connector
}

func jiraFixtureIssue(baseURL, updated string) map[string]any {
	return map[string]any{
		"id":  "10000",
		"key": "RAG-7",
		"fields": map[string]any{
			"summary":     "Implement Jira sync",
			"description": "Implement Jira sync in Go",
			"updated":     updated,
			"created":     "2026-01-02T10:00:00.000+0000",
			"status":      map[string]any{"name": "Open"},
			"priority":    map[string]any{"name": "High"},
			"issuetype":   map[string]any{"name": "Task"},
			"project":     map[string]any{"name": "RAGFlow", "key": "RAG"},
			"reporter":    map[string]any{"displayName": "Alice", "emailAddress": "alice@example.com"},
			"assignee":    map[string]any{"displayName": "Bob", "emailAddress": "bob@example.com"},
			"labels":      []string{"sync"},
			"comment": map[string]any{"comments": []map[string]any{{
				"author":  map[string]any{"displayName": "Carol", "emailAddress": "carol@example.com"},
				"created": "2026-01-03T11:00:00.000+0000",
				"body":    "Looks good",
			}}},
			"attachment": []map[string]any{{
				"id":       "10001",
				"filename": "note.txt",
				"content":  baseURL + "/attachment/10001",
				"size":     15,
				"created":  "2026-01-03T12:00:00.000+0000",
			}},
		},
	}
}

func jiraWriteJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("Encode JSON failed: %v", err)
	}
}

func TestJiraTimeParsesOffset(t *testing.T) {
	var value jiraTime
	if err := value.UnmarshalJSON([]byte(`"2026-01-03T10:00:00.000+0800"`)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	want := time.Date(2026, 1, 3, 2, 0, 0, 0, time.UTC)
	if got := value.Time(); !got.Equal(want) {
		t.Fatalf("time = %s, want %s", got, want)
	}
}

func TestJiraSyncKeepsIssuesInsideTheWatermarkMinute(t *testing.T) {
	// The fixture issue was updated at 2026-01-03T10:00:00Z. A waterline at
	// 10:00:20Z asks JQL for `updated >= "2026-01-03 09:59"` (time buffer plus
	// minute precision); the page filter must accept that same span, or the
	// buffer only widens the response and the issue is dropped by both runs.
	server := jiraFixtureServer(t)
	defer server.Close()

	connector := mustJiraResumeConnector(t, server.URL)
	start := mustTime(t, "2026-01-03T10:00:20Z")
	end := mustTime(t, "2026-01-03T11:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != server.URL+"/browse/RAG-7" {
		t.Fatalf("documents = %#v, want the issue updated at 10:00:00Z", batch.Documents)
	}
}

func TestJiraSyncHonoursConfiguredTimeBuffer(t *testing.T) {
	// The issue was updated at 09:58:10Z. A configured 120s buffer — twice the
	// default — moves the 10:00:20Z watermark back to 09:58:20, floored to
	// 09:58, so both the requested JQL bound and the page filter must keep the
	// row. With the 60s default the bound would be 09:59 and the row dropped.
	server := jiraFixtureServerWithUpdated(t, "2026-01-03T09:58:10.000+0000", `updated >= "2026-01-03 09:58"`)
	defer server.Close()

	connector := mustJiraResumeConnector(t, server.URL, map[string]any{"time_buffer_seconds": 120})
	start := mustTime(t, "2026-01-03T10:00:20Z")
	end := mustTime(t, "2026-01-03T11:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != server.URL+"/browse/RAG-7" {
		t.Fatalf("documents = %#v, want the issue updated at 09:58:10Z", batch.Documents)
	}
}
