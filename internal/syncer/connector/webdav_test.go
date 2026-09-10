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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type webdavTestFile struct {
	content  string
	size     int64
	hasSize  bool
	modified string
	isDir    bool
}

// newWebDAVTestServer serves a small static WebDAV tree.
func newWebDAVTestServer(t *testing.T, files map[string]webdavTestFile) (*httptest.Server, *[]string, *[]string) {
	t.Helper()
	return newWebDAVTestServerAtMount(t, "", files)
}

// newWebDAVMountedTestServer serves a small static WebDAV tree under a non-root endpoint path.
func newWebDAVMountedTestServer(t *testing.T, mountPath string, files map[string]webdavTestFile) (*httptest.Server, *[]string, *[]string) {
	t.Helper()
	return newWebDAVTestServerAtMount(t, mountPath, files)
}

func newWebDAVTestServerAtMount(t *testing.T, mountPath string, files map[string]webdavTestFile) (*httptest.Server, *[]string, *[]string) {
	t.Helper()
	var authHeaders []string
	var getRequests []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if mountPath != "" && !strings.HasPrefix(r.URL.Path, mountPath) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		apiPath := strings.TrimPrefix(r.URL.Path, mountPath)
		if apiPath == "" {
			apiPath = "/"
		}

		switch r.Method {
		case "PROPFIND":
			if _, ok := files[apiPath]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var body bytes.Buffer
			body.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
			body.WriteString(`<D:multistatus xmlns:D="DAV:">` + "\n")
			writeWebDAVTestResponse(&body, r.URL.Path, files[apiPath])
			for itemPath, item := range files {
				if itemPath != apiPath && webDAVTestParent(itemPath) == apiPath {
					href := itemPath
					if mountPath != "" {
						href = mountPath + itemPath
					}
					writeWebDAVTestResponse(&body, href, item)
				}
			}
			body.WriteString(`</D:multistatus>`)
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, body.String())
		case "GET":
			entry, ok := files[apiPath]
			if !ok || entry.isDir {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			getRequests = append(getRequests, r.URL.Path)
			_, _ = io.WriteString(w, entry.content)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, &authHeaders, &getRequests
}

func writeWebDAVTestResponse(body *bytes.Buffer, href string, file webdavTestFile) {
	body.WriteString(`<D:response><D:href>` + href + `</D:href><D:propstat><D:prop>`)
	if file.isDir {
		body.WriteString(`<D:resourcetype><D:collection/></D:resourcetype>`)
	}
	if file.hasSize {
		fmt.Fprintf(body, `<D:getcontentlength>%d</D:getcontentlength>`, file.size)
	}
	if file.modified != "" {
		body.WriteString(`<D:getlastmodified>` + file.modified + `</D:getlastmodified>`)
	}
	body.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`)
}

func webDAVTestParent(itemPath string) string {
	trimmed := strings.TrimRight(itemPath, "/")
	if trimmed == "" {
		return "/"
	}
	index := strings.LastIndex(trimmed, "/")
	if index < 0 {
		return "/"
	}
	return trimmed[:index+1]
}

func webDAVTestTree() map[string]webdavTestFile {
	return map[string]webdavTestFile{
		"/":                 {isDir: true},
		"/notes/":           {isDir: true},
		"/notes/alpha.txt":  {content: "hello alpha", size: 11, hasSize: true, modified: "Fri, 02 Jan 2026 00:00:00 GMT"},
		"/notes/beta.md":    {content: "hello beta", size: 10, hasSize: true, modified: "Sat, 03 Jan 2026 00:00:00 GMT"},
		"/notes/big.bin":    {content: "bin", size: 3, hasSize: true, modified: "Thu, 01 Jan 2026 00:00:00 GMT"},
		"/notes/skip.png":   {content: "png", size: 3, hasSize: true, modified: "Thu, 01 Jan 2026 00:00:00 GMT"},
		"/notes/nosize.txt": {content: "x", hasSize: false, modified: "Thu, 01 Jan 2026 00:00:00 GMT"},
		"/notes/huge.txt":   {content: "huge", size: 21, hasSize: true, modified: "Thu, 01 Jan 2026 00:00:00 GMT"},
		"/docs/":            {isDir: true},
		"/docs/alpha.txt":   {content: "hello alpha docs", size: 15, hasSize: true, modified: "Fri, 02 Jan 2026 00:00:00 GMT"},
		"/docs/gamma.pdf":   {content: "hello gamma", size: 11, hasSize: true, modified: "Tue, 06 Jan 2026 00:00:00 GMT"},
	}
}

func webDAVTestConnector(t *testing.T, serverURL string, allowImages bool, batchSize int) *WebDAVConnector {
	t.Helper()
	config := map[string]any{
		"base_url":     serverURL,
		"remote_path":  "/",
		"batch_size":   batchSize,
		"allow_images": allowImages,
		"credentials": map[string]any{
			"username": "user",
			"password": "pass",
		},
	}
	connector, err := NewWebDAVConnector(config)
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	return connector
}

func TestNewWebDAVConnectorPreservesDefaultHTTPTransport(t *testing.T) {
	connector, err := NewWebDAVConnector(map[string]any{})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	if connector.client.httpClient.Transport != nil {
		t.Fatalf("HTTP transport = %T, want nil default transport", connector.client.httpClient.Transport)
	}
}

func TestNewWebDAVConnectorUsesCustomCACertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	caCertPath := filepath.Join(t.TempDir(), "webdav-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caCertPath, caPEM, 0o600); err != nil {
		t.Fatalf("write CA certificate: %v", err)
	}

	connector, err := NewWebDAVConnector(map[string]any{
		"base_url":     server.URL,
		"ca_cert_path": caCertPath,
	})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), webdavRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := connector.client.httpClient.Do(req)
	if err != nil {
		t.Fatalf("request with custom CA failed: %v", err)
	}
	defer resp.Body.Close()
}

func TestNewWebDAVConnectorRejectsInvalidCACertificate(t *testing.T) {
	tests := []struct {
		name       string
		caCertPath func(t *testing.T) string
	}{
		{
			name: "missing file",
			caCertPath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.pem")
			},
		},
		{
			name: "invalid PEM",
			caCertPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "invalid.pem")
				if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
					t.Fatalf("write invalid CA certificate: %v", err)
				}
				return path
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWebDAVConnector(map[string]any{"ca_cert_path": tt.caCertPath(t)})
			var validationErr *ConnectorValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want ConnectorValidationError", err)
			}
		})
	}
}

func TestWebDAVConnectorValidate(t *testing.T) {
	server, _, _ := newWebDAVTestServer(t, webDAVTestTree())

	connector := webDAVTestConnector(t, server.URL, false, 2)
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	missingCredentials, err := NewWebDAVConnector(map[string]any{
		"base_url":    server.URL,
		"credentials": map[string]any{},
	})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	if err := missingCredentials.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "username and password") {
		t.Fatalf("missing credentials error = %v", err)
	}

	badPassword, err := NewWebDAVConnector(map[string]any{
		"base_url": server.URL,
		"credentials": map[string]any{
			"username": "user",
			"password": "wrong",
		},
	})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	if err := badPassword.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "credentials appear invalid") {
		t.Fatalf("bad password error = %v", err)
	}

	missingPath, err := NewWebDAVConnector(map[string]any{
		"base_url":    server.URL,
		"remote_path": "/missing",
		"credentials": map[string]any{
			"username": "user",
			"password": "pass",
		},
	})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	if err := missingPath.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing path error = %v", err)
	}
}

func TestWebDAVConnectorValidateConnectorSetting(t *testing.T) {
	server, _, _ := newWebDAVTestServer(t, webDAVTestTree())

	connector := webDAVTestConnector(t, server.URL, false, 2)
	if err := connector.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}

	missingCredentials, err := NewWebDAVConnector(map[string]any{
		"base_url":    server.URL,
		"credentials": map[string]any{},
	})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}
	err = missingCredentials.ValidateConnectorSetting(t.Context(), nil)
	if err == nil || !strings.Contains(err.Error(), "username and password") {
		t.Fatalf("missing credentials error = %v", err)
	}
}

func TestWebDAVConnectorOpenSyncFull(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, authHeaders, _ := newWebDAVTestServer(t, webDAVTestTree())
	connector := webDAVTestConnector(t, server.URL, false, 2)

	session, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     mustTime(t, "2026-01-07T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 2 {
		t.Fatalf("first batch len = %d, want 2: %+v", len(first.Documents), first.Documents)
	}
	alphaDocs := first.Documents[0]
	if alphaDocs.SourceID != "webdav:"+server.URL+":"+server.URL+"/docs/alpha.txt" {
		t.Fatalf("docs alpha source id = %s", alphaDocs.SourceID)
	}
	if alphaDocs.SemanticIdentifier != "docs / alpha.txt" {
		t.Fatalf("docs alpha semantic identifier = %q", alphaDocs.SemanticIdentifier)
	}
	if alphaDocs.Extension != ".txt" {
		t.Fatalf("docs alpha extension = %q", alphaDocs.Extension)
	}
	if string(alphaDocs.Blob) != "hello alpha docs" || alphaDocs.SizeBytes != 15 {
		t.Fatalf("docs alpha blob/size = %q/%d", alphaDocs.Blob, alphaDocs.SizeBytes)
	}
	if !alphaDocs.UpdatedAt.Equal(mustTime(t, "2026-01-02T00:00:00Z")) {
		t.Fatalf("docs alpha updated at = %v", alphaDocs.UpdatedAt)
	}
	if alphaDocs.Fingerprint == "" {
		t.Fatalf("docs alpha fingerprint is empty")
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "webdav:"+server.URL+":"+server.URL+"/docs/gamma.pdf" {
		t.Fatalf("first checkpoint = %+v", first.Checkpoint)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch second failed: %v", err)
	}
	if len(second.Documents) != 2 {
		t.Fatalf("second batch len = %d, want 2", len(second.Documents))
	}
	notesAlpha := second.Documents[0]
	if notesAlpha.SourceID != "webdav:"+server.URL+":"+server.URL+"/notes/alpha.txt" {
		t.Fatalf("notes alpha source id = %s", notesAlpha.SourceID)
	}
	if notesAlpha.SemanticIdentifier != "notes / alpha.txt" {
		t.Fatalf("notes alpha semantic identifier = %q", notesAlpha.SemanticIdentifier)
	}
	if string(second.Documents[1].Blob) != "hello beta" {
		t.Fatalf("beta blob = %q", second.Documents[1].Blob)
	}

	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if len(*authHeaders) == 0 || (*authHeaders)[0] != expectedAuth {
		t.Fatalf("auth headers = %v, want %s", *authHeaders, expectedAuth)
	}
}

func TestWebDAVConnectorOpenSyncIncremental(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, _ := newWebDAVTestServer(t, webDAVTestTree())
	connector := webDAVTestConnector(t, server.URL, false, 2)

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("incremental batch len = %d, want 1: %+v", len(batch.Documents), batch.Documents)
	}
	if batch.Documents[0].SemanticIdentifier != "beta.md" {
		t.Fatalf("incremental document = %q", batch.Documents[0].SemanticIdentifier)
	}
}

func TestWebDAVConnectorMountedUnderNonRootPath(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, getRequests := newWebDAVMountedTestServer(t, "/webdav", webDAVTestTree())
	connector := webDAVTestConnector(t, server.URL+"/webdav", false, 10)

	resolved, err := connector.client.resolve("/notes")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved != server.URL+"/webdav/notes" {
		t.Fatalf("resolved URL = %q, want %q", resolved, server.URL+"/webdav/notes")
	}

	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 4 {
		t.Fatalf("batch len = %d, want 4: %+v", len(batch.Documents), batch.Documents)
	}
	mountedSourceID := "webdav:" + server.URL + "/webdav:" + server.URL + "/webdav/notes/alpha.txt"
	foundMountedSourceID := false
	for _, doc := range batch.Documents {
		if doc.SourceID == mountedSourceID {
			foundMountedSourceID = true
		}
		if !strings.HasPrefix(doc.SourceID, "webdav:"+server.URL+"/webdav:") {
			t.Fatalf("source id escaped mount path: %q", doc.SourceID)
		}
	}
	if !foundMountedSourceID {
		t.Fatalf("missing mounted source id %q in %+v", mountedSourceID, batch.Documents)
	}
	if len(*getRequests) != 4 {
		t.Fatalf("download requests = %d, want 4: %v", len(*getRequests), *getRequests)
	}
	for _, requestPath := range *getRequests {
		if !strings.HasPrefix(requestPath, "/webdav/") {
			t.Fatalf("download request escaped mount path: %q", requestPath)
		}
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestWebDAVConnectorOpenSyncIncludesSizedFileWithoutLastModified(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, _ := newWebDAVTestServer(t, map[string]webdavTestFile{
		"/":                {isDir: true},
		"/notes/":          {isDir: true},
		"/notes/nomod.txt": {content: "no modified", size: 12, hasSize: true},
	})
	connector := webDAVTestConnector(t, server.URL, false, 10)

	windowEnd := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: windowEnd})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("batch len = %d, want 1: %+v", len(batch.Documents), batch.Documents)
	}
	if !batch.Documents[0].UpdatedAt.Equal(windowEnd) {
		t.Fatalf("updated at = %v, want %v", batch.Documents[0].UpdatedAt, windowEnd)
	}

	unsetSession, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync with unset WindowEnd failed: %v", err)
	}
	defer unsetSession.Close()
	unsetBatch, err := unsetSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch with unset WindowEnd failed: %v", err)
	}
	if len(unsetBatch.Documents) != 1 {
		t.Fatalf("unset window batch len = %d, want 1: %+v", len(unsetBatch.Documents), unsetBatch.Documents)
	}
	if batch.Documents[0].SourceID != unsetBatch.Documents[0].SourceID {
		t.Fatalf("unset window source id = %q", unsetBatch.Documents[0].SourceID)
	}
	if string(unsetBatch.Documents[0].Blob) != "no modified" {
		t.Fatalf("unset window blob = %q", unsetBatch.Documents[0].Blob)
	}
	if unsetBatch.Documents[0].UpdatedAt.IsZero() {
		t.Fatalf("unset window updated at is zero")
	}
}

func TestWebDAVConnectorOpenSyncRejectsOversizedResponse(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", fmt.Sprintf("%d", maxWebDAVResponseSize+1))
	content := strings.Repeat("x", maxWebDAVResponseSize+1)
	server, _, getRequests := newWebDAVTestServer(t, map[string]webdavTestFile{
		"/":                {isDir: true},
		"/notes/":          {isDir: true},
		"/notes/large.txt": {content: content, size: int64(len(content)), hasSize: true, modified: "Fri, 02 Jan 2026 00:00:00 GMT"},
	})
	connector := webDAVTestConnector(t, server.URL, false, 10)

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch = %v, want EOF", err)
	}
	if len(*getRequests) != 1 {
		t.Fatalf("download requests = %d, want 1", len(*getRequests))
	}
}

func TestWebDAVConnectorOpenSyncResumesAfterCheckpoint(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, _ := newWebDAVTestServer(t, webDAVTestTree())
	connector := webDAVTestConnector(t, server.URL, false, 2)

	windowEnd := mustTime(t, "2026-01-07T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: windowEnd})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}
	session.Close()

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     windowEnd,
		Resume:        first.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	defer resumed.Close()
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 2 || second.Documents[0].SemanticIdentifier != "notes / alpha.txt" {
		t.Fatalf("resume documents = %+v", second.Documents)
	}
	if second.Checkpoint == nil || second.Checkpoint.SourceID != "webdav:"+server.URL+":"+server.URL+"/notes/beta.md" {
		t.Fatalf("resume checkpoint = %+v", second.Checkpoint)
	}
	if _, err := resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resume EOF = %v", err)
	}
}

func TestWebDAVConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, _ := newWebDAVTestServer(t, webDAVTestTree())
	connector := webDAVTestConnector(t, server.URL, false, 2)

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestWebDAVConnectorOpenPrune(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, getRequests := newWebDAVTestServer(t, webDAVTestTree())
	connector := webDAVTestConnector(t, server.URL, false, 10)

	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	defer session.Close()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 4 {
		t.Fatalf("prune batch len = %d, want 4: %+v", len(batch.Documents), batch.Documents)
	}
	if batch.Documents[0].SourceID != "webdav:"+server.URL+":"+server.URL+"/docs/alpha.txt" ||
		batch.Documents[3].SourceID != "webdav:"+server.URL+":"+server.URL+"/notes/beta.md" {
		t.Fatalf("unexpected prune ids: %+v", batch.Documents)
	}
	if len(*getRequests) != 0 {
		t.Fatalf("prune must not download files, got GETs: %v", *getRequests)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("prune EOF = %v", err)
	}
}

func TestWebDAVConnectorFiltersAndImages(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")
	server, _, _ := newWebDAVTestServer(t, webDAVTestTree())

	withoutImages := webDAVTestConnector(t, server.URL, false, 10)
	session, err := withoutImages.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	for _, doc := range batch.Documents {
		if strings.Contains(doc.SourceID, "skip.png") || strings.Contains(doc.SourceID, "huge.txt") ||
			strings.Contains(doc.SourceID, "nosize.txt") || strings.Contains(doc.SourceID, "big.bin") {
			t.Fatalf("filtered document leaked into batch: %s", doc.SourceID)
		}
	}

	withImages := webDAVTestConnector(t, server.URL, true, 10)
	imageSession, err := withImages.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync with images failed: %v", err)
	}
	defer imageSession.Close()
	imageBatch, err := imageSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch with images failed: %v", err)
	}
	if len(imageBatch.Documents) != 5 {
		t.Fatalf("with images batch len = %d, want 5", len(imageBatch.Documents))
	}
}

// TestWebDAVConnectorOpenSyncRejectsExternalHref verifies that PROPFIND responses
// containing absolute or protocol-relative hrefs to a different origin are rejected
// at URL resolution time, preventing any GET to the external host.
func TestWebDAVConnectorOpenSyncRejectsExternalHref(t *testing.T) {
	t.Setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20")

	var externalRequests int
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalRequests++
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(externalServer.Close)
	externalHost := strings.TrimPrefix(externalServer.URL, "http://")

	var getPaths []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			getPaths = append(getPaths, r.URL.Path)
		}
		if r.Method != "PROPFIND" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:">
<D:response><D:href>/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
<D:response><D:href>/notes/alpha.txt</D:href><D:propstat><D:prop><D:getcontentlength>11</D:getcontentlength><D:getlastmodified>Fri, 02 Jan 2026 00:00:00 GMT</D:getlastmodified></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
<D:response><D:href>%s/secret.txt</D:href><D:propstat><D:prop><D:getcontentlength>5</D:getcontentlength><D:getlastmodified>Fri, 02 Jan 2026 00:00:00 GMT</D:getlastmodified></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
<D:response><D:href>//%s/secret.txt</D:href><D:propstat><D:prop><D:getcontentlength>5</D:getcontentlength><D:getlastmodified>Fri, 02 Jan 2026 00:00:00 GMT</D:getlastmodified></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
</D:multistatus>`, externalServer.URL, externalHost))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Build connector without auth so the test handler stays simple.
	connector, err := NewWebDAVConnector(map[string]any{
		"base_url":    server.URL,
		"remote_path": "/",
		"batch_size":  10,
	})
	if err != nil {
		t.Fatalf("NewWebDAVConnector failed: %v", err)
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

	for {
		batch, batchErr := session.NextBatch(context.Background())
		if errors.Is(batchErr, io.EOF) {
			break
		}
		if batchErr != nil {
			t.Fatalf("NextBatch failed: %v", batchErr)
		}
		for _, doc := range batch.Documents {
			if strings.Contains(doc.SourceID, externalHost) {
				t.Fatalf("external href leaked into sync results: %s", doc.SourceID)
			}
		}
	}

	if externalRequests != 0 {
		t.Fatalf("external server received %d request(s), want 0", externalRequests)
	}
	if len(getPaths) == 0 {
		t.Fatalf("expected GET for the legitimate file")
	}
}
