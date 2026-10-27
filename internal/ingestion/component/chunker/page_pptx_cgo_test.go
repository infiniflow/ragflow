//go:build cgo

package chunker

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	officeOxide "github.com/yfedoseev/office_oxide/go"
	"ragflow/internal/parser/parser"
)

func TestPageChunkerPPTXTableEndToEnd(t *testing.T) {
	writer := officeOxide.NewPptxWriter()
	defer writer.Close()
	slide := writer.AddSlide()
	writer.SetSlideTitle(slide, "Revenue 2026")
	writer.AddSlideText(slide, "Revenue rose")
	original, err := writer.ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range zr.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if file.Name == "ppt/slides/slide1.xml" {
			table := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="50" name="Table 1"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm><a:off x="0" y="0"/><a:ext cx="4000000" cy="1000000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr/><a:tblGrid><a:gridCol w="2000000"/><a:gridCol w="2000000"/></a:tblGrid><a:tr h="400000"><a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Q1</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc><a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>100</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
			source := string(data)
			if !strings.Contains(source, "</p:sp>") {
				t.Fatalf("slide does not contain shapes: %s", source)
			}
			data = []byte(strings.Replace(source, "</p:sp>", "</p:sp>"+table, 1))
		}
		w, err := zw.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	parsed := parser.NewPPTXParser().ParseWithResult(t.Context(), "deck.pptx", buf.Bytes())
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	chunks := slideChunksOf(t, map[string]any{"name": "deck.pptx", "file_type": "pptx", "output_format": "json", "json": parsed.JSON})
	if len(chunks) != 3 {
		t.Fatalf("items=%#v chunks=%#v", parser.ItemsToMaps(parsed.JSON), chunks)
	}
	text, _ := chunks[1]["text"].(string)
	if chunks[0]["text"] != "Revenue 2026" || chunks[1]["doc_type_kwd"] != "table" || chunks[1]["table"] == nil || !strings.Contains(text, "<th>Q1</th><th>100</th>") || chunks[2]["text"] != "Revenue rose" {
		t.Fatalf("PPTX slide contents lost: items=%#v chunks=%#v", parser.ItemsToMaps(parsed.JSON), chunks)
	}
}
