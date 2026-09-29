package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/parser/parser"
)

func TestCSVQAFirstRowIsData(t *testing.T) {
	p := parser.NewCSVParser()
	res := p.ParseWithResult(context.Background(), "qa.csv", []byte("q1,a1\nq2,a2\n"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "qa.csv",
		"file_type":     "csv",
		"output_format": res.OutputFormat,
		"json":          res.JSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected both QA rows, got %d chunks: %#v", len(chunks), chunks)
	}
	if got, _ := chunks[0]["text"].(string); got != "Question: q1\tAnswer: a1" {
		t.Fatalf("first row was not emitted as QA data: %q", got)
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
	chunks, _ := out["chunks"].([]map[string]any)
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
		"json": []map[string]any{{
			"doc_type_kwd": "table",
			"text": "<table><tr><td>q1</td><td>a1</td></tr>" +
				"<tr><td>q2</td><td>a2</td><td>trailing</td></tr></table>",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("malformed HTML row created a QA pair: %#v", chunks)
	}
	got, _ := chunks[0]["text"].(string)
	if !strings.Contains(got, "Answer: a1\nq2,a2,trailing") {
		t.Fatalf("malformed HTML row was not folded into the previous answer: %q", got)
	}
}
