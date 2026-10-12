package component

import (
	"testing"

	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/position"
)

func TestParserLocationNormalizationDoesNotMutateBackendResult(t *testing.T) {
	parsed := parser.NewCSVParser().ParseWithResult(t.Context(), "rows.csv", []byte("question,answer\nhello,world\n"))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	out := buildParserOutputs(t.Context(), parsed, "rows.csv", nil, "")
	if parsed.JSON[0]["sheet_index"] != 1 || parsed.JSON[0]["positions"] == nil {
		t.Fatal("normalization modified backend geometry")
	}
	rows := out["json"].([]map[string]any)
	l, err := position.FromMap(rows[0])
	if err != nil || !l.IsSpreadsheet() {
		t.Fatalf("runtime geometry=%+v err=%v", l, err)
	}
	for _, key := range []string{"positions", "sheet", "sheet_index"} {
		if _, ok := rows[0][key]; ok {
			t.Errorf("runtime retained backend field %s", key)
		}
	}
}
