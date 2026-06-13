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

// REST chunk endpoints (list/add/update/switch) mirroring the Python chunk_api,
// split from chunk.go to keep the SDK-facing surface separate.
package chunk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

func (s *ChunkService) resolveDatasetAccess(userID, datasetID string) (tenantID string, kb *entity.Knowledgebase, err error) {
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	for _, t := range tenants {
		kb, err = s.kbDAO.GetByIDAndTenantID(datasetID, t.TenantID)
		if err == nil && kb != nil {
			return t.TenantID, kb, nil
		}
	}
	return "", nil, fmt.Errorf("You don't own the dataset %s.", datasetID)
}

// getEmbeddingModelForKB resolves the embedding model for a knowledge base.
func (s *ChunkService) getEmbeddingModelForKB(kb *entity.Knowledgebase, tenantID string) (*models.EmbeddingModel, error) {
	tenantLLMDAO := dao.NewTenantLLMDAO()
	modelProviderSvc := service.NewModelProviderService()

	var embdID string
	var err error
	if kb.TenantEmbdID != nil && *kb.TenantEmbdID > 0 {
		_, embdID, err = dao.LookupTenantLLMByID(tenantLLMDAO, *kb.TenantEmbdID)
	} else if kb.EmbdID != "" {
		// Mirror Python add_chunk: DocumentService.get_embd_id returns the raw
		// kb.embd_id composite ("model@instance@provider"), which is resolved
		// directly via get_model_config_from_provider_instance. Pass it straight to
		// GetEmbeddingModel — do NOT pre-split it through the legacy tenant_llm
		// table, which mangles names containing "@" (e.g.
		// "Qwen/Qwen3-Embedding-8B@test@SILICONFLOW") and fails to match the row.
		embdID = kb.EmbdID
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embedding model: %w", err)
	}
	if embdID == "" {
		return nil, fmt.Errorf("no embedding model configured for dataset")
	}
	return modelProviderSvc.GetEmbeddingModel(tenantID, embdID)
}

// embedTexts calls the embedding model and returns the raw float64 slices plus
// an approximate token count for the embedded input.
//
// The embedding driver does not surface provider token usage (EmbeddingData has
// no token field), so the count is derived from the input text via the project
// tokenizer rather than from the embedding vector dimensionality (which is a
// fixed model property unrelated to consumption).
func embedTexts(em *models.EmbeddingModel, texts []string) ([][]float64, int, error) {
	resp, err := em.ModelDriver.Embed(em.ModelName, texts, em.APIConfig, nil)
	if err != nil {
		return nil, 0, err
	}
	vecs := make([][]float64, len(resp))
	for i, d := range resp {
		vecs[i] = d.Embedding
	}
	tokenCount := 0
	for _, t := range texts {
		tokenCount += estimateTokenCount(t)
	}
	return vecs, tokenCount, nil
}

// estimateTokenCount approximates the number of tokens in text. It tokenizes the
// text with the project tokenizer (which segments CJK and splits terms) and
// counts the resulting terms; on failure it falls back to a rune-based estimate
// of roughly one token per four characters.
func estimateTokenCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	if toks, err := tokenizer.Tokenize(text); err == nil && toks != "" {
		return len(strings.Fields(toks))
	}
	return (len([]rune(text)) + 3) / 4
}

// derefString safely dereferences a *string, returning "" when nil.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// mapDocRun maps a document's run-status code to its label, mirroring Python's
// run_mapping in chunk_api._map_doc. Unknown/nil codes map to an empty string.
func mapDocRun(run *string) string {
	if run == nil {
		return ""
	}
	switch strings.TrimSpace(*run) {
	case "0":
		return "UNSTART"
	case "1":
		return "RUNNING"
	case "2":
		return "CANCEL"
	case "3":
		return "DONE"
	case "4":
		return "FAIL"
	default:
		return ""
	}
}

// weightedVec returns 0.1*a + 0.9*b (doc_name weight vs content weight).
func weightedVec(a, b []float64) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = 0.1*a[i] + 0.9*b[i]
	}
	return out
}

// ── ListChunksREST ────────────────────────────────────────────────────────────

