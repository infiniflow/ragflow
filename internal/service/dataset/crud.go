package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// resolvePipelineVersionID determines which canvas version to bind for a
// dataset/document in Pipeline mode (design section 5 sticky snapshot). When a
// specific version is requested it must belong to the bound pipeline (ownership
// check, design T13); otherwise the latest version of the bound canvas is used.
// BuiltIn mode (no pipelineID) yields nil.
func resolvePipelineVersionID(ctx context.Context, db *gorm.DB, pipelineID *string, requestedVersionID *string) (*string, error) {
	if pipelineID == nil || strings.TrimSpace(*pipelineID) == "" {
		return nil, nil
	}
	canvasID := strings.TrimSpace(*pipelineID)
	if requestedVersionID != nil && strings.TrimSpace(*requestedVersionID) != "" {
		v, err := dao.NewUserCanvasVersionDAO().GetByID(ctx, db, strings.TrimSpace(*requestedVersionID))
		if err != nil {
			if errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
				return nil, fmt.Errorf("pipeline version %s not found: %w", *requestedVersionID, service.ErrPipelineVersionMissing)
			}
			return nil, fmt.Errorf("load pipeline version %s: %w", *requestedVersionID, err)
		}
		if v.UserCanvasID != canvasID {
			return nil, fmt.Errorf("pipeline version %s does not belong to canvas %s: %w", *requestedVersionID, canvasID, service.ErrPipelineVersionMissing)
		}
		return requestedVersionID, nil
	}
	if v, err := dao.NewUserCanvasVersionDAO().GetLatest(ctx, db, canvasID); err == nil && v != nil {
		return &v.ID, nil
	}
	return nil, nil
}

