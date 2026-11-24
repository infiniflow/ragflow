package chunker

import (
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

func TestDelimitedQAFirstRowIsData(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, data, first string }{
		{"qa.csv", "q1,a1\nq2,a2\n", "Question: q1\tAnswer: a1"},
		{"qa.TSV", "where, exactly?\tParis, France\nwhen, exactly?\tMay, 2026\n", "Question: where, exactly?\tAnswer: Paris, France"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.data)
			p, fileType, err := parser.GetParser(utility.GetFileType(tc.name), data)
			if err != nil {
				t.Fatal(err)
			}
			res := p.ParseWithResult(t.Context(), tc.name, data)
			if res.Err != nil {
				t.Fatal(res.Err)
			}
			out, err := comp.Invoke(t.Context(), nil, map[string]any{
				"name":          tc.name,
				"file_type":     string(fileType),
				"output_format": "json",
				"json":          res.JSON,
			})
			if err != nil {
				t.Fatal(err)
			}
			chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
			if len(chunks) != 2 || chunks[0]["text"] != tc.first {
				t.Fatalf("want both QA rows starting with %q, got %#v", tc.first, chunks)
			}
		})
	}
}

func TestCSVTypedRowsFoldMalformedRowIntoPreviousAnswer(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "qa.csv",
		"file_type":     "csv",
		"output_format": "json",
		"json": []map[string]any{spreadsheetSegmentItem("Sheet1",
			[]string{"q1", "a1"},
			[][]string{{"question", "", "extra"}}, 1, 2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("malformed row created or removed a pair: %#v", chunks)
	}
	got, _ := chunks[0]["text"].(string)
	if !strings.Contains(got, "Answer: a1\nquestion,,extra") {
		t.Fatalf("malformed row was not folded into the previous answer: %q", got)
	}
}

func TestCSVJSONHTMLTableUsesStrictContinuationRules(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "qa.csv",
		"file_type":     "csv",
		"output_format": "json",
		"json": []map[string]any{
			tableItem(t, "<table><tr><td>q1</td><td>a1</td></tr>"+
				"<tr><td>q2</td><td>a2</td><td>trailing</td></tr></table>"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("malformed HTML row created a QA pair: %#v", chunks)
	}
	got, _ := chunks[0]["text"].(string)
	if !strings.Contains(got, "Answer: a1\nq2,a2,trailing") {
		t.Fatalf("malformed HTML row was not folded into the previous answer: %q", got)
	}
}
