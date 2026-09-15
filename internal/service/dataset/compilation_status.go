package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// CompilationStatus is the dataset-level knowledge-compile lifecycle state
// surfaced by GET /datasets/:id/compilation/status. It is the Go scheduler
// contract that replaces the Python-era RunIndex/TraceIndex task progress for
// API_PROXY_SCHEME=go / hybrid.
//
// State only takes one of idle/pending/running/completed. Error is NOT a fifth
// state: it is a diagnostic attached to a pending/running batch left for retry,
// so the frontend should test `error != ""` on its own (and hide the counts)
// rather than treating it as a peer of state.
type CompilationStatus struct {
	State           string     `json:"state"`                       // idle | pending | running | completed
	Error           string     `json:"error,omitempty"`             // most recent batch diagnostic (empty when none)
	Progress        float64    `json:"progress"`                    // current claimed batch progress
	CurrentPhase    string     `json:"current_phase,omitempty"`     // current compiler phase
	ProgressMsg     string     `json:"progress_msg,omitempty"`      // bounded phase log for the UI
	Inflight        int        `json:"inflight"`                    // entries currently claimed (in-flight batch)
	Backlog         int        `json:"backlog"`                     // entries still waiting to be claimed
	LastCompletedAt *time.Time `json:"last_completed_at,omitempty"` // last backlog drain
	UpdatedAt       time.Time  `json:"updated_at"`                  // last scheduling-row activity
}

// GetDatasetCompilationStatus returns the scheduling-row lifecycle state for a
// dataset after verifying the calling user owns it. When no row exists the
// dataset has never had any compile work, so the state is idle.
func (d *DatasetService) GetDatasetCompilationStatus(ctx context.Context, userID, datasetID string) (CompilationStatus, common.ErrorCode, error) {
	if datasetID == "" {
		return CompilationStatus{}, common.CodeDataError, errors.New("dataset_id is required")
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return CompilationStatus{}, common.CodeDataError, errors.New("no authorization")
	}
	st := CompilationStatus{State: entity.DatasetStateIdle}
	db := dao.GetDB()
	if db == nil {
		return st, common.CodeSuccess, nil
	}
	var row entity.KnowledgeCompileDataset
	err := db.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return st, common.CodeSuccess, nil // never compiled -> idle
	}
	if err != nil {
		return st, common.CodeServerError, err
	}
	st.State = row.State
	if st.State == "" {
		st.State = entity.DatasetStateIdle
	}
	st.Error = row.ErrorMsg
	st.Progress = row.Progress
	st.CurrentPhase = row.CurrentPhase
	st.ProgressMsg = row.ProgressMsg
	st.Inflight = jsonArrayLen(row.InflightDocIDs)
	st.Backlog = jsonArrayLen(row.BacklogDocIDs)
	st.LastCompletedAt = row.LastCompletedAt
	st.UpdatedAt = row.UpdatedAt
	return st, common.CodeSuccess, nil
}

// jsonArrayLen counts the top-level elements of a JSON array stored as TEXT.
// The *_doc_ids columns hold a `[]BacklogEntry` array ({doc_id,event_type,seq}),
// so each element is one scheduling entry (NOT a deduplicated doc). Empty or
// malformed strings count as 0.
func jsonArrayLen(s string) int {
	if s == "" {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return 0
	}
	return len(arr)
}
