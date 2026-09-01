package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestGitlabConnectorOpenSyncUsesWindowAndFingerprint verifies incremental sync emits only updated docs with fingerprints.
func TestGitlabConnectorOpenSyncUsesWindowAndFingerprint(t *testing.T) {
	connector, err := NewGitlabConnector(map[string]any{
		"project_owner":      "owner",
		"project_name":       "repo",
		"include_mrs":        true,
		"include_issues":     true,
		"include_code_files": false,
		"batch_size":         10,
		"credentials":        map[string]any{"gitlab_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewGitlabConnector failed: %v", err)
	}
	connector.baseURL = "https://gitlab.com/api/v4"
	connector.doJSON = gitlabFixtureDoJSON(t)

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
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "https://gitlab.com/owner/repo/-/merge_requests/7" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestGitlabFingerprintStable verifies GitLab fingerprints are stable and content-sensitive.
func TestGitlabFingerprintStable(t *testing.T) {
	updatedAt := mustTime(t, "2026-01-03T00:00:00Z")
	mr := gitlabMergeRequest{
		IID:         7,
		Title:       "Add syncer",
		Description: "MR body",
		State:       "opened",
		WebURL:      "https://gitlab.com/owner/repo/-/merge_requests/7",
		UpdatedAt:   updatedAt,
		Author:      &gitlabUser{Name: "Alice", Username: "alice"},
	}
	fp1 := mr.toSourceDocument().Fingerprint
	fp2 := mr.toSourceDocument().Fingerprint
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprint unstable: %q %q", fp1, fp2)
	}

	changed := mr
	changed.Title = "Add syncer v2"
	if got := changed.toSourceDocument().Fingerprint; got == fp1 {
		t.Fatalf("fingerprint did not change after title update")
	}
}

func TestGitlabCodeFileFingerprintUsesTreeItemID(t *testing.T) {
	item := gitlabTreeItem{ID: "abc", Name: "main.go", Type: "blob", Path: "main.go"}
	base := gitlabCodeFileFingerprint(item, "main")
	if base == "" {
		t.Fatalf("fingerprint is empty")
	}
	if got := gitlabCodeFileFingerprint(item, "main"); got != base {
		t.Fatalf("fingerprint unstable: %q %q", base, got)
	}

	changed := item
	changed.ID = "def"
	if got := gitlabCodeFileFingerprint(changed, "main"); got == base {
		t.Fatalf("fingerprint did not change after tree item ID update")
	}
}

// TestGitlabConnectorOpenPrune verifies PRUNE returns correct web_url IDs.
func TestGitlabConnectorOpenPrune(t *testing.T) {
	connector, err := NewGitlabConnector(map[string]any{
		"project_owner":      "owner",
		"project_name":       "repo",
		"include_mrs":        true,
		"include_issues":     true,
		"include_code_files": false,
		"batch_size":         10,
		"credentials":        map[string]any{"gitlab_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewGitlabConnector failed: %v", err)
	}
	connector.baseURL = "https://gitlab.com/api/v4"
	connector.doJSON = gitlabFixtureDoJSON(t)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	want := []string{
		"https://gitlab.com/owner/repo/-/merge_requests/7",
		"https://gitlab.com/owner/repo/-/issues/3",
	}
	if len(got) != len(want) {
		t.Fatalf("ids len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestGitlabConnectorOpenSyncResumesAfterCheckpoint verifies retry skips committed GitLab documents.
func TestGitlabConnectorOpenSyncResumesAfterCheckpoint(t *testing.T) {
	connector, err := NewGitlabConnector(map[string]any{
		"project_owner":      "owner",
		"project_name":       "repo",
		"include_mrs":        true,
		"include_issues":     true,
		"include_code_files": false,
		"batch_size":         1,
		"credentials":        map[string]any{"gitlab_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewGitlabConnector failed: %v", err)
	}
	connector.baseURL = "https://gitlab.com/api/v4"
	connector.doJSON = gitlabFixtureDoJSON(t)

	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "https://gitlab.com/owner/repo/-/merge_requests/7" {
		t.Fatalf("first documents = %+v, want MR 7", first.Documents)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "https://gitlab.com/owner/repo/-/merge_requests/7" {
		t.Fatalf("first checkpoint = %+v, want MR 7", first.Checkpoint)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "https://gitlab.com/owner/repo/-/issues/3" {
		t.Fatalf("resume documents = %+v, want issue 3", second.Documents)
	}
	if second.Checkpoint == nil || second.Checkpoint.SourceID != "https://gitlab.com/owner/repo/-/issues/3" {
		t.Fatalf("resume checkpoint = %+v, want issue 3", second.Checkpoint)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resume EOF = %v", err)
	}
}

// TestGitlabConnectorOpenSyncResumeRejectsMissingSourceAnchor verifies a checkpoint without a source anchor is invalid.
func TestGitlabConnectorOpenSyncResumeRejectsMissingSourceAnchor(t *testing.T) {
	connector, err := NewGitlabConnector(map[string]any{
		"project_owner":      "owner",
		"project_name":       "repo",
		"include_mrs":        true,
		"include_issues":     true,
		"include_code_files": false,
		"batch_size":         1,
		"credentials":        map[string]any{"gitlab_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewGitlabConnector failed: %v", err)
	}
	connector.baseURL = "https://gitlab.com/api/v4"
	connector.doJSON = gitlabFixtureDoJSON(t)

	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "https://gitlab.com/owner/repo/-/merge_requests/7" {
		t.Fatalf("first documents = %+v, want MR 7", first.Documents)
	}
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}

	resumeCheckpoint := cloneGitlabCheckpointWithMissingSourceID(t, first.Checkpoint)
	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end, Resume: resumeCheckpoint})
	if resumed != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", resumed, err)
	}
}

// TestGitlabConnectorOpenSyncCodeFiles verifies BFS traversal of code files with lazy Fetch.
func TestGitlabConnectorOpenSyncCodeFiles(t *testing.T) {
	connector, err := NewGitlabConnector(map[string]any{
		"project_owner":      "owner",
		"project_name":       "repo",
		"include_mrs":        false,
		"include_issues":     false,
		"include_code_files": true,
		"batch_size":         10,
		"credentials":        map[string]any{"gitlab_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewGitlabConnector failed: %v", err)
	}
	connector.baseURL = "https://gitlab.com/api/v4"
	connector.doJSON = gitlabFixtureDoJSON(t)
	connector.doRaw = gitlabFixtureDoRaw(t)

	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2 (main.go + src/helper.go)", len(batch.Documents))
	}
	if batch.Documents[0].SourceID != "https://gitlab.com/owner/repo/-/blob/main/main.go" {
		t.Fatalf("doc[0] source id = %q", batch.Documents[0].SourceID)
	}
	if batch.Documents[1].SourceID != "https://gitlab.com/owner/repo/-/blob/main/src/helper.go" {
		t.Fatalf("doc[1] source id = %q", batch.Documents[1].SourceID)
	}
	if batch.Documents[0].FetchRef == nil {
		t.Fatalf("doc[0] has no FetchRef")
	}
	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(context.Background(), *batch.Documents[0].FetchRef)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "package main\n" {
		t.Fatalf("blob = %q, want %q", string(blob), "package main\n")
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestGitlabFetchFileRawEscapesFilePath verifies raw file paths use path-safe %20/%2F encoding.
func TestGitlabFetchFileRawEscapesFilePath(t *testing.T) {
	connector := &GitlabConnector{baseURL: "https://gitlab.com/api/v4"}
	connector.doRaw = func(ctx context.Context, apiURL string) ([]byte, error) {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse api url: %v", err)
		}
		if got := parsed.EscapedPath(); got != "/api/v4/projects/1/repository/files/src%20dir%2Fhelper%20one.go/raw" {
			t.Fatalf("escaped path = %q", got)
		}
		if got := parsed.Query().Get("ref"); got != "main" {
			t.Fatalf("ref = %q, want %q", got, "main")
		}
		return []byte("ok"), nil
	}
	body, err := connector.fetchFileRaw(context.Background(), 1, "main", "src dir/helper one.go")
	if err != nil {
		t.Fatalf("fetchFileRaw failed: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", body, "ok")
	}
}

// TestGitlabConnectorValidate verifies missing config returns clear errors.
func TestGitlabConnectorValidate(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{"missing owner", map[string]any{
			"project_name": "repo",
			"credentials":  map[string]any{"gitlab_access_token": "token"},
		}, "project_owner"},
		{"missing token", map[string]any{
			"project_owner": "owner",
			"project_name":  "repo",
			"credentials":   map[string]any{},
		}, "gitlab_access_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector, err := NewGitlabConnector(tc.config)
			if err != nil {
				t.Fatalf("NewGitlabConnector failed: %v", err)
			}
			err = connector.Validate(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func cloneGitlabCheckpointWithMissingSourceID(t *testing.T, checkpoint *SyncCheckpoint) *SyncCheckpoint {
	t.Helper()
	var cursor gitlabSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		t.Fatalf("decode checkpoint cursor: %v", err)
	}
	cursor.SourceID = ""
	data, err := json.Marshal(cursor)
	if err != nil {
		t.Fatalf("encode checkpoint cursor: %v", err)
	}
	clone := *checkpoint
	clone.Cursor = string(data)
	clone.SourceID = ""
	return &clone
}

// gitlabFixtureDoJSON returns a fixture GitLab JSON transport.
func gitlabFixtureDoJSON(t *testing.T) func(ctx context.Context, apiURL string, out any) (http.Header, error) {
	t.Helper()
	return func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse api url: %v", err)
		}
		path := parsed.EscapedPath()
		query := parsed.Query()

		var body string
		switch {
		case strings.HasSuffix(path, "/projects/owner%2Frepo"):
			body = `{"id":1,"default_branch":"main","web_url":"https://gitlab.com/owner/repo"}`
		case strings.HasSuffix(path, "/projects/1/merge_requests"):
			body = `[{
				"iid":7,
				"title":"Add syncer",
				"description":"MR body",
				"state":"opened",
				"web_url":"https://gitlab.com/owner/repo/-/merge_requests/7",
				"updated_at":"2026-01-03T00:00:00.000Z",
				"author":{"name":"Alice","username":"alice"}
			}]`
		case strings.HasSuffix(path, "/projects/1/issues"):
			body = `[{
				"iid":3,
				"title":"Prune bug",
				"description":"Issue body",
				"state":"opened",
				"web_url":"https://gitlab.com/owner/repo/-/issues/3",
				"updated_at":"2026-01-02T00:00:00.000Z",
				"author":{"name":"Bob","username":"bob"},
				"type":"ISSUE"
			}]`
		case strings.HasSuffix(path, "/projects/1/repository/tree"):
			treePath := query.Get("path")
			if treePath == "" {
				body = `[
					{"id":"abc","name":"main.go","type":"blob","path":"main.go","mode":"100644"},
					{"id":"def","name":"src","type":"tree","path":"src","mode":"040000"}
				]`
			} else if treePath == "src" {
				body = `[
					{"id":"ghi","name":"helper.go","type":"blob","path":"src/helper.go","mode":"100644"}
				]`
			} else {
				t.Fatalf("unexpected tree path %s", treePath)
			}
		case strings.HasSuffix(path, "/projects/1/repository/commits"):
			commitPath := query.Get("path")
			if commitPath == "main.go" {
				body = `[{"id":"commit1","committed_date":"2026-01-03T00:00:00.000Z"}]`
			} else if commitPath == "src/helper.go" {
				body = `[{"id":"commit2","committed_date":"2026-01-02T00:00:00.000Z"}]`
			} else {
				t.Fatalf("unexpected commit path %s", commitPath)
			}
		default:
			t.Fatalf("unexpected api path %s", path)
		}
		if err = json.Unmarshal([]byte(body), out); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		return http.Header{}, nil
	}
}

// gitlabFixtureDoRaw returns fixture raw file content.
func gitlabFixtureDoRaw(t *testing.T) func(ctx context.Context, apiURL string) ([]byte, error) {
	t.Helper()
	return func(ctx context.Context, apiURL string) ([]byte, error) {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse api url: %v", err)
		}
		path := parsed.EscapedPath()
		if strings.HasSuffix(path, "/files/main.go/raw") {
			return []byte("package main\n"), nil
		}
		if strings.HasSuffix(path, "/files/src%2Fhelper.go/raw") {
			return []byte("package src\n"), nil
		}
		t.Fatalf("unexpected raw path %s", path)
		return nil, nil
	}
}
