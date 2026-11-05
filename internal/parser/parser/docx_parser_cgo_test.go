//go:build cgo

package parser

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestDOCXParser_ParseWithResult_JSON(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()
	data := minimalDOCX(t, "Hello from JSON path")
	res := p.ParseWithResult(ctx, "sample.docx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON items is empty; expected parsed content")
	}
	for i, item := range ItemsToMaps(res.JSON) {
		if _, ok := item["text"]; !ok {
			t.Errorf("item[%d] missing 'text' field", i)
		}
		if _, ok := item["doc_type_kwd"]; !ok {
			t.Errorf("item[%d] missing 'doc_type_kwd' field", i)
		}
	}
}

func TestDOCXParser_ConfigureFromSetup_Options(t *testing.T) {
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"remove_toc": true, "remove_header_footer": true})
	if !p.RemoveTOC {
		t.Error("RemoveTOC not configured")
	}
	if !p.RemoveHeaderFooter {
		t.Error("RemoveHeaderFooter not configured")
	}
	if p.libType != "" {
		t.Errorf("libType unexpectedly = %q", p.libType)
	}
}

func TestDOCXParser_ParseWithResult_CGOMinimalDocument(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()
	data := minimalDOCX(t, "Hello from DOCX parser")
	res := p.ParseWithResult(ctx, "sample.docx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON items is empty; want parsed content")
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

// TestDOCXParser_PreservesXMLCharacterReferences pins the office_oxide version.
// v0.1.9 deleted every XML character reference in DOCX run text instead of
// decoding it, so <w:t>&lt;SEP&gt;</w:t> parsed as "SEP". That reached the
// chunker intact, so a delimiter configured as "`<SEP>`" could never match and
// the marker survived into the chunk body. v0.1.10 was the correctness release
// that fixed this defect class; v0.1.12 is the pinned version.
func TestDOCXParser_PreservesXMLCharacterReferences(t *testing.T) {
	cases := []struct {
		name string
		run  string // run text as it appears inside <w:t>, entities unescaped
		want string
	}{
		{"named", "&lt;SEP&gt;", "<SEP>"},
		{"decimal", "&#60;SEP&#62;", "<SEP>"},
		{"hex", "&#x3C;SEP&#x3E;", "<SEP>"},
		{"ampersand", "A&amp;B", "A&B"},
		{"apostrophe", "it&apos;s", "it's"},
		{"emdash", "a&#8212;b", "a—b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := minimalDOCX(t, tc.run)

			p := NewDOCXParser()
			res := p.ParseWithResult(t.Context(), "entities.docx", data)
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if got := joinItemTexts(ItemsToMaps(res.JSON)); !strings.Contains(got, tc.want) {
				t.Errorf("json text = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func joinItemTexts(items []map[string]any) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}
