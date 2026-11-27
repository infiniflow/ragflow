package parser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/signintech/gopdf"
)

// withSoMarkPollInterval shortens the poll interval for the duration of a test
// so poll-loop cases do not spend real seconds sleeping. Tests in this package
// run sequentially, so mutating the package variable is safe.
func withSoMarkPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	prev := soMarkPollInterval
	soMarkPollInterval = d
	t.Cleanup(func() { soMarkPollInterval = prev })
}

func TestPDFParser_ParseWithResult_SoMarkJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	var submitSeen bool
	var pollSeen bool
	pollCount := 0
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
			pollCount++
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "task_id=task-1") {
				t.Errorf("poll body = %q, want task_id", string(body))
				return
			}
			if pollCount == 1 {
				_, _ = w.Write([]byte(`{"code":0,"data":{"status":"QUEUING"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"title","content":"SoMark Title","title_level":2,"bbox":[5,6,50,16]},{"type":"figure","content":"Figure caption"},{"type":"table","content":"<table><tr><td>x</td></tr></table>"}]}]}}}}}`))
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
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "page 1") {
		t.Fatalf("warnings = %v, want a geometry warning for page 1", res.Warnings)
	}
	if !submitSeen || !pollSeen || pollCount < 2 {
		t.Fatalf("submit/poll seen = %v/%v (%d checks), want submit and at least two poll checks", submitSeen, pollSeen, pollCount)
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

func TestSoMarkBlock_DropsHeaderByDefault(t *testing.T) {
	if block, ok, _ := soMarkBlock(t.Context(), map[string]any{"type": "header", "content": "x"}, false, &htmlWalkState{budget: newMediaBudgetForTest()}); ok {
		t.Fatalf("block = %#v, want the header dropped", block)
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

func TestSoMarkItemsUseEnclosingPageGeometry(t *testing.T) {
	// The service omitted the middle page. A stray block-level page_idx points
	// to a different, valid page with a different pixel-to-point scale.
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": 0, "page_size": map[string]any{"w": 1024, "h": 1024}, "blocks": []any{
			map[string]any{"type": "text", "content": "first", "page_idx": 2, "bbox": []any{10, 20, 110, 40}},
		}},
		map[string]any{"page_num": 2, "page_size": map[string]any{"w": 512, "h": 512}, "blocks": []any{
			map[string]any{"type": "text", "content": "swapped", "bbox": []any{200, 90, 100, 50}},
			map[string]any{"type": "text", "content": "no bbox"},
		}},
	}}}}
	items, pageCount, warnings := soMarkItems(t.Context(), result, false, [][2]float64{{512, 512}, {}, {128, 256}})
	if pageCount != 3 || len(items) != 3 || len(warnings) != 0 {
		t.Fatalf("pageCount=%d items=%d warnings=%v", pageCount, len(items), warnings)
	}
	want := [][]any{
		{1, 5.0, 55.0, 10.0, 20.0},
		{3, 25.0, 50.0, 25.0, 45.0},
	}
	for i, row := range want {
		if items[i]["page_number"] != row[0] {
			t.Fatalf("item %d page_number = %v, want %v", i, items[i]["page_number"], row[0])
		}
		for _, key := range []string{"positions", "_pdf_positions"} {
			if !reflect.DeepEqual(items[i][key], [][]any{row}) {
				t.Fatalf("item %d %s = %#v, want %#v", i, key, items[i][key], row)
			}
		}
	}
	if items[2]["positions"] != nil || items[2]["_pdf_positions"] != nil {
		t.Fatalf("block without bbox must not carry positions: %#v", items[2])
	}
}
func TestPDFParser_ParseWithResult_SoMarkJSONCarriesPositions(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
		case "/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"page_num":0,"page_size":{"w":1024,"h":1024},"blocks":[{"type":"text","content":"hello","bbox":[5,6,50,16]}]}]}}}}}`))
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
	res := pdf.ParseWithResult(t.Context(), "sample.pdf", makeTestPDF(t, 512, 512))
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
	for i, w := range []float64{1, 2.5, 25, 3, 8} {
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
	if pos[0] != 1 {
		t.Fatalf("page = %v, want 1", pos[0])
	}
	for i, want := range []float64{wantX0, wantX1, wantTop, wantBott} {
		if got := pos[i+1].(float64); math.Abs(got-want) > 0.5 {
			t.Fatalf("pos[%d] = %v, want ~%v (unscaled pixel value would be ~2.8x larger)", i+1, got, want)
		}
	}
}

func TestSoMarkPositionsOmittedWhenScaleUnknown(t *testing.T) {
	pixels := map[string]any{"w": 1190, "h": 1684}
	points := [][2]float64{{595, 842}}
	for _, tc := range []struct {
		name   string
		pixels map[string]any
		points [][2]float64
	}{
		{"missing pixel size", nil, points},
		{"unreadable PDF", pixels, nil},
		{"unknown PDF page size", pixels, [][2]float64{{}}},
		{"nonfinite pixel size", map[string]any{"w": math.Inf(1), "h": 1684}, points},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
				map[string]any{"page_num": 0, "page_size": tc.pixels, "blocks": []any{
					map[string]any{"type": "text", "content": "DN100", "bbox": []any{72, 50, 200, 80}},
					map[string]any{"type": "text", "content": "L=200", "bbox": []any{210, 50, 340, 80}},
				}},
			}}}}
			items, _, warnings := soMarkItems(t.Context(), result, false, tc.points)
			if len(warnings) != 1 || !strings.Contains(warnings[0], "page 1") || len(items) != 2 {
				t.Fatalf("items = %v, warnings = %v", items, warnings)
			}
			for i, text := range []string{"DN100", "L=200"} {
				item := items[i]
				if item["text"] != text || item["page_number"] != 1 || item["positions"] != nil || item["_pdf_positions"] != nil {
					t.Fatalf("item %d = %#v, want text and page only", i, item)
				}
			}
		})
	}
}

