package component

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"gorm.io/gorm"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
)

// TestPDFVisionDispatchFinalizeHonorsRemoveHeaderFooter exercises the full
// maybeDispatchPDFVision normalize closure: MinerU JSON items keep a header block
// when the flag is off and drop it when the flag is on.
func TestPDFVisionDispatchFinalizeHonorsRemoveHeaderFooter(t *testing.T) {
	zipBytes := mineruZipWithHeaderAndBody(t)
	previous := resolveTenantOCRModelByProvider
	t.Cleanup(func() { resolveTenantOCRModelByProvider = previous })
	resolveTenantOCRModelByProvider = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &mineruTestDriver{}, "mineru", &modelModule.APIConfig{}, 0, nil
	}

	for _, tc := range []struct {
		name   string
		remove bool
		want   int
	}{
		{"off keeps header item", false, 2},
		{"on drops header item", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := schema.ParserSetup{
				"parse_method":         "mineru",
				"remove_header_footer": tc.remove,
			}
			items, err := mineruExtractJSONItems(zipBytes, setupRemoveHeaderFooter(setup))
			if err != nil {
				t.Fatalf("mineruExtractJSONItems: %v", err)
			}
			if len(items) != tc.want {
				t.Fatalf("extracted %d items, want %d", len(items), tc.want)
			}
			raw := mineruJSONParseResult("sample.pdf", items)
			var pdf parser.PDFParser
			pdf.ConfigureFromSetup(map[string]any(setup))
			out := pdf.FinalizePDFResult([]byte("%PDF-1.4"), pdf.NormalizeResult(t.Context(), "sample.pdf", raw))
			if len(out.JSON) != tc.want {
				t.Fatalf("final JSON len=%d, want %d", len(out.JSON), tc.want)
			}
		})
	}
}

func mineruZipWithHeaderAndBody(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[
		{"type":"text","text":"Body","page_idx":0},
		{"type":"header","text":"Running head","page_idx":0}
	]`))
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPDFVisionDispatchFinalizeHonorsRemoveTOC(t *testing.T) {
	setup := schema.ParserSetup{"remove_toc": true}
	items := []map[string]any{
		{"text": "Contents", "layout": "title", "page_number": 1},
		{"text": "Intro .......... 1", "layout": "text", "page_number": 1},
		{"text": "Chapter body text.", "layout": "text", "page_number": 2},
	}
	raw := parser.ParseResult{
		File: map[string]any{"name": "doc.pdf", "page_count": 2, "outline": []map[string]any{}},
		JSON: parser.MapsToItems(items),
	}
	var pdf parser.PDFParser
	pdf.ConfigureFromSetup(map[string]any(setup))
	out := pdf.FinalizePDFResult(nil, pdf.NormalizeResult(t.Context(), "doc.pdf", raw))
	for _, item := range out.JSON {
		text, _ := item.ToMap()["text"].(string)
		if text == "Contents" || text == "Intro .......... 1" {
			t.Fatalf("TOC entry still present: %q", text)
		}
	}
}
