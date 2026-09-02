package document

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"ragflow/internal/service"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

func (s *DocumentService) BatchUpdateDocumentStatus(ctx context.Context, userID, datasetID, status string, documentIDs []string) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByIDAndTenantID(ctx, dao.DB, datasetID, userID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("you don't own the dataset")
	}
	statusInt, convErr := strconv.Atoi(status)
	if convErr != nil {
		return nil, common.CodeArgumentError, fmt.Errorf("invalid status: %s", status)
	}

	result := make(map[string]interface{}, len(documentIDs))
	hasError := false

	documents, err := s.documentDAO.GetByIDs(ctx, dao.DB, documentIDs)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to fetch documents: %w", err)
	}
	documentByID := make(map[string]*entity.Document, len(documents))
	for _, doc := range documents {
		documentByID[doc.ID] = doc
	}

	for _, docID := range documentIDs {
		doc, ok := documentByID[docID]
		if !ok {
			result[docID] = map[string]string{"error": "Document not found"}
			hasError = true
			continue
		}

		if doc.KbID != datasetID {
			result[docID] = map[string]string{"error": "Document not found in this dataset."}
			hasError = true
			continue
		}

		currentStatus := ""
		if doc.Status != nil {
			currentStatus = *doc.Status
		}
		if currentStatus == status {
			result[docID] = map[string]string{"status": status}
			continue
		}
		previousStatus := interface{}(nil)
		if doc.Status != nil {
			previousStatus = *doc.Status
		}
		if err = s.documentDAO.UpdateByID(ctx, dao.DB, docID, map[string]interface{}{"status": status}); err != nil {
			result[docID] = map[string]string{"error": "Database error (Document update)!"}
			hasError = true
			continue
		}

		if doc.ChunkNum > 0 {
			if s.docEngine == nil {
				_ = s.documentDAO.UpdateByID(ctx, dao.DB, docID, map[string]interface{}{"status": previousStatus})
				result[docID] = map[string]string{"error": "Document store update failed: document engine not initialized"}
				hasError = true
				continue
			}
			err = s.updateSourceChunkAvailability(ctx, kb.TenantID, doc.KbID, docID, statusInt)
			if err != nil {
				_ = s.documentDAO.UpdateByID(ctx, dao.DB, docID, map[string]interface{}{"status": previousStatus})
				msg := err.Error()
				if strings.Contains(msg, "3022") {
					result[docID] = map[string]string{"error": "Document store table missing."}
				} else {
					result[docID] = map[string]string{"error": "Document store update failed: " + msg}
				}
				hasError = true
				continue
			}
		}
		s.markDocumentWikiDirty(ctx, kb.TenantID, doc.KbID, docID)
		result[docID] = map[string]string{"status": status}
	}

	if hasError {
		return result, common.CodeServerError, fmt.Errorf("partial failure")
	}
	return result, common.CodeSuccess, nil
}