func TestSoMarkPositionsAtPageEdges(t *testing.T) {
	for _, tc := range []struct {
		name string
		bbox []any
		want []any
		warn bool
	}{
		{"at tolerance", []any{-10, -20, 110, 220}, []any{1, 0.0, 50.0, 0.0, 100.0}, false},
		{"past left", []any{-11, 0, 25, 20}, nil, true},
		{"past right", []any{50, 10, 111, 20}, nil, true},
		{"past top", []any{10, -21, 50, 20}, nil, true},
		{"past bottom", []any{10, 150, 50, 221}, nil, true},
		{"entirely in bleed", []any{101, 0, 105, 20}, nil, false},
		{"zero height", []any{10, 20, 50, 20}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
				map[string]any{"page_num": 0, "page_size": map[string]any{"w": 100, "h": 200}, "blocks": []any{
					map[string]any{"type": "text", "content": "body", "bbox": tc.bbox},
				}},
			}}}}
			items, _, warnings := soMarkItems(t.Context(), result, false, [][2]float64{{50, 100}})
			if len(items) != 1 {
				t.Fatalf("items = %d, want 1", len(items))
			}
			rows, _ := items[0]["positions"].([][]any)
			var pos []any
			if len(rows) > 0 {
				pos = rows[0]
			}
			hasWarn := len(warnings) > 0
			if !reflect.DeepEqual(pos, tc.want) || hasWarn != tc.warn {
				t.Fatalf("position = %v, warnings = %q, want %v (warn=%v)", pos, warnings, tc.want, tc.warn)
			}
		})
	}
}

func TestSoMarkPositionsAtPageEdgesWithScaling(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": 0, "page_size": map[string]any{"w": 1000, "h": 2000}, "blocks": []any{
			map[string]any{"type": "text", "content": "body", "bbox": []any{-100, -200, 1100, 2200}},
		}},
	}}}}
	items, _, warnings := soMarkItems(t.Context(), result, false, [][2]float64{{612, 792}})
	if len(items) != 1 || len(warnings) != 0 {
		t.Fatalf("items = %d, warnings = %q", len(items), warnings)
	}
	want := []any{1, 0.0, 612.0, 0.0, 792.0}
	rows, _ := items[0]["positions"].([][]any)
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("position = %v, want %v", rows, want)
	}
}

