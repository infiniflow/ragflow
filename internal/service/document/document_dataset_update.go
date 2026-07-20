package document

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"ragflow/internal/service"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

func (s *DocumentService) BatchUpdateDocumentStatus(userID, datasetID, status string, documentIDs []string) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, userID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("You don't own the dataset.")
	}
	statusInt, convErr := strconv.Atoi(status)
	if convErr != nil {
		return nil, common.CodeArgumentError, fmt.Errorf("invalid status: %s", status)
	}

	result := make(map[string]interface{}, len(documentIDs))
	hasError := false

	documents, err := s.documentDAO.GetByIDs(documentIDs)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to fetch documents: %w", err)
	}
	documentByID := make(map[string]*entity.Document, len(documents))
	for _, doc := range documents {
		documentByID[doc.ID] = doc
	}

	for _, docID := range documentIDs {
		doc, ok := documentByID[docID]
		if !ok {
			result[docID] = map[string]string{"error": "Document not found"}
			hasError = true
			continue
		}

		if doc.KbID != datasetID {
			result[docID] = map[string]string{"error": "Document not found in this dataset."}
			hasError = true
			continue
		}

		currentStatus := ""
		if doc.Status != nil {
			currentStatus = *doc.Status
		}
		if currentStatus == status {
			result[docID] = map[string]string{"status": status}
			continue
		}
		previousStatus := interface{}(nil)
		if doc.Status != nil {
			previousStatus = *doc.Status
		}
		if err := s.documentDAO.UpdateByID(docID, map[string]interface{}{"status": status}); err != nil {
			result[docID] = map[string]string{"error": "Database error (Document update)!"}
			hasError = true
			continue
		}

		if doc.ChunkNum > 0 {
			if s.docEngine == nil {
				_ = s.documentDAO.UpdateByID(docID, map[string]interface{}{"status": previousStatus})
				result[docID] = map[string]string{"error": "Document store update failed: document engine not initialized"}
				hasError = true
				continue
			}
			err := s.docEngine.UpdateChunks(
				context.Background(),
				map[string]interface{}{"doc_id": docID},
				map[string]interface{}{"available_int": statusInt},
				fmt.Sprintf("ragflow_%s", kb.TenantID),
				doc.KbID,
			)
			if err != nil {
				_ = s.documentDAO.UpdateByID(docID, map[string]interface{}{"status": previousStatus})
				msg := err.Error()
				if strings.Contains(msg, "3022") {
					result[docID] = map[string]string{"error": "Document store table missing."}
				} else {
					result[docID] = map[string]string{"error": "Document store update failed: " + msg}
				}
				hasError = true
				continue
			}
		}
		result[docID] = map[string]string{"status": status}
	}

	if hasError {
		return result, common.CodeServerError, fmt.Errorf("Partial failure")
	}
	return result, common.CodeSuccess, nil
}

