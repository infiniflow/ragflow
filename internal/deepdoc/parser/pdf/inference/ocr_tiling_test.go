package inference

import (
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func testOCRBox(left, top, right, bottom float64) pdf.OCRBox {
	return pdf.OCRBox{
		X0: left, Y0: top,
		X1: right, Y1: top,
		X2: right, Y2: bottom,
		X3: left, Y3: bottom,
	}
}

func TestOCRTileStartsCoverAxis(t *testing.T) {
	if got, want := ocrTileStarts(5000), []int{0, 2120}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ocrTileStarts(5000) = %v, want %v", got, want)
	}
}

func TestDetectTiledOCRMapsCoordinates(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 5000, 10))
	var widths []int
	boxes, err := detectTiledOCR(img, func(tile image.Image) ([]pdf.OCRBox, error) {
		widths = append(widths, tile.Bounds().Dx())
		return []pdf.OCRBox{testOCRBox(0, 0, 20, 10)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := widths, []int{2880, 2880}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tile widths = %v, want %v", got, want)
	}
	if got, want := boxes, []pdf.OCRBox{testOCRBox(0, 0, 20, 10), testOCRBox(2120, 0, 2140, 10)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("boxes = %+v, want %+v", got, want)
	}
}

func TestDeduplicateOCRBoxesPreservesSameTileOverlaps(t *testing.T) {
	boxes := []pdf.OCRBox{testOCRBox(100, 100, 200, 140), testOCRBox(120, 105, 180, 135)}
	if got := deduplicateOCRBoxes(boxes, []int{0, 0}); len(got) != 2 {
		t.Fatalf("deduplicateOCRBoxes() returned %d boxes, want 2", len(got))
	}
}

func TestDeduplicateOCRBoxesMergesCrossTileChain(t *testing.T) {
	boxes := []pdf.OCRBox{
		testOCRBox(0, 0, 100, 40),
		testOCRBox(100, 0, 200, 40),
		testOCRBox(50, 0, 150, 40),
	}
	got := deduplicateOCRBoxes(boxes, []int{0, 1, 2})
	want := []pdf.OCRBox{testOCRBox(0, 0, 200, 40)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deduplicateOCRBoxes() = %+v, want %+v", got, want)
	}
}

func TestOCRDetectUsesStrictTilingThreshold(t *testing.T) {
	for _, tc := range []struct {
		name       string
		width      int
		wantWidths []int
	}{
		{name: "at threshold", width: 4096, wantWidths: []int{4096}},
		{name: "above threshold", width: 4097, wantWidths: []int{2880, 2880}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requestWidths []int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				file, _, err := r.FormFile("request")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				img, _, err := image.Decode(file)
				if err != nil {
					t.Fatal(err)
				}
				requestWidths = append(requestWidths, img.Bounds().Dx())
				json.NewEncoder(w).Encode(map[string]any{
					"output": [][][][][]float64{{{{{0, 0}, {20, 0}, {20, 10}, {0, 10}}}}},
				})
			}))
			defer srv.Close()

			client := mustNewDeepDocClient(t, srv.URL)
			boxes, err := client.OCRDetect(t.Context(), image.NewRGBA(image.Rect(0, 0, tc.width, 10)))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(requestWidths, tc.wantWidths) {
				t.Fatalf("request widths = %v, want %v", requestWidths, tc.wantWidths)
			}
			if tc.width > ocrTilingThreshold && len(boxes) != 2 {
				t.Fatalf("box count = %d, want 2", len(boxes))
			}
		})
	}
}
