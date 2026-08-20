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

func jiraFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/project/RAG", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"key": "RAG"})
	})
	mux.HandleFunc("/rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("jql"); !strings.Contains(got, `project = "RAG"`) {
			t.Fatalf("jql = %q", got)
		}
		writeJSON(t, w, map[string]any{
			"issues": []map[string]any{{"id": "10000"}},
		})
	})
	var server *httptest.Server
	mux.HandleFunc("/rest/api/3/issue/bulkfetch", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"issues": []map[string]any{jiraFixtureIssue(server.URL)}})
	})
	mux.HandleFunc("/attachment/10001", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("attachment body"))
	})
	server = httptest.NewServer(mux)
	return server
}

func jiraFixtureIssue(baseURL string) map[string]any {
	return map[string]any{
		"id":  "10000",
		"key": "RAG-7",
		"fields": map[string]any{
			"summary":     "Implement Jira sync",
			"description": "Implement Jira sync in Go",
			"updated":     "2026-01-03T10:00:00.000+0000",
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

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode JSON failed: %v", err)
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
