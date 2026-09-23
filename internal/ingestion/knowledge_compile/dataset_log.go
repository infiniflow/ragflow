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

package knowledge_compile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"

	"gorm.io/gorm"
)

var allDatasetTaskTypes = []string{
	kccommon.TaskTypeWiki,
	kccommon.TaskTypeTree,
	kccommon.TaskTypeGraph,
	kccommon.TaskTypeMindmap,
	kccommon.TaskTypeTimeline,
	kccommon.TaskTypePageIndex,
}

var structureTaskTypes = []string{
	kccommon.TaskTypeGraph,
	kccommon.TaskTypeTimeline,
	kccommon.TaskTypePageIndex,
}

// Dataset ingestion logs use the status values exposed by the Python API and
// consumed by the shared frontend. The Go scheduler uses a different set of
// internal terminal names, so translate them at the persistence boundary.
func datasetLogOperationStatus(status string) string {
	switch status {
	case common.COMPLETED:
		return "DONE"
	case common.FAILED:
		return "FAIL"
	case common.STOPPED, common.STOPPING:
		return "CANCEL"
	default:
		return status
	}
}

func datasetCompileLogID(claimToken, taskType string) string {
	sum := sha256.Sum256([]byte("knowledge-compile-dataset-log\x00" + claimToken + "\x00" + taskType))
	return hex.EncodeToString(sum[:16])
}

func taskTypesForVariants(variants []string) []string {
	seen := make(map[string]struct{}, len(variants))
	for _, variant := range variants {
		var taskType string
		switch strings.ToLower(strings.TrimSpace(variant)) {
		case "wiki":
			taskType = kccommon.TaskTypeWiki
		case "tree":
			taskType = kccommon.TaskTypeTree
		case "mindmap", "mind_map":
			taskType = kccommon.TaskTypeMindmap
		case "structure":
			// Legacy structure events predate the task-type field and cannot
			// distinguish graph, timeline, and page-index products.
			for _, structureTaskType := range structureTaskTypes {
				seen[structureTaskType] = struct{}{}
			}
		}
		if taskType != "" {
			seen[taskType] = struct{}{}
		}
	}
	return sortedTaskTypes(seen)
}

func taskTypesForEntry(entry BacklogEntry) []string {
	seen := make(map[string]struct{}, len(entry.TaskTypes))
	for _, taskType := range entry.TaskTypes {
		if taskType = normalizeTaskType(taskType); taskType != "" {
			seen[taskType] = struct{}{}
		}
	}
	if len(seen) > 0 {
		return sortedTaskTypes(seen)
	}
	if taskTypes := taskTypesForVariants(entry.Variants); len(taskTypes) > 0 {
		return taskTypes
	}
	// An event with no routing metadata must remain visible in every possible
	// dataset log. Returning the fallback per entry prevents an unknown entry
	// from disappearing when other entries in the same batch are typed.
	return append([]string(nil), allDatasetTaskTypes...)
}

func normalizeTaskType(taskType string) string {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case "wiki":
		return kccommon.TaskTypeWiki
	case "tree":
		return kccommon.TaskTypeTree
	case "graph", "knowledge_graph", "knowledgegraph":
		return kccommon.TaskTypeGraph
	case "mindmap", "mind_map":
		return kccommon.TaskTypeMindmap
	case "timeline":
		return kccommon.TaskTypeTimeline
	case "pageindex", "page_index":
		return kccommon.TaskTypePageIndex
	default:
		return ""
	}
}

func taskTypesForEntries(entries []BacklogEntry) []string {
	seen := make(map[string]struct{})
	for _, entry := range entries {
		for _, taskType := range taskTypesForEntry(entry) {
			seen[taskType] = struct{}{}
		}
	}
	if len(seen) == 0 {
		// Legacy events did not carry routing metadata. A deletion can affect any
		// dataset artifact, so expose one log per supported category rather than
		// incorrectly labeling the event as Wiki.
		for _, taskType := range allDatasetTaskTypes {
			seen[taskType] = struct{}{}
		}
	}
	return sortedTaskTypes(seen)
}

func sortedTaskTypes(seen map[string]struct{}) []string {
	taskTypes := make([]string, 0, len(seen))
	for taskType := range seen {
		taskTypes = append(taskTypes, taskType)
	}
	sort.Strings(taskTypes)
	return taskTypes
}

