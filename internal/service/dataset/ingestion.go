package dataset

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

const (
	defaultIngestionMessagesLimit = 200
	maxIngestionMessagesLimit     = 500
)

// IngestionMessagesResponse is one immutable run's keyset-paginated event
// stream. IDs are database event IDs and must be returned unchanged by clients
// when requesting adjacent pages.
type IngestionMessagesResponse struct {
	RunCount      int                          `json:"run_count"`
	Items         []service.IngestionEventItem `json:"items"`
	OldestID      int                          `json:"oldest_id"`
	NewestID      int                          `json:"newest_id"`
	HasMoreBefore bool                         `json:"has_more_before"`
	HasMoreAfter  bool                         `json:"has_more_after"`
	Terminal      bool                         `json:"terminal"`
}

func (d *DatasetService) GetIngestionSummary(ctx context.Context, datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
	}

	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("invalid Dataset ID '%s'", datasetID)
		}
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	status, err := d.documentDAO.GetParsingStatusByKBID(ctx, dao.DB, datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("database operation failed")
	}

	return map[string]interface{}{
		"doc_num":   kb.DocNum,
		"chunk_num": kb.ChunkNum,
		"token_num": kb.TokenNum,
		"status":    status,
	}, common.CodeSuccess, nil
}

// ListIngestionMessages returns events owned by the requested immutable
// pipeline log.
func (d *DatasetService) ListIngestionMessages(ctx context.Context, datasetID, userID, logID string, limit int, afterID, beforeID *int) (*IngestionMessagesResponse, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeArgumentError, errors.New(`lack of "Dataset ID"`)
	}
	if logID == "" {
		return nil, common.CodeArgumentError, errors.New(`lack of "Log ID"`)
	}
	if afterID != nil && beforeID != nil {
		return nil, common.CodeArgumentError, errors.New("after_id and before_id are mutually exclusive")
	}
	if afterID != nil && *afterID <= 0 {
		return nil, common.CodeArgumentError, errors.New("after_id must be a positive integer")
	}
	if beforeID != nil && *beforeID <= 0 {
		return nil, common.CodeArgumentError, errors.New("before_id must be a positive integer")
	}
	if limit == 0 {
		limit = defaultIngestionMessagesLimit
	}
	if limit < 0 || limit > maxIngestionMessagesLimit {
		return nil, common.CodeArgumentError, fmt.Errorf("limit must be between 1 and %d", maxIngestionMessagesLimit)
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
	}

	run, err := d.pipelineLogDAO.GetByIDAndKBID(ctx, dao.DB, logID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("log not found")
		}
		return nil, common.CodeServerError, fmt.Errorf("get ingestion log: %w", err)
	}
	if !isReadableIngestionLog(run) {
		return nil, common.CodeDataError, errors.New("log not found")
	}
	response := &IngestionMessagesResponse{}
	if run.RunCount != nil {
		response.RunCount = *run.RunCount
	}
	response.Terminal = isTerminalIngestionLogStatus(run.OperationStatus)

	page, err := dao.NewIngestionTaskLogDAO().ListEventsPageByPipelineLogID(ctx, dao.DB, logID, limit, afterID, beforeID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("list ingestion messages: %w", err)
	}
	response.Items = make([]service.IngestionEventItem, 0, len(page.Events))
	for _, event := range page.Events {
		response.Items = append(response.Items, service.IngestionEventItemFromLog(event))
	}
	if len(response.Items) > 0 {
		response.OldestID = response.Items[0].ID
		response.NewestID = response.Items[len(response.Items)-1].ID
	}
	response.HasMoreBefore = page.HasMoreBefore
	response.HasMoreAfter = page.HasMoreAfter
	return response, common.CodeSuccess, nil
}

func isTerminalIngestionLogStatus(status string) bool {
	switch status {
	case string(entity.TaskStatusDone), string(entity.TaskStatusFail), string(entity.TaskStatusCancel), "DONE", "FAIL", "CANCEL":
		return true
	default:
		return false
	}
}

func isReadableIngestionLog(log *entity.PipelineOperationLog) bool {
	if log == nil {
		return false
	}
	if log.DocumentID == entity.DatasetLogDocumentID {
		return log.RunCount == nil
	}
	return log.RunCount != nil && *log.RunCount > 0
}