// CreateDataset validates the caller's tenant and parser settings before persisting a dataset.
func (d *DatasetService) CreateDataset(ctx context.Context, req *service.CreateDatasetRequest) (map[string]interface{}, common.ErrorCode, error) {
	userID := strings.TrimSpace(permission.UserIDFromContext(ctx))
	tenantID := strings.TrimSpace(permission.TenantIDFromContext(ctx))
	if tenantID == "" {
		tenantID = userID
	}
	if tenantID == "" || userID == "" {
		return nil, common.CodeUnauthorized, errors.New("user id and tenant id are required")
	}
	if !common.IsValidString(req.Name) {
		return nil, common.CodeDataError, errors.New("dataset name must be string")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, common.CodeDataError, errors.New("dataset name can't be empty")
	}
	if utf8.RuneCountInString(name) > entity.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("dataset name length is %d which is larger than %d", utf8.RuneCountInString(name), entity.DatasetNameLimit)
	}

	tenant, err := d.tenantDAO.GetByID(ctx, dao.DB, tenantID)
	if err != nil || tenant == nil {
		return nil, common.CodeDataError, errors.New("tenant not found")
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, tenantID, permission.TenantMember); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, "dataset", permission.ActionWrite); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}

	// parse_type is required on dataset creation: it explicitly selects BuiltIn
	// (1) or Pipeline (2) mode. This replaces the previous silent default to
	// BuiltIn when the field was omitted. FromRequest validates the
	// parse_type/parser_id/pipeline_id triple; Resolve enforces that a selection
	// is actually present (current is nil on create).
	sel, err := service.FromRequest(req.ParseType, req.ParserID, req.PipelineID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if _, err = service.Resolve(nil, sel); err != nil {
		return nil, common.CodeDataError, err
	}
	if sel.IsBuiltIn() && req.PipelineID != nil {
		req.PipelineID = nil
	}
	if sel.IsPipeline() && req.ParserID != nil {
		req.ParserID = nil
	}

	parserID := string(entity.ParserTypeGeneral)
	visibility := "me"
	embeddingModel := ""
	var language *string
	pipelineID := req.PipelineID

	if req.Permission != nil {
		visibility = strings.TrimSpace(*req.Permission)
		if visibility != "me" && visibility != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
	}
	if req.ParserID != nil {
		canonicalID, err := canonicalDatasetParserID(strings.TrimSpace(*req.ParserID))
		if err != nil {
			return nil, common.CodeDataError, err
		}
		parserID = canonicalID
		pipelineID = nil
	}
	if req.PipelineID != nil {
		normalizedPipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = normalizedPipelineID
		if pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
			parserID = ""
		}
	}
	if req.EmbeddingModel != nil {
		embeddingModel = strings.TrimSpace(*req.EmbeddingModel)
		if err = validateDatasetEmbeddingModel(embeddingModel); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if req.Language != nil {
		normalized, err := normalizeDatasetLanguage(*req.Language)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		language = &normalized
	}

	if pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
		if ok, err := canvasAccessibleForUser(ctx, strings.TrimSpace(*pipelineID)); err != nil {
			return nil, common.CodeServerError, err
		} else if !ok {
			return nil, common.CodeDataError, errors.New("canvas is not accessible")
		}
	}

	if req.ParserConfig != nil {
		dropped, err := ValidateParserConfig(req.ParserConfig)
		if len(dropped) > 0 {
			common.Warn("dropping unscoped (flat) parser_config keys; keys must be component-scoped (contain ':')",
				zap.Strings("keys", dropped),
				zap.String("tenant_id", tenantID),
			)
		}
		if err != nil {
			return nil, common.CodeArgumentError, err
		}
		if err := pipelinepkg.NormalizeParserConfigPages(req.ParserConfig); err != nil {
			return nil, common.CodeArgumentError, err
		}
	}
	isPipeline := pipelineID != nil && strings.TrimSpace(*pipelineID) != ""
	// design C5/G3: an empty version in Pipeline mode is no longer auto-backfilled
	// to the canvas latest; the Save is rejected so the UI can show the red Missing
	// badge and surface the "版本信息缺失" error. BuiltIn mode (no canvas bound) is
	// exempt.
	if isPipeline && (req.PipelineVersionID == nil || strings.TrimSpace(*req.PipelineVersionID) == "") {
		return nil, common.CodeBadRequest, service.ErrPipelineVersionEmpty
	}
	pipelineVersionID, verr := resolvePipelineVersionID(ctx, dao.DB, pipelineID, req.PipelineVersionID)
	if verr != nil {
		return nil, common.CodeBadRequest, verr
	}
	dslJSON, dslErr := service.LoadPipelineDSL(ctx, isPipeline, parserID, pipelineID, pipelineVersionID)
	parserConfig := entity.JSONMap{}
	if dslErr != nil {
		common.Warn("failed to load pipeline DSL for building parser_config",
			zap.String("parserID", parserID), zap.Error(dslErr))
	} else {
		var buildErr error
		parserConfig, buildErr = pipelinepkg.BuildParserConfig(dslJSON, req.ParserConfig)
		if buildErr != nil {
			return nil, common.CodeArgumentError, buildErr
		}
	}

	// Preserve the public default shape without overriding pipeline delimiters
	// unless the request explicitly configures parent_child.
	parentChild := resolveParentChild(req.ParserConfig)
	parentChildConfigured := parentChild != nil
	if parentChild == nil {
		parentChild = map[string]interface{}{
			"use_parent_child":   false,
			"children_delimiter": "\n",
		}
	}
	// Scope parent_child onto every chunker node (component-scoped); no flat key.
	for componentID, value := range parserConfig {
		if !pipelinepkg.IsChunkerComponent(componentID) {
			continue
		}
		params, ok := value.(map[string]interface{})
		if !ok {
			params = map[string]interface{}{}
			parserConfig[componentID] = params
		}
		params["parent_child"] = parentChild
	}

	parentChildConfig := map[string]interface{}{}
	if req.ParserConfig != nil {
		for componentID, value := range req.ParserConfig {
			if pipelinepkg.IsChunkerComponent(componentID) {
				parentChildConfig[componentID] = value
			}
		}
	}
	if parentChildConfigured {
		pipelinepkg.ApplyParentChildChunkerConfig(parserConfig, parentChildConfig)
	}

	var parserConfigMap map[string]interface{} = parserConfig

	modelFactory := service.NewModelFactory()
	modelCtx := permission.WithTenantID(ctx, tenantID)
	embdID, tenantEmbdID := "", ""
	if embeddingModel == "" {
		embeddingInfo, embeddingErr := modelFactory.ResolveDefaultInfo(modelCtx, entity.ModelTypeEmbedding)
		if embeddingErr == nil {
			embdID = service.ModelReference(embeddingInfo)
			tenantEmbdID = embeddingInfo.ID
		} else if !errors.Is(embeddingErr, service.ErrNoDefaultModel) {
			return nil, common.CodeServerError, embeddingErr
		}
	}
	if embeddingModel != "" {
		ok, message := d.verifyEmbeddingAvailability(ctx, embeddingModel, tenantID)
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		embdID = embeddingModel
		tenantEmbdID = ""
	}
	if embdID != "" && tenantEmbdID == "" {
		target, err := modelFactory.ResolveInfo(modelCtx, entity.ModelTypeEmbedding, embdID)
		if err == nil {
			tenantEmbdID = target.ID
		} else {
			return nil, common.CodeDataError, err
		}
	}

	kbID := common.GenerateToken()
	status := string(entity.StatusValid)
	// Fall back to a numbered name when the requested one is already taken.
	dedupedName, dedupeErr := d.dedupeDatasetName(ctx, name, tenantID)
	if dedupeErr != nil {
		return nil, common.CodeServerError, dedupeErr
	}
	name = dedupedName

	llmID := ""
	chatInfo, chatErr := modelFactory.ResolveDefaultInfo(modelCtx, entity.ModelTypeChat)
	if chatErr == nil {
		llmID = service.ModelReference(chatInfo)
	} else if !errors.Is(chatErr, service.ErrNoDefaultModel) {
		return nil, common.CodeServerError, chatErr
	}
	parserConfig = service.ApplyComponentScopedParserConfig(parserConfig, llmID)

	kb := &entity.Knowledgebase{
		ID:                kbID,
		PipelineVersionID: pipelineVersionID,
		Name:              name,
		UserID:            userID,
		TenantID:          tenantID,
		CreatedBy:         userID,
		ParserID:          parserID,
		PipelineID:        pipelineID,
		ParserConfig:      entity.JSONMap(parserConfigMap),
		Permission:        visibility,
		Language:          language,
		EmbdID:            embdID,
		TenantEmbdID:      stringPtrIfNotEmpty(tenantEmbdID),
		Status:            &status,
	}

	if err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := d.kbDAO.Create(ctx, tx, kb); err != nil {
			return err
		}
		if err := permission.NewDatabaseChecker(tx).GrantResourceOwner(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: kb.ID}); err != nil {
			return err
		}
		// Version 0 describes the dataset as created, so every later change has a
		// starting point to diff against and restore to.
		return d.seedDatasetBaselineVersionTxEE(ctx, tx, kb, userID)
	}); err != nil {
		if dao.IsDuplicateKeyErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("dataset name '%s' already exists", name)
		}
		// Surface the real underlying DB error instead of masking it. The
		// generic "failed to save dataset" message made schema/constraint
		// mismatches in the go scheme impossible to diagnose in CI.
		common.Error("failed to save dataset", err, zap.String("name", name), zap.String("tenant_id", tenantID))
		return nil, common.CodeServerError, fmt.Errorf("failed to save dataset: %w", err)
	}

	createdKB, err := d.kbDAO.GetByID(ctx, dao.DB, kbID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("load created dataset: %w", err)
	}
	if createdKB == nil {
		return nil, common.CodeServerError, errors.New("dataset created failed")
	}

	return datasetToMap(ctx, createdKB), common.CodeSuccess, nil
}

