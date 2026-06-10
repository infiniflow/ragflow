//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"strings"


	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

// ChunkService chunk service
type ChunkService struct {
	docEngine      engine.DocEngine
	engineType     server.EngineType
	embeddingCache *utility.EmbeddingLRU
	kbDAO          *dao.KnowledgebaseDAO
	userTenantDAO  *dao.UserTenantDAO
	documentDAO    *dao.DocumentDAO
	searchService  *SearchService
}


// RetrievalTestRequest retrieval test request
type RetrievalTestRequest struct {
	Datasets               common.StringSlice      `json:"dataset_ids" binding:"required"` // string or []string
	Question               string                 `json:"question"`
	Page                   *int                   `json:"page,omitempty"`
	Size                   *int                   `json:"size,omitempty"`
	DocIDs                 []string               `json:"doc_ids,omitempty"`
	UseKG                  *bool                  `json:"use_kg,omitempty"`
	TopK                   *int                   `json:"top_k,omitempty"`
	CrossLanguages         []string               `json:"cross_languages,omitempty"`
	SearchID               *string                `json:"search_id,omitempty"`
	Filter                 map[string]interface{} `json:"meta_data_filter,omitempty"`
	TenantRerankID         *string                `json:"tenant_rerank_id,omitempty"`
	RerankID               *string                `json:"rerank_id,omitempty"`
	Keyword                *bool                  `json:"keyword,omitempty"`
	SimilarityThreshold    *float64               `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight,omitempty"`
}

// RetrievalTestResponse retrieval test response
type RetrievalTestResponse struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs"`
	Labels  *map[string]float64      `json:"labels"`
	Total   int64                    `json:"total"`
}

// GetChunkRequest request for getting a chunk by ID
type GetChunkRequest struct {
	ChunkID string `json:"chunk_id"`
}

// GetChunkResponse response for getting a chunk
type GetChunkResponse struct {
	Chunk map[string]interface{} `json:"chunk"`
}

// Get retrieves a chunk by ID
func (s *ChunkService) Get(req *GetChunkRequest, userID string) (*GetChunkResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	if req.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}

	// Try each tenant to find the chunk
	var chunk map[string]interface{}
	for _, tenant := range tenants {
		// Get kbIDs for this tenant
		kbIDs, err := s.kbDAO.GetKBIDsByTenantID(tenant.TenantID)
		if err != nil {
			continue
		}

		indexName := fmt.Sprintf("ragflow_%s", tenant.TenantID)

		doc, err := s.docEngine.GetChunk(ctx, indexName, req.ChunkID, kbIDs)
		if err != nil {
			continue
		}

		if doc != nil {
			chunk, ok := doc.(map[string]interface{})
			if ok {
				result := make(map[string]interface{})
				skipFields := map[string]bool{
					"id": true, "authors": true, "_score": true, "SCORE": true,
				}
				for k, v := range chunk {
					if skipFields[k] || isInternalField(k) {
						continue
					}
					if applyCommonChunkMapping(result, k, v) {
						continue
					}
					switch k {
					case "tag_feas":
						if utility.IsEmpty(v) {
							result[k] = map[string]interface{}{}
						} else {
							result[k] = v
						}
					case "create_timestamp_flt", "rank_flt", "weight_flt":
						if floatVal, ok := utility.ToFloat64(v); ok {
							result[k] = utility.JSONFloat64(floatVal)
						}
					default:
						result[k] = v
					}
				}
				return &GetChunkResponse{Chunk: result}, nil
			}
		}
	}

	if chunk == nil {
		return nil, fmt.Errorf("chunk not found")
	}

	return &GetChunkResponse{Chunk: chunk}, nil
}

// ListChunksRequest request for listing chunks
type ListChunksRequest struct {
	DocID        string `json:"doc_id" binding:"required"`
	Page         *int   `json:"page,omitempty"`
	Size         *int   `json:"size,omitempty"`
	Keywords     string `json:"keywords,omitempty"`
	AvailableInt *int   `json:"available_int,omitempty"`
}