// ListChunksREST mirrors Python GET /datasets/:dataset_id/documents/:document_id/chunks.
// dataset_id and document_id are path params; validation is ownership-based.
func (s *ChunkService) ListChunksREST(datasetID, documentID, userID, id string, page, pageSize int, keywords string, available *bool) (*service.ListChunksResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	tenantID, _, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return nil, err
	}

	// Verify document belongs to dataset.
	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("You don't own the document %s.", documentID)
	}
	if doc.KbID != datasetID {
		return nil, fmt.Errorf("You don't own the document %s.", documentID)
	}

	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)

	timeFormat := "2006-01-02T15:04:05"
	// Mirror Python chunk_api._map_doc: return the full document with the SDK key
	// renames (kb_id→dataset_id, chunk_num→chunk_count, token_num→token_count,
	// parser_id→chunk_method) and the run-status label mapping, so the frontend
	// receives every field it expects.
	docInfo := map[string]interface{}{
		"id":               doc.ID,
		"thumbnail":        doc.Thumbnail,
		"dataset_id":       doc.KbID,
		"chunk_method":     doc.ParserID,
		"pipeline_id":      doc.PipelineID,
		"parser_config":    doc.ParserConfig,
		"source_type":      doc.SourceType,
		"type":             doc.Type,
		"created_by":       doc.CreatedBy,
		"name":             doc.Name,
		"location":         doc.Location,
		"size":             doc.Size,
		"token_count":      doc.TokenNum,
		"chunk_count":      doc.ChunkNum,
		"progress":         utility.JSONFloat64(doc.Progress),
		"progress_msg":     doc.ProgressMsg,
		"process_begin_at": utility.FormatTimeToString(doc.ProcessBeginAt, timeFormat),
		"process_duration": doc.ProcessDuration,
		"content_hash":     doc.ContentHash,
		"meta_fields":      doc.MetaFields,
		"suffix":           doc.Suffix,
		"run":              mapDocRun(doc.Run),
		"status":           doc.Status,
		"create_time":      doc.CreateTime,
		"create_date":      utility.FormatTimeToString(doc.CreateDate, timeFormat),
		"update_time":      doc.UpdateTime,
		"update_date":      utility.FormatTimeToString(doc.UpdateDate, timeFormat),
	}

	// Single-chunk lookup by id, mirroring Python list_chunks: when ?id= is given,
	// fetch just that chunk from the doc store, confirm it belongs to this
	// document, and return it (total=1) instead of the paginated list.
	if id != "" {
		raw, gerr := s.docEngine.GetChunk(ctx, indexName, id, []string{datasetID})
		if gerr != nil || raw == nil {
			return nil, fmt.Errorf("Chunk not found: %s/%s", datasetID, id)
		}
		chunk, ok := raw.(map[string]interface{})
		if !ok || firstStr(chunk["doc_id"], chunk["document_id"]) != documentID {
			return nil, fmt.Errorf("Chunk not found: %s/%s", datasetID, id)
		}
		result := map[string]interface{}{
			"id":                 firstVal(chunk["id"], chunk["chunk_id"]),
			"content":            chunk["content_with_weight"],
			"document_id":        firstVal(chunk["doc_id"], chunk["document_id"]),
			"docnm_kwd":          chunk["docnm_kwd"],
			"important_keywords": orSlice(chunk["important_kwd"]),
			"questions":          orSlice(chunk["question_kwd"]),
			"dataset_id":         firstVal(chunk["kb_id"], chunk["dataset_id"]),
			"image_id":           orStr(chunk["img_id"]),
			"available":          intToBool(chunk["available_int"]),
			"positions":          orSlice(chunk["position_int"]),
			"tag_kwd":            orSlice(chunk["tag_kwd"]),
			"tag_feas":           orMap(chunk["tag_feas"]),
		}
		return &service.ListChunksResponse{Total: 1, Chunks: []map[string]interface{}{result}, Doc: docInfo}, nil
	}

	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		MatchExprs: []interface{}{keywords},
		KbIDs:      []string{datasetID},
		Offset:     (page - 1) * pageSize,
		Limit:      pageSize,
		Filter:     map[string]interface{}{"doc_id": documentID},
	}
	if available != nil {
		avInt := 0
		if *available {
			avInt = 1
		}
		searchReq.Filter["available_int"] = avInt
	}

	searchResp, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	chunks := make([]map[string]interface{}, 0, len(searchResp.Chunks))
	for _, chunk := range searchResp.Chunks {
		result := map[string]interface{}{
			"id":                 chunk["id"],
			"content":            chunk["content_with_weight"],
			"document_id":        chunk["doc_id"],
			"docnm_kwd":          chunk["docnm_kwd"],
			"important_keywords": orSlice(chunk["important_kwd"]),
			"questions":          orSlice(chunk["question_kwd"]),
			"tag_kwd":            orSlice(chunk["tag_kwd"]),
			"dataset_id":         datasetID,
			"image_id":           orStr(chunk["img_id"]),
			"available":          intToBool(chunk["available_int"]),
			"positions":          orSlice(chunk["position_int"]),
		}
		chunks = append(chunks, result)
	}

	return &service.ListChunksResponse{
		Total:  searchResp.Total,
		Chunks: chunks,
		Doc:    docInfo,
	}, nil
}

func orSlice(v interface{}) interface{} {
	if v == nil {
		return []interface{}{}
	}
	return v
}

