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

func TestNotionConnectorOpenSyncReadsRootPageChildrenAndAttachment(t *testing.T) {
	connector := newTestNotionConnector(t)
	connector.rootPageID = "page-1"
	connector.fetchPage = func(ctx context.Context, pageID string) (notionPage, error) {
		if pageID == "page-1" {
			return notionTestPage("page-1", "Root", "2026-01-03T00:00:00Z"), nil
		}
		if pageID == "child-1" {
			return notionTestPage("child-1", "Child", "2026-01-04T00:00:00Z"), nil
		}
		return notionPage{}, errors.New("unexpected page")
	}
	connector.fetchChildBlocks = func(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
		t.Helper()
		switch blockID {
		case "page-1":
			return notionBlockPage{Results: []notionBlock{
				notionTestTextBlock("block-1", "paragraph", "Hello"),
				notionTestFileBlock("file-1", "report.pdf", "https://files.example/report.pdf"),
				notionTestChildPageBlock("child-1"),
			}}, nil
		case "child-1":
			return notionBlockPage{Results: []notionBlock{notionTestTextBlock("block-2", "to_do", "Done", map[string]any{"checked": true})}}, nil
		default:
			return notionBlockPage{}, nil
		}
	}
	connector.downloadFile = func(ctx context.Context, rawURL string) ([]byte, error) {
		return []byte("pdf-body"), nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 3 {
		t.Fatalf("documents len = %d, want 3", len(batch.Documents))
	}
	if batch.Documents[0].SourceID != "page-1" || string(batch.Documents[0].Blob) != "\nHello\n\nFile: Root / report_file-1.pdf" {
		t.Fatalf("root doc = %+v body=%q", batch.Documents[0], batch.Documents[0].Blob)
	}
	if batch.Documents[1].SourceID != "file-1" || batch.Documents[1].Extension != ".pdf" || string(batch.Documents[1].Blob) != "pdf-body" {
		t.Fatalf("attachment doc = %+v body=%q", batch.Documents[1], batch.Documents[1].Blob)
	}
	if batch.Documents[2].SourceID != "child-1" || string(batch.Documents[2].Blob) != "\n[x] Done" {
		t.Fatalf("child doc = %+v body=%q", batch.Documents[2], batch.Documents[2].Blob)
	}
}

func TestNotionConnectorOpenSyncDefersTraversalUntilNextBatch(t *testing.T) {
	connector := newTestNotionConnector(t)
	connector.rootPageID = "page-1"
	var childBlockCalls int
	var downloadCalls int
	connector.fetchPage = func(ctx context.Context, pageID string) (notionPage, error) {
		return notionTestPage(pageID, "Root", "2026-01-03T00:00:00Z"), nil
	}
	connector.fetchChildBlocks = func(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
		childBlockCalls++
		return notionBlockPage{Results: []notionBlock{notionTestFileBlock("file-1", "report.pdf", "https://files.example/report.pdf")}}, nil
	}
	connector.downloadFile = func(ctx context.Context, rawURL string) ([]byte, error) {
		downloadCalls++
		return []byte("pdf-body"), nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if childBlockCalls != 0 || downloadCalls != 0 {
		t.Fatalf("OpenSync traversed content: childBlockCalls=%d downloadCalls=%d", childBlockCalls, downloadCalls)
	}
	if _, err := session.NextBatch(context.Background()); err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if childBlockCalls == 0 || downloadCalls == 0 {
		t.Fatalf("NextBatch did not traverse content: childBlockCalls=%d downloadCalls=%d", childBlockCalls, downloadCalls)
	}
}

func TestNotionConnectorOpenSyncUsesIncrementalWindowAndResume(t *testing.T) {
	connector := newTestNotionConnector(t)
	connector.searchPages = func(ctx context.Context, request notionSearchRequest) (notionSearchResponse, error) {
		return notionSearchResponse{Results: []notionPage{
			notionTestPage("old", "Old", "2026-01-01T00:00:00Z"),
			notionTestPage("new", "New", "2026-01-03T00:00:00Z"),
		}}, nil
	}
	connector.fetchChildBlocks = func(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
		return notionBlockPage{Results: []notionBlock{notionTestTextBlock(blockID+"-block", "paragraph", blockID+" body")}}, nil
	}
	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "new" {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Resume:      batch.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resume OpenSync: %v", err)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resumed NextBatch = %v, want EOF", err)
	}
}

func TestNotionConnectorResumeMissingCheckpointFails(t *testing.T) {
	connector := newTestNotionConnector(t)
	connector.searchPages = func(ctx context.Context, request notionSearchRequest) (notionSearchResponse, error) {
		return notionSearchResponse{Results: []notionPage{notionTestPage("a", "A", "2026-01-03T00:00:00Z")}}, nil
	}
	connector.fetchChildBlocks = func(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
		return notionBlockPage{Results: []notionBlock{notionTestTextBlock(blockID+"-block", "paragraph", blockID+" body")}}, nil
	}
	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Resume:      &SyncCheckpoint{SourceID: "missing"},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if _, err = session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestNotionConnectorIncrementalSearchContinuesPastTooNewPages(t *testing.T) {
	connector := newTestNotionConnector(t)
	connector.searchPages = func(ctx context.Context, request notionSearchRequest) (notionSearchResponse, error) {
		if request.PageSize == 1 {
			return notionSearchResponse{}, nil
		}
		switch request.StartCursor {
		case "":
			return notionSearchResponse{
				Results:    []notionPage{notionTestPage("future", "Future", "2026-01-05T00:00:00Z")},
				NextCursor: "cursor-2",
				HasMore:    true,
			}, nil
		case "cursor-2":
			return notionSearchResponse{Results: []notionPage{notionTestPage("in-window", "Current", "2026-01-03T00:00:00Z")}}, nil
		default:
			return notionSearchResponse{}, nil
		}
	}
	connector.fetchChildBlocks = func(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
		return notionBlockPage{Results: []notionBlock{notionTestTextBlock(blockID+"-block", "paragraph", blockID+" body")}}, nil
	}
	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "in-window" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestNotionConnectorFileRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "12345")
	}))
	defer server.Close()

	connector := newTestNotionConnector(t)
	connector.fileMaxBytes = 4
	_, err := connector.file(t.Context(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("file err = %v, want maximum size error", err)
	}
}

