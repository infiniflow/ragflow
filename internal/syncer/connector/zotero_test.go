package connector

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/utility"
)

func TestZoteroConnectorOpenSyncDownloadsPDFOverHTTP(t *testing.T) {
	server := newZoteroHTTPServer(t)
	connector := newHTTPZoteroConnector(t, server, zoteroStorageModeZotero, 1)
	start := mustTime(t, "2025-12-01T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: mustTime(t, "2026-02-01T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "zotero:12345678:ATTACH1" {
		t.Fatalf("unexpected batch: %+v", first.Documents)
	}
	if string(first.Documents[0].Blob) != "%PDF-1.4 test" {
		t.Fatalf("blob = %q", string(first.Documents[0].Blob))
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestZoteroConnectorOpenSyncIncrementalWindow(t *testing.T) {
	server := newZoteroHTTPServer(t)
	connector := newHTTPZoteroConnector(t, server, zoteroStorageModeZotero, 10)
	start := mustTime(t, "2025-12-01T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-15T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "zotero:12345678:ATTACH1" {
		t.Fatalf("unexpected batch: %+v", batch.Documents)
	}
}

func TestZoteroConnectorSkipsLinkedAttachments(t *testing.T) {
	server := newZoteroHTTPServer(t)
	connector := newHTTPZoteroConnector(t, server, zoteroStorageModeZotero, 10)
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		for _, doc := range batch.Documents {
			if doc.SourceID == "zotero:12345678:LINKED1" {
				t.Fatal("linked attachment was ingested")
			}
		}
	}
}

func TestZoteroConnectorValidateRequiresWebDAVURL(t *testing.T) {
	connector, err := NewZoteroConnector(map[string]any{
		"zotero_user_id": "12345678",
		"storage_mode":   zoteroStorageModeWebDAV,
		"credentials": map[string]any{
			"zotero_api_key":  "test-key",
			"webdav_password": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewZoteroConnector: %v", err)
	}
	if err := connector.Validate(t.Context()); err == nil {
		t.Fatal("expected WebDAV URL validation error")
	}
}

func TestZoteroConnectorWebDAVExtractsPDF(t *testing.T) {
	utility.AllowAnyHostForTest = true
	t.Cleanup(func() { utility.AllowAnyHostForTest = false })
	server := newZoteroHTTPServer(t)
	connector, err := NewZoteroConnector(map[string]any{
		"zotero_user_id": "12345678",
		"storage_mode":   zoteroStorageModeWebDAV,
		"webdav_url":     server.URL + "/dav",
		"batch_size":     10,
		"credentials": map[string]any{
			"zotero_api_key":  "test-key",
			"webdav_password": "dav-pass",
		},
	})
	if err != nil {
		t.Fatalf("NewZoteroConnector: %v", err)
	}
	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	connector.httpClient = client
	connector.apiBase = server.URL
	start := mustTime(t, "2025-12-01T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: mustTime(t, "2026-02-01T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || string(batch.Documents[0].Blob) != "%PDF-1.4 zipped" {
		t.Fatalf("unexpected webdav batch: %+v blob=%q", batch.Documents, string(batch.Documents[0].Blob))
	}
}

func TestRegisterBuiltInsOpensZotero(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("zotero", map[string]any{
		"zotero_user_id": "12345678",
		"credentials": map[string]any{
			"zotero_api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig: %v", err)
	}
	if _, ok := connector.(*ZoteroConnector); !ok {
		t.Fatalf("got %T", connector)
	}
}

func newHTTPZoteroConnector(t *testing.T, server *httptest.Server, mode string, batchSize int) *ZoteroConnector {
	t.Helper()
	connector, err := NewZoteroConnector(map[string]any{
		"zotero_user_id": "12345678",
		"storage_mode":   mode,
		"batch_size":     batchSize,
		"credentials": map[string]any{
			"zotero_api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("NewZoteroConnector: %v", err)
	}
	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	connector.httpClient = client
	connector.apiBase = server.URL
	return connector
}

func newZoteroHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	items := []zoteroAPIItem{
		{
			Key: "ATTACH1",
			Data: zoteroItemData{
				ItemType:     "attachment",
				Title:        "Paper One",
				Filename:     "paper-one.pdf",
				ContentType:  "application/pdf",
				LinkMode:     "imported_file",
				DateModified: "2026-01-02T00:00:00Z",
				ParentItem:   "ITEM1",
			},
		},
		{
			Key: "ATTACH2",
			Data: zoteroItemData{
				ItemType:     "attachment",
				Title:        "Old Paper",
				Filename:     "old.pdf",
				ContentType:  "application/pdf",
				LinkMode:     "imported_file",
				DateModified: "2024-01-02T00:00:00Z",
				ParentItem:   "ITEM2",
			},
		},
		{
			Key: "LINKED1",
			Data: zoteroItemData{
				ItemType:     "attachment",
				Title:        "Linked PDF",
				Filename:     "linked.pdf",
				ContentType:  "application/pdf",
				LinkMode:     "linked_file",
				DateModified: "2026-01-02T00:00:00Z",
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users/12345678/items" && r.Method == http.MethodGet:
			w.Header().Set("Total-Results", "3")
			_ = json.NewEncoder(w).Encode(items)
		case r.URL.Path == "/users/12345678/items/ATTACH1/file":
			http.Redirect(w, r, "/files/paper-one.pdf", http.StatusFound)
		case r.URL.Path == "/files/paper-one.pdf":
			_, _ = w.Write([]byte("%PDF-1.4 test"))
		case r.URL.Path == "/users/12345678/items/ATTACH2/file":
			_, _ = w.Write([]byte("%PDF-1.4 old"))
		case r.URL.Path == "/dav" || r.URL.Path == "/dav/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/dav/ATTACH1.zip":
			user, pass, ok := r.BasicAuth()
			if !ok || user != "12345678" || pass != "dav-pass" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write(mustZoteroZip(t, "paper-one.pdf", []byte("%PDF-1.4 zipped")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func mustZoteroZip(t *testing.T, name string, payload []byte) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := file.Write(payload); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
