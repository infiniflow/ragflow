package handler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/mcp"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/service/document"
)

type mcpDocumentStub struct {
	list     func(context.Context, dao.DocumentListOptions, int, int) ([]*entity.DocumentListItem, int64, error)
	metadata func(context.Context, string) (map[string]interface{}, error)
}

func (s mcpDocumentStub) ListDocumentsByDatasetIDWithOptions(ctx context.Context, opts dao.DocumentListOptions, page, size int) ([]*entity.DocumentListItem, int64, error) {
	return s.list(ctx, opts, page, size)
}

func (s mcpDocumentStub) GetDocumentMetadataByID(ctx context.Context, id string) (map[string]interface{}, error) {
	return s.metadata(ctx, id)
}

type mcpChunkStub func(context.Context, *service.ListChunksRequest) (*service.ListChunksResponse, error)

func (s mcpChunkStub) List(ctx context.Context, req *service.ListChunksRequest) (*service.ListChunksResponse, error) {
	return s(ctx, req)
}

func TestMCPDocumentToolsRequireIdentityAndPermission(t *testing.T) {
	setupDocumentPermissionDB(t, false)
	for _, ctx := range []context.Context{t.Context(), permission.WithIdentity(t.Context(), "user-1", "tenant-owner")} {
		_, err := mcpListDocuments(ctx, nil, mcp.ListDocumentsRequest{DatasetID: "kb-owner"})
		if !errors.Is(err, permission.ErrUnauthenticated) && !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatalf("documents permission error = %v", err)
		}
		_, err = mcpListChunks(ctx, nil, nil, mcp.ListChunksRequest{DatasetID: "kb-owner", DocumentID: "doc-1"})
		if !errors.Is(err, permission.ErrUnauthenticated) && !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatalf("chunks permission error = %v", err)
		}
	}
}

