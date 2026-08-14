package service

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
)

// recordingPublisher captures published task messages so reconcile tests can
// assert re-enqueueing without a live NATS engine.
type recordingPublisher struct {
	messages []common.TaskMessage
}

func (p *recordingPublisher) PublishTaskMessage(subject string, msg common.TaskMessage) error {
	p.messages = append(p.messages, msg)
	return nil
}

// Compile-time check that recordingPublisher satisfies the service contract.
var _ servicepkg.TaskPublisher = (*recordingPublisher)(nil)

func TestReconcile_RequeuesStaleCreated(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, _ = testutil.SeedTestData(t, db, testutil.WithTaskID("stale-created"))
	db.Model(&entity.IngestionTask{}).Where("id = ?", "stale-created").
		Updates(map[string]interface{}{
			"status":      common.CREATED,
			"update_date": time.Now().Add(-10 * time.Minute),
		})

	pub := &recordingPublisher{}
	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.ingestionTaskSvc.SetTaskPublisher(pub)

	ing.reconcileOnce()

	if len(pub.messages) != 1 || pub.messages[0].TaskID != "stale-created" {
		t.Fatalf("expected 1 requeue for stale-created, got %+v", pub.messages)
	}
	// Requeue does not transition the status — the task stays CREATED.
	var task entity.IngestionTask
	if err := db.First(&task, "id = ?", "stale-created").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.CREATED {
		t.Fatalf("task status = %s, want CREATED", task.Status)
	}
	// Touch advanced the watermark so the next pass skips it.
	if task.UpdateDate == nil || time.Since(*task.UpdateDate) > time.Minute {
		t.Fatalf("expected update_date to be advanced by Touch, got %v", task.UpdateDate)
	}
}

func TestReconcile_RequeuesRunningWithoutProgress(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// SeedTestData seeds RUNNING. Use an update_date in the [5,10)min window so
	// only the "no progress" query matches (the task has no log rows); the hung
	// query requires EXISTS(log) and must not fire.
	_, _, _, _ = testutil.SeedTestData(t, db, testutil.WithTaskID("no-progress"))
	db.Model(&entity.IngestionTask{}).Where("id = ?", "no-progress").
		Updates(map[string]interface{}{"update_date": time.Now().Add(-6 * time.Minute)})

	pub := &recordingPublisher{}
	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.ingestionTaskSvc.SetTaskPublisher(pub)

	ing.reconcileOnce()

	if len(pub.messages) != 1 || pub.messages[0].TaskID != "no-progress" {
		t.Fatalf("expected 1 requeue for no-progress, got %+v", pub.messages)
	}
	// Must not have been failed by the hung query.
	var task entity.IngestionTask
	if err := db.First(&task, "id = ?", "no-progress").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("task status = %s, want RUNNING (not failed)", task.Status)
	}
}

func TestReconcile_FailsHungRunning(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, _ := testutil.SeedTestData(t, db,
		testutil.WithTaskID("hung-task"), testutil.WithDocID("hung-doc"), testutil.WithKBID("hung-kb"))
	// A stale progress row proves the pipeline started, then went silent.
	if err := db.Create(&entity.IngestionTaskLog{
		TaskID: "hung-task", Checkpoint: entity.JSONMap{}, Phase: 0, Component: "File", Message: "",
	}).Error; err != nil {
		t.Fatalf("create log row: %v", err)
	}
	db.Model(&entity.IngestionTaskLog{}).Where("task_id = ?", "hung-task").
		Update("create_date", time.Now().Add(-20*time.Minute))
	db.Model(&entity.IngestionTask{}).Where("id = ?", "hung-task").
		Updates(map[string]interface{}{"update_date": time.Now().Add(-20 * time.Minute)})

	pub := &recordingPublisher{}
	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.ingestionTaskSvc.SetTaskPublisher(pub)

	ing.reconcileOnce()

	// A hung task is failed, not requeued.
	if len(pub.messages) != 0 {
		t.Fatalf("expected 0 requeues for hung task, got %+v", pub.messages)
	}
	var task entity.IngestionTask
	if err := db.First(&task, "id = ?", "hung-task").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED", task.Status)
	}
	var doc entity.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusFail) {
		t.Fatalf("doc run = %v, want FAIL", doc.Run)
	}
}

