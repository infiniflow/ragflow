package admin

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestAccountChangesPreserveAnActiveSuperuser(t *testing.T) {
	db := setupUserDeletionDB(t)
	svc := NewService()
	on, superuser := "1", true
	token := "fixture-session"
	admin := entity.User{ID: "fixture-admin", Email: "admin@example.test", Nickname: "Fixture admin", IsActive: on, Status: &on, IsSuperuser: &superuser, AccessToken: &token}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	assertConflict := func(err error) {
		t.Helper()
		var coded *common.CodedError
		if !errors.As(err, &coded) || coded.Code != common.CodeConflict {
			t.Fatalf("got %v, want a conflict", err)
		}
	}
	ctx := t.Context()
	assertConflict(svc.UpdateUserActivateStatus(ctx, admin.Email, false))
	assertConflict(svc.RevokeAdmin(ctx, admin.Email))

	second := admin
	second.ID = "fixture-second"
	second.Email = "second@example.test"
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	actorCtx := context.WithValue(ctx, adminActorKey{}, adminActor{ID: admin.ID, AccessToken: token})
	assertConflict(svc.UpdateUserActivateStatus(actorCtx, admin.Email, false))
	if err := svc.RevokeAdmin(actorCtx, second.Email); err != nil {
		t.Fatalf("revoking the other administrator should succeed: %v", err)
	}
	assertConflict(svc.RevokeAdmin(ctx, admin.Email))
}
