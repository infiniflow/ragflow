package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func seafileTestConnector(t *testing.T, config map[string]any) *SeaFileConnector {
	t.Helper()
	connector, err := NewSeaFileConnector(config)
	if err != nil {
		t.Fatalf("NewSeaFileConnector failed: %v", err)
	}
	return connector
}

func seafileMustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed.UTC()
}

func seafileTestDirent(entryType, name, id string, size int64, mtime any) seafileDirent {
	return seafileDirent{Type: entryType, Name: name, ID: id, Size: size, MTime: mtime}
}

func TestSeaFileConfigParsing(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url":    "https://seafile.example.com/",
		"batch_size":     7,
		"size_threshold": 12345,
		"credentials": map[string]any{
			"seafile_token": "token",
		},
	})
	if connector.seafileURL != "https://seafile.example.com" {
		t.Fatalf("seafile URL = %q", connector.seafileURL)
	}
	if connector.syncScope != seafileScopeAccount {
		t.Fatalf("sync scope = %q", connector.syncScope)
	}
	if connector.batchSize != 7 {
		t.Fatalf("batch size = %d", connector.batchSize)
	}
	if connector.sizeThreshold != 12345 {
		t.Fatalf("size threshold = %d", connector.sizeThreshold)
	}
	if connector.accountToken != "token" || connector.repoToken != "" {
		t.Fatalf("tokens = account %q repo %q", connector.accountToken, connector.repoToken)
	}
}

func TestSeaFileConfigDefaultsAndSyncBatchSize(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url":     "https://seafile.example.com",
		"sync_batch_size": 11,
		"include_shared":  false,
		"credentials": map[string]any{
			"seafile_token": "token",
		},
	})
	if connector.batchSize != 11 {
		t.Fatalf("batch size = %d, want 11", connector.batchSize)
	}
	if connector.sizeThreshold != seafileDefaultSizeThreshold {
		t.Fatalf("size threshold = %d, want default", connector.sizeThreshold)
	}
	if connector.includeShared {
		t.Fatalf("include_shared should be false")
	}

	library := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"sync_scope":  seafileScopeLibrary,
		"repo_id":     "repo1",
		"credentials": map[string]any{
			"seafile_token": "account-token",
			"repo_token":    "repo-token",
		},
	})
	if library.repoToken != "repo-token" {
		t.Fatalf("library repo token = %q", library.repoToken)
	}

	account := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"credentials": map[string]any{
			"seafile_token": "account-token",
			"repo_token":    "ignored",
		},
	})
	if account.repoToken != "" {
		t.Fatalf("account scope repo token = %q, want ignored", account.repoToken)
	}
}

func TestSeaFileScopeAndCredentialValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		config  map[string]any
		wantErr any
	}{
		{
			name: "library missing repo id",
			config: map[string]any{
				"seafile_url": "https://seafile.example.com",
				"sync_scope":  seafileScopeLibrary,
				"credentials": map[string]any{"seafile_token": "token"},
			},
			wantErr: &ConnectorValidationError{},
		},
		{
			name: "directory root path",
			config: map[string]any{
				"seafile_url": "https://seafile.example.com",
				"sync_scope":  seafileScopeDirectory,
				"repo_id":     "repo1",
				"sync_path":   "/",
				"credentials": map[string]any{"seafile_token": "token"},
			},
			wantErr: &ConnectorValidationError{},
		},
		{
			name: "unsupported scope",
			config: map[string]any{
				"seafile_url": "https://seafile.example.com",
				"sync_scope":  "everything",
				"credentials": map[string]any{"seafile_token": "token"},
			},
			wantErr: &ConnectorValidationError{},
		},
		{
			name: "missing credentials",
			config: map[string]any{
				"seafile_url": "https://seafile.example.com",
				"credentials": map[string]any{},
			},
			wantErr: &ConnectorMissingCredentialError{},
		},
		{
			name: "account scope cannot use only repo token",
			config: map[string]any{
				"seafile_url": "https://seafile.example.com",
				"credentials": map[string]any{"repo_token": "repo-token"},
			},
			wantErr: &ConnectorMissingCredentialError{},
		},
		{
			name: "missing url",
			config: map[string]any{
				"credentials": map[string]any{"seafile_token": "token"},
			},
			wantErr: &ConnectorValidationError{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSeaFileConnector(tc.config)
			if err == nil {
				t.Fatalf("NewSeaFileConnector unexpectedly succeeded")
			}
			switch tc.wantErr.(type) {
			case *ConnectorMissingCredentialError:
				var credentialErr *ConnectorMissingCredentialError
				if !errors.As(err, &credentialErr) {
					t.Fatalf("error = %T %v, want %T", err, err, tc.wantErr)
				}
			default:
				var validationErr *ConnectorValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("error = %T %v, want %T", err, err, tc.wantErr)
				}
			}
		})
	}
}

func seafileValidationServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api2/auth-token/":
			_ = r.ParseForm()
			if r.FormValue("username") != "alice" || r.FormValue("password") != "secret" {
				http.Error(w, `{"error":"bad credentials"}`, http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"token":"auth-token"}`)
		case "/api2/account/info/":
			if r.Header.Get("Authorization") != "Token auth-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"email":"alice@example.com"}`)
		case "/api2/repos/":
			if r.Header.Get("Authorization") != "Token auth-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `[{"id":"repo1","name":"Docs","owner":"alice@example.com","owner_email":"alice@example.com"}]`)
		case "/api/v2.1/via-repo-token/repo-info/":
			if r.Header.Get("Authorization") != "Bearer repo-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"repo_id":"repo1","repo_name":"Docs"}`)
		case "/api/v2.1/via-repo-token/dir/":
			if r.Header.Get("Authorization") != "Bearer repo-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"dirent_list":[{"type":"file","name":"a.pdf","id":"file-a","size":10,"mtime":1700000000}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestSeaFileValidateAccountToken(t *testing.T) {
	server := seafileValidationServer(t)
	defer server.Close()
	orig := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	defer func() { restAPISSRFAllowLoopback = orig }()

	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": server.URL,
		"credentials": map[string]any{"seafile_token": "auth-token"},
	})
	connector.httpClient = server.Client()
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if connector.currentUserEmail != "alice@example.com" {
		t.Fatalf("current user email = %q", connector.currentUserEmail)
	}
}

func TestSeaFileValidateConnectorSettingUsesRequestUsernamePassword(t *testing.T) {
	server := seafileValidationServer(t)
	defer server.Close()
	orig := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	defer func() { restAPISSRFAllowLoopback = orig }()

	receiver := seafileTestConnector(t, map[string]any{
		"seafile_url": server.URL,
		"credentials": map[string]any{"seafile_token": "ignored"},
	})
	receiver.httpClient = server.Client()
	request := map[string]any{
		"seafile_url": server.URL,
		"credentials": map[string]any{
			"username": "alice",
			"password": "secret",
		},
	}
	if err := receiver.ValidateConnectorSetting(context.Background(), request); err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
}

