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
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
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

// PipelineOperationLogDAO data access object for pipeline_operation_log.
type PipelineOperationLogDAO struct{}

// NewPipelineOperationLogDAO create pipeline operation log DAO.
func NewPipelineOperationLogDAO() *PipelineOperationLogDAO {
	return &PipelineOperationLogDAO{}
}

// GetDatasetLogsByKBID lists dataset-level (graph/raptor/mindmap) ingestion
// logs for a knowledge base. Pagination is only applied when both page and
// pageSize are positive, matching peewee's paginate behavior.
//
// documentID is honoured for the same reason as in GetFileLogsByKBID. Dataset
// logs belong to no single document, so a caller that names one gets an empty
// list rather than the whole dataset history.
func (dao *PipelineOperationLogDAO) GetDatasetLogsByKBID(ctx context.Context, db *gorm.DB, kbID string, page, pageSize int, terms []OrderTerm, operationStatus []string, createDateFrom, createDateTo, keywords, documentID string) ([]*entity.PipelineOperationLog, int64, error) {
	query := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("kb_id = ? AND document_id = ?", kbID, graphRaptorFakeDocID)

	if keywords != "" {
		query = query.Where("LOWER(document_name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}
	if documentID != "" {
		query = query.Where("document_id = ?", documentID)
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
	query = query.Order(pipelineLogOrderClause(terms))
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
//
// documentID narrows the list to one document exactly. The frontend needs it to
// resolve a queued document's early row: matching by document_name is a fuzzy
// LIKE search that can push the row out of the first page when several
// documents share a name.
func (dao *PipelineOperationLogDAO) GetFileLogsByKBID(ctx context.Context, db *gorm.DB, kbID string, page, pageSize int, terms []OrderTerm, keywords, documentID string, operationStatus []string, createDateFrom, createDateTo string) ([]*entity.PipelineOperationLog, int64, error) {
	query := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("kb_id = ?", kbID)

	if keywords != "" {
		query = query.Where("LOWER(document_name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}
	if documentID != "" {
		query = query.Where("document_id = ?", documentID)
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
	query = query.Order(pipelineLogOrderClause(terms))
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var logs []*entity.PipelineOperationLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, count, nil
}

// OpenPipelineOperationStatuses returns the operation_status values of a
// pipeline operation log row that a run is still moving through. A run owns
// exactly one such row from CREATED to its terminal write (DONE/FAIL/CANCEL),
// so later stages advance the same row instead of inserting a new one. It is a
// function, not a shared slice, so no caller can mutate the set.
func OpenPipelineOperationStatuses() []string {
	return []string{
		string(entity.TaskStatusUnstart),
		string(entity.TaskStatusSchedule),
		string(entity.TaskStatusRunning),
	}
}

// GetOpenLogByDocumentID returns the newest open pipeline operation log for a
// document, or nil when the document has no in-flight run.
//
// It locates a row; it deliberately does not decide what may happen to it. The
// create path drops whatever it finds (dropping a leftover from a run that no
// longer exists), and the terminal writer adopts it only when the caller has no
// bound row of its own. Whether a *terminal* write may touch a row is bound to
// the run's own row id (ingestion_task.pipeline_log_id), so a superseded run
// cannot reach the replacement run's row.
func (dao *PipelineOperationLogDAO) GetOpenLogByDocumentID(ctx context.Context, db *gorm.DB, documentID string) (*entity.PipelineOperationLog, error) {
	var log entity.PipelineOperationLog
	err := db.WithContext(ctx).
		Where("document_id = ? AND operation_status IN ?", documentID, OpenPipelineOperationStatuses()).
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

// OpenLogInput carries the bookkeeping needed to open or advance the
// pre-terminal row for a queued run. It mirrors the PipelineOperationLog
// columns that are known before the pipeline finishes; the DSL and the final
// progress/counters stay empty until the terminal writer fills them in.
type OpenLogInput struct {
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
	RunCount        int
}

// CreateOpenLog opens the pre-terminal row for a queued, numbered run. Its
// caller holds the owning document row lock and binds the result to the task
// before the task may be published.
func (dao *PipelineOperationLogDAO) CreateOpenLog(ctx context.Context, db *gorm.DB, input OpenLogInput) (*entity.PipelineOperationLog, error) {
	if input.RunCount <= 0 {
		return nil, fmt.Errorf("pipeline operation log run count must be positive")
	}
	now := time.Now().Local()
	runCount := input.RunCount
	log := &entity.PipelineOperationLog{
		ID:              utility.GenerateUUID(),
		DocumentID:      input.DocumentID,
		RunCount:        &runCount,
		TenantID:        input.TenantID,
		KbID:            input.KbID,
		ParserID:        input.ParserID,
		DocumentName:    input.DocumentName,
		DocumentSuffix:  input.DocumentSuffix,
		DocumentType:    input.DocumentType,
		SourceFrom:      input.SourceFrom,
		Progress:        0,
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

// NextRunCount returns the next display number for a document. The caller
// must already hold that document row FOR UPDATE, which makes MAX+1 safe
// across concurrent enqueue requests without a separate counter table.
func (dao *PipelineOperationLogDAO) NextRunCount(ctx context.Context, db *gorm.DB, documentID string) (int, error) {
	var next int
	err := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("document_id = ?", documentID).
		Select("COALESCE(MAX(run_count), 0) + 1").
		Scan(&next).Error
	if err != nil {
		return 0, err
	}
	return next, nil
}

// AdvanceOpenLog moves a run's own row to a later status. The row is targeted
// by id, so it can never touch another run's row, and the update is guarded by
// the from-states the caller declares:
// the transitions are monotonic (unstart -> schedule -> running), so a late
// queued write cannot regress a row a concurrent writer already advanced.
// A row that already left the declared from-states is left untouched. The
// legacy progress_msg column is intentionally not updated; run text belongs
// to ingestion_task_log events.
func (dao *PipelineOperationLogDAO) AdvanceOpenLog(ctx context.Context, db *gorm.DB, logID string, fromStatuses []string, operationStatus string) error {
	if logID == "" || len(fromStatuses) == 0 {
		return nil
	}
	return db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("id = ? AND operation_status IN ?", logID, fromStatuses).
		Update("operation_status", operationStatus).Error
}

// DeleteOpenLogByID removes only a legacy, unnumbered pre-terminal row. A
// numbered run is its document's immutable numbering ledger and must remain
// available even when publication or task cleanup later fails.
func (dao *PipelineOperationLogDAO) DeleteOpenLogByID(ctx context.Context, db *gorm.DB, logID string) error {
	if logID == "" {
		return nil
	}
	return db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("id = ? AND run_count IS NULL AND operation_status IN ?", logID, OpenPipelineOperationStatuses()).
		Delete(&entity.PipelineOperationLog{}).Error
}

// DeleteUnownedOpenLogByID removes an open row only when no live ingestion
// task owns it. Existing databases may contain more than one task per document,
// so document identity alone is not sufficient proof that a row is leftover.
func (dao *PipelineOperationLogDAO) DeleteUnownedOpenLogByID(ctx context.Context, db *gorm.DB, logID string) (bool, error) {
	if logID == "" {
		return false, nil
	}
	result := db.WithContext(ctx).Model(&entity.PipelineOperationLog{}).
		Where("id = ? AND run_count IS NULL AND operation_status IN ?", logID, OpenPipelineOperationStatuses()).
		Where("NOT EXISTS (SELECT 1 FROM ingestion_task WHERE pipeline_log_id = ? AND status IN ?)", logID, common.ActiveTaskStatuses).
		Delete(&entity.PipelineOperationLog{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// HasLiveTaskOwner reports whether a live ingestion task is bound to the given
// pipeline operation log row. Adoption of an open row is gated on this: the
// create path never takes a row a live run owns, and the terminal fallback must
// apply the same rule, or it would finalize the live run's entry with another
// run's status and swallow the live run's own terminal write.
func (dao *PipelineOperationLogDAO) HasLiveTaskOwner(ctx context.Context, db *gorm.DB, logID string) (bool, error) {
	if logID == "" {
		return false, nil
	}
	var count int64
	err := db.WithContext(ctx).Model(&entity.IngestionTask{}).
		Where("pipeline_log_id = ? AND status IN ?", logID, common.ActiveTaskStatuses).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
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

// Create inserts a new pipeline operation log.
func (dao *PipelineOperationLogDAO) Create(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
	return db.WithContext(ctx).Create(log).Error
}
