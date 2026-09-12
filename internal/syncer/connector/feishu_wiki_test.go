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
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"strings"
	"testing"
	"time"
)

type feishuWikiTestServer struct {
	server    *httptest.Server
	authCalls int
	downloads []string
}

func newFeishuWikiTestServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *feishuWikiTestServer {
	t.Helper()
	fixture := &feishuWikiTestServer{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fixture.authCalls++
			feishuWriteJSON(w, map[string]any{"code": 0, "msg": "success", "tenant_access_token": "test-token", "expire": 7200})
			return
		}
		handle(w, r)
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func feishuWikiBaseConfig() map[string]any {
	return map[string]any{
		"credentials":     map[string]any{"app_id": "cli_test", "app_secret": "secret"},
		"space_id":        "space-1",
		"root_node_token": "wikcnroot",
	}
}

func newFeishuWikiTestConnector(t *testing.T, server *httptest.Server, config map[string]any) *FeishuWikiConnector {
	t.Helper()
	merged := feishuWikiBaseConfig()
	for key, value := range config {
		merged[key] = value
	}
	connector, err := NewFeishuWikiConnector(merged)
	if err != nil {
		t.Fatalf("NewFeishuWikiConnector: %v", err)
	}
	connector.baseURL = server.URL
	connector.httpClient = server.Client()
	return connector
}

func feishuWriteJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func feishuListNode(nodeToken, objType, objToken, title string, hasChild bool, editTime any) map[string]any {
	node := map[string]any{
		"node_token": nodeToken,
		"obj_type":   objType,
		"obj_token":  objToken,
		"title":      title,
		"has_child":  hasChild,
	}
	if editTime != nil {
		node["obj_edit_time"] = editTime
	}
	return node
}

func feishuListResponse(items []map[string]any, hasMore bool, pageToken string) map[string]any {
	return map[string]any{
		"code": 0,
		"msg":  "success",
		"data": map[string]any{"items": items, "has_more": hasMore, "page_token": pageToken},
	}
}

func feishuListEndpoint(r *http.Request) (string, string, string) {
	query := r.URL.Query()
	return query.Get("parent_node_token"), query.Get("page_token"), query.Get("space_id")
}

func TestFeishuWikiConnectorConfigValidation(t *testing.T) {
	base := func() map[string]any { return feishuWikiBaseConfig() }

	tests := []struct {
		name      string
		config    func() map[string]any
		wantError string
		missing   *ConnectorMissingCredentialError
		invalid   *ConnectorValidationError
	}{
		{"missing credentials", func() map[string]any {
			cfg := base()
			cfg["credentials"] = map[string]any{}
			return cfg
		}, "Feishu Wiki", &ConnectorMissingCredentialError{}, nil},
		{"missing space id", func() map[string]any {
			cfg := base()
			cfg["space_id"] = ""
			return cfg
		}, "space ID", nil, &ConnectorValidationError{}},
		{"missing root node", func() map[string]any {
			cfg := base()
			cfg["root_node_token"] = ""
			return cfg
		}, "root node token", nil, &ConnectorValidationError{}},
		{"batch size zero", func() map[string]any {
			cfg := base()
			cfg["batch_size"] = 0
			return cfg
		}, "batch_size", nil, &ConnectorValidationError{}},
		{"batch size too large", func() map[string]any {
			cfg := base()
			cfg["batch_size"] = 11
			return cfg
		}, "batch_size", nil, &ConnectorValidationError{}},
		{"batch size not integer", func() map[string]any {
			cfg := base()
			cfg["batch_size"] = "abc"
			return cfg
		}, "batch_size", nil, &ConnectorValidationError{}},
		{"max size zero", func() map[string]any {
			cfg := base()
			cfg["max_file_size_bytes"] = 0
			return cfg
		}, "max_file_size_bytes", nil, &ConnectorValidationError{}},
		{"unsupported extension", func() map[string]any {
			cfg := base()
			cfg["include_extensions"] = []string{"exe"}
			return cfg
		}, "Unsupported Feishu Wiki file extension", nil, &ConnectorValidationError{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewFeishuWikiConnector(test.config())
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %q, want substring %q", err.Error(), test.wantError)
			}
			if test.missing != nil && !errors.As(err, &test.missing) {
				t.Fatalf("error type = %T, want *ConnectorMissingCredentialError", err)
			}
			if test.invalid != nil && !errors.As(err, &test.invalid) {
				t.Fatalf("error type = %T, want *ConnectorValidationError", err)
			}
		})
	}
}

