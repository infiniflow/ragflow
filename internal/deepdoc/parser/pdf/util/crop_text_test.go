package util

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestCropTextSectionPositionsKeepsUnresolvableCrop(t *testing.T) {
	positions := []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 90, Top: 150, Bottom: 200}}
	pages := map[int]image.Image{0: makeTestPageImage(100, 400, color.RGBA{80, 80, 80, 255})}
	if got, want := CropTextSectionPositions(positions, pages, 1), CropSectionPositions(positions, pages, 1); got != want {
		t.Fatal("a crop with no clear rows should retain its body and context boundaries")
	}
}

func TestCropTextSectionPositionsDoesNotRepeatExpandedLine(t *testing.T) {
	for _, drift := range []float64{0, 1} {
		t.Run(fmt.Sprintf("horizontal drift %.0f", drift), func(t *testing.T) {
			page := makeTestPageImage(100, 400, color.RGBA{255, 255, 255, 255}).(*image.RGBA)
			for y := 140; y < 158; y++ {
				for x := 20; x < 50; x++ {
					page.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
				}
			}
			positions := []pdf.Position{
				{PageNumbers: []int{0}, Left: 10, Right: 90, Top: 130, Bottom: 147},
				{PageNumbers: []int{0}, Left: 10 + drift, Right: 90 + drift, Top: 151, Bottom: 170},
			}
			img, err := DecodeBase64PNG(CropTextSectionPositions(positions, map[int]image.Image{0: page}, 1))
			if err != nil {
				t.Fatal(err)
			}
			rows := 0
			for y := 0; y < img.Bounds().Dy(); y++ {
				if color.RGBAModel.Convert(img.At(0, y)) == (color.RGBA{255, 255, 255, 255}) &&
					color.RGBAModel.Convert(img.At(20, y)) == (color.RGBA{0, 0, 0, 255}) {
					rows++
				}
			}
			if rows != 18 {
				t.Fatalf("expanded line has %d ink rows, want 18 without repetition", rows)
			}
		})
	}
}

func TestSnapTextBoundary(t *testing.T) {
	// Use a non-zero origin and a second column with no whitespace, proving
	// that the search stays inside both the page and the selected column.
	page := image.NewRGBA(image.Rect(10, 20, 110, 120))
	for y := 20; y < 120; y++ {
		for x := 10; x < 110; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 70 || (x >= 30 && x < 50 && y >= 50 && y < 70) {
				c = color.RGBA{0, 0, 0, 255}
			}
			page.SetRGBA(x, y, c)
		}
	}
	for _, tc := range []struct {
		name                               string
		x0, x1, y, direction, radius, want int
		safe                               bool
	}{
		{"top inside line", 20, 60, 60, -1, 15, 49, true},
		{"bottom inside line", 20, 60, 60, 1, 15, 71, true},
		{"top already safe", 20, 60, 50, -1, 15, 50, true},
		{"bottom already safe", 20, 60, 70, 1, 15, 70, true},
		{"white cut", 20, 60, 40, -1, 15, 40, true},
		{"search limit", 20, 60, 60, -1, 5, 60, false},
		{"no whitespace", 70, 100, 60, -1, 15, 60, false},
		{"page top", 20, 60, 20, -1, 15, 20, true},
		{"page bottom", 20, 60, 120, 1, 15, 120, true},
		{"outside column", 120, 130, 60, -1, 15, 60, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, safe := snapTextBoundary(page, tc.x0, tc.x1, tc.y, tc.direction, tc.radius)
			if got != tc.want || safe != tc.safe {
				t.Fatalf("boundary = (%d, %v), want (%d, %v)", got, safe, tc.want, tc.safe)
			}
		})
	}
}