func TestMCPListDocumentsMetadataAndPagination(t *testing.T) {
	setupDocumentPermissionDB(t, true)
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-owner")
	documents := mcpDocumentStub{
		list: func(ctx context.Context, opts dao.DocumentListOptions, page, size int) ([]*entity.DocumentListItem, int64, error) {
			if opts.KbID != "kb-owner" || opts.Keywords != "contract" || opts.Name != "contract.pdf" || opts.OrderBy != "create_time" || !opts.Desc || page != 2 || size != 1 {
				t.Fatalf("listing options = %+v, page=%d size=%d", opts, page, size)
			}
			if opts.AccessScope == nil {
				t.Fatal("missing document access scope")
			}
			var ids []string
			if err := opts.AccessScope.Apply(dao.DB.WithContext(ctx).Table("document")).Pluck("id", &ids).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids, []string{"doc-1"}) {
				t.Fatalf("visible document IDs = %v", ids)
			}
			return []*entity.DocumentListItem{{ID: "doc-1", KbID: "kb-owner", Name: sptr("contract.pdf"), Type: "pdf", ChunkNum: 10, TokenNum: 500, IngestionStatus: sptr(common.COMPLETED)}}, 2, nil
		},
		metadata: func(_ context.Context, id string) (map[string]interface{}, error) {
			if id != "doc-1" {
				t.Fatalf("metadata ID = %s", id)
			}
			return map[string]interface{}{"year": 2026}, nil
		},
	}
	text, err := mcpListDocuments(ctx, documents, mcp.ListDocumentsRequest{DatasetID: "kb-owner", Keywords: "contract", Name: "contract.pdf", Page: 2, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Documents []map[string]any `json:"documents"`
		Total     int              `json:"total"`
		Page      int              `json:"page"`
		PageSize  int              `json:"page_size"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Page != 2 || result.PageSize != 1 || len(result.Documents) != 1 || result.Documents[0]["name"] != "contract.pdf" || result.Documents[0]["ingestion_status"] != common.COMPLETED || result.Documents[0]["meta_fields"].(map[string]any)["year"] != float64(2026) {
		t.Fatalf("documents result = %s", text)
	}
	documents.list = func(context.Context, dao.DocumentListOptions, int, int) ([]*entity.DocumentListItem, int64, error) {
		return nil, 0, nil
	}
	text, err = mcpListDocuments(ctx, documents, mcp.ListDocumentsRequest{DatasetID: "kb-owner"})
	if err != nil || !strings.Contains(text, `"documents":[]`) {
		t.Fatalf("empty listing = %s, %v", text, err)
	}
}

func TestMCPListChunksParseStatusAndPaging(t *testing.T) {
	setupDocumentPermissionDB(t, true)
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-owner")
	status := common.UNSTART
	documents := mcpDocumentStub{list: func(_ context.Context, opts dao.DocumentListOptions, page, size int) ([]*entity.DocumentListItem, int64, error) {
		if opts.KbID != "kb-owner" || !reflect.DeepEqual(opts.DocIDs, []string{"doc-1"}) || !opts.DocIDFilterApplied || opts.AccessScope == nil || page != 1 || size != 1 {
			t.Fatalf("document selection = %+v, page=%d size=%d", opts, page, size)
		}
		return []*entity.DocumentListItem{{ID: "doc-1", KbID: "kb-owner", IngestionStatus: &status}}, 1, nil
	}}
	for _, status = range []string{common.UNSTART, common.RUNNING, common.FAILED, common.STOPPED} {
		_, err := mcpListChunks(ctx, documents, nil, mcp.ListChunksRequest{DatasetID: "kb-owner", DocumentID: "doc-1"})
		if err == nil || !strings.Contains(err.Error(), "not fully parsed") || !strings.Contains(err.Error(), status) {
			t.Fatalf("parse status %s: %v", status, err)
		}
	}
	status = common.COMPLETED
	chunks := mcpChunkStub(func(_ context.Context, req *service.ListChunksRequest) (*service.ListChunksResponse, error) {
		// Page 20 reaches past retrieval's 512-candidate window. No keyword,
		// relevance or availability filter may discard parts of the document.
		if req.DatasetID != "kb-owner" || req.DocID != "doc-1" || *req.Page != 20 || *req.Size != 30 || req.Keywords != "" || req.AvailableInt != nil {
			t.Fatalf("chunk request = %+v", req)
		}
		return &service.ListChunksResponse{Total: 700, Doc: map[string]any{"name": "contract.pdf", "chunk_count": 700}, Chunks: []map[string]any{
			{"id": "chunk-571", "content": "text", "positions": [][]int{{1, 2, 3, 4, 5}}, "important_keywords": []string{"contract"}},
		}}, nil
	})
	text, err := mcpListChunks(ctx, documents, chunks, mcp.ListChunksRequest{DatasetID: "kb-owner", DocumentID: "doc-1", Page: 20, PageSize: 30})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"total":700`, `"page":20`, `"page_size":30`, `"important_keywords":["contract"]`, `"positions":[[1,2,3,4,5]]`, `"content":"text"`, `"name":"contract.pdf"`} {
		if !strings.Contains(text, field) {
			t.Fatalf("missing %s in %s", field, text)
		}
	}
	_, err = mcpListChunks(ctx, nil, nil, mcp.ListChunksRequest{DatasetID: "another-dataset", DocumentID: "doc-1"})
	if err == nil {
		t.Fatal("cross-dataset document accepted")
	}
}

func TestMCPDocumentToolsValidateArguments(t *testing.T) {
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-owner")
	for _, req := range []mcp.ListDocumentsRequest{{}, {DatasetID: " "}, {DatasetID: "kb", Page: -1}, {DatasetID: "kb", PageSize: 101}} {
		if _, err := mcpListDocuments(ctx, nil, req); err == nil {
			t.Fatalf("documents accepted %+v", req)
		}
	}
	for _, req := range []mcp.ListChunksRequest{{}, {DatasetID: "kb"}, {DocumentID: "doc"}, {DatasetID: "kb", DocumentID: "doc", PageSize: 101}} {
		if _, err := mcpListChunks(ctx, nil, nil, req); err == nil {
			t.Fatalf("chunks accepted %+v", req)
		}
	}
	if page, size, err := mcpDocumentPage(0, 0); err != nil || page != 1 || size != 30 {
		t.Fatalf("defaults = %d, %d, %v", page, size, err)
	}
	if _, _, err := mcpDocumentPage(int(^uint(0)>>1), 100); err == nil {
		t.Fatal("overflowing pagination offset accepted")
	}
}

