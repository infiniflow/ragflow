package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/signintech/gopdf"
)

func TestPDFParser_ParseWithResult_SoMarkJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
	var submitSeen bool
	var pollSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			submitSeen = true
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				return
			}
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				body, _ := io.ReadAll(part)
				fields[part.FormName()] = string(body)
			}
			if fields["api_key"] != "somark-secret" {
				t.Errorf("api_key = %q, want somark-secret", fields["api_key"])
				return
			}
			if !strings.Contains(fields["element_formats"], "image") {
				t.Errorf("element_formats = %q", fields["element_formats"])
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
		case "/parse/async_check":
			pollSeen = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "task_id=task-1") {
				t.Errorf("poll body = %q, want task_id", string(body))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"title","content":"SoMark Title","title_level":2},{"type":"figure","content":"Figure caption"},{"type":"table","content":"<table><tr><td>x</td></tr></table>"}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "SoMark",
		"somark_base_url": server.URL,
		"somark_api_key":  "somark-secret",
	})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !submitSeen || !pollSeen {
		t.Fatalf("submit/poll seen = %v/%v, want true/true", submitSeen, pollSeen)
	}
	if len(res.JSON) < 3 {
		t.Fatalf("JSON len = %d, want >=3", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_SoMarkRequiresBaseURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "SoMark"})
	pdf.SoMarkBaseURL = ""
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "somark_base_url") {
		t.Fatalf("error = %v, want somark_base_url context", res.Err)
	}
}

func TestSoMarkBlockToItem_DropsHeaderByDefault(t *testing.T) {
	if item, _ := soMarkBlockToItems(t.Context(), map[string]any{"type": "header", "content": "x"}, false, &htmlWalkState{}); item != nil {
		t.Fatalf("item = %#v, want nil", item)
	}
}

func TestSoMarkSubmitMultipartShape(t *testing.T) {
	withSSRFBypass(t)
	var form multipart.Form
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = *r.MultipartForm
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-3"}}`))
	}))
	defer server.Close()
	taskID, err := soMarkSubmit(context.Background(), server.URL, "sample.pdf", []byte("%PDF"), NewPDFParser(), "key")
	if err != nil {
		t.Fatalf("soMarkSubmit: %v", err)
	}
	if taskID != "task-3" {
		t.Fatalf("taskID = %q, want task-3", taskID)
	}
	if got := form.Value["api_key"][0]; got != "key" {
		t.Fatalf("api_key = %q, want key", got)
	}
}

func TestSoMarkItemsEmitPagePositions(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": float64(0), "blocks": []any{
			map[string]any{"type": "text", "content": "first", "bbox": []any{10.0, 20.0, 110.0, 40.0}},
		}},
		map[string]any{"page_num": float64(2), "blocks": []any{
			map[string]any{"type": "text", "content": "swapped", "bbox": []any{200.0, 90.0, 100.0, 50.0}},
			map[string]any{"type": "text", "content": "no bbox"},
		}},
	}}}}
	items, pageCount, _ := soMarkItems(context.Background(), result, false, nil)
	if pageCount != 2 || len(items) != 3 {
		t.Fatalf("pageCount=%d items=%d, want 2/3", pageCount, len(items))
	}
	want := [][]any{
		{float64(1), 10.0, 110.0, 20.0, 40.0},
		{float64(3), 100.0, 200.0, 50.0, 90.0},
	}
	for i, w := range want {
		for _, key := range []string{"positions", "_pdf_positions"} {
			rows, _ := items[i][key].([][]any)
			if len(rows) != 1 || !reflect.DeepEqual(rows[0], w) {
				t.Fatalf("item %d %s = %#v, want %#v", i, key, items[i][key], w)
			}
		}
	}
	if _, ok := items[2]["positions"]; ok {
		t.Fatalf("block without bbox must not carry positions: %#v", items[2])
	}
}

