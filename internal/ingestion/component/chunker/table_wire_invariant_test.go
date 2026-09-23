package chunker

import (
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestTableWireHasNoRowIRChunkTypes is the cross-chunker guard of the wire
// contract: the row IR (ck_type table_header/table_row) was deleted, so no
// chunker may ever emit those values again. Every registered ingestion
// chunker is enumerated rather than a hand-picked list, so a new chunker that
// reintroduces the row IR fails here.
func TestTableWireHasNoRowIRChunkTypes(t *testing.T) {
	inputs := map[string]any{
		"name":          "orders.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			spreadsheetSegmentItem("Sheet1", []string{"ID", "Status"}, [][]string{{"A-1", "paid"}, {"A-2", "open"}}, 1, 2),
		},
	}
	tested := 0
	for _, name := range runtime.DefaultRegistry.NamesByCategory(runtime.CategoryIngestion) {
		factory, _, metadata, ok := runtime.DefaultRegistry.Lookup(name)
		if !ok || factory == nil {
			continue
		}
		if _, isChunker := metadata.Outputs["chunks"]; !isChunker {
			continue
		}
		component, err := factory(name, nil)
		if err != nil {
			// Some chunkers (e.g. GroupTitle) declare required parameters;
			// without them there is no component to exercise.
			t.Logf("skipping %s: %v", name, err)
			continue
		}
		out, err := component.Invoke(t.Context(), nil, inputs)
		if err != nil {
			t.Logf("skipping %s: %v", name, err)
			continue
		}
		if out["_ERROR"] != nil {
			t.Logf("skipping %s: %v", name, out["_ERROR"])
			continue
		}
		chunks, ok := out["chunks"].([]map[string]any)
		if !ok {
			t.Errorf("%s returned chunks of type %T, want []map[string]any", name, out["chunks"])
			continue
		}
		if len(chunks) == 0 {
			t.Logf("skipping %s: no chunks for the fixture", name)
			continue
		}
		for i, chunk := range chunks {
			kind, _ := chunk["ck_type"].(string)
			if kind == "table_header" || kind == "table_row" {
				t.Errorf("%s chunk[%d] ck_type = %q: the row IR must not come back", name, i, kind)
			}
		}
		tested++
	}
	if tested < 4 {
		t.Fatalf("exercised %d chunkers, want the registered chunker set", tested)
	}
}
