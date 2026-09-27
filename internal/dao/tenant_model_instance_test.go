package dao

import (
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupTenantModelInstanceDAOTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.TenantModelInstance{}); err != nil {
		t.Fatalf("failed to migrate tenant_model_instance: %v", err)
	}
	return db
}

func seedTenantModelInstance(t *testing.T, db *gorm.DB, instance *entity.TenantModelInstance) {
	t.Helper()
	if err := db.Create(instance).Error; err != nil {
		t.Fatalf("failed to seed tenant model instance: %v", err)
	}
}

// The random-token primary keys make the storage order unrelated to creation
// time, so the DAO must sort by create_time explicitly (mirroring Python's
// list_provider_instances).
func TestTenantModelInstanceDAOGetAllInstancesByProviderIDOrdersByCreateTimeDesc(t *testing.T) {
	db := setupTenantModelInstanceDAOTestDB(t)

	mkTime := func(v int64) *int64 { return &v }
	// Inserted so that primary-key order differs from creation order.
	seedTenantModelInstance(t, db, &entity.TenantModelInstance{ID: "c-instance-middle", InstanceName: "middle", ProviderID: "provider-1", APIKey: "k", BaseModel: entity.BaseModel{CreateTime: mkTime(2000)}})
	seedTenantModelInstance(t, db, &entity.TenantModelInstance{ID: "a-instance-oldest", InstanceName: "oldest", ProviderID: "provider-1", APIKey: "k", BaseModel: entity.BaseModel{CreateTime: mkTime(1000)}})
	seedTenantModelInstance(t, db, &entity.TenantModelInstance{ID: "b-instance-newest", InstanceName: "newest", ProviderID: "provider-1", APIKey: "k", BaseModel: entity.BaseModel{CreateTime: mkTime(3000)}})
	seedTenantModelInstance(t, db, &entity.TenantModelInstance{ID: "d-other-provider", InstanceName: "other", ProviderID: "provider-2", APIKey: "k", BaseModel: entity.BaseModel{CreateTime: mkTime(4000)}})

	instances, err := NewTenantModelInstanceDAO().GetAllInstancesByProviderID(t.Context(), db, "provider-1")
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}

	var names []string
	for _, instance := range instances {
		names = append(names, instance.InstanceName)
	}
	// Pin the provider filter explicitly: the provider-2 row must be absent.
	if slices.Contains(names, "other") {
		t.Fatalf("expected only provider-1 instances, got %v", names)
	}
	want := []string{"newest", "middle", "oldest"}
	if len(names) != len(want) {
		t.Fatalf("expected %d instances, got %v", len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("expected instance order %v, got %v", want, names)
		}
	}
}