func TestSoMarkItemsPreferBlockPageIdx(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": float64(0), "blocks": []any{
			map[string]any{"type": "text", "content": "own page", "page_idx": float64(1), "bbox": []any{10.0, 20.0, 110.0, 40.0}},
			map[string]any{"type": "text", "content": "page fallback", "bbox": []any{10.0, 20.0, 110.0, 40.0}},
		}},
		map[string]any{"page_num": float64(1), "blocks": []any{}},
	}}}}
	items, _, _ := soMarkItems(context.Background(), result, false, nil)
	if len(items) != 2 {
		t.Fatalf("items=%d, want 2", len(items))
	}
	rows, _ := items[0]["_pdf_positions"].([][]any)
	if len(rows) != 1 || rows[0][0] != float64(2) {
		t.Fatalf("block page_idx must win: %#v", items[0]["_pdf_positions"])
	}
	rows, _ = items[1]["_pdf_positions"].([][]any)
	if len(rows) != 1 || rows[0][0] != float64(1) {
		t.Fatalf("missing page_idx must fall back to page_num: %#v", items[1]["_pdf_positions"])
	}
}

func TestSoMarkItemsRejectInvalidPageIdx(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": float64(2), "blocks": []any{
			map[string]any{"type": "text", "content": "fractional", "page_idx": float64(4.5), "bbox": []any{10.0, 20.0, 110.0, 40.0}},
			map[string]any{"type": "text", "content": "overflow", "page_idx": float64(1e30), "bbox": []any{10.0, 20.0, 110.0, 40.0}},
			map[string]any{"type": "text", "content": "valid", "page_idx": float64(7), "bbox": []any{10.0, 20.0, 110.0, 40.0}},
		}},
	}}}}
	items, _, _ := soMarkItems(context.Background(), result, false, nil)
	if len(items) != 3 {
		t.Fatalf("items=%d, want 3", len(items))
	}
	// Fractional, overflowing AND merely out-of-range page_idx values all
	// fall back to the enclosing page: emitting page 8 of a 1-page result
	// would point at a page that does not exist and inflate page_count.
	for i, want := range []float64{3, 3, 3} {
		rows, _ := items[i]["_pdf_positions"].([][]any)
		if len(rows) != 1 || rows[0][0] != want {
			t.Fatalf("item %d page = %#v, want %v (invalid or out-of-range page_idx must fall back to page_num)", i, rows, want)
		}
	}
}

func TestPDFParser_ParseWithResult_SoMarkJSONCarriesPositions(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
		case "/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"page_num":1,"blocks":[{"type":"text","content":"hello","bbox":[5,6,50,16]}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "SoMark",
		"output_format":   "json",
		"somark_base_url": server.URL,
		"somark_api_key":  "k",
	})
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	rows, ok := ExtractPDFPositions(res.JSON[0].ToMap())
	if !ok || len(rows) != 1 || len(rows[0]) != 5 {
		t.Fatalf("positions = %#v ok=%v", rows, ok)
	}
	for i, w := range []float64{2, 5, 50, 6, 16} {
		if got, _ := numericAny(rows[0][i]); got != w {
			t.Fatalf("positions[0][%d] = %v, want %v (row %#v)", i, rows[0][i], w, rows[0])
		}
	}
}

// makeTestPDF synthesizes a one-page PDF with the given point size so tests
// can verify pixel-to-point rescaling against a real page box.
func makeTestPDF(t *testing.T, widthPt, heightPt float64) []byte {
	t.Helper()
	var buf bytes.Buffer
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: widthPt, H: heightPt}})
	pdf.AddPage()
	if _, err := pdf.WriteTo(&buf); err != nil {
		t.Fatalf("synthesize PDF: %v", err)
	}
	return buf.Bytes()
}

func TestSoMarkPagePointSizesReadsPageBox(t *testing.T) {
	data := makeTestPDF(t, 595.28, 841.89)
	sizes := soMarkPagePointSizes(data)
	if len(sizes) != 1 {
		t.Fatalf("sizes = %v, want one page", sizes)
	}
	if math.Abs(sizes[0][0]-595.28) > 1 || math.Abs(sizes[0][1]-841.89) > 1 {
		t.Fatalf("sizes[0] = %v, want ~[595.28 841.89]", sizes[0])
	}
	if got := soMarkPagePointSizes([]byte("not a pdf")); got != nil {
		t.Fatalf("unreadable PDF must yield nil (no rescale), got %v", got)
	}
}

