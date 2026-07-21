//go:build cgo

package parser

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestDOCXParser_ParseWithResult_JSON(t *testing.T) {
	p := NewDOCXParser()
	p.outputFormat = "json"
	data := minimalDOCX(t, "Hello from JSON path")
	res := p.ParseWithResult("sample.docx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON items is empty; expected parsed content")
	}
	for i, item := range res.JSON {
		if _, ok := item["text"]; !ok {
			t.Errorf("item[%d] missing 'text' field", i)
		}
		if _, ok := item["doc_type_kwd"]; !ok {
			t.Errorf("item[%d] missing 'doc_type_kwd' field", i)
		}
	}
}

func TestDOCXParser_ConfigureFromSetup_JSON(t *testing.T) {
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})
	if p.outputFormat != "json" {
		t.Fatalf("After ConfigureFromSetup, outputFormat = %q, want %q", p.outputFormat, "json")
	}
	if p.libType != "" {
		t.Errorf("libType unexpectedly = %q", p.libType)
	}
	// Full round-trip: json config → json output
	data := minimalDOCX(t, "Config test")
	res := p.ParseWithResult("sample.docx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", res.OutputFormat, "json")
	}
	if len(res.JSON) == 0 {
		t.Error("JSON items is empty")
	}
}

func TestDOCXParser_ConfigureFromSetup_Markdown(t *testing.T) {
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "markdown"})
	if p.outputFormat != "markdown" {
		t.Fatalf("After ConfigureFromSetup, outputFormat = %q, want %q", p.outputFormat, "markdown")
	}
	data := minimalDOCX(t, "Config md test")
	res := p.ParseWithResult("sample.docx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "markdown" {
		t.Errorf("OutputFormat = %q, want %q", res.OutputFormat, "markdown")
	}
	if res.Markdown == "" {
		t.Error("Markdown is empty")
	}
}

func TestDOCXParser_ParseWithResult_CGOMinimalDocument(t *testing.T) {
	p := NewDOCXParser()
	data := minimalDOCX(t, "Hello from DOCX parser")
	res := p.ParseWithResult("sample.docx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if res.Markdown == "" {
		t.Fatal("Markdown is empty; want parsed content")
	}
}

func minimalDOCX(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)
	writeZipFile(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)
	writeZipFile(t, zw, "word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>`+text+`</w:t></w:r></w:p>
  </w:body>
</w:document>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func writeZipFile(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry %s: %v", name, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write zip entry %s: %v", name, err)
	}
}
