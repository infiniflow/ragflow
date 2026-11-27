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

type datasetPagerankUpdate struct {
	value     int64
	index     string
	datasetID string
}

func (d *DatasetService) UpdateDataset(ctx context.Context, datasetID string, req service.UpdateDatasetRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	userID := strings.TrimSpace(permission.UserIDFromContext(ctx))
	// The row read here also feeds the embedding-switch guard below, which
	// costs an engine round trip and a provider call: it has to run before the
	// transaction opens rather than under the row lock.
	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
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

	// A switch to a model with a different dimension leaves every stored vector
	// in a column retrieval no longer queries. The console refuses it before
	// submitting; repeat the dimension half of that check here so an API client
	// cannot skip it. Authorize first: a caller who may not update the dataset
	// must neither trigger engine and provider work nor learn the dataset's
	// vector dimension from the rejection. The update is authorized again under
	// the row lock, which stays the authority.
	// switchValidated is only evidence for the binding the probe ran against:
	// the stored vectors it compared belong to that model, and a row that names
	// another one by the time the lock is taken — its chunks were rebuilt or
	// replaced in between — was never validated.
	switchValidated := false
	switchValidatedEmbdID := ""
	if embdIDProvided && embeddingSwitchNeedsCheck(kb, embdID) {
		if accessErr := permission.NewDatabaseChecker(dao.DB).CheckResource(
			ctx,
			permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID},
			permission.OperationShare,
		); accessErr != nil {
			code, permissionErr := permissionresponse.NormalizeHidden(accessErr)
			return nil, code, permissionErr
		}
		if code, switchErr := d.validateEmbeddingSwitch(ctx, kb, embdID); switchErr != nil {
			return nil, code, switchErr
		}
		switchValidated = true
		switchValidatedEmbdID = kb.EmbdID
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

	preparedLanguage := ""
	if language, ok := simpleUpdates["language"].(string); ok {
		preparedLanguage = language
	}
	storePlan, preflightCode, preflightErr := d.planChunkStoreForEmbeddingChange(ctx, kb, embdID, embdIDProvided, parserID, parserIDProvided, preparedLanguage)
	if preflightErr != nil {
		return nil, preflightCode, preflightErr
	}

	txCode := common.CodeSuccess
	var previousKB *entity.Knowledgebase
	var updatedKB *entity.Knowledgebase
	var linkedConnectors []*dao.ConnectorDatasetListItem
	var scheduledTaskIDs []string
	var pagerankUpdate *datasetPagerankUpdate
	// pagerankRetry is the same target for a request that repeats the stored
	// PageRank, kept so a retry of an earlier failed save can repeat the
	// propagation the value change no longer schedules.
	var pagerankRetry *datasetPagerankUpdate
	var chunkStoreEmbdID string
	// pendingEmbdID is the model of an empty dataset's switch, committed only
	// after its store is ready. pendingTenantEmbdID is its tenant binding.
	var pendingEmbdID string
	var pendingTenantEmbdID *string
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		lockedKB, code, authErr := d.lockAccessibleDatasetForUpdate(ctx, tx, datasetID)
		if authErr != nil {
			txCode = code
			return authErr
		}
		previousKB = lockedKB
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
			// Resolve the model in the dataset tenant without changing the caller.
			modelCtx := permission.WithIdentity(ctx, permission.UserIDFromContext(ctx), lockedKB.TenantID)
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
			switch {
			case effectiveEmbdID == lockedKB.EmbdID:
				// Nothing can fail after this transaction that would need the binding
				// to move, so it is updated like every other field.
				updates["embd_id"] = effectiveEmbdID
				updates["tenant_embd_id"] = stringPtrIfNotEmpty(tenantEmbdID)
			case lockedKB.ChunkNum != 0:
				// The probe ran against the row preflight read. Chunks committed
				// since then need that check, and so does a dataset whose binding
				// moved in between: its vectors were rebuilt or replaced, and the
				// recorded validation no longer describes what is stored. The probe
				// cannot run under this row lock, so both cases ask for a retry
				// that reruns the preflight against the current row.
				if !switchValidated || lockedKB.EmbdID != switchValidatedEmbdID {
					txCode = common.CodeServerError
					return errors.New("dataset changed while the embedding model was being prepared; retry the request")
				}
				updates["embd_id"] = effectiveEmbdID
				updates["tenant_embd_id"] = stringPtrIfNotEmpty(tenantEmbdID)
			default:
				// An empty dataset keeps the previous binding here. An existing store
				// was extended before this transaction; a missing one is created
				// afterwards, and only a successful creation commits the switch. A
				// failed one therefore leaves a row whose retry still prepares the
				// store instead of skipping it as an unchanged model.
				pendingEmbdID = effectiveEmbdID
				pendingTenantEmbdID = stringPtrIfNotEmpty(tenantEmbdID)
				chunkStoreEmbdID = effectiveEmbdID
			}
		}
		// The plan is decided before this transaction from a row that can still
		// change: a last chunk deleted in between brings the update here on the
		// empty-dataset path although no store was prepared for the new model.
		// Committing that switch would outlive the request, because a retry
		// resubmits the same model and an unchanged model is never prepared.
		// Without a document engine there is no store to prepare, and the plan
		// stays empty for that reason alone.
		if chunkStoreEmbdID != "" && d.docEngine != nil && !storePlan.covers(chunkStoreEmbdID) {
			txCode = common.CodeServerError
			return errors.New("dataset changed while the embedding model was being prepared; retry the request")
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

		// A save that does not touch parsing must not rewrite parser_config. The
		// settings form submits its whole parser_config on every save, and the
		// form's own round trip re-serialises it (retired params dropped, current
		// defaults filled in), so "parser_config was sent" is not evidence that the
		// user edited it: rebuilding on presence is what turned a language-only
		// edit into a parser_config change in the dataset's history. Rebuild only
		// when the parse selection moves, or when the submitted config is not a
		// verbatim copy of the stored one.
		parseSelectionChanged := parserIDProvided && parserID != lockedKB.ParserID
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
		parseSelectionChanged = parseSelectionChanged || pipelineChanged || versionChanged
		parserConfigSubmitted := req.ParserConfig != nil && len(req.ParserConfig) > 0 &&
			(parseSelectionChanged || !datasetJSONEqual(req.ParserConfig, lockedKB.ParserConfig))

		if parserConfigSubmitted {
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
				rebuilt := preserveDatasetParserConfigState(parserConfig, lockedKB.ParserConfig, req.ParserConfig)
				// The rebuild lands on the state the dataset already holds when the
				// submission described the same settings in another shape: the form
				// re-serialises the whole parser_config on every save, and its round
				// trip drops retired params and fills in the current defaults. Only a
				// rebuilt state that differs from the stored one is a change, so a
				// save that edits something else cannot rewrite - and version - the
				// parsing settings the user never touched.
				//
				// The comparison is against the current state, not the history: a
				// configuration that also appears in an older version and differs
				// from the head is still a change, and is recorded as one.
				if !datasetJSONEqual(rebuilt, lockedKB.ParserConfig) {
					updates["parser_config"] = rebuilt
				}
			}
		}
		if pagerankRequested {
			target := &datasetPagerankUpdate{
				value:     requestedPagerank,
				index:     fmt.Sprintf("ragflow_%s", lockedKB.TenantID),
				datasetID: lockedKB.ID,
			}
			if requestedPagerank != lockedKB.Pagerank {
				if !d.docEngine.SupportsPageRank() {
					txCode = common.CodeDataError
					return errors.New("'pagerank' can only be set when doc_engine is elasticsearch")
				}
				pagerankUpdate = target
				updates["pagerank"] = requestedPagerank
			} else if d.docEngine.SupportsPageRank() {
				pagerankRetry = target
			}
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

		// pipelineChanged and versionChanged were computed with the parser_config
		// decision above: both are "the parse selection moved" signals.
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
			// Scoping the config rewrites the submitted shape a second time: it
			// replaces every Extractor's metadata with the dataset-level one and
			// drops the legacy flat keys. A state that differed from the stored one
			// before this step can therefore land back on it here, and a state
			// copied from the stored config (the embedding-only path above) starts
			// out equal. Compare once more and leave the column alone when the
			// result is the state it already holds: an unchanged configuration must
			// not be rewritten, because the write is also the only thing a version
			// could record for a save the user made elsewhere on the form.
			if datasetJSONEqual(effectiveParserConfig, lockedKB.ParserConfig) {
				delete(updates, "parser_config")
			} else {
				updates["parser_config"] = effectiveParserConfig
			}
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

		// A relink overwrites the bindings the version writer has to record as
		// the state this change replaced, so read them first. Only a request that
		// touches the bindings needs it; any other change leaves the table alone,
		// and the writer reads it there.
		var previousBindings []entity.JSONMap
		if connectorsProvided {
			previousBindings, err = d.datasetConfigBindingsEE(ctx, tx, lockedKB)
			if err != nil {
				txCode = common.CodeServerError
				return errors.New("database operation failed")
			}
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

		// Version the change: the row the transaction locked is the state it
		// started from (version 0 is seeded from it when the dataset has no
		// history yet) and updatedKB is the state it produced. A failure fails the
		// whole update, because a committed configuration change without a version
		// would be invisible to the history.
		if len(updates) > 0 || connectorsProvided {
			if _, versionErr := d.recordDatasetConfigVersionTxEE(ctx, tx, lockedKB, updatedKB, previousBindings, entity.DatasetVersionKindDatasetConfig, userID, 0); versionErr != nil {
				txCode = common.CodeServerError
				return versionErr
			}
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
	// The saved PageRank has to reach the chunk store before any step that can
	// fail after the commit: a retry sees the value already persisted, so its
	// requested value equals the locked one and only the recovery below has to
	// repeat the write.
	if pagerankUpdate != nil {
		if err = d.updateDatasetPagerankChunks(ctx, *pagerankUpdate); err != nil {
			return nil, common.CodeServerError, err
		}
	}
	var recoveredTaskIDs []string
	if pendingEmbdID != "" {
		if storePlan.createEmbdID != "" && updatedKB != nil && updatedKB.ChunkNum == 0 {
			if createErr := d.ensureChunkStoreForEmbedding(ctx, updatedKB, storePlan.createEmbdID); createErr != nil {
				// This request never wrote the binding, so a retry prepares the
				// store again. Which model the row names by now is not stated: a
				// concurrent update may have moved it since the transaction.
				return nil, common.CodeServerError, fmt.Errorf(
					"dataset settings were saved, but the chunk store for embedding model %s could not be created and the embedding model was not switched; retry the request to switch: %w",
					storePlan.createEmbdID, createErr)
			}
		}
		// Recovered tasks are read before the binding moves: a failed read after
		// the switch would leave them pending with nothing to re-enter this path,
		// because an unchanged model never repeats the recovery.
		recoveredTaskIDs, err = pendingDatasetSyncTaskIDs(ctx, updatedKB)
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf(
				"dataset settings were saved, but the connector tasks of an earlier attempt could not be read, so the embedding model was not switched; retry the request: %w", err)
		}
		if bindErr := d.commitEmbeddingBinding(ctx, previousKB, updatedKB, pendingEmbdID, pendingTenantEmbdID); bindErr != nil {
			return nil, common.CodeServerError, fmt.Errorf(
				"dataset settings were saved, but the embedding model could not be switched: %w", bindErr)
		}
	} else if embdIDProvided || connectorsProvided {
		// The binding is settled here: this transaction moved it because the
		// dataset held chunks by then, or the row already named the requested
		// model. Either way an earlier attempt can have committed the same links
		// and tasks without publishing them, and a retry that repeats the model
		// has no other path into this recovery. A connector-only retry reaches
		// the same state: the links it resubmits already exist, so it gets no
		// task IDs of its own and the rows an attempt that failed after the
		// commit (pagerank propagation) still need their publish.
		recoveredTaskIDs, err = pendingDatasetSyncTaskIDs(ctx, updatedKB)
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf(
				"dataset settings were saved, but the connector tasks of an earlier attempt could not be read; retry the request: %w", err)
		}
	}
	// A retry of a save that failed after its commit still finds the rows that
	// attempt committed, and it repeats the same PageRank the failed attempt had
	// already saved, so no value change remains to schedule a propagation;
	// repeat the idempotent write here so a chunk-store write that failed before
	// the first publication is repaired before the recovered task runs. Adding
	// another connector in the same request schedules a task of its own, so the
	// trigger is a recovered row this request did not schedule.
	if pagerankRetry != nil && recoveredOutsideScheduled(scheduledTaskIDs, recoveredTaskIDs) {
		if err = d.updateDatasetPagerankChunks(ctx, *pagerankRetry); err != nil {
			return nil, common.CodeServerError, fmt.Errorf(
				"dataset settings were saved, but the PageRank of an earlier attempt could not be re-applied to the chunk store; retry the request: %w", err)
		}
	}
	// Publish connector tasks only after the chunk store is ready: a task the
	// syncer picks up earlier would run against a store whose vector column for
	// the new model does not exist yet. A request that repairs the store of an
	// earlier failed attempt has no task IDs of its own, because the links it
	// re-submits already existed, so the rows that attempt committed still need
	// their publish.
	publishIDs := scheduledTaskIDs
	publishIDs = append(publishIDs, recoveredTaskIDs...)
	publishDatasetSyncerTasks(publishIDs)

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
	published := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if _, dup := published[taskID]; dup {
			continue
		}
		published[taskID] = struct{}{}
		if err := publisher.PublishSyncerTask(taskID); err != nil {
			common.Warn("syncer task publish failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}
}

// recoveredOutsideScheduled reports whether recovered holds a task ID the
// transaction did not schedule itself: that row comes from an attempt that
// committed it and failed before publication.
func recoveredOutsideScheduled(scheduled, recovered []string) bool {
	if len(recovered) == 0 {
		return false
	}
	scheduledIDs := make(map[string]struct{}, len(scheduled))
	for _, id := range scheduled {
		scheduledIDs[id] = struct{}{}
	}
	for _, id := range recovered {
		if _, ok := scheduledIDs[id]; !ok {
			return true
		}
	}
	return false
}

// pendingDatasetSyncTaskIDs returns the dataset's still-scheduled connector
// tasks, which are the ones a failed attempt committed without publishing.
func pendingDatasetSyncTaskIDs(ctx context.Context, kb *entity.Knowledgebase) ([]string, error) {
	if ctx == nil || kb == nil {
		return nil, nil
	}
	var ids []string
	err := dao.DB.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("kb_id = ? AND status = ?", kb.ID, dao.SyncStatusSchedule).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("read the dataset's scheduled syncer tasks: %w", err)
	}
	return ids, nil
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
		permission.OperationShare,
	)
	if err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}

	return &kb, common.CodeSuccess, nil
}

