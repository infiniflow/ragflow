package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/mcp"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/service/document"
)

type mcpDocumentService interface {
	ListDocumentsByDatasetIDWithOptions(context.Context, dao.DocumentListOptions, int, int) ([]*entity.DocumentListItem, int64, error)
	GetDocumentMetadataByID(context.Context, string) (map[string]interface{}, error)
	GetDocumentByID(context.Context, string) (*document.DocumentResponse, error)
}

type mcpChunkService interface {
	List(context.Context, *service.ListChunksRequest) (*service.ListChunksResponse, error)
}

func mcpDocumentPage(page, size int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = 30
	}
	if page < 1 || size < 1 || size > 100 {
		return 0, 0, fmt.Errorf("page must be positive and page_size must be between 1 and 100")
	}
	if page-1 > int(^uint(0)>>1)/size {
		return 0, 0, fmt.Errorf("pagination offset is too large")
	}
	return page, size, nil
}

func mcpListDocuments(ctx context.Context, documents mcpDocumentService, req mcp.ListDocumentsRequest) (string, error) {
	if permission.UserIDFromContext(ctx) == "" {
		return "", permission.ErrUnauthenticated
	}
	if strings.TrimSpace(req.DatasetID) == "" {
		return "", fmt.Errorf("dataset_id is required")
	}
	page, size, err := mcpDocumentPage(req.Page, req.PageSize)
	if err != nil {
		return "", err
	}
	scope, err := documentReadAccessScope(ctx, req.DatasetID)
	if err != nil {
		return "", err
	}
	rows, total, err := documents.ListDocumentsByDatasetIDWithOptions(ctx, dao.DocumentListOptions{
		KbID: req.DatasetID, Keywords: req.Keywords, Name: req.Name,
		OrderBy: "create_time", Desc: true, AccessScope: &scope,
	}, page, size)
	if err != nil {
		return "", err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		metadata, err := documents.GetDocumentMetadataByID(ctx, row.ID)
		if err != nil {
			return "", err
		}
		if metadata == nil {
			metadata = map[string]any{}
		}
		mapped := mapDocumentListItem(row, metadata, nil)
		item := make(map[string]any)
		for _, key := range []string{"id", "name", "dataset_id", "type", "size", "chunk_count", "token_count", "ingestion_status", "create_date", "update_date", "meta_fields"} {
			item[key] = mapped[key]
		}
		items = append(items, item)
	}
	data, err := json.Marshal(map[string]any{"documents": items, "total": total, "page": page, "page_size": size})
	return string(data), err
}

func mcpListChunks(ctx context.Context, documents mcpDocumentService, chunks mcpChunkService, req mcp.ListChunksRequest) (string, error) {
	if permission.UserIDFromContext(ctx) == "" {
		return "", permission.ErrUnauthenticated
	}
	if strings.TrimSpace(req.DatasetID) == "" || strings.TrimSpace(req.DocumentID) == "" {
		return "", fmt.Errorf("dataset_id and document_id are required")
	}
	page, size, err := mcpDocumentPage(req.Page, req.PageSize)
	if err != nil {
		return "", err
	}
	if err := service.CheckDocumentAccess(ctx, req.DocumentID, req.DatasetID, permission.OperationRead); err != nil {
		return "", err
	}
	doc, err := documents.GetDocumentByID(ctx, req.DocumentID)
	if err != nil {
		return "", err
	}
	if doc.KbID != req.DatasetID {
		return "", permission.ErrResourceNotFound
	}
	if doc.IngestionStatus != common.COMPLETED {
		return "", fmt.Errorf("document is not fully parsed (ingestion_status: %s); wait for parsing to complete", doc.IngestionStatus)
	}
	// With no keywords, the existing chunk service sorts by document position
	// and applies document and dataset permission checks before searching.
	resp, err := chunks.List(ctx, &service.ListChunksRequest{
		DatasetID: req.DatasetID, DocID: req.DocumentID, Page: &page, Size: &size,
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(map[string]any{"chunks": resp.Chunks, "doc": resp.Doc, "total": resp.Total, "page": page, "page_size": size})
	return string(data), err
}
