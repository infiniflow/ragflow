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
	"errors"
	"fmt"
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
		Where("kb_id = ? AND document_id = ? AND run_count > 0", kbID, graphRaptorFakeDocID)

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
	if err := dao.resolveDSLReferences(ctx, db, logs); err != nil {
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
		Where("kb_id = ? AND run_count > 0", kbID)

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
	if err := dao.resolveDSLReferences(ctx, db, logs); err != nil {
		return nil, 0, err
	}
	return logs, count, nil
}

type pipelineDSLVersionKey struct {
	DSLID   string
	Version int64
}

func (dao *PipelineOperationLogDAO) resolveDSLReferences(ctx context.Context, db *gorm.DB, logs []*entity.PipelineOperationLog) error {
	keys := make(map[pipelineDSLVersionKey]struct{})
	for _, log := range logs {
		if log == nil {
			continue
		}
		log.DSLResolutionError = ""
		if (log.DSLID == nil) != (log.DSLVersion == nil) {
			log.DSL = nil
			log.DSLResolutionError = fmt.Sprintf("pipeline operation log %q has an incomplete DSL reference", log.ID)
			continue
		}
		if log.DSLID != nil {
			keys[pipelineDSLVersionKey{DSLID: *log.DSLID, Version: *log.DSLVersion}] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return nil
	}

	pairs := make([][]any, 0, len(keys))
	for key := range keys {
		pairs = append(pairs, []any{key.DSLID, key.Version})
	}
	var versions []*entity.PipelineDSLVersion
	if err := db.WithContext(ctx).
		Where("(dsl_id, version) IN ?", pairs).
		Find(&versions).Error; err != nil {
		return fmt.Errorf("load pipeline DSL versions: %w", err)
	}

	versionsByKey := make(map[pipelineDSLVersionKey]entity.JSONMap, len(versions))
	for _, version := range versions {
		if version == nil {
			continue
		}
		key := pipelineDSLVersionKey{DSLID: version.DSLID, Version: version.Version}
		versionsByKey[key] = version.DSL
	}
	for _, log := range logs {
		if log == nil || log.DSLID == nil || log.DSLVersion == nil {
			continue
		}
		key := pipelineDSLVersionKey{DSLID: *log.DSLID, Version: *log.DSLVersion}
		dsl, ok := versionsByKey[key]
		if !ok {
			log.DSL = nil
			log.DSLResolutionError = fmt.Sprintf("pipeline operation log %q references missing pipeline DSL version %q@%d", log.ID, key.DSLID, key.Version)
			continue
		}
		log.DSL = dsl
	}
	return nil
}

func (dao *PipelineOperationLogDAO) resolveDSLReference(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
	if err := dao.resolveDSLReferences(ctx, db, []*entity.PipelineOperationLog{log}); err != nil {
		return err
	}
	if log != nil && log.DSLResolutionError != "" {
		return errors.New(log.DSLResolutionError)
	}
	return nil
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

// GetByIDAndKBID fetches persisted ingestion-log metadata scoped to its
// knowledge base. It does not resolve a versioned DSL reference.
func (dao *PipelineOperationLogDAO) GetByIDAndKBID(ctx context.Context, db *gorm.DB, logID, kbID string) (*entity.PipelineOperationLog, error) {
	var log entity.PipelineOperationLog
	if err := db.WithContext(ctx).Where("id = ? AND kb_id = ?", logID, kbID).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

// GetByIDAndKBIDWithDSL fetches a scoped ingestion log and resolves its exact
// recorded DSL. It rejects incomplete or missing version references.
func (dao *PipelineOperationLogDAO) GetByIDAndKBIDWithDSL(ctx context.Context, db *gorm.DB, logID, kbID string) (*entity.PipelineOperationLog, error) {
	log, err := dao.GetByIDAndKBID(ctx, db, logID, kbID)
	if err != nil {
		return nil, err
	}
	if err := dao.resolveDSLReference(ctx, db, log); err != nil {
		return nil, err
	}
	return log, nil
}

// GetByID fetches persisted pipeline-operation-log metadata by ID, regardless
// of knowledge base. It does not resolve a versioned DSL reference. Callers
// that must scope to a dataset use GetByIDAndKBID instead.
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
