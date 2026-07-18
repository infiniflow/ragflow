package dataset

import (
	"errors"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func (d *DatasetService) GetIngestionSummary(datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	documents, _, err := d.documentDAO.GetByKBID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	total := len(documents)
	running := 0
	cancel := 0
	done := 0
	fail := 0
	unstart := 0
	for _, doc := range documents {
		run := ""
		if doc.Run != nil {
			run = *doc.Run
		}
		switch run {
		case string(entity.TaskStatusRunning):
			running++
		case string(entity.TaskStatusCancel):
			cancel++
		case string(entity.TaskStatusDone):
			done++
		case string(entity.TaskStatusFail):
			fail++
		default:
			unstart++
		}
	}

	return map[string]interface{}{
		"total":   total,
		"unset":   unstart,
		"running": running,
		"cancel":  cancel,
		"done":    done,
		"fail":    fail,
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

	logs, total, err := d.pipelineLogDAO.GetDatasetLogsByKBID(datasetID, page, pageSize, orderby, desc, operationStatus, createDateFrom, createDateTo, keywords)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("list ingestion logs: %w", err)
	}

	data := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		data = append(data, datasetIngestionLogToMap(log))
	}

	return map[string]interface{}{
		"data":  data,
		"total": total,
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
	return datasetIngestionLogToMap(log)
}