func TestSoMarkItemsScaleRepeatedPageByOwnRaster(t *testing.T) {
	result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"page_num": 1, "page_size": map[string]any{"w": 100, "h": 100}, "blocks": []any{
			map[string]any{"type": "text", "content": "first", "bbox": []any{10, 20, 50, 60}},
		}},
		map[string]any{"page_size": map[string]any{"w": 200, "h": 200}, "blocks": []any{
			map[string]any{"type": "text", "content": "second", "bbox": []any{20, 40, 100, 120}},
		}},
	}}}}
	items, _, warnings := soMarkItems(t.Context(), result, false, [][2]float64{{100, 100}, {100, 100}})
	if len(items) != 2 || len(warnings) != 0 {
		t.Fatalf("items = %d, warnings = %q", len(items), warnings)
	}
	want := []any{2, 10.0, 50.0, 20.0, 60.0}
	for i, item := range items {
		rows, _ := item["positions"].([][]any)
		if item["page_number"] != 2 || len(rows) != 1 || !reflect.DeepEqual(rows[0], want) {
			t.Fatalf("item %d positions = %v (page %v), want %v", i, rows, item["page_number"], want)
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

func TestSoMarkItemsRejectNonFiniteBox(t *testing.T) {
	for _, bbox := range [][]any{
		{1.0, math.NaN(), 110.0, 40.0},
		{1.0, 20.0, math.Inf(1), 40.0},
		{1.0, 20.0, 110.0, math.Inf(-1)},
	} {
		result := map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
			map[string]any{"page_num": float64(0), "page_size": map[string]any{"w": 200, "h": 200}, "blocks": []any{
				map[string]any{"type": "text", "content": "body", "bbox": bbox},
			}},
		}}}}
		items, _, _ := soMarkItems(context.Background(), result, false, [][2]float64{{100, 100}})
		if len(items) != 1 {
			t.Fatalf("items = %d, want the text kept", len(items))
		}
		if pos, ok := items[0]["_pdf_positions"]; ok && pos != nil {
			t.Fatalf("non-finite bbox %v must drop the position, got %#v", bbox, pos)
		}
	}
}

// buildSoMarkPDF emits a minimal but structurally valid PDF whose
// /Pages /Count is set to the supplied (possibly malformed) value, like
// deepdoc/pdf's buildPDFWithCount. Offsets are computed so the xref table is
// self-consistent and ledongthuc/pdf can open it.
func buildSoMarkPDF(t *testing.T, count int, parentAttrs, pageAttrs string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	off1 := buf.Len()
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count %d %s >>\nendobj\n", count, parentAttrs)
	off3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R %s /Resources << >> >>\nendobj\n", pageAttrs)
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", off1)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off2)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off3)
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefStart)
	return buf.Bytes()
}

func TestSoMarkPagePointSizesHonorUserUnit(t *testing.T) {
	sizes := soMarkPagePointSizes(buildSoMarkPDF(t, 1, "", "/MediaBox [0 0 612 792] /UserUnit 2.5"))
	if !reflect.DeepEqual(sizes, [][2]float64{{1530, 1980}}) {
		t.Fatalf("sizes = %v, want 612 x 792 user units at 2.5 points per unit", sizes)
	}
}

func TestSoMarkPagePointSizesIntersectPageBoxes(t *testing.T) {
	for _, tc := range []struct {
		name, parentAttrs, pageAttrs string
		want                         [2]float64
	}{
		{"inherited crop exceeds page", "/CropBox [0 0 612 792]", "/MediaBox [0 0 300 400]", [2]float64{300, 400}},
		{"contained crop", "/MediaBox [0 0 300 400]", "/CropBox [20 30 220 330]", [2]float64{200, 300}},
		{"partial overlap with rotation and units", "/CropBox [250 350 -10 50] /Rotate 90", "/MediaBox [20 10 300 220] /UserUnit 2", [2]float64{340, 460}},
		{"disjoint boxes", "/CropBox [320 30 400 50]", "/MediaBox [0 0 300 400]", [2]float64{}},
		{"only crop available", "/CropBox [20 30 220 330]", "", [2]float64{200, 300}},
		{"malformed media with valid crop", "/MediaBox [0 0 /bad 400]", "/CropBox [20 30 220 330]", [2]float64{200, 300}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sizes := soMarkPagePointSizes(buildSoMarkPDF(t, 1, tc.parentAttrs, tc.pageAttrs))
			if len(sizes) != 1 || sizes[0] != tc.want {
				t.Fatalf("sizes = %v, want one page of size %v", sizes, tc.want)
			}
		})
	}
}

