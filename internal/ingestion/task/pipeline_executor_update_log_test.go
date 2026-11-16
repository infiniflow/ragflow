package task

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// seedOpenLogRow inserts an OPEN pipeline_operation_log row (operation_status
// in {"0","1","5"}) so updateOpenLogRow's WHERE clause can target it.
func seedOpenLogRow(t *testing.T, id, docID string) {
	t.Helper()
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              id,
		DocumentID:      docID,
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		OperationStatus: string(entity.TaskStatusSchedule),
	}).Error; err != nil {
		t.Fatalf("seed open pipeline log: %v", err)
	}
}

func deriveTestDoc(docID string) entity.Document {
	name := "derive.pdf"
	status := "1"
	return entity.Document{
		ID:         docID,
		KbID:       "kb-1",
		ParserID:   "naive",
		SourceType: "local",
		Type:       "pdf",
		Name:       &name,
		Suffix:     ".pdf",
		Status:     &status,
	}
}

func loadLog(t *testing.T, id string) entity.PipelineOperationLog {
	t.Helper()
	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "id = ?", id).Error; err != nil {
		t.Fatalf("load pipeline log %s: %v", id, err)
	}
	return log
}

// TestUpdateOpenLogRowDerivesLogFieldsFromInput locks the core refactor:
// operation_status / pipeline_id / pipeline_version_id / dsl are derived from
// PipelineLogInput inside updateOpenLogRow, while pipeline_title is a passed
// parameter. A wrong implementation reading the old scalars would fail here.
func TestUpdateOpenLogRowDerivesLogFieldsFromInput(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	seedOpenLogRow(t, "run-derive-1", "doc-derive-1")

	doc := deriveTestDoc("doc-derive-1")
	// Adversarial: input.Status (done) differs from doc.Status ("1"). The
	// operation_status must come from input.Status, not from the document.
	err := updateOpenLogRow(t.Context(), dao.DB, PipelineLogInput{
		DocumentID:              "doc-derive-1",
		Status:                  string(entity.TaskStatusDone),
		PipelineID:              "canvas-1",
		PipelineVersionID:       "v1",
		PipelineBindingResolved: true,
		DSL:                     `{"steps":["a","b"]}`,
		PipelineLogID:           "run-derive-1",
	}, "Canvas Title", nil, doc)
	if err != nil {
		t.Fatalf("updateOpenLogRow: %v", err)
	}

	log := loadLog(t, "run-derive-1")
	if log.OperationStatus != string(entity.TaskStatusDone) {
		t.Fatalf("OperationStatus = %q, want %q (from input.Status)", log.OperationStatus, entity.TaskStatusDone)
	}
	if log.PipelineID == nil || *log.PipelineID != "canvas-1" {
		t.Fatalf("PipelineID = %v, want canvas-1 (from input.PipelineID)", log.PipelineID)
	}
	if log.PipelineVersionID == nil || *log.PipelineVersionID != "v1" {
		t.Fatalf("PipelineVersionID = %v, want v1 (from input.PipelineVersionID)", log.PipelineVersionID)
	}
	if log.PipelineTitle == nil || *log.PipelineTitle != "Canvas Title" {
		t.Fatalf("PipelineTitle = %v, want Canvas Title (passed param)", log.PipelineTitle)
	}
	if _, ok := log.DSL["steps"]; !ok {
		t.Fatalf("DSL = %v, want parsed from input.DSL", log.DSL)
	}
}

// TestUpdateOpenLogRowDerivesStatusValueFromDoc verifies the status column is
// still derived from the reloaded doc (not from input.Status), with the "1"
// default when the document carries no status.
func TestUpdateOpenLogRowDerivesStatusValueFromDoc(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	cases := []struct {
		name       string
		rowID      string
		docStatus  *string
		wantStatus string
	}{
		{name: "nil doc status defaults to 1", rowID: "run-status-nil", docStatus: nil, wantStatus: "1"},
		{name: "doc status wins", rowID: "run-status-set", docStatus: strPtr("8"), wantStatus: "8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seedOpenLogRow(t, tc.rowID, "doc-"+tc.rowID)
			doc := deriveTestDoc("doc-" + tc.rowID)
			doc.Status = tc.docStatus

			err := updateOpenLogRow(t.Context(), dao.DB, PipelineLogInput{
				DocumentID:    "doc-" + tc.rowID,
				Status:        string(entity.TaskStatusDone),
				PipelineLogID: tc.rowID,
			}, "title", nil, doc)
			if err != nil {
				t.Fatalf("updateOpenLogRow: %v", err)
			}

			log := loadLog(t, tc.rowID)
			if log.Status == nil || *log.Status != tc.wantStatus {
				t.Fatalf("Status = %v, want %q (from doc.Status)", log.Status, tc.wantStatus)
			}
			if log.OperationStatus != string(entity.TaskStatusDone) {
				t.Fatalf("OperationStatus = %q, want %q (from input.Status)", log.OperationStatus, entity.TaskStatusDone)
			}
		})
	}
}

