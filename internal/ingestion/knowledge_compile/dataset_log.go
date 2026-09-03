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
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

const datasetLogDocumentID = "graph_raptor_x"

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

func datasetCompileLogID(claimToken string) string {
	sum := sha256.Sum256([]byte("wiki-dataset-log\x00" + claimToken))
	return hex.EncodeToString(sum[:16])
}

func startDatasetCompileLog(ctx context.Context, tenantID, datasetID, claimToken string, entries []BacklogEntry) error {
	if kcDB == nil || claimToken == "" {
		return nil
	}
	now := time.Now()
	status := "1"
	message := timestampProgressMessage(fmt.Sprintf("Created automatic Wiki dataset task for %d document event(s)", len(entries)))
	entryData := make([]any, 0, len(entries))
	for _, entry := range entries {
		entryData = append(entryData, map[string]any{
			"doc_id":     entry.DocID,
			"event_type": entry.EventType,
			"variants":   entry.Variants,
		})
	}
	log := entity.PipelineOperationLog{
		ID:              datasetCompileLogID(claimToken),
		DocumentID:      datasetLogDocumentID,
		TenantID:        tenantID,
		KbID:            datasetID,
		ParserID:        "wiki",
		DocumentName:    "Wiki",
		DocumentSuffix:  "",
		DocumentType:    "dataset",
		SourceFrom:      "knowledgebase",
		Progress:        0,
		ProgressMsg:     &message,
		ProcessBeginAt:  &now,
		DSL:             entity.JSONMap{"entries": entryData},
		TaskType:        string(entity.PipelineTaskTypeWiki),
		OperationStatus: common.RUNNING,
		Status:          &status,
	}
	return kcDB.WithContext(ctx).Where("id = ?", log.ID).FirstOrCreate(&log).Error
}

func updateDatasetCompileLog(ctx context.Context, claimToken string, progress float64, message string) error {
	if kcDB == nil || claimToken == "" {
		return nil
	}
	logID := datasetCompileLogID(claimToken)
	var log entity.PipelineOperationLog
	if err := kcDB.WithContext(ctx).Where("id = ?", logID).First(&log).Error; err != nil {
		return err
	}
	progressMessage := ""
	if log.ProgressMsg != nil {
		progressMessage = *log.ProgressMsg
	}
	progressMessage = appendProgressMessage(progressMessage, timestampProgressMessage(message))
	duration := log.ProcessDuration
	if log.ProcessBeginAt != nil {
		duration = max(0, time.Since(*log.ProcessBeginAt).Seconds())
	}
	return kcDB.WithContext(ctx).Model(&entity.PipelineOperationLog{}).Where("id = ?", logID).Updates(map[string]any{
		"progress":         progress,
		"progress_msg":     progressMessage,
		"process_duration": duration,
		"operation_status": common.RUNNING,
	}).Error
}

func finishDatasetCompileLog(ctx context.Context, claimToken, operationStatus, message string, progress float64) error {
	if kcDB == nil || claimToken == "" {
		return nil
	}
	logID := datasetCompileLogID(claimToken)
	var log entity.PipelineOperationLog
	if err := kcDB.WithContext(ctx).Where("id = ?", logID).First(&log).Error; err != nil {
		return err
	}
	progressMessage := ""
	if log.ProgressMsg != nil {
		progressMessage = *log.ProgressMsg
	}
	progressMessage = appendProgressMessage(progressMessage, timestampProgressMessage(message))
	duration := log.ProcessDuration
	if log.ProcessBeginAt != nil {
		duration = max(0, time.Since(*log.ProcessBeginAt).Seconds())
	}
	return kcDB.WithContext(ctx).Model(&entity.PipelineOperationLog{}).Where("id = ?", logID).Updates(map[string]any{
		"progress":         progress,
		"progress_msg":     progressMessage,
		"process_duration": duration,
		"operation_status": datasetLogOperationStatus(operationStatus),
	}).Error
}