func TestSoMarkPagePointSizesRejectsMalformedCount(t *testing.T) {
	// An invalid catalog /Count makes the PDF page sizes unavailable.
	for _, count := range []int{-1, 1 << 30} {
		if got := soMarkPagePointSizes(buildSoMarkPDF(t, count, "", "/MediaBox [0 0 612 792]")); got != nil {
			t.Fatalf("/Count %d: got %v, want nil fallback", count, got)
		}
	}
	// A sane count still reads page boxes.
	if got := soMarkPagePointSizes(buildSoMarkPDF(t, 1, "", "/MediaBox [0 0 612 792]")); len(got) != 1 {
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
		// No box is resolvable through the cycle.
		if len(sizes) != 1 || sizes[0] != [2]float64{} {
			t.Fatalf("cyclic /Parent page: got %v, want one empty size", sizes)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("soMarkPagePointSizes hung on a self-referencing /Parent")
	}
}

// soMarkPollServer serves /parse/async_check from a scripted sequence of
// responses and reports how many polls were made. Once the sequence is
// exhausted the last response repeats, so "always failing" is one entry.
func soMarkPollServer(t *testing.T, responses ...func(w http.ResponseWriter, poll int)) (*httptest.Server, *int) {
	t.Helper()
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parse/async_check" {
			http.NotFound(w, r)
			return
		}
		polls++
		idx := polls - 1
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		responses[idx](w, polls)
	}))
	t.Cleanup(server.Close)
	return server, &polls
}

func soMarkStatusResponse(status string) func(http.ResponseWriter, int) {
	return func(w http.ResponseWriter, _ int) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"` + status + `"}}`))
	}
}

func soMarkSuccessResponse(w http.ResponseWriter, _ int) {
	_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[]}}}}}`))
}

func TestSoMarkPollRetriesTransientFailures(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server, polls := soMarkPollServer(t,
		func(w http.ResponseWriter, _ int) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
		},
		func(w http.ResponseWriter, _ int) {
			_, _ = w.Write([]byte(`{"code":1133,"message":"task not found"}`))
		},
		soMarkSuccessResponse,
	)
	result, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err != nil {
		t.Fatalf("soMarkPoll: %v", err)
	}
	if result == nil || *polls != 3 {
		t.Fatalf("result=%v polls=%d, want success after 3 polls", result, *polls)
	}
}

func TestSoMarkPollFailsAfterConsecutiveTransientFailures(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server, polls := soMarkPollServer(t, func(w http.ResponseWriter, _ int) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "consecutive transient failures") {
		t.Fatalf("error = %v, want consecutive transient failure", err)
	}
	if *polls != maxSoMarkPollFailures {
		t.Fatalf("polls = %d, want %d", *polls, maxSoMarkPollFailures)
	}
}

func TestSoMarkPollPollsDocumentedInProgressStatuses(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server, polls := soMarkPollServer(t,
		soMarkStatusResponse("WAITING"),
		soMarkStatusResponse("QUEUING"),
		soMarkStatusResponse("CONVERTING"),
		soMarkStatusResponse("PROCESSING"),
		soMarkSuccessResponse,
	)
	result, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err != nil {
		t.Fatalf("soMarkPoll: %v", err)
	}
	if result == nil || *polls != 5 {
		t.Fatalf("result=%v polls=%d, want success after 5 polls", result, *polls)
	}
}

func TestSoMarkPollFailedStatusFailsFast(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server, polls := soMarkPollServer(t, soMarkStatusResponse("FAILED"))
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "failed status=FAILED") {
		t.Fatalf("error = %v, want failed status", err)
	}
	if *polls != 1 {
		t.Fatalf("polls = %d, want 1 (no retry on a terminal status)", *polls)
	}
}

