package service

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

// tableProfileTestEngine answers only the metadata read the derived field map
// needs. The embedded nil interface makes any other engine call panic, which is
// what a test that reached for more than it declared should do.
type tableProfileTestEngine struct {
	engine.DocEngine
	engineType string
	records    []map[string]interface{}
}

func (e *tableProfileTestEngine) GetType() string { return e.engineType }

func (e *tableProfileTestEngine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	wanted, _ := req.Filter["id"].([]string)
	keep := make(map[string]struct{}, len(wanted))
	for _, id := range wanted {
		keep[id] = struct{}{}
	}
	out := make([]map[string]interface{}, 0, len(e.records))
	for _, record := range e.records {
		docID, _ := record["id"].(string)
		if _, ok := keep[docID]; ok {
			out = append(out, record)
		}
	}
	return &types.SearchMetadataResult{MetadataRecords: out, Total: int64(len(out))}, nil
}

func setupTableProfileDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	status := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1", Name: "sales", Status: &status, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("seed kb: %v", err)
	}
	return db
}

func seedDocument(t *testing.T, db *gorm.DB, id string, enabled bool) {
	t.Helper()
	status := string(entity.StatusValid)
	if !enabled {
		status = string(entity.StatusInvalid)
	}
	name := id + ".xlsx"
	doc := &entity.Document{ID: id, KbID: "kb-1", Status: &status, Name: &name, Suffix: "xlsx", ParserConfig: entity.JSONMap{}}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("seed document %s: %v", id, err)
	}
}

func profileRecord(t *testing.T, docID, engineName string, headers ...string) map[string]interface{} {
	t.Helper()
	cols := entity.DeriveTableColumns(headers)
	profile := &entity.TableProfile{Engine: engineName, Columns: cols}
	raw, err := profile.Encode()
	if err != nil {
		t.Fatalf("encode profile: %v", err)
	}
	return map[string]interface{}{
		"id":    docID,
		"kb_id": "kb-1",
		"meta_fields": map[string]interface{}{
			entity.TableProfileMetadataField: raw,
			"作者":                             "张三",
		},
	}
}

func tableProfileService(engineType string, records []map[string]interface{}) *MetadataService {
	return NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), &tableProfileTestEngine{engineType: engineType, records: records})
}

func TestTableFieldMapReportsMalformedProfile(t *testing.T) {
	db := setupTableProfileDB(t)
	seedDocument(t, db, "doc-bad", true)
	seedDocument(t, db, "doc-good", true)
	bad := map[string]any{"id": "doc-bad", "meta_fields": map[string]any{entity.TableProfileMetadataField: "{broken"}}
	_, _, err := tableProfileService("infinity", []map[string]any{bad, profileRecord(t, "doc-good", "infinity", "金额")}).TableFieldMap(t.Context(), []string{"kb-1"})
	if err == nil || !strings.Contains(err.Error(), "doc-bad") {
		t.Fatalf("malformed publisher was silently dropped: %v", err)
	}
}

func TestTableFieldMapSkipsIncompleteProfiles(t *testing.T) {
	for _, raw := range []string{`{"columns":[{"key":"金额"}]}`, `{"engine":"infinity","columns":[]}`} {
		t.Run(raw, func(t *testing.T) {
			db := setupTableProfileDB(t)
			seedDocument(t, db, "doc-incomplete", true)
			seedDocument(t, db, "doc-good", true)
			incomplete := map[string]any{"id": "doc-incomplete", "meta_fields": map[string]any{entity.TableProfileMetadataField: raw}}
			fields, ids, err := tableProfileService("infinity", []map[string]any{incomplete, profileRecord(t, "doc-good", "infinity", "金额")}).TableFieldMap(t.Context(), []string{"kb-1"})
			if err != nil || len(fields) != 1 || len(ids) != 1 || ids[0] != "doc-good" {
				t.Fatalf("incomplete profile affected the valid publisher: fields=%v ids=%v err=%v", fields, ids, err)
			}
		})
	}
}

