package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (d *DatasetService) CreateDataset(req *service.CreateDatasetRequest, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	if !common.IsValidString(req.Name) {
		return nil, common.CodeDataError, errors.New("Dataset name must be string.")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
	}
	if len(name) > entity.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), entity.DatasetNameLimit)
	}

	tenant, err := d.tenantDAO.GetByID(tenantID)
	if err != nil || tenant == nil {
		return nil, common.CodeDataError, errors.New("Tenant not found.")
	}

	isPipelineMode := req.ParseType != nil && *req.ParseType == 2
	isBuiltinMode := req.ParseType != nil && *req.ParseType == 1

	if isBuiltinMode && req.PipelineID != nil {
		req.PipelineID = nil
	}
	if isPipelineMode && req.ParserID != nil {
		req.ParserID = nil
	}

	if req.ParseType == nil && req.ParserID != nil && req.PipelineID != nil {
		return nil, common.CodeDataError, errors.New("parser_id and pipeline_id are mutually exclusive")
	}

	parserID := ""
	permission := "me"
	embeddingModel := ""
	pipelineID := req.PipelineID

	if req.Permission != nil {
		permission = strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
	}
	if req.ParserID != nil {
		parserID = strings.TrimSpace(*req.ParserID)
		if err := validateParserID(parserID); err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = nil
	}
	if req.PipelineID != nil {
		normalizedPipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = normalizedPipelineID
	}
	if req.EmbeddingModel != nil {
		embeddingModel = strings.TrimSpace(*req.EmbeddingModel)
		if err := validateDatasetEmbeddingModel(embeddingModel); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
		if ok, err := canvasAccessibleForUser(tenantID, strings.TrimSpace(*pipelineID)); err != nil {
			return nil, common.CodeServerError, err
		} else if !ok {
			return nil, common.CodeDataError, errors.New("canvas is not accessible")
		}
	}

	parserConfig, cpErr := service.ResolveComponentParamsDefaults(parserID, pipelineID)
	if cpErr != nil {
		common.Warn("failed to resolve component params defaults for dataset",
			zap.String("parserID", parserID), zap.Error(cpErr))
		parserConfig = entity.JSONMap{}
	}

	var parserConfigMap map[string]interface{} = parserConfig

	embdID := tenant.EmbdID
	tenantEmbdID := ptrStringValue(tenant.TenantEmbdID)
	if embeddingModel != "" {
		ok, message := d.verifyEmbeddingAvailability(embeddingModel, tenantID)
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		embdID = embeddingModel
		tenantEmbdID = ""
	}
	if embdID != "" && tenantEmbdID == "" {
		resolvedID, err := service.NewModelProviderService().ResolveModelID(tenantID, entity.ModelTypeEmbedding, embdID)
		if err == nil {
			tenantEmbdID = resolvedID
		} else {
			return nil, common.CodeDataError, err
		}
	}

	kbID := utility.GenerateToken()
	status := string(entity.StatusValid)
	// Reject duplicate name within tenant to match the established API contract.
	existing, err := d.kbDAO.GetByName(name, tenantID)
	if err != nil && !dao.IsNotFoundErr(err) {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	if existing != nil {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name '%s' already exists", name)
	}

	kb := &entity.Knowledgebase{
		ID:           kbID,
		Name:         name,
		TenantID:     tenantID,
		CreatedBy:    tenantID,
		ParserID:     parserID,
		PipelineID:   pipelineID,
		ParserConfig: entity.JSONMap(parserConfigMap),
		Permission:   permission,
		EmbdID:       embdID,
		TenantEmbdID: stringPtrIfNotEmpty(tenantEmbdID),
		Status:       &status,
	}

	if err = d.kbDAO.Create(kb); err != nil {
		if dao.IsDuplicateKeyErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("Dataset name '%s' already exists", name)
		}
		return nil, common.CodeServerError, errors.New("Failed to save dataset")
	}

	createdKB, err := d.kbDAO.GetByID(kbID)
	if err != nil || createdKB == nil {
		return nil, common.CodeServerError, errors.New("Dataset created failed")
	}

	return datasetToMap(createdKB), common.CodeSuccess, nil
}

func (d *DatasetService) GetDataset(datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}

	normalizedID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	datasetID = normalizedID

	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, datasetID)
	}

	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
	}

	data := datasetToMap(kb)

	size, err := d.documentDAO.SumSizeByDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	data["size"] = size

	connectors, err := d.connectorDAO.ListByDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	data["connectors"] = datasetConnectorsOrEmpty(connectors)

	return data, common.CodeSuccess, nil
}

