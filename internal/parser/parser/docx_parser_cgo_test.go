//go:build cgo

package parser

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestDOCXParser_ParseWithResult_CGOMinimalDocument(t *testing.T) {
	p, err := NewDOCXParser(OfficeOxide)
	if err != nil {
		t.Fatalf("NewDOCXParser: %v", err)
	}
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
