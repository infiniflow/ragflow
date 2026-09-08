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

type dropboxTestServer struct {
	server        *httptest.Server
	listRequests  []dropboxListFolderRequest
	continueCalls int
	downloadPaths []string
	downloadArgs  []string
}

func newDropboxTestServer(t *testing.T) *dropboxTestServer {
	t.Helper()
	fixture := &dropboxTestServer{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error_summary":"invalid_access_token"}`)
			return
		}
		switch r.URL.Path {
		case "/2/files/list_folder":
			var request dropboxListFolderRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode list_folder request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fixture.listRequests = append(fixture.listRequests, request)
			if request.Limit == 1 {
				_ = json.NewEncoder(w).Encode(dropboxListFolderResponse{Cursor: "validate"})
				return
			}
			writeDropboxTestListResponse(w, []map[string]any{
				{".tag": "folder", "id": "id:folder", "name": "Folder", "path_display": "/Folder"},
				{".tag": "file", "id": "id:old", "name": "old.txt", "path_display": "/old.txt", "client_modified": "2026-01-01T00:00:00Z", "size": 3},
				{".tag": "file", "id": "id:alpha", "name": "alpha.txt", "path_display": "/alpha.txt", "client_modified": "2026-01-02T00:00:00Z", "size": 5},
				{".tag": "file", "id": "id:broken", "name": "broken.txt", "path_display": "/broken.txt", "client_modified": "2026-01-02T00:00:00Z", "size": 5},
				{".tag": "file", "id": "id:dup-1", "name": "dup.md", "path_display": "/a/dup.md", "client_modified": "2026-01-03T00:00:00Z", "size": 5},
				{".tag": "file", "id": "id:image", "name": "image.png", "path_display": "/image.png", "client_modified": "2026-01-03T00:00:00Z", "size": 5},
			}, "cursor-1", true)
		case "/2/files/list_folder/continue":
			fixture.continueCalls++
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode continue request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request["cursor"] != "cursor-1" {
				t.Errorf("cursor = %q", request["cursor"])
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeDropboxTestListResponse(w, []map[string]any{
				{".tag": "file", "id": "id:dup-2", "name": "dup.md", "path_display": "/b/dup.md", "client_modified": "2026-01-04T00:00:00Z", "size": 5},
				{".tag": "file", "id": "id:late", "name": "late.txt", "path_display": "/late.txt", "client_modified": "2026-01-06T00:00:00Z", "size": 4},
				{".tag": "file", "id": "id:bin", "name": "skip.bin", "path_display": "/skip.bin", "client_modified": "2026-01-03T00:00:00Z", "size": 5},
			}, "", false)
		case "/2/files/download":
			fixture.downloadArgs = append(fixture.downloadArgs, r.Header.Get("Dropbox-API-Arg"))
			var arg map[string]string
			if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg); err != nil {
				t.Errorf("decode download arg: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fixture.downloadPaths = append(fixture.downloadPaths, arg["path"])
			if arg["path"] == "/broken.txt" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error_summary":"broken"}`)
				return
			}
			_, _ = io.WriteString(w, "body:"+arg["path"])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	fixture.server = httptest.NewServer(handler)
	t.Cleanup(fixture.server.Close)
	return fixture
}

func writeDropboxTestListResponse(w http.ResponseWriter, entries []map[string]any, cursor string, hasMore bool) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"entries":  entries,
		"cursor":   cursor,
		"has_more": hasMore,
	})
}

