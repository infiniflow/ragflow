package knowledge_compile

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newSQLTestScheduler(t *testing.T) *mysqlScheduler {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	scheduler := newScheduler(db, nil, "worker", time.Minute)
	if err := scheduler.Provision(t.Context()); err != nil {
		t.Fatal(err)
	}
	return scheduler
}

func assertSQLQueueCounts(t *testing.T, s *mysqlScheduler, datasetID string, backlog, inflight int64) {
	t.Helper()
	for _, queue := range []struct {
		model interface{}
		want  int64
	}{
		{&entity.KnowledgeCompileBacklog{}, backlog},
		{&entity.KnowledgeCompileInflight{}, inflight},
	} {
		var count int64
		if err := s.db.Model(queue.model).Where("dataset_id = ?", datasetID).Count(&count).Error; err != nil || count != queue.want {
			t.Fatalf("%T count=%d want=%d err=%v", queue.model, count, queue.want, err)
		}
	}
}

func TestSQLSchedulerPreservesDuplicatesAndClosedBatches(t *testing.T) {
	s := newSQLTestScheduler(t)
	s.claimBatchSize = 2
	entries := []BacklogEntry{
		{DocID: "same", EventType: "doc_completed", Variants: []string{"wiki"}, TaskTypes: []string{"Wiki"}},
		{DocID: "same", EventType: "doc_completed", Variants: []string{"wiki"}, TaskTypes: []string{"Wiki"}},
		{DocID: "same", EventType: "doc_deleted", Variants: []string{"wiki"}, TaskTypes: []string{"Wiki"}},
	}
	for _, entry := range entries {
		if err := s.Publish(t.Context(), "tenant", "kb", entry.DocID, entry.EventType, entry.Variants, entry.TaskTypes); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Publish(t.Context(), "other-tenant", "other-kb", "same", "doc_completed", nil, nil); err != nil {
		t.Fatal(err)
	}
	first, ok, err := s.Claim(t.Context(), "kb")
	if err != nil || !ok || !reflect.DeepEqual(first.Entries, entries[:2]) {
		t.Fatalf("claim=%+v ok=%v err=%v", first, ok, err)
	}
	assertSQLQueueCounts(t, s, "kb", 1, 2)
	if _, acquired, err := s.Claim(t.Context(), "kb"); err != nil || acquired {
		t.Fatalf("live dataset was claimed twice: acquired=%v err=%v", acquired, err)
	}
	if _, err := s.Ack(t.Context(), "kb", "stale-token", first.Entries); !errors.Is(err, ErrClaimTokenMismatch) {
		t.Fatalf("stale ack error=%v", err)
	}
	assertSQLQueueCounts(t, s, "kb", 1, 2)
	// A new completion for the same document must survive acknowledgment of
	// the earlier completions, even when all event fields are identical.
	if err := s.Publish(t.Context(), "tenant", "kb", "same", "doc_completed", []string{"wiki"}, []string{"Wiki"}); err != nil {
		t.Fatal(err)
	}
	if remaining, err := s.Ack(t.Context(), "kb", first.Token, first.Entries); err != nil || remaining != 2 {
		t.Fatalf("ack remaining=%d err=%v", remaining, err)
	}
	assertSQLQueueCounts(t, s, "kb", 2, 0)
	second, ok, err := s.Claim(t.Context(), "kb")
	want := []BacklogEntry{entries[2], entries[0]}
	if err != nil || !ok || !reflect.DeepEqual(second.Entries, want) {
		t.Fatalf("second claim=%+v want=%+v ok=%v err=%v", second, want, ok, err)
	}
	if _, err := s.Ack(t.Context(), "kb", second.Token, second.Entries); err != nil {
		t.Fatal(err)
	}
	assertSQLQueueCounts(t, s, "kb", 0, 0)
	assertSQLQueueCounts(t, s, "other-kb", 1, 0)
	row, err := s.loadRow(t.Context(), "kb")
	if err != nil || row.State != DatasetStateCompleted || row.LastCompletedAt == nil || row.ClaimToken != "" {
		t.Fatalf("completed row=%+v err=%v", row, err)
	}
}

func TestSQLSchedulerCancellationAndExpiredLeaseRecovery(t *testing.T) {
	s := newSQLTestScheduler(t)
	if err := s.Publish(t.Context(), "tenant", "kb", "first", "doc_completed", []string{"structure"}, []string{"Graph"}); err != nil {
		t.Fatal(err)
	}
	first, ok, err := s.TryClaim(t.Context())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := s.Publish(t.Context(), "tenant", "kb", "new", "doc_deleted", []string{"wiki"}, []string{"Wiki"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelInflight(t.Context(), "kb", first.Token); err != nil {
		t.Fatal(err)
	}
	assertSQLQueueCounts(t, s, "kb", 2, 0)
	second, ok, err := s.TryClaim(t.Context())
	if err != nil || !ok || len(second.Entries) != 2 || second.Entries[0].DocID != "new" || second.Entries[1].DocID != "first" {
		t.Fatalf("cancel must append inflight after backlog: claim=%+v ok=%v err=%v", second, ok, err)
	}
	if err := s.db.Model(&entity.KnowledgeCompileDataset{}).Where("dataset_id = ?", "kb").
		Update("claim_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	third, ok, err := s.TryClaim(t.Context())
	if err != nil || !ok || third.Token == second.Token || !reflect.DeepEqual(third.Entries, second.Entries) {
		t.Fatalf("reclaim=%+v previous=%+v ok=%v err=%v", third, second, ok, err)
	}
	if _, err := s.Ack(t.Context(), "kb", second.Token, second.Entries); !errors.Is(err, ErrClaimTokenMismatch) {
		t.Fatalf("expired worker ack error=%v", err)
	}
	if _, err := s.Ack(t.Context(), "kb", third.Token, third.Entries); err != nil {
		t.Fatal(err)
	}
	assertSQLQueueCounts(t, s, "kb", 0, 0)
}

func TestSQLSchedulerClaimRollsBackQueueMoveOnFailure(t *testing.T) {
	s := newSQLTestScheduler(t)
	if err := s.Publish(t.Context(), "tenant", "kb", "doc", "doc_completed", nil, nil); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("queue delete failed")
	if err := s.db.Callback().Delete().Before("gorm:delete").Register("fail_backlog_delete", func(tx *gorm.DB) {
		if tx.Statement.Table == (entity.KnowledgeCompileBacklog{}).TableName() {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Claim(t.Context(), "kb"); !errors.Is(err, failure) {
		t.Fatalf("claim error=%v want=%v", err, failure)
	}
	assertSQLQueueCounts(t, s, "kb", 1, 0)
	row, err := s.loadRow(t.Context(), "kb")
	if err != nil || row.ClaimToken != "" || row.State != DatasetStatePending {
		t.Fatalf("failed claim changed lease: row=%+v err=%v", row, err)
	}
}
