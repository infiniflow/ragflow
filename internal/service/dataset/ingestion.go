package dataset

import (
	"errors"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func (d *DatasetService) GetIngestionSummary(datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("Invalid Dataset ID '%s'", datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	status, err := d.documentDAO.GetParsingStatusByKBID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	return map[string]interface{}{
		"doc_num":   kb.DocNum,
		"chunk_num": kb.ChunkNum,
		"token_num": kb.TokenNum,
		"status":    status,
	}, common.CodeSuccess, nil
}

func (d *DatasetService) ListIngestionLogs(datasetID, userID string, page, pageSize int, orderby string, desc bool, operationStatus []string, createDateFrom, createDateTo, logType, keywords string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}
	if orderby == "" {
		orderby = "create_time"
	}

	var (
		logs  []*entity.PipelineOperationLog
		total int64
		err   error
	)
	if logType == "file" {
		logs, total, err = d.pipelineLogDAO.GetFileLogsByKBID(datasetID, page, pageSize, orderby, desc, keywords, operationStatus, createDateFrom, createDateTo)
	} else {
		logs, total, err = d.pipelineLogDAO.GetDatasetLogsByKBID(datasetID, page, pageSize, orderby, desc, operationStatus, createDateFrom, createDateTo, keywords)
	}
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("list ingestion logs: %w", err)
	}

	items := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		if logType == "file" {
			items = append(items, fileIngestionLogToMap(log))
		} else {
			items = append(items, datasetIngestionLogToMap(log))
		}
	}

	return map[string]interface{}{
		"total": total,
		"logs":  items,
	}, common.CodeSuccess, nil
}

func (d *DatasetService) GetIngestionLog(datasetID, userID, logID string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if logID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Log ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	log, err := d.pipelineLogDAO.GetByIDAndKBID(logID, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("get ingestion log: %w", err)
	}
	if log == nil {
		return nil, common.CodeDataError, errors.New("Log not found")
	}

	return datasetIngestionLogToMap(log), common.CodeSuccess, nil
}

func datasetIngestionLogToMap(log *entity.PipelineOperationLog) map[string]interface{} {
	m := map[string]interface{}{
		"id":               log.ID,
		"dataset_id":       log.KbID,
		"tenant_id":        log.TenantID,
		"document_id":      log.DocumentID,
		"document_name":    log.DocumentName,
		"document_suffix":  log.DocumentSuffix,
		"source_from":      log.SourceFrom,
		"task_type":        log.TaskType,
		"operation_status": log.OperationStatus,
		"progress":         log.Progress,
		"create_time":      log.CreateTime,
		"update_time":      log.UpdateTime,
	}
	if log.PipelineID != nil {
		m["pipeline_id"] = *log.PipelineID
	}
	if log.ProgressMsg != nil {
		m["progress_msg"] = *log.ProgressMsg
	}
	if log.Status != nil {
		m["status"] = *log.Status
	}
	return m
}

func fileIngestionLogToMap(log *entity.PipelineOperationLog) map[string]interface{} {
	return map[string]interface{}{
		"id":               log.ID,
		"document_id":      log.DocumentID,
		"tenant_id":        log.TenantID,
		"kb_id":            log.KbID,
		"pipeline_id":      stringPointerValue(log.PipelineID),
		"pipeline_title":   stringPointerValue(log.PipelineTitle),
		"parser_id":        log.ParserID,
		"document_name":    log.DocumentName,
		"document_suffix":  log.DocumentSuffix,
		"document_type":    log.DocumentType,
		"source_from":      log.SourceFrom,
		"progress":         log.Progress,
		"progress_msg":     stringPointerValue(log.ProgressMsg),
		"process_begin_at": timePointerValue(log.ProcessBeginAt),
		"process_duration": log.ProcessDuration,
		"dsl":              jsonMapValue(log.DSL),
		"task_type":        log.TaskType,
		"operation_status": log.OperationStatus,
		"avatar":           stringPointerValue(log.Avatar),
		"status":           stringPointerValue(log.Status),
		"create_time":      int64PointerValue(log.CreateTime),
		"create_date":      timePointerValue(log.CreateDate),
		"update_time":      int64PointerValue(log.UpdateTime),
		"update_date":      timePointerValue(log.UpdateDate),
	}
}
