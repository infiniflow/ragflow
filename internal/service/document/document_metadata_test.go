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
	"ragflow/internal/service"
)

func TestCloneDocumentMetadata(t *testing.T) {
	if got := cloneDocumentMetadata(nil); got == nil {
		t.Fatal("nil input should return a non-nil empty map")
	}

	orig := map[string]interface{}{
		"list": []interface{}{"a", "b"},
		"str":  "x",
	}
	clone := cloneDocumentMetadata(orig)

	// Mutating the clone must not affect the original.
	clone["str"] = "changed"
	if orig["str"] != "x" {
		t.Error("clone mutation leaked into original scalar")
	}
	cloneList := clone["list"].([]interface{})
	cloneList[0] = "mutated"
	if origList, ok := orig["list"].([]interface{}); ok && origList[0] == "mutated" {
		t.Error("nested slice was not deep-copied")
	}
}

func TestDocumentMetadataValuesEqual(t *testing.T) {
	if !documentMetadataValuesEqual(1, "1") {
		t.Error("1 and \"1\" should compare equal (formatted form)")
	}
	if !documentMetadataValuesEqual(int64(1), "1") {
		t.Error("int64(1) and \"1\" should compare equal")
	}
	if documentMetadataValuesEqual("a", "b") {
		t.Error("distinct values should differ")
	}
}

func TestNormalizeMetadataListValue(t *testing.T) {
	if _, ok := normalizeMetadataListValue("scalar"); ok {
		t.Error("a scalar should not be reported as a list")
	}
	list, ok := normalizeMetadataListValue([]interface{}{"a", "b"})
	if !ok || len(list) != 2 {
		t.Errorf("[]interface{} should normalize: %v (ok=%v)", list, ok)
	}
	list2, ok2 := normalizeMetadataListValue([]string{"a", "b"})
	if !ok2 || len(list2) != 2 {
		t.Errorf("[]string should normalize: %v (ok=%v)", list2, ok2)
	}
}

func TestSplitCombinedDocumentMetadataValues(t *testing.T) {
	meta := map[string]interface{}{
		"character": []interface{}{"关羽、孙权", "张辽|赵云", "曹操"},
		"author":    "alice,bob",
		"year":      2026,
	}

	got := splitCombinedDocumentMetadataValues(meta)
	characters, ok := got["character"].([]interface{})
	if !ok {
		t.Fatalf("character has unexpected type: %T", got["character"])
	}
	want := []interface{}{"关羽", "孙权", "张辽", "赵云", "曹操"}
	if len(characters) != len(want) {
		t.Fatalf("character length = %d, want %d: %#v", len(characters), len(want), characters)
	}
	for i := range want {
		if characters[i] != want[i] {
			t.Fatalf("character[%d] = %#v, want %#v", i, characters[i], want[i])
		}
	}
	if got["author"] != "alice,bob" {
		t.Fatalf("scalar author should be preserved, got %#v", got["author"])
	}
	if got["year"] != 2026 {
		t.Fatalf("year should be preserved, got %#v", got["year"])
	}
}

func TestFirstScalarMetadataValue(t *testing.T) {
	if v, ok := firstScalarMetadataValue([]interface{}{"a", "b"}); !ok || v != "a" {
		t.Errorf("should return first non-nil scalar: %v (ok=%v)", v, ok)
	}
	if _, ok := firstScalarMetadataValue(nil); ok {
		t.Error("nil should report not-found")
	}
}

func TestNormalizeDocumentMetadataUpdateValue(t *testing.T) {
	if v := normalizeDocumentMetadataUpdateValue("42", "number"); v != int64(42) {
		t.Errorf("number string → int64(42), got %v (%T)", v, v)
	}
	list, ok := normalizeDocumentMetadataUpdateValue([]interface{}{"a"}, "list").([]interface{})
	if !ok || len(list) != 1 {
		t.Errorf("list normalize failed: %v (ok=%v)", list, ok)
	}
	if v := normalizeDocumentMetadataUpdateValue(123, "string"); v != "123" {
		t.Errorf("string normalize: got %v", v)
	}
	if v := normalizeDocumentMetadataUpdateValue("bar", "unknown"); v != "bar" {
		t.Errorf("unknown valueType should pass through: got %v", v)
	}
}

