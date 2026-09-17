package dao

import (
	"errors"
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