// dedupeDatasetName returns a non-colliding dataset name within the tenant,
// appending (1), (2), ... when the requested name is already taken.
func (d *DatasetService) dedupeDatasetName(ctx context.Context, name, tenantID string) (string, error) {
	return common.UniqueName(name, entity.DatasetNameLimit, func(candidate string) (bool, error) {
		return d.kbDAO.NameExists(ctx, dao.DB, tenantID, candidate)
	})
}

// GetDataset hides inaccessible datasets and enriches authorized results with size and connector data.
func (d *DatasetService) GetDataset(ctx context.Context, datasetID string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("lack of \"Dataset ID\"")
	}

	// Mirror Python's get_dataset: no UUID validation up front — any unknown
	// or malformed id simply fails the permission check.
	normalizedID, err := normalizeDatasetID(datasetID)
	if err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
		return nil, code, permissionErr
	}
	datasetID = normalizedID

	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, permission.OperationRead); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}

	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil || kb == nil {
		code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
		return nil, code, permissionErr
	}

	data := datasetToMap(ctx, kb)
	access, err := permission.NewDatabaseChecker(dao.DB).ResolveAccess(ctx, permission.ResourceRef{
		Kind: permission.ResourceKindDataset,
		ID:   kb.ID,
	})
	if err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}
	data["operator_permission"] = access.PermissionMask

	size, err := d.documentDAO.SumSizeByDatasetID(ctx, dao.DB, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
	}
	data["size"] = size

	connectors, err := d.connectorDAO.ListByDatasetID(ctx, dao.DB, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
	}
	data["connectors"] = datasetConnectorsOrEmpty(connectors)

	return data, common.CodeSuccess, nil
}