func (s *DocumentService) UpdateDatasetDocument(userID, datasetID, documentID string, req *UpdateDatasetDocumentRequest, present map[string]bool) (*UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	tenantID := userID
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("You don't own the dataset.")
		}
		return nil, common.CodeDataError, errors.New("Can't find this dataset!")
	}

	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("The dataset doesn't own the document.")
		}
		return nil, common.CodeServerError, err
	}

	if code, err := s.validateDatasetDocumentUpdate(datasetID, documentID, userID, doc, req, present); err != nil {
		return nil, code, err
	}

	if present["meta_fields"] {
		if err := s.replaceDocumentMetadata(documentID, req.MetaFields); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["name"] && req.Name != nil && (doc.Name == nil || *req.Name != *doc.Name) {
		if err := s.updateDocumentNameOnly(doc, kb.TenantID, *req.Name); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["parser_config"] && req.ParserConfig != nil {
		// Resolve effective pipeline to load the DSL for cleaning.
		isCanvas := kb.PipelineID != nil && strings.TrimSpace(*kb.PipelineID) != ""
		if req.PipelineID != nil {
			isCanvas = strings.TrimSpace(*req.PipelineID) != ""
		}
		if req.ParserID != nil {
			isCanvas = false
		}
		effParserID := kb.ParserID
		if req.ParserID != nil {
			effParserID = strings.TrimSpace(*req.ParserID)
		}
		effPipelineID := kb.PipelineID
		if req.PipelineID != nil {
			effPipelineID = req.PipelineID
		}
		if req.ParserID != nil && req.PipelineID == nil && kb.PipelineID != nil {
			effPipelineID = nil
		}

		dslJSON, err := service.LoadPipelineDSL(isCanvas, effParserID, effPipelineID)
		if err != nil {
			common.Warn("cleanAndUpdateDocumentParserConfig: failed to load DSL, falling back to merge",
				zap.Error(err))
			if err := s.updateDocumentParserConfig(doc.ID, req.ParserConfig); err != nil {
				return nil, common.CodeDataError, err
			}
		} else {
			cleaned := pipelinepkg.BuildParserConfig(dslJSON, map[string]interface{}(req.ParserConfig))
			if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{
				"parser_config": cleaned,
			}); err != nil {
				return nil, common.CodeDataError, err
			}
		}
	}

	if present["pipeline_id"] {
		if req.PipelineID != nil && strings.TrimSpace(*req.PipelineID) != "" {
			if err := s.resetDocumentForReparse(doc, kb.TenantID, nil, req.PipelineID); err != nil {
				return nil, common.CodeDataError, err
			}
		} else {
			// Explicitly cleared: drop the custom canvas so the worker falls
			// back to the built-in template, matching validation.
			empty := ""
			if err := s.resetDocumentForReparse(doc, kb.TenantID, nil, &empty); err != nil {
				return nil, common.CodeDataError, err
			}
		}
	} else if present["parser_id"] && req.ParserID != nil && strings.TrimSpace(*req.ParserID) != "" {
		parserID := strings.TrimSpace(*req.ParserID)
		if err := s.resetDocumentForReparse(doc, kb.TenantID, &parserID, nil); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["enabled"] && req.Enabled != nil {
		if err := s.updateDocumentStatusOnly(doc, kb, *req.Enabled); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	updatedDoc, err := s.documentDAO.GetByID(doc.ID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("Can not get document by id:%s", doc.ID)
		}
		return nil, common.CodeDataError, errors.New("Database operation failed")
	}

	metaFields := map[string]interface{}{}
	if s.docEngine != nil && s.metadataSvc != nil {
		metaFields, _ = s.GetDocumentMetadataByID(updatedDoc.ID)
	}

	return s.toUpdateDatasetDocumentResponse(updatedDoc, metaFields), common.CodeSuccess, nil
}

func (s *DocumentService) validateDatasetDocumentUpdate(datasetID, documentID, userID string, doc *entity.Document, req *UpdateDatasetDocumentRequest, present map[string]bool) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("Invalid request payload")
	}
	if present["chunk_count"] && req.ChunkCount != nil && *req.ChunkCount != 0 && *req.ChunkCount != doc.ChunkNum {
		return common.CodeDataError, errors.New("Can't change `chunk_count`.")
	}
	if present["token_count"] && req.TokenCount != nil && *req.TokenCount != 0 && *req.TokenCount != doc.TokenNum {
		return common.CodeDataError, errors.New("Can't change `token_count`.")
	}
	if present["progress"] && req.Progress != nil {
		if *req.Progress > 1 {
			return common.CodeDataError, fmt.Errorf("Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <%v>", *req.Progress)
		}
		if *req.Progress != 0 && math.Abs(*req.Progress-doc.Progress) > 1e-9 {
			return common.CodeDataError, errors.New("Can't change `progress`.")
		}
	}

	if present["enabled"] {
		if req.Enabled == nil || (*req.Enabled != 0 && *req.Enabled != 1) {
			return common.CodeDataError, errors.New("`enabled` value invalid, only accept 0 or 1")
		}
	}

	if present["parser_id"] && req.ParserID != nil {
		parserID := strings.TrimSpace(*req.ParserID)
		if (doc.Type == "visual" && parserID != "picture") || (isPresentationFile(doc.Name) && parserID != "presentation") {
			return common.CodeDataError, errors.New("Not supported yet!")
		}
	}
	if present["name"] && req.Name != nil {
		if err := s.validateDocumentName(doc, *req.Name); err != nil {
			return common.CodeDataError, err
		}
	}

	if present["meta_fields"] {
		if err := validateMetaFields(req.MetaFields); err != nil {
			return common.CodeDataError, err
		}
	}

	return common.CodeSuccess, nil
}

