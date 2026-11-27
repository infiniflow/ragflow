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

// latestActiveStage returns the newest stage with a valid active unit. When
// skipTerminal is set, completed/failed stages are ignored so a later running
// stage supplies the summary.
func latestActiveStage(snap *progressstore.Snapshot, skipTerminal bool) *progressstore.Stage {
	for i := len(snap.Stages) - 1; i >= 0; i-- {
		stage := &snap.Stages[i]
		if stage.Active == "" {
			continue
		}
		if _, ok := stage.Units[stage.Active]; !ok {
			continue
		}
		if skipTerminal && (stage.Phase == progressstore.PhaseDone || stage.Phase == progressstore.PhaseFailed) {
			continue
		}
		return stage
	}
	return nil
}

// latestRunningStage returns the newest stage that is still running, even when
// it has no Active unit yet. It lets the summary identify the current component
// without borrowing a completed stage's position.
func latestRunningStage(snap *progressstore.Snapshot) *progressstore.Stage {
	for i := len(snap.Stages) - 1; i >= 0; i-- {
		if snap.Stages[i].Phase == progressstore.PhaseRunning {
			return &snap.Stages[i]
		}
	}
	return nil
}

// mergeProgressSnapshot combines the authoritative run state with the latest
// running-state snapshot. A missing snapshot — or one with no active unit —
// marks fallback instead of presenting a fabricated 0/0 position.
func mergeProgressSnapshot(run *entity.PipelineOperationLog, snap *progressstore.Snapshot, chunkNum int64, attemptStartedAt *time.Time) *IngestionProgressResponse {
	resp := &IngestionProgressResponse{ChunkNum: chunkNum}
	if run != nil {
		resp.Status = run.OperationStatus
		resp.StartedAt = run.ProcessBeginAt
		resp.DurationS = run.ProcessDuration
		// ProcessDuration is written only at settlement, so it stays 0 for a
		// running run and the UI would show "—". Derive a live elapsed from the
		// immutable begin time; terminal runs keep their persisted duration.
		if !isTerminalIngestionLogStatus(run.OperationStatus) && run.ProcessBeginAt != nil {
			resp.DurationS = max(0, time.Since(*run.ProcessBeginAt).Seconds())
		}
	}
	if snap == nil {
		resp.Fallback = true
		return resp
	}
	// A snapshot left by an earlier execution — or written before attempts
	// existed — describes positions that no longer belong to this run. The
	// document row is stamped on every worker takeover, so it identifies the
	// current execution; the run row's begin time is fixed at creation.
	if attemptStartedAt != nil &&
		(snap.AttemptID == "" || snap.AttemptStartedAt.Before(*attemptStartedAt)) {
		resp.Fallback = true
		return resp
	}
	resp.Phase = snap.Status
	if !snap.UpdatedAt.IsZero() {
		updated := snap.UpdatedAt
		resp.UpdatedAt = &updated
	}
	// Prefer the most recently reported stage that is still running. A
	// completed stage keeps its Active unit, so scanning forward would freeze
	// the summary on a finished stage and hide the one actually in progress.
	active := latestActiveStage(snap, true)
	if active == nil {
		// A stage can be running before it reports its first unit (e.g. a
		// component has just entered). Surface that stage's identity instead
		// of falling back to a completed stage's stale position, which would
		// read as a frozen 100% bar.
		if running := latestRunningStage(snap); running != nil {
			resp.Stage = running.Name
			// No unit position exists yet: mark fallback instead of fabricating
			// 0/0. Stage stays set so the UI can still name the current phase.
			// Keep resp.Phase as the snapshot status (uppercase) — never overwrite
			// it with the stage's lowercase lifecycle label.
			resp.Fallback = true
			return resp
		}
		// At settlement no stage is running; still surface the latest position
		// so consumers do not see a fabricated 0/0.
		active = latestActiveStage(snap, false)
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
	var attemptStartedAt *time.Time
	if run.DocumentID != "" {
		if doc, docErr := d.documentDAO.GetByID(ctx, dao.DB, run.DocumentID); docErr == nil && doc != nil {
			chunkNum = doc.ChunkNum
			attemptStartedAt = doc.ProcessBeginAt
		}
	}
	return mergeProgressSnapshot(run, snap, chunkNum, attemptStartedAt), common.CodeSuccess, nil
}
