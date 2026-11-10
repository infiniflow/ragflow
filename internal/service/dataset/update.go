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
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const datasetPagerankUpdateTimeout = 30 * time.Second

// restoreEmbeddingBindingTimeout bounds the compensating write that runs after
// the request context may already be cancelled.
const restoreEmbeddingBindingTimeout = 10 * time.Second

// The compensating write is the only way back to a model whose store exists,
// so a transient database failure must not be accepted on the first attempt.
const (
	restoreEmbeddingBindingAttempts = 3
	restoreEmbeddingBindingRetry    = 250 * time.Millisecond
)

type datasetPagerankUpdate struct {
	value     int64
	index     string
	datasetID string
}

func (d *DatasetService) UpdateDataset(ctx context.Context, datasetID string, req service.UpdateDatasetRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	userID := strings.TrimSpace(permission.UserIDFromContext(ctx))
	if _, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID); err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
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
			return nil, common.CodeDataError, errors.New("String should have at least 1 character")
		}
		if utf8.RuneCountInString(name) > entity.DatasetNameLimit {
			return nil, common.CodeDataError, fmt.Errorf("String should have at most %d characters", entity.DatasetNameLimit)
		}
		simpleUpdates["name"] = name
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return nil, common.CodeDataError, err
		}
		simpleUpdates["avatar"] = *req.Avatar
	}
	if req.Description != nil {
		if len(*req.Description) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		simpleUpdates["description"] = *req.Description
	}
	if req.Language != nil {
		language, err := normalizeDatasetLanguage(*req.Language)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		simpleUpdates["language"] = language
	}
	if req.Permission != nil {
		permission := strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
		simpleUpdates["permission"] = permission
	}

	// Validate the parse_type/parser_id/pipeline_id triple. FromRequest rejects
	// any partial or contradictory combination (e.g. an id without parse_type,
	// or both ids). A fully-absent block is allowed on update and means "keep
	// current" — handled below via Resolve.
	sel, modeErr := service.FromRequest(req.ParseType, req.ParserID, req.PipelineID)
	if modeErr != nil {
		return nil, common.CodeDataError, modeErr
	}
	if sel != nil {
		if sel.IsBuiltIn() && req.PipelineID != nil {
			req.PipelineID = nil
		}
		if sel.IsPipeline() && req.ParserID != nil {
			req.ParserID = nil
		}
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

	embdID, embdIDProvided, err := datasetUpdateEmbeddingID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}

	if req.ParserConfig != nil {
		dropped, err := ValidateParserConfig(req.ParserConfig)
		if len(dropped) > 0 {
			common.Warn("dropping unscoped (flat) parser_config keys; keys must be component-scoped (contain ':')",
				zap.Strings("keys", dropped),
				zap.String("dataset_id", datasetID),
				zap.String("user_id", userID),
			)
		}
		if err != nil {
			return nil, common.CodeArgumentError, err
		}
		if err = pipelinepkg.NormalizeParserConfigPages(req.ParserConfig); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	var requestedPagerank int64
	pagerankRequested := req.Pagerank != nil
	if pagerankRequested {
		requestedPagerank = *req.Pagerank
		if *req.Pagerank < 0 {
			return nil, common.CodeDataError, errors.New("input should be greater than or equal to 0")
		}
		if *req.Pagerank > 100 {
			return nil, common.CodeDataError, errors.New("input should be less than or equal to 100")
		}
		if d.docEngine == nil {
			return nil, common.CodeServerError, errors.New("document engine is not initialized")
		}
	}

	requestedAnyUpdate := len(simpleUpdates) > 0 || connectorsProvided || parserIDProvided ||
		pipelineID != nil || embdIDProvided || pagerankRequested || req.ParserConfig != nil || req.ParserConfigProvided || req.PipelineVersionID != nil
	if !requestedAnyUpdate {
		return nil, common.CodeDataError, errors.New("no properties were modified")
	}

	txCode := common.CodeSuccess
	var updatedKB *entity.Knowledgebase
	var linkedConnectors []*dao.ConnectorDatasetListItem
	var scheduledTaskIDs []string
	var pagerankUpdate *datasetPagerankUpdate
	var chunkStoreEmbdID string
	var previousEmbdID string
	var previousTenantEmbdID *string
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		lockedKB, code, authErr := d.lockAccessibleDatasetForUpdate(ctx, tx, datasetID)
		if authErr != nil {
			txCode = code
			return authErr
		}
		if sel != nil || pipelineID != nil || req.ParserConfig != nil || req.ParserConfigProvided {
			current := service.CurrentSelection(lockedKB.ParserID, lockedKB.PipelineID)
			effective, resolveErr := service.Resolve(current, sel)
			if resolveErr != nil {
				txCode = common.CodeDataError
				return resolveErr
			}
			isPipeline, _, effectivePipelineID := effective.Effective()
			if isPipeline && effectivePipelineID != nil && strings.TrimSpace(*effectivePipelineID) != "" {
				accessErr := permission.NewLockingDatabaseChecker(tx).CheckDependency(
					ctx,
					permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: lockedKB.ID},
					permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: strings.TrimSpace(*effectivePipelineID)},
					permission.OperationUpdate,
					permission.OperationUse,
				)
				if accessErr != nil {
					txCode, accessErr = permissionresponse.Normalize(accessErr)
					return accessErr
				}
			}
		}

		updates := make(map[string]interface{}, len(simpleUpdates)+6)
		for key, value := range simpleUpdates {
			updates[key] = value
		}
		if permissionValue, ok := updates["permission"].(string); ok {
			if permissionValue != lockedKB.Permission {
				accessErr := permission.NewLockingDatabaseChecker(tx).CheckResource(
					ctx,
					permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID},
					permission.OperationShare,
				)
				if accessErr != nil {
					txCode, accessErr = permissionresponse.Normalize(accessErr)
					return accessErr
				}
			}
			if permissionValue == lockedKB.Permission {
				delete(updates, "permission")
			}
		}

		if nameValue, ok := updates["name"].(string); ok {
			available, nameErr := common.NameAvailable(lockedKB.Name, nameValue, func(candidate string) (bool, error) {
				var existing entity.Knowledgebase
				lookupErr := tx.Where("LOWER(name) = LOWER(?) AND tenant_id = ? AND status = ?", candidate, lockedKB.TenantID, string(entity.StatusValid)).First(&existing).Error
				if lookupErr != nil {
					if dao.IsNotFoundErr(lookupErr) {
						return false, nil
					}
					return false, lookupErr
				}
				return true, nil
			})
			if nameErr != nil {
				txCode = common.CodeServerError
				return fmt.Errorf("database operation failed: %w", nameErr)
			}
			if !available {
				txCode = common.CodeDataError
				return fmt.Errorf("Dataset name '%s' already exists", nameValue)
			}
			if nameValue != lockedKB.Name {
				code, renameErr := renameDatasetFolder(ctx, tx, lockedKB, nameValue)
				if renameErr != nil {
					txCode = code
					if code == common.CodeServerError {
						common.Warn("dataset folder rename failed",
							zap.String("dataset_id", lockedKB.ID), zap.Error(renameErr))
						return fmt.Errorf("database operation failed: %w", renameErr)
					}
					return renameErr
				}
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
			modelCtx := permission.WithTenantID(ctx, lockedKB.TenantID)
			ok, message := d.verifyEmbeddingAvailability(modelCtx, effectiveEmbdID, lockedKB.TenantID)
			if !ok {
				txCode = common.CodeDataError
				return errors.New(message)
			}
			if effectiveEmbdID != "" && tenantEmbdID == "" {
				target, err := service.NewModelFactory().ResolveInfo(modelCtx, entity.ModelTypeEmbedding, effectiveEmbdID)
				if err == nil {
					tenantEmbdID = target.ID
				}
			}
			updates["embd_id"] = effectiveEmbdID
			updates["tenant_embd_id"] = stringPtrIfNotEmpty(tenantEmbdID)
			// An empty dataset can switch models, but its chunk store keeps the
			// vector column of the previous model. Remember the change so the new
			// column is added after the commit; with rows present the switch is
			// rejected upstream because the old vectors cannot be reinterpreted.
			if effectiveEmbdID != "" && effectiveEmbdID != lockedKB.EmbdID && lockedKB.ChunkNum == 0 {
				chunkStoreEmbdID = effectiveEmbdID
				previousEmbdID = lockedKB.EmbdID
				if lockedKB.TenantEmbdID != nil {
					tenantEmbdID := *lockedKB.TenantEmbdID
					previousTenantEmbdID = &tenantEmbdID
				} else {
					previousTenantEmbdID = nil
				}
			}
		}

		// Design §5 fallback chains (v5.1/v5.2): when the request omits a field,
		// fall back to the persisted (locked) binding rather than resetting to
		// latest/nil. This is what makes the sticky snapshot (B4), the pinned-DSL
		// parser_config rebuild (B1), and single-field PATCH (B2) correct.
		canvasCtx := pipelineID
		if canvasCtx == nil {
			canvasCtx = lockedKB.PipelineID
		}
		pipelineChanged := pipelineID != nil && (lockedKB.PipelineID == nil || *pipelineID != *lockedKB.PipelineID)
		requestedVersion := req.PipelineVersionID
		if requestedVersion == nil && !pipelineChanged {
			requestedVersion = lockedKB.PipelineVersionID
		}

		if req.ParserConfig != nil && len(req.ParserConfig) > 0 {
			// Resolve the effective mode/IDs once. sel is the request selection
			// (nil when the whole selection block was omitted); current is the
			// persisted selection. Resolve applies PATCH semantics: an omitted
			// block keeps the current mode so only parser_config changes still
			// load the correct DSL.
			current := service.CurrentSelection(lockedKB.ParserID, lockedKB.PipelineID)
			eff, resolveErr := service.Resolve(current, sel)
			if resolveErr != nil {
				return resolveErr
			}
			isPipeline, effParserID, effPipelineID := eff.Effective()
			// Thread the effective pipeline version (design §5): use the §5 fallback
			// requestedVersion (explicit pin when supplied, else the persisted
			// pinned version) so a parser_config-only PATCH (B1) rebuilds from the
			// pinned DSL rather than the canvas latest. The binding block below
			// re-resolves the same value (and surfaces any ownership error); a load
			// failure here is best-effort for parser_config rebuild only.
			effVersionID, _ := resolvePipelineVersionID(ctx, tx, effPipelineID, requestedVersion)
			dslJSON, dslErr := service.LoadPipelineDSL(ctx, isPipeline, effParserID, effPipelineID, effVersionID)
			if dslErr != nil {
				common.Warn("failed to load pipeline DSL for building parser_config",
					zap.String("parserID", effParserID), zap.Error(dslErr))
			}
			if dslJSON != nil {
				parserConfig, buildErr := pipelinepkg.BuildParserConfig(dslJSON, map[string]interface{}(req.ParserConfig))
				if buildErr != nil {
					txCode = common.CodeDataError
					return buildErr
				}
				updates["parser_config"] = preserveDatasetParserConfigState(parserConfig, lockedKB.ParserConfig, req.ParserConfig)
			}
		}
		if pagerankRequested && requestedPagerank != lockedKB.Pagerank {
			if !d.docEngine.SupportsPageRank() {
				txCode = common.CodeDataError
				return errors.New("'pagerank' can only be set when doc_engine is elasticsearch")
			}
			pagerankUpdate = &datasetPagerankUpdate{
				value:     requestedPagerank,
				index:     fmt.Sprintf("ragflow_%s", lockedKB.TenantID),
				datasetID: lockedKB.ID,
			}
			updates["pagerank"] = requestedPagerank
		}
		if parserIDProvided && parserID != lockedKB.ParserID {
			if _, ok := updates["parser_config"]; !ok {
				if resolved, cpErr := service.ResolveComponentParamsDefaults(ctx, parserID, nil, nil); cpErr != nil {
					common.Warn("failed to resolve component params defaults on parser_id switch",
						zap.String("parserID", parserID), zap.Error(cpErr))
				} else if resolved != nil {
					updates["parser_config"] = preserveDatasetParserConfigState(resolved, lockedKB.ParserConfig, req.ParserConfig)
				}
			}
		}
		if lockedKB.PipelineID != nil && parserIDProvided {
			if _, ok := updates["pipeline_id"]; !ok {
				updates["pipeline_id"] = nil
			}
		}

		// pipelineChanged already computed in the §5 prep block above.
		// C3 (T26/V5): an explicit, non-empty version that differs from the
		// currently pinned version is a version switch even when the pipeline
		// canvas is unchanged. Such a switch must reset the component params to
		// the new version defaults (below), not only a pipeline_id change.
		versionChanged := false
		if req.PipelineVersionID != nil && strings.TrimSpace(*req.PipelineVersionID) != "" {
			if lockedKB.PipelineVersionID == nil || strings.TrimSpace(*req.PipelineVersionID) != strings.TrimSpace(*lockedKB.PipelineVersionID) {
				versionChanged = true
			}
		}
		// Design C5/G3: an empty pipeline_version_id in Pipeline mode is REJECTED
		// (no longer silently cleared or auto-backfilled to the canvas latest).
		// The Save fails with ErrPipelineVersionEmpty so the UI can show the red
		// Missing badge and the "mission version info" error. BuiltIn mode (no canvas
		// bound) is exempt and may carry an empty version.
		isEffPipeline := canvasCtx != nil && strings.TrimSpace(*canvasCtx) != ""
		if req.PipelineVersionID != nil {
			if strings.TrimSpace(*req.PipelineVersionID) != "" {
				bound, perr := resolvePipelineVersionID(ctx, tx, canvasCtx, req.PipelineVersionID)
				if perr != nil {
					txCode = common.CodeBadRequest
					return perr
				}
				updates["pipeline_version_id"] = bound
			} else if isEffPipeline {
				// Explicit empty version in Pipeline mode: reject (do not clear the
				// pin). Only BuiltIn-mode explicit empties are allowed to clear.
				txCode = common.CodeBadRequest
				return service.ErrPipelineVersionEmpty
			} else {
				// BuiltIn mode explicit empty: clear the pin (no canvas to pin to).
				updates["pipeline_version_id"] = nil
			}
		} else if pipelineChanged && pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
			// Switching to a Pipeline canvas without an explicit version is a
			// missing-version Save (design C5/G3): reject rather than silently
			// locking the new canvas's latest version.
			txCode = common.CodeBadRequest
			return service.ErrPipelineVersionEmpty
		}
		// C3 / A3: a pipeline (canvas) switch or a version switch resets
		// parser_config to the new DSL defaults — but only when the request
		// carried no parser_config of its own. A submitted config has already
		// been reconciled against the new version's DSL by BuildParserConfig
		// above (unknown operators dropped, missing params defaulted), and the
		// settings form itself resets to the new defaults on a switch before
		// the user edits — so a submitted config is authoritative and must not
		// be discarded.
		if pipelineChanged || versionChanged {
			if _, submitted := updates["parser_config"]; !submitted {
				cfgParserID := lockedKB.ParserID
				if parserIDProvided {
					cfgParserID = parserID
				}
				effVersionID, _ := resolvePipelineVersionID(ctx, tx, canvasCtx, requestedVersion)
				cpDefaults, cpErr := service.ResolveComponentParamsDefaults(ctx, cfgParserID, canvasCtx, effVersionID)
				if cpErr != nil {
					common.Warn("failed to resolve component params defaults on pipeline/version change",
						zap.String("parserID", cfgParserID), zap.Error(cpErr))
				}
				if cpErr == nil {
					updates["parser_config"] = cpDefaults
				}
			}
		}

		effectiveParserConfig := parserConfigJSONMap(updates["parser_config"])
		if effectiveParserConfig == nil && embdIDProvided {
			effectiveParserConfig = cloneJSONMap(lockedKB.ParserConfig)
		}
		if effectiveParserConfig != nil {
			llmID := ""
			var membership entity.UserTenant
			if tenantErr := tx.Where("user_id = ? AND tenant_id = ? AND status = ?", userID, lockedKB.TenantID, entity.TenantMembershipStatusJoined).Take(&membership).Error; tenantErr == nil {
				llmID = membership.LLMID
				if membership.TenantLLMID != nil && strings.TrimSpace(*membership.TenantLLMID) != "" {
					llmID = strings.TrimSpace(*membership.TenantLLMID)
				}
			}
			effectiveParserConfig = service.ApplyComponentScopedParserConfig(
				effectiveParserConfig,
				llmID,
			)
			updates["parser_config"] = effectiveParserConfig
		}

		if len(updates) > 0 {
			if err = tx.Model(&entity.Knowledgebase{}).Where("id = ?", lockedKB.ID).Updates(updates).Error; err != nil {
				if dao.IsDuplicateKeyErr(err) {
					if nameValue, ok := updates["name"].(string); ok {
						txCode = common.CodeDataError
						return fmt.Errorf("Dataset name '%s' already exists", nameValue)
					}
					txCode = common.CodeDataError
					return errors.New("dataset name already exists")
				}
				txCode = common.CodeServerError
				return fmt.Errorf("dataset update error. (database error): %w", err)
			}
		}

		if connectorsProvided {
			scheduledTaskIDs, err = d.connectorDAO.LinkDatasetConnectorsTx(ctx, tx, lockedKB.ID, lockedKB.TenantID, connectorLinks)
			if err != nil {
				if dao.IsConnectorNotAccessibleErr(err) {
					txCode = common.CodeDataError
					return err
				}
				txCode = common.CodeServerError
				return fmt.Errorf("database operation failed: %w", err)
			}
		}

		updatedKB = &entity.Knowledgebase{}
		if err = tx.Where("id = ? AND status = ?", lockedKB.ID, string(entity.StatusValid)).First(updatedKB).Error; err != nil {
			txCode = common.CodeDataError
			return fmt.Errorf("dataset updated failed: %w", err)
		}

		linkedConnectors, err = d.connectorDAO.ListByDatasetIDTx(ctx, tx, lockedKB.ID)
		if err != nil {
			txCode = common.CodeServerError
			return fmt.Errorf("database operation failed: %w", err)
		}

		return nil
	})
	if err != nil {
		if txCode == common.CodeSuccess {
			txCode = common.CodeServerError
		}
		return nil, txCode, err
	}
	publishDatasetSyncerTasks(scheduledTaskIDs)

	if chunkStoreEmbdID != "" {
		if err = d.ensureChunkStoreForEmbedding(ctx, updatedKB, chunkStoreEmbdID); err != nil {
			// The update is already committed, so leaving the new model in place
			// would strand the dataset on a store without its vector column: a
			// retry sends the same model and skips the preparation entirely.
			// Restore the previous binding instead, so the retry takes the
			// change path and repairs the store.
			if restoreErr := d.restoreEmbeddingBinding(ctx, updatedKB.ID, chunkStoreEmbdID, previousEmbdID, previousTenantEmbdID); restoreErr != nil {
				return nil, common.CodeServerError, fmt.Errorf(
					"dataset settings were saved, but the chunk store preparation failed and the embedding binding could not be restored; select the embedding model again to repair the store: %w",
					errors.Join(err, restoreErr))
			}
			return nil, common.CodeServerError, fmt.Errorf(
				"dataset settings were saved, but the chunk store preparation failed; the embedding binding was restored so a retry repairs it: %w", err)
		}
	}

	if pagerankUpdate != nil {
		if err = d.updateDatasetPagerankChunks(ctx, *pagerankUpdate); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	data := datasetToMap(ctx, updatedKB)
	data["connectors"] = datasetConnectorsOrEmpty(linkedConnectors)
	return data, common.CodeSuccess, nil
}