// TestUpdateOpenLogRowDerivesDSLFromInput covers the three DSL branches:
// valid JSON, empty string, and malformed JSON (stored as {"raw": ...}).
func TestUpdateOpenLogRowDerivesDSLFromInput(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	cases := []struct {
		name      string
		rowID     string
		dsl       string
		assertion func(t *testing.T, dsl entity.JSONMap)
	}{
		{
			name:  "valid json",
			rowID: "run-dsl-valid",
			dsl:   `{"steps":["a","b"]}`,
			assertion: func(t *testing.T, dsl entity.JSONMap) {
				if _, ok := dsl["steps"]; !ok {
					t.Fatalf("DSL = %v, want parsed steps", dsl)
				}
			},
		},
		{
			name:  "empty dsl becomes empty map",
			rowID: "run-dsl-empty",
			dsl:   "",
			assertion: func(t *testing.T, dsl entity.JSONMap) {
				if len(dsl) != 0 {
					t.Fatalf("DSL = %v, want empty JSONMap", dsl)
				}
			},
		},
		{
			name:  "malformed dsl stored as raw",
			rowID: "run-dsl-invalid",
			dsl:   "not-json",
			assertion: func(t *testing.T, dsl entity.JSONMap) {
				if dsl["raw"] != "not-json" {
					t.Fatalf("DSL = %v, want raw=not-json", dsl)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seedOpenLogRow(t, tc.rowID, "doc-"+tc.rowID)
			doc := deriveTestDoc("doc-" + tc.rowID)

			err := updateOpenLogRow(t.Context(), dao.DB, PipelineLogInput{
				DocumentID:    "doc-" + tc.rowID,
				Status:        string(entity.TaskStatusDone),
				DSL:           tc.dsl,
				PipelineLogID: tc.rowID,
			}, "title", nil, doc)
			if err != nil {
				t.Fatalf("updateOpenLogRow: %v", err)
			}

			tc.assertion(t, loadLog(t, tc.rowID).DSL)
		})
	}
}

// TestUpdateOpenLogRowPipelineBindingGating verifies pipeline_id and
// pipeline_version_id are only written when the binding is resolved AND a
// pipeline id exists — the coupled-field / single-field adversarial cases.
func TestUpdateOpenLogRowPipelineBindingGating(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	cases := []struct {
		name        string
		rowID       string
		resolved    bool
		pipelineID  string
		version     string
		wantPID     *string
		wantVersion *string
	}{
		{
			name:       "unresolved binding writes no pipeline columns",
			rowID:      "run-gate-unresolved",
			resolved:   false,
			pipelineID: "canvas-x",
			version:    "v9",
		},
		{
			name:        "resolved with version writes both",
			rowID:       "run-gate-both",
			resolved:    true,
			pipelineID:  "canvas-x",
			version:     "v9",
			wantPID:     strPtr("canvas-x"),
			wantVersion: strPtr("v9"),
		},
		{
			name:       "resolved without version leaves version nil",
			rowID:      "run-gate-noversion",
			resolved:   true,
			pipelineID: "canvas-x",
			wantPID:    strPtr("canvas-x"),
		},
		{
			name:     "resolved without pipeline leaves both nil",
			rowID:    "run-gate-nopipeline",
			resolved: true,
			version:  "v9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seedOpenLogRow(t, tc.rowID, "doc-"+tc.rowID)
			doc := deriveTestDoc("doc-" + tc.rowID)

			err := updateOpenLogRow(t.Context(), dao.DB, PipelineLogInput{
				DocumentID:              "doc-" + tc.rowID,
				Status:                  string(entity.TaskStatusDone),
				PipelineID:              tc.pipelineID,
				PipelineVersionID:       tc.version,
				PipelineBindingResolved: tc.resolved,
				PipelineLogID:           tc.rowID,
			}, "title", nil, doc)
			if err != nil {
				t.Fatalf("updateOpenLogRow: %v", err)
			}

			log := loadLog(t, tc.rowID)
			assertStringPtr(t, "PipelineID", log.PipelineID, tc.wantPID)
			assertStringPtr(t, "PipelineVersionID", log.PipelineVersionID, tc.wantVersion)
		})
	}
}

func assertStringPtr(t *testing.T, field string, got, want *string) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s = %q, want nil", field, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s = %v, want %q", field, got, *want)
	}
}
