//go:build cgo

package chunker

import (
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/util"
	"ragflow/internal/ingestion/component/schema"
)

type textCropEngine struct {
	mockCropEngine
	page image.Image
}

func (e textCropEngine) RenderPageImage(int, float64) (image.Image, error) {
	return e.page, nil
}

func TestCropImageChunksPreservesCompleteTextLines(t *testing.T) {
	page := image.NewRGBA(image.Rect(0, 0, 240, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 240; x++ {
			page.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	for _, top := range []int{100, 200} {
		for y := top; y < top+24; y++ {
			for x := 40; x < 100; x++ {
				page.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	pos := jsonPositions(t, []float64{1, 10, 60, 112.0 / 3, 212.0 / 3})
	for _, tc := range []struct {
		name     string
		ck       schema.ChunkDoc
		wantRows int
	}{
		{"text", schema.ChunkDoc{Text: "two lines", PDFPositions: pos}, 48},
		{"text fallback", schema.ChunkDoc{Text: "two lines", DocType: "text", Positions: pos}, 48},
		{"image", schema.ChunkDoc{Text: "figure", DocType: "image", PDFPositions: pos}, 24},
		{"table", schema.ChunkDoc{Text: "table", DocType: "table", PDFPositions: pos}, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := cropImageChunks(t.Context(), textCropEngine{page: page}, []schema.ChunkDoc{tc.ck})
			if out[0].Text != tc.ck.Text || !reflect.DeepEqual(out[0].PDFPositions, tc.ck.PDFPositions) || !reflect.DeepEqual(out[0].Positions, tc.ck.Positions) {
				t.Fatal("preview correction changed text or source coordinates")
			}
			img, err := util.DecodeBase64PNG(strings.TrimPrefix(out[0].Image, "data:image/png;base64,"))
			if err != nil {
				t.Fatal(err)
			}
			rows := 0
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				// White margin identifies the body, excluding dimmed context.
				if color.RGBAModel.Convert(img.At(0, y)) == (color.RGBA{255, 255, 255, 255}) &&
					color.RGBAModel.Convert(img.At(20, y)) == (color.RGBA{0, 0, 0, 255}) {
					rows++
				}
			}
			if rows != tc.wantRows {
				t.Fatalf("visible text rows = %d, want %d", rows, tc.wantRows)
			}
		})
	}
}

func TestCropImageChunksKeepsSeparateMediaBoxes(t *testing.T) {
	page := image.NewRGBA(image.Rect(0, 0, 240, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 240; x++ {
			page.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	for _, typ := range []string{"image", "table"} {
		t.Run(typ, func(t *testing.T) {
			pos := jsonPositions(t,
				[]float64{1, 10, 60, 112.0 / 3, 162.0 / 3},
				[]float64{1, 10, 60, 162.0 / 3, 212.0 / 3})
			out := cropImageChunks(t.Context(), textCropEngine{page: page}, []schema.ChunkDoc{{Text: typ, DocType: typ, PDFPositions: pos}})
			img, err := util.DecodeBase64PNG(strings.TrimPrefix(out[0].Image, "data:image/png;base64,"))
			if err != nil {
				t.Fatal(err)
			}
			// Media layout (Python cropout): two 50px boxes and two 6px
			// stitch gaps, no context bands. Touching media boxes remain
			// separate; the gap between them stays gray.
			if img.Bounds().Dy() != 112 {
				t.Fatalf("media height = %d, want 112", img.Bounds().Dy())
			}
			if got := color.RGBAModel.Convert(img.At(0, 52)); got != (color.RGBA{245, 245, 245, 255}) {
				t.Fatalf("media box separator = %v, want original gap", got)
			}
		})
	}
}
