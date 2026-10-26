package document

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"ragflow/internal/service"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	componentpkg "ragflow/internal/ingestion/component"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

func (s *DocumentService) BatchUpdateDocumentStatus(ctx context.Context, userID, datasetID, status string, documentIDs []string) (map[string]interface{}, common.ErrorCode, error) {
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
	accessibleDocuments := make(map[string]*entity.Document, len(documentIDs))
	var permissionDenied error
	otherFailures := 0
	for _, docID := range documentIDs {
		doc, ok := documentByID[docID]
		if !ok || doc.KbID != datasetID {
			result[docID] = map[string]string{"error": "Document not found in this dataset."}
			otherFailures++
			hasError = true
			continue
		}
		if err := service.CheckDocumentAccess(ctx, permission.Subject{UserID: userID}, doc.ID, datasetID, permission.OperationUpdate); err != nil {
			result[docID] = map[string]string{"error": "Permission denied"}
			if permissionresponse.IsPermissionError(err) {
				permissionDenied = err
			} else {
				otherFailures++
			}
			hasError = true
			continue
		}
		accessibleDocuments[docID] = doc
	}
	if len(accessibleDocuments) == 0 {
		if hasError {
			if permissionDenied != nil && otherFailures == 0 {
				code, normalizedErr := permissionresponse.Normalize(permissionDenied)
				return result, code, normalizedErr
			}
			return result, common.CodeServerError, fmt.Errorf("partial failure")
		}
		return result, common.CodeSuccess, nil
	}
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		return result, common.CodeDataError, fmt.Errorf("dataset not found")
	}

	for _, docID := range documentIDs {
		doc, ok := accessibleDocuments[docID]
		if !ok {
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
			err = s.updateDocumentChunkAvailability(ctx, kb.TenantID, doc.KbID, docID, statusInt)
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
		// A status transition changes document availability, not its parsed
		// content. The status event handles dataset-level retraction/re-enable;
		// scheduling a delayed Wiki recompilation here creates duplicate events
		// and a visible 20-second oscillation.
		s.publishKnowledgeCompileStatusChange(ctx, kb.TenantID, doc.KbID, docID, statusInt)
		result[docID] = map[string]string{"status": status}
	}

	if hasError {
		return result, common.CodeServerError, fmt.Errorf("partial failure")
	}
	return result, common.CodeSuccess, nil
}