// renameDatasetFolder updates an existing dataset folder in the caller's
// transaction. Datasets without a folder do not create one during a rename.
func renameDatasetFolder(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase, newName string) (common.ErrorCode, error) {
	var root entity.File
	err := tx.WithContext(ctx).
		Where("tenant_id = ? AND parent_id = id AND type = ?", kb.TenantID, "folder").
		Take(&root).Error
	if dao.IsNotFoundErr(err) {
		return common.CodeSuccess, nil
	}
	if err != nil {
		return common.CodeServerError, err
	}

	var kbRoot entity.File
	err = tx.WithContext(ctx).
		Where("tenant_id = ? AND parent_id = ? AND name = ?",
			kb.TenantID, root.ID, dao.DatasetFolderName).
		Take(&kbRoot).Error
	if dao.IsNotFoundErr(err) {
		return common.CodeSuccess, nil
	}
	if err != nil {
		return common.CodeServerError, err
	}
	if kbRoot.Type != "folder" {
		return common.CodeDataError, errors.New("Knowledgebase root is not a folder")
	}
	fileDAO := dao.NewFileDAO()

	entries := func() *gorm.DB {
		return tx.WithContext(ctx).Model(&entity.File{}).
			Where("tenant_id = ? AND parent_id = ?", kb.TenantID, kbRoot.ID)
	}
	var original entity.File
	// Uploads reuse entries by tenant, parent and name, regardless of source.
	err = entries().Where("name = ?", kb.Name).
		Take(&original).Error
	if err != nil && !dao.IsNotFoundErr(err) {
		return common.CodeServerError, err
	}
	if original.ID != "" && original.Type != "folder" {
		return common.CodeDataError, errors.New("Dataset path is not a folder")
	}

	exists, err := fileDAO.NameExists(ctx, tx, newName, kbRoot.ID, kb.TenantID, original.ID)
	if err != nil {
		return common.CodeServerError, err
	}
	if exists {
		return common.CodeDataError, fmt.Errorf("Dataset folder name '%s' already exists", newName)
	}
	if original.ID == "" {
		return common.CodeSuccess, nil
	}
	if err = entries().Where("id = ? AND type = ?", original.ID, "folder").Update("name", newName).Error; err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

func (d *DatasetService) updateDatasetPagerankChunks(ctx context.Context, update datasetPagerankUpdate) error {
	newCtx, cancel := context.WithTimeout(ctx, datasetPagerankUpdateTimeout)
	defer cancel()
	var err error
	if update.value > 0 {
		err = d.docEngine.UpdateChunks(newCtx, map[string]interface{}{"kb_id": update.datasetID}, map[string]interface{}{common.PAGERANK_FLD: update.value}, update.index, update.datasetID)
	} else {
		err = d.docEngine.UpdateChunks(newCtx, map[string]interface{}{"exists": common.PAGERANK_FLD}, map[string]interface{}{"remove": common.PAGERANK_FLD}, update.index, update.datasetID)
	}
	if errors.Is(err, types.ErrIndexNotFound) {
		// Python's docStoreConn.update logs and returns False on a missing
		// index; the dataset-level pagerank update tolerates it.
		return nil
	}
	return err
}

type datasetSyncTaskPublisher interface {
	PublishSyncerTask(taskID string) error
}

func publishDatasetSyncerTasks(taskIDs []string) {
	if len(taskIDs) == 0 {
		return
	}
	publisher, ok := engine.GetMessageQueueEngine().(datasetSyncTaskPublisher)
	if !ok {
		common.Warn("syncer task publisher is not configured")
		return
	}
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if err := publisher.PublishSyncerTask(taskID); err != nil {
			common.Warn("syncer task publish failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}
}

func (d *DatasetService) lockAccessibleDatasetForUpdate(ctx context.Context, tx *gorm.DB, datasetID string) (*entity.Knowledgebase, common.ErrorCode, error) {
	var kb entity.Knowledgebase
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", datasetID, string(entity.StatusValid)).
		First(&kb).Error
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, fmt.Errorf("database operation failed: %w", err)
	}

	err = permission.NewLockingDatabaseChecker(tx).CheckResource(
		ctx,
		permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID},
		permission.OperationUpdate,
	)
	if err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}

	return &kb, common.CodeSuccess, nil
}