func TestSeaFileValidateRepoToken(t *testing.T) {
	server := seafileValidationServer(t)
	defer server.Close()
	orig := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	defer func() { restAPISSRFAllowLoopback = orig }()

	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": server.URL,
		"sync_scope":  seafileScopeLibrary,
		"repo_id":     "repo1",
		"credentials": map[string]any{"repo_token": "repo-token"},
	})
	connector.httpClient = server.Client()
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestSeaFileDefaultListLibrariesFiltersShared(t *testing.T) {
	orig := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	defer func() { restAPISSRFAllowLoopback = orig }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/repos/" || r.Header.Get("Authorization") != "Token token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{"id":"own","name":"Own","owner":"alice@example.com","owner_email":"alice@example.com"},
			{"id":"shared","name":"Shared","owner":"bob@example.com","owner_email":"bob@example.com"}
		]`)
	}))
	defer server.Close()

	connector := seafileTestConnector(t, map[string]any{
		"seafile_url":    server.URL,
		"include_shared": false,
		"credentials":    map[string]any{"seafile_token": "token"},
	})
	connector.httpClient = server.Client()
	connector.currentUserEmail = "alice@example.com"
	libraries, err := connector.defaultListLibraries(context.Background())
	if err != nil {
		t.Fatalf("defaultListLibraries failed: %v", err)
	}
	if len(libraries) != 1 || libraries[0].ID != "own" {
		t.Fatalf("libraries = %#v, want own only", libraries)
	}
}

func TestSeaFileOpenSyncAccountRecursiveListing(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url":    "https://seafile.example.com",
		"include_shared": true,
		"credentials":    map[string]any{"seafile_token": "token"},
	})
	connector.listLibraries = func(ctx context.Context) ([]seafileLibrary, error) {
		return []seafileLibrary{
			{ID: "repo1", Name: "Docs"},
			{ID: "repo2", Name: "Archive"},
		}, nil
	}
	connector.listDirectory = func(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error) {
		if useRepoToken {
			return nil, fmt.Errorf("account scope should not use repo token")
		}
		switch repoID + ":" + path {
		case "repo1:/":
			return []seafileDirent{
				seafileTestDirent("dir", "sub", "", 0, nil),
				seafileTestDirent("file", "a.txt", "file-a", 10, 1700000000),
			}, nil
		case "repo1:/sub":
			return []seafileDirent{
				seafileTestDirent("file", "b.pdf", "file-b", 20, "2023-11-14T22:13:20+01:00"),
			}, nil
		case "repo2:/":
			return []seafileDirent{
				seafileTestDirent("file", "c.md", "file-c", 30, "1700000000"),
			}, nil
		default:
			return nil, fmt.Errorf("unexpected directory %q", path)
		}
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
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
	first := batch.Documents[0]
	if first.SourceID != "seafile:repo1:/a.txt" {
		t.Fatalf("first source id = %q", first.SourceID)
	}
	if first.SemanticIdentifier != "Docs/a.txt" || first.Extension != ".txt" {
		t.Fatalf("first identity = %q ext %q", first.SemanticIdentifier, first.Extension)
	}
	if !first.UpdatedAt.Equal(time.Unix(1700000000, 0).UTC()) {
		t.Fatalf("first updated at = %s", first.UpdatedAt)
	}
	if first.FetchRef == nil || first.FetchRef.SizeHint != 10 {
		t.Fatalf("first fetch ref = %#v", first.FetchRef)
	}
	if got := first.Fingerprint; got != seafileFingerprint("repo1", "/a.txt", "file-a", 10, first.UpdatedAt) {
		t.Fatalf("fingerprint = %q", got)
	}
	if first.Metadata["repo_name"] != "Docs" || first.Metadata["path"] != "/a.txt" {
		t.Fatalf("metadata = %#v", first.Metadata)
	}
	second := batch.Documents[1]
	if second.SourceID != "seafile:repo1:/sub/b.pdf" || second.SemanticIdentifier != "Docs/sub/b.pdf" || second.Extension != ".pdf" {
		t.Fatalf("second document = %#v", second)
	}
	third, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("third NextBatch failed: %v", err)
	}
	if len(third.Documents) != 1 || third.Documents[0].SourceID != "seafile:repo2:/c.md" {
		t.Fatalf("third batch = %#v", third.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final NextBatch err = %v, want io.EOF", err)
	}
}

func TestSeaFileOpenSyncLibraryAndDirectoryScopes(t *testing.T) {
	library := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"sync_scope":  seafileScopeLibrary,
		"repo_id":     "repo1",
		"credentials": map[string]any{"seafile_token": "token"},
	})
	library.getRepoInfo = func(ctx context.Context) (seafileRepoInfo, error) {
		return seafileRepoInfo{ID: "repo1", Name: "Library"}, nil
	}
	library.listDirectory = func(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error) {
		if repoID != "repo1" || path != "/" || useRepoToken {
			return nil, fmt.Errorf("unexpected library list %q %q %v", repoID, path, useRepoToken)
		}
		return []seafileDirent{seafileTestDirent("file", "lib.txt", "file-lib", 10, 1700000000)}, nil
	}
	librarySession, err := library.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("library OpenSync failed: %v", err)
	}
	libraryBatch, err := librarySession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("library NextBatch failed: %v", err)
	}
	if len(libraryBatch.Documents) != 1 || libraryBatch.Documents[0].SemanticIdentifier != "Library/lib.txt" {
		t.Fatalf("library docs = %#v", libraryBatch.Documents)
	}

	directory := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"sync_scope":  seafileScopeDirectory,
		"repo_id":     "repo1",
		"sync_path":   "/Docs",
		"credentials": map[string]any{"seafile_token": "token"},
	})
	directory.getRepoInfo = func(ctx context.Context) (seafileRepoInfo, error) {
		return seafileRepoInfo{ID: "repo1", Name: "Library"}, nil
	}
	directory.listDirectory = func(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error) {
		if repoID != "repo1" || path != "/Docs" {
			return nil, fmt.Errorf("unexpected directory list %q %q", repoID, path)
		}
		return []seafileDirent{seafileTestDirent("file", "guide.pdf", "file-guide", 10, 1700000000)}, nil
	}
	directorySession, err := directory.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("directory OpenSync failed: %v", err)
	}
	directoryBatch, err := directorySession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("directory NextBatch failed: %v", err)
	}
	if len(directoryBatch.Documents) != 1 || directoryBatch.Documents[0].SemanticIdentifier != "Library/Docs/guide.pdf" {
		t.Fatalf("directory docs = %#v", directoryBatch.Documents)
	}
}

func TestSeaFileListingFailurePropagates(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"credentials": map[string]any{"seafile_token": "token"},
	})
	connector.listLibraries = func(ctx context.Context) ([]seafileLibrary, error) {
		return []seafileLibrary{{ID: "repo1", Name: "Docs"}}, nil
	}
	connector.listDirectory = func(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error) {
		return nil, fmt.Errorf("directory listing failed")
	}
	if _, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true}); err == nil || !strings.Contains(err.Error(), "directory listing failed") {
		t.Fatalf("OpenSync listing err = %v", err)
	}
	if _, err := connector.OpenPrune(context.Background(), PruneRequest{}); err == nil || !strings.Contains(err.Error(), "directory listing failed") {
		t.Fatalf("OpenPrune listing err = %v", err)
	}
}

func TestSeaFileParseMtime(t *testing.T) {
	cases := []struct {
		raw  any
		want time.Time
	}{
		{raw: 1575514722, want: time.Unix(1575514722, 0).UTC()},
		{raw: float64(1575514722), want: time.Unix(1575514722, 0).UTC()},
		{raw: "1575514722", want: time.Unix(1575514722, 0).UTC()},
		{raw: "2026-02-15T17:26:53+01:00", want: seafileMustTime(t, "2026-02-15T16:26:53Z")},
	}
	for _, tc := range cases {
		if got := seafileParseMtime(tc.raw); !got.Equal(tc.want) {
			t.Fatalf("mtime %v = %s, want %s", tc.raw, got, tc.want)
		}
	}
}

func TestSeaFileIncludeDocumentWindowAndFingerprint(t *testing.T) {
	start := seafileMustTime(t, "2026-01-02T00:00:00Z")
	end := seafileMustTime(t, "2026-01-04T00:00:00Z")
	document := SourceDocument{
		SourceID:    "seafile:repo1:/a.txt",
		UpdatedAt:   seafileMustTime(t, "2026-01-03T00:00:00Z"),
		Fingerprint: "fingerprint",
	}
	cases := []struct {
		name    string
		request SyncRequest
		want    bool
	}{
		{name: "beginning", request: SyncRequest{FromBeginning: true}, want: true},
		{name: "same fingerprint skipped", request: SyncRequest{Fingerprints: map[string]string{document.SourceID: "fingerprint"}}, want: false},
		{name: "changed fingerprint included", request: SyncRequest{Fingerprints: map[string]string{document.SourceID: "old"}}, want: true},
		{name: "missing fingerprint included", request: SyncRequest{Fingerprints: map[string]string{"other": "fingerprint"}}, want: true},
		{name: "exclusive start", request: SyncRequest{WindowStart: &document.UpdatedAt, WindowEnd: end}, want: false},
		{name: "window included", request: SyncRequest{WindowStart: &start, WindowEnd: end}, want: true},
		{name: "inclusive end", request: SyncRequest{WindowStart: &start, WindowEnd: document.UpdatedAt}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := includeSeaFileDocument(tc.request, document); got != tc.want {
				t.Fatalf("include = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSeaFileFetch(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"sync_scope":  seafileScopeLibrary,
		"repo_id":     "repo1",
		"credentials": map[string]any{"repo_token": "repo-token"},
	})
	connector.getDownloadLink = func(ctx context.Context, repoID, path string, useRepoToken bool) (string, error) {
		if repoID != "repo1" || path != "/a.txt" || !useRepoToken {
			return "", fmt.Errorf("unexpected download request %q %q %v", repoID, path, useRepoToken)
		}
		return "https://download.example/a.txt", nil
	}
	connector.download = func(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
		if rawURL != "https://download.example/a.txt" {
			return nil, fmt.Errorf("unexpected download URL %q", rawURL)
		}
		return []byte("file body"), nil
	}
	ref := FetchReference{Key: `{"repo_id":"repo1","path":"/a.txt","use_repo_token":true,"size":9}`}
	blob, err := connector.Fetch(context.Background(), ref)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "file body" {
		t.Fatalf("blob = %q", blob)
	}
}

func TestSeaFileFetchMissingLinkAndOversize(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"credentials": map[string]any{"seafile_token": "token"},
	})
	connector.sizeThreshold = 10
	connector.getDownloadLink = func(ctx context.Context, repoID, path string, useRepoToken bool) (string, error) {
		return "", nil
	}
	_, err := connector.Fetch(context.Background(), FetchReference{Key: `{"repo_id":"repo1","path":"/a.txt","use_repo_token":false,"size":1}`})
	if err == nil || !strings.Contains(err.Error(), "no download link") {
		t.Fatalf("missing link err = %v", err)
	}
	connector.getDownloadLink = func(ctx context.Context, repoID, path string, useRepoToken bool) (string, error) {
		return "https://download.example/a.txt", nil
	}
	_, err = connector.Fetch(context.Background(), FetchReference{Key: `{"repo_id":"repo1","path":"/a.txt","use_repo_token":false,"size":11}`})
	if err == nil || !strings.Contains(err.Error(), "size threshold") {
		t.Fatalf("oversize err = %v", err)
	}
}

func TestSeaFileSyncResume(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"credentials": map[string]any{"seafile_token": "token"},
	})
	session := &seafileSyncSession{
		connector: connector,
		documents: []SourceDocument{
			{SourceID: "seafile:repo1:/a.txt"},
			{SourceID: "seafile:repo1:/sub/b.pdf"},
			{SourceID: "seafile:repo1:/c.txt"},
		},
		batchSize: 2,
	}
	if err := session.applyResume(&SyncCheckpoint{SourceID: "seafile:repo1:/a.txt"}); err != nil {
		t.Fatalf("applyResume failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 || batch.Documents[0].SourceID != "seafile:repo1:/sub/b.pdf" {
		t.Fatalf("resume batch = %#v", batch.Documents)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "seafile:repo1:/c.txt" {
		t.Fatalf("resume checkpoint = %#v", batch.Checkpoint)
	}

	missing := &seafileSyncSession{documents: session.documents}
	err = missing.applyResume(&SyncCheckpoint{SourceID: "seafile:repo1:missing"})
	if !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("missing anchor err = %v, want ErrSyncResumeInvalid", err)
	}
	err = missing.applyResume(&SyncCheckpoint{})
	if !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("empty checkpoint err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestSeaFileOpenPruneRecursiveFilterAndBatching(t *testing.T) {
	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": "https://seafile.example.com",
		"sync_scope":  seafileScopeLibrary,
		"repo_id":     "repo1",
		"batch_size":  2,
		"credentials": map[string]any{"seafile_token": "token"},
	})
	connector.sizeThreshold = 100
	connector.getRepoInfo = func(ctx context.Context) (seafileRepoInfo, error) {
		return seafileRepoInfo{ID: "repo1", Name: "Docs"}, nil
	}
	connector.listDirectory = func(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error) {
		switch path {
		case "/":
			return []seafileDirent{
				seafileTestDirent("dir", "sub", "", 0, nil),
				seafileTestDirent("file", "a.txt", "file-a", 10, 1700000000),
				seafileTestDirent("file", "large.bin", "file-large", 1000, 1700000000),
			}, nil
		case "/sub":
			return []seafileDirent{
				seafileTestDirent("file", "b.md", "file-b", 20, 1700000000),
				seafileTestDirent("file", "c.pdf", "file-c", 30, 1700000000),
			}, nil
		default:
			return nil, fmt.Errorf("unexpected prune path %q", path)
		}
	}

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first prune batch failed: %v", err)
	}
	if len(first.Documents) != 2 || first.Documents[0].SourceID != "seafile:repo1:/a.txt" || first.Documents[1].SourceID != "seafile:repo1:/sub/b.md" {
		t.Fatalf("first prune batch = %#v", first.Documents)
	}
	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second prune batch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "seafile:repo1:/sub/c.pdf" {
		t.Fatalf("second prune batch = %#v", second.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final prune batch err = %v, want io.EOF", err)
	}
}

func TestSeaFileRegistryOpen(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("seafile", map[string]any{
		"seafile_url": "https://seafile.example.com",
		"credentials": map[string]any{"seafile_token": "token"},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*SeaFileConnector); !ok {
		t.Fatalf("connector type = %T, want *SeaFileConnector", connector)
	}
}

func TestSeaFileDirentWrapperAndDownloadLinkDecode(t *testing.T) {
	orig := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	defer func() { restAPISSRFAllowLoopback = orig }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api2/repos/repo1/dir/":
			if r.Header.Get("Authorization") != "Token token" || r.URL.Query().Get("p") != "/Docs" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"dirent_list":[{"type":"file","name":"a.txt","id":"file-a","size":10,"mtime":1700000000}]}`)
		case "/api2/repos/repo1/file/":
			if r.Header.Get("Authorization") != "Token token" || r.URL.Query().Get("p") != "/Docs/a.txt" || r.URL.Query().Get("reuse") != "1" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `"https://download.example/a.txt"`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	connector := seafileTestConnector(t, map[string]any{
		"seafile_url": server.URL,
		"credentials": map[string]any{"seafile_token": "token"},
	})
	connector.httpClient = server.Client()
	entries, err := connector.defaultListDirectory(context.Background(), "repo1", "/Docs", false)
	if err != nil {
		t.Fatalf("defaultListDirectory failed: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "file-a" {
		t.Fatalf("entries = %#v", entries)
	}
	link, err := connector.defaultGetDownloadLink(context.Background(), "repo1", "/Docs/a.txt", false)
	if err != nil {
		t.Fatalf("defaultGetDownloadLink failed: %v", err)
	}
	if link != "https://download.example/a.txt" {
		t.Fatalf("link = %q", link)
	}

	var fetch seafileFetchReference
	if err := json.Unmarshal([]byte(`{"repo_id":"repo1","path":"/Docs/a.txt","use_repo_token":false,"size":10}`), &fetch); err != nil {
		t.Fatalf("fetch reference decode failed: %v", err)
	}
	if fetch.RepoID != "repo1" || fetch.Path != "/Docs/a.txt" || fetch.Size != 10 {
		t.Fatalf("fetch reference = %#v", fetch)
	}
}

