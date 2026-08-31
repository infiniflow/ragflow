package dao

import (
	"errors"
	"testing"
	"time"

	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestIngestionTaskDAOUpdateStatusIfCurrentSucceeds(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	task := &entity.IngestionTask{
		ID:         "task-1",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	ctx := t.Context()
	updated, err := NewIngestionTaskDAO().UpdateStatusIfCurrent(ctx, db, "task-1", common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent failed: %v", err)
	}
	if !updated {
		t.Fatal("expected update to succeed")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.RUNNING)
	}
}

func TestIngestionTaskDAOCreateRejectsExistingTerminalTask(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	testCases := []struct {
		name   string
		status string
	}{
		{name: "failed", status: common.FAILED},
		{name: "stopped", status: common.STOPPED},
	}

	ctx := t.Context()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.WithContext(ctx).Where("id = ?", "task-1").Delete(&entity.IngestionTask{}).Error; err != nil {
				t.Fatalf("clear task: %v", err)
			}
			task := &entity.IngestionTask{ID: "task-1", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: tc.status}
			if err := db.WithContext(ctx).Create(task).Error; err != nil {
				t.Fatalf("create task: %v", err)
			}
			_, err := NewIngestionTaskDAO().Create(ctx, db, &entity.IngestionTask{ID: "task-2", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED})
			if err == nil {
				t.Fatal("expected Create to reject duplicate document task")
			}
			reloaded, err := NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
			if err != nil {
				t.Fatalf("reload task: %v", err)
			}
			if reloaded.Status != tc.status {
				t.Fatalf("status = %q, want %q", reloaded.Status, tc.status)
			}
		})
	}
}

func TestIngestionTaskDAODocumentIDIsUniqueAtDBLevel(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	first := &entity.IngestionTask{ID: "task-1", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("create first task: %v", err)
	}

	second := &entity.IngestionTask{ID: "task-2", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED}
	err := db.Create(second).Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("expected duplicated key error, got %v", err)
	}
}

func TestIngestionTaskDAOUpdateStatusIfCurrentRejectsMismatchedStatus(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	task := &entity.IngestionTask{
		ID:         "task-1",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     common.STOPPING,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	ctx := t.Context()
	updated, err := NewIngestionTaskDAO().UpdateStatusIfCurrent(ctx, db, "task-1", common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent failed: %v", err)
	}
	if updated {
		t.Fatal("expected update to be rejected")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.STOPPING {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.STOPPING)
	}
}

func TestIngestionTaskDAOTryClaimAllowsOnlyOneOwner(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:         "task-1",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     common.SCHEDULED,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	now := time.Unix(1_700_000_000, 0)
	claimed, err := NewIngestionTaskDAO().TryClaim(t.Context(), db, task.ID, "owner-a", now, 15*time.Second)
	if err != nil {
		t.Fatalf("first TryClaim: %v", err)
	}
	if !claimed {
		t.Fatal("first owner should claim the scheduled task")
	}

	claimed, err = NewIngestionTaskDAO().TryClaim(t.Context(), db, task.ID, "owner-b", now, 15*time.Second)
	if err != nil {
		t.Fatalf("second TryClaim: %v", err)
	}
	if claimed {
		t.Fatal("second owner must not claim an already running task")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.RUNNING)
	}
	if reloaded.ClaimToken != "owner-a" {
		t.Fatalf("claim token = %q, want owner-a", reloaded.ClaimToken)
	}
	if reloaded.ClaimExpiresAt != now.Add(15*time.Second).UnixMilli() {
		t.Fatalf("claim expiry = %d, want %d", reloaded.ClaimExpiresAt, now.Add(15*time.Second).UnixMilli())
	}
}