// chunkStorePlan is what the pre-transaction store check decided.
type chunkStorePlan struct {
	// storeExists reports whether the dataset's chunk store existed before the
	// transaction.
	storeExists bool
	// preparedEmbdID is the model whose vector column was added to the existing
	// store before the transaction. Additive writes are safe there: a rejected
	// update leaves every column the row still needs.
	preparedEmbdID string
	// createEmbdID is the model whose store must be created after the commit.
	// Creating a store fixes its analyzer and dimensions, so doing it before a
	// transaction that can still roll back would leave a store the row never
	// asked for. The binding moves only after creation succeeds, so a failed
	// creation leaves the retry on the preparation path.
	createEmbdID string
}

// covers reports whether this plan prepared the store for model. The transaction
// uses it to reject a switch whose preparation the plan decided not to run.
func (p chunkStorePlan) covers(model string) bool {
	return model != "" && (p.preparedEmbdID == model || p.createEmbdID == model)
}

// planChunkStoreForEmbeddingChange decides how an empty dataset's chunk store
// is prepared for an embedding-model change. The caller passes the parser and
// language the same update is about to persist, because engines fix both when
// they create the store.
func (d *DatasetService) planChunkStoreForEmbeddingChange(ctx context.Context, kb *entity.Knowledgebase, embdID string, embdIDProvided bool, parserID string, parserIDProvided bool, language string) (chunkStorePlan, common.ErrorCode, error) {
	var plan chunkStorePlan
	if d.docEngine == nil || kb == nil || !embdIDProvided || embdID == "" || kb.ChunkNum != 0 {
		return plan, common.CodeSuccess, nil
	}
	// Re-submitting the current model is not a model change. Probing the
	// provider here would make a rename fail while the provider is down, and a
	// store missing in that state is not something this update can have left
	// behind: a failed creation never moves the binding, so the retry of that
	// request still sees a model change.
	if embdID == kb.EmbdID {
		return plan, common.CodeSuccess, nil
	}
	// Gate the store probes behind the update permission: reaching this point
	// only required reading the row, and a caller without update access must
	// not be able to tell whether a dataset has a chunk store from the error
	// or timing of the probes below. The transaction re-checks under the row
	// lock, which stays authoritative against a permission change in between.
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: kb.ID}, permission.OperationUpdate); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return plan, code, permissionErr
	}
	exists, err := d.docEngine.ChunkStoreExists(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), kb.ID)
	if err != nil {
		return plan, common.CodeServerError, fmt.Errorf("check the chunk store for dataset %s: %w", kb.ID, err)
	}
	plan.storeExists = exists
	if !exists {
		plan.createEmbdID = embdID
		return plan, common.CodeSuccess, nil
	}
	// Keep the unavailable-model contract of the transaction below: the probe
	// fails first for an unknown model, and that is a client error.
	if ok, message := d.verifyEmbeddingAvailability(ctx, embdID, kb.TenantID); !ok {
		return plan, common.CodeDataError, errors.New(message)
	}
	preparedKB := *kb
	if parserIDProvided {
		preparedKB.ParserID = parserID
	}
	if language != "" {
		preparedKB.Language = &language
	}
	if err := d.ensureChunkStoreForEmbedding(ctx, &preparedKB, embdID); err != nil {
		return plan, common.CodeServerError, fmt.Errorf("prepare the chunk store for embedding model %s: %w", embdID, err)
	}
	plan.preparedEmbdID = embdID
	return plan, common.CodeSuccess, nil
}

