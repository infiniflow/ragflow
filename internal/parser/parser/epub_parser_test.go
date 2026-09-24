package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// buildEPUB writes a minimal EPUB whose OPF lives at OEBPS/content.opf and
// lists manifestHrefs in spine order; entries maps ZIP entry names to text.
func buildEPUB(t *testing.T, manifestHrefs []string, entries map[string]string) []byte {
	t.Helper()
	var items, refs strings.Builder
	for i, href := range manifestHrefs {
		fmt.Fprintf(&items, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i, href)
		fmt.Fprintf(&refs, `<itemref idref="c%d"/>`, i)
	}
	files := map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
		"OEBPS/content.opf":      `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><manifest>` + items.String() + `</manifest><spine>` + refs.String() + `</spine></package>`,
	}
	for name, text := range entries {
		files[name] = `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>` + text + `</p></body></html>`
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
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

// Manifest hrefs are URLs, so non-ASCII and space file names are
// percent-encoded and may carry a fragment.
func TestEPUBParser_ResolvesManifestHrefsAsURLs(t *testing.T) {
	data := buildEPUB(t,
		[]string{"Text/%E7%AC%AC%E4%B8%80%E7%AB%A0.xhtml", "Text/chapter%202.xhtml#start"},
		map[string]string{
			"OEBPS/Text/第一章.xhtml":       "ALPHA chapter",
			"OEBPS/Text/chapter 2.xhtml": "BRAVO chapter",
		})

	res := NewEPUBParser().ParseWithResult(context.Background(), "book.epub", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var combined strings.Builder
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		combined.WriteString(text + "\n")
	}
	for _, want := range []string{"ALPHA chapter", "BRAVO chapter"} {
		if !strings.Contains(combined.String(), want) {
			t.Errorf("missing %q in %q", want, combined.String())
		}
	}
}
