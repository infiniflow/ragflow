package admin

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"gorm.io/gorm"
)

func TestAccountChangesPreserveAnActiveSuperuser(t *testing.T) {
	db := setupAccountGuardDB(t)
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

func TestPasswordResetPreservesConcurrentActivation(t *testing.T) {
	db := setupAccountGuardDB(t)
	inactive, valid, superuser := "0", "1", true
	user := entity.User{
		ID: "fixture-target", Email: "target@example.test", Nickname: "Fixture target",
		IsActive: inactive, Status: &valid, IsSuperuser: &superuser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	activationWritten := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:activate-before-password-write", func(tx *gorm.DB) {
		if activationWritten || tx.Statement.Table != "user" {
			return
		}
		activationWritten = true
		result := tx.Exec("UPDATE user SET is_active = ? WHERE id = ?", "1", user.ID)
		if result.Error != nil {
			_ = tx.AddError(result.Error)
		}
	}); err != nil {
		t.Fatal(err)
	}

	const password = "new-password-fixture"
	if err := NewService().ChangePassword(t.Context(), "fixture-admin", user.Email, password); err != nil {
		t.Fatal(err)
	}
	var current entity.User
	if err := db.Take(&current, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !activationWritten || !activeSuperuser(&current) {
		t.Fatal("password reset overwrote the concurrent activation")
	}
	if current.Password == nil || !common.CheckWerkzeugPassword(password, *current.Password) {
		t.Fatal("password was not updated")
	}
}

func setupAccountGuardDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t, &entity.User{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	return db
}
