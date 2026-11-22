package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"path/filepath"

	"ragflow/internal/permission"
	"ragflow/internal/service"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	componentpkg "ragflow/internal/ingestion/component"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service/datasetversion"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ErrDocumentNameTaken is returned when a restore cannot put a document back
// under its recorded name because another document of the dataset holds it.
var ErrDocumentNameTaken = errors.New("another document of the dataset already holds that name")

func (s *DocumentService) BatchUpdateDocumentStatus(ctx context.Context, datasetID, status string, documentIDs []string) (map[string]interface{}, common.ErrorCode, error) {
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
		if err := service.CheckDocumentAccess(ctx, doc.ID, datasetID, permission.OperationUpdate); err != nil {
			_, normalizedErr := permissionresponse.Normalize(err)
			result[docID] = map[string]string{"error": normalizedErr.Error()}
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
			return result, common.CodeServerError, fmt.Errorf("partial failure: %v", result)
		}
		return result, common.CodeSuccess, nil
	}
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		return result, common.CodeDataError, fmt.Errorf("dataset not found")
	}

	// changed collects the documents this call actually moved, so the dataset
	// timeline records one version for the switch instead of one per document.
	changed := make([]DocumentStatusChange, 0, len(accessibleDocuments))
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
			result[docID] = map[string]string{"error": "Database error (Document update): " + err.Error()}
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
		changed = append(changed, DocumentStatusChange{
			DocumentID: docID,
			DatasetID:  datasetID,
			Name:       documentNameOf(doc),
			Status:     status,
		})
	}

	// The switch takes the document's chunks out of retrieval, so it is a change
	// to what the dataset exposes: one version for the batch, with a state row per
	// document so a restore can put the switch back. A toggle that moved nothing
	// records nothing.
	if recordErr := RecordDocumentStatusChangesEE(ctx, dao.DB, datasetID, changed); recordErr != nil {
		return result, common.CodeServerError, fmt.Errorf("documents updated but their status version was not recorded: %w", recordErr)
	}

	if hasError {
		return result, common.CodeServerError, fmt.Errorf("partial failure: %v", result)
	}
	return result, common.CodeSuccess, nil
}