func TestFeishuWikiConnectorDefaultsAndNormalization(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		feishuWriteJSON(w, feishuListResponse(nil, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, map[string]any{
		"include_extensions": []any{"pdf", ".docx", "PDF"},
		"include_keywords":   "project, handbook",
		"exclude_keywords":   []any{"draft"},
	})
	if connector.batchSize != defaultFeishuBatchSize {
		t.Fatalf("batch_size = %d, want %d", connector.batchSize, defaultFeishuBatchSize)
	}
	if connector.maxFileSizeBytes != defaultFeishuMaxFileSize {
		t.Fatalf("max_file_size_bytes = %d, want %d", connector.maxFileSizeBytes, defaultFeishuMaxFileSize)
	}
	for _, ext := range []string{".pdf", ".docx"} {
		if _, ok := connector.includeExtensions[ext]; !ok {
			t.Fatalf("include_extensions missing %q: %v", ext, connector.includeExtensions)
		}
	}
	if _, ok := connector.includeKeywords["handbook"]; !ok {
		t.Fatalf("include_keywords missing handbook: %v", connector.includeKeywords)
	}
	if _, ok := connector.excludeKeywords["draft"]; !ok {
		t.Fatalf("exclude_keywords missing draft: %v", connector.excludeKeywords)
	}
}

func TestFeishuWikiAuthTokenCaching(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		feishuWriteJSON(w, feishuListResponse(nil, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)

	token, err := connector.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("getAccessToken: %v", err)
	}
	if token != "test-token" {
		t.Fatalf("token = %q, want test-token", token)
	}
	if server.authCalls != 1 {
		t.Fatalf("auth calls = %d, want 1", server.authCalls)
	}
	// Cached token must not re-authenticate.
	_, _ = connector.getAccessToken(context.Background())
	if server.authCalls != 1 {
		t.Fatalf("auth calls after cache = %d, want 1", server.authCalls)
	}
	// Expired token must refresh.
	connector.accessTokenExpiresAt = time.Now().Add(-time.Minute)
	_, _ = connector.getAccessToken(context.Background())
	if server.authCalls != 2 {
		t.Fatalf("auth calls after expiry = %d, want 2", server.authCalls)
	}
}

func TestFeishuWikiCollectFilesPaginationAndRecursion(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/spaces/") {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		parent, pageToken, _ := feishuListEndpoint(r)
		switch parent {
		case "wikcnroot":
			if pageToken == "" {
				feishuWriteJSON(w, feishuListResponse([]map[string]any{
					feishuListNode("n1", "file", "o1", "a.pdf", false, "1700000000"),
					feishuListNode("n2", "docx", "odoc", "native doc", true, "1700000001"),
				}, true, "p2"))
				return
			}
			feishuWriteJSON(w, feishuListResponse([]map[string]any{
				feishuListNode("n3", "file", "o3", "b.docx", false, "1700000002"),
			}, false, ""))
		case "n2":
			feishuWriteJSON(w, feishuListResponse([]map[string]any{
				feishuListNode("n4", "file", "o4", "nested.pdf", false, "1700000003"),
			}, false, ""))
		default:
			feishuWriteJSON(w, feishuListResponse(nil, false, ""))
		}
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)

	files, err := connector.collectFiles(context.Background())
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("collected %d files, want 3: %+v", len(files), files)
	}
	// n2 is a non-file node and is not collected, but its child n4 is.
	got := []string{files[0].nodeToken, files[1].nodeToken, files[2].nodeToken}
	want := []string{"n1", "n3", "n4"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collected order = %v, want %v", got, want)
		}
	}
	if !files[2].hasUpdatedAt || !files[2].updatedAt.Equal(time.Unix(1700000003, 0).UTC()) {
		t.Fatalf("nested file updatedAt = %v (has=%v), want 1700000003", files[2].updatedAt, files[2].hasUpdatedAt)
	}
}

