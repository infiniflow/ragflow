package pdfoxide

import (
	"math"
	"testing"
)

func TestParseCropBoxFromRaw(t *testing.T) {
	eps := 1e-6

	tests := []struct {
		name    string
		raw     string
		pageIdx int
		want    [4]float64
		ok      bool
	}{
		{
			name: "standard A4 portrait",
			raw:  "/CropBox [0 0 595.28 841.89]",
			want: [4]float64{0, 0, 595.28, 841.89},
			ok:   true,
		},
		{
			name: "non-zero origin",
			raw:  "/CropBox [30 20 575 832]",
			want: [4]float64{30, 20, 575, 832},
			ok:   true,
		},
		{
			name: "with extra whitespace",
			raw:  "/CropBox  [  0.5   10.25   595.3   842.0  ]",
			want: [4]float64{0.5, 10.25, 595.3, 842.0},
			ok:   true,
		},
		{
			name: "no spaces inside brackets",
			raw:  "/CropBox[0 0 595 842]",
			want: [4]float64{0, 0, 595, 842},
			ok:   true,
		},
		{
			name:    "page index 1 picks second CropBox",
			raw:     "/CropBox [0 0 1 1] /Rotate 90 /CropBox [2 2 3 3]",
			pageIdx: 1,
			want:    [4]float64{2, 2, 3, 3},
			ok:      true,
		},
		{
			name:    "page index out of range",
			raw:     "/CropBox [0 0 1 1]",
			pageIdx: 5,
			want:    [4]float64{},
			ok:      false,
		},
		{
			name: "no cropbox",
			raw:  "/MediaBox [0 0 595 842] /Rotate 90",
			want: [4]float64{},
			ok:   false,
		},
		{
			name: "empty input",
			raw:  "",
			want: [4]float64{},
			ok:   false,
		},
		{
			name: "incomplete array — fewer than 4 values",
			raw:  "/CropBox [0 0 595]",
			want: [4]float64{},
			ok:   false,
		},
		{
			name: "negative values",
			raw:  "/CropBox [-10 -20 595 842]",
			want: [4]float64{-10, -20, 595, 842},
			ok:   true,
		},
		{
			name: "real pypdf output format (multiple spaces, decimals)",
			raw:  "/Type /Page /MediaBox [0 0 595.2756 841.8898] /CropBox [30.0 20.0 575.0 832.0] /Rotate 90",
			want: [4]float64{30.0, 20.0, 575.0, 832.0},
			ok:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseCropBoxFromRaw([]byte(tt.raw), tt.pageIdx)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			for i := 0; i < 4; i++ {
				if math.Abs(got[i]-tt.want[i]) > eps {
					t.Errorf("[%d]: got %.4f, want %.4f", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		s    string
		want float64
		n    int
	}{
		{"0", 0, 1},
		{"595.28", 595.28, 6},
		{"  42", 42, 4},
		{"-10.5", -10.5, 5},
		{"+3.14", 3.14, 5},
		{"123abc", 123, 3},
		{"abc", 0, 0},
		{"", 0, 0},
		{".5", 0.5, 2},
	}
	for _, tt := range tests {
		v, n := parseFloat([]byte(tt.s))
		if n != tt.n || math.Abs(v-tt.want) > 1e-6 {
			t.Errorf("parseFloat(%q) = (%.4f, %d), want (%.4f, %d)",
				tt.s, v, n, tt.want, tt.n)
		}
	}
}
