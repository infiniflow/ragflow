//go:build cgo && manual

package pdfium

import "testing"

const minimalPDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R>>endobj\nxref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \ntrailer<</Size 4/Root 1 0 R>>\nstartxref\n190\n%%EOF"

func TestDocumentRejectsUseAfterClose(t *testing.T) {
	doc, err := OpenDocument([]byte(minimalPDF))
	if err != nil {
		t.Fatal(err)
	}
	doc.Close()

	if _, _, err := doc.PageSize(0); err == nil {
		t.Fatal("expected PageSize to reject a closed document")
	}
	if _, err := doc.RenderPage(0, 72); err == nil {
		t.Fatal("expected RenderPage to reject a closed document")
	}
}

func TestDocumentsKeepIndependentPDFData(t *testing.T) {
	first, err := OpenDocument([]byte(minimalPDF))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	secondData := []byte(minimalPDF)
	for i := range len(secondData) - 2 {
		if string(secondData[i:i+3]) == "612" {
			copy(secondData[i:i+3], "712")
			break
		}
	}
	second, err := OpenDocument(secondData)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	firstWidth, _, err := first.PageSize(0)
	if err != nil {
		t.Fatalf("first document became invalid after opening second: %v", err)
	}
	secondWidth, _, err := second.PageSize(0)
	if err != nil {
		t.Fatalf("second document: %v", err)
	}
	if firstWidth != 612 || secondWidth != 712 {
		t.Fatalf("got widths %.0f and %.0f, want 612 and 712", firstWidth, secondWidth)
	}
}
