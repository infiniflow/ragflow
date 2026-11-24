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

package dataset

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/progressstore"
)

// IngestionProgressResponse is the one-screen summary served to the UI.
// Status is authoritative (MySQL run row); Stage/Unit/Percent come from the
// running-state snapshot and may be absent when that snapshot is unavailable.
type IngestionProgressResponse struct {
	Status    string     `json:"status"`
	Phase     string     `json:"phase,omitempty"`
	Stage     string     `json:"stage,omitempty"`
	Unit      string     `json:"unit,omitempty"`
	Current   uint32     `json:"current"`
	Total     uint32     `json:"total"`
	Percent   float64    `json:"percent"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	DurationS float64    `json:"duration_s"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	ChunkNum  int64      `json:"chunk_num"`
	Fallback  bool       `json:"fallback"`
}

// mergeProgressSnapshot combines the authoritative run state with the latest
// running-state snapshot. A missing snapshot — or one with no active unit —
// marks fallback instead of presenting a fabricated 0/0 position.
func mergeProgressSnapshot(run *entity.PipelineOperationLog, snap *progressstore.Snapshot, chunkNum int64) *IngestionProgressResponse {
	resp := &IngestionProgressResponse{ChunkNum: chunkNum}
	if run != nil {
		resp.Status = run.OperationStatus
		resp.StartedAt = run.ProcessBeginAt
		resp.DurationS = run.ProcessDuration
	}
	if snap == nil {
		resp.Fallback = true
		return resp
	}
	resp.Phase = snap.Status
	if !snap.UpdatedAt.IsZero() {
		updated := snap.UpdatedAt
		resp.UpdatedAt = &updated
	}
	var active *progressstore.Stage
	for i := range snap.Stages {
		stage := &snap.Stages[i]
		if stage.Active == "" {
			continue
		}
		if _, ok := stage.Units[stage.Active]; ok {
			active = stage
			break
		}
	}
	if active == nil {
		resp.Fallback = true
		return resp
	}
	pos := active.Units[active.Active]
	resp.Stage = active.Name
	resp.Unit = active.Active
	resp.Current = pos.Current
	resp.Total = pos.Total
	if pos.Total > 0 {
		resp.Percent = float64(pos.Current) / float64(pos.Total) * 100
	}
	return resp
}

// progressReader is the read-only seam for the running-state snapshot.
type progressReader interface {
	Load(ctx context.Context, id string) (*progressstore.Snapshot, bool)
}

// SetProgressReader installs a reader (tests inject a fake; production uses
// the Kvrocks-backed store installed by NewDatasetService).
func (d *DatasetService) SetProgressReader(reader progressReader) {
	d.progressReader = reader
}

// GetIngestionProgress returns the summary for one immutable pipeline log.
// Access control and dataset scoping mirror ListIngestionMessages.
func (d *DatasetService) GetIngestionProgress(ctx context.Context, datasetID, logID string) (*IngestionProgressResponse, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeArgumentError, errors.New(`lack of "Dataset ID"`)
	}
	if logID == "" {
		return nil, common.CodeArgumentError, errors.New(`lack of "Log ID"`)
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, permission.OperationRead); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
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

	var snap *progressstore.Snapshot
	if d.progressReader != nil {
		snap, _ = d.progressReader.Load(ctx, logID)
	}
	var chunkNum int64
	if run.DocumentID != "" {
		if doc, docErr := d.documentDAO.GetByID(ctx, dao.DB, run.DocumentID); docErr == nil && doc != nil {
			chunkNum = doc.ChunkNum
		}
	}
	return mergeProgressSnapshot(run, snap, chunkNum), common.CodeSuccess, nil
}
