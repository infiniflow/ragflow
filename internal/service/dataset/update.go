package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"

	"go.uber.org/zap"
)

func (d *DatasetService) UpdateDataset(datasetID, tenantID string, req service.UpdateDatasetRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	tenantID = strings.TrimSpace(tenantID)
	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Dataset not found")
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	if !d.Accessible(datasetID, tenantID) {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	connectorsProvided := req.Connectors != nil
	connectors := make([]service.DatasetConnectorRequest, 0)
	if req.Connectors != nil {
		connectors = *req.Connectors
	}

	updates := make(map[string]interface{})
	var pagerankUpdate *int64

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, common.CodeDataError, errors.New("`name` is required.")
		}
		if len(name) > 128 {
			return nil, common.CodeDataError, errors.New("String should have at most 128 characters")
		}
		updates["name"] = name
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return nil, common.CodeDataError, err
		}
		updates["avatar"] = *req.Avatar
	}
	if req.Description != nil {
		if len(*req.Description) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		updates["description"] = *req.Description
	}
	if req.Language != nil {
		language := strings.TrimSpace(*req.Language)
		if len(language) > 32 {
			return nil, common.CodeDataError, errors.New("String should have at most 32 characters")
		}
		updates["language"] = language
	}
	if req.Permission != nil {
		permission := strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
		updates["permission"] = permission
	}

	isPipelineMode := req.ParseType != nil && *req.ParseType == 2
	isBuiltinMode := req.ParseType != nil && *req.ParseType == 1

	if isBuiltinMode && req.PipelineID != nil {
		req.PipelineID = nil
	}
	if isPipelineMode && req.ParserID != nil {
		req.ParserID = nil
	}

	if req.PipelineID != nil {
		pipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if pipelineID != nil {
			updates["pipeline_id"] = *pipelineID
		}
	}

	parserID, parserIDProvided, err := datasetUpdateParserID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if parserIDProvided {
		updates["parser_id"] = parserID
	}

	if req.ParseType == nil && parserIDProvided && req.PipelineID != nil {
		return nil, common.CodeDataError, errors.New("parser_id and pipeline_id are mutually exclusive")
	}

	embdID, embdIDProvided, err := datasetUpdateEmbeddingID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if embdIDProvided {
		tenantEmbdID := ptrStringValue(kb.TenantEmbdID)
		if embdID == "" {
			embdID = kb.EmbdID
		} else {
			tenantEmbdID = ""
		}
		ok, message := d.verifyEmbeddingAvailability(embdID, tenantID)
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		if embdID != "" && tenantEmbdID == "" {
			resolvedID, err := service.NewModelProviderService().ResolveModelID(tenantID, entity.ModelTypeEmbedding, embdID)
			if err == nil {
				tenantEmbdID = resolvedID
			}
		}
		updates["embd_id"] = embdID
		updates["tenant_embd_id"] = stringPtrIfNotEmpty(tenantEmbdID)
	}

	if req.ParserConfig != nil {
		if err := validateDatasetParserConfigSize(req.ParserConfig); err != nil {
			return nil, common.CodeDataError, err
		}
		if len(req.ParserConfig) > 0 {
			effectiveParserID := kb.ParserID
			if parserIDProvided {
				effectiveParserID = parserID
			}
			effectivePipelineID := kb.PipelineID
			if req.PipelineID != nil {
				if normalized, err := normalizeDatasetPipelineID(*req.PipelineID); err == nil {
					effectivePipelineID = normalized
				}
			} else if parserIDProvided && kb.PipelineID != nil {
				effectivePipelineID = nil
			}

			isCanvas := effectivePipelineID != nil && strings.TrimSpace(*effectivePipelineID) != ""
			dslJSON, dslErr := service.LoadPipelineDSL(isCanvas, effectiveParserID, effectivePipelineID)
			if dslErr != nil {
				common.Warn("failed to load pipeline DSL for building parser_config",
					zap.String("parserID", effectiveParserID), zap.Error(dslErr))
			}
			if dslJSON != nil {
				updates["parser_config"] = pipelinepkg.BuildParserConfig(dslJSON, map[string]interface{}(req.ParserConfig))
			}
		}
	}

	if req.Pagerank != nil && *req.Pagerank != kb.Pagerank {
		if *req.Pagerank < 0 || *req.Pagerank > 100 {
			return nil, common.CodeDataError, errors.New("Input should be less than or equal to 100")
		}
		if !d.docEngine.SupportsPageRank() {
			return nil, common.CodeDataError, errors.New("'pagerank' can only be set when doc_engine is elasticsearch")
		}
		pagerank := *req.Pagerank
		pagerankUpdate = &pagerank
		updates["pagerank"] = pagerank
	}

	if parserIDProvided && parserID != kb.ParserID {
		if _, ok := updates["parser_config"]; !ok {
			if resolved, cpErr := service.ResolveComponentParamsDefaults(parserID, nil); cpErr != nil {
				common.Warn("failed to resolve component params defaults on parser_id switch",
					zap.String("parserID", parserID), zap.Error(cpErr))
			} else if resolved != nil {
				updates["parser_config"] = resolved
			}
		}
	}
	if kb.PipelineID != nil && parserIDProvided {
		if _, ok := updates["pipeline_id"]; !ok {
			updates["pipeline_id"] = nil
		}
	}

	pipelineChanged := req.PipelineID != nil && (kb.PipelineID == nil || *req.PipelineID != *kb.PipelineID)
	if pipelineChanged {
		cfgParserID := kb.ParserID
		if parserIDProvided {
			cfgParserID = parserID
		}
		cfgPipelineID, _ := updates["pipeline_id"].(string)
		var cpPipelineID *string
		if cfgPipelineID != "" {
			cpPipelineID = &cfgPipelineID
		}
		if cpDefaults, cpErr := service.ResolveComponentParamsDefaults(cfgParserID, cpPipelineID); cpErr != nil {
			common.Warn("failed to resolve component params defaults on pipeline change",
				zap.String("parserID", cfgParserID), zap.Error(cpErr))
		} else if cpDefaults != nil {
			updates["parser_config"] = cpDefaults
		}
	}

	if nameValue, ok := updates["name"].(string); ok && strings.ToLower(nameValue) != strings.ToLower(kb.Name) {
		existing, lookupErr := d.kbDAO.GetByName(nameValue, tenantID)
		if lookupErr != nil && !dao.IsNotFoundErr(lookupErr) {
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
		if existing != nil {
			return nil, common.CodeDataError, fmt.Errorf("Dataset name '%s' already exists", nameValue)
		}
	}

	if len(updates) == 0 && !connectorsProvided && !req.ParserConfigProvided {
		return nil, common.CodeDataError, errors.New("No properties were modified")
	}

	connectorLinks := make([]dao.DatasetConnectorLink, 0, len(connectors))
	if len(updates) > 0 {
		if err = d.kbDAO.UpdateByID(kb.ID, updates); err != nil {
			if dao.IsDuplicateKeyErr(err) {
				if nameValue, ok := updates["name"].(string); ok {
					return nil, common.CodeDataError, fmt.Errorf("Dataset name '%s' already exists", nameValue)
				}
				return nil, common.CodeDataError, errors.New("Dataset name already exists")
			}
			return nil, common.CodeServerError, errors.New("Update dataset error.(Database error)")
		}
	}

	if connectorsProvided {
		connectorLinks := make([]dao.DatasetConnectorLink, 0, len(connectors))
		for _, connector := range connectors {
			connectorID := strings.TrimSpace(connector.ID)
			if connectorID == "" {
				return nil, common.CodeDataError, errors.New("connector id is required")
			}
			connectorLinks = append(connectorLinks, dao.DatasetConnectorLink{
				ID:        connectorID,
				AutoParse: connector.AutoParse,
			})
		}
	}

	txCode := common.CodeSuccess
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		lockedKB, code, authErr := d.lockAccessibleDatasetForUpdate(tx, kb.ID, tenantID)
		if authErr != nil {
			txCode = code
			return authErr
		}

		if pagerankUpdate != nil {
			indexName := fmt.Sprintf("ragflow_%s", lockedKB.TenantID)
			if *pagerankUpdate > 0 {
				err = d.docEngine.UpdateChunks(context.Background(), map[string]interface{}{"kb_id": lockedKB.ID}, map[string]interface{}{common.PAGERANK_FLD: *pagerankUpdate}, indexName, lockedKB.ID)
			} else {
				err = d.docEngine.UpdateChunks(context.Background(), map[string]interface{}{"exists": common.PAGERANK_FLD}, map[string]interface{}{"remove": common.PAGERANK_FLD}, indexName, lockedKB.ID)
			}
			if err != nil {
				txCode = common.CodeServerError
				return err
			}
		}

		if len(updates) > 0 {
			if err = tx.Model(&entity.Knowledgebase{}).Where("id = ?", lockedKB.ID).Updates(updates).Error; err != nil {
				txCode = common.CodeServerError
				return errors.New("Update dataset error.(Database error)")
			}
		}

		if connectorsProvided {
			if err = d.connectorDAO.LinkDatasetConnectorsTx(tx, lockedKB.ID, connectorLinks); err != nil {
				txCode = common.CodeServerError
				return errors.New("Database operation failed")
			}
		}

		return nil
	})
	if err != nil {
		return nil, txCode, err
	}

	updatedKB, err := d.kbDAO.GetByID(kb.ID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Dataset updated failed")
	}

	data := datasetToMap(updatedKB)
	linkedConnectors, err := d.connectorDAO.ListByDatasetID(kb.ID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	data["connectors"] = datasetConnectorsOrEmpty(linkedConnectors)
	return data, common.CodeSuccess, nil
}

func (d *DatasetService) lockAccessibleDatasetForUpdate(tx *gorm.DB, datasetID, userID string) (*entity.Knowledgebase, common.ErrorCode, error) {
	var kb entity.Knowledgebase
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", datasetID, string(entity.StatusValid)).
		First(&kb).Error
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Dataset not found")
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	if kb.TenantID == userID {
		return &kb, common.CodeSuccess, nil
	}
	if kb.Permission != string(entity.TenantPermissionTeam) {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, datasetID)
	}

	var relation entity.UserTenant
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND user_id = ? AND status = ?", kb.TenantID, userID, "1").
		First(&relation).Error
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	return &kb, common.CodeSuccess, nil
}