func TestSoMarkPositionsRescaledFromPixelsToPoints(t *testing.T) {
	// SoMark reports bbox in its rendered pixel space (page_size {w,h});
	// RAGFlow positions are PDF points. An A4 page (~595x842 pt) rendered by
	// the service at ~200 dpi is 1654x2339 px, so raw pixel boxes are ~2.78x
	// too large without rescaling.
	data := makeTestPDF(t, 595.28, 841.89)
	sizes := soMarkPagePointSizes(data)
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{
		"pages": []any{
			map[string]any{
				"page_num":  float64(0),
				"page_size": map[string]any{"w": float64(1654), "h": float64(2339)},
				"blocks": []any{
					map[string]any{"type": "text", "content": "scaled", "bbox": []any{165.4, 233.9, 827.0, 467.8}},
				},
			},
		},
	}}}
	items, _, _ := soMarkItems(context.Background(), result, false, sizes)
	pos := items[0]["_pdf_positions"].([][]any)[0]
	wantX0 := 165.4 / 1654 * 595.28
	wantX1 := 827.0 / 1654 * 595.28
	wantTop := 233.9 / 2339 * 841.89
	wantBott := 467.8 / 2339 * 841.89
	if pos[0] != float64(1) {
		t.Fatalf("page = %v, want 1", pos[0])
	}
	for i, want := range []float64{wantX0, wantX1, wantTop, wantBott} {
		if got := pos[i+1].(float64); math.Abs(got-want) > 0.5 {
			t.Fatalf("pos[%d] = %v, want ~%v (unscaled pixel value would be ~2.8x larger)", i+1, got, want)
		}
	}
}

func TestSoMarkPositionsClampedToPage(t *testing.T) {
	data := makeTestPDF(t, 595.28, 841.89)
	sizes := soMarkPagePointSizes(data)
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{
		"pages": []any{
			map[string]any{
				"page_num":  float64(0),
				"page_size": map[string]any{"w": float64(1654), "h": float64(2339)},
				"blocks": []any{
					map[string]any{"type": "text", "content": "overflow", "bbox": []any{-50.0, -10.0, 1700.0, 2400.0}},
				},
			},
		},
	}}}
	items, _, _ := soMarkItems(context.Background(), result, false, sizes)
	pos := items[0]["_pdf_positions"].([][]any)[0]
	for i, v := range []float64{0, 595.28, 0, 841.89} {
		if got := pos[i+1].(float64); math.Abs(got-v) > 1 {
			t.Fatalf("pos[%d] = %v, want clamped ~%v", i+1, got, v)
		}
	}
}

func TestSoMarkPositionsRawWhenScaleUnknown(t *testing.T) {
	// No page_size in the payload (or unknown page dims): keep the raw box,
	// matching the Python converter's fallback when a page cannot be rendered.
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{
		"pages": []any{
			map[string]any{
				"page_num": float64(0),
				"blocks": []any{
					map[string]any{"type": "text", "content": "raw", "bbox": []any{10.0, 20.0, 110.0, 40.0}},
				},
			},
		},
	}}}
	items, _, _ := soMarkItems(context.Background(), result, false, nil)
	pos := items[0]["_pdf_positions"].([][]any)[0]
	for i, v := range []float64{10, 110, 20, 40} {
		if got := pos[i+1].(float64); got != v {
			t.Fatalf("pos[%d] = %v, want raw %v", i+1, got, v)
		}
	}
}

func TestSoMarkPositionsCrossPageScaling(t *testing.T) {
	// A block whose page_idx points at another page must scale against THAT
	// page's pixel space AND point size. Fixture pages: 595.5x842 pt (page 1)
	// and 300x400 pt (page 2); the block lives in page 1's entry but claims
	// page_idx 1 (page 2), rendered at 827x1169 px.
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Skipf("fixture: %v", err)
	}
	sizes := soMarkPagePointSizes(data)
	if len(sizes) != 3 {
		t.Fatalf("sizes = %v, want three pages", sizes)
	}
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{
		"pages": []any{
			map[string]any{
				"page_num":  float64(0),
				"page_size": map[string]any{"w": float64(1654), "h": float64(2339)},
				"blocks": []any{
					map[string]any{"type": "text", "content": "cross", "page_idx": float64(1),
						"bbox": []any{413.5, 292.25, 620.25, 438.375}},
				},
			},
			map[string]any{
				"page_num":  float64(1),
				"page_size": map[string]any{"w": float64(827), "h": float64(1169)},
				"blocks":    []any{},
			},
		},
	}}}
	items, _, _ := soMarkItems(context.Background(), result, false, sizes)
	pos := items[0]["_pdf_positions"].([][]any)[0]
	if pos[0] != float64(2) {
		t.Fatalf("page = %v, want 2", pos[0])
	}
	// Hand-computed against page 2 (300x400 pt at 827x1169 px):
	// x0 = 413.5/827*300 = 150, x1 = 620.25/827*300 = 225,
	// top = 292.25/1169*400 = 100, bott = 438.375/1169*400 = 150.
	for i, want := range []float64{150, 225, 100, 150} {
		if got := pos[i+1].(float64); math.Abs(got-want) > 0.5 {
			t.Fatalf("pos[%d] = %v, want ~%v (mixing the enclosing page's 1654px width would give ~75/112.5)", i+1, got, want)
		}
	}
}

