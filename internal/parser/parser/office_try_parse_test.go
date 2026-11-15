package parser

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"

	"ragflow/internal/utility"
)

func TestOfficeTryParse(t *testing.T) {
	docx := officeProbeZIP(t, "[Content_Types].xml", "word/document.xml")
	xlsx := officeProbeZIP(t, "[Content_Types].xml", "xl/workbook.xml")
	pptx := officeProbeZIP(t, "[Content_Types].xml", "ppt/presentation.xml")
	ordinaryZIP := officeProbeZIP(t, "notes.txt")
	missingContentTypes := officeProbeZIP(t, "word/document.xml")
	ambiguousZIP := officeProbeZIP(t, "[Content_Types].xml", "word/document.xml", "xl/workbook.xml")
	doc, err := os.ReadFile("testdata/table.doc")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want utility.FileType
	}{
		{"docx", docx, utility.FileTypeDOCX},
		{"xlsx", xlsx, utility.FileTypeXLSX},
		{"pptx", pptx, utility.FileTypePPTX},
		{"doc", doc, utility.FileTypeDOC},
		{"ordinary_zip", ordinaryZIP, utility.FileTypeOTHER},
		{"missing_content_types", missingContentTypes, utility.FileTypeOTHER},
		{"ambiguous_zip", ambiguousZIP, utility.FileTypeOTHER},
		{"truncated_zip", []byte("PK\x03\x04"), utility.FileTypeOTHER},
		{"truncated_ole", CompoundDocumentSignature, utility.FileTypeOTHER},
		{"unknown", []byte("plain text"), utility.FileTypeOTHER},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tryParseOffice(tc.data); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func officeProbeZIP(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("<document/>")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOfficeParsersSupportTryParse(t *testing.T) {
	for _, ft := range []utility.FileType{utility.FileTypeDOC, utility.FileTypeDOCX, utility.FileTypeXLS, utility.FileTypeXLSX, utility.FileTypePPT, utility.FileTypePPTX} {
		p, err := newParser(ft, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := p.(TryParser); !ok {
			t.Fatalf("%s parser does not support TryParse", ft)
		}
	}
}