func TestMCPListChunksUsesDocumentListStatus(t *testing.T) {
	for _, tt := range []struct {
		name, run, taskStatus, wantStatus string
		withTask                          bool
	}{
		{name: "completed without task", run: "3", wantStatus: common.COMPLETED},
		{name: "unstarted without task", run: "0", wantStatus: common.UNSTART},
		{name: "failed without task", run: "4", wantStatus: common.FAILED},
		{name: "new task running", run: "3", withTask: true, taskStatus: common.RUNNING, wantStatus: common.RUNNING},
		{name: "new task failed", run: "3", withTask: true, taskStatus: common.FAILED, wantStatus: common.FAILED},
		{name: "empty task status", run: "3", withTask: true, wantStatus: common.UNSTART},
		{name: "new task completed", run: "0", withTask: true, taskStatus: common.COMPLETED, wantStatus: common.COMPLETED},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupDocumentPermissionDB(t, true)
			if err := dao.DB.AutoMigrate(&entity.File{}, &entity.File2Document{}, &entity.UserCanvas{}, &entity.IngestionTask{}); err != nil {
				t.Fatal(err)
			}
			// Keep the old column out of the Go model, as in the real list query.
			if err := dao.DB.Exec("ALTER TABLE document ADD COLUMN run TEXT").Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Table("document").Where("id = ?", "doc-1").UpdateColumn("run", tt.run).Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Create(&entity.File{ID: "file-1", ParentID: "root", TenantID: "tenant-owner", CreatedBy: "owner-user", Name: "contract.pdf", Type: "pdf"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Create(&entity.File2Document{ID: "link-1", FileID: sptr("file-1"), DocumentID: sptr("doc-1")}).Error; err != nil {
				t.Fatal(err)
			}
			if tt.withTask {
				if err := dao.DB.Create(&entity.IngestionTask{ID: "task-1", UserID: "owner-user", DocumentID: "doc-1", DatasetID: "kb-owner", Status: tt.taskStatus}).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-owner")
			documents := document.NewDocumentService()
			rows, total, err := documents.ListDocumentsByDatasetIDWithOptions(ctx, dao.DocumentListOptions{KbID: "kb-owner"}, 1, 1)
			if err != nil || total != 1 || len(rows) != 1 || stringValue(rows[0].IngestionStatus) != tt.wantStatus {
				t.Fatalf("document list: rows=%+v total=%d err=%v, want %s", rows, total, err, tt.wantStatus)
			}
			called := false
			chunks := mcpChunkStub(func(context.Context, *service.ListChunksRequest) (*service.ListChunksResponse, error) {
				called = true
				return &service.ListChunksResponse{Chunks: []map[string]any{}, Doc: map[string]any{"id": "doc-1"}}, nil
			})
			_, err = mcpListChunks(ctx, documents, chunks, mcp.ListChunksRequest{DatasetID: "kb-owner", DocumentID: "doc-1"})
			if tt.wantStatus == common.COMPLETED {
				if err != nil || !called {
					t.Fatalf("completed document rejected: called=%t err=%v", called, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "ingestion_status: "+tt.wantStatus) || called {
				t.Fatalf("parse guard: called=%t err=%v, want %s", called, err, tt.wantStatus)
			}
			if tt.name == "completed without task" {
				if err := dao.DB.Create(&entity.Knowledgebase{ID: "kb-other", TenantID: "tenant-owner", Name: "other", CreatedBy: "owner-user", Permission: string(entity.TenantPermissionTeam), Status: sptr(string(entity.StatusValid))}).Error; err != nil {
					t.Fatal(err)
				}
				for _, req := range []mcp.ListChunksRequest{
					{DatasetID: "kb-owner", DocumentID: "missing"},
					{DatasetID: "kb-other", DocumentID: "doc-1"},
				} {
					called = false
					if _, err := mcpListChunks(ctx, documents, chunks, req); !errors.Is(err, permission.ErrResourceNotFound) || called {
						t.Fatalf("unlisted document: request=%+v called=%t err=%v", req, called, err)
					}
				}
			}
		})
	}
}
