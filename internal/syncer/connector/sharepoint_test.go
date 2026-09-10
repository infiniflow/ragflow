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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustSharePointConnector(t *testing.T, serverURL string, extra map[string]any) *SharePointConnector {
	t.Helper()
	config := map[string]any{
		"batch_size": 2,
		"credentials": map[string]any{
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
			"site_url":      "https://contoso.sharepoint.com/sites/MySite",
		},
	}
	for key, value := range extra {
		config[key] = value
	}
	connector, err := NewSharePointConnector(config)
	if err != nil {
		t.Fatalf("NewSharePointConnector: %v", err)
	}
	if serverURL != "" {
		connector.tokenURL = serverURL + "/oauth2/v2.0/token"
		connector.graphBaseURL = serverURL + "/v1.0"
	}
	return connector
}

type sharePointTestFixtures struct {
	tokenStatus int
	tokenBody   string

	siteStatus int
	siteBody   string
	siteCalls  *atomic.Int64

	drivesBody  string
	drivesBody2 string
	drivesCalls *atomic.Int64

	childrenByKey map[string]string
	childrenPage2 map[string]string
	childrenCalls *atomic.Int64

	contentByKey map[string]string
	contentCalls *atomic.Int64

	tokenCalls *atomic.Int64
}

const sharePointDefaultSiteBody = `{"id":"site-1","name":"MySite","webUrl":"https://contoso.sharepoint.com/sites/MySite"}`

func newTestSharePointServer(t *testing.T, fixtures *sharePointTestFixtures) *httptest.Server {
	t.Helper()
	if fixtures.tokenStatus == 0 {
		fixtures.tokenStatus = http.StatusOK
	}
	if fixtures.tokenBody == "" {
		fixtures.tokenBody = `{"access_token":"tok-1","expires_in":3600}`
	}
	if fixtures.siteStatus == 0 {
		fixtures.siteStatus = http.StatusOK
	}
	if fixtures.siteBody == "" {
		fixtures.siteBody = sharePointDefaultSiteBody
	}
	if fixtures.drivesBody == "" {
		fixtures.drivesBody = `{"value":[]}`
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		if fixtures.tokenCalls != nil {
			fixtures.tokenCalls.Add(1)
		}
		w.WriteHeader(fixtures.tokenStatus)
		w.Write([]byte(fixtures.tokenBody))
	})

	mux.HandleFunc("/v1.0/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v1.0/")
		switch {
		case strings.HasPrefix(rest, "sites/"):
			if strings.HasSuffix(rest, "/drives") {
				if fixtures.drivesCalls != nil {
					fixtures.drivesCalls.Add(1)
				}
				if r.URL.Query().Get("page") == "2" && fixtures.drivesBody2 != "" {
					w.Write([]byte(fixtures.drivesBody2))
					return
				}
				w.Write([]byte(fixtures.drivesBody))
				return
			}
			if fixtures.siteCalls != nil {
				fixtures.siteCalls.Add(1)
			}
			w.WriteHeader(fixtures.siteStatus)
			w.Write([]byte(fixtures.siteBody))
		case strings.HasPrefix(rest, "drives/"):
			parts := strings.Split(rest, "/")
			if len(parts) < 4 {
				http.NotFound(w, r)
				return
			}
			driveID := parts[1]
			switch parts[2] {
			case "root":
				if len(parts) != 4 || parts[3] != "children" {
					http.NotFound(w, r)
					return
				}
				if fixtures.childrenCalls != nil {
					fixtures.childrenCalls.Add(1)
				}
				key := driveID + ":/"
				if r.URL.Query().Get("page") == "2" && fixtures.childrenPage2[key] != "" {
					w.Write([]byte(fixtures.childrenPage2[key]))
					return
				}
				w.Write([]byte(fixtures.childrenByKey[key]))
			case "items":
				if len(parts) != 5 {
					http.NotFound(w, r)
					return
				}
				itemID := parts[3]
				switch parts[4] {
				case "children":
					if fixtures.childrenCalls != nil {
						fixtures.childrenCalls.Add(1)
					}
					key := driveID + ":" + itemID
					if r.URL.Query().Get("page") == "2" && fixtures.childrenPage2[key] != "" {
						w.Write([]byte(fixtures.childrenPage2[key]))
						return
					}
					w.Write([]byte(fixtures.childrenByKey[key]))
				case "content":
					if fixtures.contentCalls != nil {
						fixtures.contentCalls.Add(1)
					}
					w.Write([]byte(fixtures.contentByKey[driveID+":"+itemID]))
				default:
					http.NotFound(w, r)
				}
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})
	return server
}

