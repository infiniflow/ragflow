//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package dao

import (
	"context"
	"strings"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

var legacyPipelineOperationStatuses = map[string]string{
	"0": "UNSTART",
	"1": "RUNNING",
	"2": "CANCEL",
	"3": "DONE",
	"4": "FAIL",
	"5": "SCHEDULE",
}

func normalizePipelineOperationStatuses(statuses []string) []string {
	if len(statuses) == 0 {
		return statuses
	}
	normalized := make([]string, len(statuses))
	for i, status := range statuses {
		if canonical, ok := legacyPipelineOperationStatuses[status]; ok {
			normalized[i] = canonical
		} else {
			normalized[i] = status
		}
	}
	return normalized
}

// graphRaptorFakeDocID is the placeholder document_id used for dataset-level
// (graph/raptor/mindmap) pipeline logs, mirroring GRAPH_RAPTOR_FAKE_DOC_ID in
// api/db/services/task_service.py.
const graphRaptorFakeDocID = "graph_raptor_x"

// pipelineLogOrderableColumns whitelists the columns that may appear in an
// ORDER BY clause so an attacker cannot inject arbitrary SQL through the
// `orderby` query parameter.
var pipelineLogOrderableColumns = map[string]struct{}{
	"id":               {},
	"document_id":      {},
	"tenant_id":        {},
	"kb_id":            {},
	"pipeline_id":      {},
	"pipeline_title":   {},
	"parser_id":        {},
	"document_name":    {},
	"document_suffix":  {},
	"document_type":    {},
	"source_from":      {},
	"progress":         {},
	"process_begin_at": {},
	"process_duration": {},
	"task_type":        {},
	"operation_status": {},
	"status":           {},
	"create_time":      {},
	"create_date":      {},
	"update_time":      {},
	"update_date":      {},
}

func pipelineLogOrderClause(orderby string, desc bool) string {
	if _, ok := pipelineLogOrderableColumns[orderby]; !ok {
		orderby = "create_time"
	}
	if desc {
		return orderby + " DESC"
	}
	return orderby + " ASC"
}

// PipelineOperationLogDAO data access object for pipeline_operation_log.
type PipelineOperationLogDAO struct{}

// NewPipelineOperationLogDAO create pipeline operation log DAO.
func NewPipelineOperationLogDAO() *PipelineOperationLogDAO {
	return &PipelineOperationLogDAO{}
}

// GetDatasetLogsByKBID lists dataset-level (graph/raptor/mindmap) ingestion
// logs for a knowledge base. Pagination is only applied when both page and
// pageSize are positive, matching peewee's paginate behavior.
func (dao *PipelineOperationLogDAO) GetDatasetLogsByKBID(ctx context.Context, db *gorm.DB, kbID string, page, pageSize int, orderby string, desc bool, operationStatus []string, createDateFrom, createDateTo, keywords string) ([]*entity.PipelineOperationLog, int64, error) {
	query := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("kb_id = ? AND document_id = ?", kbID, graphRaptorFakeDocID)

	if keywords != "" {
		query = query.Where("LOWER(document_name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}
	if len(operationStatus) > 0 {
		query = query.Where("operation_status IN ?", normalizePipelineOperationStatuses(operationStatus))
	}
	if createDateFrom != "" {
		query = query.Where("create_date >= ?", createDateFrom)
	}
	if createDateTo != "" {
		query = query.Where("create_date <= ?", createDateTo)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	// above validates `orderby` against pipelineLogOrderableColumns
	// (a closed allowlist of column names) and defaults to a safe value
	// if no match is found. The only string that flows into Order() is
	// the whitelisted column name + " ASC"/" DESC" suffix.
	// codeql[go/sql-injection] False positive: pipelineLogOrderClause
	query = query.Order(pipelineLogOrderClause(orderby, desc))
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var logs []*entity.PipelineOperationLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, count, nil
}

// GetFileLogsByKBID lists per-file ingestion logs for a knowledge base.
func (dao *PipelineOperationLogDAO) GetFileLogsByKBID(ctx context.Context, db *gorm.DB, kbID string, page, pageSize int, orderby string, desc bool, keywords string, operationStatus []string, createDateFrom, createDateTo string) ([]*entity.PipelineOperationLog, int64, error) {
	query := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("kb_id = ?", kbID)

	if keywords != "" {
		query = query.Where("LOWER(document_name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}
	query = query.Where("document_id <> ?", graphRaptorFakeDocID)

	if len(operationStatus) > 0 {
		query = query.Where("operation_status IN ?", operationStatus)
	}
	if createDateFrom != "" {
		query = query.Where("create_date >= ?", createDateFrom)
	}
	if createDateTo != "" {
		query = query.Where("create_date <= ?", createDateTo)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	// above validates `orderby` against pipelineLogOrderableColumns
	// (a closed allowlist of column names) and defaults to a safe value
	// if no match is found. The only string that flows into Order() is
	// the whitelisted column name + " ASC"/" DESC" suffix.
	// codeql[go/sql-injection] False positive: pipelineLogOrderClause
	query = query.Order(pipelineLogOrderClause(orderby, desc))
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var logs []*entity.PipelineOperationLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, count, nil
}

// OpenPipelineOperationStatuses are the operation_status values of a
// pipeline operation log row that a run is still moving through. A run owns
// exactly one such row from CREATED to its terminal write (DONE/FAIL/CANCEL),
// so later stages advance the same row instead of inserting a new one.
var OpenPipelineOperationStatuses = []string{"0", "5", "1"}

// GetOpenLogByDocumentID returns the newest open pipeline operation log for
// a document, or nil when the document has no in-flight run. Open rows are
// adopted by document_id alone: the ingestion_task.document_id unique index
// guarantees at most one non-terminal task per document, so a stale open row
// can only exist after its task was deleted — and the task-deleting paths
// (Remove, rerun clear, delete-only ingest) drop it. A retry after a terminal
// state therefore starts a fresh row instead of resurrecting the finished one.
func (dao *PipelineOperationLogDAO) GetOpenLogByDocumentID(ctx context.Context, db *gorm.DB, documentID string) (*entity.PipelineOperationLog, error) {
	var log entity.PipelineOperationLog
	err := db.WithContext(ctx).
		Where("document_id = ? AND operation_status IN ?", documentID, OpenPipelineOperationStatuses).
		Order("create_time DESC").
		Order("id DESC").
		First(&log).Error
	if err != nil {
		if IsNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}
	return &log, nil
}

// EarlyLogInput carries the bookkeeping needed to open or advance the
// pre-terminal row for a queued run. It mirrors the PipelineOperationLog
// columns that are known before the pipeline finishes; the DSL and the final
// progress/counters stay empty until the terminal writer fills them in.
type EarlyLogInput struct {
	DocumentID      string
	KbID            string
	TenantID        string
	PipelineID      string
	PipelineTitle   string
	ParserID        string
	DocumentName    string
	DocumentSuffix  string
	DocumentType    string
	SourceFrom      string
	Avatar          *string
	OperationStatus string
	ProgressMsg     string
}

// CreateEarlyLog opens the pre-terminal row for a queued run. Callers must
// have verified no open row exists (or accept a best-effort duplicate on a
// lost race); the error is returned so tests can assert it, while production
// callers log and continue.
func (dao *PipelineOperationLogDAO) CreateEarlyLog(ctx context.Context, db *gorm.DB, input EarlyLogInput) (*entity.PipelineOperationLog, error) {
	now := time.Now().Local()
	msg := input.ProgressMsg
	log := &entity.PipelineOperationLog{
		ID:              utility.GenerateUUID(),
		DocumentID:      input.DocumentID,
		TenantID:        input.TenantID,
		KbID:            input.KbID,
		ParserID:        input.ParserID,
		DocumentName:    input.DocumentName,
		DocumentSuffix:  input.DocumentSuffix,
		DocumentType:    input.DocumentType,
		SourceFrom:      input.SourceFrom,
		Progress:        0,
		ProgressMsg:     &msg,
		ProcessBeginAt:  &now,
		ProcessDuration: 0,
		DSL:             entity.JSONMap{},
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: input.OperationStatus,
		Avatar:          input.Avatar,
	}
	statusValue := "1"
	log.Status = &statusValue
	if input.PipelineID != "" {
		pipelineID := input.PipelineID
		log.PipelineID = &pipelineID
	}
	if input.PipelineTitle != "" {
		title := input.PipelineTitle
		log.PipelineTitle = &title
	}
	if err := db.WithContext(ctx).Create(log).Error; err != nil {
		return nil, err
	}
	return log, nil
}

// AdvanceEarlyLog moves the open row for a document to a later status,
// refreshing its queued message. It targets the newest open row in a single
// statement: a concurrent advance that already moved the row out of the open
// set is left alone (RowsAffected 0, reported as false) rather than being
// overwritten, so competing writers cannot regress each other's status.
func (dao *PipelineOperationLogDAO) AdvanceEarlyLog(ctx context.Context, db *gorm.DB, documentID, operationStatus, progressMsg string) (bool, error) {
	result := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where(`id = (
			SELECT id FROM (
				SELECT id FROM pipeline_operation_log
				WHERE document_id = ? AND operation_status IN ?
				ORDER BY create_time DESC, id DESC LIMIT 1
			) AS open_row
		)`, documentID, OpenPipelineOperationStatuses).
		Updates(map[string]interface{}{
			"operation_status": operationStatus,
			"progress_msg":     progressMsg,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// DeleteOpenLogsByDocumentID removes the open (non-terminal) rows for a
// document. Used to clean up the early row when task creation rolls back, so
// the detail page is not left with a permanently queued entry.
func (dao *PipelineOperationLogDAO) DeleteOpenLogsByDocumentID(ctx context.Context, db *gorm.DB, documentID string) error {
	return db.WithContext(ctx).
		Where("document_id = ? AND operation_status IN ?", documentID, OpenPipelineOperationStatuses).
		Delete(&entity.PipelineOperationLog{}).Error
}

// GetByIDAndKBID fetches a single ingestion log scoped to its knowledge base.
func (dao *PipelineOperationLogDAO) GetByIDAndKBID(ctx context.Context, db *gorm.DB, logID, kbID string) (*entity.PipelineOperationLog, error) {
	var log entity.PipelineOperationLog
	if err := db.WithContext(ctx).Where("id = ? AND kb_id = ?", logID, kbID).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

// GetByID fetches a single pipeline operation log by id, regardless of
// knowledge base. Callers that must scope to a dataset use
// GetByIDAndKBID instead.
func (dao *PipelineOperationLogDAO) GetByID(ctx context.Context, db *gorm.DB, logID string) (*entity.PipelineOperationLog, error) {
	var log entity.PipelineOperationLog
	if err := db.WithContext(ctx).Where("id = ?", logID).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

// UpdateDSL replaces the DSL stored on a pipeline operation log. Used by the
// dataflow rerun endpoint to persist the front-end's edited component
// configuration plus the rerun entry point (dsl.path = [component_id]),
// mirroring Python's PipelineOperationLogService.update_by_id(id, {"dsl": dsl}).
func (dao *PipelineOperationLogDAO) UpdateDSL(ctx context.Context, db *gorm.DB, logID string, dsl entity.JSONMap) error {
	return db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("id = ?", logID).
		Update("dsl", dsl).Error
}

// Create inserts a new pipeline operation log.
func (dao *PipelineOperationLogDAO) Create(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
	return db.WithContext(ctx).Create(log).Error
}
