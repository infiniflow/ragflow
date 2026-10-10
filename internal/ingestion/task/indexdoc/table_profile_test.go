package indexdoc

import (
	"encoding/json"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// tableRowChunk builds one indexed row the way the chunker hands it over:
// chunk_data keyed by data_key, and the row's own column identity carrying the
// roles that were declared for it.
func tableRowChunk(t *testing.T, mode string, declared map[string]string, headers []string, cells map[string]string) map[string]any {
	t.Helper()
	cols := entity.DeriveTableColumns(headers)
	data := make(map[string]any, len(cols))
	for _, col := range cols {
		if mode == entity.TableModeAuto || declared[col.Key] != entity.TableRoleIndexing {
			data[col.DataKey] = cells[col.Key]
		}
	}
	doc := schema.ChunkDoc{
		Text:        "row",
		ChunkData:   data,
		TableRowInt: 1,
		TableRowSource: &schema.TableRowSource{
			SheetIndex: 1,
			SourceRow:  2,
			Mode:       mode,
			Columns:    cols,
			Roles:      declared,
		},
	}
	// The round trip is what the pipeline actually carries: numbers arrive as
	// float64 and nested values as plain maps.
	raw, err := json.Marshal(doc.ToMap())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var ck map[string]any
	if err := json.Unmarshal(raw, &ck); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return ck
}

func findColumn(cols []entity.TableColumn, key string) (entity.TableColumn, bool) {
	for _, col := range cols {
		if col.Key == key {
			return col, true
		}
	}
	return entity.TableColumn{}, false
}

func columnKeys(cols []entity.TableColumn) map[string]string {
	out := make(map[string]string, len(cols))
	for _, col := range cols {
		out[col.Key] = col.DisplayName
	}
	return out
}

// TestProjectTableChunksManualRoles is the contract the retrieval path reads:
// queryable columns from chunk_data, document values from the explicitly
// configured metadata/both columns.
func TestProjectTableChunksManualRoles(t *testing.T) {
	declared := map[string]string{"金额": "metadata", "编号": "both", "名称": "indexing"}
	headers := []string{"金额", "编号", "名称"}
	chunks := []map[string]any{
		tableRowChunk(t, entity.TableModeManual, declared, headers, map[string]string{"金额": "100", "编号": "A-1", "名称": "x"}),
		tableRowChunk(t, entity.TableModeManual, declared, headers, map[string]string{"金额": "200", "编号": "A-1"}),
		tableRowChunk(t, entity.TableModeManual, declared, headers, map[string]string{"金额": ""}),
	}

	profile, values := ProjectTableChunks(chunks, "infinity")
	if profile == nil {
		t.Fatal("manual rows must produce a profile")
	}
	keys := columnKeys(profile.Columns)
	if _, ok := keys["名称"]; ok {
		t.Errorf("an indexing-only column has no structured value to query: %v", keys)
	}
	if len(keys) != 2 || keys["金额"] != "金额" || keys["编号"] != "编号" {
		t.Errorf("profile columns = %v, want 金额 and 编号", keys)
	}

	want := map[string][]string{"金额": {"100", "200"}, "编号": {"A-1"}}
	if len(values) != len(want) {
		t.Fatalf("values = %v, want %v", values, want)
	}
	for col, expected := range want {
		got, ok := values[col].([]string)
		if !ok {
			t.Fatalf("values[%s] type %T, want []string", col, values[col])
		}
		if len(got) != len(expected) {
			t.Fatalf("values[%s] = %v, want %v", col, got, expected)
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("values[%s][%d] = %q, want %q", col, i, got[i], expected[i])
			}
		}
	}

	if profile.Engine != "infinity" {
		t.Errorf("engine = %q", profile.Engine)
	}
}

