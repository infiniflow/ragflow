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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const boxTestTokens = `{"client_id":"client","client_secret":"secret","access_token":"token","refresh_token":"refresh"}`

func boxTestConnector(t *testing.T, config map[string]any) *BoxConnector {
	t.Helper()
	connector, err := NewBoxConnector(config)
	if err != nil {
		t.Fatalf("NewBoxConnector failed: %v", err)
	}
	return connector
}

func boxTestFile(id, name string, size int64, modified string) boxFile {
	return boxFile{
		ID:         id,
		Name:       name,
		Size:       &size,
		ModifiedAt: modified,
	}
}

func TestNewBoxConnectorConfigDefaults(t *testing.T) {
	connector := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	if connector.folderID != "0" {
		t.Fatalf("folder_id = %q, want 0", connector.folderID)
	}
	if connector.batchSize != defaultBoxBatchSize {
		t.Fatalf("batch_size = %d, want %d", connector.batchSize, defaultBoxBatchSize)
	}
	if connector.sizeThreshold != defaultBoxSizeThreshold {
		t.Fatalf("size_threshold = %d, want %d", connector.sizeThreshold, defaultBoxSizeThreshold)
	}
	if connector.allowImages {
		t.Fatalf("allow_images = true, want false")
	}
	if connector.clientID != "client" || connector.clientSecret != "secret" || connector.accessToken != "token" || connector.refreshToken != "refresh" {
		t.Fatalf("credentials = %q/%q/%q/%q", connector.clientID, connector.clientSecret, connector.accessToken, connector.refreshToken)
	}
}

func TestNewBoxConnectorConfigOverrides(t *testing.T) {
	connector := boxTestConnector(t, map[string]any{
		"folder_id":       "123",
		"sync_batch_size": 7,
		"batch_size":      3,
		"size_threshold":  "10",
		"allow_images":    true,
		"credentials": map[string]any{
			"box_tokens": map[string]any{
				"client_id":     "client",
				"client_secret": "secret",
				"access_token":  "token",
				"refresh_token": "refresh",
			},
		},
	})
	if connector.folderID != "123" {
		t.Fatalf("folder_id = %q, want 123", connector.folderID)
	}
	if connector.batchSize != 7 {
		t.Fatalf("batch_size = %d, want 7", connector.batchSize)
	}
	if connector.sizeThreshold != 10 {
		t.Fatalf("size_threshold = %d, want 10", connector.sizeThreshold)
	}
	if !connector.allowImages {
		t.Fatalf("allow_images = false, want true")
	}
}

func TestNewBoxConnectorRejectsInvalidBoxTokens(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "non string object", value: 123, want: "JSON string or object"},
		{name: "invalid json", value: "{", want: "parse box_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBoxConnector(map[string]any{
				"credentials": map[string]any{"box_tokens": tt.value},
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewBoxConnector err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBoxConnectorValidateCallsCurrentUserOnly(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/users/me" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"1"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	connector := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.apiBaseURL = server.URL
	connector.httpClient = server.Client()

	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/users/me" {
		t.Fatalf("validation paths = %#v, want only /users/me", paths)
	}
}

func TestRegisterBuiltInsOpensBox(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("box", map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*BoxConnector); !ok {
		t.Fatalf("connector type = %T, want *BoxConnector", connector)
	}
}

func TestBoxConnectorValidateMissingCredentials(t *testing.T) {
	connector := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{},
	})
	err := connector.Validate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "credentials must include") {
		t.Fatalf("Validate err = %v, want missing credential error", err)
	}
}