func TestSeaFileAssertURLSafe(t *testing.T) {
	orig := restAPISSRFAllowLoopback
	defer func() { restAPISSRFAllowLoopback = orig }()

	restAPISSRFAllowLoopback = false
	if _, _, err := seafileAssertURLSafe(context.Background(), "ftp://example.com"); err == nil {
		t.Fatalf("expected scheme rejection")
	}
	if _, _, err := seafileAssertURLSafe(context.Background(), "localhost"); err == nil {
		t.Fatalf("expected missing host rejection")
	}
	if _, _, err := seafileAssertURLSafe(context.Background(), "https://localhost/seafile"); err == nil {
		t.Fatalf("expected localhost rejection")
	}
	if _, _, err := seafileAssertURLSafe(context.Background(), "https://127.0.0.1/seafile"); err == nil {
		t.Fatalf("expected loopback rejection")
	}

	restAPISSRFAllowLoopback = true
	hostname, pinIP, err := seafileAssertURLSafe(context.Background(), "http://127.0.0.1:8080/seafile")
	if err != nil {
		t.Fatalf("loopback should be allowed: %v", err)
	}
	if hostname != "127.0.0.1" || !pinIP.IsLoopback() {
		t.Fatalf("hostname/pinIP = %q/%v", hostname, pinIP)
	}
}
