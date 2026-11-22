package document

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	componentpkg "ragflow/internal/ingestion/component"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	"ragflow/internal/utility"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplyDatasetParserConfigResponse struct {
	DatasetID          string `json:"dataset_id"`
	TotalDocuments     int64  `json:"total_documents"`
	UpdatedDocuments   int64  `json:"updated_documents"`
	UnchangedDocuments int64  `json:"unchanged_documents"`
}

// ApplyDatasetParserConfig resets document parser settings without changing
// parsed content or scheduling ingestion. Dataset grants cover its documents.
func (s *DocumentService) ApplyDatasetParserConfig(ctx context.Context, datasetID string) (*ApplyDatasetParserConfigResponse, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("dataset_id is required")
	}
	result := &ApplyDatasetParserConfigResponse{DatasetID: datasetID}
	code := common.CodeServerError
	// Every document the transaction rewrites, so the change lands on the
	// dataset timeline as one version instead of an untraceable row update, and
	// its settings land in the document's own history. The rows are built from
	// the settings the transaction already resolved, which saves reading the
	// documents back (parser_config is the largest column they carry).
	changedIDs := make([]string, 0)
	changedDocs := make([]*entity.Document, 0)
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", datasetID, string(entity.StatusValid)).Take(&kb).Error; err != nil {
			if dao.IsNotFoundErr(err) {
				var normalized error
				code, normalized = permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
				return normalized
			}
			return err
		}
		checker := permission.NewDatabaseChecker(tx)
		ref := permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}
		if err := checker.CheckResource(ctx, ref, permission.OperationUpdate); err != nil {
			var normalized error
			code, normalized = permissionresponse.NormalizeHidden(err)
			return normalized
		}
		if kb.PipelineID != nil && strings.TrimSpace(*kb.PipelineID) != "" {
			if err := checker.CheckDependency(ctx, ref,
				permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: *kb.PipelineID},
				permission.OperationUpdate, permission.OperationUse); err != nil {
				var normalized error
				code, normalized = permissionresponse.Normalize(err)
				return normalized
			}
		}
		selection := service.CurrentSelection(kb.ParserID, kb.PipelineID)
		isPipeline, parserID, pipelineID := selection.Effective()
		if _, err := service.LoadPipelineDSL(ctx, isPipeline, parserID, pipelineID, kb.PipelineVersionID); err != nil {
			code = common.CodeDataError
			return err
		}

		type parserSettings struct {
			parserID   string
			pipelineID *string
			versionID  *string
			config     entity.JSONMap
			encoded    []byte
		}
		// Resolution depends on file family and its dedicated parser, not file
		// identity. Keep one settings snapshot per family for the entire request.
		settingsByFamily := make(map[string]*parserSettings)
		lastID := ""
		for {
			var docs []entity.Document
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("kb_id = ? AND id > ?", datasetID, lastID).
				Order("id").Limit(200).Find(&docs).Error; err != nil {
				return err
			}
			if len(docs) == 0 {
				break
			}
			ids := make([]string, len(docs))
			for i := range docs {
				ids[i] = docs[i].ID
			}
			// Lock in document -> task order, matching ingestion run creation.
			// A new run cannot be published until its document lock is released.
			var tasks []entity.IngestionTask
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("document_id IN ?", ids).Order("document_id").Find(&tasks).Error; err != nil {
				return err
			}
			for _, task := range tasks {
				if !common.IsTerminalTaskStatus(task.Status) {
					code = common.CodeConflict
					return fmt.Errorf("document %s has ingestion status %q; stop parsing or wait for it to finish before applying parser settings", task.DocumentID, task.Status)
				}
			}
			changedByFamily := make(map[string][]string)
			for _, doc := range docs {
				name := "file." + doc.Suffix
				if doc.Name != nil {
					name = *doc.Name
				}
				filetype := utility.FileType(doc.Type)
				family := componentpkg.ParserFileFamily(utility.GetFileExtension(name))
				key := family + ":" + dedicatedDocumentParserID(filetype, name)
				settings := settingsByFamily[key]
				if settings == nil {
					sel, config := resolveDocumentParser(ctx, &kb, name, filetype, cloneParserConfigForDocument(kb.ParserConfig))
					parserID, pipelineID := parserSelectionColumns(sel)
					var versionID *string
					if sel.IsPipeline() {
						versionID = kb.PipelineVersionID
					}
					if declared, found := configDeclaredFamily(config, family); family != "" && found && !declared {
						code = common.CodeDataError
						return fmt.Errorf("could not resolve parser settings for file type %q", family)
					}
					encoded, err := json.Marshal(config)
					if err != nil {
						return err
					}
					settings = &parserSettings{parserID, pipelineID, versionID, config, encoded}
					settingsByFamily[key] = settings
				}
				encoded, err := json.Marshal(doc.ParserConfig)
				if err != nil {
					return err
				}
				result.TotalDocuments++
				if doc.ParserID == settings.parserID && sameOptionalString(doc.PipelineID, settings.pipelineID) &&
					sameOptionalString(doc.PipelineVersionID, settings.versionID) && bytes.Equal(encoded, settings.encoded) {
					result.UnchangedDocuments++
					continue
				}
				changedByFamily[key] = append(changedByFamily[key], doc.ID)
			}
			for key, ids := range changedByFamily {
				settings := settingsByFamily[key]
				if err := tx.Model(&entity.Document{}).Where("kb_id = ? AND id IN ?", datasetID, ids).
					Updates(map[string]any{
						"parser_id": settings.parserID, "pipeline_id": settings.pipelineID,
						"pipeline_version_id": settings.versionID, "parser_config": settings.config,
					}).Error; err != nil {
					return err
				}
				result.UpdatedDocuments += int64(len(ids))
				changedIDs = append(changedIDs, ids...)
				for _, id := range ids {
					changedDocs = append(changedDocs, &entity.Document{
						ID: id, KbID: datasetID,
						ParserID:          settings.parserID,
						PipelineID:        settings.pipelineID,
						PipelineVersionID: settings.versionID,
						ParserConfig:      settings.config,
					})
				}
			}
			lastID = docs[len(docs)-1].ID
		}
		return nil
	})
	if err != nil {
		return nil, code, err
	}
	if len(changedIDs) > 0 {
		// The version and the settings it names are one write: a history that
		// could not take them fails the request rather than leaving documents
		// that moved with no state a replay can read.
		if err := recordDocumentVersionSettingsTx(ctx, datasetID,
			documentVersionChanges(entity.DatasetVersionChangeActionUpdate, documentNamesByID(ctx, changedIDs)),
			changedDocs); err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to record the settings history: %w", err)
		}
	}
	return result, common.CodeSuccess, nil
}

func sameOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