func drainSharePointSync(t *testing.T, session SyncSession) []SourceDocument {
	t.Helper()
	var documents []SourceDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			return documents
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		documents = append(documents, batch.Documents...)
	}
}

func sharePointFileJSON(itemID, name, webURL, modified string, size int64) string {
	return `{"id":"` + itemID + `","name":"` + name + `","webUrl":"` + webURL +
		`","lastModifiedDateTime":"` + modified + `","size":` + strconv.FormatInt(size, 10) +
		`,"file":{"mimeType":"text/plain"}}`
}

func sharePointFolderJSON(itemID, name string) string {
	return `{"id":"` + itemID + `","name":"` + name + `","folder":{"childCount":1}}`
}

func TestSharePointValidateMissingCredentials(t *testing.T) {
	connector := mustSharePointConnector(t, "", map[string]any{
		"credentials": map[string]any{"tenant_id": "t", "client_id": "c"},
	})
	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatal("Validate succeeded, want error")
	}
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("error type = %T, want *ConnectorMissingCredentialError", err)
	}
}

func TestSharePointValidateSuccess(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		tokenCalls: &atomic.Int64{},
		siteCalls:  &atomic.Int64{},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if fixtures.tokenCalls.Load() != 1 {
		t.Fatalf("token calls = %d, want 1", fixtures.tokenCalls.Load())
	}
	if fixtures.siteCalls.Load() != 1 {
		t.Fatalf("site calls = %d, want 1", fixtures.siteCalls.Load())
	}
}

func TestSharePointValidateTokenFailure(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		tokenStatus: http.StatusUnauthorized,
		tokenBody:   `{"error":"invalid_client","error_description":"bad secret"}`,
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatal("Validate succeeded, want error")
	}
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("error type = %T, want *ConnectorMissingCredentialError", err)
	}
	if !strings.Contains(err.Error(), "Failed to acquire SharePoint access token") {
		t.Fatalf("error = %q, want token acquisition failure", err)
	}
}

func TestSharePointValidateSiteForbidden(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		siteStatus: http.StatusForbidden,
		siteBody:   `{"error":{"code":"accessDenied","message":"Access denied"}}`,
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatal("Validate succeeded, want error")
	}
	var validation *ConnectorValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error type = %T, want *ConnectorValidationError", err)
	}
	if !strings.Contains(err.Error(), "Invalid credentials or insufficient permissions") {
		t.Fatalf("error = %q, want permission message", err)
	}
}

func TestSharePointValidateSiteNotFound(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		siteStatus: http.StatusNotFound,
		siteBody:   `{"error":{"code":"itemNotFound","message":"Site not found"}}`,
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatal("Validate succeeded, want error")
	}
	var validation *ConnectorValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error type = %T, want *ConnectorValidationError", err)
	}
	if !strings.Contains(err.Error(), "SharePoint validation error") {
		t.Fatalf("error = %q, want validation error message", err)
	}
}

func TestSharePointSiteAPIPath(t *testing.T) {
	connector := mustSharePointConnector(t, "", map[string]any{
		"credentials": map[string]any{
			"tenant_id": "t", "client_id": "c", "client_secret": "s",
			"site_url": "https://contoso.sharepoint.com/sites/MySite",
		},
	})
	if got := connector.siteAPIPath(); got != "/sites/contoso.sharepoint.com:/sites/MySite" {
		t.Fatalf("siteAPIPath() = %q", got)
	}

	connector.siteURL = "https://contoso.sharepoint.com/sites/MySite/"
	if got := connector.siteAPIPath(); got != "/sites/contoso.sharepoint.com:/sites/MySite" {
		t.Fatalf("siteAPIPath(trailing slash) = %q", got)
	}

	connector.siteURL = "https://contoso.sharepoint.com/"
	if got := connector.siteAPIPath(); got != "/sites/contoso.sharepoint.com" {
		t.Fatalf("siteAPIPath(root) = %q", got)
	}

	connector.siteURL = "not a url"
	if got := connector.siteAPIPath(); got != "" {
		t.Fatalf("siteAPIPath(invalid) = %q, want empty", got)
	}
}

