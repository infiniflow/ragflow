package parser

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildSampleDOCX constructs a minimal but valid .docx (ZIP/OOXML) in memory
// from the given body children of <w:body>. The caller supplies the inner XML
// of <w:body> (paragraphs and tables). A default styles.xml is included so the
// document is well-formed; style names are referenced by w:pStyle w:val.
func buildSampleDOCX(t *testing.T, bodyInner string) []byte {
	t.Helper()
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` + bodyInner + `</w:body></w:document>`
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="caption"/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="题注"><w:name w:val="caption"/></w:style>` +
		`</w:styles>`
	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`</Types>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`</Relationships>`
	docRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`</Relationships>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":          contentTypes,
		"_rels/.rels":                  rels,
		"word/document.xml":            document,
		"word/styles.xml":              styles,
		"word/_rels/document.xml.rels": docRels,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func docxScanPara(styleID, text string) string {
	return `<w:p xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:pPr><w:pStyle w:val="` + styleID + `"/></w:pPr>` +
		`<w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func plainPara(text string) string {
	return `<w:p xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func docxScanTable() string {
	return `<w:tbl xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
}

// A Caption-styled paragraph immediately above a table is recovered.
func TestExtractDOCXTableCaptions_CaptionBeforeTable(t *testing.T) {
	doc := buildSampleDOCX(t,
		docxScanPara("Heading1", "设备台账")+
			docxScanPara("Caption", "设备流转明细")+
			docxScanTable()+
			plainPara("普通正文"))
	caps, err := extractDOCXTableCaptions(doc)
	if err != nil {
		t.Fatalf("extractDOCXTableCaptions: %v", err)
	}
	if len(caps) != 1 {
		t.Fatalf("want 1 table caption, got %d: %v", len(caps), caps)
	}
	if caps[0] != "设备流转明细" {
		t.Errorf("caption = %q, want %q", caps[0], "设备流转明细")
	}
}

// The Chinese caption style "题注" is detected the same way.
func TestExtractDOCXTableCaptions_ChineseCaption(t *testing.T) {
	doc := buildSampleDOCX(t, docxScanPara("题注", "设备明细表")+docxScanTable())
	caps, err := extractDOCXTableCaptions(doc)
	if err != nil {
		t.Fatalf("extractDOCXTableCaptions: %v", err)
	}
	if len(caps) != 1 || caps[0] != "设备明细表" {
		t.Fatalf("caption = %v, want [设备明细表]", caps)
	}
}

// A non-caption paragraph between the caption and the table detaches it.
func TestExtractDOCXTableCaptions_CaptionNotImmediate(t *testing.T) {
	doc := buildSampleDOCX(t,
		docxScanPara("Caption", "设备流转明细")+
			plainPara("中间说明文字")+
			docxScanTable())
	caps, err := extractDOCXTableCaptions(doc)
	if err != nil {
		t.Fatalf("extractDOCXTableCaptions: %v", err)
	}
	if len(caps) != 1 || caps[0] != "" {
		t.Fatalf("caption = %v, want [\"\"]", caps)
	}
}

// Multiple tables get the caption of their own preceding caption paragraph.
func TestExtractDOCXTableCaptions_MultipleTables(t *testing.T) {
	doc := buildSampleDOCX(t,
		docxScanPara("Caption", "第一表")+docxScanTable()+
			plainPara("分隔")+
			docxScanPara("Caption", "第二表")+docxScanTable())
	caps, err := extractDOCXTableCaptions(doc)
	if err != nil {
		t.Fatalf("extractDOCXTableCaptions: %v", err)
	}
	if len(caps) != 2 || caps[0] != "第一表" || caps[1] != "第二表" {
		t.Fatalf("captions = %v, want [第一表 第二表]", caps)
	}
}
