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
)

// UpdateDocumentMetadataConfig updates the metadata config for a document in a dataset.
func (d *DatasetService) UpdateDocumentMetadataConfig(ctx context.Context, userID, datasetID, documentID string, req map[string]interface{}) (*entity.Document, common.ErrorCode, error) {
	userID = strings.TrimSpace(userID)
	datasetID = strings.TrimSpace(datasetID)
	if !d.Accessible(ctx, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("you don't own the dataset")
	}

	doc, err := d.documentDAO.GetByDocumentIDAndDatasetID(ctx, dao.DB, documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("document %s not found in dataset %s", documentID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	metadata, ok := req["metadata"]
	if !ok {
		return nil, common.CodeArgumentError, errors.New("metadata is required")
	}

	parserConfig := doc.ParserConfig
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}
	parserConfig["metadata"] = metadata
	kb, kbErr := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if kbErr != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	if kb == nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	tenant, tenantErr := d.tenantDAO.GetByID(ctx, dao.DB, kb.TenantID)
	if tenantErr != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	llmID := ""
	if tenant != nil {
		llmID = tenant.LLMID
	}
	parserConfig = service.ApplyComponentScopedParserConfig(parserConfig, llmID)

	if err = d.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{"parser_config": parserConfig}); err != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	doc, err = d.documentDAO.GetByID(ctx, dao.DB, doc.ID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	return doc, common.CodeSuccess, nil
}

// GetMetadataConfig gets the auto-metadata configuration for a dataset.
func (d *DatasetService) GetMetadataConfig(ctx context.Context, datasetID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	tenantID = strings.TrimSpace(tenantID)
	if !d.Accessible(ctx, datasetID, tenantID) {
		return nil, common.CodeDataError, fmt.Errorf("user '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("dataset not found")
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	if kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("user '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	_, enabled, metadata, builtInMetadata := modularMetadataConfig(kb.ParserConfig)

	return map[string]interface{}{
		"enabled":           enabled,
		"metadata":          metadata,
		"built_in_metadata": builtInMetadata,
	}, common.CodeSuccess, nil
}

// UpdateMetadataConfig updates the auto-metadata configuration for a dataset.
func (d *DatasetService) UpdateMetadataConfig(ctx context.Context, datasetID, tenantID string, req *service.MetadataConfigRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	tenantID = strings.TrimSpace(tenantID)

	if !d.Accessible(ctx, datasetID, tenantID) {
		return nil, common.CodeDataError, fmt.Errorf("user '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("dataset not found")
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	if kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("user '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	if req == nil {
		req = &service.MetadataConfigRequest{}
	}

	metadata, err := normalizeMetadataConfigFields(req.Metadata, "metadata")
	if err != nil {
		return nil, common.CodeDataError, err
	}
	builtInMetadata, err := normalizeMetadataConfigFields(req.BuiltInMetadata, "built_in_metadata")
	if err != nil {
		return nil, common.CodeDataError, err
	}

	parserConfig := kb.ParserConfig
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}
	present, currentEnabled, _, _ := modularMetadataConfig(parserConfig)
	enabled := false
	switch {
	case req.Enabled != nil:
		enabled = *req.Enabled
	case present:
		enabled = currentEnabled
	default:
		enabled = len(metadata) > 0 || len(builtInMetadata) > 0
	}
	if req.Enabled == nil && len(metadata) == 0 && len(builtInMetadata) == 0 {
		enabled = false
	}
	parserConfig["metadata"] = map[string]any{
		"enabled":           enabled,
		"metadata":          metadata,
		"built_in_metadata": builtInMetadata,
	}
	delete(parserConfig, "enable_metadata")
	delete(parserConfig, "built_in_metadata")
	tenant, tenantErr := d.tenantDAO.GetByID(ctx, dao.DB, kb.TenantID)
	if tenantErr != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}
	llmID := ""
	if tenant != nil {
		llmID = tenant.LLMID
	}
	parserConfig = service.ApplyComponentScopedParserConfig(parserConfig, llmID)

	if err = d.kbDAO.UpdateByID(ctx, dao.DB, kb.ID, map[string]interface{}{"parser_config": parserConfig}); err != nil {
		return nil, common.CodeServerError, errors.New("update auto-metadata error.(Database error)")
	}

	return map[string]interface{}{
		"enabled":           enabled,
		"metadata":          metadata,
		"built_in_metadata": builtInMetadata,
	}, common.CodeSuccess, nil
}

// modularMetadataConfig reads the modular dataset-level metadata object
// ({"enabled", "metadata", "built_in_metadata"}) from parser_config. Missing
// or malformed config yields a disabled, empty result.
func modularMetadataConfig(parserConfig map[string]any) (bool, bool, []any, []any) {
	if parserConfig == nil {
		return false, false, []any{}, []any{}
	}
	metaObj, ok := parserConfig["metadata"].(map[string]any)
	if !ok {
		return false, false, []any{}, []any{}
	}
	enabled, _ := metaObj["enabled"].(bool)
	metadata := anyOrEmptyList(metaObj["metadata"])
	builtIn := anyOrEmptyList(metaObj["built_in_metadata"])
	return true, enabled, metadata, builtIn
}

func anyOrEmptyList(value any) []any {
	if value == nil {
		return []any{}
	}
	if list, ok := value.([]any); ok {
		return list
	}
	if list, ok := value.([]map[string]any); ok {
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, item)
		}
		return out
	}
	return []any{}
}