func TestSharePointOpenSyncWalksLibrariesAndDownloads(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[
			{"id":"drv-A","name":"Documents","driveType":"documentLibrary"},
			{"id":"drv-B","name":"SiteAssets","driveType":"documentLibrary"}
		]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[
				` + sharePointFileJSON("f1", "readme.txt", "https://contoso.sharepoint.com/f1", "2026-01-01T12:00:00Z", 16) + `,
				` + sharePointFolderJSON("d2", "sub") + `
			]}`,
			"drv-A:d2": `{"value":[` + sharePointFileJSON("f2", "report.md", "https://contoso.sharepoint.com/f2", "2026-02-01T12:00:00Z", 8) + `]}`,
			"drv-B:/":  `{"value":[` + sharePointFileJSON("f3", "logo.png", "https://contoso.sharepoint.com/f3", "2026-03-01T12:00:00Z", 5) + `]}`,
		},
		contentByKey: map[string]string{
			"drv-A:f1": "hello sharepoint",
			"drv-A:f2": "# Report",
			"drv-B:f3": "PNGDATA",
		},
		contentCalls: &atomic.Int64{},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := drainSharePointSync(t, session)

	if len(documents) != 3 {
		t.Fatalf("documents = %d, want 3", len(documents))
	}
	byID := map[string]SourceDocument{}
	for _, document := range documents {
		byID[document.SourceID] = document
	}
	if len(byID) != 3 {
		t.Fatalf("duplicate SourceIDs: %v", documents)
	}
	for _, wantID := range []string{"drv-A:f1", "drv-A:f2", "drv-B:f3"} {
		if _, ok := byID[wantID]; !ok {
			t.Fatalf("missing document %q in %v", wantID, byID)
		}
	}

	first := byID["drv-A:f1"]
	if string(first.Blob) != "hello sharepoint" {
		t.Fatalf("f1 blob = %q", first.Blob)
	}
	if first.SemanticIdentifier != "readme.txt" {
		t.Fatalf("f1 semantic = %q", first.SemanticIdentifier)
	}
	if first.Extension != ".txt" {
		t.Fatalf("f1 extension = %q", first.Extension)
	}
	if first.SizeBytes != 16 {
		t.Fatalf("f1 size = %d, want 16", first.SizeBytes)
	}
	if first.Metadata["drive"] != "Documents" || first.Metadata["drive_id"] != "drv-A" || first.Metadata["drive_item_id"] != "f1" {
		t.Fatalf("f1 metadata = %v", first.Metadata)
	}
	if first.Metadata["web_url"] != "https://contoso.sharepoint.com/f1" {
		t.Fatalf("f1 web_url = %v", first.Metadata["web_url"])
	}
	wantModified := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !first.UpdatedAt.Equal(wantModified) {
		t.Fatalf("f1 updated_at = %v, want %v", first.UpdatedAt, wantModified)
	}
	if byID["drv-A:f2"].Extension != ".md" {
		t.Fatalf("f2 extension = %q", byID["drv-A:f2"].Extension)
	}
	if fixtures.contentCalls.Load() != 3 {
		t.Fatalf("content calls = %d, want 3", fixtures.contentCalls.Load())
	}
}

func TestSharePointWindowFilter(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[{"id":"drv-A","name":"Documents","driveType":"documentLibrary"}]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[
				` + sharePointFileJSON("f1", "jan.txt", "https://contoso.sharepoint.com/f1", "2026-01-01T12:00:00Z", 1) + `,
				` + sharePointFileJSON("f2", "feb.txt", "https://contoso.sharepoint.com/f2", "2026-02-01T12:00:00Z", 1) + `,
				{"id":"f3","name":"no-time.txt","webUrl":"https://contoso.sharepoint.com/f3","size":1,"file":{"mimeType":"text/plain"}}
			]}`,
		},
		contentByKey: map[string]string{
			"drv-A:f1": "a", "drv-A:f2": "b", "drv-A:f3": "c",
		},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	windowStart := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &windowStart,
		WindowEnd:   windowEnd,
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := drainSharePointSync(t, session)
	if len(documents) != 2 {
		t.Fatalf("incremental documents = %d, want 2 (f2, f3)", len(documents))
	}
	ids := map[string]bool{}
	for _, document := range documents {
		ids[document.SourceID] = true
	}
	if !ids["drv-A:f2"] || !ids["drv-A:f3"] || ids["drv-A:f1"] {
		t.Fatalf("incremental ids = %v", ids)
	}

	fullSession, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync(full): %v", err)
	}
	full := drainSharePointSync(t, fullSession)
	if len(full) != 3 {
		t.Fatalf("full documents = %d, want 3", len(full))
	}
}

