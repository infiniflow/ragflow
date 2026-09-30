package indexdoc

import (
	"encoding/json"
	"testing"
)

func tableChunkerParserConfig(t *testing.T, params map[string]any) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"TableChunker:FastFoxesJump": params})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestAggregateTableDocMetadataManualRoles(t *testing.T) {
	cfg := tableChunkerParserConfig(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"金额": "metadata", "编号": "both", "名称": "indexing"},
	})
	chunks := []map[string]any{
		{"chunk_data": map[string]any{"金额": "100", "编号": "A-1", "名称": "x"}},
		{"chunk_data": map[string]any{"金额": "200", "编号": "A-1"}},
		{"chunk_data": map[string]any{"金额": ""}},
	}
	got := AggregateTableDocMetadata(chunks, cfg)
	want := map[string][]string{"金额": {"100", "200"}, "编号": {"A-1"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for col, values := range want {
		g, ok := got[col].([]string)
		if !ok {
			t.Fatalf("metadata[%s] type %T, want []string", col, got[col])
		}
		set := map[string]bool{}
		for _, s := range g {
			set[s] = true
		}
		if len(set) != len(g) {
			t.Errorf("metadata[%s] not deduped: %v", col, g)
		}
		for _, s := range values {
			if !set[s] {
				t.Errorf("metadata[%s] missing %q, got %v", col, s, g)
			}
		}
	}
	if _, ok := got["名称"]; ok {
		t.Error("indexing column must not enter document metadata")
	}
}

func TestAggregateTableDocMetadataAutoEmitsNothing(t *testing.T) {
	cfg := tableChunkerParserConfig(t, map[string]any{
		"column_mode":  "auto",
		"column_roles": map[string]any{"金额": "metadata"},
	})
	chunks := []map[string]any{{"chunk_data": map[string]any{"金额": "100"}}}
	if got := AggregateTableDocMetadata(chunks, cfg); got != nil {
		t.Errorf("auto mode must not aggregate, got %v", got)
	}
}

func TestAggregateTableDocMetadataIgnoresLegacyKeys(t *testing.T) {
	cfg := map[string]interface{}{
		"table_column_mode":  "manual",
		"table_column_roles": map[string]interface{}{"金额": "metadata"},
		"table_column_names": []interface{}{"金额"},
		"Parser:HipSignsRhyme": map[string]interface{}{
			"spreadsheet": map[string]interface{}{
				"column_mode":  "manual",
				"column_roles": map[string]interface{}{"金额": "metadata"},
			},
		},
	}
	chunks := []map[string]any{{"chunk_data": map[string]any{"金额": "100"}}}
	if got := AggregateTableDocMetadata(chunks, cfg); got != nil {
		t.Errorf("legacy config must not aggregate, got %v", got)
	}
}

func TestResolveTableColumnConfigDefaults(t *testing.T) {
	mode, roles := resolveTableColumnConfig(map[string]interface{}{})
	if mode != "auto" || roles != nil {
		t.Errorf("empty config: mode=%q roles=%#v", mode, roles)
	}
}