// DeleteDatasets limits deletion to authorized resources; delete-all remains tenant-scoped and owner-only.
func (d *DatasetService) DeleteDatasets(ctx context.Context, ids []string, deleteAll bool) (map[string]interface{}, common.ErrorCode, error) {
	userID := strings.TrimSpace(permission.UserIDFromContext(ctx))
	tenantID := strings.TrimSpace(permission.TenantIDFromContext(ctx))
	if tenantID == "" {
		tenantID = userID
	}
	if userID == "" {
		return nil, common.CodeUnauthorized, errors.New("user id is required")
	}

	normalizedIDs := make([]string, 0, len(ids))
	seenIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		normalizedID, err := normalizeDatasetID(id)
		if err != nil {
			return nil, common.CodeArgumentError, err
		}
		if _, seen := seenIDs[normalizedID]; seen {
			continue
		}
		seenIDs[normalizedID] = struct{}{}
		normalizedIDs = append(normalizedIDs, normalizedID)
	}

	checker := permission.NewDatabaseChecker(dao.DB)
	// Delete-all is tenant-scoped and remains an owner-only operation.
	if len(normalizedIDs) == 0 {
		if !deleteAll {
			return map[string]interface{}{"deleted": []string{}}, common.CodeSuccess, nil
		}
		if tenantID == "" {
			return nil, common.CodeUnauthorized, errors.New("tenant id is required to delete all datasets")
		}
		if err := checker.CheckTenant(ctx, tenantID, permission.TenantOwner); err != nil {
			code, permissionErr := permissionresponse.Normalize(err)
			return nil, code, permissionErr
		}
		kbs, err := d.kbDAO.Query(ctx, dao.DB, map[string]interface{}{"tenant_id": tenantID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
		}
		for _, kb := range kbs {
			normalizedIDs = append(normalizedIDs, kb.ID)
		}
	}

	refs := make([]permission.ResourceRef, 0, len(normalizedIDs))
	for _, id := range normalizedIDs {
		refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: id})
	}
	accessible, err := checker.FilterResources(ctx, refs, permission.OperationDelete)
	if err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}
	accessibleIDs := make(map[string]struct{}, len(accessible))
	for _, ref := range accessible {
		accessibleIDs[ref.ID] = struct{}{}
	}

	accessibleIDsInOrder := make([]string, 0, len(accessibleIDs))
	for _, id := range normalizedIDs {
		if _, ok := accessibleIDs[id]; ok {
			accessibleIDsInOrder = append(accessibleIDsInOrder, id)
		}
	}
	loaded, loadErr := service.LoadKBsInOrder(ctx, dao.DB, d.kbDAO, accessibleIDsInOrder)
	if loadErr != nil {
		return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", loadErr)
	}

	kbs := make([]*entity.Knowledgebase, 0, len(accessibleIDsInOrder))
	unauthorizedIDs := make([]string, 0)
	for i, kb := range loaded {
		if kb == nil {
			unauthorizedIDs = append(unauthorizedIDs, accessibleIDsInOrder[i])
			continue
		}
		kbs = append(kbs, kb)
	}
	for _, id := range normalizedIDs {
		if _, ok := accessibleIDs[id]; !ok {
			unauthorizedIDs = append(unauthorizedIDs, id)
		}
	}
	if len(unauthorizedIDs) > 0 {
		_, permissionErr := permissionresponse.Normalize(permission.ErrPermissionDenied)
		return nil, common.CodeForbidden, permissionErr
	}

	successCount := 0
	for _, kb := range kbs {
		plan, err := d.DeleteDatasetTx(ctx, dao.DB, kb)
		if err != nil {
			common.Warn("DeleteDatasetTx failed", zap.String("dataset", datasetNameAndID(kb)), zap.String("kb_id", kb.ID), zap.Error(err))
			return nil, common.CodeServerError, err
		}
		// dao.DB is a standalone handle: DeleteDatasetTx committed on return, so
		// it is now safe to remove the non-transactional external resources.
		d.CleanupDatasetExternal(ctx, plan)
		successCount++
	}

	return map[string]interface{}{
		"success_count": successCount,
		"errors":        []string{},
	}, common.CodeSuccess, nil
}

// DatasetCleanupPlan is the immutable snapshot of the external resources that
// belong to a dataset. DeleteDatasetTx collects it inside the DB transaction;
// the caller then passes it to CleanupDatasetExternal exactly once, after the
// handle has really committed. Discard the plan if the transaction rolls back.
type DatasetCleanupPlan struct {
	KBID         string
	TenantID     string
	DatasetName  string
	DocLocations []string
	DocIDs       []string
}

