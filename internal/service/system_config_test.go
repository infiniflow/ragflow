package service

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/server"
)

func TestPublicConfigAPITokenLimit(t *testing.T) {
	if server.GetConfig() == nil {
		if err := server.Init(""); err != nil {
			t.Fatal(err)
		}
	}
	db := testutil.SetupTestDB(t, &entity.SystemSettings{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"", 16},
		{"24", 24},
		{"invalid", 16},
	} {
		if tc.value != "" {
			if err := dao.NewSystemSettingsDAO().SaveOrCreate(t.Context(), db, "api_key.max_count", tc.value, "admin", "integer"); err != nil {
				t.Fatal(err)
			}
		}
		config, err := (&SystemService{}).GetConfig(t.Context())
		if err != nil || config.APIKeyMaxCount != tc.want {
			t.Fatalf("public API key limit for %q: config=%+v error=%v", tc.value, config, err)
		}
	}
	if err := db.Migrator().DropTable(&entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	config, err := (&SystemService{}).GetConfig(t.Context())
	if err != nil || config.APIKeyMaxCount != 16 {
		t.Fatalf("database error did not use public default: config=%+v error=%v", config, err)
	}
}