func TestReconcile_SkipsHealthyTasks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// Fresh CREATED task.
	_, _, _, _ = testutil.SeedTestData(t, db,
		testutil.WithTaskID("healthy-created"), testutil.WithDocID("hc-doc"), testutil.WithKBID("hc-kb"), testutil.WithTenantID("hc-tenant"))
	db.Model(&entity.IngestionTask{}).Where("id = ?", "healthy-created").
		Update("status", common.CREATED)

	// Fresh RUNNING task with a recent progress row.
	_, _, _, _ = testutil.SeedTestData(t, db,
		testutil.WithTaskID("healthy-running"), testutil.WithDocID("hr-doc"), testutil.WithKBID("hr-kb"), testutil.WithTenantID("hr-tenant"))
	if err := db.Create(&entity.IngestionTaskLog{
		TaskID: "healthy-running", Checkpoint: entity.JSONMap{}, Phase: 0, Component: "File", Message: "",
	}).Error; err != nil {
		t.Fatalf("create log row: %v", err)
	}

	pub := &recordingPublisher{}
	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.ingestionTaskSvc.SetTaskPublisher(pub)

	ing.reconcileOnce()

	if len(pub.messages) != 0 {
		t.Fatalf("expected 0 requeues for healthy tasks, got %+v", pub.messages)
	}
	var c, r entity.IngestionTask
	if err := db.First(&c, "id = ?", "healthy-created").Error; err != nil {
		t.Fatalf("load created: %v", err)
	}
	if err := db.First(&r, "id = ?", "healthy-running").Error; err != nil {
		t.Fatalf("load running: %v", err)
	}
	if c.Status != common.CREATED || r.Status != common.RUNNING {
		t.Fatalf("healthy tasks changed: created=%s running=%s", c.Status, r.Status)
	}
}

func TestReconcile_SkipsClaimedTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// Pipeline started then went quiet → a hung candidate, but an in-process
	// worker still owns the claim, so it must not be failed.
	_, _, _, _ = testutil.SeedTestData(t, db,
		testutil.WithTaskID("claimed-task"), testutil.WithDocID("cl-doc"), testutil.WithKBID("cl-kb"), testutil.WithTenantID("cl-tenant"))
	if err := db.Create(&entity.IngestionTaskLog{
		TaskID: "claimed-task", Checkpoint: entity.JSONMap{}, Phase: 0, Component: "File", Message: "",
	}).Error; err != nil {
		t.Fatalf("create log row: %v", err)
	}
	db.Model(&entity.IngestionTaskLog{}).Where("task_id = ?", "claimed-task").
		Update("create_date", time.Now().Add(-20*time.Minute))
	db.Model(&entity.IngestionTask{}).Where("id = ?", "claimed-task").
		Updates(map[string]interface{}{"update_date": time.Now().Add(-20 * time.Minute)})

	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.ingestionTaskSvc.SetTaskPublisher(&recordingPublisher{})
	ing.claimTask("claimed-task") // in-process worker owns it
	defer ing.releaseTask("claimed-task")

	ing.reconcileOnce()

	var task entity.IngestionTask
	if err := db.First(&task, "id = ?", "claimed-task").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("claimed task was failed: status = %s, want RUNNING", task.Status)
	}
}

func TestReconcile_DoesNotTouchCompletedDoc(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, _ := testutil.SeedTestData(t, db,
		testutil.WithTaskID("done-task"), testutil.WithDocID("done-doc"), testutil.WithKBID("done-kb"))
	db.Model(&entity.IngestionTask{}).Where("id = ?", "done-task").
		Update("status", common.COMPLETED)
	db.Model(&entity.Document{}).Where("id = ?", docID).
		Updates(map[string]interface{}{"run": string(entity.TaskStatusDone), "progress": float64(1)})

	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	// failHungTask on a non-RUNNING task must early-return and leave the
	// document untouched (MarkFailed is idempotent on terminal states).
	ing.failHungTask(context.Background(), "done-task")

	var doc entity.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusDone) {
		t.Fatalf("doc run = %v, want DONE (not overwritten)", doc.Run)
	}
}
