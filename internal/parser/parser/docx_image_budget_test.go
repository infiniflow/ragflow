package parser

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// docxIRWithImages builds an office_oxide IR JSON holding one small embedded
// image per count, each separated by a paragraph so the walker treats every
// image as its own element.
func docxIRWithImages(count int) string {
	var b strings.Builder
	b.WriteString(`{"sections":[{"elements":[`)
	for i := range count {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"type":"paragraph","content":[{"type":"text","text":"p%d"}]},`, i)
		fmt.Fprintf(&b, `{"type":"image","data":"%s"}`,
			base64.StdEncoding.EncodeToString([]byte("im")))
	}
	b.WriteString(`]}`)
	b.WriteString(`]}`)
	return b.String()
}

// TestDOCXParseImageHeavyDocumentKeepsEveryImage pins that the document image
// budget bounds the chunk payloads, not the same images twice.
//
// The budget caps a document at maxEmbeddedMediaItems image payloads. The
// figure metadata walk used to draw on the very same allowance for the very
// same images, so a document with more than half the cap in images lost every
// image past that point: the section walk stops when the budget reports the
// document limit, so those images never reached a chunk and the UI showed an
// image chunk with no picture to upload.
func TestDOCXParseImageHeavyDocumentKeepsEveryImage(t *testing.T) {
	const images = maxEmbeddedMediaItems/2 + 10
	irJSON := docxIRWithImages(images)

	orig := docxExtract
	t.Cleanup(func() { docxExtract = orig })
	docxExtract = func([]byte, string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	res := NewDOCXParser().ParseWithResult(t.Context(), "image-heavy.docx", []byte("not a real docx"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var withPayload, omitted int
	for _, item := range res.JSON {
		m := item.ToMap()
		if dt, _ := m["doc_type_kwd"].(string); dt != "image" {
			continue
		}
		if img, _ := m["image"].(string); img != "" {
			withPayload++
			continue
		}
		omitted++
	}
	if withPayload != images {
		t.Fatalf("image items with a payload = %d, want %d (payload-less: %d)", withPayload, images, omitted)
	}
}
