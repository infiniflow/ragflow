package schema

import (
	"encoding/json"
	"testing"

	"ragflow/internal/parser/position"
)

func TestChunkDocEmitsOneSourceLocation(t *testing.T) {
	doc := ChunkDoc{Text: "row", Location: position.FromParserMap(map[string]any{"sheet_index": 2, "positions": [][]float64{{2, 42, 42, 1, 3}}}), Extra: map[string]json.RawMessage{
		"positions": json.RawMessage(`[[1,1,1,1,1]]`), "sheet_index": json.RawMessage(`1`), "source_location": json.RawMessage(`{"page":{}}`), "custom": json.RawMessage(`true`),
	}}
	m := doc.ToMap()
	for _, key := range []string{"positions", "_pdf_positions", "sheet", "sheet_index", "page_number"} {
		if _, ok := m[key]; ok {
			t.Errorf("backend field %s leaked into runtime payload", key)
		}
	}
	l, err := position.FromMap(m)
	if err != nil || !l.IsSpreadsheet() || *l.Spreadsheet.Index != 2 {
		t.Fatalf("typed location was shadowed: %+v err=%v", l, err)
	}
	if m["custom"] != true {
		t.Fatal("unrelated enrichment lost")
	}
}

func TestChunkDocRejectsMixedLocation(t *testing.T) {
	var doc ChunkDoc
	if err := json.Unmarshal([]byte(`{"text":"body","source_location":{"page":{},"spreadsheet":{}}}`), &doc); err == nil {
		t.Fatal("ambiguous coordinate vocabulary accepted")
	}
}