// DeleteDatasetTx deletes a dataset's metadata rows through the caller-supplied
// db handle and returns a snapshot of the external resources to clean up. The
// caller must invoke CleanupDatasetExternal after db has really committed:
// pass dao.DB for a standalone delete (committed on return), or, when db is a
// transaction handle, wait for the outer transaction to commit. If the caller
// rolls back, discard the returned plan — no external cleanup will have run,
// leaving storage/index data behind but never a row pointing at deleted data.
// Only DB mutations happen here; storage and engine cleanup are intentionally
// deferred so they cannot be observed by a rolled-back transaction.
func (d *DatasetService) DeleteDatasetTx(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase) (DatasetCleanupPlan, error) {
	var plan DatasetCleanupPlan
	datasetNameID := datasetNameAndID(kb)

	// Only the columns the cleanup loop reads (id, name, location) are fetched;
	// the full document row - including the potentially large parser_config - is
	// never loaded. Collect the snapshot inside the caller's handle so it is
	// consistent with the rows this transaction is about to delete.
	var documents []entity.Document
	if err := db.Select("id", "name", "location").Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
		return plan, fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
	}
	plan = DatasetCleanupPlan{KBID: kb.ID, TenantID: kb.TenantID, DatasetName: datasetNameID, DocIDs: extractDocIDs(documents)}
	for _, document := range documents {
		if document.Location == nil || *document.Location == "" {
			continue
		}
		plan.DocLocations = append(plan.DocLocations, *document.Location)
	}
	docIDs := plan.DocIDs

	if err := db.Transaction(func(tx *gorm.DB) error {
		// Delete index tasks referencing this KB.
		if taskIDs := datasetIndexTaskIDs(kb); len(taskIDs) > 0 {
			if err := tx.Where("id IN ?", taskIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
			}
		}

		if len(docIDs) > 0 {
			var mappings []entity.File2Document
			if err := tx.Where("document_id IN ?", docIDs).Find(&mappings).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
			}
			fileIDs := extractUniqueFileIDs(mappings)

			if err := tx.Where("doc_id IN ?", docIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&entity.File2Document{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
			}
			if len(fileIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ? AND source_type = ?", fileIDs, string(entity.FileSourceKnowledgebase)).Delete(&entity.File{}).Error; err != nil {
					return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
				}
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&entity.Document{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
			}
		}

		// Delete the KB folder file record.
		if err := tx.Unscoped().
			Where("source_type = ? AND type = ? AND name = ? AND tenant_id = ?",
				string(entity.FileSourceKnowledgebase), "folder", kb.Name, kb.TenantID).
			Delete(&entity.File{}).Error; err != nil {
			return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
		}

		if err := tx.Where("id = ?", kb.ID).Delete(&entity.Knowledgebase{}).Error; err != nil {
			return fmt.Errorf("delete dataset error for %s: %w", kb.ID, err)
		}
		// The history names a dataset that no longer exists, so nothing can
		// restore it; drop it in the same transaction.
		if err := d.deleteDatasetVersionsTxEE(ctx, tx, kb.ID); err != nil {
			return fmt.Errorf("delete dataset error for %s", kb.ID)
		}
		// The per-chunk history and index state of the removed documents have no
		// other collector: the removed-document cleanup only walks removal rows,
		// and a dataset delete never writes one.
		if err := deleteDatasetChunkRuntimeTxEE(ctx, tx, kb.ID); err != nil {
			return fmt.Errorf("delete dataset content history for %s: %w", kb.ID, err)
		}
		return nil
	}); err != nil {
		return plan, err
	}
	return plan, nil
}

