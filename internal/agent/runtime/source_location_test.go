package runtime

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/parser/position"
)

func TestCanvasStatePreservesSourceLocationJSON(t *testing.T) {
	l := position.FromParserMap(map[string]any{"sheet": "Orders", "sheet_index": 2, "positions": [][]float64{{2, 42, 42, 1, 3}}})
	state := NewCanvasState("run", "session")
	state.Outputs["chunker"] = map[string]any{"chunks": []map[string]any{{"text": "row", "source_location": l}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored CanvasState
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	rows := restored.Outputs["chunker"]["chunks"].([]any)
	back, err := position.FromMap(rows[0].(map[string]any))
	if err != nil || !reflect.DeepEqual(back, l) {
		t.Fatalf("restored position=%+v err=%v", back, err)
	}
}