func TestBoxConnectorValidateConnectorSettingUsesRequest(t *testing.T) {
	var calls int
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/users/me" || authHeader != "Bearer request-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"1"}`)
	}))
	defer server.Close()

	receiver := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	receiver.apiBaseURL = server.URL
	receiver.httpClient = server.Client()

	err := receiver.ValidateConnectorSetting(context.Background(), map[string]any{
		"folder_id": "0",
		"credentials": map[string]any{
			"box_tokens": `{"client_id":"client","client_secret":"secret","access_token":"request-token","refresh_token":"refresh"}`,
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
	if calls != 1 || authHeader != "Bearer request-token" {
		t.Fatalf("calls = %d, auth = %q; want one request with request token", calls, authHeader)
	}
}

func TestBoxConnectorValidateConnectorSettingRejectsMissingRequestCredentials(t *testing.T) {
	receiver := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	err := receiver.ValidateConnectorSetting(context.Background(), map[string]any{
		"credentials": map[string]any{},
	})
	if err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("ValidateConnectorSetting err = %v, want missing request token error", err)
	}
}

func TestBoxConnectorOpenSyncListsLazilyAndFetches(t *testing.T) {
	var listCalls int
	var downloadIDs []string
	connector := boxTestConnector(t, map[string]any{
		"batch_size":  2,
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.listFolderItems = func(ctx context.Context, folderID, marker string, limit int) (boxItemsPage, error) {
		listCalls++
		if folderID == "0" {
			return boxItemsPage{Entries: []boxItemEntry{{Type: "folder", ID: "folder", Name: "Folder"}}}, nil
		}
		return boxItemsPage{Entries: []boxItemEntry{
			{Type: "file", ID: "alpha", Name: "alpha.txt"},
			{Type: "file", ID: "beta", Name: "beta.txt"},
		}}, nil
	}
	connector.getFile = func(ctx context.Context, fileID string) (boxFile, error) {
		size := int64(5)
		switch fileID {
		case "alpha":
			return boxFile{ID: "alpha", Name: "alpha.txt", Size: &size, ModifiedAt: "2026-01-02T00:00:00Z", WebLink: "https://app.box.com/file/alpha"}, nil
		case "beta":
			return boxFile{ID: "beta", Name: "beta.txt", Size: &size, ModifiedAt: "2026-01-03T00:00:00Z"}, nil
		}
		return boxFile{}, nil
	}
	connector.downloadFile = func(ctx context.Context, fileID string, sizeThreshold int64) ([]byte, error) {
		downloadIDs = append(downloadIDs, fileID)
		return []byte("body:" + fileID), nil
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if listCalls != 0 {
		t.Fatalf("OpenSync listed eagerly: %d calls", listCalls)
	}

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("batch documents = %d, want 2", len(batch.Documents))
	}
	if batch.Documents[0].SourceID != "box:alpha" || batch.Documents[0].SemanticIdentifier != "Folder / alpha.txt" {
		t.Fatalf("first document = %#v", batch.Documents[0])
	}
	if batch.Documents[1].SourceID != "box:beta" || batch.Documents[1].SemanticIdentifier != "Folder / beta.txt" {
		t.Fatalf("second document = %#v", batch.Documents[1])
	}
	if batch.Documents[0].Metadata["url"] != "https://app.box.com/file/alpha" {
		t.Fatalf("metadata url = %#v", batch.Documents[0].Metadata["url"])
	}
	if listCalls != 2 {
		t.Fatalf("list calls = %d, want 2", listCalls)
	}

	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("sync session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(context.Background(), *batch.Documents[0].FetchRef)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "body:alpha" || strings.Join(downloadIDs, ",") != "alpha" {
		t.Fatalf("fetched blob = %q, download ids = %#v", blob, downloadIDs)
	}
}

func TestBoxConnectorOpenSyncFiltersAndWindow(t *testing.T) {
	connector := boxTestConnector(t, map[string]any{
		"batch_size":  10,
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.sizeThreshold = 5
	files := []boxFile{
		boxTestFile("old", "old.txt", 5, "2026-01-01T00:00:00Z"),
		boxTestFile("alpha", "alpha.txt", 5, "2026-01-02T12:00:00Z"),
		boxTestFile("future", "future.txt", 5, "2026-01-04T00:00:00Z"),
		boxTestFile("image", "image.png", 5, "2026-01-02T12:00:00Z"),
		boxTestFile("big", "big.txt", 6, "2026-01-02T12:00:00Z"),
		boxTestFile("unknown", "unknown.xyz", 5, "2026-01-02T12:00:00Z"),
	}
	connector.listFolderItems = boxListFromFiles(files)
	connector.getFile = boxGetFromFiles(files)

	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-03T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "box:alpha" {
		t.Fatalf("filtered documents = %#v", batch.Documents)
	}
}

func TestBoxConnectorOpenSyncAllowImages(t *testing.T) {
	files := []boxFile{
		boxTestFile("doc", "doc.txt", 5, "2026-01-02T00:00:00Z"),
		boxTestFile("image", "image.png", 5, "2026-01-02T00:00:00Z"),
	}
	for _, allow := range []bool{false, true} {
		name := "allow_images=false"
		if allow {
			name = "allow_images=true"
		}
		t.Run(name, func(t *testing.T) {
			connector := boxTestConnector(t, map[string]any{
				"batch_size":   10,
				"allow_images": allow,
				"credentials":  map[string]any{"box_tokens": boxTestTokens},
			})
			connector.listFolderItems = boxListFromFiles(files)
			connector.getFile = boxGetFromFiles(files)
			session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
			if err != nil {
				t.Fatalf("OpenSync failed: %v", err)
			}
			defer session.Close()
			batch, err := session.NextBatch(context.Background())
			if err != nil {
				t.Fatalf("NextBatch failed: %v", err)
			}
			want := 1
			if allow {
				want = 2
			}
			if len(batch.Documents) != want {
				t.Fatalf("documents = %d, want %d", len(batch.Documents), want)
			}
		})
	}
}

func TestBoxConnectorOpenSyncFingerprintFilter(t *testing.T) {
	alpha := boxTestFile("alpha", "alpha.txt", 5, "2026-01-02T00:00:00Z")
	beta := boxTestFile("beta", "beta.txt", 5, "2026-01-03T00:00:00Z")
	gamma := boxTestFile("gamma", "gamma.txt", 5, "2026-01-04T00:00:00Z")
	files := []boxFile{alpha, beta, gamma}
	connector := boxTestConnector(t, map[string]any{
		"batch_size":  10,
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.listFolderItems = boxListFromFiles(files)
	connector.getFile = boxGetFromFiles(files)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		Fingerprints: map[string]string{
			"box:alpha": alpha.fingerprint(),
			"box:gamma": "old-fingerprint",
		},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	var ids []string
	for _, document := range batch.Documents {
		ids = append(ids, document.SourceID)
	}
	if strings.Join(ids, ",") != "box:beta,box:gamma" {
		t.Fatalf("fingerprint filtered ids = %#v", ids)
	}
}

func TestBoxFileUpdatedAtPrecedence(t *testing.T) {
	modified := boxFile{
		ModifiedAt:        "2026-01-03T00:00:00Z",
		ContentModifiedAt: "2026-01-02T00:00:00Z",
		CreatedAt:         "2026-01-01T00:00:00Z",
	}
	if !modified.updatedAt().Equal(mustTime(t, "2026-01-03T00:00:00Z")) {
		t.Fatalf("updatedAt = %v, want modified_at", modified.updatedAt())
	}
	content := boxFile{ContentModifiedAt: "2026-01-02T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z"}
	if !content.updatedAt().Equal(mustTime(t, "2026-01-02T00:00:00Z")) {
		t.Fatalf("updatedAt = %v, want content_modified_at", content.updatedAt())
	}
	created := boxFile{CreatedAt: "2026-01-01T00:00:00Z"}
	if !created.updatedAt().Equal(mustTime(t, "2026-01-01T00:00:00Z")) {
		t.Fatalf("updatedAt = %v, want created_at", created.updatedAt())
	}
}

func TestBoxConnectorSkipsOversizeBeforeDownload(t *testing.T) {
	var downloadCalls int
	files := []boxFile{boxTestFile("big", "big.txt", 10, "2026-01-02T00:00:00Z")}
	connector := boxTestConnector(t, map[string]any{
		"batch_size":  10,
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.sizeThreshold = 5
	connector.listFolderItems = boxListFromFiles(files)
	connector.getFile = boxGetFromFiles(files)
	connector.downloadFile = func(ctx context.Context, fileID string, sizeThreshold int64) ([]byte, error) {
		downloadCalls++
		return nil, nil
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if _, err := session.NextBatch(context.Background()); err != io.EOF {
		t.Fatalf("NextBatch err = %v, want io.EOF", err)
	}
	if downloadCalls != 0 {
		t.Fatalf("download calls = %d, want 0", downloadCalls)
	}
}

func TestBoxConnectorPrune(t *testing.T) {
	var listCalls int
	files := []boxFile{
		boxTestFile("a", "a.txt", 5, "2026-01-02T00:00:00Z"),
		boxTestFile("b", "b.txt", 5, "2026-01-02T00:00:00Z"),
		boxTestFile("image", "image.png", 5, "2026-01-02T00:00:00Z"),
		boxTestFile("big", "big.txt", 10, "2026-01-02T00:00:00Z"),
	}
	connector := boxTestConnector(t, map[string]any{
		"batch_size":  1,
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.sizeThreshold = 5
	connector.listFolderItems = func(ctx context.Context, folderID, marker string, limit int) (boxItemsPage, error) {
		listCalls++
		return boxListFromFiles(files)(ctx, folderID, marker, limit)
	}
	connector.getFile = boxGetFromFiles(files)

	prune, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	defer prune.Close()
	if listCalls != 0 {
		t.Fatalf("OpenPrune listed eagerly: %d calls", listCalls)
	}

	first, err := prune.NextBatch(context.Background())
	if err != nil || len(first.Documents) != 1 || first.Documents[0].SourceID != "box:a" {
		t.Fatalf("first prune batch = %#v, err = %v", first, err)
	}
	second, err := prune.NextBatch(context.Background())
	if err != nil || len(second.Documents) != 1 || second.Documents[0].SourceID != "box:b" {
		t.Fatalf("second prune batch = %#v, err = %v", second, err)
	}
	if _, err := prune.NextBatch(context.Background()); err != io.EOF {
		t.Fatalf("final prune NextBatch err = %v, want io.EOF", err)
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", listCalls)
	}
}

func TestBoxConnectorResume(t *testing.T) {
	files := []boxFile{
		boxTestFile("a", "a.txt", 5, "2026-01-02T00:00:00Z"),
		boxTestFile("b", "b.txt", 5, "2026-01-03T00:00:00Z"),
		boxTestFile("c", "c.txt", 5, "2026-01-04T00:00:00Z"),
	}
	connector := boxTestConnector(t, map[string]any{
		"batch_size":  10,
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.listFolderItems = boxListFromFiles(files)
	connector.getFile = boxGetFromFiles(files)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{SourceID: "box:b"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "box:c" {
		t.Fatalf("resumed documents = %#v", batch.Documents)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "box:c" {
		t.Fatalf("resumed checkpoint = %#v", batch.Checkpoint)
	}
}

func TestBoxConnectorResumeRejectsInvalidCheckpoint(t *testing.T) {
	files := []boxFile{
		boxTestFile("a", "a.txt", 5, "2026-01-02T00:00:00Z"),
		boxTestFile("b", "b.txt", 5, "2026-01-03T00:00:00Z"),
	}
	tests := []*SyncCheckpoint{
		{},
		{SourceID: "dropbox:a"},
		{SourceID: "box:"},
		{SourceID: "box:missing"},
	}
	for _, checkpoint := range tests {
		t.Run(checkpoint.SourceID, func(t *testing.T) {
			connector := boxTestConnector(t, map[string]any{
				"batch_size":  10,
				"credentials": map[string]any{"box_tokens": boxTestTokens},
			})
			connector.listFolderItems = boxListFromFiles(files)
			connector.getFile = boxGetFromFiles(files)
			session, err := connector.OpenSync(context.Background(), SyncRequest{Resume: checkpoint})
			if err != nil {
				t.Fatalf("OpenSync err = %v, want session to defer resume validation", err)
			}
			defer session.Close()
			if _, err := session.NextBatch(context.Background()); !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("NextBatch err = %v, want ErrSyncResumeInvalid", err)
			}
		})
	}
}

func TestBoxConnectorOAuthRefreshAndRetry(t *testing.T) {
	var refreshRequests int
	var downloadRequests int
	var downloadAuth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			refreshRequests++
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh"}`)
		case "/files/file/content":
			downloadRequests++
			downloadAuth = append(downloadAuth, r.Header.Get("Authorization"))
			if downloadRequests == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "hello")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	connector := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.apiBaseURL = server.URL
	connector.tokenURL = server.URL + "/oauth2/token"
	connector.httpClient = server.Client()

	blob, err := connector.defaultDownloadFile(context.Background(), "file", 10)
	if err != nil {
		t.Fatalf("defaultDownloadFile failed: %v", err)
	}
	if string(blob) != "hello" || refreshRequests != 1 || downloadRequests != 2 {
		t.Fatalf("blob = %q, refresh = %d, downloads = %d", blob, refreshRequests, downloadRequests)
	}
	if connector.accessToken != "new-access" || connector.refreshToken != "new-refresh" {
		t.Fatalf("tokens = %q/%q, want new-access/new-refresh", connector.accessToken, connector.refreshToken)
	}
	if len(downloadAuth) != 2 || downloadAuth[0] != "Bearer token" || downloadAuth[1] != "Bearer new-access" {
		t.Fatalf("download auth headers = %#v", downloadAuth)
	}
}