// CleanupDatasetExternal removes the non-transactional resources of a deleted
// dataset: storage objects and the dataset bucket, engine chunks/metadata, and
// EE metadata states. Call this only after the handle behind plan has really
// committed. The operation is idempotent — already-missing objects and buckets
// are WARNed and skipped — so a caller may safely replay it. Failures are
// logged, not returned: the DB delete is already committed and cannot be undone.
func (d *DatasetService) CleanupDatasetExternal(ctx context.Context, plan DatasetCleanupPlan) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		common.Error("Dataset cleanup skipped: storage not initialized", nil, zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID))
		return
	}
	// Cleanup runs after the DB commit. Once the rows are gone there is no
	// retry path, so the cleanup must not be truncated by the request lifecycle:
	// a client disconnect cancels ctx, and the API write deadline (180 s) is
	// shorter than this budget. Detach from the request and give cleanup its own
	// full 5-minute window, matching the post-commit pattern in document/chunk.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	for _, location := range plan.DocLocations {
		exists, err := storageImpl.ObjectExists(cleanupCtx, plan.KBID, location)
		if err != nil {
			common.Error("Dataset cleanup could not check object", err, zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID), zap.String("location", location))
			continue
		}
		if !exists {
			common.Warn("Dataset document object already missing", zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID), zap.String("location", location))
			continue
		}
		if err := storageImpl.Remove(cleanupCtx, plan.KBID, location); err != nil {
			common.Error("Dataset cleanup could not remove object", err, zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID), zap.String("location", location))
			continue
		}
		common.Info("Removed dataset document object", zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID), zap.String("location", location))
	}
	exists, err := storageImpl.BucketExistsWithError(cleanupCtx, plan.KBID)
	if err != nil {
		common.Error("Dataset cleanup could not check bucket", err, zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID))
	} else if !exists {
		common.Warn("Dataset bucket already missing", zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID))
	} else if err := storageImpl.RemoveBucket(cleanupCtx, plan.KBID); err != nil {
		common.Error("Dataset cleanup could not remove bucket", err, zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID))
	}
	if len(plan.DocIDs) > 0 {
		d.deleteDatasetEngineData(cleanupCtx, &entity.Knowledgebase{ID: plan.KBID, TenantID: plan.TenantID}, plan.DocIDs)
	}
	// The metadata values of the removed documents live in the document engine,
	// which the DB transaction cannot reach: drop them once the rows that named
	// them are committed away.
	deleteDatasetMetadataStatesEE(cleanupCtx, plan.KBID)
	common.Info("Deleted dataset", zap.String("dataset", plan.DatasetName), zap.String("kb_id", plan.KBID))
}

func datasetNameAndID(kb *entity.Knowledgebase) string {
	if kb.Name == "" {
		return kb.ID
	}
	return fmt.Sprintf("%s (%s)", kb.Name, kb.ID)
}

