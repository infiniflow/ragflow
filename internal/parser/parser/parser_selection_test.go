//go:build cgo

package parser

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/xuri/excelize/v2"
	"ragflow/internal/utility"
)

func TestParserSelectionCorrectsFormat(t *testing.T) {
	type selector func(utility.FileType, []byte) (ParseResultProducer, utility.FileType, error)
	doc, err := os.ReadFile("testdata/table.doc")
	if err != nil {
		t.Fatal(err)
	}
	xlsx := newTestXLSX(t, func(f *excelize.File) { mustSetCell(t, f, "Sheet1", "A1", "Selected workbook") })
	xls, err := base64.StdEncoding.DecodeString(genuineBIFF8FixtureBase64)
	if err != nil {
		t.Fatal(err)
	}
	for mode, selectParser := range map[string]selector{"ingestion": GetParser, "text": GetTextOnlyParser} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name            string
				requested, want utility.FileType
				data            []byte
			}{
				{"doc_as_docx", utility.FileTypeDOCX, utility.FileTypeDOC, doc},
				{"docx_as_doc", utility.FileTypeDOC, utility.FileTypeDOCX, minimalDOCX(t, "Selected document")},
				{"xlsx_as_xls", utility.FileTypeXLS, utility.FileTypeXLSX, xlsx},
				{"xls_as_xlsx", utility.FileTypeXLSX, utility.FileTypeXLS, xls},
				{"pptx_as_ppt", utility.FileTypePPT, utility.FileTypePPTX, buildPPTX(t, "Selected presentation")},
				{"unknown", utility.FileTypeDOC, utility.FileTypeDOC, []byte("unrecognized")},
				{"unchecked_text", utility.FileTypeTXT, utility.FileTypeTXT, []byte("plain text")},
			} {
				t.Run(tc.name, func(t *testing.T) {
					p, actual, err := selectParser(tc.requested, tc.data)
					if err != nil {
						t.Fatal(err)
					}
					if actual != tc.want {
						t.Fatalf("format=%q, want %q", actual, tc.want)
					}
					res := p.ParseWithResult(t.Context(), "original."+string(tc.requested), tc.data)
					if tc.name == "unknown" {
						if res.Err == nil {
							t.Fatal("unrecognized document should still fail in the original parser")
						}
						return
					}
					if res.Err != nil {
						t.Fatal(res.Err)
					}
					if !res.Produced() {
						t.Fatal("selected parser produced no result")
					}
					if actual != utility.FileTypeTXT && res.File["format"] != string(actual) {
						t.Fatalf("result format=%v, selected type=%s", res.File["format"], actual)
					}
				})
			}
		})
	}
}

func TestOfficeParsersDoNotChangeFormat(t *testing.T) {
	doc, err := os.ReadFile("testdata/table.doc")
	if err != nil {
		t.Fatal(err)
	}
	if res := NewDOCXParser().ParseWithResult(t.Context(), "renamed.docx", doc); res.Err == nil {
		t.Fatal("DOCX parser silently accepted DOC")
	}
	xlsx := newTestXLSX(t, func(f *excelize.File) { mustSetCell(t, f, "Sheet1", "A1", "workbook") })
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatal(err)
	}
	if res := p.ParseWithResult(t.Context(), "renamed.xls", xlsx); res.Err == nil {
		t.Fatal("XLS parser silently accepted XLSX")
	}
}

func TestCorrectedTextOnlyParserSkipsMedia(t *testing.T) {
	data, err := os.ReadFile("testdata/docx_table_image.docx")
	if err != nil {
		t.Fatal(err)
	}
	p, actual, err := GetTextOnlyParser(utility.FileTypeDOC, data)
	if err != nil {
		t.Fatal(err)
	}
	if actual != utility.FileTypeDOCX {
		t.Fatalf("actual=%q", actual)
	}
	res := p.ParseWithResult(t.Context(), "renamed.doc", data)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("no extracted contents")
	}
	for _, item := range res.JSON {
		if item.ToMap()["doc_type_kwd"] == "image" {
			t.Fatal("text-only selection extracted an image")
		}
	}
}
