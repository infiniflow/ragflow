//go:build integration

package knowledge_compile

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"ragflow/internal/entity"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMySQLSchedulerQueues_Integration(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: requires an empty MySQL test database")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	models := []interface{}{&entity.KnowledgeCompileDataset{}, &entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}}
	for _, model := range models {
		if db.Migrator().HasTable(model) {
			t.Fatalf("%T already exists; this test requires an empty database", model)
		}
	}
	t.Cleanup(func() {
		for _, model := range models {
			if err := db.Migrator().DropTable(model); err != nil {
				t.Errorf("drop %T: %v", model, err)
			}
		}
	})
	s := newScheduler(db, nil, "worker1", time.Minute)
	if err := s.Provision(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(t.Context(), "tenant", "kb", "first", "doc_completed", []string{"wiki"}, []string{"Wiki"}); err != nil {
		t.Fatal(err)
	}
	var sqlLog bytes.Buffer
	dryRun := db.Session(&gorm.Session{
		DryRun: true,
		Logger: logger.New(log.New(&sqlLog, "", 0), logger.Config{LogLevel: logger.Info}),
	})
	if _, _, err := s.findClaimableID(t.Context(), dryRun, time.Now()); err != nil {
		t.Fatal(err)
	}
	t.Logf("MySQL candidate query: %s", sqlLog.String())

	const publishers = 4
	const eventsPerPublisher = 16
	errorsCh := make(chan error, publishers)
	var writers sync.WaitGroup
	for worker := 0; worker < publishers; worker++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for event := 0; event < eventsPerPublisher; event++ {
				if err := s.Publish(t.Context(), "tenant", "kb", fmt.Sprintf("doc-%d-%d", worker, event), "doc_completed", []string{"wiki"}, []string{"Wiki"}); err != nil {
					errorsCh <- err
					return
				}
			}
		}()
	}
	writers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatal(err)
	}
	assertSQLQueueCounts(t, s, "kb", 65, 0)
	var entries []entity.KnowledgeCompileEntry
	if err := db.Table((entity.KnowledgeCompileBacklog{}).TableName()).Order("queue_position").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	for i, entry := range entries {
		if entry.QueuePosition != int64(i+1) {
			t.Fatalf("concurrent publishers reused queue positions: %+v", entries)
		}
	}

	type claimOutcome struct {
		claim ClaimResult
		ok    bool
		err   error
	}
	start := make(chan struct{})
	claimed := make(chan claimOutcome, 2)
	for _, scheduler := range []*mysqlScheduler{s, newScheduler(db, nil, "worker2", time.Minute)} {
		go func() {
			<-start
			claim, ok, err := scheduler.TryClaim(t.Context())
			claimed <- claimOutcome{claim, ok, err}
		}()
	}
	close(start)
	var claim ClaimResult
	acquired := 0
	for range 2 {
		outcome := <-claimed
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.ok {
			acquired++
			claim = outcome.claim
		}
	}
	if acquired != 1 || len(claim.Entries) != defaultClaimBatch {
		t.Fatalf("concurrent claims: acquired=%d claim=%+v", acquired, claim)
	}
	assertSQLQueueCounts(t, s, "kb", 33, 32)
	if remaining, err := s.Ack(t.Context(), "kb", claim.Token, claim.Entries); err != nil || remaining != 33 {
		t.Fatalf("ack remaining=%d want=33 err=%v", remaining, err)
	}
	second, ok, err := s.TryClaim(t.Context())
	if err != nil || !ok || len(second.Entries) != defaultClaimBatch {
		t.Fatalf("second claim=%+v ok=%v err=%v", second, ok, err)
	}
	if remaining, err := s.Ack(t.Context(), "kb", second.Token, second.Entries); err != nil || remaining != 1 {
		t.Fatalf("second ack remaining=%d want=1 err=%v", remaining, err)
	}
	lastClaim, ok, err := s.TryClaim(t.Context())
	if err != nil || !ok || len(lastClaim.Entries) != 1 {
		t.Fatalf("last claim=%+v ok=%v err=%v", lastClaim, ok, err)
	}
	if err := db.Model(&entity.KnowledgeCompileDataset{}).Where("dataset_id = ?", "kb").
		Update("claim_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	recovered, ok, err := s.TryClaim(t.Context())
	if err != nil || !ok || recovered.Token == lastClaim.Token || !reflect.DeepEqual(recovered.Entries, lastClaim.Entries) {
		t.Fatalf("recovered=%+v previous=%+v ok=%v err=%v", recovered, lastClaim, ok, err)
	}
	if _, err := s.Ack(t.Context(), "kb", lastClaim.Token, lastClaim.Entries); !errors.Is(err, ErrClaimTokenMismatch) {
		t.Fatalf("stale acknowledgment error=%v", err)
	}
	if _, err := s.Ack(t.Context(), "kb", recovered.Token, recovered.Entries); err != nil {
		t.Fatal(err)
	}
	assertSQLQueueCounts(t, s, "kb", 0, 0)
}
