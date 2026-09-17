package document

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// escapeSQLLikePattern escapes the SQL LIKE wildcards ('%', '_') and
// the escape character itself ('!') so a literal user-supplied
// filename can be safely interpolated into a `LIKE ? ESCAPE '!'`
// pattern. Without this, "%.png" would match any string ending in
// ".png" and "_" would match a single character — bypassing the
// filename-specific authorization check. PR review round 5, Major #8.
func escapeSQLLikePattern(s string) string {
	r := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)
	return r.Replace(s)
}

// ListDocuments list documents
func (s *DocumentService) ListDocuments(ctx context.Context, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.List(ctx, dao.DB, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	docIDs := make([]string, 0, len(documents))
	for _, doc := range documents {
		if doc != nil && doc.ID != "" {
			docIDs = append(docIDs, doc.ID)
		}
	}
	var taskMap map[string]*entity.IngestionTask
	if s.ingestionTaskDAO != nil && len(docIDs) > 0 {
		var err error
		taskMap, err = s.ingestionTaskDAO.GetLatestByDocumentIDs(ctx, dao.DB, docIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get ingestion tasks for documents: %w", err)
		}
	}
	latestEventsByDocument, err := s.latestIngestionEventsByDocument(ctx, taskMap)
	if err != nil {
		common.Warn(fmt.Sprintf("failed to get latest ingestion events for documents: %v", err))
		latestEventsByDocument = make(map[string]*service.IngestionEventItem)
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		var task *entity.IngestionTask
		var latestEvent *service.IngestionEventItem
		if taskMap != nil && doc != nil {
			task = taskMap[doc.ID]
			latestEvent = latestEventsByDocument[doc.ID]
		}
		responses[i] = s.toResponseWithTask(doc, task, latestEvent)
	}

	return responses, total, nil
}

func (s *DocumentService) GetThumbnails(ctx context.Context, userID string, docIDs []string) (map[string]string, error) {
	if len(docIDs) == 0 {
		return map[string]string{}, nil
	}

	tenantIDs := []string{userID}
	if userID != "" {
		ids, err := dao.NewUserTenantDAO().GetTenantIDsByUserID(ctx, dao.DB, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch user tenants: %w", err)
		}
		tenantIDs = append(tenantIDs, ids...)
	}

	documents, err := s.documentDAO.GetByIDsAndTenantIDs(ctx, dao.DB, docIDs, tenantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document thumbnails: %w", err)
	}

	result := make(map[string]string, len(documents))
	for _, document := range documents {
		if document == nil {
			continue
		}

		thumbnail := ""
		if document.Thumbnail != nil && *document.Thumbnail != "" {
			if strings.HasPrefix(*document.Thumbnail, imgBase64Prefix) {
				thumbnail = *document.Thumbnail
			} else {
				thumbnail = fmt.Sprintf(
					"/api/v1/documents/images/%s-%s",
					document.KbID,
					*document.Thumbnail,
				)
			}
		}

		result[document.ID] = thumbnail
	}

	return result, nil
}

// ListDocumentsByDatasetID list documents by knowledge base ID
func (s *DocumentService) ListDocumentsByDatasetID(ctx context.Context, kbID, keywords string, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	return s.ListDocumentsByDatasetIDWithOptions(ctx, dao.DocumentListOptions{
		KbID:     kbID,
		Keywords: keywords,
		OrderBy:  "create_time",
		Desc:     true,
	}, page, pageSize)
}

// ListDocumentsByDatasetIDWithOptions lists documents by knowledge base ID with filters.
func (s *DocumentService) ListDocumentsByDatasetIDWithOptions(ctx context.Context, opts dao.DocumentListOptions, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	opts.Offset = (page - 1) * pageSize
	opts.Limit = pageSize
	if opts.OrderBy == "" {
		opts.OrderBy = "create_time"
	}
	documents, total, err := s.documentDAO.ListByKBIDWithOptions(ctx, dao.DB, opts)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*entity.DocumentListItem, len(documents))
	for i, doc := range documents {
		responses[i] = doc
	}

	return responses, total, nil
}