// ListDatasets applies resource permissions before pagination and optional canvas filtering.
func (d *DatasetService) ListDatasets(ctx context.Context, id, name string, page, pageSize int, terms []dao.OrderTerm, keywords string, ownerIDs []string, accessUserID, parserID string, ids []string, canvasID string) ([]map[string]interface{}, int64, common.ErrorCode, error) {
	id = strings.TrimSpace(id)
	canvasID = strings.TrimSpace(canvasID)
	accessUserID = strings.TrimSpace(accessUserID)
	if id != "" && len(ids) > 0 {
		return nil, 0, common.CodeDataError, fmt.Errorf("should not provide both 'id':%s and 'ids':%s", id, pythonStringListRepr(ids))
	}
	if id != "" {
		normalizedID, err := normalizeDatasetID(id)
		if err != nil {
			return nil, 0, common.CodeArgumentError, err
		}
		id = normalizedID
	}

	name = strings.TrimSpace(name)

	if page <= 0 {
		page = common.DefaultPage
	}
	if pageSize <= 0 {
		pageSize = common.DefaultPageSize
	}

	terms = keepDatasetOrderTerms(terms)

	keywords = strings.TrimSpace(keywords)
	parserID = strings.TrimSpace(parserID)

	filteredOwnerIDs := make([]string, 0, len(ownerIDs))
	seenOwnerIDs := make(map[string]struct{}, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID == "" {
			continue
		}
		if _, exists := seenOwnerIDs[ownerID]; exists {
			continue
		}
		seenOwnerIDs[ownerID] = struct{}{}
		filteredOwnerIDs = append(filteredOwnerIDs, ownerID)
	}

	var kbs []*entity.KnowledgebaseListItem
	var total int64
	var deniedIDs []string
	operatorPermissions := make(map[string]int32)
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		checker := permission.NewDatabaseChecker(tx)
		accessCtx, hasAccessUser := accessUserContext(ctx, accessUserID)
		operation := permission.OperationRead
		var entry *permission.ResourceRef
		if canvasID != "" {
			operation = permission.OperationUse
			entry = &permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: canvasID}
		}
		checkAccessAs := func(checkCtx context.Context, datasetID string) error {
			dependency := permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}
			if entry == nil {
				return checker.CheckResource(checkCtx, dependency, operation)
			}
			return checker.CheckDependency(checkCtx, *entry, dependency, permission.OperationRun, operation)
		}
		checkAccess := func(datasetID string) error {
			if err := checkAccessAs(ctx, datasetID); err != nil {
				return err
			}
			if hasAccessUser {
				return checkAccessAs(accessCtx, datasetID)
			}
			return nil
		}
		listQuery := tx.WithContext(ctx).Model(&entity.Knowledgebase{})
		var resourceIDs []string

		switch {
		case id != "":
			checkErr := checkAccess(id)
			if checkErr != nil {
				return checkErr
			}
			listQuery = listQuery.Where("knowledgebase.id = ?", id)
		case len(ids) > 0:
			resourceIDs = make([]string, 0, len(ids))
			for _, datasetID := range ids {
				checkErr := checkAccess(datasetID)
				if checkErr == nil {
					resourceIDs = append(resourceIDs, datasetID)
					continue
				}
				if errors.Is(checkErr, permission.ErrPermissionDenied) ||
					errors.Is(checkErr, permission.ErrMembershipNotFound) ||
					errors.Is(checkErr, permission.ErrResourceNotFound) {
					deniedIDs = append(deniedIDs, datasetID)
					continue
				}
				return checkErr
			}
			if len(resourceIDs) == 0 {
				listQuery = listQuery.Where("1 = 0")
			} else {
				listQuery = listQuery.Where("knowledgebase.id IN ?", resourceIDs)
			}
		default:
			scopeQuery := permission.ScopeQuery{Kind: permission.ResourceKindDataset, Operation: operation}
			if entry != nil {
				scopeQuery.Entry = entry
				scopeQuery.EntryOp = permission.OperationRun
			}
			scope, checkErr := checker.QueryScope(ctx, scopeQuery)
			if checkErr != nil {
				return checkErr
			}
			listQuery = scope.Apply(listQuery)
			if hasAccessUser {
				accessScope, checkErr := checker.QueryScope(accessCtx, scopeQuery)
				if checkErr != nil {
					if errors.Is(checkErr, permission.ErrPermissionDenied) || errors.Is(checkErr, permission.ErrMembershipNotFound) || errors.Is(checkErr, permission.ErrResourceNotFound) {
						listQuery = listQuery.Where("1 = 0")
					} else {
						return checkErr
					}
				} else {
					listQuery = accessScope.Apply(listQuery)
				}
			}
		}
		listed, count, err := d.kbDAO.ListByQuery(ctx, listQuery, filteredOwnerIDs, page, pageSize, terms, keywords, parserID, name)
		if err != nil {
			return err
		}
		kbs, total = listed, count
		for _, kb := range kbs {
			if kb == nil {
				continue
			}
			access, accessErr := checker.ResolveAccess(ctx, permission.ResourceRef{
				Kind: permission.ResourceKindDataset,
				ID:   kb.ID,
			})
			if accessErr != nil {
				return accessErr
			}
			operatorPermissions[kb.ID] = access.PermissionMask
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, permission.ErrUnauthenticated) || errors.Is(err, permission.ErrPermissionDenied) ||
			errors.Is(err, permission.ErrMembershipNotFound) || errors.Is(err, permission.ErrResourceNotFound) {
			code, permissionErr := permissionresponse.Normalize(err)
			return nil, 0, code, permissionErr
		}
		return nil, 0, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
	}
	if len(deniedIDs) > 0 {
		common.Warn("User lacks permission for datasets",
			zap.String("user_id", permission.UserIDFromContext(ctx)),
			zap.Strings("dataset_ids", common.Deduplicate(deniedIDs)),
		)
	}

	data := make([]map[string]interface{}, 0, len(kbs))
	modelNameCache := make(map[string]string)
	// Resolve the latest version id per canvas once for the whole page so the
	// per-row outdated flag does not issue a GetLatest per dataset (N+1).
	latestVersionByCanvas := latestVersionIDsByCanvas(ctx, kbs)
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		item := datasetListItemToMap(latestVersionByCanvas, kb)
		item["operator_permission"] = operatorPermissions[kb.ID]
		// Mirror the memory list: surface the concrete model display name
		// (modelName@instance@provider) instead of a raw tenant_model ID.
		tenantEmbdID := ptrStringValue(kb.TenantEmbdID)
		if tenantEmbdID == "" && isHexID(kb.EmbdID) {
			tenantEmbdID = kb.EmbdID
		}
		item["embedding_model"] = service.ResolveTenantModelDisplayName(ctx, dao.DB, tenantEmbdID, kb.EmbdID, modelNameCache)
		data = append(data, item)
	}
	return data, total, common.CodeSuccess, nil
}

