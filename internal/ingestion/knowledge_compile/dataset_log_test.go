package knowledge_compile

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestDatasetCompileLogLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:dataset-log?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline log: %v", err)
	}
	oldDB := kcDB
	kcDB = db
	t.Cleanup(func() { kcDB = oldDB })

	entries := []BacklogEntry{
		{DocID: "doc-1", EventType: string(EventTypeCompleted), Variants: []string{"wiki"}, TaskTypes: []string{kccommon.TaskTypeWiki}},
		{DocID: "doc-2", EventType: string(EventTypeCompleted), Variants: []string{"structure"}, TaskTypes: []string{kccommon.TaskTypeGraph}},
	}
	if err := startDatasetCompileLog(t.Context(), "tenant-1", "kb-1", "claim-1", entries); err != nil {
		t.Fatalf("start dataset log: %v", err)
	}
	if err := updateDatasetCompileLog(t.Context(), "claim-1", []string{kccommon.TaskTypeWiki, kccommon.TaskTypeGraph}, 0.5, "Comparing knowledge contributions: 2 affected page(s)"); err != nil {
		t.Fatalf("update dataset log: %v", err)
	}
	if err := finishDatasetCompileLog(t.Context(), "claim-1", []string{kccommon.TaskTypeWiki, kccommon.TaskTypeGraph}, common.COMPLETED, "Knowledge compilation completed", 1); err != nil {
		t.Fatalf("finish dataset log: %v", err)
	}

	var logs []entity.PipelineOperationLog
	if err := db.Order("task_type").Find(&logs).Error; err != nil {
		t.Fatalf("load dataset logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("want one log per task type, got %d: %+v", len(logs), logs)
	}
	for _, log := range logs {
		if log.DocumentID != datasetLogDocumentID || log.OperationStatus != "DONE" || log.Progress != 1 {
			t.Fatalf("unexpected dataset log state: %+v", log)
		}
		if log.ID != datasetCompileLogID("claim-1", log.TaskType) {
			t.Fatalf("dataset log id is not task-type scoped: %+v", log)
		}
		if log.ProgressMsg == nil || !strings.Contains(*log.ProgressMsg, "2 affected page(s)") || !strings.Contains(*log.ProgressMsg, "completed") {
			t.Fatalf("progress messages were not persisted: %v", log.ProgressMsg)
		}
		if log.DSL["task_type"] != log.TaskType {
			t.Fatalf("dataset log DSL task type = %v, want %s", log.DSL["task_type"], log.TaskType)
		}
		entries, ok := log.DSL["entries"].([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("dataset log entries were not split by task type: %+v", log.DSL["entries"])
		}
	}
}