// ListChunksResponse response for listing chunks
type ListChunksResponse struct {
	Chunks []map[string]interface{} `json:"chunks"`
	Doc    map[string]interface{}   `json:"doc"`
	Total  int64                    `json:"total"`
}

// List retrieves chunks for a document
func (s *ChunkService) List(req *ListChunksRequest, userID string) (*ListChunksResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	if req.DocID == "" {
		return nil, fmt.Errorf("doc_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}

	// Get document to find its tenant
	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(req.DocID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("document not found")
	}

	// Get knowledge base to find tenant
	kb, err := s.kbDAO.GetByID(doc.KbID)
	if err != nil || kb == nil {
		return nil, fmt.Errorf("knowledge base not found")
	}

	// Find which tenant this document belongs to
	var targetTenantID string
	for _, tenant := range tenants {
		if tenant.TenantID == kb.TenantID {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return nil, fmt.Errorf("user does not have access to this document")
	}

	// Get kbIDs for this tenant
	kbIDs, err := s.kbDAO.GetKBIDsByTenantID(targetTenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kb ids: %w", err)
	}

	indexName := fmt.Sprintf("ragflow_%s", targetTenantID)

	page := common.CoalesceInt(req.Page, 1)
	size := common.CoalesceInt(req.Size, 30)
	keywords := req.Keywords

	// Build search request - same as retrieval test but filtered by doc_id
	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		MatchExprs: []interface{}{keywords},
		KbIDs:      kbIDs,
		Offset:     (page - 1) * size,
		Limit:      size,
		Filter: map[string]interface{}{
			"doc_id": req.DocID,
		},
	}

	// Add available_int filter if specified
	if req.AvailableInt != nil {
		searchReq.Filter["available_int"] = *req.AvailableInt
	}

	// Execute search through unified engine interface
	searchResp, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	chunks := make([]map[string]interface{}, 0, len(searchResp.Chunks))
	for _, chunk := range searchResp.Chunks {
		// Inline formatChunkForList
		result := make(map[string]interface{})
		skipFields := map[string]bool{
			"_id": true, "authors": true, "_score": true, "SCORE": true,
			"important_kwd_empty_count": true, "kb_id": true, "mom_id": true, "page_num_int": true,
		}
		for k, v := range chunk {
			if skipFields[k] || isInternalField(k) {
				continue
			}
			if applyCommonChunkMapping(result, k, v) {
				continue
			}
			switch k {
			case "img_id":
				if strVal, ok := v.(string); ok {
					result["image_id"] = strVal
				} else {
					result["image_id"] = ""
				}
			case "position_int":
				result["positions"] = v
			case "id":
				result["chunk_id"] = v
			default:
				if strings.HasSuffix(k, "_kwd") && k != "knowledge_graph_kwd" {
					result[k] = splitKwdHash(v)
				} else {
					result[k] = v
				}
			}
		}
		chunks = append(chunks, result)
	}

	// Build document info
	timeFormat := "2006-01-02T15:04:05"
	docInfo := map[string]interface{}{
		"id":               doc.ID,
		"thumbnail":        doc.Thumbnail,
		"kb_id":            doc.KbID,
		"parser_id":        doc.ParserID,
		"pipeline_id":      doc.PipelineID,
		"parser_config":    doc.ParserConfig,
		"source_type":      doc.SourceType,
		"type":             doc.Type,
		"created_by":       doc.CreatedBy,
		"name":             doc.Name,
		"location":         doc.Location,
		"size":             doc.Size,
		"token_num":        doc.TokenNum,
		"chunk_num":        doc.ChunkNum,
		"progress":         utility.JSONFloat64(doc.Progress),
		"progress_msg":     doc.ProgressMsg,
		"process_begin_at": utility.FormatTimeToString(doc.ProcessBeginAt, timeFormat),
		"process_duration": doc.ProcessDuration,
		"content_hash":     doc.ContentHash,
		"suffix":           doc.Suffix,
		"run":              doc.Run,
		"status":           doc.Status,
		"create_time":      doc.CreateTime,
		"create_date":      utility.FormatTimeToString(doc.CreateDate, timeFormat),
		"update_time":      doc.UpdateTime,
		"update_date":      utility.FormatTimeToString(doc.UpdateDate, timeFormat),
	}

	return &ListChunksResponse{
		Total:  searchResp.Total,
		Chunks: chunks,
		Doc:    docInfo,
	}, nil
}

// UpdateChunkRequest request for updating a chunk
type UpdateChunkRequest struct {
	DatasetID    string        `json:"dataset_id"`
	DocumentID   string        `json:"document_id"`
	ChunkID      string        `json:"chunk_id"`
	Content      *string       `json:"content,omitempty"`
	ImportantKwd []string      `json:"important_keywords,omitempty"`
	Questions    []string      `json:"questions,omitempty"`
	Available    *bool         `json:"available,omitempty"`
	Positions    []interface{} `json:"positions,omitempty"`
	TagKwd       []string      `json:"tag_kwd,omitempty"`
	TagFeas      interface{}   `json:"tag_feas,omitempty"`
}

// UpdateChunk updates a chunk fields
func (s *ChunkService) UpdateChunk(req *UpdateChunkRequest, userID string) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}

	if req.ChunkID == "" {
		return fmt.Errorf("chunk_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return fmt.Errorf("user has no accessible tenants")
	}

	// Find the tenant that owns this dataset
	var targetTenantID string
	for _, tenant := range tenants {
		kb, err := s.kbDAO.GetByIDAndTenantID(req.DatasetID, tenant.TenantID)
		if err == nil && kb != nil {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return fmt.Errorf("user does not have access to this dataset")
	}

	// Verify document belongs to dataset
	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(req.DocumentID)
	if err != nil || doc == nil {
		return fmt.Errorf("document not found")
	}
	if doc.KbID != req.DatasetID {
		return fmt.Errorf("document does not belong to this dataset")
	}

	// Fetch existing chunk first
	indexName := fmt.Sprintf("ragflow_%s", targetTenantID)
	existingChunk, err := s.docEngine.GetChunk(ctx, indexName, req.ChunkID, []string{req.DatasetID})
	if err != nil {
		return fmt.Errorf("failed to get existing chunk: %w", err)
	}

	existing, ok := existingChunk.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid chunk format")
	}

	// Build update dict
	d := make(map[string]interface{})

	// Content - use new value or existing
	if req.Content != nil {
		d["content_with_weight"] = *req.Content
	} else {
		if v, ok := existing["content_with_weight"].(string); ok {
			d["content_with_weight"] = v
		} else if v, ok := existing["content"].(string); ok {
			d["content_with_weight"] = v
		} else {
			d["content_with_weight"] = ""
		}
	}

	// Tokenize content
	contentStr := d["content_with_weight"].(string)
	d["content_ltks"], _ = tokenizer.Tokenize(contentStr)
	d["content_sm_ltks"], _ = tokenizer.FineGrainedTokenize(d["content_ltks"].(string))

	// Important keywords - convert []string to []interface{} for transformChunkFields
	if req.ImportantKwd != nil {
		impKwd := make([]interface{}, len(req.ImportantKwd))
		for i, v := range req.ImportantKwd {
			impKwd[i] = v
		}
		d["important_kwd"] = impKwd
	}

	// Questions
	if req.Questions != nil {
		// Filter out empty questions and trim
		filteredQuestions := []string{}
		for _, q := range req.Questions {
			q = strings.TrimSpace(q)
			if q != "" {
				filteredQuestions = append(filteredQuestions, q)
			}
		}
		d["question_kwd"] = filteredQuestions
	}

	// Available
	if req.Available != nil {
		if *req.Available {
			d["available_int"] = 1
		} else {
			d["available_int"] = 0
		}
	}

	// Positions
	if req.Positions != nil {
		d["position_int"] = req.Positions
	}

	// Tag keywords
	if req.TagKwd != nil {
		d["tag_kwd"] = req.TagKwd
	}

	// Tag features
	if req.TagFeas != nil {
		d["tag_feas"] = req.TagFeas
	}

	// Always include id
	d["id"] = req.ChunkID

	// Call update
	condition := map[string]interface{}{
		"id": req.ChunkID,
	}

	err = s.docEngine.UpdateChunks(ctx, condition, d, indexName, req.DatasetID)
	if err != nil {
		return fmt.Errorf("failed to update chunk: %w", err)
	}

	return nil
}

// RemoveChunksRequest request for removing chunks
type RemoveChunksRequest struct {
	DocID     string   `json:"doc_id"`
	ChunkIDs  []string `json:"chunk_ids,omitempty"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

// RemoveChunks removes chunks from the dataset table.
// If ChunkIDs is empty and DeleteAll is true, removes all chunks for the document.
// Otherwise removes only the specified chunks.
func (s *ChunkService) RemoveChunks(req *RemoveChunksRequest, userID string) (int64, error) {
	if s.docEngine == nil {
		return 0, fmt.Errorf("doc engine not initialized")
	}

	if req.DocID == "" {
		return 0, fmt.Errorf("doc_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return 0, fmt.Errorf("user has no accessible tenants")
	}

	// Verify document exists and belongs to a dataset (do this first to get doc.KbID)
	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(req.DocID)
	if err != nil || doc == nil {
		return 0, fmt.Errorf("document not found")
	}

	// Find the tenant that owns this document
	var targetTenantID string
	for _, tenant := range tenants {
		kb, err := s.kbDAO.GetByIDAndTenantID(doc.KbID, tenant.TenantID)
		if err == nil && kb != nil {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return 0, fmt.Errorf("user does not have access to this document")
	}

	indexName := fmt.Sprintf("ragflow_%s", targetTenantID)

	// Build condition
	condition := make(map[string]interface{})
	switch {
	case len(req.ChunkIDs) > 0 && req.DeleteAll:
		return 0, fmt.Errorf("chunk_ids and delete_all are mutually exclusive")
	case len(req.ChunkIDs) > 0:
		// Delete specific chunks - convert []string to []interface{} for buildFilterFromCondition
		chunkIDsIf := make([]interface{}, len(req.ChunkIDs))
		for i, id := range req.ChunkIDs {
			chunkIDsIf[i] = id
		}
		condition["id"] = chunkIDsIf
		condition["doc_id"] = req.DocID
	case req.DeleteAll:
		// Delete all chunks for this document
		condition["doc_id"] = req.DocID
	default:
		return 0, fmt.Errorf("either chunk_ids or delete_all must be provided")
	}

	deletedCount, err := s.docEngine.DeleteChunks(ctx, condition, indexName, doc.KbID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete chunks: %w", err)
	}

	return deletedCount, nil
}

// SourcedChunk is a typed, normalized view over a retrieval result chunk.
// It decouples the ask pipeline (KbPrompt, ChunksFormat) from the raw
// map[string]interface{} that flows through the retrieval engine.
type SourcedChunk struct {
	ID               string                 // chunk_id or id
	Content          string                 // content_with_weight or content
	DocID            string                 // doc_id or document_id
	DocName          string                 // docnm_kwd or document_name
	DatasetID        string                 // kb_id or dataset_id
	ImageID          string                 // image_id or img_id
	Positions        string                 // positions or position_int
	URL              string                 // url
	Similarity       float64                // similarity score
	VectorSimilarity float64                // vector_similarity score
	TermSimilarity   float64                // term_similarity score
	DocType          string                 // doc_type_kwd or doc_type
	DocumentMetadata map[string]interface{} // document_metadata
}

// NewSourcedChunks normalizes raw retrieval chunks into typed SourcedChunk values.
// It handles the key aliases used by different engine backends (ES, Infinity).
func NewSourcedChunks(raw []map[string]interface{}) []SourcedChunk {
	out := make([]SourcedChunk, 0, len(raw))
	for _, ck := range raw {
		if ck == nil {
			continue
		}
		out = append(out, SourcedChunk{
			ID:               getStr(ck, "chunk_id", "id"),
			Content:          getStr(ck, "content_with_weight", "content"),
			DocID:            getStr(ck, "doc_id", "document_id"),
			DocName:          getStr(ck, "docnm_kwd", "document_name"),
			DatasetID:        getStr(ck, "kb_id", "dataset_id"),
			ImageID:          getStr(ck, "image_id", "img_id"),
			Positions:        getStr(ck, "positions", "position_int"),
			URL:              getStr(ck, "url"),
			Similarity:       getFloat(ck, "similarity"),
			VectorSimilarity: getFloat(ck, "vector_similarity"),
			TermSimilarity:   getFloat(ck, "term_similarity"),
			DocType:          getStr(ck, "doc_type_kwd", "doc_type"),
			DocumentMetadata: getMap(ck, "document_metadata"),
		})
	}
	return out
}

// getStr tries each key in order and returns the first non-empty string value.
// The first key is the primary name; subsequent keys are fallback aliases
// used by different engine backends (e.g. "content_with_weight" vs "content").
func getStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// getFloat extracts a float64 value from the map, handling the various
// numeric types that different JSON decoders and engine drivers may produce
// (float64, float32, json.Number, int, int64).
func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case float32:
			return float64(f)
		case json.Number:
			if n, err := f.Float64(); err == nil {
				return n
			}
		case int:
			return float64(f)
		case int64:
			return float64(f)
		}
	}
	return 0
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]interface{}); ok {
			// Return a shallow copy so callers cannot mutate the original chunk data.
			out := make(map[string]interface{}, len(mm))
			for k, val := range mm {
				out[k] = val
			}
			return out
		}
	}
	return nil
}

// isInternalField reports whether k is an internal/technical field that
// should be excluded from API chunk responses.
func isInternalField(k string) bool {
	return strings.HasSuffix(k, "_vec") ||
		strings.Contains(k, "_sm_") ||
		strings.HasSuffix(k, "_tks") ||
		strings.HasSuffix(k, "_ltks")
}

// applyCommonChunkMapping applies field mappings shared between GetChunk and
// ListChunks. Returns true if the field was handled.
func applyCommonChunkMapping(result map[string]interface{}, k string, v interface{}) bool {
	switch k {
	case "content":
		result["content_with_weight"] = v
	case "docnm":
		result["docnm_kwd"] = v
	case "important_keywords":
		utility.SetFieldArray(result, "important_kwd", v)
	case "questions":
		utility.SetFieldArray(result, "question_kwd", v)
	case "entities_kwd", "entity_kwd", "entity_type_kwd", "from_entity_kwd",
		"name_kwd", "raptor_kwd", "removed_kwd", "source_id", "tag_kwd",
		"to_entity_kwd", "toc_kwd", "doc_type_kwd":
		if utility.IsEmpty(v) {
			result[k] = []interface{}{}
		} else {
			result[k] = v
		}
	default:
		return false
	}
	return true
}

// splitKwdHash splits a "###"-separated _kwd string into a slice.
// Non-string values or values without "###" are returned unchanged.
func splitKwdHash(v interface{}) interface{} {
	strVal, ok := v.(string)
	if !ok || !strings.Contains(strVal, "###") {
		return v
	}
	parts := strings.Split(strVal, "###")
	filtered := make([]interface{}, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
