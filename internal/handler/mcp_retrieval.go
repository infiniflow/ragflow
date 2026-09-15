package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/mcp"
	"ragflow/internal/service"
	dataset "ragflow/internal/service/dataset"
	"ragflow/internal/service/document"
)

// mcpRerankCandidatesCount is the fixed rerank candidate window sent with every
// retrieval request, so the ranking cannot shift between pages of one
// pagination sequence. Requests whose page * page_size exceeds it are rejected
// up front. Keep in sync with _RERANK_CANDIDATES_COUNT in mcp/server/server.py.
const mcpRerankCandidatesCount = 512

// validateRetrievalWindow checks that the requested page fits inside the fixed
// rerank candidate window. page/page_size default to the same values as the
// Python MCP server (1/30) when unset. The comparison divides instead of
// multiplying so a hostile page value cannot overflow page * pageSize.
func validateRetrievalWindow(page, pageSize int) error {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}
	if page > mcpRerankCandidatesCount/pageSize {
		return fmt.Errorf("page (%d) * page_size (%d) exceeds the fixed rerank candidate window (%d); narrow page or page_size", page, pageSize, mcpRerankCandidatesCount)
	}
	return nil
}

// MCPRetrieval executes a retrieval request on behalf of the MCP tool handler.
// It translates the mcp.RetrievalRequest into a service.SearchDatasetsRequest
// and calls DatasetService.SearchDatasets. The result is serialized as JSON.
func MCPRetrieval(ctx context.Context, ds *dataset.DatasetService, userID string, req mcp.RetrievalRequest) (string, error) {
	if err := validateRetrievalWindow(req.Page, req.PageSize); err != nil {
		return "", err
	}
	// Resolve dataset IDs: if none provided, fetch ALL accessible datasets
	// across all pages (matching Python _fetch_all_datasets behaviour).
	datasetIDs := req.DatasetIDs
	if len(datasetIDs) == 0 {
		const maxPageSize = 100
		ids, err := fetchAllDatasetIDs(func(page, pageSize int) ([]map[string]interface{}, int64, error) {
			data, total, _, err := ds.ListDatasets(ctx,
				"", "", page, pageSize, "create_time", true,
				"", nil, "", userID, nil,
			)
			return data, total, err
		}, maxPageSize)
		if err != nil {
			return "", fmt.Errorf("cannot resolve accessible datasets: %w", err)
		}
		if len(ids) == 0 {
			return "", fmt.Errorf("No accessible datasets found.")
		}
		datasetIDs = ids
	}

	searchReq := &service.SearchDatasetsRequest{
		DatasetIDs:   datasetIDs,
		Question:     req.Question,
		DocumentIDs:  req.DocumentIDs,
		ForceRefresh: req.ForceRefresh,
	}

	if req.Page > 0 {
		v := req.Page
		searchReq.Page = &v
	}
	if req.PageSize > 0 {
		v := req.PageSize
		searchReq.PageSize = &v
	}
	if req.TopK > 0 {
		v := req.TopK
		searchReq.TopK = &v
	}
	{
		v := req.SimilarityThreshold
		searchReq.SimilarityThreshold = &v
	}
	{
		v := req.VectorSimilarityWeight
		searchReq.VectorSimilarityWeight = &v
	}
	if req.RerankID != "" {
		v := req.RerankID
		searchReq.RerankID = &v
	}
	{
		v := mcpRerankCandidatesCount
		searchReq.RerankCandidatesCount = &v
	}
	{
		v := req.Keyword
		searchReq.Keyword = &v
	}

	resp, err := ds.SearchDatasets(ctx, searchReq, userID)
	if err != nil {
		return "", err
	}

	// Metadata reads use the same authorized service paths as the REST API.
	// No shared cache: force_refresh always sees current metadata, without
	// cross-tenant cache state or a second metadata implementation.
	documents := document.NewDocumentService()
	datasetNames := make(map[string]string)
	documentMetadata := make(map[string]map[string]any)
	for _, id := range datasetIDs {
		info, code, err := ds.GetDataset(ctx, id, userID)
		if err != nil || code != common.CodeSuccess {
			continue
		}
		name, _ := info["name"].(string)
		datasetNames[id] = name
		if !ds.Accessible(ctx, id, userID) {
			continue
		}
		// Read only documents returned by retrieval, through the existing listing
		// and metadata services. This avoids loading entire document collections.
		for _, chunk := range resp.Chunks {
			chunkDataset, _ := chunk["dataset_id"].(string)
			docID, _ := chunk["document_id"].(string)
			if chunkDataset != id || docID == "" {
				continue
			}
			if _, ok := documentMetadata[docID]; ok {
				continue
			}
			docs, _, err := documents.ListDocumentsByDatasetIDWithOptions(ctx, dao.DocumentListOptions{KbID: id, DocIDs: []string{docID}}, 1, 1)
			if err != nil || len(docs) == 0 {
				continue
			}
			fields, err := documents.GetDocumentMetadataByID(ctx, docID)
			if err != nil {
				fields = map[string]any{}
			}
			doc := mapDocumentListItem(docs[0], fields)
			metadata := map[string]any{"document_id": docID}
			for _, key := range []string{"name", "location", "type", "size", "chunk_count", "create_date", "update_date", "token_count", "thumbnail", "dataset_id", "meta_fields"} {
				metadata[key] = doc[key]
			}
			documentMetadata[docID] = metadata
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result, err := json.Marshal(mcpRetrievalResult(req, datasetIDs, resp, datasetNames, documentMetadata))
	if err != nil {
		return "", fmt.Errorf("failed to serialize retrieval result: %w", err)
	}
	return string(result), nil
}

// fetchAllDatasetIDs pages through listPage collecting dataset IDs until the
// total reported by the service is reached, or a short or empty page arrives.
// Stopping at the reported total avoids an extra empty-page request when the
// dataset count is an exact multiple of pageSize.
func fetchAllDatasetIDs(listPage func(page, pageSize int) ([]map[string]interface{}, int64, error), pageSize int) ([]string, error) {
	var ids []string
	seen := make(map[string]bool)
	page := 1
	fetched := 0
	for {
		data, total, err := listPage(page, pageSize)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			break
		}
		fetched += len(data)
		for _, d := range data {
			if id, ok := d["id"].(string); ok && id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		// Stop once the reported total is reached so exact multiples of
		// pageSize do not pay an extra empty-page request.
		if total > 0 && int64(fetched) >= total {
			break
		}
		// A page smaller than pageSize is the last page.
		if len(data) < pageSize {
			break
		}
		page++
	}
	return ids, nil
}

// mcpRetrievalResult preserves REST chunk fields while adding the MCP envelope.
func mcpRetrievalResult(req mcp.RetrievalRequest, datasetIDs []string, resp *service.SearchDatasetsResponse, names map[string]string, metadata map[string]map[string]any) map[string]any {
	chunks := make([]map[string]any, 0, len(resp.Chunks))
	for _, raw := range resp.Chunks {
		chunk := maps.Clone(raw)
		id, _ := chunk["dataset_id"].(string)
		if id == "" {
			id, _ = chunk["kb_id"].(string)
		}
		name, ok := names[id]
		if !ok {
			name = "Unknown"
		}
		chunk["dataset_name"] = name
		documentName, ok := chunk["document_keyword"]
		if !ok {
			documentName = ""
		}
		chunk["document_name"] = documentName
		docID, _ := chunk["document_id"].(string)
		if doc, ok := metadata[docID]; ok {
			chunk["document_metadata"] = doc
		}
		chunks = append(chunks, chunk)
	}
	page, size := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 30
	}
	return map[string]any{
		"chunks":     chunks,
		"pagination": map[string]any{"page": page, "page_size": size, "total_chunks": resp.Total, "total_pages": (resp.Total + int64(size) - 1) / int64(size)},
		"query_info": map[string]any{"question": req.Question, "similarity_threshold": req.SimilarityThreshold, "vector_weight": req.VectorSimilarityWeight, "keyword_search": req.Keyword, "dataset_count": len(datasetIDs)},
	}
}
