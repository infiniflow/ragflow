//go:build cgo

package document

import (
	"os"
	"strings"
	"testing"
)

func TestDOCPreviewCorrectsDOCXContent(t *testing.T) {
	data, err := os.ReadFile("../../parser/parser/testdata/docx_table_image.docx")
	if err != nil {
		t.Fatal(err)
	}
	text, err := extractDOCPreviewText(t.Context(), "renamed.doc", data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(text) == "" || text == string(data) {
		t.Fatal("preview did not extract corrected DOCX content")
	}
}