func (s *DocumentService) UpdateDatasetDocument(ctx context.Context, datasetID, documentID string, req *UpdateDatasetDocumentRequest, present map[string]bool) (*UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	checker := permission.NewDatabaseChecker(dao.DB)
	datasetRef := permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}
	if err := checker.CheckResource(ctx, datasetRef, permission.OperationUpdate); err != nil {
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
	if err := service.CheckDocumentAccess(ctx, documentID, datasetID, permission.OperationUpdate); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}
	if code, err := s.validateDatasetDocumentUpdate(ctx, datasetID, documentID, doc, req, present); err != nil {
		return nil, code, err
	}

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

	effectiveVersionID := doc.PipelineVersionID
	repointVersion := false
	if isPipeline && effPipelineID != nil && strings.TrimSpace(*effPipelineID) != "" && present["pipeline_version_id"] {
		canvasID := strings.TrimSpace(*effPipelineID)
		if req.PipelineVersionID != nil && strings.TrimSpace(*req.PipelineVersionID) != "" {
			requested := strings.TrimSpace(*req.PipelineVersionID)
			v, verr := dao.NewUserCanvasVersionDAO().GetByID(ctx, dao.DB, requested)
			if verr != nil {
				if errors.Is(verr, dao.ErrUserCanvasVersionNotFound) {
					return nil, common.CodeDataError, fmt.Errorf("pipeline version %s not found: %w", requested, service.ErrPipelineVersionMissing)
				}
				return nil, common.CodeServerError, fmt.Errorf("load pipeline version %s: %w", requested, verr)
			}
			if v.UserCanvasID != canvasID {
				return nil, common.CodeDataError, fmt.Errorf("pipeline version %s does not belong to canvas %s: %w", requested, canvasID, service.ErrPipelineVersionMissing)
			}
			effectiveVersionID = &requested
		} else {
			if isPipeline && effPipelineID != nil && strings.TrimSpace(*effPipelineID) != "" {
				return nil, common.CodeBadRequest, service.ErrPipelineVersionEmpty
			}
			effectiveVersionID = nil
		}
		repointVersion = true
	}

	if !present["pipeline_version_id"] && effPipelineID != nil && strings.TrimSpace(*effPipelineID) != "" {
		// C5/G3: a canvas-to-canvas switch without supplying a version is a
		// missing-version Save and is rejected, so the document cannot silently
		// track the new canvas latest. A builtin-to-canvas switch is allowed: it
		// tracks the new canvas latest, and any broken-DSL error surfaces below.
		docOnCanvas := doc.PipelineID != nil && strings.TrimSpace(*doc.PipelineID) != ""
		pipelineChanged := doc.PipelineID == nil || strings.TrimSpace(*doc.PipelineID) != strings.TrimSpace(*effPipelineID)
		if docOnCanvas && pipelineChanged {
			return nil, common.CodeBadRequest, service.ErrPipelineVersionEmpty
		}
	}

	versionChanged := false
	if repointVersion {
		oldV := ""
		if doc.PipelineVersionID != nil {
			oldV = strings.TrimSpace(*doc.PipelineVersionID)
		}
		newV := ""
		if effectiveVersionID != nil {
			newV = strings.TrimSpace(*effectiveVersionID)
		}
		versionChanged = newV != oldV
	}

	if (sel != nil || present["parser_config"]) && isPipeline && effPipelineID != nil && strings.TrimSpace(*effPipelineID) != "" {
		accessErr := permission.NewDatabaseChecker(dao.DB).CheckDependency(
			ctx,
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
		dslJSON, err := service.LoadPipelineDSL(ctx, isPipeline, effParserID, effPipelineID, effectiveVersionID)
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
		// File-family compatibility is only declared by builtin parser DSLs. Canvas
		// pipelines do not declare supported file families, so skip the check there.
		if !isPipeline && doc.Name != nil {
			family := componentpkg.ParserFileFamily(utility.GetFileExtension(*doc.Name))
			if family != "" {
				if declared, found := configDeclaredFamily(cleaned, family); found && !declared {
					return nil, common.CodeDataError, fmt.Errorf("selected pipeline does not support file type %q", family)
				}
			}
		}
		if selectionChanged || hasConfig {
			parentChildOverrides := maps.Clone(rawConfig)
			for componentID, raw := range req.ParserConfig {
				if requested, ok := raw.(map[string]any); ok {
					if _, provided := requested["parent_child"]; provided {
						parentChildOverrides[componentID] = requested
					}
				}
			}
			pipelinepkg.ApplyParentChildChunkerConfig(cleaned, parentChildOverrides)
			llmID := ""
			if modelInfo, modelErr := service.NewModelFactory().ResolveDefaultInfo(permission.WithTenantID(ctx, kb.TenantID), entity.ModelTypeChat); modelErr == nil {
				llmID = service.ModelReference(modelInfo)
			}
			cleaned = service.ApplyComponentScopedParserConfig(cleaned, llmID)
			parserUpdates["parser_config"] = cleaned
		}
	}

	// C3 / A3: a pipeline (canvas) switch OR a version switch resets
	// parser_config to the new DSL defaults — but only when the request carried
	// no parser_config of its own. A submitted config has already been
	// reconciled against the new version's DSL by BuildParserConfig above
	// (unknown operators dropped, missing params defaulted), and the settings
	// form resets itself to the new defaults on a switch before the user
	// edits — so a submitted config is authoritative and must not be
	// discarded. This mirrors the dataset-layer behavior so the two layers
	// stay consistent.
	//
	// pipelineChanged tracks a genuine canvas change (incl. builtin→canvas);
	// versionChanged tracks a pinned-version change. Builtin→builtin switches are
	// not pipelineID changes and stay outside this reset (they are handled by the
	// first block as a normal selection rebuild).
	pipelineChanged := isPipeline && req.PipelineID != nil &&
		(doc.PipelineID == nil || strings.TrimSpace(*req.PipelineID) != strings.TrimSpace(*doc.PipelineID))
	if (pipelineChanged || versionChanged) && !hasConfig {
		cpDefaults, cpErr := service.ResolveComponentParamsDefaults(ctx, effParserID, effPipelineID, effectiveVersionID)
		if cpErr != nil {
			common.Warn("document pipeline/version switch: failed to resolve component params defaults, keeping prior parser_config",
				zap.String("parserID", effParserID), zap.Error(cpErr))
		} else {
			llmID := ""
			if modelInfo, llmErr := service.NewModelFactory().ResolveDefaultInfo(permission.WithTenantID(ctx, kb.TenantID), entity.ModelTypeChat); llmErr == nil {
				llmID = service.ModelReference(modelInfo)
			} else if !errors.Is(llmErr, service.ErrNoDefaultModel) {
				common.Warn("document pipeline/version switch: failed to resolve default chat model, falling back to no llm binding",
					zap.String("tenant_id", kb.TenantID), zap.Error(llmErr))
			}
			cleaned := service.ApplyComponentScopedParserConfig(cpDefaults, llmID)
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

	if repointVersion {
		var colValue interface{}
		if effectiveVersionID != nil {
			colValue = *effectiveVersionID
		}
		parserUpdates["pipeline_version_id"] = colValue
	}

	if present["meta_fields"] {
		// The metadata values live in the metadata store, not on the document row, so
		// the state being replaced is read from there. A store that is not wired
		// answers with nothing, and the request is then taken at its word: what was
		// just written is the change.
		beforeMetadata := s.documentMetadataForUpdate(ctx, doc.ID)
		if err = s.replaceDocumentMetadata(ctx, documentID, req.MetaFields); err != nil {
			return nil, common.CodeDataError, err
		}
		// An edit of the values is a version of its own: the store is not rebuilt by a
		// restore, so the timeline is what says whose metadata moved and when.
		if configValueChanged(beforeMetadata, req.MetaFields) && kb != nil {
			// The version is part of the edit: a change the timeline cannot name
			// fails the request instead of leaving values no restore can reach.
			if err := recordDocumentMetadataEditEE(ctx, kb.ID, doc, beforeMetadata, req.MetaFields); err != nil {
				return nil, common.CodeServerError, fmt.Errorf("failed to record the metadata history: %w", err)
			}
		}
	}
	if present["name"] && req.Name != nil && (doc.Name == nil || *req.Name != *doc.Name) {
		if err = s.updateDocumentNameOnly(ctx, doc, kb, *req.Name); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	// A save resends the settings form, so parserUpdates is filled even when
	// nothing moved. The write stays - it also reconciles the row with the current
	// pipeline template - but the version and the settings history below do not: a
	// timeline entry for an edit the user never made would fill the history with
	// no-ops, each one restorable to the state it already holds.
	//
	// What counts as a change is the request's own selection, its pinned pipeline
	// version, and the parameters it submitted. A parameter the request omits is
	// the rebuild's business, not the user's: the template fills it in.
	parserSettingsChanged := len(parserUpdates) > 0 &&
		(selectionChanged || versionChanged ||
			(hasConfig && parserConfigSubmittedChanged(doc.ParserConfig, req.ParserConfig)))
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
		return nil, common.CodeDataError, fmt.Errorf("database operation failed: %w", err)
	}

	if parserSettingsChanged {
		// The document's parse settings moved, which changes what its next parse
		// runs under, so the dataset timeline names it the same way a
		// dataset-level configuration change does. Best effort: the settings are
		// already written and a history row must not undo them.
		//
		// The settings themselves are recorded under that version, which is what a
		// restore reads to put them back. One record: an edit that lands in the
		// same millisecond as another still gets its own version number, so a
		// restore can tell the two apart instead of reading one overwritten row.
		if err := recordDocumentVersionSettingsTx(ctx, datasetID,
			[]entity.DatasetVersionChange{documentVersionChange(entity.DatasetVersionChangeActionUpdate, updatedDoc)},
			[]*entity.Document{updatedDoc}); err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to record the settings history: %w", err)
		}
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

func activeTaskStatusLabel(status string) string {
	switch status {
	case common.CREATED, common.SCHEDULED:
		return "Queued"
	case common.RUNNING, common.STOPPING:
		return "Parsing"
	default:
		return status
	}
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
			"document is currently %q and cannot be modified; stop parsing or wait for it to finish before updating its configuration", activeTaskStatusLabel(task.Status))
	case common.IsTerminalTaskStatus(task.Status):
		return common.CodeSuccess, nil
	default:
		return common.CodeDataError, fmt.Errorf("document has unrecognized task status %q", task.Status)
	}
}

func (s *DocumentService) validateDatasetDocumentUpdate(ctx context.Context, datasetID, documentID string, doc *entity.Document, req *UpdateDatasetDocumentRequest, present map[string]bool) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("invalid request payload")
	}

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

	if present["parse_type"] || present["parser_id"] || present["pipeline_id"] {
		if _, err := service.FromRequest(req.ParseType, req.ParserID, req.PipelineID); err != nil {
			return common.CodeDataError, err
		}
	}
	if present["name"] {
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
	if err := common.ValidateNameRunes(newName, common.NameLimit255); err != nil {
		return common.CodeArgumentError, err
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

// updateDocumentNameOnly renames a document and records the rename as a version
// of its own: a name is metadata, not content, so the timeline says who renamed
// which file while the document set is left alone (it is keyed on ids, and the
// names it carries are a snapshot of the moment it was taken).
func (s *DocumentService) updateDocumentNameOnly(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase, newName string) error {
	// Captured before the row moves: a version older than this rename has no
	// rename of its own to answer with, so this entry carries the name it replaced
	// and is what that older version reads (see datasetVersionDocumentNames).
	previousName := ""
	if doc.Name != nil {
		previousName = *doc.Name
	}
	if err := s.renameDocumentRow(ctx, doc, kb, newName); err != nil {
		return err
	}
	// Recorded after the row moved: the index title fields the rename also writes
	// are a mirror of it, and a failure there does not undo the rename.
	if kb != nil {
		if _, err := datasetversion.RecordContentVersionChangesOwnTx(
			ctx, kb.ID, entity.DatasetVersionKindDocumentRename,
			[]entity.DatasetVersionChange{{
				DocID:        doc.ID,
				Entity:       entity.DatasetVersionChangeEntityDocument,
				Action:       entity.DatasetVersionChangeActionRename,
				Name:         newName,
				PreviousName: previousName,
			}}); err != nil {
			return err
		}
	}
	return nil
}

// RenameDocumentForRestore puts a document back under the name it carried at the
// version being restored, and reports whether it moved.
//
// No version is recorded here: the restore that calls this records one version for
// the whole operation, and a rename version written mid-restore would describe a
// state the restore itself is undoing. A name another document of the dataset
// already holds is refused with ErrDocumentNameTaken - the caller keeps the current
// name rather than failing, because the content still came back.
func (s *DocumentService) RenameDocumentForRestore(ctx context.Context, datasetID, docID, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if datasetID == "" || docID == "" || name == "" || dao.DB == nil {
		return false, nil
	}
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The document did not come back (its row was gone), so there is no
			// row to rename.
			return false, nil
		}
		return false, fmt.Errorf("load document %s for its restore: %w", docID, err)
	}
	if doc == nil || doc.KbID != datasetID {
		return false, nil
	}
	if doc.Name != nil && *doc.Name == name {
		return false, nil
	}
	taken, err := s.documentDAO.NameExistsInKB(ctx, dao.DB, datasetID, name)
	if err != nil {
		return false, fmt.Errorf("check the restored name of %s: %w", docID, err)
	}
	if taken {
		return false, ErrDocumentNameTaken
	}
	kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		return false, fmt.Errorf("load dataset %s for a document rename: %w", datasetID, err)
	}
	if kb == nil {
		return false, nil
	}
	return true, s.renameDocumentRow(ctx, doc, kb, name)
}