// commitEmbeddingBinding switches an empty dataset to the model whose store was
// prepared. The previous binding is the guard: a chunk that landed first, or a
// concurrent settings write, leaves the row on the model its chunks actually
// used, and the caller reports the failure instead of claiming a switch that
// did not happen.
func (d *DatasetService) commitEmbeddingBinding(ctx context.Context, previous, kb *entity.Knowledgebase, embdID string, tenantEmbdID *string) error {
	if previous == nil || kb == nil {
		return errors.New("the dataset row locked by the update is not available")
	}
	updates := map[string]interface{}{"embd_id": embdID}
	if tenantEmbdID == nil {
		updates["tenant_embd_id"] = nil
	} else {
		updates["tenant_embd_id"] = *tenantEmbdID
	}
	result := dao.DB.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id = ? AND embd_id = ? AND chunk_num = 0", kb.ID, previous.EmbdID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// A concurrent request may have made the same switch already; a new chunk
		// or a different model must be reported instead of overwritten.
		fresh, err := d.kbDAO.GetByID(ctx, dao.DB, kb.ID)
		if err != nil {
			return err
		}
		if fresh.EmbdID != embdID || fresh.ChunkNum != 0 {
			return fmt.Errorf("the dataset changed while its embedding model was being switched (model %s, %d chunks); retry the request",
				fresh.EmbdID, fresh.ChunkNum)
		}
		kb.TenantEmbdID = fresh.TenantEmbdID
	}
	kb.EmbdID = embdID
	if tenantEmbdID != nil {
		kb.TenantEmbdID = tenantEmbdID
	}
	return nil
}

// ensureChunkStoreForEmbedding prepares the chunk store for a new embedding
// model on an empty dataset. Every engine's CreateChunkStore is additive on an
// existing store: Infinity adds the missing q_<n>_vec column, the SQL engines
// add missing columns, and Elasticsearch leaves an existing chunk index
// untouched.
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
	if err := d.createChunkStore(ctx, kb, dimension); err != nil {
		// Creation is additive on every engine, and the engines that write a
		// table before its indexes can complete a half-built store by running
		// the same request again. Retrying here keeps a transient failure from
		// leaving a store a later request would accept as ready.
		if retryErr := d.createChunkStore(ctx, kb, dimension); retryErr != nil {
			return fmt.Errorf("prepare chunk store for embedding model %s: %w", embdID, retryErr)
		}
	}
	return nil
}

func (d *DatasetService) createChunkStore(ctx context.Context, kb *entity.Knowledgebase, dimension int) error {
	language := ""
	if kb.Language != nil {
		language = *kb.Language
	}
	return d.docEngine.CreateChunkStore(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), kb.ID, dimension, kb.ParserID, language)
}
