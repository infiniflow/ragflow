//go:build integration

package dao

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"ragflow/internal/entity"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestDocumentCleanupClaimMySQLTakeoverAndFencing exercises the row-lock and
// fencing-token contract against two independent MySQL connections. Set
// RAGFLOW_TEST_MYSQL_DSN to a dedicated integration database before running
// the integration tier; the test never selects a database implicitly.
func TestDocumentCleanupClaimMySQLTakeoverAndFencing(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN is not set")
	}

	dbA := openMySQLClaimTestDB(t, dsn)
	dbB := openMySQLClaimTestDB(t, dsn)
	if err := dbA.AutoMigrate(&entity.DocumentCleanupClaim{}); err != nil {
		t.Fatalf("migrate cleanup claim table: %v", err)
	}

	documentID := fmt.Sprintf("c%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = dbA.Where("document_id = ?", documentID).Delete(&entity.DocumentCleanupClaim{}).Error
	})
	claims := NewDocumentCleanupClaimDAO()

	first, err := claims.Acquire(t.Context(), dbA, documentID, "old-owner", 100, 120, 45)
	if err != nil {
		t.Fatalf("initial acquire: %v", err)
	}
	if first.ExpiresAt != 220 {
		t.Fatalf("initial expiry = %d, want 220", first.ExpiresAt)
	}
	if _, err = claims.Acquire(t.Context(), dbB, documentID, "grace-owner", 264, 120, 45); !errors.Is(err, ErrDocumentCleanupClaimHeld) {
		t.Fatalf("acquire during takeover grace error = %v, want ErrDocumentCleanupClaimHeld", err)
	}
	second, err := claims.Acquire(t.Context(), dbB, documentID, "new-owner", 266, 120, 45)
	if err != nil {
		t.Fatalf("takeover after grace: %v", err)
	}
	if second.Token == first.Token || second.Owner != "new-owner" {
		t.Fatalf("replacement claim = %+v, want a new new-owner token", second)
	}
	if err = claims.Renew(t.Context(), dbA, documentID, first.Token, 267, 120); !errors.Is(err, ErrDocumentCleanupClaimLost) {
		t.Fatalf("renew with fenced token error = %v, want ErrDocumentCleanupClaimLost", err)
	}

	if released, err := claims.Release(t.Context(), dbB, documentID, second.Token); err != nil || !released {
		t.Fatalf("release replacement claim = %t, %v; want true, nil", released, err)
	}

	if err = dbA.Create(&entity.DocumentCleanupClaim{
		DocumentID: documentID,
		Token:      "expired-token",
		Owner:      "crashed-owner",
		ExpiresAt:  100,
		UpdateTime: 1,
	}).Error; err != nil {
		t.Fatalf("seed expired claim: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, db := range []*gorm.DB{dbA, dbB} {
		wg.Add(1)
		go func(db *gorm.DB) {
			defer wg.Done()
			<-start
			results <- db.Transaction(func(tx *gorm.DB) error {
				_, err := claims.Acquire(t.Context(), tx, documentID, "racing-owner", 200, 120, 45)
				return err
			})
		}(db)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	held := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrDocumentCleanupClaimHeld):
			held++
		default:
			t.Fatalf("concurrent MySQL takeover error = %v", err)
		}
	}
	if successes != 1 || held != 1 {
		t.Fatalf("concurrent MySQL takeover outcomes: successes=%d held=%d, want one each", successes, held)
	}

	var stored entity.DocumentCleanupClaim
	if err = dbA.First(&stored, "document_id = ?", documentID).Error; err != nil {
		t.Fatalf("load winning claim: %v", err)
	}
	if stored.Owner != "racing-owner" || stored.Token == "expired-token" {
		t.Fatalf("stored winner = %+v, want racing-owner with a replacement token", stored)
	}
}

func openMySQLClaimTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL test DB: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL test DB handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