func TestSoMarkPollRejectsUnknownStatus(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server, polls := soMarkPollServer(t, soMarkStatusResponse("MYSTERY"))
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "unknown status") {
		t.Fatalf("error = %v, want unknown status protocol error", err)
	}
	if *polls != 1 {
		t.Fatalf("polls = %d, want 1 (unknown status is a protocol error, not in-progress)", *polls)
	}
}

func TestSoMarkPollRejectsEmptyStatus(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	server, _ := soMarkPollServer(t, func(w http.ResponseWriter, _ int) {
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), `unknown status ""`) {
		t.Fatalf("error = %v, want empty-status protocol error", err)
	}
}

func TestSoMarkPollHonorsConfiguredTimeout(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	t.Setenv("SOMARK_POLL_TIMEOUT", "20ms")
	server, _ := soMarkPollServer(t, soMarkStatusResponse("PROCESSING"))
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestSoMarkPollTimeoutFallsBackOnMalformedEnv(t *testing.T) {
	t.Setenv("SOMARK_POLL_TIMEOUT", "not-a-duration")
	if got := soMarkPollTimeout(); got != defaultSoMarkPollTimeout {
		t.Fatalf("soMarkPollTimeout() = %v, want default %v", got, defaultSoMarkPollTimeout)
	}
	t.Setenv("SOMARK_POLL_TIMEOUT", "")
	if got := soMarkPollTimeout(); got != defaultSoMarkPollTimeout {
		t.Fatalf("empty SOMARK_POLL_TIMEOUT = %v, want default %v", got, defaultSoMarkPollTimeout)
	}
}

func TestSoMarkPollTimeoutReadsEnv(t *testing.T) {
	t.Setenv("SOMARK_POLL_TIMEOUT", "90s")
	if got := soMarkPollTimeout(); got != 90*time.Second {
		t.Fatalf("soMarkPollTimeout() = %v, want 90s", got)
	}
}

func TestSoMarkPollDeadlineDuringRequestFormatsTimeout(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	t.Setenv("SOMARK_POLL_TIMEOUT", "20ms")
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Never respond: the client's poll budget, not the server, must end
		// the request. (r.Context().Done() is not reliable here — the server
		// may not observe the client's cancellation until it writes.)
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want the standardized timeout error (with the last status), not the raw transport error", err)
	}
}

func TestSoMarkPollRejectsPermanentHTTPErrorBeforeReadingBody(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, 5*time.Millisecond)
	t.Setenv("SOMARK_POLL_TIMEOUT", "500ms")
	release := make(chan struct{})
	var polls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		polls.Add(1)
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusUnauthorized)
		// Flush the headers, then never write the body: a client that reads
		// before classifying would block until the poll deadline.
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	start := time.Now()
	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("error = %v, want HTTP 401", err)
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("took %v, want fast failure before reading the stalled body", elapsed)
	}
	if got := polls.Load(); got != 1 {
		t.Fatalf("polls = %d, want 1 (a 4xx is terminal)", got)
	}
}

func TestSoMarkPollCancelledErrorIncludesLastStatus(t *testing.T) {
	withSSRFBypass(t)
	withSoMarkPollInterval(t, time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var polls atomic.Int64
	// Buffered so the handler's non-blocking send cannot be dropped when the
	// goroutine is not yet receiving; a dropped notification would leave both
	// the handler and the goroutine blocked.
	secondPoll := make(chan struct{}, 1)
	releaseSecond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if polls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"RUNNING"}}`))
			return
		}
		// Block the second poll until the test has cancelled the request. The
		// first poll has then already recorded RUNNING, so the cancellation is
		// observed with that status no matter how long response processing
		// takes — no sleep, no flaky window.
		select {
		case secondPoll <- struct{}{}:
		default:
		}
		<-releaseSecond
	}))
	t.Cleanup(server.Close)

	go func() {
		<-secondPoll
		cancel()
		close(releaseSecond)
	}()
	_, err := soMarkPoll(ctx, server.URL, "task-1", "")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if !strings.Contains(err.Error(), `last status="RUNNING"`) {
		t.Fatalf("error = %v, want the cancelled message to carry the last status", err)
	}
}
