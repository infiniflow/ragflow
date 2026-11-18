//go:build !ee

package dao

import (
	"testing"

	"ragflow/internal/entity"
)

func TestAPITokenLimitCommunityIgnoresSetting(t *testing.T) {
	db := setupAPITokenLimitDB(t)
	for _, value := range []string{"1", "100", "invalid"} {
		if err := NewSystemSettingsDAO().SaveOrCreate(t.Context(), db, "api_key.max_count", value, "admin", "string"); err != nil {
			t.Fatal(err)
		}
		limit, err := GetAPITokenLimit(t.Context(), db)
		if err != nil || limit != 16 {
			t.Fatalf("community limit changed to %d: %v", limit, err)
		}
	}
	if err := db.Migrator().DropTable(&entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	if limit, err := GetAPITokenLimit(t.Context(), db); err != nil || limit != 16 {
		t.Fatalf("community unexpectedly requires settings: %d, %v", limit, err)
	}
}