// ListDatasetFilters restricts owner choices to datasets the caller may read or use from the given canvas.
func (d *DatasetService) ListDatasetFilters(ctx context.Context, canvasID string) (map[string]interface{}, common.ErrorCode, error) {
	owners := make([]*entity.DatasetOwnerFilter, 0)
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		query := permission.ScopeQuery{Kind: permission.ResourceKindDataset, Operation: permission.OperationRead}
		if canvasID = strings.TrimSpace(canvasID); canvasID != "" {
			query.Operation = permission.OperationUse
			query.Entry = &permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: canvasID}
			query.EntryOp = permission.OperationRun
		}
		scope, err := permission.NewDatabaseChecker(tx).Scope(ctx, query)
		if err != nil {
			return err
		}
		if scope.Mode == permission.ScopeNone {
			return nil
		}
		owners, err = d.kbDAO.GetOwnerFilterByResourceIDs(ctx, tx, scope.ResourceIDs)
		return err
	})
	if err != nil {
		if errors.Is(err, permission.ErrUnauthenticated) || errors.Is(err, permission.ErrPermissionDenied) ||
			errors.Is(err, permission.ErrMembershipNotFound) || errors.Is(err, permission.ErrResourceNotFound) {
			code, permissionErr := permissionresponse.Normalize(err)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
	}

	var total int64
	for _, owner := range owners {
		if owner != nil {
			total += owner.Count
		}
	}

	return map[string]interface{}{
		"filter": map[string]interface{}{
			"owner": owners,
		},
		"total": total,
	}, common.CodeSuccess, nil
}

// ptrStringValue safely dereferences a *string.
func ptrStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// stringPtrIfNotEmpty returns a pointer to s if s is non-empty.
func stringPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// extractDocIDs returns the document IDs from a slice of documents.
// datasetIndexTaskIDs returns the deduplicated set of dataset-level index task
// ids recorded on the KB (graphrag/raptor/mindmap legacy task fields). It is
// used by deleteDataset to clear residual entity.Task rows when a KB is deleted.
// Kept here because it belongs to the dataset delete lifecycle, not the retired
// RunIndex scheduling path.
func datasetIndexTaskIDs(kb *entity.Knowledgebase) []string {
	taskIDs := make([]string, 0, 3)
	for _, taskID := range []*string{kb.GraphragTaskID, kb.RaptorTaskID, kb.MindmapTaskID} {
		if taskID != nil && *taskID != "" {
			taskIDs = append(taskIDs, *taskID)
		}
	}
	return common.Deduplicate(taskIDs)
}

func extractDocIDs(docs []entity.Document) []string {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	return ids
}

// deleteDatasetEngineData cleans up engine-level chunks and metadata for all
// documents in a dataset being deleted. Called before the DB transaction
// because engine operations are not transactional.
func (d *DatasetService) deleteDatasetEngineData(ctx context.Context, kb *entity.Knowledgebase, docIDs []string) {
	if d.docEngine == nil || len(docIDs) == 0 {
		return
	}
	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)

	if _, err := d.docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": docIDs}, indexName, kb.ID); err != nil {
		common.Logger.Warn(fmt.Sprintf("deleteDataset: failed to delete chunks for kb %s: %v", kb.ID, err))
	}
	if _, err := d.docEngine.DeleteMetadata(ctx, map[string]interface{}{"kb_id": kb.ID}, kb.TenantID); err != nil {
		common.Logger.Warn(fmt.Sprintf("deleteDataset: failed to delete metadata for kb %s: %v", kb.ID, err))
	}
	// Infinity keeps one chunk table per dataset. Dropping the dataset must
	// drop the table too, otherwise it leaks forever. Other engines share
	// their store, so this is Infinity-only.
	if d.docEngine.GetType() == string(engine.EngineInfinity) {
		exists, err := d.docEngine.ChunkStoreExists(ctx, indexName, kb.ID)
		if err != nil {
			common.Logger.Warn(fmt.Sprintf("deleteDataset: failed to check chunk table for kb %s: %v", kb.ID, err))
		} else if exists {
			if err := d.docEngine.DropChunkStore(ctx, indexName, kb.ID); err != nil {
				common.Logger.Warn(fmt.Sprintf("deleteDataset: failed to drop chunk table for kb %s: %v", kb.ID, err))
			}
		}
	}
}

// extractUniqueFileIDs returns deduplicated, non-empty file IDs from
// file2document mappings. The extraction loop skips nil/empty FileIDs; the
// deduplication is delegated to the canonical common.Deduplicate, which preserves
// first-occurrence order and returns a non-nil empty slice for an all-skip input.
func extractUniqueFileIDs(mappings []entity.File2Document) []string {
	ids := make([]string, 0, len(mappings))
	for _, m := range mappings {
		if m.FileID == nil || *m.FileID == "" {
			continue
		}
		ids = append(ids, *m.FileID)
	}
	return common.Deduplicate(ids)
}