func TestSharePointResumeSkipsSyncedDrives(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[
			{"id":"drv-A","name":"A","driveType":"documentLibrary"},
			{"id":"drv-B","name":"B","driveType":"documentLibrary"},
			{"id":"drv-C","name":"C","driveType":"documentLibrary"}
		]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[` + sharePointFileJSON("a1", "a.txt", "https://c/a1", "2026-01-01T00:00:00Z", 1) + `]}`,
			"drv-B:/": `{"value":[` + sharePointFileJSON("b1", "b.txt", "https://c/b1", "2026-01-01T00:00:00Z", 1) + `]}`,
			"drv-C:/": `{"value":[` + sharePointFileJSON("c1", "c.txt", "https://c/c1", "2026-01-01T00:00:00Z", 1) + `]}`,
		},
		contentByKey: map[string]string{
			"drv-A:a1": "a", "drv-B:b1": "b", "drv-C:c1": "c",
		},
		contentCalls: &atomic.Int64{},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		Resume: &SyncCheckpoint{Cursor: "sharepoint_drive_drv-A"},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}

	var checkpoints []*SyncCheckpoint
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if batch.Checkpoint != nil {
			checkpoints = append(checkpoints, batch.Checkpoint)
		}
	}
	if len(checkpoints) != 2 {
		t.Fatalf("checkpoints = %d, want 2", len(checkpoints))
	}
	if checkpoints[0].Cursor != "sharepoint_drive_drv-B" || checkpoints[1].Cursor != "sharepoint_drive_drv-C" {
		t.Fatalf("checkpoint cursors = %q, %q", checkpoints[0].Cursor, checkpoints[1].Cursor)
	}
	if fixtures.contentCalls.Load() != 2 {
		t.Fatalf("content calls = %d, want 2 (drv-A skipped)", fixtures.contentCalls.Load())
	}
}

func TestSharePointResumeRejectsMissingDrive(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[
			{"id":"drv-A","name":"A","driveType":"documentLibrary"},
			{"id":"drv-B","name":"B","driveType":"documentLibrary"}
		]}`,
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		Resume: &SyncCheckpoint{Cursor: "sharepoint_drive_drv-C"},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestSharePointCheckpointAdvancesAfterDriveFlush(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[
			{"id":"drv-A","name":"A","driveType":"documentLibrary"},
			{"id":"drv-B","name":"B","driveType":"documentLibrary"}
		]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[` + sharePointFileJSON("a1", "a.txt", "https://c/a1", "2026-01-01T00:00:00Z", 1) + `]}`,
			"drv-B:/": `{"value":[` + sharePointFileJSON("b1", "b.txt", "https://c/b1", "2026-01-01T00:00:00Z", 1) + `]}`,
		},
		contentByKey: map[string]string{"drv-A:a1": "a", "drv-B:b1": "b"},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}

	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "drv-A:a1" {
		t.Fatalf("first batch = %+v", first)
	}
	if first.Checkpoint == nil || first.Checkpoint.Cursor != "sharepoint_drive_drv-A" {
		t.Fatalf("first checkpoint = %+v, want sharepoint_drive_drv-A", first.Checkpoint)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "drv-B:b1" {
		t.Fatalf("second batch = %+v", second)
	}
	if second.Checkpoint == nil || second.Checkpoint.Cursor != "sharepoint_drive_drv-B" {
		t.Fatalf("second checkpoint = %+v, want sharepoint_drive_drv-B", second.Checkpoint)
	}

	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final NextBatch err = %v, want io.EOF", err)
	}
}

func TestSharePointOpenPruneListsIdsWithoutDownload(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[
			{"id":"drv-A","name":"Documents","driveType":"documentLibrary"},
			{"id":"drv-B","name":"SiteAssets","driveType":"documentLibrary"}
		]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[
				` + sharePointFileJSON("f1", "a.txt", "https://c/f1", "2026-01-01T00:00:00Z", 1) + `,
				` + sharePointFolderJSON("d2", "sub") + `
			]}`,
			"drv-A:d2": `{"value":[` + sharePointFileJSON("f2", "b.txt", "https://c/f2", "2026-01-01T00:00:00Z", 1) + `]}`,
			"drv-B:/":  `{"value":[` + sharePointFileJSON("f3", "c.txt", "https://c/f3", "2026-01-01T00:00:00Z", 1) + `]}`,
		},
		contentCalls: &atomic.Int64{},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	prune, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	var ids []string
	for {
		batch, err := prune.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		for _, document := range batch.Documents {
			ids = append(ids, document.SourceID)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("slim ids = %v, want 3", ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, want := range []string{"drv-A:f1", "drv-A:f2", "drv-B:f3"} {
		if !seen[want] {
			t.Fatalf("missing slim id %q in %v", want, ids)
		}
	}
	if fixtures.contentCalls.Load() != 0 {
		t.Fatalf("content calls = %d, want 0", fixtures.contentCalls.Load())
	}
}

func TestSharePointChildrenPagination(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[{"id":"drv-A","name":"Documents","driveType":"documentLibrary"}]}`,
		childrenPage2: map[string]string{
			"drv-A:/": `{"value":[` + sharePointFileJSON("f2", "two.txt", "https://c/f2", "2026-01-01T00:00:00Z", 1) + `]}`,
		},
		contentByKey: map[string]string{"drv-A:f1": "one", "drv-A:f2": "two"},
	}
	server := newTestSharePointServer(t, fixtures)
	fixtures.childrenByKey = map[string]string{
		"drv-A:/": `{"@odata.nextLink":"` + server.URL + `/v1.0/drives/drv-A/root/children?page=2","value":[` +
			sharePointFileJSON("f1", "one.txt", "https://c/f1", "2026-01-01T00:00:00Z", 1) + `]}`,
	}
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := drainSharePointSync(t, session)
	if len(documents) != 2 {
		t.Fatalf("documents = %d, want 2 (paginated)", len(documents))
	}
}

func TestSharePointTokenCachedAcrossCalls(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		tokenCalls: &atomic.Int64{},
		drivesBody: `{"value":[{"id":"drv-A","name":"Documents","driveType":"documentLibrary"}]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[` + sharePointFileJSON("f1", "a.txt", "https://c/f1", "2026-01-01T00:00:00Z", 1) + `]}`,
		},
		contentByKey: map[string]string{"drv-A:f1": "a"},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	drainSharePointSync(t, session)

	if fixtures.tokenCalls.Load() != 1 {
		t.Fatalf("token calls = %d, want 1 (cached)", fixtures.tokenCalls.Load())
	}
}

func TestSharePointFileSizeCap(t *testing.T) {
	origMax := sharepointMaxFileSize
	sharepointMaxFileSize = 4
	t.Cleanup(func() { sharepointMaxFileSize = origMax })

	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[{"id":"drv-A","name":"Documents","driveType":"documentLibrary"}]}`,
		childrenByKey: map[string]string{
			"drv-A:/": `{"value":[` + sharePointFileJSON("f1", "big.txt", "https://c/f1", "2026-01-01T00:00:00Z", 100) + `]}`,
		},
		contentByKey: map[string]string{"drv-A:f1": "0123456789"},
	}
	server := newTestSharePointServer(t, fixtures)
	connector := mustSharePointConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("NextBatch err = %v, want size limit error", err)
	}
}
