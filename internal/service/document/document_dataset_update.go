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
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, userID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("You don't own the dataset.")
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
			err := s.docEngine.UpdateChunks(
				context.Background(),
				map[string]interface{}{"doc_id": docID},
				map[string]interface{}{"available_int": statusInt},
				fmt.Sprintf("ragflow_%s", kb.TenantID),
				doc.KbID,
			)
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
		result[docID] = map[string]string{"status": status}
	}

	if hasError {
		return result, common.CodeServerError, fmt.Errorf("partial failure")
	}
	return result, common.CodeSuccess, nil
}

func (s *DocumentService) UpdateDatasetDocument(ctx context.Context, userID, datasetID, documentID string, req *UpdateDatasetDocumentRequest, present map[string]bool) (*UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	tenantID := userID
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("You don't own the dataset.")
		}
		return nil, common.CodeDataError, errors.New("can't find this dataset")
	}

	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(ctx, dao.DB, documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("The dataset doesn't own the document.")
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
		dslJSON, err = service.LoadPipelineDSL(isPipeline, effParserID, effPipelineID)
		if err != nil {
			common.Warn("cleanAndUpdateDocumentParserConfig: failed to load DSL, falling back to merge",
				zap.Error(err))
			if err = s.updateDocumentParserConfig(ctx, doc.ID, req.ParserConfig); err != nil {
				return nil, common.CodeDataError, err
			}
		} else {
			cleaned := pipelinepkg.BuildParserConfig(dslJSON, req.ParserConfig)
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

func (s *DocumentService) validateDatasetDocumentUpdate(ctx context.Context, datasetID, documentID, userID string, doc *entity.Document, req *UpdateDatasetDocumentRequest, present map[string]bool) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("invalid request payload")
	}
	if present["chunk_count"] && req.ChunkCount != nil && *req.ChunkCount != 0 && *req.ChunkCount != doc.ChunkNum {
		return common.CodeDataError, errors.New("can't change `chunk_count`")
	}
	if present["token_count"] && req.TokenCount != nil && *req.TokenCount != 0 && *req.TokenCount != doc.TokenNum {
		return common.CodeDataError, errors.New("can't change `token_count`")
	}
	if present["progress"] && req.Progress != nil {
		if *req.Progress > 1 {
			return common.CodeDataError, fmt.Errorf("Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <%v>", *req.Progress)
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
	if present["name"] && req.Name != nil {
		if err := s.validateDocumentName(ctx, doc, *req.Name); err != nil {
			return common.CodeDataError, err
		}
	}

	if present["meta_fields"] {
		if err := validateMetaFields(req.MetaFields); err != nil {
			return common.CodeDataError, err
		}
	}

	return common.CodeSuccess, nil
}

func (s *DocumentService) validateDocumentName(ctx context.Context, doc *entity.Document, newName string) error {
	if strings.TrimSpace(newName) == "" {
		return errors.New("file name can't be empty")
	}
	if len([]byte(newName)) > 255 {
		return errors.New("file name must be 255 bytes or less")
	}

	oldName := ""
	if doc.Name != nil {
		oldName = *doc.Name
	}

	if strings.ToLower(filepath.Ext(newName)) != strings.ToLower(filepath.Ext(oldName)) {
		return errors.New("the extension of file can't be changed")
	}

	docs, err := s.documentDAO.GetByNameAndKBID(ctx, dao.DB, newName, doc.KbID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		if d.ID != doc.ID && d.Name != nil && *d.Name == newName {
			return errors.New("duplicated document name in the same dataset")
		}
	}

	return nil
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
					return fmt.Errorf("the type is not supported in list: %v", typed)
				}
			}
		default:
			return fmt.Errorf("the type is not supported: %v", v)
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
		context.Background(),
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