func (s *DocumentService) UpdateDatasetDocument(ctx context.Context, userID, datasetID, documentID string, req *UpdateDatasetDocumentRequest, present map[string]bool) (*UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	if err := service.CheckDocumentAccess(ctx, permission.Subject{UserID: userID}, documentID, datasetID, permission.OperationUpdate); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}

	kb, err := s.kbDAO.GetByID(ctx, dao.DB, datasetID)
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
	if code, err := s.validateDatasetDocumentUpdate(ctx, doc, req, present); err != nil {
		return nil, code, err
	}

	// Normalize chunk_method into the same selection used to clean and save
	// parser_config. An omitted selection keeps the document's own template.
	parseType, parserID := req.ParseType, req.ParserID
	hasSelection := parseType != nil ||
		(parserID != nil && strings.TrimSpace(*parserID) != "") ||
		(req.PipelineID != nil && strings.TrimSpace(*req.PipelineID) != "")
	if !hasSelection && present["chunk_method"] {
		builtin := int(service.ParseModeBuiltIn)
		parseType, parserID = &builtin, req.ChunkMethod
	}
	sel, selErr := service.FromRequest(parseType, parserID, req.PipelineID)
	if selErr != nil {
		return nil, common.CodeDataError, selErr
	}
	current := service.CurrentSelection(doc.ParserID, doc.PipelineID)
	eff, _ := service.Resolve(current, sel)
	isPipeline, effParserID, effPipelineID := eff.Effective()
	selectionChanged := current.Mode != eff.Mode ||
		(isPipeline && current.PipelineID != eff.PipelineID) ||
		(!isPipeline && current.ParserID != effParserID)
	if selectionChanged && current.IsBuiltIn() && eff.IsBuiltIn() {
		registry, err := pipelinepkg.DefaultRegistry()
		if err != nil {
			return nil, common.CodeDataError, fmt.Errorf("builtin pipeline registry: %w", err)
		}
		previous, previousOK := registry.Get(current.ParserID)
		target, targetOK := registry.Get(effParserID)
		selectionChanged = !previousOK || !targetOK || previous.ParserID != target.ParserID
	}
	hasConfig := present["parser_config"] && req.ParserConfig != nil
	if (sel != nil || present["parser_config"]) && isPipeline && effPipelineID != nil && strings.TrimSpace(*effPipelineID) != "" {
		accessErr := permission.NewDatabaseChecker(dao.DB).CheckDependency(
			ctx,
			permission.Subject{UserID: userID, TenantID: kb.TenantID},
			permission.ResourceRef{Kind: permission.ResourceKindDocument, ID: doc.ID},
			permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: strings.TrimSpace(*effPipelineID)},
			permission.OperationUpdate,
			permission.OperationUse,
		)
		if accessErr != nil {
			code, permissionErr := permissionresponse.Normalize(accessErr)
			return nil, code, permissionErr
		}
	}

	parserUpdates := map[string]interface{}{}
	if sel != nil || hasConfig {
		// Validate the target before any document, file, or metadata writes.
		dslJSON, err := service.LoadPipelineDSL(ctx, isPipeline, effParserID, effPipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		rawConfig := req.ParserConfig
		if !selectionChanged {
			rawConfig = common.DeepMergeMaps(doc.ParserConfig, req.ParserConfig)
		}
		cleaned, err := pipelinepkg.BuildParserConfig(dslJSON, rawConfig)
		if err != nil {
			return nil, common.CodeDataError, fmt.Errorf("invalid pipeline DSL: %w", err)
		}
		if doc.Name != nil {
			family := componentpkg.ParserFileFamily(utility.GetFileExtension(*doc.Name))
			if family != "" {
				if declared, found := configDeclaredFamily(cleaned, family); found && !declared {
					return nil, common.CodeDataError, fmt.Errorf("selected pipeline does not support file type %q", family)
				}
			}
		}
		if selectionChanged || hasConfig {
			// Inherited runtime delimiters are not an explicit override when
			// this request changes parent_child.
			parentChildOverrides := maps.Clone(rawConfig)
			for componentID, raw := range req.ParserConfig {
				if requested, ok := raw.(map[string]any); ok {
					if _, provided := requested["parent_child"]; provided {
						parentChildOverrides[componentID] = requested
					}
				}
			}
			pipelinepkg.ApplyParentChildChunkerConfig(cleaned, parentChildOverrides)
			llmID, _, _ := service.NewTenantService().GetDefaultModelRefsForUser(ctx, userID, kb.TenantID, entity.ModelTypeChat)
			cleaned = service.ApplyComponentScopedParserConfig(cleaned, llmID)
			parserUpdates["parser_config"] = cleaned
		}
	}

	if sel != nil {
		if isPipeline {
			parserUpdates["pipeline_id"] = *effPipelineID
		} else {
			parserUpdates["parser_id"] = effParserID
			parserUpdates["pipeline_id"] = ""
		}
	}
	if present["meta_fields"] {
		if err = s.replaceDocumentMetadata(ctx, documentID, req.MetaFields); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if present["name"] && req.Name != nil && (doc.Name == nil || *req.Name != *doc.Name) {
		if err = s.updateDocumentNameOnly(ctx, doc, kb, *req.Name); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	// Save selection and config together; parsed content changes only on reparse.
	if len(parserUpdates) > 0 {
		if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, parserUpdates); err != nil {
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

	resp, err := s.toUpdateDatasetDocumentResponse(ctx, updatedDoc, metaFields)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return resp, common.CodeSuccess, nil
}

// validateDocumentModifiable rejects configuration edits while the document is
// actively parsing or scheduled.
func (s *DocumentService) validateDocumentModifiable(ctx context.Context, doc *entity.Document) (common.ErrorCode, error) {
	if s.ingestionTaskDAO == nil {
		return common.CodeServerError, errors.New("ingestion task DAO not initialized")
	}
	if doc == nil || doc.ID == "" {
		return common.CodeDataError, errors.New("document is nil or has empty ID")
	}
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to get ingestion task for document %s: %w", doc.ID, err)
	}
	if task == nil {
		return common.CodeSuccess, nil
	}
	switch {
	case common.IsActiveTaskStatus(task.Status):
		return common.CodeDataError, fmt.Errorf(
			"document is currently %q and cannot be modified; stop parsing or wait for it to finish before updating its configuration", task.Status)
	case common.IsTerminalTaskStatus(task.Status):
		return common.CodeSuccess, nil
	default:
		return common.CodeDataError, fmt.Errorf("document has unrecognized task status %q", task.Status)
	}
}

func (s *DocumentService) validateDatasetDocumentUpdate(ctx context.Context, doc *entity.Document, req *UpdateDatasetDocumentRequest, present map[string]bool) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("invalid request payload")
	}

	// Reject any configuration edit while the document is parsing or scheduled.
	// This guard is field-agnostic: every editable field (name, parser_config,
	// chunk_method, pipeline_id, enabled, meta_fields) is blocked so the
	// in-flight parser never reads a config that changed underneath it.
	if len(present) > 0 {
		if code, err := s.validateDocumentModifiable(ctx, doc); err != nil {
			return code, err
		}
	}
	if present["progress"] && req.Progress != nil {
		if *req.Progress > 1 {
			return common.CodeDataError, fmt.Errorf("Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <%s>", pythonFloatRepr(*req.Progress))
		}
	}
	if err := validateImmutableDocumentFields(doc, immutableDocumentFields{
		chunkNum:            requestField(req.ChunkCount, present["chunk_count"]),
		chunkNumRequestName: "chunk_count",
		tokenNum:            requestField(req.TokenCount, present["token_count"]),
		tokenNumRequestName: "token_count",
		progress:            requestField(req.Progress, present["progress"]),
	}); err != nil {
		return common.CodeDataError, err
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
		registry, err := pipelinepkg.DefaultRegistry()
		if err != nil {
			return common.CodeDataError, fmt.Errorf("builtin pipeline registry: %w", err)
		}
		if !registry.IsValid(*req.ChunkMethod) {
			return common.CodeDataError, fmt.Errorf("unknown builtin parser_id: %s", strings.TrimSpace(*req.ChunkMethod))
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
	if present["parser_config"] && req.ParserConfig != nil {
		// Reject invalid page ranges before any names or metadata are persisted.
		if err := pipelinepkg.NormalizeParserConfigPages(req.ParserConfig); err != nil {
			return common.CodeDataError, err
		}
	}

	return common.CodeSuccess, nil
}

func requestField[T any](value *T, present bool) *T {
	if !present {
		return nil
	}
	return value
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

	available, err := common.NameAvailable(oldName, newName, func(candidate string) (bool, error) {
		return s.documentDAO.NameExistsInKB(ctx, dao.DB, doc.KbID, candidate)
	})
	if err != nil {
		return common.CodeServerError, err
	}
	if !available {
		return common.CodeDataError, errors.New("duplicated document name in the same dataset")
	}

	return common.CodeSuccess, nil
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

// updateDocumentNameOnly renames a document and re-tokenizes its chunks' title
// fields with the dataset language, the way ingestion tokenized them.
func (s *DocumentService) updateDocumentNameOnly(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase, newName string) error {
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

	language := ""
	if kb.Language != nil {
		language = *kb.Language
	}
	tok := tokenizer.New(language)
	titleTks, _ := tok.Tokenize(newName)
	titleSmTks, _ := tok.FineGrainedTokenize(titleTks)
	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
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

func (s *DocumentService) toUpdateDatasetDocumentResponse(ctx context.Context, doc *entity.Document, metaFields map[string]interface{}) (*UpdateDatasetDocumentResponse, error) {
	if metaFields == nil {
		metaFields = map[string]interface{}{}
	}
	ingestionStatus := "UNSTART"
	var task *entity.IngestionTask
	if s.ingestionTaskDAO != nil && doc != nil && doc.ID != "" {
		var err error
		task, err = s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get ingestion task for document %s: %w", doc.ID, err)
		}
		if task != nil && task.Status != "" {
			ingestionStatus = task.Status
		}
	}
	latestEventsByDocument, err := s.latestIngestionEventsByDocument(ctx, map[string]*entity.IngestionTask{doc.ID: task})
	if err != nil {
		common.Warn(fmt.Sprintf("get latest ingestion event for document %s: %v", doc.ID, err))
		latestEventsByDocument = make(map[string]*service.IngestionEventItem)
	}
	return &UpdateDatasetDocumentResponse{
		ID:                   doc.ID,
		Thumbnail:            doc.Thumbnail,
		DatasetID:            doc.KbID,
		ParserID:             doc.ParserID,
		PipelineID:           doc.PipelineID,
		ParserConfig:         doc.ParserConfig,
		SourceType:           doc.SourceType,
		Type:                 doc.Type,
		CreatedBy:            doc.CreatedBy,
		Name:                 doc.Name,
		Location:             doc.Location,
		Size:                 doc.Size,
		TokenCount:           doc.TokenNum,
		ChunkCount:           doc.ChunkNum,
		Progress:             doc.Progress,
		ProgressMsg:          doc.ProgressMsg,
		LatestIngestionEvent: latestEventsByDocument[doc.ID],
		ProcessBeginAt:       doc.ProcessBeginAt,
		ProcessDuration:      doc.ProcessDuration,
		ContentHash:          doc.ContentHash,
		MetaFields:           metaFields,
		Suffix:               doc.Suffix,
		IngestionStatus:      ingestionStatus,
		Status:               doc.Status,
		CreateTime:           doc.CreateTime,
		CreateDate:           doc.CreateDate,
		UpdateTime:           doc.UpdateTime,
		UpdateDate:           doc.UpdateDate,
	}, nil
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