func (s *DocumentService) UpdateDatasetDocument(ctx context.Context, userID, datasetID, documentID string, req *UpdateDatasetDocumentRequest, present map[string]bool) (*UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	tenantID := userID
	kb, err := s.kbDAO.GetByIDAndTenantID(ctx, dao.DB, datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("you don't own the dataset")
		}
		return nil, common.CodeDataError, errors.New("can't find this dataset")
	}

	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(ctx, dao.DB, documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("the dataset doesn't own the document")
		}
		return nil, common.CodeServerError, err
	}

	if code, err := s.validateDatasetDocumentUpdate(ctx, datasetID, documentID, userID, doc, req, present); err != nil {
		return nil, code, err
	}

	if present["meta_fields"] {
		if err = s.replaceDocumentMetadata(ctx, documentID, req.MetaFields); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["name"] && req.Name != nil && (doc.Name == nil || *req.Name != *doc.Name) {
		if err = s.updateDocumentNameOnly(ctx, doc, kb.TenantID, *req.Name); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	// Resolve the effective parse mode once: parse_type is authoritative when
	// present, otherwise inherit the dataset's current mode. Both the
	// parser_config cleaning and the reparse targeting derive from this single
	// resolution so they can never disagree. (See service.ResolveParseMode.)
	isPipeline, effParserID, effPipelineID := service.ResolveParseMode(
		req.ParseType, req.ParserID, req.PipelineID,
		service.ParseModeState{ParserID: kb.ParserID, PipelineID: kb.PipelineID})

	if present["parser_config"] && req.ParserConfig != nil {
		// Normalize "pages" ranges before persistence. Invalid ranges are
		// rejected (fail-fast) — the request is aborted with a clear error.
		if err = pipelinepkg.NormalizeParserConfigPages(req.ParserConfig); err != nil {
			return nil, common.CodeDataError, err
		}
		var dslJSON []byte
		dslJSON, err = service.LoadPipelineDSL(ctx, isPipeline, effParserID, effPipelineID)
		if err != nil {
			common.Warn("cleanAndUpdateDocumentParserConfig: failed to load DSL, falling back to merge",
				zap.Error(err))
			if err = s.updateDocumentParserConfig(ctx, doc.ID, req.ParserConfig); err != nil {
				return nil, common.CodeDataError, err
			}
		} else {
			cleaned := pipelinepkg.BuildParserConfig(dslJSON, req.ParserConfig)
			tenant, tenantErr := dao.NewTenantDAO().GetByID(ctx, dao.DB, kb.TenantID)
			if tenantErr == nil && tenant != nil {
				cleaned = service.ApplyComponentScopedParserConfig(
					cleaned,
					tenant.LLMID,
				)
			}
			if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
				"parser_config": cleaned,
			}); err != nil {
				return nil, common.CodeDataError, err
			}
		}
	}

	// Apply parser_id / pipeline_id changes. parse_type is validated earlier
	// (in validateDatasetDocumentUpdate); reparse only fires when parse_type
	// explicitly selects a mode. The mode direction comes from the same
	// isPipeline resolution above.
	var reparseParserID, reparsePipelineID *string
	if req.ParseType != nil {
		switch {
		case !isPipeline: // BuiltIn
			if present["parser_id"] && req.ParserID != nil {
				if p := strings.TrimSpace(*req.ParserID); p != "" {
					reparseParserID = &p
				}
			}
			// Drop any prior canvas so the worker falls back to the builtin template.
			empty := ""
			reparsePipelineID = &empty
		case isPipeline: // Pipeline
			if present["pipeline_id"] && req.PipelineID != nil {
				reparsePipelineID = req.PipelineID
			}
		}
	} else if present["chunk_method"] && req.ChunkMethod != nil {
		// chunk_method is the public alias for a direct parser change; it does
		// not require parse_type (mirrors Python's update_chunk_method).
		if p := strings.TrimSpace(*req.ChunkMethod); !strings.EqualFold(p, doc.ParserID) {
			reparseParserID = &p
			empty := ""
			reparsePipelineID = &empty
		} else if doc.PipelineID != nil && *doc.PipelineID != "" {
			empty := ""
			reparsePipelineID = &empty
		}
	}
	if reparseParserID != nil || reparsePipelineID != nil {
		if err = s.resetDocumentForReparse(ctx, doc, kb.TenantID, reparseParserID, reparsePipelineID); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["enabled"] && req.Enabled != nil {
		if err = s.updateDocumentStatusOnly(ctx, doc, kb, *req.Enabled); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	updatedDoc, err := s.documentDAO.GetByID(ctx, dao.DB, doc.ID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("can not get document by id:%s", doc.ID)
		}
		return nil, common.CodeDataError, errors.New("database operation failed")
	}

	metaFields := map[string]interface{}{}
	if s.docEngine != nil && s.metadataSvc != nil {
		metaFields, _ = s.GetDocumentMetadataByID(ctx, updatedDoc.ID)
	}

	return s.toUpdateDatasetDocumentResponse(updatedDoc, metaFields), common.CodeSuccess, nil
}

// validateDocumentModifiable rejects configuration edits while the document is
// actively parsing or scheduled. Editing a document that is mid-parse races
// with the running worker and can leave the document stuck and undeletable.
func (s *DocumentService) validateDocumentModifiable(doc *entity.Document) (common.ErrorCode, error) {
	if doc.Run != nil {
		run := entity.TaskStatus(*doc.Run)
		if !entity.DocumentModifiableStatuses[run] {
			return common.CodeDataError, fmt.Errorf(
				"document is currently %q and cannot be modified; stop parsing or wait for it to finish before updating its configuration", run)
		}
	}
	return common.CodeSuccess, nil
}