// renameDocumentRow writes the name onto the document row, the file row it came
// from and the title fields of its chunks. It records nothing: a user rename wraps
// it with a version, a restore does not (it has one of its own).
func (s *DocumentService) renameDocumentRow(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase, newName string) error {
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{"name": newName}); err != nil {
		return fmt.Errorf("database error (Document rename): %w", err)
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
	docEngineType := engine.Type(s.docEngine)
	if doc.ChunkNum == 0 && (docEngineType == engine.EngineElasticsearch || docEngineType == engine.EngineOpenSearch) {
		exists, err := s.docEngine.ChunkStoreExists(ctx, indexName, doc.KbID)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}
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
		PipelineVersionID:    doc.PipelineVersionID,
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

// documentMetadataForUpdate reads the metadata values a save is about to replace.
//
// It answers nil when the metadata store is not wired or cannot be read: the caller
// then treats the submitted values as the change, rather than dropping the edit
// from the timeline. A store that is simply empty reads as an empty map, which is
// what makes "nothing moved" detectable.
func (s *DocumentService) documentMetadataForUpdate(ctx context.Context, docID string) map[string]interface{} {
	if s.docEngine == nil || s.metadataSvc == nil || !s.metadataSvc.Available() || docID == "" {
		return nil
	}
	meta, err := s.GetDocumentMetadataByID(ctx, docID)
	if err != nil {
		return nil
	}
	return meta
}

// documentNameOf is a document's display name, empty when the row carries none.
func documentNameOf(doc *entity.Document) string {
	if doc == nil || doc.Name == nil {
		return ""
	}
	return *doc.Name
}

// parserConfigSubmittedChanged reports whether the configuration a save carries
// differs from the one the document holds.
//
// Only the parameters the request names are compared. The settings form resends
// the values it shows and omits the rest, and the rebuild that follows fills every
// omitted parameter from the pipeline template; a template default arriving is not
// an edit the user made, so it must not append a version to the timeline.
//
// Values compare through their JSON form, so a number read back from storage as
// float64 equals the int the request carried, and an unset parameter (absent,
// null or empty) equals another unset one - the form renders all three as one
// empty field.
func parserConfigSubmittedChanged(stored entity.JSONMap, submitted map[string]any) bool {
	if len(submitted) == 0 {
		return false
	}
	return configValueChanged(stored, submitted)
}

// configValueChanged descends into the objects a parameter is nested in. A
// component the stored row does not carry reads as an empty one, so a component
// whose submitted values are all unset is not a change either.
func configValueChanged(stored any, submitted any) bool {
	submittedObject, submittedIsObject := submitted.(map[string]any)
	if !submittedIsObject {
		return configValueKey(stored) != configValueKey(submitted)
	}
	storedObject, storedIsObject := stored.(map[string]any)
	if !storedIsObject {
		if jsonMap, ok := stored.(entity.JSONMap); ok {
			storedObject, storedIsObject = jsonMap, true
		}
	}
	if !storedIsObject {
		storedObject = map[string]any{}
	}
	for key, value := range submittedObject {
		if configValueChanged(storedObject[key], value) {
			return true
		}
	}
	return false
}

// configValueKey renders one parameter for comparison: an unset value
// answers the empty key, everything else answers its JSON form.
func configValueKey(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok && text == "" {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
