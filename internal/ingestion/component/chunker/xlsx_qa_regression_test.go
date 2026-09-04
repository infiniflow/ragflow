package chunker

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
	"ragflow/internal/parser/parser"
)

func TestXLSXQARegression(t *testing.T) {
	f := excelize.NewFile()
	sh := f.GetSheetName(0)
	for i, r := range [][]string{
		{"question", "answer"},
		{"What is RAGFlow?", "A RAG engine."},
		{"Where are the docs?", "On the website."},
	} {
		for j, c := range r {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			_ = f.SetCellValue(sh, cell, c)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	p, err := parser.NewXLSXParser("")
	if err != nil {
		t.Fatal(err)
	}
	res := p.ParseWithResult(context.Background(), "qa.xlsx", buf.Bytes())
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	inputs := map[string]any{"name": "qa.xlsx", "output_format": res.OutputFormat}
	switch res.OutputFormat {
	case "json":
		inputs["json"] = res.JSON
	case "html":
		inputs["html"] = res.HTML
	}

	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	t.Logf("format=%q QA chunks=%d", res.OutputFormat, len(chunks))
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
}
