package dao

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDocumentCleanupClaimDAOFencesExpiredOwners(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.DocumentCleanupClaim{}); err != nil {
		t.Fatalf("migrate cleanup claim: %v", err)
	}

	claims := NewDocumentCleanupClaimDAO()
	first, err := claims.Acquire(t.Context(), db, "doc-1", "api-a", 100, 120, 45)
	if err != nil {
		t.Fatalf("acquire first claim: %v", err)
	}
	if first.Token == "" || first.ExpiresAt != 220 {
		t.Fatalf("first claim = %+v, want token and expiry 220", first)
	}

	if _, err = claims.Acquire(t.Context(), db, "doc-1", "api-b", 200, 120, 45); !errors.Is(err, ErrDocumentCleanupClaimHeld) {
		t.Fatalf("acquire during lease error = %v, want ErrDocumentCleanupClaimHeld", err)
	}
	if _, err = claims.Acquire(t.Context(), db, "doc-1", "api-b", 264, 120, 45); !errors.Is(err, ErrDocumentCleanupClaimHeld) {
		t.Fatalf("acquire during takeover grace error = %v, want ErrDocumentCleanupClaimHeld", err)
	}
	if err = claims.Renew(t.Context(), db, "doc-1", "wrong-token", 200, 120); !errors.Is(err, ErrDocumentCleanupClaimLost) {
		t.Fatalf("renew with old token error = %v, want ErrDocumentCleanupClaimLost", err)
	}

	second, err := claims.Acquire(t.Context(), db, "doc-1", "api-b", 266, 120, 45)
	if err != nil {
		t.Fatalf("take over expired claim: %v", err)
	}
	if second.Token == first.Token || second.Owner != "api-b" || second.ExpiresAt != 386 {
		t.Fatalf("replacement claim = %+v, want new api-b token expiring at 386", second)
	}
	if claims.Validate(t.Context(), db, "doc-1", first.Token, 266) {
		t.Fatal("old fencing token remained valid after takeover")
	}
	if !claims.Validate(t.Context(), db, "doc-1", second.Token, 266) {
		t.Fatal("replacement fencing token is not valid")
	}

	if released, err := claims.Release(t.Context(), db, "doc-1", first.Token); err != nil || released {
		t.Fatalf("old token release = %t, %v; want false, nil", released, err)
	}
	if released, err := claims.Release(t.Context(), db, "doc-1", second.Token); err != nil || !released {
		t.Fatalf("owner release = %t, %v; want true, nil", released, err)
	}
}

func TestDocumentCleanupClaimDAOConcurrentTakeoverHasSingleWinner(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	dbA, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open first sqlite handle: %v", err)
	}
	dbB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open second sqlite handle: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, closeErr := dbA.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, closeErr := dbB.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
	})
	if err = dbA.AutoMigrate(&entity.DocumentCleanupClaim{}); err != nil {
		t.Fatalf("migrate cleanup claim: %v", err)
	}
	if err = dbA.Create(&entity.DocumentCleanupClaim{
		DocumentID: "doc-1",
		Token:      "expired-token",
		Owner:      "old-owner",
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
				_, err := NewDocumentCleanupClaimDAO().Acquire(t.Context(), tx, "doc-1", "new-owner", 200, 120, 45)
				return err
			})
		}(db)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	held := 0
	transient := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrDocumentCleanupClaimHeld):
			held++
		case strings.Contains(err.Error(), "database table is locked"):
			// SQLite cannot wait for a read transaction to upgrade to a
			// writer. A real MySQL instance serializes this at the row lock;
			// here the loser is the transient error a caller must retry.
			transient++
		default:
			t.Fatalf("concurrent takeover error = %v, want success, held, or transient lock", err)
		}
	}
	if successes != 1 || held+transient != 1 {
		t.Fatalf("concurrent takeover outcomes: successes=%d held=%d transient=%d, want one winner and one loser", successes, held, transient)
	}
}
