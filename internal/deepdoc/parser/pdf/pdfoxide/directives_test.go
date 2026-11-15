package pdfoxide

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// referenceRotationScan is the per-page scanner scanPageDirectives replaced.
// It is kept here verbatim so the merged scan can be proven equivalent on
// every input, including malformed ones.
func referenceRotationScan(data []byte, pageIdx int) int {
	var rotations []int
	rest := data
	for {
		idx := -1
		for i := 0; i < len(rest)-7; i++ {
			if rest[i] == '/' && rest[i+1] == 'R' && rest[i+2] == 'o' &&
				rest[i+3] == 't' && rest[i+4] == 'a' && rest[i+5] == 't' &&
				rest[i+6] == 'e' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		rest = rest[idx+7:]
		for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n' || rest[0] == '\r') {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			break
		}
		val := 0
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			val = val*10 + int(rest[i]-'0')
			i++
		}
		if i > 0 {
			rotations = append(rotations, val)
		}
		rest = rest[i:]
	}
	if pageIdx < len(rotations) {
		return rotations[pageIdx]
	}
	return 0
}

// referenceIndexAfter is the byte search the previous CropBox scanner used.
func referenceIndexAfter(data []byte, s string) int {
	for i := 0; i < len(data)-len(s); i++ {
		match := true
		for j := 0; j < len(s); j++ {
			if data[i+j] != s[j] {
				match = false
				break
			}
		}
		if match {
			return i + len(s)
		}
	}
	return -1
}

// referenceCropBoxScan is the per-page /CropBox scanner scanPageDirectives
// replaced.
func referenceCropBoxScan(data []byte, pageIdx int) ([4]float64, bool) {
	type cb [4]float64
	var boxes []cb
	rest := data
	for {
		idx := referenceIndexAfter(rest, "/CropBox")
		if idx < 0 {
			break
		}
		rest = rest[idx:]
		for len(rest) > 0 && isSpace(rest[0]) {
			rest = rest[1:]
		}
		if len(rest) == 0 || rest[0] != '[' {
			continue
		}
		rest = rest[1:]
		var vals [4]float64
		ok := true
		for i := 0; i < 4; i++ {
			for len(rest) > 0 && isSpace(rest[0]) {
				rest = rest[1:]
			}
			v, n := parseFloat(rest)
			if n == 0 {
				ok = false
				break
			}
			vals[i] = v
			rest = rest[n:]
		}
		if !ok {
			continue
		}
		boxes = append(boxes, cb(vals))
	}
	if pageIdx < len(boxes) {
		return boxes[pageIdx], true
	}
	return [4]float64{}, false
}

// comparedPages bounds how many leading page indexes each comparison checks.
// It exceeds the page count of the shipped fixtures so real PDFs are covered
// end to end.
const comparedPages = 32

// compareScanners asserts that the merged scan reports exactly what the two
// former per-page scanners reported for every page index in range.
func compareScanners(t *testing.T, data []byte, label string) {
	t.Helper()
	got := scanPageDirectives(data)
	for page := 0; page < comparedPages; page++ {
		wantRot := referenceRotationScan(data, page)
		if d := got[page]; d.rotate != wantRot {
			t.Fatalf("%s: page %d rotate = %d, want %d", label, page, d.rotate, wantRot)
		}
		wantBox, wantOK := referenceCropBoxScan(data, page)
		d := got[page]
		if d.hasCrop != wantOK {
			t.Fatalf("%s: page %d hasCrop = %v, want %v", label, page, d.hasCrop, wantOK)
		}
		if wantOK {
			for i := 0; i < 4; i++ {
				if d.cropBox[i] != wantBox[i] {
					t.Fatalf("%s: page %d cropBox[%d] = %v, want %v", label, page, i, d.cropBox[i], wantBox[i])
				}
			}
		}
	}
}

func TestScanPageDirectivesMatchesPreviousScanners(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"no directives", "/MediaBox [0 0 595 842] /Type /Page"},
		{"rotate only", "/Rotate 90"},
		{"cropbox only", "/CropBox [0 0 595 842]"},
		{"no space after key", "/CropBox[0 0 595 842]/Rotate 180"},
		{"extra whitespace", "/CropBox  [  0.5   10.25   595.3   842.0  ]"},
		{"negative cropbox", "/CropBox [-10 -20 595 842]"},
		{"interleaved pages", "/Rotate 90 /CropBox [0 0 1 1] /Rotate 270 /CropBox [2 2 3 3]"},
		{"cropbox before rotate", "/CropBox [5 5 6 6] /Rotate 90"},
		{"non-numeric rotate is skipped", "/Rotate </Rotate 90"},
		{"negative rotate is skipped", "/Rotate -90 /Rotate 90"},
		{"malformed cropbox is skipped", "/CropBox [0 0 595] /CropBox [0 0 1 1]"},
		{"cropbox without array", "/CropBox /Rotate 90 /CropBox [0 0 1 1]"},
		{"trailing rotate", "/Rotate 90<<>>"},
		{"keyword inside value", "/CropBox [0 0 595 842] /Rotate 90 endobj /Rotate 180"},
		{"media box only", "/MediaBox [0 0 612 792] endobj /MediaBox [0 0 612 792]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compareScanners(t, []byte(tc.raw), tc.name)
		})
	}
}

func TestScanPageDirectivesRandomizedEquivalence(t *testing.T) {
	tokens := []string{
		"/Rotate", " 90", " 0", " 270", " -45", " </Rotate", " abc",
		"/CropBox", " [0 0 1 1]", " [ 10.5 -2 30 40 ]", " [0 0 595]",
		" ", "\n", "/MediaBox", "[0 0 9 9]", "endobj", "/Type /Page",
	}
	rng := rand.New(rand.NewSource(20261104))
	for iter := 0; iter < 300; iter++ {
		var buf []byte
		for n := rng.Intn(40); n > 0; n-- {
			buf = append(buf, tokens[rng.Intn(len(tokens))]...)
		}
		compareScanners(t, buf, "random")
	}
}

// TestScanPageDirectivesOnRealPDFs sweeps every PDF shipped in testdata so the
// merged scan is compared against both former scanners on real byte streams,
// not only on synthetic ones.
func TestScanPageDirectivesOnRealPDFs(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "testdata", "real_pdfs", "*.pdf"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no real PDFs in ../testdata/real_pdfs")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		compareScanners(t, data, filepath.Base(path))
	}
}

func TestScanPageDirectivesCropBox(t *testing.T) {
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
			d := scanPageDirectives([]byte(tt.raw))[tt.pageIdx]
			got, ok := d.cropBox, d.hasCrop
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
