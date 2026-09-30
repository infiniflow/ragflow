package document

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	ingestiontable "ragflow/internal/ingestion/table"
	"ragflow/internal/service"
)

func publishedProfile(t *testing.T, owned ...string) string {
	t.Helper()
	spec := ingestiontable.Spec{
		Mode:  ingestiontable.ModeManual,
		Roles: map[string]string{"金额": ingestiontable.RoleMetadata},
	}
	profile := &ingestiontable.Profile{
		Engine:        "infinity",
		Columns:       ingestiontable.DeriveColumns([]string{"金额"}),
		Specs:         map[string]ingestiontable.Spec{spec.Key(): spec},
		OwnedMetadata: owned,
	}
	raw, err := profile.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

func revokeTestService(t *testing.T, records map[string]map[string]any) (*DocumentService, *metadataDocEngine) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "sales.xlsx", 0, 0)

	engine := newMetadataDocEngine(records, map[string]string{"doc-1": "kb-1"})
	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engine)
	return svc, engine
}

// TestRevokeTableProfileRemovesItsOwnContributions is what keeps a re-parse from
// leaving columns queryable against rows that no longer exist.
func TestRevokeTableProfileRemovesItsOwnContributions(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {
			ingestiontable.ProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                                []string{"100", "200"},
			"作者":                                "张三",
		},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if _, ok := record[ingestiontable.ProfileMetadataField]; ok {
		t.Errorf("the profile survived: %v", record)
	}
	if _, ok := record["金额"]; ok {
		t.Errorf("a column value the table system owned survived: %v", record)
	}
	if record["作者"] != "张三" {
		t.Errorf("metadata the table system never owned was removed: %v", record)
	}
}

// TestRevokeTableProfileLeavesTakenOverKey: a key dropped from the ownership
// list belongs to a user or the LLM now, so discarding the document's indexed
// rows must not discard their value.
func TestRevokeTableProfileLeavesTakenOverKey(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {
			ingestiontable.ProfileMetadataField: publishedProfile(t),
			"金额":                                "用户写的",
		},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if _, ok := record[ingestiontable.ProfileMetadataField]; ok {
		t.Errorf("the profile survived: %v", record)
	}
	if record["金额"] != "用户写的" {
		t.Errorf("a taken-over key was removed: %v", record)
	}
}

func TestRevokeTableProfileWithoutARecordIsNoop(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {"作者": "张三"},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if len(record) != 1 || record["作者"] != "张三" {
		t.Errorf("an unrelated record changed: %v", record)
	}
}

func TestRevokeTableProfileClearsUnreadableRecord(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {
			ingestiontable.ProfileMetadataField: "{ not json",
			"作者":                                "张三",
		},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if _, ok := record[ingestiontable.ProfileMetadataField]; ok {
		t.Errorf("an unreadable record must still go: %v", record)
	}
	if record["作者"] != "张三" {
		t.Errorf("other metadata removed while clearing a broken record: %v", record)
	}
}

func TestRevokeTableProfileWithoutEngineIsNoop(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "sales.xlsx", 0, 0)

	svc := testDocumentService(t)
	svc.docEngine = nil
	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke without an engine: %v", err)
	}
	var count int64
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("document disappeared: %d", count)
	}
}