func (s *DocumentService) validateDocumentName(doc *entity.Document, newName string) error {
	if strings.TrimSpace(newName) == "" {
		return errors.New("File name can't be empty.")
	}
	if len([]byte(newName)) > 255 {
		return errors.New("File name must be 255 bytes or less.")
	}

	oldName := ""
	if doc.Name != nil {
		oldName = *doc.Name
	}

	if strings.ToLower(filepath.Ext(newName)) != strings.ToLower(filepath.Ext(oldName)) {
		return errors.New("The extension of file can't be changed")
	}

	docs, err := s.documentDAO.GetByNameAndKBID(newName, doc.KbID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		if d.ID != doc.ID && d.Name != nil && *d.Name == newName {
			return errors.New("Duplicated document name in the same dataset.")
		}
	}

	return nil
}

func isPresentationFile(name *string) bool {
	if name == nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(*name))
	return ext == ".ppt" || ext == ".pptx" || ext == ".pages"
}

func validateMetaFields(meta map[string]any) error {
	if meta == nil {
		return nil
	}

	for _, v := range meta {
		switch typed := v.(type) {
		case string, float64, int, int64, float32:
			continue
		case []any:
			for _, item := range typed {
				switch item.(type) {
				case string, float64, int, int64, float32:
					continue
				default:
					return fmt.Errorf("The type is not supported in list: %v", typed)
				}
			}
		default:
			return fmt.Errorf("The type is not supported: %v", v)
		}
	}

	return nil
}

func (s *DocumentService) updateDocumentNameOnly(doc *entity.Document, tenantID, newName string) error {
	if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"name": newName}); err != nil {
		return errors.New("Database error (Document rename)!")
	}

	mappings, err := s.file2DocumentDAO.GetByDocumentID(doc.ID)
	if err == nil && len(mappings) > 0 && mappings[0].FileID != nil && s.fileDAO != nil {
		_ = s.fileDAO.UpdateByID(*mappings[0].FileID, map[string]interface{}{"name": newName})
	}

	if s.docEngine == nil {
		return nil
	}

	titleTks, _ := tokenizer.Tokenize(newName)
	titleSmTks, _ := tokenizer.FineGrainedTokenize(titleTks)
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	return s.docEngine.UpdateChunks(
		context.Background(),
		map[string]interface{}{"doc_id": doc.ID},
		map[string]interface{}{
			"docnm_kwd":    newName,
			"title_tks":    titleTks,
			"title_sm_tks": titleSmTks,
		},
		indexName,
		doc.KbID,
	)
}

func (s *DocumentService) updateDocumentParserConfig(documentID string, config map[string]any) error {
	if len(config) == 0 {
		return nil
	}

	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil {
		return fmt.Errorf("Document(%s) not found.", documentID)
	}

	merged := common.DeepMergeMaps(map[string]interface{}(doc.ParserConfig), map[string]interface{}(config))
	if _, ok := config["raptor"]; !ok {
		delete(merged, "raptor")
	}

	return s.documentDAO.UpdateByID(documentID, map[string]interface{}{
		"parser_config": entity.JSONMap(merged),
	})
}

func (s *DocumentService) toUpdateDatasetDocumentResponse(doc *entity.Document, metaFields map[string]interface{}) *UpdateDatasetDocumentResponse {
	if metaFields == nil {
		metaFields = map[string]interface{}{}
	}
	return &UpdateDatasetDocumentResponse{
		ID:              doc.ID,
		Thumbnail:       doc.Thumbnail,
		DatasetID:       doc.KbID,
		ParserID:        doc.ParserID,
		PipelineID:      doc.PipelineID,
		ParserConfig:    map[string]interface{}(doc.ParserConfig),
		SourceType:      doc.SourceType,
		Type:            doc.Type,
		CreatedBy:       doc.CreatedBy,
		Name:            doc.Name,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenCount:      doc.TokenNum,
		ChunkCount:      doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessBeginAt:  doc.ProcessBeginAt,
		ProcessDuration: doc.ProcessDuration,
		ContentHash:     doc.ContentHash,
		MetaFields:      metaFields,
		Suffix:          doc.Suffix,
		Run:             mapDocumentRunStatus(doc.Run),
		Status:          doc.Status,
		CreateTime:      doc.CreateTime,
		CreateDate:      doc.CreateDate,
		UpdateTime:      doc.UpdateTime,
		UpdateDate:      doc.UpdateDate,
	}
}

func mapDocumentRunStatus(run *string) string {
	if run == nil {
		return "UNSTART"
	}
	switch *run {
	case string(entity.TaskStatusRunning):
		return "RUNNING"
	case string(entity.TaskStatusCancel):
		return "CANCEL"
	case string(entity.TaskStatusDone):
		return "DONE"
	case string(entity.TaskStatusFail):
		return "FAIL"
	default:
		return "UNSTART"
	}
}
