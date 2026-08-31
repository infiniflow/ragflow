package document

import "testing"

func TestNormalizeWebDocumentName(t *testing.T) {
	pdfBlob := []byte("%PDF-1.4 fake")
	htmlBlob := []byte("<html><body>hi</body></html>")
	cases := []struct {
		name, filename, ct string
		blob               []byte
		want               string
	}{
		{"pdf detected by blob", "report", "application/octet-stream", pdfBlob, "report.pdf"},
		{"html detected by blob", "page", "application/octet-stream", htmlBlob, "page.html"},
		{"dot stripped by utility, no type hint", "image.png", "application/octet-stream", []byte("plain"), "imagepng"},
		{"no hint, no extension", "doc", "application/json", []byte("{}"), "doc"},
	}
	for _, c := range cases {
		if got := normalizeWebDocumentName(c.filename, c.ct, c.blob); got != c.want {
			t.Errorf("%s: normalizeWebDocumentName = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUniqueUploadName(t *testing.T) {
	if got := uniqueUploadName("a.txt", map[string]bool{}); got != "a.txt" {
		t.Errorf("free name: got %q", got)
	}
	if got := uniqueUploadName("a.txt", map[string]bool{"a.txt": true}); got != "a(1).txt" {
		t.Errorf("single clash: got %q", got)
	}
	if got := uniqueUploadName("a.txt", map[string]bool{"a.txt": true, "a(1).txt": true}); got != "a(2).txt" {
		t.Errorf("double clash: got %q", got)
	}
	if got := uniqueUploadName("noext", map[string]bool{"noext": true}); got != "noext(1)" {
		t.Errorf("no-extension clash: got %q", got)
	}
}
