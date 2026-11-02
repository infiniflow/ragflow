package chunker

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestGeneralPDFChildPositionsFollowSourceText(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":    512,
		"delimiters":          []string{"#"},
		"children_delimiters": []string{"#"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		{"text": "上一段的长长长长长长长长长长长长长长长长长长长长长长长长长长长句。", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 200, 10, 40}}},
		{"text": "#", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 20, 45, 55}}},
		{"text": "本段第一句。", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 200, 60, 75}}},
		{"text": "本段第二句。", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 200, 80, 95}}},
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{"name": "source.pdf", "file_type": "pdf", "output_format": "json", "json": items})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2: %#v", len(chunks), chunks)
	}
	for index, want := range [][][]float64{
		{{1, 10, 200, 10, 40}, {1, 10, 20, 45, 55}},
		{{1, 10, 200, 60, 75}, {1, 10, 200, 80, 95}},
	} {
		raw, err := json.Marshal(chunks[index]["positions"])
		if err != nil {
			t.Fatal(err)
		}
		var got [][]float64
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("chunk %d positions = %v, want %v", index, got, want)
		}
	}
}

func TestGeneralPDFOverlapChildPositionsIncludeSourceTail(t *testing.T) {
	firstPosition := json.RawMessage(`[[1,10,20,0,100]]`)
	secondPosition := json.RawMessage(`[[1,10,20,200,300]]`)
	chunks := []schema.ChunkDoc{
		{Text: "ABCDE", DocType: "text", Positions: firstPosition},
		{Text: "FGHIJ", DocType: "text", Positions: secondPosition},
	}
	sources := [][]pdfSourceSpan{{{start: 0, end: 5, positions: firstPosition}}, {{start: 0, end: 5, positions: secondPosition}}}
	chunks, sources = applyGeneralOverlapWithSources(chunks, sources, 40, "\n")
	children := splitGeneralChildrenWithSources(chunks, sources, compileDelimPattern([]string{"#"}))
	if len(children) != 2 || children[1].Text != "DE\nFGHIJ" {
		t.Fatalf("unexpected overlapped children: %#v", children)
	}
	got := decodeGeneralMatrix(t, children[1].Positions)
	want := [][]float64{{1, 10, 20, 60, 100}, {1, 10, 20, 200, 300}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("overlap positions = %v, want %v", got, want)
	}
}

func TestGeneralPDFChildrenWithinOneSourceUseProportionalPositions(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":    512,
		"delimiters":          []string{"#"},
		"children_delimiters": []string{"#"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{
		"name": "source.pdf", "file_type": "pdf", "output_format": "json",
		"json": []map[string]any{{"text": "one#two", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 20, 0, 70}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	for index, want := range [][][]float64{{{1, 10, 20, 0, 40}}, {{1, 10, 20, 40, 70}}} {
		raw, err := json.Marshal(chunks[index]["positions"])
		if err != nil {
			t.Fatal(err)
		}
		var got [][]float64
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("chunk %d positions = %v, want %v", index, got, want)
		}
	}
}

func TestGeneralPDFChildPositionsFollowPageOrder(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "delimiters": []string{"#"}, "children_delimiters": []string{"#"}})
	if err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		{"text": "second page", "doc_type_kwd": "text", "positions": [][]float64{{2, 10, 100, 10, 20}}},
		{"text": "first page", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 100, 10, 20}}},
		{"text": "#", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 20, 25, 35}}},
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{"name": "source.pdf", "file_type": "pdf", "output_format": "json", "json": items})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2: %#v", len(chunks), chunks)
	}
	for index, want := range [][][]float64{{{1, 10, 100, 10, 20}, {1, 10, 20, 25, 35}}, {{2, 10, 100, 10, 20}}} {
		raw, err := json.Marshal(chunks[index]["positions"])
		if err != nil {
			t.Fatal(err)
		}
		var got [][]float64
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("chunk %d positions = %v, want %v", index, got, want)
		}
	}
}
