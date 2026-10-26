package dataset

import (
	"context"
	"net/url"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupServiceTestDB initializes an in-memory SQLite database for tests.
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	models := []interface{}{
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.Task{},
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.File2Document{},
		&entity.File{},
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.API4Conversation{},
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserCanvas{},
	}
	models = append(models, datasetTestExtraModels()...)
	if err = db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// pushServiceDB swaps dao.DB for the test and restores after.
func pushServiceDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	oldDB := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = oldDB })
}

func ensureDatasetTestMembership(t *testing.T, userID, tenantID, role string) {
	t.Helper()
	user := entity.User{
		ID: userID, Nickname: userID, Email: userID + "@test.local",
		IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0",
	}
	if err := dao.DB.Where("id = ?", userID).FirstOrCreate(&user).Error; err != nil {
		t.Fatalf("ensure test user: %v", err)
	}
	role = datasetTestMembershipRole(t, tenantID, role)
	var count int64
	if err := dao.DB.Model(&entity.UserTenant{}).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, string(entity.StatusValid)).
		Count(&count).Error; err != nil {
		t.Fatalf("count test tenant memberships: %v", err)
	}
	if count > 0 {
		return
	}
	if err := dao.DB.Create(&entity.UserTenant{
		ID:        userID + "-" + tenantID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      role,
		InvitedBy: tenantID,
		Status:    sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("insert test tenant membership: %v", err)
	}
}

func withDatasetTestContext(ctx context.Context, userID, tenantID string) context.Context {
	return permission.WithSubject(ctx, permission.Subject{UserID: userID, TenantID: tenantID})
}

func sptr(s string) *string { return &s }
