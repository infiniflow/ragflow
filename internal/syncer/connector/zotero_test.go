package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestZoteroConnectorOpenSyncDownloadsPDF(t *testing.T) {
	server := newZoteroTestServer(t)
	connector, err := NewZoteroConnector(map[string]any{
		"zotero_user_id": "12345678",
		"storage_mode":   zoteroStorageModeZotero,
		"batch_size":     1,
		"credentials": map[string]any{
			"zotero_api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("NewZoteroConnector: %v", err)
	}
	connector.httpClient = server.Client()
	downloads := 0
	connector.listItems = func(ctx context.Context, start int) ([]zoteroAPIItem, int, error) {
		return server.listItems(start)
	}
	connector.downloadPDF = func(ctx context.Context, attachment zoteroAPIItem) ([]byte, string, error) {
		downloads++
		return server.downloadPDF(attachment.Key)
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-02-01T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if downloads != 0 {
		t.Fatalf("OpenSync downloaded %d attachments before NextBatch", downloads)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(first.Documents) != 1 {
		t.Fatalf("batch len = %d", len(first.Documents))
	}
	doc := first.Documents[0]
	if doc.SourceID != "zotero:12345678:ATTACH1" {
		t.Fatalf("source id = %s", doc.SourceID)
	}
	if string(doc.Blob) != "%PDF-1.4 test" {
		t.Fatalf("blob = %q", string(doc.Blob))
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestZoteroParseTime(t *testing.T) {
	parsed, err := zoteroParseTime("2026-01-02T12:34:56Z")
	if err != nil {
		t.Fatalf("zoteroParseTime: %v", err)
	}
	if parsed.UTC().Format(time.RFC3339) != "2026-01-02T12:34:56Z" {
		t.Fatalf("parsed = %s", parsed.UTC().Format(time.RFC3339))
	}
}

func TestZoteroNextBatchRetriesFailedDownloadAfterPartialBatch(t *testing.T) {
	connector, err := NewZoteroConnector(map[string]any{
		"zotero_user_id": "12345678",
		"storage_mode":   zoteroStorageModeZotero,
		"batch_size":     10,
		"credentials": map[string]any{
			"zotero_api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("NewZoteroConnector: %v", err)
	}
	attempts := map[string]int{}
	connector.downloadPDF = func(ctx context.Context, attachment zoteroAPIItem) ([]byte, string, error) {
		attempts[attachment.Key]++
		switch attachment.Key {
		case "ATTACH1":
			return []byte("%PDF"), "one.pdf", nil
		case "ATTACH2":
			return nil, "", errors.New("transient failure")
		default:
			return nil, "", errors.New("unexpected attachment")
		}
	}
	session := &zoteroSyncSession{
		connector: connector,
		records: []zoteroPDFRecord{
			{attachment: zoteroAPIItem{Key: "ATTACH1"}, title: "One"},
			{attachment: zoteroAPIItem{Key: "ATTACH2"}, title: "Two"},
		},
		batchSize: 10,
	}

	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "zotero:12345678:ATTACH1" {
		t.Fatalf("unexpected first batch: %+v", first.Documents)
	}
	if session.index != 1 {
		t.Fatalf("index = %d, want 1 to retry failed attachment", session.index)
	}

	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("second NextBatch: %v", err)
	}
	if attempts["ATTACH2"] < 2 {
		t.Fatalf("ATTACH2 attempts = %d, want at least 2", attempts["ATTACH2"])
	}
}

func TestZoteroConnectorOpenSyncIncrementalWindow(t *testing.T) {
	server := newZoteroTestServer(t)
	connector, err := NewZoteroConnector(map[string]any{
		"zotero_user_id": "12345678",
		"batch_size":     10,
		"credentials": map[string]any{
			"zotero_api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("NewZoteroConnector: %v", err)
	}
	connector.httpClient = server.Client()
	connector.listItems = func(ctx context.Context, start int) ([]zoteroAPIItem, int, error) {
		return server.listItems(start)
	}
	connector.downloadPDF = func(ctx context.Context, attachment zoteroAPIItem) ([]byte, string, error) {
		return server.downloadPDF(attachment.Key)
	}

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

type zoteroTestServer struct {
	*httptest.Server
	items []zoteroAPIItem
}

func newZoteroTestServer(t *testing.T) *zoteroTestServer {
	state := &zoteroTestServer{
		items: []zoteroAPIItem{
			{
				Key: "ATTACH1",
				Data: zoteroItemData{
					ItemType:     "attachment",
					Title:        "Paper One",
					Filename:     "paper-one.pdf",
					ContentType:  "application/pdf",
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
					DateModified: "2024-01-02T00:00:00Z",
					ParentItem:   "ITEM2",
				},
			},
		},
	}
	state.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users/12345678/items" && r.Method == http.MethodGet:
			w.Header().Set("Total-Results", "2")
			_ = json.NewEncoder(w).Encode(state.items)
		case r.URL.Path == "/users/12345678/items/ATTACH1/file":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("%PDF-1.4 test"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(state.Close)
	return state
}

func (s *zoteroTestServer) listItems(start int) ([]zoteroAPIItem, int, error) {
	if start >= len(s.items) {
		return nil, len(s.items), nil
	}
	return s.items[start:], len(s.items), nil
}

func (s *zoteroTestServer) downloadPDF(key string) ([]byte, string, error) {
	if key == "ATTACH1" {
		return []byte("%PDF-1.4 test"), "paper-one.pdf", nil
	}
	return nil, "", errors.New("missing attachment")
}
