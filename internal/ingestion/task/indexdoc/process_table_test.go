package indexdoc

import (
	"encoding/json"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	ingestiontable "ragflow/internal/ingestion/table"
)

// tableRowChunk renders one indexed row the way the chunker hands it over: the
// column identity travels with the row, and chunk_data is keyed by data_key.
func tableRowChunk(t *testing.T, mode string, roles map[string]string, headers []string, values map[string]string) map[string]any {
	t.Helper()
	cols := ingestiontable.DeriveColumns(headers)
	data := make(map[string]any, len(cols))
	for _, col := range cols {
		if v, ok := values[col.Key]; ok {
			data[col.DataKey] = v
			continue
		}
		data[col.DataKey] = ""
	}
	src := schema.TableRowSource{
		NodeID:     "TableChunker:FastFoxesJump",
		SheetIndex: 1,
		SourceRow:  2,
		Mode:       mode,
		Columns:    cols,
		Roles:      roles,
	}
	raw, err := json.Marshal(map[string]any{"table_row_source": src, "chunk_data": data})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var ck map[string]any
	if err := json.Unmarshal(raw, &ck); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return ck
}

func TestAggregateTableDocMetadataManualRoles(t *testing.T) {
	cols := []string{"金额", "编号", "名称"}
	roles := map[string]string{"金额": "metadata", "编号": "both", "名称": "indexing"}
	chunks := []map[string]any{
		tableRowChunk(t, "manual", roles, cols, map[string]string{"金额": "100", "编号": "A-1", "名称": "x"}),
		tableRowChunk(t, "manual", roles, cols, map[string]string{"金额": "200", "编号": "A-1"}),
		tableRowChunk(t, "manual", roles, cols, map[string]string{"金额": ""}),
	}
	got := AggregateTableDocMetadata(chunks)
	want := map[string][]string{"金额": {"100", "200"}, "编号": {"A-1"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for col, values := range want {
		g, ok := got[col].([]string)
		if !ok {
			t.Fatalf("metadata[%s] type %T, want []string", col, got[col])
		}
		if len(g) != len(values) {
			t.Fatalf("metadata[%s] = %v, want %v", col, g, values)
		}
		for i := range values {
			if g[i] != values[i] {
				t.Errorf("metadata[%s][%d] = %q, want %q", col, i, g[i], values[i])
			}
		}
	}
	if _, ok := got["名称"]; ok {
		t.Error("indexing column must not enter document metadata")
	}
}

func TestAggregateTableDocMetadataAutoEmitsNothing(t *testing.T) {
	roles := map[string]string{"金额": "metadata"}
	chunks := []map[string]any{
		tableRowChunk(t, "auto", roles, []string{"金额"}, map[string]string{"金额": "100"}),
	}
	if got := AggregateTableDocMetadata(chunks); got != nil {
		t.Errorf("auto mode must not aggregate, got %v", got)
	}
}

func TestAggregateTableDocMetadataIgnoresNonTableRowChunks(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_data": map[string]any{"金额": "100"}},
		{"content_with_weight": "prose"},
	}
	if got := AggregateTableDocMetadata(chunks); got != nil {
		t.Errorf("a chunk without row identity must not aggregate, got %v", got)
	}
}
