package service

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestTerminalErrorCodeSurvivesRetentionAndIsRunScoped(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	svc := NewIngestionTaskService()
	svc.logSettings.MaxRowsPerRun = 4
	insertFoldPipelineLog(t, db, "license-run", entity.TaskStatusFail, 1)
	for range 8 {
		insertFoldEvent(t, db, "license-run", "task-1", dao.EventTypeMessage, 0, "", "detail")
	}
	if err := svc.RecordTerminal(t.Context(), "license-run", "task-1", "license detail", "license_required"); err != nil {
		t.Fatal(err)
	}
	// Settlement retries preserve the original failure metadata.
	if err := svc.RecordTerminal(t.Context(), "license-run", "task-1", "generic failure", ""); err != nil {
		t.Fatal(err)
	}
	insertFoldPipelineLog(t, db, "retry-run", entity.TaskStatusFail, 2)
	if err := svc.RecordTerminal(t.Context(), "retry-run", "task-1", "ordinary failure", ""); err != nil {
		t.Fatal(err)
	}
	logs, err := dao.NewIngestionTaskLogDAO().LatestEventsByPipelineLogIDs(t.Context(), db, []string{"license-run", "retry-run"})
	if err != nil {
		t.Fatal(err)
	}
	for runID, wantCode := range map[string]string{"license-run": "license_required", "retry-run": ""} {
		event := IngestionEventItemFromLog(logs[runID])
		if event.ErrorCode != wantCode {
			t.Errorf("%s code = %q, want %q", runID, event.ErrorCode, wantCode)
		}
		if runID == "license-run" && (event.EventType != dao.EventTypeTerminal || event.Message != "license detail") {
			t.Errorf("retention lost terminal detail: %+v", event)
		}
	}
}

func TestIngestionEventErrorCodeOmittedWithoutStringMetadata(t *testing.T) {
	for _, checkpoint := range []entity.JSONMap{nil, {}, {"error_code": 123}} {
		event := IngestionEventItemFromLog(&entity.IngestionTaskLog{Checkpoint: checkpoint})
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "error_code") {
			t.Fatalf("unexpected error code in %s", data)
		}
	}
}
