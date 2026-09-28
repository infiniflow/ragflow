package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestGoogleDriveConnectorOpenSyncUsesWindowFingerprintAndFetch verifies incremental listing and lazy download.
func TestGoogleDriveConnectorOpenSyncUsesWindowFingerprintAndFetch(t *testing.T) {
	connector, err := NewGoogleDriveConnector(map[string]any{
		"my_drive_emails": "admin@example.com",
		"batch_size":      2,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleDriveConnector failed: %v", err)
	}
	var gotRequest googleDriveListRequest
	connector.listFiles = func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error) {
		gotRequest = request
		return googleDriveFilePage{Files: []googleDriveFile{{
			ID:           "file-1",
			Name:         "Plan.txt",
			MimeType:     "text/plain",
			ModifiedTime: "2026-01-03T00:00:00Z",
			CreatedTime:  "2026-01-01T00:00:00Z",
			WebViewLink:  "https://drive.google.com/file/d/file-1/view?usp=sharing",
			Size:         "9",
			MD5Checksum:  "md5-1",
			Owners: []struct {
				EmailAddress string `json:"emailAddress"`
			}{{EmailAddress: "owner@example.com"}},
		}}}, nil
	}
	connector.downloadFile = func(ctx context.Context, userEmail string, file googleDriveFile) ([]byte, string, error) {
		if userEmail != "admin@example.com" || file.ID != "file-1" {
			t.Fatalf("unexpected fetch user/file: %s %s", userEmail, file.ID)
		}
		return []byte("plan body"), ".txt", nil
	}

	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if gotRequest.WindowStart == nil || !gotRequest.WindowStart.Equal(start) || !gotRequest.WindowEnd.Equal(end) {
		t.Fatalf("window = %v %v", gotRequest.WindowStart, gotRequest.WindowEnd)
	}
	if gotRequest.Scope.userEmail != "admin@example.com" || gotRequest.Scope.corpora != "user" {
		t.Fatalf("scope = %+v", gotRequest.Scope)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "https://drive.google.com/file/d/file-1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if doc.FetchRef == nil {
		t.Fatalf("fetch ref is nil")
	}
	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(t.Context(), *doc.FetchRef)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "plan body" {
		t.Fatalf("blob = %q", string(blob))
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestGoogleDriveConnectorOpenSyncResumesWithinPage verifies Drive checkpoints resume inside a list page.
func TestGoogleDriveConnectorOpenSyncResumesWithinPage(t *testing.T) {
	connector, err := NewGoogleDriveConnector(map[string]any{
		"my_drive_emails": "admin@example.com",
		"batch_size":      2,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleDriveConnector failed: %v", err)
	}
	files := []googleDriveFile{
		googleDriveTestFile("file-1", "One.txt", "2026-01-01T00:00:00Z"),
		googleDriveTestFile("file-2", "Two.txt", "2026-01-02T00:00:00Z"),
		googleDriveTestFile("file-3", "Three.txt", "2026-01-03T00:00:00Z"),
	}
	var pageTokens []string
	connector.listFiles = func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error) {
		pageTokens = append(pageTokens, request.PageToken)
		return googleDriveFilePage{Files: files}, nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(first.Documents) != 2 || first.Documents[0].SourceID != "https://drive.google.com/file/d/file-1" || first.Documents[1].SourceID != "https://drive.google.com/file/d/file-2" {
		t.Fatalf("first documents = %+v", first.Documents)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "https://drive.google.com/file/d/file-2" {
		t.Fatalf("first checkpoint = %+v, want file-2", first.Checkpoint)
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "https://drive.google.com/file/d/file-3" {
		t.Fatalf("resume documents = %+v, want file-3", second.Documents)
	}
	if second.Checkpoint == nil || second.Checkpoint.SourceID != "https://drive.google.com/file/d/file-3" {
		t.Fatalf("resume checkpoint = %+v, want file-3", second.Checkpoint)
	}
	if len(pageTokens) != 2 || pageTokens[1] != "" {
		t.Fatalf("page tokens = %v, want resume from first page", pageTokens)
	}
}

// TestGoogleDriveConnectorResumeRejectsMissingRemoteAnchor verifies a deleted list item invalidates the saved anchor.
func TestGoogleDriveConnectorResumeRejectsMissingRemoteAnchor(t *testing.T) {
	connector, err := NewGoogleDriveConnector(map[string]any{
		"my_drive_emails": "admin@example.com",
		"batch_size":      2,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleDriveConnector failed: %v", err)
	}
	files := []googleDriveFile{
		googleDriveTestFile("file-1", "One.txt", "2026-01-01T00:00:00Z"),
		googleDriveTestFile("file-2", "Two.txt", "2026-01-02T00:00:00Z"),
		googleDriveTestFile("file-3", "Three.txt", "2026-01-03T00:00:00Z"),
	}
	connector.listFiles = func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error) {
		return googleDriveFilePage{Files: files}, nil
	}
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "https://drive.google.com/file/d/file-2" {
		t.Fatalf("first checkpoint = %+v, want file-2", first.Checkpoint)
	}

	files = []googleDriveFile{
		googleDriveTestFile("file-1", "One.txt", "2026-01-01T00:00:00Z"),
		googleDriveTestFile("file-3", "Three.txt", "2026-01-03T00:00:00Z"),
	}
	resumeListCalls := 0
	connector.listFiles = func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error) {
		resumeListCalls++
		if request.PageToken != "" {
			t.Fatalf("resume used unexpected pageToken=%q", request.PageToken)
		}
		return googleDriveFilePage{Files: files}, nil
	}
	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	_, err = resumed.NextBatch(context.Background())
	if err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
	if resumeListCalls == 0 {
		t.Fatalf("resume should have listed the current remote page")
	}
}

func TestGoogleDriveConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector, err := NewGoogleDriveConnector(map[string]any{
		"my_drive_emails": "admin@example.com",
		"batch_size":      2,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleDriveConnector failed: %v", err)
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

// TestGoogleDriveSharedFolderScopesRecurse verifies shared folders walk child folders.
func TestGoogleDriveSharedFolderScopesRecurse(t *testing.T) {
	connector, err := NewGoogleDriveConnector(map[string]any{
		"shared_folder_urls": "https://drive.google.com/drive/folders/root-folder",
		"batch_size":         10,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleDriveConnector failed: %v", err)
	}
	connector.listFiles = func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error) {
		switch request.Scope.folderID {
		case "root-folder":
			return googleDriveFilePage{}, nil
		case "child-folder":
			return googleDriveFilePage{Files: []googleDriveFile{{
				ID:           "child-file",
				Name:         "Child.txt",
				MimeType:     "text/plain",
				ModifiedTime: "2026-01-03T00:00:00Z",
				WebViewLink:  "https://drive.google.com/file/d/child-file/view",
			}}}, nil
		default:
			t.Fatalf("unexpected folder scope %q", request.Scope.folderID)
			return googleDriveFilePage{}, nil
		}
	}
	connector.listFolders = func(ctx context.Context, userEmail, parentID string) ([]string, error) {
		if parentID == "root-folder" {
			return []string{"child-folder"}, nil
		}
		return nil, nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "https://drive.google.com/file/d/child-file" {
		t.Fatalf("unexpected recursive documents: %+v", batch.Documents)
	}
}

// TestGoogleDriveRateLimitRetries verifies rate limits do not truncate a scope.
func TestGoogleDriveRateLimitRetries(t *testing.T) {
	connector, err := NewGoogleDriveConnector(map[string]any{
		"my_drive_emails": "admin@example.com",
		"batch_size":      10,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleDriveConnector failed: %v", err)
	}
	calls := 0
	connector.listFiles = func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error) {
		calls++
		if calls == 1 {
			return googleDriveFilePage{}, googleHTTPError{
				status: http.StatusForbidden,
				body:   `{"error":{"errors":[{"reason":"rateLimitExceeded"}],"status":"RESOURCE_EXHAUSTED"}}`,
			}
		}
		return googleDriveFilePage{Files: []googleDriveFile{{
			ID:           "file-1",
			Name:         "Plan.txt",
			MimeType:     "text/plain",
			ModifiedTime: "2026-01-03T00:00:00Z",
			WebViewLink:  "https://drive.google.com/file/d/file-1/view",
		}}}, nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("list calls = %d, want retry", calls)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
}

// TestGoogleDriveFingerprintStable verifies fingerprints are stable and metadata-sensitive.
func TestGoogleDriveFingerprintStable(t *testing.T) {
	file := googleDriveFile{
		ID:           "file-1",
		Name:         "Plan.txt",
		MimeType:     "text/plain",
		ModifiedTime: "2026-01-03T00:00:00Z",
		CreatedTime:  "2026-01-01T00:00:00Z",
		MD5Checksum:  "md5-1",
		Owners: []struct {
			EmailAddress string `json:"emailAddress"`
		}{{EmailAddress: "zoe@example.com"}, {EmailAddress: "alice@example.com"}},
	}
	fp1 := file.fingerprint()
	fp2 := file.fingerprint()
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprint unstable: %q %q", fp1, fp2)
	}
	reordered := file
	reordered.Owners = []struct {
		EmailAddress string `json:"emailAddress"`
	}{{EmailAddress: "alice@example.com"}, {EmailAddress: "zoe@example.com"}}
	if got := reordered.fingerprint(); got != fp1 {
		t.Fatalf("fingerprint changed after owner order-only change: %q != %q", got, fp1)
	}
	changed := file
	changed.MD5Checksum = "md5-2"
	if got := changed.fingerprint(); got == fp1 {
		t.Fatalf("fingerprint did not change after checksum update")
	}
}

// TestGoogleDriveFileQueryUsesIncrementalWindow verifies Python-compatible Drive time filters.
func TestGoogleDriveFileQueryUsesIncrementalWindow(t *testing.T) {
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	query := googleDriveFileQuery(googleDriveScope{corpora: "user", includeSharedWithMe: false}, &start, end)
	if !strings.Contains(query, "modifiedTime > '2026-01-02T00:00:00Z'") ||
		!strings.Contains(query, "createdTime >= '2026-01-02T00:00:00Z'") ||
		!strings.Contains(query, "modifiedTime <= '2026-01-04T00:00:00Z'") ||
		!strings.Contains(query, "'me' in owners") {
		t.Fatalf("query = %q", query)
	}
}

func googleDriveTestFile(id, name, modifiedTime string) googleDriveFile {
	return googleDriveFile{
		ID:           id,
		Name:         name,
		MimeType:     "text/plain",
		ModifiedTime: modifiedTime,
		WebViewLink:  "https://drive.google.com/file/d/" + id + "/view",
	}
}