func TestIngestionTaskDAOTouchClaimRejectsExpiredLease(t *testing.T) {
	db := setupTaskTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	task := &entity.IngestionTask{
		ID:             "task-1",
		UserID:         "user-1",
		DocumentID:     "doc-1",
		DatasetID:      "kb-1",
		Status:         common.RUNNING,
		ClaimToken:     "owner-a",
		ClaimExpiresAt: now.Add(-time.Millisecond).UnixMilli(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	alive, err := NewIngestionTaskDAO().TouchClaim(t.Context(), db, task.ID, "owner-a", now, 15*time.Second)
	if err != nil {
		t.Fatalf("TouchClaim: %v", err)
	}
	if alive {
		t.Fatal("expired owner must not renew its lease")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.ClaimExpiresAt != task.ClaimExpiresAt {
		t.Fatalf("claim expiry = %d, want unchanged %d", reloaded.ClaimExpiresAt, task.ClaimExpiresAt)
	}
}

func TestIngestionTaskDAOListExpiredClaimsRequiresAnActiveLease(t *testing.T) {
	db := setupTaskTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	for _, task := range []*entity.IngestionTask{
		{ID: "task-unleased", UserID: "user-1", DocumentID: "doc-unleased", DatasetID: "kb-1", Status: common.RUNNING},
		{ID: "task-expired", UserID: "user-1", DocumentID: "doc-expired", DatasetID: "kb-1", Status: common.RUNNING, ClaimToken: "owner-a", ClaimExpiresAt: now.Add(-time.Second).UnixMilli()},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	tasks, err := NewIngestionTaskDAO().ListExpiredClaims(t.Context(), db, []string{common.RUNNING}, now, "", 0)
	if err != nil {
		t.Fatalf("ListExpiredClaims: %v", err)
	}
	if got := idsOfTasks(tasks); fmt.Sprint(got) != "[task-expired]" {
		t.Fatalf("expired tasks = %v, want [task-expired]", got)
	}
}

func TestIngestionTaskDAORecoverExpiredClaimIgnoresUnleasedTask(t *testing.T) {
	db := setupTaskTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	task := &entity.IngestionTask{
		ID:         "task-unleased",
		UserID:     "user-1",
		DocumentID: "doc-unleased",
		DatasetID:  "kb-1",
		Status:     common.RUNNING,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	requeued, poisoned, err := NewIngestionTaskDAO().RecoverExpiredClaim(t.Context(), db, task.ID, now, 3)
	if err != nil {
		t.Fatalf("RecoverExpiredClaim: %v", err)
	}
	if requeued || poisoned {
		t.Fatalf("unleased recovery = requeued:%v poisoned:%v, want false:false", requeued, poisoned)
	}
	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.RUNNING {
		t.Fatalf("unleased task status = %q, want %q", reloaded.Status, common.RUNNING)
	}
}

func TestIngestionTaskDAOFinalizeClaimRejectsExpiredOwner(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:             "task-expired",
		UserID:         "user-1",
		DocumentID:     "doc-expired",
		DatasetID:      "kb-1",
		Status:         common.RUNNING,
		ClaimToken:     "owner-a",
		ClaimExpiresAt: time.Now().Add(-time.Second).UnixMilli(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	finalized, err := NewIngestionTaskDAO().FinalizeClaim(t.Context(), db, task.ID, task.ClaimToken, common.RUNNING, common.COMPLETED)
	if err != nil {
		t.Fatalf("FinalizeClaim: %v", err)
	}
	if finalized {
		t.Fatal("expired owner must not finalize the task")
	}
	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.RUNNING {
		t.Fatalf("expired task status = %q, want %q", reloaded.Status, common.RUNNING)
	}
}

func TestIngestionTaskDAOReleaseExpiredClaimClearsOwner(t *testing.T) {
	db := setupTaskTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	task := &entity.IngestionTask{
		ID:             "task-1",
		UserID:         "user-1",
		DocumentID:     "doc-1",
		DatasetID:      "kb-1",
		Status:         common.RUNNING,
		ClaimToken:     "owner-a",
		ClaimExpiresAt: now.Add(-time.Millisecond).UnixMilli(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	released, err := NewIngestionTaskDAO().ReleaseExpiredClaim(t.Context(), db, task.ID, now)
	if err != nil {
		t.Fatalf("ReleaseExpiredClaim: %v", err)
	}
	if !released {
		t.Fatal("expired claim should be released")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.SCHEDULED)
	}
	if reloaded.ClaimToken != "" || reloaded.ClaimExpiresAt != 0 {
		t.Fatalf("released claim fields = (%q, %d), want empty", reloaded.ClaimToken, reloaded.ClaimExpiresAt)
	}
}

// TestIngestionTaskDAORecoverExpiredClaimLimitsLeaseRecovery verifies that
// only expired RUNNING leases consume the recovery budget: the third recovery
// schedules another execution, while the next expired lease becomes FAILED.
func TestIngestionTaskDAORecoverExpiredClaimLimitsLeaseRecovery(t *testing.T) {
	db := setupTaskTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	task := &entity.IngestionTask{
		ID:                   "task-1",
		UserID:               "user-1",
		DocumentID:           "doc-1",
		DatasetID:            "kb-1",
		Status:               common.RUNNING,
		ClaimToken:           "owner-a",
		ClaimExpiresAt:       now.Add(-time.Second).UnixMilli(),
		LeaseRecoveryAttempt: 2,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	requeued, poisoned, err := NewIngestionTaskDAO().RecoverExpiredClaim(t.Context(), db, task.ID, now, 3)
	if err != nil {
		t.Fatalf("RecoverExpiredClaim: %v", err)
	}
	if !requeued || poisoned {
		t.Fatalf("first recovery = requeued:%v poisoned:%v, want true:false", requeued, poisoned)
	}

	var recovered entity.IngestionTask
	if err := db.Where("id = ?", task.ID).First(&recovered).Error; err != nil {
		t.Fatalf("reload requeued task: %v", err)
	}
	if recovered.Status != common.SCHEDULED || recovered.LeaseRecoveryAttempt != 3 || recovered.ClaimToken != "" {
		t.Fatalf("requeued task = status:%q attempt:%d token:%q", recovered.Status, recovered.LeaseRecoveryAttempt, recovered.ClaimToken)
	}

	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":           common.RUNNING,
		"claim_token":      "owner-b",
		"claim_expires_at": now.Add(-time.Second).UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("expire next claim: %v", err)
	}

	requeued, poisoned, err = NewIngestionTaskDAO().RecoverExpiredClaim(t.Context(), db, task.ID, now, 3)
	if err != nil {
		t.Fatalf("RecoverExpiredClaim after limit: %v", err)
	}
	if requeued || !poisoned {
		t.Fatalf("recovery after limit = requeued:%v poisoned:%v, want false:true", requeued, poisoned)
	}
	if err := db.Where("id = ?", task.ID).First(&recovered).Error; err != nil {
		t.Fatalf("reload poisoned task: %v", err)
	}
	if recovered.Status != common.FAILED || recovered.LeaseRecoveryAttempt != 3 || recovered.ClaimToken != "" {
		t.Fatalf("poisoned task = status:%q attempt:%d token:%q", recovered.Status, recovered.LeaseRecoveryAttempt, recovered.ClaimToken)
	}
}

func TestIngestionTaskDAOScheduleResetsClaimAndDispatchState(t *testing.T) {
	db := setupTaskTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	task := &entity.IngestionTask{
		ID:                   "task-1",
		UserID:               "user-1",
		DocumentID:           "doc-1",
		DatasetID:            "kb-1",
		Status:               common.FAILED,
		LastDispatchedAt:     now.Add(-time.Hour).UnixMilli(),
		ClaimToken:           "old-owner",
		ClaimExpiresAt:       now.Add(time.Hour).UnixMilli(),
		LeaseRecoveryAttempt: 3,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	scheduled, err := NewIngestionTaskDAO().Schedule(t.Context(), db, task.ID, common.FAILED, now)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if !scheduled {
		t.Fatal("failed task should become scheduled")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.SCHEDULED)
	}
	if reloaded.ScheduledAt != now.UnixMilli() || reloaded.LastDispatchedAt != 0 {
		t.Fatalf("schedule state = (%d, %d), want (%d, 0)", reloaded.ScheduledAt, reloaded.LastDispatchedAt, now.UnixMilli())
	}
	if reloaded.ClaimToken != "" || reloaded.ClaimExpiresAt != 0 {
		t.Fatalf("claim fields = (%q, %d), want empty", reloaded.ClaimToken, reloaded.ClaimExpiresAt)
	}
	if reloaded.LeaseRecoveryAttempt != 0 {
		t.Fatalf("lease recovery attempt = %d, want 0 after explicit retry", reloaded.LeaseRecoveryAttempt)
	}
}

func TestIngestionTaskDAOFinalizeClaimRejectsStaleOwner(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:             "task-1",
		UserID:         "user-1",
		DocumentID:     "doc-1",
		DatasetID:      "kb-1",
		Status:         common.RUNNING,
		ClaimToken:     "owner-b",
		ClaimExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	finalized, err := NewIngestionTaskDAO().FinalizeClaim(t.Context(), db, task.ID, "owner-a", common.RUNNING, common.COMPLETED)
	if err != nil {
		t.Fatalf("FinalizeClaim: %v", err)
	}
	if finalized {
		t.Fatal("stale owner must not finalize the task")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.RUNNING || reloaded.ClaimToken != "owner-b" {
		t.Fatalf("stale finalize changed task: %+v", reloaded)
	}
}

func TestIngestionTaskDAOFinalizeClaimCompletesCurrentOwner(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:             "task-1",
		UserID:         "user-1",
		DocumentID:     "doc-1",
		DatasetID:      "kb-1",
		Status:         common.RUNNING,
		ClaimToken:     "owner-a",
		ClaimExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	finalized, err := NewIngestionTaskDAO().FinalizeClaim(t.Context(), db, task.ID, "owner-a", common.RUNNING, common.COMPLETED)
	if err != nil {
		t.Fatalf("FinalizeClaim: %v", err)
	}
	if !finalized {
		t.Fatal("current owner should finalize the task")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.COMPLETED {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.COMPLETED)
	}
	if reloaded.ClaimToken != "" || reloaded.ClaimExpiresAt != 0 {
		t.Fatalf("completed claim fields = (%q, %d), want empty", reloaded.ClaimToken, reloaded.ClaimExpiresAt)
	}
}

func TestIngestionTaskDAOListScheduledForDispatchAdvancesByKeyset(t *testing.T) {
	db := setupTaskTestDB(t)
	cutoff := time.Unix(1_700_000_100, 0)
	for _, task := range []*entity.IngestionTask{
		{ID: "task-a", UserID: "user-1", DocumentID: "doc-a", DatasetID: "kb-1", Status: common.SCHEDULED, LastDispatchedAt: cutoff.Add(-2 * time.Minute).UnixMilli()},
		{ID: "task-b", UserID: "user-1", DocumentID: "doc-b", DatasetID: "kb-1", Status: common.SCHEDULED, LastDispatchedAt: cutoff.Add(-2 * time.Minute).UnixMilli()},
		{ID: "task-c", UserID: "user-1", DocumentID: "doc-c", DatasetID: "kb-1", Status: common.SCHEDULED, LastDispatchedAt: cutoff.Add(-time.Minute).UnixMilli()},
		{ID: "task-fresh", UserID: "user-1", DocumentID: "doc-fresh", DatasetID: "kb-1", Status: common.SCHEDULED, LastDispatchedAt: cutoff.UnixMilli()},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	d := NewIngestionTaskDAO()
	firstPage, err := d.ListScheduledForDispatch(t.Context(), db, cutoff, 0, "", 2)
	if err != nil {
		t.Fatalf("first ListScheduledForDispatch: %v", err)
	}
	if got := idsOfTasks(firstPage); fmt.Sprint(got) != "[task-a task-b]" {
		t.Fatalf("first page = %v, want [task-a task-b]", got)
	}

	last := firstPage[len(firstPage)-1]
	secondPage, err := d.ListScheduledForDispatch(t.Context(), db, cutoff, last.LastDispatchedAt, last.ID, 2)
	if err != nil {
		t.Fatalf("second ListScheduledForDispatch: %v", err)
	}
	if got := idsOfTasks(secondPage); fmt.Sprint(got) != "[task-c]" {
		t.Fatalf("second page = %v, want [task-c]", got)
	}
}

func TestIngestionTaskDAOTryReserveDispatchOnlyUpdatesExpectedScheduledTask(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:         "task-1",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     common.SCHEDULED,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	dispatchedAt := time.Unix(1_700_000_000, 0)
	updated, err := NewIngestionTaskDAO().TryReserveDispatch(t.Context(), db, task.ID, 0, dispatchedAt)
	if err != nil {
		t.Fatalf("TryReserveDispatch: %v", err)
	}
	if !updated {
		t.Fatal("scheduled task should record a successful dispatch")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.LastDispatchedAt != dispatchedAt.UnixMilli() {
		t.Fatalf("last dispatch = %d, want %d", reloaded.LastDispatchedAt, dispatchedAt.UnixMilli())
	}

	updated, err = NewIngestionTaskDAO().TryReserveDispatch(t.Context(), db, task.ID, 0, dispatchedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("TryReserveDispatch with stale expected timestamp: %v", err)
	}
	if updated {
		t.Fatal("stale dispatch reservation must not update the task")
	}
}

func TestIngestionTaskDAOReleaseClaimReturnsCurrentOwnerTaskToScheduled(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:             "task-1",
		UserID:         "user-1",
		DocumentID:     "doc-1",
		DatasetID:      "kb-1",
		Status:         common.RUNNING,
		ClaimToken:     "owner-a",
		ClaimExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	released, err := NewIngestionTaskDAO().ReleaseClaim(t.Context(), db, task.ID, "owner-a")
	if err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	if !released {
		t.Fatal("current owner should release its claim")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.SCHEDULED || reloaded.ClaimToken != "" || reloaded.ClaimExpiresAt != 0 {
		t.Fatalf("released task = %+v, want scheduled task with no claim", reloaded)
	}
}

// seedStaleTask inserts an ingestion task whose create/update timestamps are
// aged back so ListStaleByStatus staleness windows can be asserted.
func seedStaleTask(t *testing.T, db *gorm.DB, id, status string, age time.Duration) {
	t.Helper()
	ts := time.Now().Add(-age).UnixMilli()
	task := &entity.IngestionTask{
		ID:         id,
		UserID:     "user-1",
		DocumentID: "doc-" + id,
		DatasetID:  "kb-1",
		Status:     status,
		BaseModel:  entity.BaseModel{CreateTime: &ts, UpdateTime: &ts},
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task %s: %v", id, err)
	}
}

// TestIngestionTaskDAOListStaleByStatus covers the three contract axes of the
// startup-reconciliation query: status filtering, the staleness time window
// (per column), and the result limit.
func TestIngestionTaskDAOListStaleByStatus(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	seedStaleTask(t, db, "run-stale", common.RUNNING, 20*time.Minute)
	seedStaleTask(t, db, "run-fresh", common.RUNNING, 1*time.Minute)
	seedStaleTask(t, db, "created-stale", common.CREATED, 20*time.Minute)
	seedStaleTask(t, db, "created-fresh", common.CREATED, 1*time.Minute)
	seedStaleTask(t, db, "completed-stale", common.COMPLETED, 20*time.Minute)

	ctx := t.Context()
	d := NewIngestionTaskDAO()
	threshold := time.Now().Add(-15 * time.Minute)

	// Status filter + update_time window: only the stale RUNNING row matches.
	running, err := d.ListStaleByStatus(ctx, db, []string{common.RUNNING}, "update_time", threshold, 0)
	if err != nil {
		t.Fatalf("ListStaleByStatus(RUNNING): %v", err)
	}
	if len(running) != 1 || running[0].ID != "run-stale" {
		t.Fatalf("RUNNING stale rows = %v, want [run-stale]", idsOfTasks(running))
	}

	// CREATED is judged by create_time: fresh CREATED rows stay outside the
	// window even though their status is in the filter.
	created, err := d.ListStaleByStatus(ctx, db, []string{common.CREATED}, "create_time", threshold, 0)
	if err != nil {
		t.Fatalf("ListStaleByStatus(CREATED): %v", err)
	}
	if len(created) != 1 || created[0].ID != "created-stale" {
		t.Fatalf("CREATED stale rows = %v, want [created-stale]", idsOfTasks(created))
	}

	// Rows in unlisted statuses (COMPLETED) are never returned.
	all, err := d.ListStaleByStatus(ctx, db, []string{common.RUNNING, common.CREATED}, "create_time", threshold, 0)
	if err != nil {
		t.Fatalf("ListStaleByStatus(RUNNING+CREATED): %v", err)
	}
	for _, task := range all {
		if task.ID == "completed-stale" {
			t.Fatal("COMPLETED row must not be returned for RUNNING/CREATED filter")
		}
	}

	// Limit caps the (oldest-first) result set.
	limited, err := d.ListStaleByStatus(ctx, db, []string{common.RUNNING, common.CREATED}, "create_time", threshold, 1)
	if err != nil {
		t.Fatalf("ListStaleByStatus(limit): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited rows = %v, want exactly 1", idsOfTasks(limited))
	}

	// An unsupported staleness column is rejected, not silently ignored.
	if _, err = d.ListStaleByStatus(ctx, db, []string{common.RUNNING}, "status; --", threshold, 0); err == nil {
		t.Fatal("expected error for unsupported staleness column")
	}
}

func idsOfTasks(tasks []*entity.IngestionTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func TestIngestionTaskDAODeleteIfTerminal_RemovesOnlyTerminal(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	// Create tasks in different statuses, each with a unique docID.
	statuses := []string{common.CREATED, common.RUNNING, common.STOPPING, common.COMPLETED, common.STOPPED, common.FAILED}
	for i, status := range statuses {
		docID := fmt.Sprintf("doc-%d", i)
		task := &entity.IngestionTask{
			ID:         fmt.Sprintf("task-%d", i),
			UserID:     "user-1",
			DocumentID: docID,
			DatasetID:  "kb-1",
			Status:     status,
		}
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", status, err)
		}
	}

	ctx := t.Context()
	// DeleteIfTerminal deletes everything except RUNNING and STOPPING.
	// CREATED is safe to delete (no worker has claimed it yet);
	// COMPLETED/STOPPED/FAILED are terminal.
	// Call it for every doc and verify the negative cases survived.
	for i := 0; i < len(statuses); i++ {
		docID := fmt.Sprintf("doc-%d", i)
		_, err := NewIngestionTaskDAO().DeleteIfTerminal(ctx, db, docID)
		if err != nil {
			t.Fatalf("DeleteIfTerminal(doc-%d): %v", i, err)
		}
	}

	// RUNNING and STOPPING must survive.
	for _, i := range []int{1, 2} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task == nil {
			t.Fatalf("%s task (doc=%d) must not be deleted", statuses[i], i)
		}
	}
	// CREATED, COMPLETED, STOPPED, FAILED must be gone.
	for _, i := range []int{0, 3, 4, 5} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task != nil {
			t.Fatalf("%s task (doc=%d) should be deleted, still present", statuses[i], i)
		}
	}
}