func TestFeishuWikiFilters(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		feishuWriteJSON(w, feishuListResponse(nil, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, map[string]any{
		"include_extensions": []any{"pdf"},
		"include_keywords":   []any{"handbook"},
		"exclude_keywords":   []any{"draft"},
	})

	if !connector.matchesFilters("Project Handbook.pdf") {
		t.Fatalf("expected handbook.pdf to match")
	}
	if connector.matchesFilters("Other.pdf") {
		t.Fatalf("expected Other.pdf to be excluded by include_keywords")
	}
	if connector.matchesFilters("Handbook Draft.pdf") {
		t.Fatalf("expected Handbook Draft.pdf to be excluded by exclude_keywords")
	}
	if connector.matchesFilters("Handbook.docx") {
		t.Fatalf("expected Handbook.docx to be excluded by include_extensions")
	}
	if connector.matchesFilters("Handbook.exe") {
		t.Fatalf("expected Handbook.exe to be excluded by supported extensions")
	}
}

func TestFeishuWikiWindowFiltering(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		feishuWriteJSON(w, feishuListResponse([]map[string]any{
			feishuListNode("before", "file", "o-before", "before.pdf", false, "1700000000"),
			feishuListNode("inside", "file", "o-inside", "inside.pdf", false, "1700000100"),
			feishuListNode("after", "file", "o-after", "after.pdf", false, "1700000200"),
			feishuListNode("missing", "file", "o-missing", "missing.pdf", false, nil),
		}, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)

	start := time.Unix(1700000050, 0).UTC()
	end := time.Unix(1700000150, 0).UTC()
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	feishuSession := session.(*feishuWikiSyncSession)
	if len(feishuSession.files) != 2 {
		t.Fatalf("accepted %d files, want 2 (inside + missing timestamp): %+v", len(feishuSession.files), feishuSession.files)
	}
}

func TestFeishuWikiDownload(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/files/") {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		objectToken := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open-apis/drive/v1/files/"), "/download")
		switch objectToken {
		case "o-good":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "file-bytes")
		case "o-declared-oversize":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "999999999")
			_, _ = io.WriteString(w, "file-bytes")
		case "o-body-oversize":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, strings.Repeat("x", 51*1024*1024))
		case "o-forbidden":
			w.WriteHeader(http.StatusForbidden)
			feishuWriteJSON(w, map[string]any{"code": 99991663, "msg": "permission denied"})
		case "o-http-error":
			w.WriteHeader(http.StatusBadRequest)
			feishuWriteJSON(w, map[string]any{"code": 131006, "msg": "invalid param"})
		case "o-json-envelope":
			w.Header().Set("Content-Type", "application/json")
			feishuWriteJSON(w, map[string]any{"code": 1061045, "msg": "rate limit exceeded"})
		case "o-json-file":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"hello":"world"}`)
		default:
			http.NotFound(w, r)
		}
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)
	ctx := context.Background()

	if blob, err := connector.downloadFile(ctx, "o-good"); err != nil || string(blob) != "file-bytes" {
		t.Fatalf("download good = %q, %v", blob, err)
	}
	if _, err := connector.downloadFile(ctx, "o-declared-oversize"); err == nil || !strings.Contains(err.Error(), "size exceeds") {
		t.Fatalf("declared oversize error = %v", err)
	}
	if _, err := connector.downloadFile(ctx, "o-body-oversize"); err == nil || !strings.Contains(err.Error(), "size exceeds") {
		t.Fatalf("body oversize error = %v", err)
	}
	if _, err := connector.downloadFile(ctx, "o-forbidden"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("forbidden error = %v", err)
	}
	if _, err := connector.downloadFile(ctx, "o-http-error"); err == nil || !strings.Contains(err.Error(), "API error 131006") || !strings.Contains(err.Error(), "invalid param") {
		t.Fatalf("http error = %v", err)
	}
	if _, err := connector.downloadFile(ctx, "o-json-envelope"); err == nil || !strings.Contains(err.Error(), "JSON error envelope") {
		t.Fatalf("json envelope error = %v", err)
	}
	if blob, err := connector.downloadFile(ctx, "o-json-file"); err != nil || string(blob) != `{"hello":"world"}` {
		t.Fatalf("json file download = %q, %v", blob, err)
	}
}

func TestFeishuWikiSyncSessionBatchingAndResume(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		feishuWriteJSON(w, feishuListResponse([]map[string]any{
			feishuListNode("n1", "file", "o1", "a.pdf", false, "1700000000"),
			feishuListNode("n2", "file", "o2", "b.pdf", false, "1700000001"),
			feishuListNode("n3", "file", "o3", "c.pdf", false, "1700000002"),
			feishuListNode("n4", "file", "o4", "d.pdf", false, "1700000003"),
			feishuListNode("n5", "file", "o5", "e.pdf", false, "1700000004"),
		}, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, map[string]any{"batch_size": 2})

	end := time.Now().UTC()
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	feishuSession := session.(*feishuWikiSyncSession)
	if len(feishuSession.files) != 5 {
		t.Fatalf("accepted %d files, want 5", len(feishuSession.files))
	}

	batch, err := feishuSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch 1: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("batch 1 size = %d, want 2", len(batch.Documents))
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "feishu_wiki:space-1:n2" {
		t.Fatalf("batch 1 checkpoint = %+v", batch.Checkpoint)
	}
	for i, document := range batch.Documents {
		if document.Blob == nil || len(document.Blob) == 0 {
			t.Fatalf("batch 1 doc %d has no blob", i)
		}
		if document.SemanticIdentifier == "" || document.Fingerprint == "" {
			t.Fatalf("batch 1 doc %d missing identifier/fingerprint", i)
		}
	}

	batch2, err := feishuSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch 2: %v", err)
	}
	if len(batch2.Documents) != 2 || batch2.Checkpoint.SourceID != "feishu_wiki:space-1:n4" {
		t.Fatalf("batch 2 = %d docs, checkpoint %+v", len(batch2.Documents), batch2.Checkpoint)
	}

	batch3, err := feishuSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch 3: %v", err)
	}
	if len(batch3.Documents) != 1 || batch3.Checkpoint.SourceID != "feishu_wiki:space-1:n5" {
		t.Fatalf("batch 3 = %d docs, checkpoint %+v", len(batch3.Documents), batch3.Checkpoint)
	}
	if _, err := feishuSession.NextBatch(context.Background()); err != io.EOF {
		t.Fatalf("NextBatch after end = %v, want io.EOF", err)
	}

	// Resume from the batch-2 checkpoint advances past the committed docs.
	resumed, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowEnd: end,
		Resume:    batch2.Checkpoint,
	})
	if err != nil {
		t.Fatalf("OpenSync with resume: %v", err)
	}
	resumedSession := resumed.(*feishuWikiSyncSession)
	if resumedSession.index != 4 {
		t.Fatalf("resume index = %d, want 4", resumedSession.index)
	}

	// An anchor that is no longer present forces a window restart.
	_, err = connector.OpenSync(context.Background(), SyncRequest{
		WindowEnd: end,
		Resume:    &SyncCheckpoint{SourceID: "feishu_wiki:space-1:gone"},
	})
	if err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume with missing anchor = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestFeishuWikiOpenPruneUnsupported(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		feishuWriteJSON(w, feishuListResponse(nil, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)
	_, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err == nil || !errors.Is(err, ErrPruneUnsupported) {
		t.Fatalf("OpenPrune = %v, want ErrPruneUnsupported", err)
	}
}

func TestFeishuWikiValidateConnects(t *testing.T) {
	server := newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		parent, _, _ := feishuListEndpoint(r)
		if parent != "wikcnroot" {
			t.Errorf("validate listed parent %q, want wikcnroot", parent)
		}
		feishuWriteJSON(w, feishuListResponse(nil, false, ""))
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)
	if err := connector.ValidateConnectorSetting(context.Background(), feishuWikiBaseConfig()); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
	if server.authCalls != 1 {
		t.Fatalf("auth calls = %d, want 1", server.authCalls)
	}
}

func TestFeishuWiki429Retry(t *testing.T) {
	var server *feishuWikiTestServer
	server = newFeishuWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/files/") {
			http.NotFound(w, r)
			return
		}
		if len(server.downloads) == 0 {
			w.Header().Set("x-ogw-ratelimit-reset", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			feishuWriteJSON(w, map[string]any{"code": 99991400, "msg": "request trigger frequency limit"})
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "retried-bytes")
		}
		server.downloads = append(server.downloads, r.URL.Path)
	})
	connector := newFeishuWikiTestConnector(t, server.server, nil)

	savedWaits := feishu429MaxWaits
	feishu429MaxWaits = 2
	t.Cleanup(func() { feishu429MaxWaits = savedWaits })

	blob, err := connector.downloadFile(context.Background(), "o-1")
	if err != nil {
		t.Fatalf("download after 429: %v", err)
	}
	if string(blob) != "retried-bytes" {
		t.Fatalf("blob = %q", blob)
	}
	if len(server.downloads) != 2 {
		t.Fatalf("download requests = %d, want 2", len(server.downloads))
	}
}

func TestFeishuWikiRegistryRegistration(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)

	connector, err := registry.OpenFromConfig("feishu_wiki", feishuWikiBaseConfig())
	if err != nil {
		t.Fatalf("OpenFromConfig feishu_wiki: %v", err)
	}
	if _, ok := connector.(*FeishuWikiConnector); !ok {
		t.Fatalf("connector type = %T, want *FeishuWikiConnector", connector)
	}

	_, err = registry.OpenFromConfig("feishu_wiki", map[string]any{})
	var credErr *ConnectorMissingCredentialError
	if err == nil || !errors.As(err, &credErr) {
		t.Fatalf("missing-credential error = %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "Feishu Wiki") {
		t.Fatalf("missing-credential message = %v", err)
	}

	// The task-context factory path must also resolve.
	taskConnector, err := registry.Open(context.Background(), dao.SyncTaskContext{
		Connector: entity.Connector{Source: "feishu_wiki", Config: entity.JSONMap(feishuWikiBaseConfig())},
	})
	if err != nil {
		t.Fatalf("Open feishu_wiki: %v", err)
	}
	if _, ok := taskConnector.(*FeishuWikiConnector); !ok {
		t.Fatalf("task connector type = %T", taskConnector)
	}
}