// TestProjectTableChunksManualDefaultColumn: a manual column with no entry is
// written to chunk_data as "both" but contributes no document value, which is
// what keeps a wide table from turning its whole content into document metadata.
func TestProjectTableChunksManualDefaultColumn(t *testing.T) {
	declared := map[string]string{"金额": "metadata"}
	chunks := []map[string]any{
		tableRowChunk(t, entity.TableModeManual, declared, []string{"金额", "备注"}, map[string]string{"金额": "100", "备注": "急"}),
	}

	profile, values := ProjectTableChunks(chunks, "infinity")
	if _, ok := findColumn(profile.Columns, "备注"); !ok {
		t.Error("an unconfigured manual column must still be queryable")
	}
	if _, exists := values["备注"]; exists {
		t.Errorf("an unconfigured column must not be aggregated: %v", values)
	}
	if values["金额"] == nil {
		t.Errorf("the configured metadata column is missing: %v", values)
	}
}

func TestProjectTableChunksAutoPublishesNoDocumentValues(t *testing.T) {
	chunks := []map[string]any{
		tableRowChunk(t, entity.TableModeAuto, nil, []string{"金额"}, map[string]string{"金额": "100"}),
	}

	profile, values := ProjectTableChunks(chunks, "infinity")
	if profile == nil {
		t.Fatal("auto rows still describe queryable columns")
	}
	if len(values) != 0 {
		t.Errorf("auto mode must not aggregate document values, got %v", values)
	}
	if _, ok := findColumn(profile.Columns, "金额"); !ok {
		t.Errorf("auto columns missing: %#v", profile.Columns)
	}
}

func TestProjectTableChunksIgnoresNonTableRowChunks(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_data": map[string]any{"c_1": "100"}},
		{"content_with_weight": "prose"},
	}
	profile, values := ProjectTableChunks(chunks, "infinity")
	if profile != nil || values != nil {
		t.Errorf("a chunk without row identity must contribute nothing: %#v %#v", profile, values)
	}
}

func TestProjectTableChunksEmptyCellsStillPublishColumns(t *testing.T) {
	chunks := []map[string]any{
		tableRowChunk(t, entity.TableModeManual, map[string]string{"金额": "metadata"}, []string{"金额"}, map[string]string{"金额": ""}),
	}
	profile, values := ProjectTableChunks(chunks, "infinity")
	if profile == nil {
		t.Fatal("the column is indexed, so the profile must describe it")
	}
	if len(values) != 0 {
		t.Errorf("empty cells must not become document values: %v", values)
	}
	if _, ok := findColumn(profile.Columns, "金额"); !ok {
		t.Errorf("the column must stay queryable: %#v", profile.Columns)
	}
}

// The profile is a cross-sheet union keyed by data_key: one header is one
// column no matter where it sits, and a position that only describes one of the
// sheets is not recorded.
func TestProjectTableChunksDropsColumnPositionAcrossSheets(t *testing.T) {
	first := tableRowChunk(t, entity.TableModeAuto, nil,
		[]string{"\u91d1\u989d", "\u5907\u6ce8"}, map[string]string{"\u91d1\u989d": "100", "\u5907\u6ce8": "a"})
	second := tableRowChunk(t, entity.TableModeAuto, nil,
		[]string{"\u5907\u6ce8", "\u91d1\u989d"}, map[string]string{"\u5907\u6ce8": "b", "\u91d1\u989d": "200"})
	source, ok := second["table_row_source"].(map[string]any)
	if !ok {
		t.Fatalf("row source is %T", second["table_row_source"])
	}
	source["sheet_index"] = 2

	profile, _ := ProjectTableChunks([]map[string]any{first, second}, "infinity")
	if profile == nil {
		t.Fatal("expected a profile")
	}
	if len(profile.Columns) != 2 {
		t.Fatalf("columns = %#v", profile.Columns)
	}
	col, found := findColumn(profile.Columns, "\u91d1\u989d")
	if !found {
		t.Fatalf("column missing from %#v", profile.Columns)
	}
	if col.Index != 0 {
		t.Errorf("the profile kept a per-sheet position: %d", col.Index)
	}
	if col.DataKey != entity.TableDataKey("\u91d1\u989d") {
		t.Errorf("data key = %q", col.DataKey)
	}
}