func startDatasetCompileLog(ctx context.Context, tenantID, datasetID, claimToken string, entries []BacklogEntry) error {
	if kcDB == nil || claimToken == "" {
		return nil
	}
	now := time.Now()
	status := "1"
	groups := make(map[string][]BacklogEntry)
	for _, entry := range entries {
		for _, taskType := range taskTypesForEntry(entry) {
			groups[taskType] = append(groups[taskType], entry)
		}
	}
	if len(groups) == 0 {
		for _, taskType := range allDatasetTaskTypes {
			groups[taskType] = entries
		}
	}
	return kcDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, taskType := range sortedTaskTypesFromGroups(groups) {
			entryData := make([]any, 0, len(groups[taskType]))
			for _, entry := range groups[taskType] {
				entryData = append(entryData, map[string]any{
					"doc_id":     entry.DocID,
					"event_type": entry.EventType,
					"variants":   entry.Variants,
					"task_type":  taskType,
				})
			}
			log := entity.PipelineOperationLog{
				ID:              datasetCompileLogID(claimToken, taskType),
				DocumentID:      entity.DatasetLogDocumentID,
				TenantID:        tenantID,
				KbID:            datasetID,
				ParserID:        "knowledge_compile",
				DocumentName:    taskType,
				DocumentSuffix:  "",
				DocumentType:    "dataset",
				SourceFrom:      "knowledgebase",
				Progress:        0,
				ProcessBeginAt:  &now,
				DSL:             entity.JSONMap{"entries": entryData, "task_type": taskType},
				TaskType:        taskType,
				OperationStatus: common.RUNNING,
				Status:          &status,
			}
			result := tx.WithContext(ctx).Where("id = ?", log.ID).FirstOrCreate(&log)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			message := fmt.Sprintf("Created automatic %s dataset task for %d document event(s)", taskType, len(groups[taskType]))
			if err := createDatasetLogEvent(ctx, tx, log.ID, dao.EventTypeMessage, message); err != nil {
				return err
			}
		}
		return nil
	})
}

func updateDatasetCompileLog(ctx context.Context, claimToken string, taskTypes []string, progress float64, message string) error {
	if kcDB == nil || claimToken == "" {
		return nil
	}
	for _, taskType := range taskTypes {
		if err := updateDatasetCompileLogForType(ctx, datasetCompileLogID(claimToken, taskType), progress, message); err != nil {
			return err
		}
	}
	return nil
}

func updateDatasetCompileLogForType(ctx context.Context, logID string, progress float64, message string) error {
	return recordDatasetCompileLogEvent(ctx, logID, progress, common.RUNNING, dao.EventTypeMessage, message)
}

func finishDatasetCompileLog(ctx context.Context, claimToken string, taskTypes []string, operationStatus, message string, progress float64) error {
	if kcDB == nil || claimToken == "" {
		return nil
	}
	for _, taskType := range taskTypes {
		logID := datasetCompileLogID(claimToken, taskType)
		if err := recordDatasetCompileLogEvent(ctx, logID, progress, datasetLogOperationStatus(operationStatus), dao.EventTypeTerminal, message); err != nil {
			return err
		}
	}
	return nil
}

func recordDatasetCompileLogEvent(ctx context.Context, logID string, progress float64, operationStatus string, eventType int, message string) error {
	return kcDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var log entity.PipelineOperationLog
		if err := tx.WithContext(ctx).Where("id = ?", logID).First(&log).Error; err != nil {
			return err
		}
		duration := log.ProcessDuration
		if log.ProcessBeginAt != nil {
			duration = max(0, time.Since(*log.ProcessBeginAt).Seconds())
		}
		if err := tx.WithContext(ctx).Model(&entity.PipelineOperationLog{}).Where("id = ?", logID).Updates(map[string]any{
			"progress":         progress,
			"process_duration": duration,
			"operation_status": operationStatus,
		}).Error; err != nil {
			return err
		}
		return createDatasetLogEvent(ctx, tx, logID, eventType, message)
	})
}

func createDatasetLogEvent(ctx context.Context, db *gorm.DB, logID string, eventType int, message string) error {
	// Dataset compilation has no ingestion_task row, so its immutable pipeline
	// log ID also serves as the event's stable task identity.
	pipelineLogID := logID
	return dao.NewIngestionTaskLogDAO().Create(ctx, db, &entity.IngestionTaskLog{
		TaskID:        logID,
		PipelineLogID: &pipelineLogID,
		Checkpoint:    entity.JSONMap{},
		EventType:     eventType,
		Message:       message,
	})
}

func sortedTaskTypesFromGroups(groups map[string][]BacklogEntry) []string {
	seen := make(map[string]struct{}, len(groups))
	for taskType := range groups {
		seen[taskType] = struct{}{}
	}
	return sortedTaskTypes(seen)
}
