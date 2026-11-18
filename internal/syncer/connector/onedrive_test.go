//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

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

func TestNewOneDriveConnectorParsesConfig(t *testing.T) {
	connector, err := NewOneDriveConnector(map[string]any{
		"credentials": map[string]any{
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
		},
		"folder_path":     " Documents/Reports/ ",
		"batch_size":      "0",
		"sync_batch_size": "3",
		"size_threshold":  "123",
	})
	if err != nil {
		t.Fatalf("NewOneDriveConnector failed: %v", err)
	}
	if connector.tenantID != "tenant" || connector.clientID != "client" || connector.clientSecret != "secret" {
		t.Fatalf("credentials = %q/%q/%q", connector.tenantID, connector.clientID, connector.clientSecret)
	}
	if connector.folderPath != "/Documents/Reports" {
		t.Fatalf("folder path = %q", connector.folderPath)
	}
	if connector.batchSize != 3 {
		t.Fatalf("batch size = %d, want 3", connector.batchSize)
	}
	if connector.sizeThreshold != 123 {
		t.Fatalf("size threshold = %d, want 123", connector.sizeThreshold)
	}
}

func TestNewOneDriveConnectorDefaults(t *testing.T) {
	connector, err := NewOneDriveConnector(nil)
	if err != nil {
		t.Fatalf("NewOneDriveConnector failed: %v", err)
	}
	if connector.batchSize != onedriveDefaultBatchSize {
		t.Fatalf("batch size = %d, want %d", connector.batchSize, onedriveDefaultBatchSize)
	}
	if connector.sizeThreshold != onedriveDefaultSizeThreshold {
		t.Fatalf("size threshold = %d, want %d", connector.sizeThreshold, onedriveDefaultSizeThreshold)
	}
	if connector.folderPath != "" {
		t.Fatalf("folder path = %q, want empty", connector.folderPath)
	}
}

func TestNewOneDriveConnectorRejectsParentFolderPath(t *testing.T) {
	_, err := NewOneDriveConnector(map[string]any{"folder_path": "/Documents/../secret"})
	if err == nil || !strings.Contains(err.Error(), "..") {
		t.Fatalf("invalid folder path error = %v", err)
	}
}

func TestNormalizeOneDriveFolderPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"/", ""},
		{"//", ""},
		{"Documents/Reports", "/Documents/Reports"},
		{"//Documents//Reports", "/Documents/Reports"},
		{"/Documents/Reports/", "/Documents/Reports"},
	}
	for _, test := range tests {
		got, err := normalizeOneDriveFolderPath(test.input)
		if err != nil {
			t.Fatalf("normalize %q: %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("normalize %q = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestOneDriveValidateRejectsMissingCredentials(t *testing.T) {
	connector, _ := NewOneDriveConnector(nil)
	err := connector.Validate(context.Background())
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("Validate error = %v, want ConnectorMissingCredentialError", err)
	}
}

func TestOneDriveValidateRejectsBatchSize(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.batchSize = 0
	err := connector.Validate(context.Background())
	var validation *ConnectorValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "batch_size") {
		t.Fatalf("Validate error = %v, want batch_size validation error", err)
	}
}

func TestOneDriveValidateProbesDrives(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.acquireAccessToken = func(ctx context.Context) (string, error) { return "token", nil }
	connector.getJSON = func(ctx context.Context, apiURL string, out any) error {
		if !strings.Contains(apiURL, "/drives?$top=1") {
			t.Fatalf("validation URL = %q, want /drives?$top=1", apiURL)
		}
		return json.Unmarshal([]byte(`{"value":[]}`), out)
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestOneDriveValidateClassifiesGraphErrors(t *testing.T) {
	tests := []struct {
		status int
		want   any
	}{
		{http.StatusUnauthorized, &ConnectorMissingCredentialError{}},
		{http.StatusForbidden, &ConnectorValidationError{}},
		{http.StatusBadGateway, &ConnectorValidationError{}},
	}
	for _, test := range tests {
		connector := newOneDriveTestConnector()
		connector.acquireAccessToken = func(ctx context.Context) (string, error) { return "token", nil }
		connector.getJSON = func(ctx context.Context, apiURL string, out any) error {
			return &onedriveHTTPError{status: test.status, body: "graph error"}
		}
		err := connector.Validate(context.Background())
		switch test.want.(type) {
		case *ConnectorMissingCredentialError:
			var missing *ConnectorMissingCredentialError
			if !errors.As(err, &missing) {
				t.Fatalf("status %d error = %v, want ConnectorMissingCredentialError", test.status, err)
			}
		case *ConnectorValidationError:
			var validation *ConnectorValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("status %d error = %v, want ConnectorValidationError", test.status, err)
			}
		}
	}
}

func TestOneDriveValidateConnectorSettingUsesCandidateConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Form.Get("client_id") != "request-client" || r.Form.Get("client_secret") != "request-secret" {
				t.Errorf("token credentials = %q/%q", r.Form.Get("client_id"), r.Form.Get("client_secret"))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1.0/drives":
			w.Write([]byte(`{"value":[]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	receiver := &OneDriveConnector{
		httpClient:   server.Client(),
		now:          time.Now,
		graphBaseURL: server.URL + "/v1.0",
		tokenURL:     server.URL + "/token",
	}
	err := receiver.ValidateConnectorSetting(context.Background(), map[string]any{
		"credentials": map[string]any{
			"tenant_id":     "request-tenant",
			"client_id":     "request-client",
			"client_secret": "request-secret",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
}

func TestOneDriveOpenSyncUsesDeltaAndFetch(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.batchSize = 2
	connector.getJSON = oneDriveDeltaStub(`{"value":[{"id":"drive-1"}]}`, `{"value":[
		{"id":"file-1","name":"Report.PDF","file":{},"size":5,"webUrl":"https://example.test/report","lastModifiedDateTime":"2026-01-02T03:04:05Z","eTag":"etag-1","createdBy":{"user":{"displayName":"Alice"}}},
		{"id":"photo","name":"photo.png","file":{},"size":5},
		{"id":"folder","name":"Docs","folder":{}},
		{"id":"deleted","name":"gone.pdf","file":{},"deleted":{"state":"deleted"}},
		{"id":"file-2","name":"Notes.txt","file":{},"size":4,"lastModifiedDateTime":"2026-01-03T03:04:05Z","eTag":"etag-2"}
	],"@odata.deltaLink":"delta-1"}`)
	var fetchedURL string
	connector.getBytes = func(ctx context.Context, apiURL string) ([]byte, error) {
		fetchedURL = apiURL
		if !strings.Contains(apiURL, "/items/file-1/content") {
			t.Fatalf("fetch URL = %q, want file-1 content", apiURL)
		}
		return []byte("pdf-body"), nil
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
	doc := batch.Documents[0]
	if doc.SourceID != "file-1" || doc.SemanticIdentifier != "Report.PDF" || doc.Extension != ".pdf" {
		t.Fatalf("document shape = %+v", doc)
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-02T03:04:05Z")) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Fingerprint != "etag-1" {
		t.Fatalf("fingerprint = %q, want etag-1", doc.Fingerprint)
	}
	if doc.Metadata["drive_id"] != "drive-1" || doc.Metadata["web_url"] != "https://example.test/report" || doc.Metadata["created_by"] != "Alice" {
		t.Fatalf("metadata = %+v", doc.Metadata)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "file-2" {
		t.Fatalf("checkpoint = %+v, want file-2", batch.Checkpoint)
	}

	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(context.Background(), *batch.Documents[0].FetchRef)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "pdf-body" || fetchedURL == "" {
		t.Fatalf("fetch blob = %q, url = %q", blob, fetchedURL)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestOneDriveOpenSyncWindowFilter(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.batchSize = 10
	connector.getJSON = oneDriveDeltaStub(`{"value":[{"id":"drive-1"}]}`, `{"value":[
		{"id":"before","name":"before.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-01T00:00:00Z","eTag":"e1"},
		{"id":"inside","name":"inside.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-03T00:00:00Z","eTag":"e2"},
		{"id":"after","name":"after.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-05T00:00:00Z","eTag":"e3"}
	]}`)
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "inside" {
		t.Fatalf("documents = %+v, want inside", batch.Documents)
	}
}

func TestOneDriveOpenSyncFingerprintFilter(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.batchSize = 10
	connector.getJSON = oneDriveDeltaStub(`{"value":[{"id":"drive-1"}]}`, `{"value":[
		{"id":"changed","name":"changed.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-03T00:00:00Z","eTag":"new"},
		{"id":"same","name":"same.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-03T00:00:00Z","eTag":"same"},
		{"id":"missing","name":"missing.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-03T00:00:00Z","eTag":"missing"}
	]}`)
	request := SyncRequest{
		WindowStart: onedriveMustTimePointer(t, "2026-01-02T00:00:00Z"),
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			"changed": "old",
			"same":    "same",
		},
	}
	session, err := connector.OpenSync(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	if len(got) != 2 || got[0] != "changed" || got[1] != "missing" {
		t.Fatalf("documents = %v, want changed and missing", got)
	}
}

func TestOneDriveOpenSyncResumeWithinPage(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.batchSize = 2
	connector.getJSON = oneDriveDeltaStub(`{"value":[{"id":"drive-1"}]}`, `{"value":[
		{"id":"file-1","name":"a.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-01T00:00:00Z","eTag":"e1"},
		{"id":"file-2","name":"b.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-02T00:00:00Z","eTag":"e2"},
		{"id":"file-3","name":"c.pdf","file":{},"size":1,"lastModifiedDateTime":"2026-01-03T00:00:00Z","eTag":"e3"}
	],"@odata.deltaLink":"delta-1"}`)

	first, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("first OpenSync failed: %v", err)
	}
	batch, err := first.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 || batch.Checkpoint == nil || batch.Checkpoint.SourceID != "file-2" {
		t.Fatalf("first batch = %+v, want two docs ending at file-2", batch)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: batch.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "file-3" {
		t.Fatalf("resume documents = %+v, want file-3", second.Documents)
	}
}

func TestOneDriveOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.getJSON = oneDriveDeltaStub(`{"value":[{"id":"drive-1"}]}`, `{"value":[]}`)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestOneDriveOpenSyncResumeRejectsMissingAnchor(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.getJSON = oneDriveDeltaStub(`{"value":[{"id":"drive-1"}]}`, `{"value":[
		{"id":"file-1","name":"a.pdf","file":{},"size":1,"eTag":"e1"},
		{"id":"file-3","name":"c.pdf","file":{},"size":1,"eTag":"e3"}
	]}`)
	checkpoint := &SyncCheckpoint{
		Cursor: `{"drive_id":"drive-1","page_url":"` + connector.deltaURL("drive-1") + `","source_id":"file-2"}`,
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestOneDriveOpenPrune(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.batchSize = 1
	firstURL := connector.deltaURL("drive-1")
	secondURL := "https://graph.microsoft.com/v1.0/next"
	connector.getJSON = func(ctx context.Context, apiURL string, out any) error {
		switch apiURL {
		case connector.graphBaseURL + "/drives":
			return json.Unmarshal([]byte(`{"value":[{"id":"drive-1"}]}`), out)
		case firstURL:
			return json.Unmarshal([]byte(`{"value":[
				{"id":"file-1","name":"a.pdf","file":{},"size":1},
				{"id":"folder","name":"Docs","folder":{}},
				{"id":"deleted","name":"gone.pdf","file":{},"deleted":{"state":"deleted"}},
				{"id":"photo","name":"photo.png","file":{},"size":1}
			],"@odata.nextLink":"`+secondURL+`"}`), out)
		case secondURL:
			return json.Unmarshal([]byte(`{"value":[{"id":"file-2","name":"b.pdf","file":{},"size":1}]}`), out)
		default:
			t.Fatalf("unexpected prune URL %s", apiURL)
			return nil
		}
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	var got []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
		for _, doc := range batch.Documents {
			got = append(got, doc.SourceID)
		}
	}
	if len(got) != 2 || got[0] != "file-1" || got[1] != "file-2" {
		t.Fatalf("prune documents = %v, want file-1 and file-2", got)
	}
}

func TestOneDrivePrunePaginationStall(t *testing.T) {
	connector := newOneDriveTestConnector()
	pageURL := connector.deltaURL("drive-1")
	connector.getJSON = func(ctx context.Context, apiURL string, out any) error {
		if apiURL == connector.graphBaseURL+"/drives" {
			return json.Unmarshal([]byte(`{"value":[{"id":"drive-1"}]}`), out)
		}
		return json.Unmarshal([]byte(`{"value":[{"id":"file-1","name":"a.pdf","file":{},"size":1}],"@odata.nextLink":"`+pageURL+`"}`), out)
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("prune NextBatch err = %v, want stalled pagination error", err)
	}
}

func TestOneDriveFetch(t *testing.T) {
	connector := newOneDriveTestConnector()
	connector.sizeThreshold = 5
	ref := FetchReference{Key: `{"drive_id":"drive-1","item_id":"item-1","name":"big.pdf","size":10}`}
	if _, err := connector.Fetch(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "exceeds size threshold") {
		t.Fatalf("oversize Fetch err = %v, want size threshold error", err)
	}

	var fetchedURL string
	connector.getBytes = func(ctx context.Context, apiURL string) ([]byte, error) {
		fetchedURL = apiURL
		if !strings.Contains(apiURL, "/items/item-1/content") {
			t.Fatalf("fetch URL = %q, want item-1 content", apiURL)
		}
		return []byte("hello"), nil
	}
	ref.Key = `{"drive_id":"drive-1","item_id":"item-1","name":"ok.pdf","size":5}`
	blob, err := connector.Fetch(context.Background(), ref)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "hello" || fetchedURL == "" {
		t.Fatalf("fetch blob = %q, url = %q", blob, fetchedURL)
	}
}

func TestOneDriveDeltaURL(t *testing.T) {
	connector := newOneDriveTestConnector()
	if got := connector.deltaURL("drive 1"); got != "https://graph.microsoft.com/v1.0/drives/drive%201/root/delta" {
		t.Fatalf("root delta URL = %q", got)
	}
	connector.folderPath = "/Documents/A B"
	if got := connector.deltaURL("drive-1"); got != "https://graph.microsoft.com/v1.0/drives/drive-1/root:/Documents/A%20B:/delta" {
		t.Fatalf("scoped delta URL = %q", got)
	}
}

func newOneDriveTestConnector() *OneDriveConnector {
	return &OneDriveConnector{
		tenantID:      "tenant",
		clientID:      "client",
		clientSecret:  "secret",
		batchSize:     onedriveDefaultBatchSize,
		sizeThreshold: onedriveDefaultSizeThreshold,
		graphBaseURL:  onedriveGraphBase,
		httpClient:    http.DefaultClient,
		now:           time.Now,
	}
}

func oneDriveDeltaStub(drivesJSON, deltaJSON string) func(ctx context.Context, apiURL string, out any) error {
	return func(ctx context.Context, apiURL string, out any) error {
		if strings.HasSuffix(apiURL, "/drives") {
			return json.Unmarshal([]byte(drivesJSON), out)
		}
		return json.Unmarshal([]byte(deltaJSON), out)
	}
}

func onedriveMustTimePointer(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed := mustTime(t, value)
	return &parsed
}
