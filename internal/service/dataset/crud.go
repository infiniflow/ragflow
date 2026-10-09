package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (d *DatasetService) CreateDataset(ctx context.Context, req *service.CreateDatasetRequest, subject permission.Subject) (map[string]interface{}, common.ErrorCode, error) {
	tenantID := strings.TrimSpace(subject.TenantID)
	userID := strings.TrimSpace(subject.UserID)
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
	if len(name) > entity.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), entity.DatasetNameLimit)
	}

	tenant, err := d.tenantDAO.GetByID(ctx, dao.DB, tenantID)
	if err != nil || tenant == nil {
		return nil, common.CodeDataError, errors.New("tenant not found")
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, subject, tenantID, permission.TenantMember); err != nil {
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
	permission := "me"
	embeddingModel := ""
	var language *string
	pipelineID := req.PipelineID

	if req.Permission != nil {
		permission = strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
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
		if ok, err := canvasAccessibleForUser(ctx, userID, strings.TrimSpace(*pipelineID)); err != nil {
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
	dslJSON, dslErr := service.LoadPipelineDSL(ctx, isPipeline, parserID, pipelineID)
	parserConfig := entity.JSONMap{}
	if dslErr != nil {
		common.Warn("failed to load pipeline DSL for building parser_config",
			zap.String("parserID", parserID), zap.Error(dslErr))
	} else {
		parserConfig = pipelinepkg.BuildParserConfig(dslJSON, req.ParserConfig)
	}

	// Preserve the public default shape when parser_config is empty. The
	// parent_child block remains the single source of truth; chunker
	// children_delimiters are derived below only when it is configured.
	parentChild := resolveParentChild(req.ParserConfig)
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
	pipelinepkg.ApplyParentChildChunkerConfig(parserConfig, parentChildConfig)

	var parserConfigMap map[string]interface{} = parserConfig

	embdID := tenant.EmbdID
	tenantEmbdID := ptrStringValue(tenant.TenantEmbdID)
	if embeddingModel != "" {
		ok, message := d.verifyEmbeddingAvailability(ctx, embeddingModel, service.ModelAccess{UserID: userID, TenantID: tenantID})
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		embdID = embeddingModel
		tenantEmbdID = ""
	}
	if embdID != "" && tenantEmbdID == "" {
		target, err := service.NewModelFactory().ResolveInfo(ctx, service.ModelAccess{UserID: userID, TenantID: tenantID}, entity.ModelTypeEmbedding, embdID)
		if err == nil {
			tenantEmbdID = target.ID
		} else {
			return nil, common.CodeDataError, err
		}
	}

	kbID := utility.GenerateToken()
	status := string(entity.StatusValid)
	// Mirror Python's duplicate_name: append (1), (2), ... until the name is
	// unique within the tenant.
	name = d.dedupeDatasetName(ctx, name, tenantID)

	parserConfig = service.ApplyComponentScopedParserConfig(
		parserConfig,
		tenant.LLMID,
	)

	kb := &entity.Knowledgebase{
		ID:           kbID,
		Name:         name,
		TenantID:     tenantID,
		CreatedBy:    userID,
		ParserID:     parserID,
		PipelineID:   pipelineID,
		ParserConfig: entity.JSONMap(parserConfigMap),
		Permission:   permission,
		Language:     language,
		EmbdID:       embdID,
		TenantEmbdID: stringPtrIfNotEmpty(tenantEmbdID),
		Status:       &status,
	}

	if err = d.kbDAO.Create(ctx, dao.DB, kb); err != nil {
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
	if err != nil || createdKB == nil {
		return nil, common.CodeServerError, errors.New("dataset created failed")
	}

	return datasetToMap(createdKB), common.CodeSuccess, nil
}

// dedupeDatasetName mirrors Python's duplicate_name: if the name already
// exists within the tenant, append (1), (2), ... until it is unique.
func (d *DatasetService) dedupeDatasetName(ctx context.Context, name, tenantID string) string {
	candidate := name
	for i := 1; i < 1000; i++ {
		existing, err := d.kbDAO.GetByName(ctx, dao.DB, candidate, tenantID)
		if err != nil || existing == nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s(%d)", name, i)
	}
	return candidate
}

func (d *DatasetService) GetDataset(ctx context.Context, datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
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

	if err := d.CheckAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationRead); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}

	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil || kb == nil {
		code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
		return nil, code, permissionErr
	}

	data := datasetToMap(kb)

	size, err := d.documentDAO.SumSizeByDatasetID(ctx, dao.DB, datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	data["size"] = size

	connectors, err := d.connectorDAO.ListByDatasetID(ctx, dao.DB, datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	data["connectors"] = datasetConnectorsOrEmpty(connectors)

	return data, common.CodeSuccess, nil
}

func (d *DatasetService) DeleteDatasets(ctx context.Context, ids []string, deleteAll bool, subject permission.Subject) (map[string]interface{}, common.ErrorCode, error) {
	subject.UserID = strings.TrimSpace(subject.UserID)
	subject.TenantID = strings.TrimSpace(subject.TenantID)
	if subject.UserID == "" {
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
		if subject.TenantID == "" {
			return nil, common.CodeUnauthorized, errors.New("tenant id is required to delete all datasets")
		}
		if err := checker.CheckTenant(ctx, subject, subject.TenantID, permission.TenantOwner); err != nil {
			code, permissionErr := permissionresponse.Normalize(err)
			return nil, code, permissionErr
		}
		kbs, err := d.kbDAO.Query(ctx, dao.DB, map[string]interface{}{"tenant_id": subject.TenantID})
		if err != nil {
			return nil, common.CodeServerError, errors.New("database operation failed")
		}
		for _, kb := range kbs {
			normalizedIDs = append(normalizedIDs, kb.ID)
		}
	}

	refs := make([]permission.ResourceRef, 0, len(normalizedIDs))
	for _, id := range normalizedIDs {
		refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: id})
	}
	accessible, err := checker.FilterResources(ctx, subject, refs, permission.OperationDelete)
	if err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}
	accessibleIDs := make(map[string]struct{}, len(accessible))
	for _, ref := range accessible {
		accessibleIDs[ref.ID] = struct{}{}
	}

	kbs := make([]*entity.Knowledgebase, 0, len(normalizedIDs))
	unauthorizedIDs := make([]string, 0)
	for _, id := range normalizedIDs {
		if _, ok := accessibleIDs[id]; !ok {
			unauthorizedIDs = append(unauthorizedIDs, id)
			continue
		}
		kb, err := d.kbDAO.GetByID(ctx, dao.DB, id)
		if err != nil || kb == nil {
			unauthorizedIDs = append(unauthorizedIDs, id)
			continue
		}
		kbs = append(kbs, kb)
	}
	if len(unauthorizedIDs) > 0 {
		_, permissionErr := permissionresponse.Normalize(permission.ErrPermissionDenied)
		return nil, common.CodeForbidden, permissionErr
	}

	successCount := 0
	for _, kb := range kbs {
		if err := d.deleteDataset(ctx, kb); err != nil {
			common.Warn("deleteDataset failed", zap.String("dataset", datasetNameAndID(kb)), zap.String("kb_id", kb.ID), zap.Error(err))
			return nil, common.CodeServerError, err
		}
		successCount++
	}

	return map[string]interface{}{
		"success_count": successCount,
		"errors":        []string{},
	}, common.CodeSuccess, nil
}

