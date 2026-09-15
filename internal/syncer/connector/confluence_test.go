package connector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestConfluenceConnectorSyncPagesCommentsAndAttachments(t *testing.T) {
	server := newConfluenceFixtureServer(t, http.StatusOK)
	defer server.Close()

	connector := newTestConfluenceConnector(t, server.URL, map[string]any{
		"index_mode":        "page",
		"page_id":           "123",
		"index_recursively": true,
		"batch_size":        10,
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     confluenceTestTime(t, "2026-01-03T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync() error = %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch() error = %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("len(documents) = %d, want 2", len(batch.Documents))
	}

	page := batch.Documents[0]
	if page.SourceID != server.URL+"/wiki/display/SPACE/Page" {
		t.Fatalf("page SourceID = %q", page.SourceID)
	}
	if page.SemanticIdentifier != "Engineering / Root / Page" {
		t.Fatalf("page SemanticIdentifier = %q", page.SemanticIdentifier)
	}
	if !strings.Contains(string(page.Blob), "Hello Confluence") || !strings.Contains(string(page.Blob), "Comment text") {
		t.Fatalf("page blob did not include page and comment text: %q", string(page.Blob))
	}
	if page.Extension != ".txt" {
		t.Fatalf("page Extension = %q", page.Extension)
	}
	if page.Metadata["space"] != "Engineering" {
		t.Fatalf("page metadata = %#v", page.Metadata)
	}
	if page.Fingerprint != contentFingerprint(page.Blob) {
		t.Fatalf("page fingerprint mismatch")
	}

	attachment := batch.Documents[1]
	if attachment.SourceID != server.URL+"/wiki/download/attachments/file.txt?version=1" {
		t.Fatalf("attachment SourceID = %q", attachment.SourceID)
	}
	if attachment.Extension != ".txt" {
		t.Fatalf("attachment Extension = %q", attachment.Extension)
	}
	if attachment.SemanticIdentifier != "Engineering / Root / Page / file.txt" {
		t.Fatalf("attachment SemanticIdentifier = %q", attachment.SemanticIdentifier)
	}
	if string(attachment.Blob) != "attachment bytes" {
		t.Fatalf("attachment Blob = %q", string(attachment.Blob))
	}
	if attachment.Metadata["parent_page_id"] != page.SourceID {
		t.Fatalf("attachment metadata = %#v", attachment.Metadata)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != attachment.SourceID {
		t.Fatalf("checkpoint = %#v", batch.Checkpoint)
	}

	_, err = session.NextBatch(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("second NextBatch() error = %v, want EOF", err)
	}
}

func TestConfluenceConnectorResume(t *testing.T) {
	server := newConfluenceFixtureServer(t, http.StatusOK)
	defer server.Close()

	connector := newTestConfluenceConnector(t, server.URL, map[string]any{"batch_size": 1})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     confluenceTestTime(t, "2026-01-03T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync() error = %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch() error = %v", err)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     confluenceTestTime(t, "2026-01-03T00:00:00Z"),
		Resume:        first.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resumed OpenSync() error = %v", err)
	}
	batch, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch() error = %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].Extension != ".txt" || batch.Documents[0].SourceID == first.Documents[0].SourceID {
		t.Fatalf("resumed batch = %#v first = %#v", batch.Documents, first.Documents)
	}
}

func TestConfluenceConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	server := newConfluenceFixtureServer(t, http.StatusOK)
	defer server.Close()

	connector := newTestConfluenceConnector(t, server.URL, map[string]any{"batch_size": 1})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     confluenceTestTime(t, "2026-01-03T00:00:00Z"),
		Resume:        &SyncCheckpoint{},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestConfluenceConnectorPrune(t *testing.T) {
	server := newConfluenceFixtureServer(t, http.StatusOK)
	defer server.Close()

	connector := newTestConfluenceConnector(t, server.URL, nil)
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune() error = %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch() error = %v", err)
	}
	got := map[string]bool{}
	for _, doc := range batch.Documents {
		got[doc.SourceID] = true
	}
	for _, want := range []string{
		server.URL + "/wiki/display/SPACE/Page",
		server.URL + "/wiki/download/attachments/file.txt?version=1",
	} {
		if !got[want] {
			t.Fatalf("missing prune source id %q in %#v", want, batch.Documents)
		}
	}
}

func TestConfluenceConnectorValidateMapsUnauthorized(t *testing.T) {
	server := newConfluenceFixtureServer(t, http.StatusUnauthorized)
	defer server.Close()

	connector := newTestConfluenceConnector(t, server.URL, nil)
	err := connector.ValidateConnectorSetting(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid or expired") {
		t.Fatalf("ValidateConnectorSetting() error = %v", err)
	}
}

func newTestConfluenceConnector(t *testing.T, wikiBase string, overrides map[string]any) *ConfluenceConnector {
	t.Helper()
	config := map[string]any{
		"wiki_base":  wikiBase,
		"is_cloud":   true,
		"batch_size": 10,
		"credentials": map[string]any{
			"confluence_username":     "user@example.com",
			"confluence_access_token": "token",
		},
	}
	for key, value := range overrides {
		config[key] = value
	}
	connector, err := NewConfluenceConnector(config)
	if err != nil {
		t.Fatalf("NewConfluenceConnector() error = %v", err)
	}
	return connector
}