func orStr(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// firstVal returns the first non-nil value, mirroring Python's dict.get(a, b)
// fallback (e.g. chunk.get("id", chunk.get("chunk_id"))).
func firstVal(vals ...interface{}) interface{} {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// firstStr returns the first non-empty string value among the candidates.
func firstStr(vals ...interface{}) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// orMap returns v when it is non-nil, otherwise an empty map — mirroring Python's
// chunk.get("tag_feas", {}).
func orMap(v interface{}) interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	return v
}

func intToBool(v interface{}) bool {
	switch t := v.(type) {
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	case string:
		return t != "0" && t != ""
	}
	return true // default available
}

// ── AddChunk ─────────────────────────────────────────────────────────────────

// AddChunk mirrors Python POST /datasets/:dataset_id/documents/:document_id/chunks.
func (s *ChunkService) AddChunk(datasetID, documentID, userID string, req *service.AddChunkRequest) (map[string]interface{}, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("`content` is required")
	}

	tenantID, kb, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return nil, err
	}

	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil || doc.KbID != datasetID {
		return nil, fmt.Errorf("You don't own the document %s.", documentID)
	}
	docName := derefString(doc.Name)

	// Deterministic chunk ID: xxhash64(content + document_id).
	chunkID := fmt.Sprintf("%x", xxhash.Sum64String(req.Content+documentID))

	// Tokenize content.
	contentLtks, _ := tokenizer.Tokenize(req.Content)
	contentSmLtks, _ := tokenizer.FineGrainedTokenize(contentLtks)

	// Build questions list (trimmed, non-empty).
	questions := make([]string, 0, len(req.Questions))
	for _, q := range req.Questions {
		if q = strings.TrimSpace(q); q != "" {
			questions = append(questions, q)
		}
	}
	importantKwd := req.ImportantKeywords
	if importantKwd == nil {
		importantKwd = []string{}
	}

	now := time.Now()
	d := map[string]interface{}{
		"id":                   chunkID,
		"content_ltks":         contentLtks,
		"content_sm_ltks":      contentSmLtks,
		"content_with_weight":  req.Content,
		"important_kwd":        importantKwd,
		"important_tks":        strings.Join(importantKwd, " "),
		"question_kwd":         questions,
		"question_tks":         strings.Join(questions, "\n"),
		"create_time":          now.Format("2006-01-02 15:04:05"),
		"create_timestamp_flt": float64(now.Unix()),
		"kb_id":                datasetID,
		"docnm_kwd":            docName,
		"doc_id":               documentID,
		"available_int":        1,
	}
	if len(req.TagKwd) > 0 {
		d["tag_kwd"] = req.TagKwd
	}
	if req.TagFeas != nil {
		d["tag_feas"] = req.TagFeas
	}

	// Compute embedding: 0.1 * embed(doc.name) + 0.9 * embed(content).
	em, err := s.getEmbeddingModelForKB(kb, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	embedInput := req.Content
	if len(questions) > 0 {
		embedInput = strings.Join(questions, "\n")
	}
	vecs, tokenCount, err := embedTexts(em, []string{docName, embedInput})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	if len(vecs) >= 2 {
		vec := weightedVec(vecs[0], vecs[1])
		d[fmt.Sprintf("q_%d_vec", len(vec))] = vec
	}

	// Insert into document store.
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if _, err := s.docEngine.InsertChunks(context.Background(), []map[string]interface{}{d}, indexName, datasetID); err != nil {
		return nil, fmt.Errorf("failed to insert chunk: %w", err)
	}

	// Increment document chunk_num and token_num.
	_ = s.documentDAO.UpdateByID(documentID, map[string]interface{}{
		"chunk_num": gorm.Expr("chunk_num + 1"),
		"token_num": gorm.Expr("token_num + ?", tokenCount),
	})

	// Build response matching Python key_mapping.
	renamed := map[string]interface{}{
		"id":               chunkID,
		"content":          req.Content,
		"document_id":      documentID,
		"important_keywords": importantKwd,
		"questions":        questions,
		"dataset_id":       datasetID,
		"create_timestamp": float64(now.Unix()),
		"create_time":      d["create_time"],
	}
	if len(req.TagKwd) > 0 {
		renamed["tag_kwd"] = req.TagKwd
	}
	return map[string]interface{}{"chunk": renamed}, nil
}

// ── UpdateChunkREST ───────────────────────────────────────────────────────────