func TestNotionConnectorOpenPruneReturnsPageAndAttachmentIDs(t *testing.T) {
	connector := newTestNotionConnector(t)
	connector.rootPageID = "page-1"
	connector.fetchPage = func(ctx context.Context, pageID string) (notionPage, error) {
		return notionTestPage(pageID, pageID, "2026-01-03T00:00:00Z"), nil
	}
	connector.fetchChildBlocks = func(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
		if blockID == "page-1" {
			return notionBlockPage{Results: []notionBlock{notionTestFileBlock("file-1", "a.txt", "https://files.example/a.txt"), notionTestChildPageBlock("child-1")}}, nil
		}
		return notionBlockPage{}, nil
	}
	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 3 {
		t.Fatalf("documents len = %d, want 3", len(batch.Documents))
	}
	got := []string{batch.Documents[0].SourceID, batch.Documents[1].SourceID, batch.Documents[2].SourceID}
	want := []string{"page-1", "file-1", "child-1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

func newTestNotionConnector(t *testing.T) *NotionConnector {
	t.Helper()
	connector, err := NewNotionConnector(map[string]any{
		"batch_size": 10,
		"credentials": map[string]any{
			"notion_integration_token": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewNotionConnector: %v", err)
	}
	return connector
}

func notionTestPage(id, title, updatedAt string) notionPage {
	return notionPage{
		ID:             id,
		Object:         "page",
		LastEditedTime: updatedAt,
		URL:            "https://notion.so/" + id,
		Properties: map[string]any{
			"Name": map[string]any{"type": "title", "title": []any{map[string]any{"plain_text": title}}},
		},
	}
}

func notionTestTextBlock(id, blockType, text string, extra ...map[string]any) notionBlock {
	object := map[string]any{"rich_text": []any{map[string]any{"plain_text": text}}}
	for _, fields := range extra {
		for key, value := range fields {
			object[key] = value
		}
	}
	return notionBlock{ID: id, Type: blockType, Raw: map[string]any{blockType: object}}
}

func notionTestFileBlock(id, name, rawURL string) notionBlock {
	return notionBlock{
		ID:   id,
		Type: "file",
		Raw: map[string]any{"file": map[string]any{
			"type": "file",
			"name": name,
			"file": map[string]any{"url": rawURL},
		}},
	}
}

func notionTestChildPageBlock(id string) notionBlock {
	return notionBlock{ID: id, Type: "child_page", HasChildren: true, Raw: map[string]any{"child_page": map[string]any{}}}
}