func (d *DatasetService) DeleteDatasets(ids []string, deleteAll bool, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	normalizedIDs := make([]string, 0, len(ids))
	seenIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		normalizedID, err := normalizeDatasetID(id)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if _, seen := seenIDs[normalizedID]; seen {
			continue
		}
		seenIDs[normalizedID] = struct{}{}
		normalizedIDs = append(normalizedIDs, normalizedID)
	}

	// If no explicit ids and deleteAll is set, resolve all datasets for this tenant.
	if len(normalizedIDs) == 0 {
		if !deleteAll {
			return map[string]interface{}{"deleted": []string{}}, common.CodeSuccess, nil
		}
		kbs, err := d.kbDAO.Query(map[string]interface{}{"tenant_id": tenantID})
		if err != nil {
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
		for _, kb := range kbs {
			normalizedIDs = append(normalizedIDs, kb.ID)
		}
	}

	// Validate ownership: collect KBs that exist and belong to this tenant.
	kbs := make([]*entity.Knowledgebase, 0, len(normalizedIDs))
	unauthorizedIDs := make([]string, 0)
	for _, id := range normalizedIDs {
		kb, err := d.kbDAO.GetByIDAndTenantID(id, tenantID)
		if err != nil || kb == nil {
			unauthorizedIDs = append(unauthorizedIDs, id)
			continue
		}
		kbs = append(kbs, kb)
	}
	if len(unauthorizedIDs) > 0 {
		return nil, common.CodeDataError,
			fmt.Errorf("User '%s' lacks permission for datasets: '%s'", tenantID, strings.Join(unauthorizedIDs, ", "))
	}

	successCount := 0
	errorsList := make([]string, 0)
	for _, kb := range kbs {
		if err := d.deleteDataset(tenantID, kb); err != nil {
			errorsList = append(errorsList, err.Error())
			common.Warn("deleteDataset failed", zap.String("kb_id", kb.ID), zap.Error(err))
			continue
		}
		successCount++
	}

	return map[string]interface{}{
		"success_count": successCount,
		"errors":        errorsList,
	}, common.CodeSuccess, nil
}

func (d *DatasetService) deleteDataset(tenantID string, kb *entity.Knowledgebase) error {
	// Collect document IDs first so engine cleanup can run before the
	// transaction (engine ops are not transactional).
	var documents []entity.Document
	if err := dao.DB.Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
		return fmt.Errorf("Delete dataset error for %s", kb.ID)
	}
	docIDs := extractDocIDs(documents)
	if len(docIDs) > 0 {
		d.deleteDatasetEngineData(kb, docIDs)
	}

	return dao.DB.Transaction(func(tx *gorm.DB) error {
		// Delete index tasks referencing this KB.
		if taskIDs := datasetIndexTaskIDs(kb); len(taskIDs) > 0 {
			if err := tx.Where("id IN ?", taskIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
		}

		if len(docIDs) > 0 {
			var mappings []entity.File2Document
			if err := tx.Where("document_id IN ?", docIDs).Find(&mappings).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			fileIDs := extractUniqueFileIDs(mappings)

			if err := tx.Where("doc_id IN ?", docIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&entity.File2Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if len(fileIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ?", fileIDs).Delete(&entity.File{}).Error; err != nil {
					return fmt.Errorf("Delete dataset error for %s", kb.ID)
				}
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&entity.Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
		}

		// Delete the KB folder file record.
		if err := tx.Unscoped().
			Where("source_type = ? AND type = ? AND name = ? AND tenant_id = ?",
				string(entity.FileSourceKnowledgebase), "folder", kb.Name, tenantID).
			Delete(&entity.File{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		if err := tx.Where("id = ?", kb.ID).Delete(&entity.Knowledgebase{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}
		return nil
	})
}

func (d *DatasetService) ListDatasets(id, name string, page, pageSize int, orderby string, desc bool, keywords string, ownerIDs []string, parserID, userID string) ([]map[string]interface{}, int64, common.ErrorCode, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		normalizedID, err := normalizeDatasetID(id)
		if err != nil {
			return nil, 0, common.CodeDataError, err
		}
		id = normalizedID

		kbs, err := d.kbDAO.GetKBByIDAndUserID(id, userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, 0, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, id)
		}
	}

	name = strings.TrimSpace(name)
	if name != "" {
		kbs, err := d.kbDAO.GetKBByNameAndUserID(name, userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, 0, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, name)
		}
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}

	orderby = strings.TrimSpace(orderby)
	if _, ok := datasetAllowedOrderByFields[orderby]; !ok {
		orderby = "create_time"
	}

	keywords = strings.TrimSpace(keywords)
	parserID = strings.TrimSpace(parserID)

	tenantIDs := make([]string, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID != "" {
			tenantIDs = append(tenantIDs, ownerID)
		}
	}
	if len(tenantIDs) == 0 {
		joinedTenants, err := d.tenantDAO.GetJoinedTenantsByUserID(userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		for _, joinedTenant := range joinedTenants {
			if joinedTenant == nil || joinedTenant.TenantID == "" {
				continue
			}
			tenantIDs = append(tenantIDs, joinedTenant.TenantID)
		}
	}

	kbs, total, err := d.kbDAO.GetByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords, parserID, id, name)
	if err != nil {
		return nil, 0, common.CodeServerError, errors.New("Database operation failed")
	}

	data := make([]map[string]interface{}, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		data = append(data, datasetListItemToMap(kb))
	}

	return data, total, common.CodeSuccess, nil
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
func (d *DatasetService) deleteDatasetEngineData(kb *entity.Knowledgebase, docIDs []string) {
	if d.docEngine == nil || len(docIDs) == 0 {
		return
	}
	ctx := context.Background()
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
