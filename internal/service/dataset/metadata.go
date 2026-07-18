package dataset

import (
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// UpdateDocumentMetadataConfig updates the metadata config for a document in a dataset.
func (d *DatasetService) UpdateDocumentMetadataConfig(userID, datasetID, documentID string, req map[string]interface{}) (*entity.Document, common.ErrorCode, error) {
	if _, err := d.kbDAO.GetByIDAndTenantID(datasetID, userID); err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("You don't own the dataset.")
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	doc, err := d.documentDAO.GetByDocumentIDAndDatasetID(documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("Document %s not found in dataset %s", documentID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
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

	if err = d.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"parser_config": parserConfig}); err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	doc, err = d.documentDAO.GetByID(doc.ID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	return doc, common.CodeSuccess, nil
}

// GetMetadataConfig gets the auto-metadata configuration for a dataset.
func (d *DatasetService) GetMetadataConfig(datasetID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := d.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	if kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	return map[string]interface{}{
		"metadata":          parserConfigValueOrEmptyList(kb.ParserConfig, "metadata"),
		"built_in_metadata": parserConfigValueOrEmptyList(kb.ParserConfig, "built_in_metadata"),
	}, common.CodeSuccess, nil
}

// UpdateMetadataConfig updates the auto-metadata configuration for a dataset.
func (d *DatasetService) UpdateMetadataConfig(datasetID, tenantID string, req *service.MetadataConfigRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	tenantID = strings.TrimSpace(tenantID)

	kb, err := d.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	if kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
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
	parserConfig["metadata"] = metadata
	parserConfig["built_in_metadata"] = builtInMetadata

	if err = d.kbDAO.UpdateByID(kb.ID, map[string]interface{}{"parser_config": parserConfig}); err != nil {
		return nil, common.CodeServerError, errors.New("Update auto-metadata error.(Database error)")
	}

	return map[string]interface{}{
		"metadata":          metadata,
		"built_in_metadata": builtInMetadata,
	}, common.CodeSuccess, nil
}