func TestBoxConnectorDownloadEnforcesSizeThreshold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "123456")
	}))
	defer server.Close()

	connector := boxTestConnector(t, map[string]any{
		"credentials": map[string]any{"box_tokens": boxTestTokens},
	})
	connector.apiBaseURL = server.URL
	connector.httpClient = server.Client()

	_, err := connector.defaultDownloadFile(context.Background(), "file", 5)
	if err == nil || !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("defaultDownloadFile err = %v, want size threshold error", err)
	}
}

func boxListFromFiles(files []boxFile) func(ctx context.Context, folderID, marker string, limit int) (boxItemsPage, error) {
	return func(ctx context.Context, folderID, marker string, limit int) (boxItemsPage, error) {
		entries := make([]boxItemEntry, 0, len(files))
		for _, file := range files {
			entries = append(entries, boxItemEntry{Type: "file", ID: file.ID, Name: file.Name})
		}
		return boxItemsPage{Entries: entries}, nil
	}
}

func boxGetFromFiles(files []boxFile) func(ctx context.Context, fileID string) (boxFile, error) {
	byID := make(map[string]boxFile, len(files))
	for _, file := range files {
		byID[file.ID] = file
	}
	return func(ctx context.Context, fileID string) (boxFile, error) {
		file, ok := byID[fileID]
		if !ok {
			return boxFile{}, nil
		}
		return file, nil
	}
}
