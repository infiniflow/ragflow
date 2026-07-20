//go:build cgo

package parser

import (
	"os"
	"testing"
)

func TestPPTParser_ParseWithResult_CGO(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.ppt")
	if err != nil {
		t.Skip("testdata/sample.ppt not available:", err)
	}
	p := NewPPTParser()
	res := p.ParseWithResult("sample.ppt", data)
	if res.Err != nil {
		t.Fatalf("PPTParser.ParseWithResult: expected no error, got %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want %q", res.OutputFormat, "json")
	}
	if got := res.File["format"]; got != "ppt" {
		t.Fatalf("File[format] = %v, want %q", got, "ppt")
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON items is empty; expected at least one slide")
	}
	for i, item := range res.JSON {
		text, ok := item["text"].(string)
		if !ok || text == "" {
			t.Fatalf("JSON[%d][text] missing or empty", i)
		}
	}
}