func newDropboxTestConnector(t *testing.T, fixture *dropboxTestServer, allowImages bool) *DropboxConnector {
	t.Helper()
	connector, err := NewDropboxConnector(map[string]any{
		"batch_size":   2,
		"allow_images": allowImages,
		"credentials": map[string]any{
			"dropbox_access_token": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewDropboxConnector failed: %v", err)
	}
	connector.apiBaseURL = fixture.server.URL + "/2"
	connector.contentBaseURL = fixture.server.URL + "/2"
	return connector
}

func TestDropboxConnectorValidate(t *testing.T) {
	fixture := newDropboxTestServer(t)
	connector := newDropboxTestConnector(t, fixture, false)

	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if len(fixture.listRequests) != 1 || fixture.listRequests[0].Limit != 1 {
		t.Fatalf("validation requests = %#v", fixture.listRequests)
	}

	missingToken, err := NewDropboxConnector(map[string]any{"credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewDropboxConnector failed: %v", err)
	}
	if err := missingToken.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("missing token error = %v", err)
	}
}

func TestDropboxConnectorOpenSync(t *testing.T) {
	fixture := newDropboxTestServer(t)
	connector := newDropboxTestConnector(t, fixture, false)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if len(fixture.downloadPaths) != 0 {
		t.Fatalf("OpenSync downloaded files eagerly: %#v", fixture.downloadPaths)
	}

	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 {
		t.Fatalf("first batch size = %d", len(first.Documents))
	}
	if first.Documents[0].SourceID != "dropbox:id:alpha" || string(first.Documents[0].Blob) != "body:/alpha.txt" {
		t.Fatalf("first document = %#v", first.Documents[0])
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "dropbox:id:broken" {
		t.Fatalf("checkpoint = %#v", first.Checkpoint)
	}
	if strings.Join(fixture.downloadPaths, ",") != "/alpha.txt,/broken.txt" {
		t.Fatalf("first batch download paths = %#v", fixture.downloadPaths)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch second failed: %v", err)
	}
	if len(second.Documents) != 2 || second.Documents[0].SourceID != "dropbox:id:dup-1" || second.Documents[1].SourceID != "dropbox:id:dup-2" {
		t.Fatalf("second batch = %#v", second.Documents)
	}
	if second.Documents[0].SemanticIdentifier != "a / dup.md" {
		t.Fatalf("duplicate semantic identifier = %q", second.Documents[0].SemanticIdentifier)
	}
	if _, err := session.NextBatch(context.Background()); err != io.EOF {
		t.Fatalf("final NextBatch err = %v", err)
	}
	if fixture.continueCalls != 1 {
		t.Fatalf("continue calls = %d", fixture.continueCalls)
	}
	if strings.Join(fixture.downloadPaths, ",") != "/alpha.txt,/broken.txt,/a/dup.md,/b/dup.md" {
		t.Fatalf("download paths = %#v", fixture.downloadPaths)
	}
}

func TestDropboxConnectorOpenPruneAndResume(t *testing.T) {
	fixture := newDropboxTestServer(t)
	connector := newDropboxTestConnector(t, fixture, true)

	prune, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	defer prune.Close()

	batch, err := prune.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("prune NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("first prune batch size = %d", len(batch.Documents))
	}

	syncSession, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Resume:        &SyncCheckpoint{SourceID: "dropbox:id:dup-1"},
	})
	if err != nil {
		t.Fatalf("OpenSync with resume failed: %v", err)
	}
	defer syncSession.Close()

	resumed, err := syncSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch failed: %v", err)
	}
	if len(resumed.Documents) != 2 || resumed.Documents[0].SourceID != "dropbox:id:dup-2" || resumed.Documents[1].SourceID != "dropbox:id:image" {
		t.Fatalf("resumed docs = %#v", resumed.Documents)
	}
}

func TestDropboxConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	fixture := newDropboxTestServer(t)
	connector := newDropboxTestConnector(t, fixture, false)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Resume:        &SyncCheckpoint{},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestDropboxConnectorDownloadFileEscapesAPIArgHeader(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Dropbox-API-Arg")
		_, _ = io.WriteString(w, "body")
	}))
	t.Cleanup(server.Close)

	connector, err := NewDropboxConnector(map[string]any{
		"credentials": map[string]any{"dropbox_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewDropboxConnector failed: %v", err)
	}
	connector.contentBaseURL = server.URL + "/2"

	path := "/Résumé/数据/emoji-😀/del-\x7f.txt"
	if _, err := connector.downloadFile(context.Background(), path); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}
	want := `{"path":"/R\u00e9sum\u00e9/\u6570\u636e/emoji-\ud83d\ude00/del-\u007f.txt"}`
	if gotHeader != want {
		t.Fatalf("Dropbox-API-Arg = %q, want %q", gotHeader, want)
	}
}

func TestDropboxConnectorSessionsNormalizeNonPositiveBatchSize(t *testing.T) {
	fixture := newDropboxTestServer(t)
	connector := newDropboxTestConnector(t, fixture, true)
	connector.batchSize = 0

	syncSession, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	syncBatch, err := syncSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("sync NextBatch failed: %v", err)
	}
	if len(syncBatch.Documents) == 0 {
		t.Fatalf("sync batch should advance with a positive effective size")
	}

	pruneSession, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	pruneBatch, err := pruneSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("prune NextBatch failed: %v", err)
	}
	if len(pruneBatch.Documents) == 0 {
		t.Fatalf("prune batch should advance with a positive effective size")
	}
}
