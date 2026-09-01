package knowledge_compile

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
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

	entries := []BacklogEntry{{DocID: "doc-1", EventType: string(EventTypeCompleted), Variants: []string{"wiki"}}}
	if err := startDatasetCompileLog(t.Context(), "tenant-1", "kb-1", "claim-1", entries); err != nil {
		t.Fatalf("start dataset log: %v", err)
	}
	if err := updateDatasetCompileLog(t.Context(), "claim-1", 0.5, "Comparing Wiki contributions: 2 affected page(s)"); err != nil {
		t.Fatalf("update dataset log: %v", err)
	}
	if err := finishDatasetCompileLog(t.Context(), "claim-1", common.COMPLETED, "Wiki dataset compilation completed", 1); err != nil {
		t.Fatalf("finish dataset log: %v", err)
	}

	var log entity.PipelineOperationLog
	if err := db.Where("id = ?", datasetCompileLogID("claim-1")).First(&log).Error; err != nil {
		t.Fatalf("load dataset log: %v", err)
	}
	if log.DocumentID != datasetLogDocumentID || log.TaskType != string(entity.PipelineTaskTypeWiki) {
		t.Fatalf("unexpected dataset log identity: %+v", log)
	}
	if log.OperationStatus != "DONE" || log.Progress != 1 {
		t.Fatalf("unexpected final state: status=%s progress=%v", log.OperationStatus, log.Progress)
	}
	if log.ProgressMsg == nil || !strings.Contains(*log.ProgressMsg, "2 affected page(s)") || !strings.Contains(*log.ProgressMsg, "completed") {
		t.Fatalf("progress messages were not persisted: %v", log.ProgressMsg)
	}
}