func TestAggregateMetadata(t *testing.T) {
	chunks := []map[string]interface{}{
		{"meta_fields": map[string]interface{}{"author": "alice"}},
		{"meta_fields": map[string]interface{}{"author": "bob"}},
		{"meta_fields": map[string]interface{}{"author": "alice"}},
	}
	result := aggregateMetadata(chunks)

	field, ok := result["author"].(map[string]interface{})
	if !ok {
		t.Fatalf("author field missing from summary: %v", result)
	}
	values, ok := field["values"].([][2]interface{})
	if !ok {
		t.Fatalf("values has unexpected shape: %v", field["values"])
	}
	counts := map[string]int{}
	for _, pair := range values {
		if s, ok := pair[0].(string); ok {
			counts[s] = pair[1].(int)
		}
	}
	if counts["alice"] != 2 {
		t.Errorf("alice count = %d, want 2", counts["alice"])
	}
	if counts["bob"] != 1 {
		t.Errorf("bob count = %d, want 1", counts["bob"])
	}
}

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
		entity.TableProfileMetadataField: publishedProfile(t, "金额", "作者"), "金额": "100", "作者": "张三",
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
	profile, ok, err := entity.DecodeTableProfile(base.records["doc-1"][entity.TableProfileMetadataField])
	if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
		t.Fatalf("concurrent edits lost an ownership transfer: %#v, %v", profile, err)
	}
}

func publishedProfile(t *testing.T, owned ...string) string {
	t.Helper()
	profile := &entity.TableProfile{
		Engine:        "infinity",
		Columns:       entity.DeriveTableColumns([]string{"金额"}),
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
			entity.TableProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                             []string{"100", "200"},
			"作者":                             "张三",
		},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if _, ok := record[entity.TableProfileMetadataField]; ok {
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
			entity.TableProfileMetadataField: publishedProfile(t),
			"金额":                             "用户写的",
		},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if _, ok := record[entity.TableProfileMetadataField]; ok {
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
			entity.TableProfileMetadataField: "{ not json",
			"作者":                             "张三",
		},
	})

	if err := svc.revokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	record := engine.records["doc-1"]
	if _, ok := record[entity.TableProfileMetadataField]; ok {
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
			entity.TableProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                             []string{"100"},
		},
	})
	if err := svc.SetDocumentMetadata(t.Context(), "doc-1", map[string]any{"金额": []string{"100"}}); err != nil {
		t.Fatal(err)
	}
	profile, ok, err := entity.DecodeTableProfile(engine.records["doc-1"][entity.TableProfileMetadataField])
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
			entity.TableProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                             []string{"100"},
			"作者":                             "张三",
		},
	})
	if err := svc.replaceDocumentMetadata(t.Context(), "doc-1", map[string]any{"作者": "李四"}); err != nil {
		t.Fatal(err)
	}
	profile, ok, err := entity.DecodeTableProfile(engine.records["doc-1"][entity.TableProfileMetadataField])
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
			entity.TableProfileMetadataField: publishedProfile(t, "金额"),
			"金额":                             "100",
		},
	})
	_, _, err := svc.BatchUpdateDocumentMetadatas(t.Context(), "kb-1", &MetadataSelector{DocumentIDs: []string{"doc-1"}}, []MetadataUpdate{{Key: "金额", Value: "100"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok, err := entity.DecodeTableProfile(engine.records["doc-1"][entity.TableProfileMetadataField])
	if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
		t.Fatalf("batch same-value edit did not take over: %#v, %v", profile, err)
	}
}

func TestDeleteMetadataRetainsProfileAndReleasesKey(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "key", true: "all"}[all], func(t *testing.T) {
			svc, engine := revokeTestService(t, map[string]map[string]any{"doc-1": {
				entity.TableProfileMetadataField: publishedProfile(t, "金额"), "金额": "100",
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
			profile, ok, err := entity.DecodeTableProfile(engine.records["doc-1"][entity.TableProfileMetadataField])
			if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
				t.Fatalf("metadata deletion changed indexed columns or retained ownership: %#v, %v", profile, err)
			}
			if _, ok := engine.records["doc-1"]["金额"]; ok {
				t.Fatal("deleted value survived")
			}
		})
	}
}

type memoryMetadataLocks struct {
	mu     sync.Mutex
	owners map[string]string
}

func (s *memoryMetadataLocks) SetNX(_ context.Context, key, owner string, _ time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners == nil {
		s.owners = make(map[string]string)
	}
	if _, held := s.owners[key]; held {
		return false
	}
	s.owners[key] = owner
	return true
}

func (s *memoryMetadataLocks) DeleteIfEqual(_ context.Context, key, owner string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners[key] != owner {
		return false
	}
	delete(s.owners, key)
	return true
}