// GetDocumentFiltersByDatasetID returns aggregate filter values for documents in a dataset.
func (s *DocumentService) GetDocumentFiltersByDatasetID(ctx context.Context, opts dao.DocumentListOptions) (map[string]interface{}, int64, error) {
	filters, total, err := s.documentDAO.GetFilterByKBID(ctx, dao.DB, opts)
	if err != nil {
		return nil, 0, err
	}
	docIDs, err := s.documentDAO.ListIDsByKBIDWithOptions(ctx, dao.DB, opts)
	if err != nil {
		return nil, 0, err
	}
	metadataFilter, err := s.getDocumentMetadataFilter(ctx, opts.KbID, docIDs)
	if err != nil {
		return nil, 0, err
	}
	filters["metadata"] = metadataFilter
	return filters, total, nil
}

func (s *DocumentService) getDocumentMetadataFilter(ctx context.Context, kbID string, docIDs []string) (map[string]interface{}, error) {
	metadataByKey, err := s.GetMetadataByKBs(ctx, []string{kbID})
	if err != nil {
		return nil, err
	}
	candidateSet := make(map[string]bool, len(docIDs))
	for _, docID := range docIDs {
		candidateSet[docID] = true
	}

	metadataCounter := map[string]interface{}{}
	docIDsWithMetadata := map[string]bool{}
	for key, rawValues := range metadataByKey {
		values, ok := rawValues.(map[string][]string)
		if !ok {
			continue
		}
		valueCounter := map[string]int64{}
		for value, valueDocIDs := range values {
			for _, docID := range valueDocIDs {
				if !candidateSet[docID] {
					continue
				}
				valueCounter[value]++
				docIDsWithMetadata[docID] = true
			}
		}
		if len(valueCounter) > 0 {
			metadataCounter[key] = valueCounter
		}
	}
	metadataCounter["empty_metadata"] = map[string]int64{"true": int64(len(docIDs) - len(docIDsWithMetadata))}
	return metadataCounter, nil
}

// ListDocumentIDsByDatasetIDWithOptions lists matching document IDs without pagination.
func (s *DocumentService) ListDocumentIDsByDatasetIDWithOptions(ctx context.Context, opts dao.DocumentListOptions) ([]string, error) {
	return s.documentDAO.ListIDsByKBIDWithOptions(ctx, dao.DB, opts)
}

// GetDocumentsByAuthorID get documents by author ID
func (s *DocumentService) GetDocumentsByAuthorID(ctx context.Context, authorID, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.GetByAuthorID(ctx, dao.DB, fmt.Sprintf("%d", authorID), offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	docIDs := make([]string, 0, len(documents))
	for _, doc := range documents {
		if doc != nil && doc.ID != "" {
			docIDs = append(docIDs, doc.ID)
		}
	}
	var taskMap map[string]*entity.IngestionTask
	if s.ingestionTaskDAO != nil && len(docIDs) > 0 {
		var err error
		taskMap, err = s.ingestionTaskDAO.GetLatestByDocumentIDs(ctx, dao.DB, docIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get ingestion tasks for documents: %w", err)
		}
	}
	latestEventsByDocument, err := s.latestIngestionEventsByDocument(ctx, taskMap)
	if err != nil {
		common.Warn(fmt.Sprintf("failed to get latest ingestion events for documents: %v", err))
		latestEventsByDocument = make(map[string]*service.IngestionEventItem)
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		var task *entity.IngestionTask
		var latestEvent *service.IngestionEventItem
		if taskMap != nil && doc != nil {
			task = taskMap[doc.ID]
			latestEvent = latestEventsByDocument[doc.ID]
		}
		responses[i] = s.toResponseWithTask(doc, task, latestEvent)
	}

	return responses, total, nil
}

// toResponse convert model.Document to DocumentResponse
func (s *DocumentService) toResponse(ctx context.Context, doc *entity.Document) (*DocumentResponse, error) {
	if s.ingestionTaskDAO == nil || doc == nil || doc.ID == "" {
		return s.toResponseWithTask(doc, nil, nil), nil
	}
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return nil, fmt.Errorf("get ingestion task for document %s: %w", doc.ID, err)
	}
	latestEventsByDocument, err := s.latestIngestionEventsByDocument(ctx, map[string]*entity.IngestionTask{doc.ID: task})
	if err != nil {
		common.Warn(fmt.Sprintf("get latest ingestion event for document %s: %v", doc.ID, err))
		latestEventsByDocument = make(map[string]*service.IngestionEventItem)
	}
	return s.toResponseWithTask(doc, task, latestEventsByDocument[doc.ID]), nil
}

