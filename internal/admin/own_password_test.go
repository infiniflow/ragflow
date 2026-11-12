package admin

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestChangeOwnPasswordInvalidatesPreviousSession(t *testing.T) {
	db := setupUserDeletionDB(t)
	hash, err := common.GenerateWerkzeugPasswordHash("current-password-fixture")
	if err != nil {
		t.Fatal(err)
	}
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
	err = svc.ChangeOwnPassword(t.Context(), &actor, "replacement-password", "another-password")
	if !errors.As(err, &coded) || coded.Code != common.CodeUnauthorized {
		t.Fatalf("old session can still change the password: %v", err)
	}
}
