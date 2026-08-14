package service

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"

	"gorm.io/gorm"
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

// mustAffectRows asserts a setup write succeeded and affected exactly want rows,
// so a broken test precondition reports its own cause instead of surfacing later
// as a reconciliation behavior failure.
func mustAffectRows(t *testing.T, res *gorm.DB, want int64) {
	t.Helper()
	if res.Error != nil || res.RowsAffected != want {
		t.Fatalf("setup write: err=%v rows=%d want=%d", res.Error, res.RowsAffected, want)
	}
}

func TestReconcile_RequeuesStaleCreated(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, _ = testutil.SeedTestData(t, db, testutil.WithTaskID("stale-created"))
	mustAffectRows(t, db.Model(&entity.IngestionTask{}).Where("id = ?", "stale-created").
		Updates(map[string]interface{}{
			"status":      common.CREATED,
			"update_date": time.Now().Add(-10 * time.Minute),
		}), 1)

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
	mustAffectRows(t, db.Model(&entity.IngestionTask{}).Where("id = ?", "no-progress").
		Updates(map[string]interface{}{"update_date": time.Now().Add(-6 * time.Minute)}), 1)

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
	mustAffectRows(t, db.Model(&entity.IngestionTaskLog{}).Where("task_id = ?", "hung-task").
		Update("create_date", time.Now().Add(-20*time.Minute)), 1)
	mustAffectRows(t, db.Model(&entity.IngestionTask{}).Where("id = ?", "hung-task").
		Updates(map[string]interface{}{"update_date": time.Now().Add(-20 * time.Minute)}), 1)

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
	mustAffectRows(t, db.Model(&entity.IngestionTask{}).Where("id = ?", "healthy-created").
		Update("status", common.CREATED), 1)

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

func TestReconcile_SkipsHeartbeatedTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// Pipeline started then went quiet → a hung candidate by progress rows, but
	// the worker's durable heartbeat keeps update_date fresh (e.g. a long single
	// component writing no progress rows for minutes), so it must not be failed.
	_, _, _, _ = testutil.SeedTestData(t, db,
		testutil.WithTaskID("heartbeated-task"), testutil.WithDocID("hb-doc"), testutil.WithKBID("hb-kb"), testutil.WithTenantID("hb-tenant"))
	if err := db.Create(&entity.IngestionTaskLog{
		TaskID: "heartbeated-task", Checkpoint: entity.JSONMap{}, Phase: 0, Component: "File", Message: "",
	}).Error; err != nil {
		t.Fatalf("create log row: %v", err)
	}
	mustAffectRows(t, db.Model(&entity.IngestionTaskLog{}).Where("task_id = ?", "heartbeated-task").
		Update("create_date", time.Now().Add(-20*time.Minute)), 1)
	// Fresh durable watermark: the executing worker's heartbeat Touch keeps the
	// task row's update_date current even though no progress row was written.
	mustAffectRows(t, db.Model(&entity.IngestionTask{}).Where("id = ?", "heartbeated-task").
		Update("update_date", time.Now()), 1)

	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.ingestionTaskSvc.SetTaskPublisher(&recordingPublisher{})

	ing.reconcileOnce()

	var task entity.IngestionTask
	if err := db.First(&task, "id = ?", "heartbeated-task").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("heartbeated task was failed: status = %s, want RUNNING", task.Status)
	}
}

func TestReconcile_DoesNotTouchCompletedDoc(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, _ := testutil.SeedTestData(t, db,
		testutil.WithTaskID("done-task"), testutil.WithDocID("done-doc"), testutil.WithKBID("done-kb"))
	mustAffectRows(t, db.Model(&entity.IngestionTask{}).Where("id = ?", "done-task").
		Update("status", common.COMPLETED), 1)
	mustAffectRows(t, db.Model(&entity.Document{}).Where("id = ?", docID).
		Updates(map[string]interface{}{"run": string(entity.TaskStatusDone), "progress": float64(1)}), 1)

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

// TestReconcile_ResetsStaleRunningDocWithoutTask: a document left RUNNING with
// no ingestion_task row and a stale update_date (crashed/aborted flow) is reset
// to unstart with progress cleared so it does not stay wedged in "parsing".
func TestReconcile_ResetsStaleRunningDocWithoutTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	running := string(entity.TaskStatusRunning)
	name := "orphan.pdf"
	loc := "doc_store/orphan-doc"
	doc := &entity.Document{
		ID: "orphan-doc", KbID: "kb-1", ParserID: "naive", ParserConfig: entity.JSONMap{},
		CreatedBy: "u1", Name: &name, Location: &loc, Status: testutil.StrPtr("1"),
		Type: "pdf", Suffix: "pdf", Run: &running, Progress: 0.5,
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	mustAffectRows(t, db.Model(&entity.Document{}).Where("id = ?", "orphan-doc").
		Update("update_date", time.Now().Add(-10*time.Minute)), 1)

	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.reconcileOnce()

	var got entity.Document
	if err := db.First(&got, "id = ?", "orphan-doc").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if got.Run == nil || *got.Run != string(entity.TaskStatusUnstart) {
		t.Fatalf("doc run = %v, want UNSTART", got.Run)
	}
	if got.Progress != 0 {
		t.Fatalf("doc progress = %v, want 0", got.Progress)
	}
}

// TestReconcile_KeepsFreshRunningDocWithoutTask: a RUNNING document with no task
// row but a fresh update_date (run flipped just before its task row lands) must
// NOT be reset — the staleness bound protects the enqueue window.
func TestReconcile_KeepsFreshRunningDocWithoutTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	running := string(entity.TaskStatusRunning)
	name := "fresh.pdf"
	loc := "doc_store/fresh-doc"
	doc := &entity.Document{
		ID: "fresh-doc", KbID: "kb-1", ParserID: "naive", ParserConfig: entity.JSONMap{},
		CreatedBy: "u1", Name: &name, Location: &loc, Status: testutil.StrPtr("1"),
		Type: "pdf", Suffix: "pdf", Run: &running, Progress: 0.25,
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	ing := NewIngestor("reconcile-test", 1, []string{"pdf"})
	ing.reconcileOnce()

	var got entity.Document
	if err := db.First(&got, "id = ?", "fresh-doc").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if got.Run == nil || *got.Run != string(entity.TaskStatusRunning) {
		t.Fatalf("fresh doc run = %v, want RUNNING (not reset)", got.Run)
	}
}