// ensureChunkStoreForEmbedding prepares the chunk store for a new embedding
// model on an empty dataset. Every engine's CreateChunkStore is additive on an
// existing store: Infinity adds the missing q_<n>_vec column, the SQL engines
// add missing columns, and Elasticsearch leaves an existing chunk index
// untouched.
// restoreEmbeddingBinding reverts the model switch that the failed
// post-commit preparation could not complete. The guard on the new model keeps
// a concurrent switch from being overwritten.
func (d *DatasetService) restoreEmbeddingBinding(ctx context.Context, datasetID, preparedEmbdID, previousEmbdID string, previousTenantEmbdID *string) error {
	updates := map[string]interface{}{"embd_id": previousEmbdID}
	if previousTenantEmbdID != nil {
		updates["tenant_embd_id"] = *previousTenantEmbdID
	} else {
		updates["tenant_embd_id"] = nil
	}
	// The probe usually fails because the provider hangs and the request is
	// cancelled, so restoring the binding must not inherit that cancellation:
	// this write is what lets the retry repair the store.
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreEmbeddingBindingTimeout)
	defer cancel()
	var err error
	for attempt := 0; attempt < restoreEmbeddingBindingAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-restoreCtx.Done():
				return errors.Join(err, restoreCtx.Err())
			case <-time.After(restoreEmbeddingBindingRetry):
			}
		}
		err = dao.DB.WithContext(restoreCtx).Model(&entity.Knowledgebase{}).
			Where("id = ? AND embd_id = ?", datasetID, preparedEmbdID).
			Updates(updates).Error
		if err == nil {
			return nil
		}
	}
	common.Warn("failed to restore the embedding binding after chunk store preparation failed",
		zap.String("dataset_id", datasetID), zap.Error(err))
	return err
}

func (d *DatasetService) ensureChunkStoreForEmbedding(ctx context.Context, kb *entity.Knowledgebase, embdID string) error {
	if d.docEngine == nil || kb == nil {
		return nil
	}
	probe := d.embeddingDimension
	if probe == nil {
		factory := service.NewModelFactory()
		probe = func(ctx context.Context, tenantID, embdID string) (int, error) {
			return service.ProbeEmbeddingDimension(ctx, factory, tenantID, embdID)
		}
	}
	dimension, err := probe(ctx, kb.TenantID, embdID)
	if err != nil {
		return fmt.Errorf("resolve embedding dimension for %s: %w", embdID, err)
	}
	language := ""
	if kb.Language != nil {
		language = *kb.Language
	}
	if err := d.docEngine.CreateChunkStore(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), kb.ID, dimension, kb.ParserID, language); err != nil {
		return fmt.Errorf("prepare chunk store for embedding model %s: %w", embdID, err)
	}
	return nil
}
