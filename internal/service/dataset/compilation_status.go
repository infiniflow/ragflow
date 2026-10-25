package dataset

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

// CompilationStatus is the dataset-level knowledge-compile lifecycle state
// surfaced by GET /datasets/:id/compilation/status.
//
// State only takes one of idle/pending/running/completed. Error is NOT a fifth
// state: it is a diagnostic attached to a pending/running batch left for retry,
// so the frontend should test `error != ""` on its own (and hide the counts)
// rather than treating it as a peer of state.
type CompilationStatus struct {
	Kind            string     `json:"kind"`                        // normalized requested compile kind
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
func (d *DatasetService) GetDatasetCompilationStatus(ctx context.Context, userID, datasetID, kind string) (CompilationStatus, common.ErrorCode, error) {
	if datasetID == "" {
		return CompilationStatus{}, common.CodeDataError, errors.New("dataset_id is required")
	}
	if err := d.CheckAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationRead); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return CompilationStatus{}, code, permissionErr
	}
	requestedKind := strings.ToLower(strings.TrimSpace(kind))
	normalizedKind := normalizeCompilationKind(requestedKind)
	if requestedKind != "" && normalizedKind == "" {
		return CompilationStatus{}, common.CodeDataError, errors.New("unsupported compilation kind")
	}
	st := CompilationStatus{Kind: requestedKind, State: entity.DatasetStateIdle}
	db := dao.GetDB()
	if db == nil {
		return st, common.CodeSuccess, nil
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.KnowledgeCompileDataset
		if err := tx.Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // never compiled -> idle
			}
			return err
		}
		st.State = row.State
		if st.State == "" {
			st.State = entity.DatasetStateIdle
		}
		st.Error = row.ErrorMsg
		st.Progress = row.Progress
		st.CurrentPhase = row.CurrentPhase
		st.ProgressMsg = row.ProgressMsg
		var err error
		st.Inflight, err = countCompilationStatusEntries(tx, (entity.KnowledgeCompileInflight{}).TableName(), datasetID, normalizedKind)
		if err != nil {
			return err
		}
		st.Backlog, err = countCompilationStatusEntries(tx, (entity.KnowledgeCompileBacklog{}).TableName(), datasetID, normalizedKind)
		if err != nil {
			return err
		}
		if requestedKind != "" && st.Inflight == 0 && st.Backlog == 0 {
			st.State = entity.DatasetStateIdle
			st.Error = ""
			st.Progress = 0
			st.CurrentPhase = ""
			st.ProgressMsg = ""
		}
		st.LastCompletedAt = row.LastCompletedAt
		st.UpdatedAt = row.UpdatedAt
		return nil
	})
	if err != nil {
		return CompilationStatus{}, common.CodeServerError, err
	}
	return st, common.CodeSuccess, nil
}

func countCompilationStatusEntries(db *gorm.DB, table, datasetID, kind string) (int, error) {
	query := db.Table(table).Where("dataset_id = ?", datasetID)
	if kind == "" {
		var count int64
		err := query.Count(&count).Error
		return int(count), err
	}
	rows, err := query.Select("variants", "task_types").Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var entry entity.KnowledgeCompileEntry
		if err := db.ScanRows(rows, &entry); err != nil {
			return 0, err
		}
		if compilationStatusEntryMatches(entry, kind) {
			count++
		}
	}
	return count, rows.Err()
}

func compilationStatusEntryMatches(entry entity.KnowledgeCompileEntry, kind string) bool {
	for _, taskType := range entry.TaskTypes {
		if normalizeCompilationKind(taskType) == kind {
			return true
		}
	}
	if len(entry.TaskTypes) > 0 {
		return false
	}
	if len(entry.Variants) == 0 {
		// Legacy entries without routing metadata can affect every product kind.
		return true
	}
	for _, variant := range entry.Variants {
		if normalizeCompilationKind(variant) == kind {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(variant), "structure") &&
			(kind == "Graph" || kind == "Timeline" || kind == "PageIndex") {
			return true
		}
	}
	return false
}

func normalizeCompilationKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "wiki", "artifact":
		return "Wiki"
	case "tree", "raptor":
		return "Tree"
	case "graph", "knowledge_graph", "knowledgegraph":
		return "Graph"
	case "mindmap", "mind_map":
		return "Mindmap"
	case "timeline":
		return "Timeline"
	case "pageindex", "page_index", "skill":
		return "PageIndex"
	default:
		return ""
	}
}
