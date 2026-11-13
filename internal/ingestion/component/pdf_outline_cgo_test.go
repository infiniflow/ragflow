//go:build cgo

package component

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

// TestPDFDispatchAttachesPDFOutlines pins the outline attachment on the
// ingestion dispatch boundary: the tenant parser serves content only, so the
// bookmarks must come from the source bytes the component already holds.
// Reverting the call in maybeDispatchPDFVision's normalize closure fails here
// and nowhere else, because the built-in parse methods have their own boundary.
func TestPDFDispatchAttachesPDFOutlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		archive := zip.NewWriter(&buf)
		file, _ := archive.Create("sample/sample.json")
		_ = json.NewEncoder(file).Encode(map[string]any{"layouts": []any{map[string]any{"label": "Text", "content": "body"}}})
		_ = archive.Close()
		_, _ = w.Write(buf.Bytes())
	}))
	defer server.Close()
	previousResolver := resolveTenantOCRModelByProvider
	t.Cleanup(func() { resolveTenantOCRModelByProvider = previousResolver })
	resolveTenantOCRModelByProvider = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &monkeyOCRv2FakeDriver{}, "monkey", &modelModule.APIConfig{}, 0, nil
	}
	setup := schema.ParserSetup{"parse_method": "monkeyocrv2", "monkeyocrv2_server_url": server.URL}

	result, handled, err := maybeDispatchPDFVision(t.Context(), nil, utility.FileTypePDF, "bookmarked.pdf", singleBookmarkPDF(t), nil, map[string]schema.ParserSetup{"pdf": setup})
	if err != nil || !handled || !result.Produced() {
		t.Fatalf("dispatch failed: handled=%v err=%v", handled, err)
	}
	outline, ok := result.File["outline"].([]map[string]any)
	if !ok || len(outline) != 1 || outline[0]["title"] != "Chapter 1" {
		t.Fatalf("File[outline] = %#v, want the bookmark read from the source PDF", result.File["outline"])
	}
}

// singleBookmarkPDF builds the smallest PDF that carries one bookmark, so the
// dispatch boundary has real bytes to extract from.
func singleBookmarkPDF(t *testing.T) []byte {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /Outlines 3 0 R >>",
		"<< /Type /Pages /Kids [4 0 R] /Count 1 >>",
		"<< /Type /Outlines /First 5 0 R /Last 5 0 R /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 6 0 R >>",
		"<< /Title (Chapter 1) /Parent 3 0 R /Dest [4 0 R /XYZ null null null] >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buf.Bytes()
}
