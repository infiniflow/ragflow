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
	"strings"

	"ragflow/internal/entity"
)

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
// pageSize are positive, matching peewee's paginate behaviour.
func (dao *PipelineOperationLogDAO) GetDatasetLogsByKBID(kbID string, page, pageSize int, orderby string, desc bool, operationStatus []string, createDateFrom, createDateTo, keywords string) ([]*entity.PipelineOperationLog, int64, error) {
	query := DB.Model(&entity.PipelineOperationLog{}).
		Where("kb_id = ? AND document_id = ?", kbID, graphRaptorFakeDocID)

	if keywords != "" {
		query = query.Where("LOWER(document_name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}
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

// GetFileLogsByKBID lists per-file ingestion logs for a knowledge base.
func (dao *PipelineOperationLogDAO) GetFileLogsByKBID(kbID string, page, pageSize int, orderby string, desc bool, keywords string, operationStatus []string, createDateFrom, createDateTo string) ([]*entity.PipelineOperationLog, int64, error) {
	query := DB.Model(&entity.PipelineOperationLog{}).
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

// GetByIDAndKBID fetches a single ingestion log scoped to its knowledge base.
func (dao *PipelineOperationLogDAO) GetByIDAndKBID(logID, kbID string) (*entity.PipelineOperationLog, error) {
	var log entity.PipelineOperationLog
	if err := DB.Where("id = ? AND kb_id = ?", logID, kbID).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}
