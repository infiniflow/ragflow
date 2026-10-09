package document

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	ingestiontable "ragflow/internal/ingestion/table"
	"ragflow/internal/service"
)

type concurrentMetadataEngine struct {
	*metadataDocEngine
	mu        sync.Mutex
	reads     atomic.Int32
	firstRead chan struct{}
	release   chan struct{}
}

func (e *concurrentMetadataEngine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	e.mu.Lock()
	result, err := e.metadataDocEngine.SearchMetadata(ctx, req)
	for _, record := range result.MetadataRecords {
		record["meta_fields"] = cloneDocumentMetadata(record["meta_fields"].(map[string]any))
	}
	e.mu.Unlock()
	if e.reads.Add(1) == 1 {
		close(e.firstRead)
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return result, err
}

func (e *concurrentMetadataEngine) UpdateMetadata(ctx context.Context, docID, kbID string, meta map[string]any, tenantID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.metadataDocEngine.UpdateMetadata(ctx, docID, kbID, meta, tenantID)
}

func TestConcurrentMetadataEditsRetainBothOwnershipTransfers(t *testing.T) {
	svc, base := revokeTestService(t, map[string]map[string]any{"doc-1": {
		ingestiontable.ProfileMetadataField: publishedProfile(t, "金额", "作者"), "金额": "100", "作者": "张三",
	}})
	e := &concurrentMetadataEngine{metadataDocEngine: base, firstRead: make(chan struct{}), release: make(chan struct{})}
	svc.docEngine = e
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), e)
	first := make(chan error, 1)
	go func() { first <- svc.SetDocumentMetadata(t.Context(), "doc-1", map[string]any{"金额": "100"}) }()
	<-e.firstRead
	second := make(chan error, 1)
	go func() { second <- svc.SetDocumentMetadata(t.Context(), "doc-1", map[string]any{"作者": "张三"}) }()
	select {
	case err := <-second:
		if err != nil {
			t.Error(err)
		}
		second <- nil
	case <-time.After(50 * time.Millisecond):
	}
	close(e.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	profile, ok, err := ingestiontable.DecodeProfile(base.records["doc-1"][ingestiontable.ProfileMetadataField])
	if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
		t.Fatalf("concurrent edits lost an ownership transfer: %#v, %v", profile, err)
	}
}

func publishedProfile(t *testing.T, owned ...string) string {
	t.Helper()
	profile := &ingestiontable.Profile{
		Engine:        "infinity",
		Columns:       ingestiontable.DeriveColumns([]string{"金额"}),
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

func TestSetMetadataTakesOverColumnEvenWithSameValue(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {
			ingestiontable.ProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                                []string{"100"},
		},
	})
	if err := svc.SetDocumentMetadata(t.Context(), "doc-1", map[string]any{"金额": []string{"100"}}); err != nil {
		t.Fatal(err)
	}
	profile, ok, err := ingestiontable.DecodeProfile(engine.records["doc-1"][ingestiontable.ProfileMetadataField])
	if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
		t.Fatalf("user edit must preserve profile and relinquish ownership: %#v, %v", profile, err)
	}
	if err := svc.RevokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatal(err)
	}
	if engine.records["doc-1"]["金额"] == nil {
		t.Fatal("revoking rows deleted the user's value")
	}
}

func TestReplaceMetadataPreservesTableProfile(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {
			ingestiontable.ProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                                []string{"100"},
			"作者":                                "张三",
		},
	})
	if err := svc.replaceDocumentMetadata(t.Context(), "doc-1", map[string]any{"作者": "李四"}); err != nil {
		t.Fatal(err)
	}
	profile, ok, err := ingestiontable.DecodeProfile(engine.records["doc-1"][ingestiontable.ProfileMetadataField])
	if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
		t.Fatalf("replacement must retain columns without owning user metadata: %#v, %v", profile, err)
	}
	if _, ok := engine.records["doc-1"]["金额"]; ok {
		t.Fatal("replacement retained an omitted value")
	}
}

func TestBatchMetadataSameValueTakesOverColumn(t *testing.T) {
	svc, engine := revokeTestService(t, map[string]map[string]any{
		"doc-1": {
			ingestiontable.ProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                                "100",
		},
	})
	_, _, err := svc.BatchUpdateDocumentMetadatas(t.Context(), "kb-1", &MetadataSelector{DocumentIDs: []string{"doc-1"}}, []MetadataUpdate{{Key: "金额", Value: "100"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok, err := ingestiontable.DecodeProfile(engine.records["doc-1"][ingestiontable.ProfileMetadataField])
	if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
		t.Fatalf("batch same-value edit did not take over: %#v, %v", profile, err)
	}
}

func TestDeleteMetadataRetainsProfileAndReleasesKey(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "key", true: "all"}[all], func(t *testing.T) {
			svc, engine := revokeTestService(t, map[string]map[string]any{"doc-1": {
				ingestiontable.ProfileMetadataField: publishedProfile(t, "金额"), "金额": "100",
			}})
			var err error
			if all {
				err = svc.DeleteDocumentAllMetadata(t.Context(), "doc-1")
			} else {
				err = svc.DeleteDocumentMetadata(t.Context(), "doc-1", []string{"金额"})
			}
			if err != nil {
				t.Fatal(err)
			}
			profile, ok, err := ingestiontable.DecodeProfile(engine.records["doc-1"][ingestiontable.ProfileMetadataField])
			if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
				t.Fatalf("metadata deletion changed indexed columns or retained ownership: %#v, %v", profile, err)
			}
			if _, ok := engine.records["doc-1"]["金额"]; ok {
				t.Fatal("deleted value survived")
			}
		})
	}
}