func TestSoMarkPagePointSizesHonorRotate(t *testing.T) {
	// Fixture page 3 declares MediaBox [0 0 400 300] with /Rotate 90: the
	// rendered page is 300 wide x 400 high, and positions must scale and
	// clamp against the rotated size.
	data, err := os.ReadFile("testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Skipf("fixture: %v", err)
	}
	sizes := soMarkPagePointSizes(data)
	if len(sizes) != 3 {
		t.Fatalf("sizes = %v, want three pages", sizes)
	}
	if math.Abs(sizes[0][0]-595.5) > 1 || math.Abs(sizes[0][1]-842) > 1 {
		t.Fatalf("page 1 = %v, want ~[595.5 842]", sizes[0])
	}
	if math.Abs(sizes[1][0]-300) > 1 || math.Abs(sizes[1][1]-400) > 1 {
		t.Fatalf("page 2 = %v, want ~[300 400]", sizes[1])
	}
	if math.Abs(sizes[2][0]-300) > 1 || math.Abs(sizes[2][1]-400) > 1 {
		t.Fatalf("page 3 = %v, want rotated ~[300 400], not raw MediaBox [400 300]", sizes[2])
	}
}

func TestSoMarkBlockPositionRejectsNonFinite(t *testing.T) {
	for _, bbox := range [][]any{
		{1.0, math.NaN(), 110.0, 40.0},
		{1.0, 20.0, math.Inf(1), 40.0},
		{1.0, 20.0, 110.0, math.Inf(-1)},
	} {
		if pos := soMarkBlockPosition(map[string]any{"bbox": bbox}, 0, 100, 100, nil); pos != nil {
			t.Fatalf("non-finite bbox %v must drop the position, got %#v", bbox, pos)
		}
	}
}

// buildSoMarkPDFWithCount emits a minimal but structurally valid PDF whose
// /Pages /Count is set to the supplied (possibly malformed) value, like
// deepdoc/pdf's buildPDFWithCount. Offsets are computed so the xref table is
// self-consistent and ledongthuc/pdf can open it.
func buildSoMarkPDFWithCount(t *testing.T, count int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	off1 := buf.Len()
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count %d >>\nendobj\n", count)
	off3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>\nendobj\n")
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", off1)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off2)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off3)
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefStart)
	return buf.Bytes()
}

func TestSoMarkPagePointSizesRejectsMalformedCount(t *testing.T) {
	// A negative or absurd catalog /Count must fall back to the caller's
	// pixel-space scaling instead of panicking make or allocating/iterating
	// without bound.
	for _, count := range []int{-1, 1 << 30} {
		if got := soMarkPagePointSizes(buildSoMarkPDFWithCount(t, count)); got != nil {
			t.Fatalf("/Count %d: got %v, want nil fallback", count, got)
		}
	}
	// A sane count still reads page boxes.
	if got := soMarkPagePointSizes(buildSoMarkPDFWithCount(t, 1)); len(got) != 1 {
		t.Fatalf("/Count 1: got %v, want one page", got)
	}
}

func TestSoMarkPagePointSizesStopsAtParentCycle(t *testing.T) {
	// ledongthuc/pdf detects no reference cycles: a page whose /Parent points
	// at itself must not hang the inheritance walk.
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	off1 := buf.Len()
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	off3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 3 0 R /Resources << >> >>\nendobj\n")
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", off1)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off2)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off3)
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefStart)

	done := make(chan [][2]float64, 1)
	go func() { done <- soMarkPagePointSizes(buf.Bytes()) }()
	select {
	case sizes := <-done:
		// No box is resolvable through the cycle: the page falls back to an
		// empty size (the caller's pixel-space scaling), as an unreadable
		// box already does.
		if len(sizes) != 1 || sizes[0] != [2]float64{} {
			t.Fatalf("cyclic /Parent page: got %v, want one empty size", sizes)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("soMarkPagePointSizes hung on a self-referencing /Parent")
	}
}