func TestCropTextSectionPositionsKeepsWholeContextLines(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprintf("wider context %v", wide), func(t *testing.T) {
			page := makeTestPageImage(100, 500, color.RGBA{255, 255, 255, 255}).(*image.RGBA)
			for i, top := range []int{20, 100, 140, 240, 300, 360} {
				for y := top; y < top+20; y++ {
					for x := 20; x < 40; x++ {
						page.SetRGBA(x, y, color.RGBA{uint8(40 + 20*i), 0, 0, 255})
					}
				}
			}
			positions := []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 90, Top: 150, Bottom: 250}}
			if wide {
				positions = []pdf.Position{
					{PageNumbers: []int{0}, Left: 10, Right: 50, Top: 150, Bottom: 210},
					{PageNumbers: []int{0}, Left: 10, Right: 90, Top: 210, Bottom: 250},
				}
				// The wider context must not leave a fragment of this adjacent
				// column's line at its inner edge.
				for y := 130; y < 145; y++ {
					for x := 70; x < 80; x++ {
						page.SetRGBA(x, y, color.RGBA{0, 0, 200, 255})
					}
				}
			}
			img, err := DecodeBase64PNG(CropTextSectionPositions(positions, map[int]image.Image{0: page}, 1))
			if err != nil {
				t.Fatal(err)
			}
			counts := make(map[uint8]int)
			for y := 0; y < img.Bounds().Dy(); y++ {
				p := color.RGBAModel.Convert(img.At(10, y)).(color.RGBA)
				if p.G == 0 && p.B == 0 && p.A == 255 {
					counts[p.R]++
				}
				if wide {
					p = color.RGBAModel.Convert(img.At(60, y)).(color.RGBA)
					if p.R == 0 && p.G == 0 && p.B > 0 {
						t.Fatal("context clips adjacent column's line")
					}
				}
			}
			// Each source line appears exactly once: four dimmed context
			// lines (factor 127/255) and two unchanged body lines.
			want := map[uint8]int{19: 20, 29: 20, 80: 20, 100: 20, 59: 20, 69: 20}
			if !reflect.DeepEqual(counts, want) {
				t.Fatalf("source line rows = %v, want %v", counts, want)
			}
		})
	}
}

func TestCropTextSectionPositionsKeepsLinesAtPageEdges(t *testing.T) {
	page := makeTestPageImage(100, 100, color.RGBA{255, 255, 255, 255}).(*image.RGBA)
	for _, top := range []int{0, 80} {
		for y := top; y < top+20; y++ {
			for x := 20; x < 40; x++ {
				page.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	for _, tc := range []struct {
		name      string
		positions []pdf.Position
	}{
		{"page edges", []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 90, Top: 10, Bottom: 90}}},
		{"cross page", []pdf.Position{
			{PageNumbers: []int{0}, Left: 10, Right: 90, Top: 90, Bottom: 100},
			{PageNumbers: []int{1}, Left: 10, Right: 90, Top: 0, Bottom: 10},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img, err := DecodeBase64PNG(CropTextSectionPositions(tc.positions, map[int]image.Image{0: page, 1: page}, 1))
			if err != nil {
				t.Fatal(err)
			}
			rows := 0
			for y := 0; y < img.Bounds().Dy(); y++ {
				if color.RGBAModel.Convert(img.At(0, y)) == (color.RGBA{255, 255, 255, 255}) &&
					color.RGBAModel.Convert(img.At(10, y)) == (color.RGBA{0, 0, 0, 255}) {
					rows++
				}
			}
			if rows != 40 {
				t.Fatalf("whole body lines = %d rows, want 40", rows)
			}
		})
	}
}

func TestCropTextSectionPositionsKeepsColumnsSeparate(t *testing.T) {
	page := makeTestPageImage(100, 400, color.RGBA{255, 255, 255, 255}).(*image.RGBA)
	for i, left := range []int{20, 70} {
		for y := 140; y < 158; y++ {
			for x := left; x < left+10; x++ {
				page.SetRGBA(x, y, color.RGBA{uint8(40 + 40*i), 0, 0, 255})
			}
		}
	}
	positions := []pdf.Position{
		{PageNumbers: []int{0}, Left: 10, Right: 50, Top: 130, Bottom: 170},
		{PageNumbers: []int{0}, Left: 60, Right: 90, Top: 130, Bottom: 170},
	}
	img, err := DecodeBase64PNG(CropTextSectionPositions(positions, map[int]image.Image{0: page}, 1))
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[uint8]int)
	for y := 0; y < img.Bounds().Dy(); y++ {
		p := color.RGBAModel.Convert(img.At(10, y)).(color.RGBA)
		if p.G == 0 && p.B == 0 {
			counts[p.R]++
		}
	}
	if want := (map[uint8]int{40: 18, 80: 18}); !reflect.DeepEqual(counts, want) {
		t.Fatalf("column line rows = %v, want %v", counts, want)
	}
}
