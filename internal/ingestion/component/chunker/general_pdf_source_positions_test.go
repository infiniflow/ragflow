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

func TestPDFSourceOffsetsSkipCustomDelimiter(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		text       string
		wantTexts  []string
		wantBounds [][2]float64
	}{
		{"delimiter substring", "a###", []string{"a", "#"}, [][2]float64{{0, 25}, {75, 100}}},
		{"leading and repeated", "##a####b", []string{"a", "b"}, [][2]float64{{25, 37.5}, {87.5, 100}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			unit := schema.ChunkDoc{Text: testCase.text, DocType: "text", Positions: json.RawMessage(`[[1,10,200,0,100]]`)}
			pieces, sources := splitPDFUnits([]schema.ChunkDoc{unit}, pdfUnitSources([]schema.ChunkDoc{unit}), compileDelimPattern([]string{"`##`"}), false)
			if len(pieces) != len(testCase.wantTexts) {
				t.Fatalf("pieces = %v, want %v", generalChunkTexts(pieces), testCase.wantTexts)
			}
			for index, piece := range pieces {
				if piece.Text != testCase.wantTexts[index] {
					t.Errorf("piece %d text = %q, want %q", index, piece.Text, testCase.wantTexts[index])
				}
				_, raw := pdfSourcePositions(sources[index])
				positions := decodeGeneralMatrix(t, raw)
				if len(positions) != 1 || positions[0][3] != testCase.wantBounds[index][0] || positions[0][4] != testCase.wantBounds[index][1] {
					t.Errorf("piece %d positions = %v, want bounds %v", index, positions, testCase.wantBounds[index])
				}
			}
		})
	}
}

func TestGeneralPDFCustomDelimiterPreservesSourceBounds(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512, "delimiters": []string{"`##`"}, "children_delimiters": []string{"。"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{
		"name": "source.pdf", "file_type": "pdf", "output_format": "json",
		"json": []map[string]any{{"text": "a###", "doc_type_kwd": "text", "positions": [][]float64{{1, 10, 200, 0, 100}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 || chunks[0]["text"] != "a\n#" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	raw, err := json.Marshal(chunks[0]["positions"])
	if err != nil {
		t.Fatal(err)
	}
	var got [][]float64
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := [][]float64{{1, 10, 200, 0, 25}, {1, 10, 200, 75, 100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func TestPDFSourceOffsetsIgnorePositionTags(t *testing.T) {
	unit := schema.ChunkDoc{
		Text: "a@@1\t2\t3\t4\t5##xyyb", DocType: "text",
		Positions: json.RawMessage(`[[1,10,200,0,100]]`),
	}
	pieces, sources := splitPDFUnits([]schema.ChunkDoc{unit}, pdfUnitSources([]schema.ChunkDoc{unit}), compileDelimPattern([]string{"`xy`"}), false)
	if len(pieces) != 2 || removeTag(pieces[1].Text) != "yb" {
		t.Fatalf("unexpected pieces: %v", generalChunkTexts(pieces))
	}
	_, raw := pdfSourcePositions(sources[1])
	got := decodeGeneralMatrix(t, raw)
	want := [][]float64{{1, 10, 200, 60, 100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func TestPDFSourceOffsetsWithWhitespaceDelimiters(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		text      string
		delimiter string
		bounds    [][2]float64
	}{
		{"repeated spaces", "a  b", "` `", [][2]float64{{0, 25}, {75, 100}}},
		{"repeated tabs", "a\t\tb", "`\t`", [][2]float64{{0, 25}, {75, 100}}},
		{"spaces in delimiter", "a / b", "` / `", [][2]float64{{0, 20}, {80, 100}}},
		{"position tag", "a@@1\t2\t3\t4\t5##  b", "` `", [][2]float64{{0, 25}, {75, 100}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			unit := schema.ChunkDoc{Text: testCase.text, DocType: "text", Positions: json.RawMessage(`[[1,10,200,0,100]]`)}
			pieces, sources := splitPDFUnits([]schema.ChunkDoc{unit}, pdfUnitSources([]schema.ChunkDoc{unit}), compileDelimPattern([]string{testCase.delimiter}), false)
			if len(pieces) != 2 || removeTag(pieces[0].Text) != "a" || removeTag(pieces[1].Text) != "b" {
				t.Fatalf("unexpected pieces: %v", generalChunkTexts(pieces))
			}
			for index, want := range testCase.bounds {
				_, raw := pdfSourcePositions(sources[index])
				positions := decodeGeneralMatrix(t, raw)
				if len(positions) != 1 || positions[0][3] != want[0] || positions[0][4] != want[1] {
					t.Errorf("piece %d positions = %v, want bounds %v", index, positions, want)
				}
			}
		})
	}
}

func TestGeneralPDFWrappedChildExcludesPreviousParagraph(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512, "delimiters": []string{"#"}, "children_delimiters": []string{"#"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		{"text": "上一段很长，不能算进下一段。", "doc_type_kwd": "text", "_pdf_positions": [][]float64{{1, 10, 200, 10, 40}}},
		{"text": "#", "doc_type_kwd": "text", "_pdf_positions": [][]float64{{1, 10, 20, 45, 55}}},
		{"text": "将军向宠，性行淑均，\n晓畅军事。\n愚以为营中之事。", "doc_type_kwd": "text", "_pdf_positions": [][]float64{{1, 10, 200, 60, 100}}},
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{"name": "source.pdf", "file_type": "pdf", "output_format": "json", "json": items})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	raw, err := json.Marshal(chunks[1]["_pdf_positions"])
	if err != nil {
		t.Fatal(err)
	}
	var got [][]float64
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := [][]float64{{1, 10, 200, 60, 100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrapped child positions = %v, want %v", got, want)
	}
}
