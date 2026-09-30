package knowledge_compile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestDatasetCompileLogLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:dataset-log?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}, &entity.IngestionTaskLog{}); err != nil {
		t.Fatalf("migrate ingestion logs: %v", err)
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
	if err := startDatasetCompileLog(t.Context(), "tenant-1", "kb-1", "claim-1", entries); err != nil {
		t.Fatalf("restart dataset log: %v", err)
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
		if log.DocumentID != entity.DatasetLogDocumentID || log.RunCount != nil || log.OperationStatus != "DONE" || log.Progress != 1 {
			t.Fatalf("unexpected dataset log state: %+v", log)
		}
		if log.ID != datasetCompileLogID("claim-1", log.TaskType) {
			t.Fatalf("dataset log id is not task-type scoped: %+v", log)
		}
		if log.ProgressMsg != nil {
			t.Fatalf("dataset log retained progress_msg: %q", *log.ProgressMsg)
		}
		if log.DSL["task_type"] != log.TaskType {
			t.Fatalf("dataset log DSL task type = %v, want %s", log.DSL["task_type"], log.TaskType)
		}
		entries, ok := log.DSL["entries"].([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("dataset log entries were not split by task type: %+v", log.DSL["entries"])
		}

		var events []entity.IngestionTaskLog
		if err := db.Where("pipeline_log_id = ?", log.ID).Order("id ASC").Find(&events).Error; err != nil {
			t.Fatalf("load dataset log events: %v", err)
		}
		if len(events) != 3 {
			t.Fatalf("dataset log %s event count = %d, want 3: %+v", log.ID, len(events), events)
		}
		if events[0].EventType != dao.EventTypeMessage || !strings.Contains(events[0].Message, "Created automatic "+log.TaskType+" dataset task") {
			t.Fatalf("unexpected initial event: %+v", events[0])
		}
		if events[1].EventType != dao.EventTypeMessage || !strings.Contains(events[1].Message, "2 affected page(s)") {
			t.Fatalf("unexpected progress event: %+v", events[1])
		}
		if events[2].EventType != dao.EventTypeTerminal || !strings.Contains(events[2].Message, "completed") {
			t.Fatalf("unexpected terminal event: %+v", events[2])
		}
		for _, event := range events {
			if event.TaskID != log.ID || event.PipelineLogID == nil || *event.PipelineLogID != log.ID {
				t.Fatalf("event is not scoped to dataset log %s: %+v", log.ID, event)
			}
		}
	}
}

func TestTaskTypesForEntryLegacyFallbacks(t *testing.T) {
	got := taskTypesForEntry(BacklogEntry{Variants: []string{"structure"}})
	want := []string{kccommon.TaskTypeGraph, kccommon.TaskTypePageIndex, kccommon.TaskTypeTimeline}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy structure task types = %v, want %v", got, want)
	}

	got = taskTypesForEntry(BacklogEntry{})
	if !reflect.DeepEqual(got, allDatasetTaskTypes) {
		t.Fatalf("unknown entry task types = %v, want %v", got, allDatasetTaskTypes)
	}

	if got = taskTypesForEntry(BacklogEntry{EventType: string(EventTypeDeleted)}); len(got) != 0 {
		t.Fatalf("unscoped deletion task types = %v, want none", got)
	}
}
