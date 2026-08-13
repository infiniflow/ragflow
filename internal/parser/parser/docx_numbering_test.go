package parser

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"
)

func numberedHeadingsDOCX(t *testing.T) []byte {
	t.Helper()
	parts := map[string]string{
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
  <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Общие сведения</w:t></w:r></w:p>
  <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Установка</w:t></w:r></w:p>
  <w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Начало работы</w:t></w:r></w:p>
  <w:p><w:pPr><w:numPr><w:ilvl w:val="2"/><w:numId w:val="42"/></w:numPr></w:pPr><w:r><w:t>Обычный пункт</w:t></w:r></w:p>
</w:body></w:document>`,
		"word/styles.xml": `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="42"/></w:numPr><w:outlineLvl w:val="0"/></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="42"/></w:numPr><w:outlineLvl w:val="1"/></w:pPr></w:style>
</w:styles>`,
		"word/numbering.xml": `<?xml version="1.0" encoding="UTF-8"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="7">
    <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1"/><w:pStyle w:val="Heading1"/></w:lvl>
    <w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/><w:pStyle w:val="Heading2"/></w:lvl>
    <w:lvl w:ilvl="2"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2.%3"/></w:lvl>
  </w:abstractNum>
  <w:num w:numId="42"><w:abstractNumId w:val="7"/></w:num>
</w:numbering>`,
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range parts {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := file.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close docx: %v", err)
	}
	return buffer.Bytes()
}

func TestExtractDOCXNumberedHeadings(t *testing.T) {
	got := extractDOCXNumberedHeadings(numberedHeadingsDOCX(t))
	want := []docxNumberedHeading{
		{Text: "Общие сведения", NumberedText: "1 Общие сведения", Level: 1},
		{Text: "Установка", NumberedText: "2 Установка", Level: 1},
		{Text: "Начало работы", NumberedText: "2.1 Начало работы", Level: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractDOCXNumberedHeadings mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestApplyDOCXNumberedHeadingsToSections(t *testing.T) {
	headings := extractDOCXNumberedHeadings(numberedHeadingsDOCX(t))
	items := []map[string]any{
		{"text": "Общие сведения\nУстановка\nНачало работы", "image": nil, "doc_type_kwd": "text"},
		{"text": "Обычный текст", "image": nil, "doc_type_kwd": "text"},
	}
	got := applyDOCXNumberedHeadingsToSections(items, headings)
	want := []map[string]any{
		{"text": "1 Общие сведения", "image": nil, "doc_type_kwd": "text", "ck_type": "heading"},
		{"text": "2 Установка", "image": nil, "doc_type_kwd": "text", "ck_type": "heading"},
		{"text": "2.1 Начало работы", "image": nil, "doc_type_kwd": "text", "ck_type": "heading"},
		{"text": "Обычный текст", "image": nil, "doc_type_kwd": "text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("applyDOCXNumberedHeadingsToSections mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestApplyDOCXNumberedHeadingsToMarkdown(t *testing.T) {
	headings := extractDOCXNumberedHeadings(numberedHeadingsDOCX(t))
	markdown := "1. Общие сведения\n2. Установка\n   1. Начало работы\n\nОбычный текст"
	want := "# 1 Общие сведения\n# 2 Установка\n## 2.1 Начало работы\n\nОбычный текст"
	if got := applyDOCXNumberedHeadingsToMarkdown(markdown, headings); got != want {
		t.Fatalf("markdown = %q, want %q", got, want)
	}
}

func TestFormatDOCXNumber(t *testing.T) {
	tests := []struct {
		name   string
		value  int
		format string
		want   string
	}{
		{name: "decimal", value: 12, format: "decimal", want: "12"},
		{name: "lower letter", value: 27, format: "lowerLetter", want: "aa"},
		{name: "upper roman", value: 14, format: "upperRoman", want: "XIV"},
		{name: "decimal zero", value: 3, format: "decimalZero", want: "03"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatDOCXNumber(test.value, test.format); got != test.want {
				t.Fatalf("formatDOCXNumber(%d, %q) = %q, want %q", test.value, test.format, got, test.want)
			}
		})
	}
}