func newConfluenceFixtureServer(t *testing.T, validateStatus int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if validateStatus != http.StatusOK && strings.HasPrefix(r.URL.Path, "/wiki/rest/api/space") {
			http.Error(w, "unauthorized", validateStatus)
			return
		}
		if !confluenceHasBasicAuth(r, "user@example.com", "token") {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/wiki/rest/api/space":
			writeConfluenceJSON(t, w, map[string]any{"results": []map[string]any{{"key": "SPACE", "name": "Engineering"}}})
		case r.URL.Path == "/wiki/rest/api/space/SPACE":
			writeConfluenceJSON(t, w, map[string]any{"key": "SPACE", "name": "Engineering"})
		case r.URL.Path == "/wiki/rest/api/content/search":
			cql := r.URL.Query().Get("cql")
			switch {
			case strings.Contains(cql, "type=page"):
				writeConfluenceJSON(t, w, map[string]any{"results": []map[string]any{confluenceFixturePage()}})
			case strings.Contains(cql, "type=comment"):
				writeConfluenceJSON(t, w, map[string]any{"results": []map[string]any{confluenceFixtureComment()}})
			case strings.Contains(cql, "type=attachment"):
				writeConfluenceJSON(t, w, map[string]any{"results": []map[string]any{confluenceFixtureAttachment()}})
			default:
				t.Errorf("Unexpected CQL: %q", cql)
				http.Error(w, "unexpected cql", http.StatusInternalServerError)
			}
		case r.URL.Path == "/wiki/download/attachments/file.txt":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("attachment bytes"))
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	return server
}

func confluenceHasBasicAuth(r *http.Request, username, password string) bool {
	const prefix = "Basic "
	raw := r.Header.Get("Authorization")
	if !strings.HasPrefix(raw, prefix) {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, prefix))
	if err != nil {
		return false
	}
	return string(decoded) == username+":"+password
}

func writeConfluenceJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Errorf("encode fixture: %v", err)
	}
}

func confluenceFixturePage() map[string]any {
	return map[string]any{
		"id":    123,
		"type":  "page",
		"title": "Page",
		"body":  map[string]any{"storage": map[string]any{"value": "<p>Hello <strong>Confluence</strong></p>"}},
		"space": map[string]any{"key": "SPACE", "name": "Engineering"},
		"ancestors": []map[string]any{
			{"title": "Root"},
		},
		"version":  map[string]any{"when": "2026-01-01T10:00:00Z"},
		"metadata": map[string]any{"labels": map[string]any{"results": []map[string]any{{"name": "docs"}}}},
		"_links":   map[string]any{"webui": "/display/SPACE/Page"},
	}
}

func confluenceFixtureComment() map[string]any {
	return map[string]any{
		"id":      456,
		"type":    "comment",
		"title":   "Comment",
		"body":    map[string]any{"storage": map[string]any{"value": "<p>Comment text</p>"}},
		"version": map[string]any{"when": "2026-01-01T11:00:00Z"},
		"_links":  map[string]any{"webui": "/display/SPACE/Page?focusedCommentId=c1"},
	}
}

func confluenceFixtureAttachment() map[string]any {
	return map[string]any{
		"id":    789,
		"type":  "attachment",
		"title": "file.txt",
		"space": map[string]any{"key": "SPACE", "name": "Engineering"},
		"metadata": map[string]any{
			"mediaType": "text/plain",
			"labels":    map[string]any{"results": []map[string]any{{"name": "file"}}},
		},
		"extensions": map[string]any{"fileSize": 16},
		"version":    map[string]any{"when": "2026-01-01T12:00:00Z"},
		"_links": map[string]any{
			"webui":    "/download/attachments/file.txt?version=1",
			"download": "/download/attachments/file.txt?version=1",
		},
	}
}

func confluenceTestTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse test time %q: %v", value, err)
	}
	return parsed
}

func TestBuildConfluenceDocumentIDAddsWikiForCloud(t *testing.T) {
	got := buildConfluenceDocumentID("https://example.atlassian.net", "/display/SPACE/Page", true)
	want := "https://example.atlassian.net/wiki/display/SPACE/Page"
	if got != want {
		t.Fatalf("buildConfluenceDocumentID() = %q, want %q", got, want)
	}

	got = buildConfluenceDocumentID("https://example.atlassian.net/wiki", "/display/SPACE/Page", true)
	if got != want {
		t.Fatalf("buildConfluenceDocumentID() with wiki base = %q, want %q", got, want)
	}
}

func TestConfluenceCQLPathEscapesQuery(t *testing.T) {
	raw := confluenceCQLPath("type=page and space='A B'", "version", 10)
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if parsed.Query().Get("cql") != "type=page and space='A B'" {
		t.Fatalf("cql = %q", parsed.Query().Get("cql"))
	}
}

func TestConfluenceCQLQuoteEscapesBackslashes(t *testing.T) {
	got := confluenceCQLQuote(`path\name's`)
	want := `path\\name\'s`
	if got != want {
		t.Fatalf("confluenceCQLQuote() = %q, want %q", got, want)
	}
}

func TestConfluenceSemanticIdentifierAlwaysUsesFullPath(t *testing.T) {
	// Every identifier carries the full hierarchical path: space, ancestors, title.
	got := confluenceSemanticIdentifier("Engineering", []string{"Root"}, "Page")
	if got != "Engineering / Root / Page" {
		t.Fatalf("first occurrence identifier = %q, want full path", got)
	}
	if again := confluenceSemanticIdentifier("Engineering", []string{"Root"}, "Page"); again != got {
		t.Fatalf("later occurrence identifier = %q, want stable %q", again, got)
	}
	if bare := confluenceSemanticIdentifier("", nil, "Standalone"); bare != "Standalone" {
		t.Fatalf("no-space identifier = %q, want title only", bare)
	}
	if unnamed := confluenceSemanticIdentifier("Space", nil, ""); unnamed != "Space / Untitled" {
		t.Fatalf("untitled identifier = %q, want Space / Untitled", unnamed)
	}
}
