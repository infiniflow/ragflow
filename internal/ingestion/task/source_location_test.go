package task

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/parser/position"
)

func TestDebugOutputProjectsSourceLocationWithoutChangingPipelineState(t *testing.T) {
	l := position.FromParserMap(map[string]any{"sheet_index": 2, "positions": [][]float64{{2, 42, 42, 1, 3}}})
	chunk := map[string]any{"text": "row", "source_location": l}
	executor := &PipelineExecutor{taskCtx: &TaskContext{Doc: entity.Document{ID: "doc", KbID: "kb"}}}
	result, err := executor.collectDebugOutput(t.Context(), map[string]any{"chunks": []map[string]any{chunk}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || !reflect.DeepEqual(result.Chunks[0]["positions"], [][]float64{{2, 42, 42, 1, 3}}) {
		t.Fatalf("debug output=%v", result.Chunks)
	}
	if _, ok := result.Chunks[0]["source_location"]; ok {
		t.Fatal("internal location leaked into public result")
	}
	if chunk["source_location"] != l {
		t.Fatal("projection modified pipeline state")
	}
	if _, ok := chunk["positions"]; ok {
		t.Fatal("projection added public geometry to pipeline state")
	}
}

func TestDebugExportProjectsLocationsAndKeepsUserMetadata(t *testing.T) {
	l := position.FromParserMap(map[string]any{"sheet_index": 2, "positions": [][]float64{{2, 42, 42, 1, 3}}})
	row := map[string]any{"text": "row", "source_location": l, "q_4_vec": []float64{1, 2, 3, 4}, "metadata": map[string]any{"source_location": "customer value"}}
	out := map[string]any{"chunks": []map[string]any{row}}
	message := endOutputMessage([]debugLogEntry{{ComponentID: "chunker"}}, map[string]any{"chunker": out})
	var public map[string]any
	if err := json.Unmarshal([]byte(message), &public); err != nil {
		t.Fatal(err)
	}
	chunk := public["chunks"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(chunk["positions"], []any{[]any{float64(2), float64(42), float64(42), float64(1), float64(3)}}) {
		t.Fatalf("export geometry=%v", chunk["positions"])
	}
	if _, ok := chunk["source_location"]; ok {
		t.Fatal("internal location leaked into export")
	}
	if _, ok := chunk["q_4_vec"]; ok {
		t.Fatal("embedding leaked into export")
	}
	if chunk["metadata"].(map[string]any)["source_location"] != "customer value" {
		t.Fatal("user metadata changed")
	}
	if row["source_location"] != l || row["q_4_vec"] == nil {
		t.Fatal("export mutated runtime state")
	}
}
