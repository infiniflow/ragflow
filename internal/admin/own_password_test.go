package admin

import (
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"golang.org/x/crypto/pbkdf2"
)

func TestChangeOwnPasswordInvalidatesPreviousSession(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.User{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	salt := []byte("legacy-password-fixture")
	const iterations = 1000
	key := pbkdf2.Key([]byte("current-password-fixture"), salt, iterations, 32, sha512.New)
	hash := fmt.Sprintf("pbkdf2:sha512:%d$%s$%s", iterations,
		base64.StdEncoding.EncodeToString(salt), base64.StdEncoding.EncodeToString(key))
	on, superuser, token := "1", true, "fixture-session-token"
	actor := entity.User{ID: "fixture-admin", Email: "admin@example.test", Nickname: "Fixture admin", Password: &hash, IsActive: on, Status: &on, IsSuperuser: &superuser, AccessToken: &token}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	if err := svc.ChangeOwnPassword(t.Context(), &actor, "wrong-password", "replacement-password"); err == nil {
		t.Fatal("incorrect current password accepted")
	}
	if err := svc.ChangeOwnPassword(t.Context(), &actor, "current-password-fixture", "replacement-password"); err != nil {
		t.Fatal(err)
	}
	var updated entity.User
	if err := db.Take(&updated, "id = ?", actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.AccessToken == nil || *updated.AccessToken == token ||
		updated.Password == nil || !common.CheckWerkzeugPassword("replacement-password", *updated.Password) {
		t.Fatal("password and session state were not updated")
	}
	var coded *common.CodedError
	err := svc.ChangeOwnPassword(t.Context(), &actor, "replacement-password", "another-password")
	if !errors.As(err, &coded) || coded.Code != common.CodeUnauthorized {
		t.Fatalf("old session can still change the password: %v", err)
	}
}