func (s *DocumentService) latestIngestionEventsByDocument(ctx context.Context, tasksByDocument map[string]*entity.IngestionTask) (map[string]*service.IngestionEventItem, error) {
	result := make(map[string]*service.IngestionEventItem)
	if len(tasksByDocument) == 0 {
		return result, nil
	}
	runIDs := make([]string, 0, len(tasksByDocument))
	for _, task := range tasksByDocument {
		if task != nil && task.PipelineLogID != nil && *task.PipelineLogID != "" {
			runIDs = append(runIDs, *task.PipelineLogID)
		}
	}
	if len(runIDs) == 0 {
		return result, nil
	}
	latestByRun, err := s.LatestIngestionEventsByPipelineLogIDs(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	for documentID, task := range tasksByDocument {
		if task == nil || task.PipelineLogID == nil {
			continue
		}
		event := latestByRun[*task.PipelineLogID]
		if event == nil {
			continue
		}
		result[documentID] = event
	}
	return result, nil
}

// LatestIngestionEventsByPipelineLogIDs projects one real latest event for
// every supplied run in a single batch query. Document list mappers receive
// this data from their caller and never perform per-row event lookups.
func (s *DocumentService) LatestIngestionEventsByPipelineLogIDs(ctx context.Context, pipelineLogIDs []string) (map[string]*service.IngestionEventItem, error) {
	eventDAO := s.ingestionTaskLogDAO
	if eventDAO == nil {
		eventDAO = dao.NewIngestionTaskLogDAO()
	}
	latestByRun, err := eventDAO.LatestEventsByPipelineLogIDs(ctx, dao.DB, pipelineLogIDs)
	if err != nil {
		return nil, err
	}
	items := make(map[string]*service.IngestionEventItem, len(latestByRun))
	for runID, event := range latestByRun {
		item := service.IngestionEventItemFromLog(event)
		items[runID] = &item
	}
	return items, nil
}

func (s *DocumentService) toResponseWithTask(doc *entity.Document, task *entity.IngestionTask, latestEvent *service.IngestionEventItem) *DocumentResponse {
	if doc == nil {
		return nil
	}
	createdAt := ""
	if doc.CreateTime != nil {
		// Check if timestamp is in milliseconds (13 digits) or seconds (10 digits)
		var ts int64
		if *doc.CreateTime > 1000000000000 {
			// Milliseconds - convert to seconds
			ts = *doc.CreateTime / 1000
		} else {
			ts = *doc.CreateTime
		}
		createdAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	updatedAt := ""
	if doc.UpdateTime != nil {
		// Accept both historical second-based values and current millisecond-based values.
		ts := *doc.UpdateTime
		if ts > 1000000000000 {
			ts /= 1000
		}
		updatedAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	ingestionStatus := "UNSTART"
	if task != nil && task.Status != "" {
		ingestionStatus = task.Status
	}
	return &DocumentResponse{
		ID:                   doc.ID,
		Name:                 doc.Name,
		KbID:                 doc.KbID,
		ParserID:             doc.ParserID,
		PipelineID:           doc.PipelineID,
		Type:                 doc.Type,
		SourceType:           doc.SourceType,
		CreatedBy:            doc.CreatedBy,
		Location:             doc.Location,
		Size:                 doc.Size,
		TokenNum:             doc.TokenNum,
		ChunkNum:             doc.ChunkNum,
		Progress:             doc.Progress,
		ProgressMsg:          doc.ProgressMsg,
		LatestIngestionEvent: latestEvent,
		ProcessBeginAt:       doc.ProcessBeginAt,
		ProcessDuration:      doc.ProcessDuration,
		Suffix:               doc.Suffix,
		IngestionStatus:      ingestionStatus,
		Status:               doc.Status,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}
}