func (d *DatasetService) deleteDataset(ctx context.Context, kb *entity.Knowledgebase) error {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return fmt.Errorf("storage not initialized")
	}
	datasetNameID := datasetNameAndID(kb)

	// Collect document IDs first so engine cleanup can run before the
	// transaction (engine ops are not transactional).
	var documents []entity.Document
	if err := dao.DB.Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
		return fmt.Errorf("delete dataset error for %s", kb.ID)
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	for _, document := range documents {
		if document.Location == nil || *document.Location == "" {
			continue
		}
		docNameID := documentNameAndID(document)
		exists, err := storageImpl.ObjectExists(cleanupCtx, kb.ID, *document.Location)
		if err != nil {
			return fmt.Errorf("check document %s in dataset %s: %w", document.ID, kb.ID, err)
		}
		if !exists {
			common.Warn("Dataset document object already missing", zap.String("document", docNameID), zap.String("document_id", document.ID), zap.String("dataset", datasetNameID), zap.String("bucket", kb.ID))
			continue
		}
		if err := storageImpl.Remove(cleanupCtx, kb.ID, *document.Location); err != nil {
			return fmt.Errorf("remove document %s from dataset %s: %w", docNameID, kb.ID, err)
		}
		common.Info("Removed dataset document object", zap.String("document", docNameID), zap.String("document_id", document.ID), zap.String("dataset", datasetNameID), zap.String("bucket", kb.ID))
	}
	exists, err := storageImpl.BucketExistsWithError(cleanupCtx, kb.ID)
	if err != nil {
		return fmt.Errorf("check dataset bucket %s: %w", kb.ID, err)
	}
	if !exists {
		common.Warn("Dataset bucket already missing", zap.String("dataset", datasetNameID), zap.String("bucket", kb.ID))
	} else {
		if err := storageImpl.RemoveBucket(cleanupCtx, kb.ID); err != nil {
			return fmt.Errorf("remove dataset bucket for dataset %s: %w", datasetNameID, err)
		}
		common.Info("Removed dataset bucket", zap.String("dataset", datasetNameID), zap.String("bucket", kb.ID))
	}
	docIDs := extractDocIDs(documents)
	if len(docIDs) > 0 {
		d.deleteDatasetEngineData(ctx, kb, docIDs)
	}

	if err := dao.DB.Transaction(func(tx *gorm.DB) error {
		// Delete index tasks referencing this KB.
		if taskIDs := datasetIndexTaskIDs(kb); len(taskIDs) > 0 {
			if err := tx.Where("id IN ?", taskIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s", kb.ID)
			}
		}

		if len(docIDs) > 0 {
			var mappings []entity.File2Document
			if err := tx.Where("document_id IN ?", docIDs).Find(&mappings).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s", kb.ID)
			}
			fileIDs := extractUniqueFileIDs(mappings)

			if err := tx.Where("doc_id IN ?", docIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s", kb.ID)
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&entity.File2Document{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s", kb.ID)
			}
			if len(fileIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ? AND source_type = ?", fileIDs, string(entity.FileSourceKnowledgebase)).Delete(&entity.File{}).Error; err != nil {
					return fmt.Errorf("delete dataset error for %s", kb.ID)
				}
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&entity.Document{}).Error; err != nil {
				return fmt.Errorf("delete dataset error for %s", kb.ID)
			}
		}

		// Delete the KB folder file record.
		if err := tx.Unscoped().
			Where("source_type = ? AND type = ? AND name = ? AND tenant_id = ?",
				string(entity.FileSourceKnowledgebase), "folder", kb.Name, kb.TenantID).
			Delete(&entity.File{}).Error; err != nil {
			return fmt.Errorf("delete dataset error for %s", kb.ID)
		}

		if err := tx.Where("id = ?", kb.ID).Delete(&entity.Knowledgebase{}).Error; err != nil {
			return fmt.Errorf("delete dataset error for %s", kb.ID)
		}
		return nil
	}); err != nil {
		return err
	}
	common.Info("Deleted dataset", zap.String("dataset", datasetNameID), zap.String("kb_id", kb.ID))
	return nil
}