func TestTableFieldMapUnionsIndexedDocuments(t *testing.T) {
	db := setupTableProfileDB(t)
	seedDocument(t, db, "doc-1", true)
	seedDocument(t, db, "doc-2", true)
	seedDocument(t, db, "doc-off", false)

	recordOne := profileRecord(t, "doc-1", "infinity", "金额")
	recordTwo := profileRecord(t, "doc-2", "infinity", "编号")
	recordOff := profileRecord(t, "doc-off", "infinity", "不该出现")

	fields, docIDs, err := tableProfileService("infinity", []map[string]interface{}{recordOne, recordTwo, recordOff}).
		TableFieldMap(context.Background(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("TableFieldMap: %v", err)
	}

	want := map[string]string{
		entity.TableDataKey("金额"): "金额",
		entity.TableDataKey("编号"): "编号",
	}
	if len(fields) != len(want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
	for dataKey, name := range want {
		if fields[dataKey] != name {
			t.Errorf("fields[%q] = %v, want %q", dataKey, fields[dataKey], name)
		}
	}
	if strings.Join(docIDs, ",") != "doc-1,doc-2" {
		t.Errorf("contributing documents = %v, want the two enabled ones", docIDs)
	}
}

func TestTableFieldMapSkipsDocumentsThatCannotBeQueried(t *testing.T) {
	db := setupTableProfileDB(t)
	seedDocument(t, db, "doc-1", true)
	seedDocument(t, db, "doc-other-engine", true)
	seedDocument(t, db, "doc-plain", true)

	records := []map[string]interface{}{
		profileRecord(t, "doc-1", "infinity", "金额"),
		// Rows a previous engine indexed are not readable from the current one.
		profileRecord(t, "doc-other-engine", "elasticsearch", "旧列"),
		// A document with no table run at all simply has no record.
		{"id": "doc-plain", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"作者": "张三"}},
	}
	fields, docIDs, err := tableProfileService("infinity", records).TableFieldMap(context.Background(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("TableFieldMap: %v", err)
	}
	if len(fields) != 1 || len(docIDs) != 1 || docIDs[0] != "doc-1" {
		t.Errorf("fields = %v, docs = %v, want only doc-1", fields, docIDs)
	}
	for dataKey := range fields {
		if dataKey == entity.TableDataKey("旧列") {
			t.Error("a record from another engine must not contribute fields")
		}
	}
}

func TestTableFieldMapReadsIndexedFieldsOnEveryEngine(t *testing.T) {
	db := setupTableProfileDB(t)
	seedDocument(t, db, "doc-1", true)

	// A read-only view of what was indexed is useful even where the engine
	// cannot query it; whether SQL may run is the caller's gate, not this read's.
	fields, docIDs, err := tableProfileService("elasticsearch", []map[string]interface{}{
		profileRecord(t, "doc-1", "elasticsearch", "金额"),
	}).TableFieldMap(context.Background(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("TableFieldMap: %v", err)
	}
	if len(fields) != 1 || len(docIDs) != 1 {
		t.Errorf("fields = %v, docs = %v, want the indexed column", fields, docIDs)
	}
}

func TestTableFieldMapReportsConflictingColumnNames(t *testing.T) {
	db := setupTableProfileDB(t)
	seedDocument(t, db, "doc-1", true)
	seedDocument(t, db, "doc-2", true)

	// Both documents indexed the header "金额", so both write the same data_key;
	// only a publisher bug could give it two different names.
	one := profileRecord(t, "doc-1", "infinity", "金额")
	two := profileRecord(t, "doc-2", "infinity", "金额")
	two["meta_fields"].(map[string]interface{})[entity.TableProfileMetadataField] = func() string {
		profile := &entity.TableProfile{
			Engine:  "infinity",
			Columns: []entity.TableColumn{{Index: 1, Key: "金额", DisplayName: "金额(抄错的)", DataKey: entity.TableDataKey("金额")}},
		}
		raw, err := profile.Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return raw
	}()

	fields, _, err := tableProfileService("infinity", []map[string]interface{}{one, two}).
		TableFieldMap(context.Background(), []string{"kb-1"})
	if err == nil {
		t.Fatalf("expected a conflict error, got fields %v", fields)
	}
	if !strings.Contains(err.Error(), "金额") {
		t.Errorf("error does not name the conflicting column: %v", err)
	}
}

func TestTableFieldMapNoKnowledgeBases(t *testing.T) {
	setupTableProfileDB(t)
	fields, docIDs, err := tableProfileService("infinity", nil).TableFieldMap(context.Background(), nil)
	if err != nil || fields != nil || docIDs != nil {
		t.Errorf("fields = %v, docs = %v, err = %v, want no result", fields, docIDs, err)
	}
}

func TestConvertSearchResultToDocMetaHidesProfileField(t *testing.T) {
	record := profileRecord(t, "doc-1", "infinity", "金额")
	meta := ConvertSearchResultToDocMeta([]map[string]interface{}{record})
	fields, ok := meta["doc-1"]
	if !ok {
		t.Fatal("document missing from the read")
	}
	if _, exists := fields[entity.TableProfileMetadataField]; exists {
		t.Error("the system record leaked into user-visible metadata")
	}
	if fields["作者"] != "张三" {
		t.Errorf("ordinary metadata lost: %v", fields)
	}
}
