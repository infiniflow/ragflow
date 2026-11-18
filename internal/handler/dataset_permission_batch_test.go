package handler

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

func permissionBatchDB(t *testing.T, n int) *gorm.DB {
	t.Helper()
	db := setupHandlerAccessDB(t)
	for i := 2; i <= n; i++ {
		id := fmt.Sprintf("ds-%d", i)
		db.Create(&entity.Knowledgebase{
			ID: id, TenantID: "tenant-1", Name: id, EmbdID: "embd-1",
			CreatedBy: "user-1", Permission: string(entity.TenantPermissionTeam),
			Status: sptr(string(entity.StatusValid)),
		})
	}
	return db
}

func countPermissionQueries(db *gorm.DB) *int {
	n := new(int)
	db.Callback().Query().Before("gorm:query").Register("handler:count_queries", func(tx *gorm.DB) {
		*n++
	})
	return n
}

// TestCheckDatasetsReadableBatchesPermissionQueries is the RED test for §2.5:
// the old per-dataset loop issues one authorization query per dataset; the
// batched path must use a single scoped query regardless of dataset count.
func TestCheckDatasetsReadableBatchesPermissionQueries(t *testing.T) {
	db := permissionBatchDB(t, 3)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	ctx := permission.WithIdentity(context.Background(), "user-1", "tenant-1")
	measure := func(n int) int {
		dbN := permissionBatchDB(t, n)
		counter := countPermissionQueries(dbN)
		ids := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			ids = append(ids, fmt.Sprintf("ds-%d", i))
		}
		if err := checkDatasetsReadable(ctx, dbN, ids); err != nil {
			t.Fatalf("checkDatasetsReadable (%d datasets) failed: %v", n, err)
		}
		return *counter
	}
	q1, q3, q6 := measure(1), measure(3), measure(6)
	if q3 > q1+2 || q6 > q1+2 {
		t.Fatalf("permission queries grew with dataset count: 1=%d, 3=%d, 6=%d (want sublinear batch load)", q1, q3, q6)
	}
}

func TestCheckDatasetsReadableRejectsInaccessibleDataset(t *testing.T) {
	db := permissionBatchDB(t, 1)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// ds-foreign belongs to a tenant the caller is not a member of.
	db.Create(&entity.Tenant{ID: "tenant-2"})
	db.Create(&entity.Knowledgebase{
		ID: "ds-foreign", TenantID: "tenant-2", Name: "foreign", EmbdID: "embd-1",
		CreatedBy: "owner-2", Permission: string(entity.TenantPermissionTeam),
		Status: sptr(string(entity.StatusValid)),
	})
	ctx := permission.WithIdentity(context.Background(), "user-1", "tenant-1")
	if err := checkDatasetsReadable(ctx, db, []string{"ds-1", "ds-foreign"}); err == nil {
		t.Fatal("expected permission error for inaccessible dataset, got nil")
	}
}

func TestCheckDatasetsReadableEmpty(t *testing.T) {
	db := permissionBatchDB(t, 1)
	counter := countPermissionQueries(db)
	ctx := permission.WithIdentity(context.Background(), "user-1", "tenant-1")
	if err := checkDatasetsReadable(ctx, db, nil); err != nil {
		t.Fatalf("empty dataset IDs should be allowed: %v", err)
	}
	if *counter != 0 {
		t.Fatalf("empty dataset IDs issued %d queries, want 0", *counter)
	}
}

func TestCheckDatasetsReadableUnauthenticated(t *testing.T) {
	db := permissionBatchDB(t, 1)
	ctx := context.Background()
	if err := checkDatasetsReadable(ctx, db, []string{"ds-1"}); err == nil {
		t.Fatal("expected error for unauthenticated context, got nil")
	}
}