func (d *DatasetService) ListIngestionLogs(ctx context.Context, datasetID, userID string, page, pageSize int, terms []dao.OrderTerm, operationStatus []string, createDateFrom, createDateTo, logType, keywords, documentID string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
	}
	if logType != "dataset" && logType != "file" {
		return nil, common.CodeDataError, errors.New(`Invalid "log_type", expected "dataset" or "file"`)
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}

	var (
		logs  []*entity.PipelineOperationLog
		total int64
		err   error
	)
	if logType == "file" {
		logs, total, err = d.pipelineLogDAO.GetFileLogsByKBID(ctx, dao.DB, datasetID, page, pageSize, terms, keywords, documentID, operationStatus, createDateFrom, createDateTo)
	} else {
		logs, total, err = d.pipelineLogDAO.GetDatasetLogsByKBID(ctx, dao.DB, datasetID, page, pageSize, terms, operationStatus, createDateFrom, createDateTo, keywords, documentID)
	}
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("list ingestion logs: %w", err)
	}
	logIDs := make([]string, 0, len(logs))
	for _, log := range logs {
		if log != nil && log.ID != "" {
			logIDs = append(logIDs, log.ID)
		}
	}
	latestEvents, err := dao.NewIngestionTaskLogDAO().LatestEventsByPipelineLogIDs(ctx, dao.DB, logIDs)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("list latest ingestion events: %w", err)
	}

	items := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		latestEvent := ingestionEventItem(latestEvents[log.ID])
		if logType == "file" {
			items = append(items, fileIngestionLogToMap(log, latestEvent))
		} else {
			items = append(items, datasetIngestionLogToMap(log, latestEvent))
		}
	}

	return map[string]interface{}{
		"total": total,
		"logs":  items,
	}, common.CodeSuccess, nil
}

func (d *DatasetService) GetIngestionLog(ctx context.Context, datasetID, userID, logID string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`lack of "Dataset ID"`)
	}
	if logID == "" {
		return nil, common.CodeDataError, errors.New(`lack of "Log ID"`)
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
	}

	log, err := d.pipelineLogDAO.GetByIDAndKBID(ctx, dao.DB, logID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("log not found")
		}
		return nil, common.CodeServerError, fmt.Errorf("get ingestion log: %w", err)
	}
	if !isReadableIngestionLog(log) {
		return nil, common.CodeDataError, errors.New("log not found")
	}

	latestEvents, err := dao.NewIngestionTaskLogDAO().LatestEventsByPipelineLogIDs(ctx, dao.DB, []string{log.ID})
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("get latest ingestion event: %w", err)
	}
	return datasetIngestionLogToMap(log, ingestionEventItem(latestEvents[log.ID])), common.CodeSuccess, nil
}

func datasetIngestionLogToMap(log *entity.PipelineOperationLog, latestEvent *service.IngestionEventItem) map[string]interface{} {
	m := map[string]interface{}{
		"id":                     log.ID,
		"dataset_id":             log.KbID,
		"tenant_id":              log.TenantID,
		"document_id":            log.DocumentID,
		"document_name":          log.DocumentName,
		"document_suffix":        log.DocumentSuffix,
		"source_from":            log.SourceFrom,
		"task_type":              log.TaskType,
		"operation_status":       log.OperationStatus,
		"progress":               log.Progress,
		"process_begin_at":       log.ProcessBeginAt,
		"process_duration":       log.ProcessDuration,
		"dsl":                    log.DSL,
		"avatar":                 log.Avatar,
		"create_time":            log.CreateTime,
		"create_date":            log.CreateDate,
		"update_time":            log.UpdateTime,
		"update_date":            log.UpdateDate,
		"latest_ingestion_event": latestEvent,
	}
	if log.PipelineID != nil {
		m["pipeline_id"] = *log.PipelineID
	}
	if log.Status != nil {
		m["status"] = *log.Status
	}
	return m
}

func fileIngestionLogToMap(log *entity.PipelineOperationLog, latestEvent *service.IngestionEventItem) map[string]interface{} {
	return map[string]interface{}{
		"id":                     log.ID,
		"document_id":            log.DocumentID,
		"tenant_id":              log.TenantID,
		"kb_id":                  log.KbID,
		"pipeline_id":            stringPointerValue(log.PipelineID),
		"pipeline_title":         stringPointerValue(log.PipelineTitle),
		"parser_id":              log.ParserID,
		"document_name":          log.DocumentName,
		"document_suffix":        log.DocumentSuffix,
		"document_type":          log.DocumentType,
		"source_from":            log.SourceFrom,
		"progress":               log.Progress,
		"process_begin_at":       timePointerValue(log.ProcessBeginAt),
		"process_duration":       log.ProcessDuration,
		"dsl":                    jsonMapValue(log.DSL),
		"task_type":              log.TaskType,
		"operation_status":       log.OperationStatus,
		"avatar":                 stringPointerValue(log.Avatar),
		"status":                 stringPointerValue(log.Status),
		"create_time":            int64PointerValue(log.CreateTime),
		"create_date":            timePointerValue(log.CreateDate),
		"update_time":            int64PointerValue(log.UpdateTime),
		"update_date":            timePointerValue(log.UpdateDate),
		"latest_ingestion_event": latestEvent,
	}
}

func ingestionEventItem(event *entity.IngestionTaskLog) *service.IngestionEventItem {
	if event == nil {
		return nil
	}
	item := service.IngestionEventItemFromLog(event)
	return &item
}