func documentNameAndID(document entity.Document) string {
	if document.Name == nil || *document.Name == "" {
		return document.ID
	}
	return fmt.Sprintf("%s (%s)", *document.Name, document.ID)
}

func datasetNameAndID(kb *entity.Knowledgebase) string {
	if kb.Name == "" {
		return kb.ID
	}
	return fmt.Sprintf("%s (%s)", kb.Name, kb.ID)
}

func (d *DatasetService) ListDatasets(ctx context.Context, id, name string, page, pageSize int, terms []dao.OrderTerm, keywords string, ownerIDs []string, parserID, userID string, ids []string) ([]map[string]interface{}, int64, common.ErrorCode, error) {
	id = strings.TrimSpace(id)
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
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
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
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		checker := permission.NewDatabaseChecker(tx)
		subject := permission.Subject{UserID: userID}
		var resourceIDs []string

		switch {
		case id != "":
			checkErr := checker.CheckResource(ctx, subject, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: id}, permission.OperationRead)
			if checkErr != nil {
				return checkErr
			}
			resourceIDs = []string{id}
		case len(ids) > 0:
			refs := make([]permission.ResourceRef, 0, len(ids))
			for _, datasetID := range ids {
				refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID})
			}
			accessible, checkErr := checker.FilterResources(ctx, subject, refs, permission.OperationRead)
			if checkErr != nil {
				return checkErr
			}
			accessibleIDs := make(map[string]struct{}, len(accessible))
			for _, ref := range accessible {
				accessibleIDs[ref.ID] = struct{}{}
			}
			resourceIDs = make([]string, 0, len(accessibleIDs))
			for _, datasetID := range ids {
				if _, ok := accessibleIDs[datasetID]; ok {
					resourceIDs = append(resourceIDs, datasetID)
				} else {
					deniedIDs = append(deniedIDs, datasetID)
				}
			}
		default:
			scope, checkErr := checker.Scope(ctx, subject, permission.ScopeQuery{
				Kind: permission.ResourceKindDataset, Operation: permission.OperationRead,
			})
			if checkErr != nil {
				return checkErr
			}
			if scope.Mode == permission.ScopeIDs {
				resourceIDs = scope.ResourceIDs
			}
		}

		if len(resourceIDs) == 0 {
			return nil
		}
		listed, count, err := d.kbDAO.ListByResourceIDs(ctx, tx, resourceIDs, filteredOwnerIDs, page, pageSize, terms, keywords, parserID, name)
		if err != nil {
			return err
		}
		kbs, total = listed, count
		return nil
	})
	if err != nil {
		if errors.Is(err, permission.ErrUnauthenticated) || errors.Is(err, permission.ErrPermissionDenied) ||
			errors.Is(err, permission.ErrMembershipNotFound) || errors.Is(err, permission.ErrResourceNotFound) {
			code, permissionErr := permissionresponse.Normalize(err)
			return nil, 0, code, permissionErr
		}
		return nil, 0, common.CodeServerError, errors.New("database operation failed")
	}
	if len(deniedIDs) > 0 {
		common.Warn("User lacks permission for datasets",
			zap.String("user_id", userID),
			zap.Strings("dataset_ids", common.Deduplicate(deniedIDs)),
		)
	}

	data := make([]map[string]interface{}, 0, len(kbs))
	modelNameCache := make(map[string]string)
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		item := datasetListItemToMap(kb)
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

func (d *DatasetService) ListDatasetFilters(ctx context.Context, userID string) (map[string]interface{}, common.ErrorCode, error) {
	var owners []*entity.DatasetOwnerFilter
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		scope, err := permission.NewDatabaseChecker(tx).Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
			Kind: permission.ResourceKindDataset, Operation: permission.OperationRead,
		})
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
		return nil, common.CodeServerError, errors.New("database operation failed")
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
	if _, err := d.docEngine.DeleteMetadata(ctx, map[string]interface{}{"doc_id": docIDs}, kb.TenantID); err != nil {
		common.Logger.Warn(fmt.Sprintf("deleteDataset: failed to delete metadata for kb %s: %v", kb.ID, err))
	}
}

// extractUniqueFileIDs returns deduplicated, non-empty file IDs from
// file2document mappings.
func extractUniqueFileIDs(mappings []entity.File2Document) []string {
	ids := make([]string, 0, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, m := range mappings {
		if m.FileID == nil || *m.FileID == "" {
			continue
		}
		if _, exists := seen[*m.FileID]; exists {
			continue
		}
		seen[*m.FileID] = struct{}{}
		ids = append(ids, *m.FileID)
	}
	return ids
}