// UpdateChunkREST mirrors Python PATCH /datasets/:dataset_id/documents/:document_id/chunks/:chunk_id.
// Like the existing UpdateChunk but with re-embedding on content change.
func (s *ChunkService) UpdateChunkREST(datasetID, documentID, chunkID, userID string, req *service.UpdateChunkRESTRequest) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}

	tenantID, kb, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return err
	}

	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil || doc.KbID != datasetID {
		return fmt.Errorf("You don't own the document %s.", documentID)
	}
	docName := derefString(doc.Name)

	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)

	// Get existing chunk.
	rawChunk, err := s.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
	if err != nil || rawChunk == nil {
		return fmt.Errorf("Can't find this chunk %s", chunkID)
	}
	existing, ok := rawChunk.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Can't find this chunk %s", chunkID)
	}
	existingDocID := ""
	if v, ok := existing["doc_id"].(string); ok {
		existingDocID = v
	} else if v, ok := existing["document_id"].(string); ok {
		existingDocID = v
	}
	if existingDocID != documentID {
		return fmt.Errorf("Can't find this chunk %s", chunkID)
	}

	// Determine content.
	content := ""
	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			return fmt.Errorf("`content` is required")
		}
		content = *req.Content
	} else {
		if v, ok := existing["content_with_weight"].(string); ok {
			content = v
		} else if v, ok := existing["content"].(string); ok {
			content = v
		}
	}

	// Tokenize.
	contentLtks, _ := tokenizer.Tokenize(content)
	contentSmLtks, _ := tokenizer.FineGrainedTokenize(contentLtks)

	d := map[string]interface{}{
		"id":                  chunkID,
		"content_with_weight": content,
		"content_ltks":        contentLtks,
		"content_sm_ltks":     contentSmLtks,
	}

	if req.ImportantKeywords != nil {
		d["important_kwd"] = req.ImportantKeywords
		d["important_tks"] = strings.Join(req.ImportantKeywords, " ")
	}

	questions := []string{}
	if req.Questions != nil {
		for _, q := range req.Questions {
			if q = strings.TrimSpace(q); q != "" {
				questions = append(questions, q)
			}
		}
		d["question_kwd"] = questions
		d["question_tks"] = strings.Join(questions, "\n")
	}

	if req.Available != nil {
		avInt := 0
		if *req.Available {
			avInt = 1
		}
		d["available_int"] = avInt
	}
	if req.Positions != nil {
		d["position_int"] = req.Positions
	}
	if req.TagKwd != nil {
		d["tag_kwd"] = req.TagKwd
	}
	if req.TagFeas != nil {
		d["tag_feas"] = req.TagFeas
	}

	// Re-embed when content or questions changed.
	if req.Content != nil || req.Questions != nil {
		em, err := s.getEmbeddingModelForKB(kb, tenantID)
		if err != nil {
			return fmt.Errorf("failed to get embedding model: %w", err)
		}
		embedInput := content
		if len(questions) > 0 {
			embedInput = strings.Join(questions, "\n")
		}
		vecs, _, err := embedTexts(em, []string{docName, embedInput})
		if err != nil {
			return fmt.Errorf("embedding failed: %w", err)
		}
		if len(vecs) >= 2 {
			vec := weightedVec(vecs[0], vecs[1])
			d[fmt.Sprintf("q_%d_vec", len(vec))] = vec
		}
	}

	condition := map[string]interface{}{"id": chunkID}
	return s.docEngine.UpdateChunks(ctx, condition, d, indexName, datasetID)
}

// ── SwitchChunks ─────────────────────────────────────────────────────────────

// SwitchChunks mirrors Python PATCH /datasets/:dataset_id/documents/:document_id/chunks
// (without chunk_id) — bulk toggle of available_int.
func (s *ChunkService) SwitchChunks(datasetID, documentID, userID string, chunkIDs []string, availableInt int) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}
	if len(chunkIDs) == 0 {
		return fmt.Errorf("`chunk_ids` is required.")
	}

	tenantID, _, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return err
	}

	// Mirror Python: verify the document belongs to the dataset before touching
	// the index.
	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil || doc.KbID != datasetID {
		return fmt.Errorf("Document not found!")
	}

	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)

	// Update each chunk's available_int. Python's docStoreConn.update returns False
	// for a non-existent chunk id, surfacing as "Index updating failure" (code 102);
	// a blind update would otherwise report a false-positive success. Confirm the
	// chunk exists, then update.
	for _, chunkID := range chunkIDs {
		existing, gerr := s.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
		if gerr != nil || existing == nil {
			return fmt.Errorf("Index updating failure")
		}
		condition := map[string]interface{}{"id": chunkID}
		update := map[string]interface{}{"available_int": availableInt}
		if err := s.docEngine.UpdateChunks(ctx, condition, update, indexName, datasetID); err != nil {
			common.Warn("SwitchChunks: failed to update chunk", zap.String("chunkID", chunkID), zap.Error(err))
			return fmt.Errorf("Index updating failure")
		}
	}
	return nil
}