func (s *DocumentService) validateDatasetDocumentUpdate(ctx context.Context, datasetID, documentID, userID string, doc *entity.Document, req *UpdateDatasetDocumentRequest, present map[string]bool) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("invalid request payload")
	}

	// Reject any configuration edit while the document is parsing or scheduled.
	// This guard is field-agnostic: every editable field (name, parser_config,
	// chunk_method, pipeline_id, enabled, meta_fields) is blocked so the
	// in-flight parser never reads a config that changed underneath it.
	if len(present) > 0 {
		if code, err := s.validateDocumentModifiable(doc); err != nil {
			return code, err
		}
	}
	if present["chunk_count"] && req.ChunkCount != nil && *req.ChunkCount != 0 && *req.ChunkCount != doc.ChunkNum {
		return common.CodeDataError, errors.New("can't change `chunk_count`")
	}
	if present["token_count"] && req.TokenCount != nil && *req.TokenCount != 0 && *req.TokenCount != doc.TokenNum {
		return common.CodeDataError, errors.New("can't change `token_count`")
	}
	if present["progress"] && req.Progress != nil {
		if *req.Progress > 1 {
			return common.CodeDataError, fmt.Errorf("Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <%s>", pythonFloatRepr(*req.Progress))
		}
		if *req.Progress != 0 && math.Abs(*req.Progress-doc.Progress) > 1e-9 {
			return common.CodeDataError, errors.New("can't change `progress`")
		}
	}

	if present["enabled"] {
		if req.Enabled == nil || (*req.Enabled != 0 && *req.Enabled != 1) {
			return common.CodeDataError, errors.New("`enabled` value invalid, only accept 0 or 1")
		}
	}

	if present["chunk_method"] {
		if req.ChunkMethod == nil || strings.TrimSpace(*req.ChunkMethod) == "" {
			return common.CodeDataError, errors.New("`chunk_method` (empty string) is not valid")
		}
		cm := strings.TrimSpace(*req.ChunkMethod)
		if !validDocumentChunkMethods[cm] {
			return common.CodeDataError, fmt.Errorf("Field: <chunk_method> - Message: <`chunk_method` %s doesn't exist> - Value: <%s>", cm, cm)
		}
		if (doc.Type == "visual" && cm != "picture") || (isPresentationFile(doc.Name) && cm != "presentation") {
			return common.CodeDataError, errors.New("not supported yet")
		}
	}

	if present["parse_type"] || present["parser_id"] || present["pipeline_id"] {
		isBuiltin, _, err := service.ValidateParseTypeMode(req.ParseType, req.ParserID, req.PipelineID)
		if err != nil {
			return common.CodeDataError, err
		}
		// The parser_id type constraint (visual/presentation) only applies in
		// builtin mode — in pipeline mode parser_id is not applicable.
		if isBuiltin && present["parser_id"] && req.ParserID != nil {
			parserID := strings.TrimSpace(*req.ParserID)
			if (doc.Type == "visual" && parserID != "picture") || (isPresentationFile(doc.Name) && parserID != "presentation") {
				return common.CodeDataError, errors.New("not supported yet")
			}
		}
	}
	if present["name"] {
		// An explicit null name is a type error, not an unset field.
		if req.Name == nil {
			return common.CodeDataError, errors.New("Field: <name> - Message: <Input should be a valid string> - Value: <None>")
		}
		if code, err := s.validateDocumentName(ctx, doc, *req.Name); err != nil {
			return code, err
		}
	}

	if present["meta_fields"] {
		if err := validateMetaFields(req.MetaFields); err != nil {
			return common.CodeDataError, err
		}
	}

	return common.CodeSuccess, nil
}

// validateDocumentName mirrors Python's validate_document_name: length check
// (101), then extension check (101), then duplicate check (102).
func (s *DocumentService) validateDocumentName(ctx context.Context, doc *entity.Document, newName string) (common.ErrorCode, error) {
	if len([]byte(newName)) > 255 {
		return common.CodeArgumentError, errors.New("File name must be 255 bytes or less.")
	}

	oldName := ""
	if doc.Name != nil {
		oldName = *doc.Name
	}

	if strings.ToLower(filepath.Ext(newName)) != strings.ToLower(filepath.Ext(oldName)) {
		return common.CodeArgumentError, errors.New("the extension of file can't be changed")
	}

	docs, err := s.documentDAO.GetByNameAndKBID(ctx, dao.DB, newName, doc.KbID)
	if err != nil {
		return common.CodeServerError, err
	}
	for _, d := range docs {
		if d.ID != doc.ID && d.Name != nil && *d.Name == newName {
			return common.CodeDataError, errors.New("duplicated document name in the same dataset")
		}
	}

	return common.CodeSuccess, nil
}

func isPresentationFile(name *string) bool {
	if name == nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(*name))
	return ext == ".ppt" || ext == ".pptx" || ext == ".pages"
}

func validateMetaFields(meta map[string]any) error {
	if meta == nil {
		return nil
	}

	for _, v := range meta {
		switch typed := v.(type) {
		case string, float64, int, int64, float32:
			continue
		case []any:
			for _, item := range typed {
				switch item.(type) {
				case string, float64, int, int64, float32:
					continue
				default:
					return fmt.Errorf("Field: <meta_fields> - Message: <The type is not supported in list: %s> - Value: <%s>", pyRepr(typed), pyRepr(map[string]interface{}(meta)))
				}
			}
		default:
			return fmt.Errorf("Field: <meta_fields> - Message: <The type is not supported: %s> - Value: <%s>", pyRepr(v), pyRepr(map[string]interface{}(meta)))
		}
	}

	return nil
}

