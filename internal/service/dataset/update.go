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
	"ragflow/internal/service"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const datasetPagerankUpdateTimeout = 30 * time.Second

type datasetPagerankUpdate struct {
	value     int64
	index     string
	datasetID string
}

func (d *DatasetService) UpdateDataset(ctx context.Context, datasetID, tenantID string, req service.UpdateDatasetRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	tenantID = strings.TrimSpace(tenantID)
	if _, err := d.kbDAO.GetByID(datasetID); err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("dataset not found")
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	connectorsProvided := req.Connectors != nil
	connectors := make([]service.DatasetConnectorRequest, 0)
	if req.Connectors != nil {
		connectors = *req.Connectors
	}

	connectorLinks := make([]dao.DatasetConnectorLink, 0, len(connectors))
	if connectorsProvided {
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

	simpleUpdates := make(map[string]interface{})

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, common.CodeDataError, errors.New("`name` is required")
		}
		if len(name) > 128 {
			return nil, common.CodeDataError, errors.New("string should have at most 128 characters")
		}
		simpleUpdates["name"] = name
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 65535 {
			return nil, common.CodeDataError, errors.New("string should have at most 65535 characters")
		}
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return nil, common.CodeDataError, err
		}
		simpleUpdates["avatar"] = *req.Avatar
	}
	if req.Description != nil {
		if len(*req.Description) > 65535 {
			return nil, common.CodeDataError, errors.New("string should have at most 65535 characters")
		}
		simpleUpdates["description"] = *req.Description
	}
	if req.Language != nil {
		language := strings.TrimSpace(*req.Language)
		if len(language) > 32 {
			return nil, common.CodeDataError, errors.New("string should have at most 32 characters")
		}
		simpleUpdates["language"] = language
	}
	if req.Permission != nil {
		permission := strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("input should be 'me' or 'team'")
		}
		simpleUpdates["permission"] = permission
	}

	isPipelineMode := req.ParseType != nil && *req.ParseType == 2
	isBuiltinMode := req.ParseType != nil && *req.ParseType == 1

	if isBuiltinMode && req.PipelineID != nil {
		req.PipelineID = nil
	}
	if isPipelineMode && req.ParserID != nil {
		req.ParserID = nil
	}

	var pipelineID *string
	if req.PipelineID != nil {
		normalizedPipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = normalizedPipelineID
	}

	parserID, parserIDProvided, err := datasetUpdateParserID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}

	if req.ParseType == nil && parserIDProvided && req.PipelineID != nil {
		return nil, common.CodeDataError, errors.New("parser_id and pipeline_id are mutually exclusive")
	}

	embdID, embdIDProvided, err := datasetUpdateEmbeddingID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}

	if req.ParserConfig != nil {
		if err := validateDatasetParserConfigSize(req.ParserConfig); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	var requestedPagerank int64
	pagerankRequested := req.Pagerank != nil
	if pagerankRequested {
		requestedPagerank = *req.Pagerank
		if *req.Pagerank < 0 || *req.Pagerank > 100 {
			return nil, common.CodeDataError, errors.New("input should be less than or equal to 100")
		}
		if d.docEngine == nil {
			return nil, common.CodeServerError, errors.New("document engine is not initialized")
		}
		if !d.docEngine.SupportsPageRank() {
			return nil, common.CodeDataError, errors.New("'pagerank' can only be set when doc_engine is elasticsearch")
		}
	}

	requestedAnyUpdate := len(simpleUpdates) > 0 || connectorsProvided || parserIDProvided ||
		pipelineID != nil || embdIDProvided || pagerankRequested || req.ParserConfig != nil || req.ParserConfigProvided
	if !requestedAnyUpdate {
		return nil, common.CodeDataError, errors.New("no properties were modified")
	}

	txCode := common.CodeSuccess
	var updatedKB *entity.Knowledgebase
	var linkedConnectors []*dao.ConnectorDatasetListItem
	var pagerankUpdate *datasetPagerankUpdate
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		lockedKB, code, authErr := d.lockAccessibleDatasetForUpdate(tx, datasetID, tenantID)
		if authErr != nil {
			txCode = code
			return authErr
		}

		if req.Permission != nil && lockedKB.TenantID != tenantID {
			txCode = common.CodeDataError
			return errors.New("only dataset owner can change permission")
		}

		updates := make(map[string]interface{}, len(simpleUpdates)+6)
		for key, value := range simpleUpdates {
			updates[key] = value
		}

		if nameValue, ok := updates["name"].(string); ok && strings.ToLower(nameValue) != strings.ToLower(lockedKB.Name) {
			var existing entity.Knowledgebase
			lookupErr := tx.Where("LOWER(name) = LOWER(?) AND tenant_id = ? AND status = ?", nameValue, tenantID, string(entity.StatusValid)).First(&existing).Error
			if lookupErr != nil && !dao.IsNotFoundErr(lookupErr) {
				txCode = common.CodeServerError
				return errors.New("database operation failed")
			}
			if lookupErr == nil {
				txCode = common.CodeDataError
				return fmt.Errorf("dataset name '%s' already exists", nameValue)
			}
		}

		if pipelineID != nil {
			updates["pipeline_id"] = *pipelineID
		}
		if parserIDProvided {
			updates["parser_id"] = parserID
		}
		if embdIDProvided {
			effectiveEmbdID := embdID
			tenantEmbdID := ptrStringValue(lockedKB.TenantEmbdID)
			if effectiveEmbdID == "" {
				effectiveEmbdID = lockedKB.EmbdID
			} else {
				tenantEmbdID = ""
			}
			ok, message := d.verifyEmbeddingAvailability(effectiveEmbdID, tenantID)
			if !ok {
				txCode = common.CodeDataError
				return errors.New(message)
			}
			if effectiveEmbdID != "" && tenantEmbdID == "" {
				resolvedID, err := service.NewModelProviderService().ResolveModelID(tenantID, entity.ModelTypeEmbedding, effectiveEmbdID)
				if err == nil {
					tenantEmbdID = resolvedID
				}
			}
			updates["embd_id"] = effectiveEmbdID
			updates["tenant_embd_id"] = stringPtrIfNotEmpty(tenantEmbdID)
		}

		if req.ParserConfig != nil && len(req.ParserConfig) > 0 {
			effectiveParserID := lockedKB.ParserID
			if parserIDProvided {
				effectiveParserID = parserID
			}
			effectivePipelineID := lockedKB.PipelineID
			if pipelineID != nil {
				effectivePipelineID = pipelineID
			} else if parserIDProvided && lockedKB.PipelineID != nil {
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
		if pagerankRequested {
			pagerankUpdate = &datasetPagerankUpdate{
				value:     requestedPagerank,
				index:     fmt.Sprintf("ragflow_%s", lockedKB.TenantID),
				datasetID: lockedKB.ID,
			}
			if requestedPagerank != lockedKB.Pagerank {
				updates["pagerank"] = requestedPagerank
			}
		}
		if parserIDProvided && parserID != lockedKB.ParserID {
			if _, ok := updates["parser_config"]; !ok {
				if resolved, cpErr := service.ResolveComponentParamsDefaults(parserID, nil); cpErr != nil {
					common.Warn("failed to resolve component params defaults on parser_id switch",
						zap.String("parserID", parserID), zap.Error(cpErr))
				} else if resolved != nil {
					updates["parser_config"] = resolved
				}
			}
		}
		if lockedKB.PipelineID != nil && parserIDProvided {
			if _, ok := updates["pipeline_id"]; !ok {
				updates["pipeline_id"] = nil
			}
		}

		pipelineChanged := pipelineID != nil && (lockedKB.PipelineID == nil || *pipelineID != *lockedKB.PipelineID)
		if pipelineChanged {
			cfgParserID := lockedKB.ParserID
			if parserIDProvided {
				cfgParserID = parserID
			}
			if cpDefaults, cpErr := service.ResolveComponentParamsDefaults(cfgParserID, pipelineID); cpErr != nil {
				common.Warn("failed to resolve component params defaults on pipeline change",
					zap.String("parserID", cfgParserID), zap.Error(cpErr))
			} else if cpDefaults != nil {
				updates["parser_config"] = cpDefaults
			}
		}
		if len(updates) > 0 {
			if err = tx.Model(&entity.Knowledgebase{}).Where("id = ?", lockedKB.ID).Updates(updates).Error; err != nil {
				if dao.IsDuplicateKeyErr(err) {
					if nameValue, ok := updates["name"].(string); ok {
						txCode = common.CodeDataError
						return fmt.Errorf("dataset name '%s' already exists", nameValue)
					}
					txCode = common.CodeDataError
					return errors.New("dataset name already exists")
				}
				txCode = common.CodeServerError
				return errors.New("dataset update error. (database error)")
			}
		}

		if connectorsProvided {
			if err = d.connectorDAO.LinkDatasetConnectorsTx(ctx, tx, lockedKB.ID, lockedKB.TenantID, connectorLinks); err != nil {
				if dao.IsConnectorNotAccessibleErr(err) {
					txCode = common.CodeDataError
					return err
				}
				txCode = common.CodeServerError
				return errors.New("database operation failed")
			}
		}

		updatedKB = &entity.Knowledgebase{}
		if err = tx.Where("id = ? AND status = ?", lockedKB.ID, string(entity.StatusValid)).First(updatedKB).Error; err != nil {
			txCode = common.CodeDataError
			return errors.New("dataset updated failed")
		}

		linkedConnectors, err = d.connectorDAO.ListByDatasetIDTx(ctx, tx, lockedKB.ID)
		if err != nil {
			txCode = common.CodeServerError
			return errors.New("database operation failed")
		}

		return nil
	})
	if err != nil {
		if txCode == common.CodeSuccess {
			txCode = common.CodeServerError
		}
		return nil, txCode, err
	}

	if pagerankUpdate != nil {
		if err = d.updateDatasetPagerankChunks(*pagerankUpdate); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	data := datasetToMap(updatedKB)
	data["connectors"] = datasetConnectorsOrEmpty(linkedConnectors)
	return data, common.CodeSuccess, nil
}

func (d *DatasetService) updateDatasetPagerankChunks(update datasetPagerankUpdate) error {
	ctx, cancel := context.WithTimeout(context.Background(), datasetPagerankUpdateTimeout)
	defer cancel()
	if update.value > 0 {
		return d.docEngine.UpdateChunks(ctx, map[string]interface{}{"kb_id": update.datasetID}, map[string]interface{}{common.PAGERANK_FLD: update.value}, update.index, update.datasetID)
	}
	return d.docEngine.UpdateChunks(ctx, map[string]interface{}{"exists": common.PAGERANK_FLD}, map[string]interface{}{"remove": common.PAGERANK_FLD}, update.index, update.datasetID)
}

func (d *DatasetService) lockAccessibleDatasetForUpdate(tx *gorm.DB, datasetID, userID string) (*entity.Knowledgebase, common.ErrorCode, error) {
	var kb entity.Knowledgebase
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", datasetID, string(entity.StatusValid)).
		First(&kb).Error
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("dataset not found")
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	if kb.TenantID == userID {
		return &kb, common.CodeSuccess, nil
	}
	if kb.Permission != string(entity.TenantPermissionTeam) {
		return nil, common.CodeDataError, fmt.Errorf("user '%s' lacks permission for dataset '%s'", userID, datasetID)
	}

	var relation entity.UserTenant
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND user_id = ? AND status = ?", kb.TenantID, userID, "1").
		First(&relation).Error
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("user '%s' lacks permission for dataset '%s'", userID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	return &kb, common.CodeSuccess, nil
}