func (s *DocumentService) updateDocumentNameOnly(ctx context.Context, doc *entity.Document, tenantID, newName string) error {
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{"name": newName}); err != nil {
		return errors.New("database error (Document rename)")
	}

	mappings, err := s.file2DocumentDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err == nil && len(mappings) > 0 && mappings[0].FileID != nil && s.fileDAO != nil {
		err = s.fileDAO.UpdateByID(ctx, dao.DB, *mappings[0].FileID, map[string]interface{}{"name": newName})
		if err != nil {
			return fmt.Errorf("file rename failed after document rename: %w", err)
		}
	}

	if s.docEngine == nil {
		return nil
	}

	titleTks, _ := tokenizer.Tokenize(newName)
	titleSmTks, _ := tokenizer.FineGrainedTokenize(titleTks)
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	return s.docEngine.UpdateChunks(
		ctx,
		map[string]interface{}{"doc_id": doc.ID},
		map[string]interface{}{
			"docnm_kwd":    newName,
			"title_tks":    titleTks,
			"title_sm_tks": titleSmTks,
		},
		indexName,
		doc.KbID,
	)
}

func (s *DocumentService) updateDocumentParserConfig(ctx context.Context, documentID string, config map[string]any) error {
	if len(config) == 0 {
		return nil
	}

	doc, err := s.documentDAO.GetByID(ctx, dao.DB, documentID)
	if err != nil {
		return fmt.Errorf("document(%s) not found", documentID)
	}

	merged := common.DeepMergeMaps(doc.ParserConfig, config)
	if _, ok := config["raptor"]; !ok {
		delete(merged, "raptor")
	}

	return s.documentDAO.UpdateByID(ctx, dao.DB, documentID, map[string]interface{}{
		"parser_config": entity.JSONMap(merged),
	})
}

func (s *DocumentService) toUpdateDatasetDocumentResponse(doc *entity.Document, metaFields map[string]interface{}) *UpdateDatasetDocumentResponse {
	if metaFields == nil {
		metaFields = map[string]interface{}{}
	}
	return &UpdateDatasetDocumentResponse{
		ID:              doc.ID,
		Thumbnail:       doc.Thumbnail,
		DatasetID:       doc.KbID,
		ParserID:        doc.ParserID,
		PipelineID:      doc.PipelineID,
		ParserConfig:    doc.ParserConfig,
		SourceType:      doc.SourceType,
		Type:            doc.Type,
		CreatedBy:       doc.CreatedBy,
		Name:            doc.Name,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenCount:      doc.TokenNum,
		ChunkCount:      doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessBeginAt:  doc.ProcessBeginAt,
		ProcessDuration: doc.ProcessDuration,
		ContentHash:     doc.ContentHash,
		MetaFields:      metaFields,
		Suffix:          doc.Suffix,
		Run:             mapDocumentRunStatus(doc.Run),
		Status:          doc.Status,
		CreateTime:      doc.CreateTime,
		CreateDate:      doc.CreateDate,
		UpdateTime:      doc.UpdateTime,
		UpdateDate:      doc.UpdateDate,
	}
}

func mapDocumentRunStatus(run *string) string {
	if run == nil {
		return "UNSTART"
	}
	switch *run {
	case string(entity.TaskStatusRunning):
		return "RUNNING"
	case string(entity.TaskStatusCancel):
		return "CANCEL"
	case string(entity.TaskStatusDone):
		return "DONE"
	case string(entity.TaskStatusFail):
		return "FAIL"
	default:
		return "UNSTART"
	}
}

// validDocumentChunkMethods mirrors Python's UpdateDocumentReq chunk_method set.
var validDocumentChunkMethods = map[string]bool{
	"naive": true, "manual": true, "qa": true, "table": true, "paper": true,
	"book": true, "laws": true, "presentation": true, "picture": true,
	"one": true, "knowledge_graph": true, "email": true, "tag": true,
}

// pythonFloatRepr formats a float the way Python's str()/repr() does:
// integral floats keep a trailing ".0".
func pythonFloatRepr(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.1f", v)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// pyRepr renders a decoded JSON value the way Python's repr() does, for
// pydantic-style "Field: <f> - Message: <m> - Value: <v>" contract messages.
func pyRepr(v interface{}) string {
	switch typed := v.(type) {
	case nil:
		return "None"
	case string:
		return "'" + strings.ReplaceAll(typed, "'", "\\'") + "'"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case float64:
		if typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		return pyRepr(float64(typed))
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case []interface{}:
		parts := make([]string, len(typed))
		for i, item := range typed {
			parts[i] = pyRepr(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]interface{}:
		parts := make([]string, 0, len(typed))
		for k, item := range typed {
			parts = append(parts, pyRepr(k)+": "+pyRepr(item))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprintf("%v", v)
}
