package util

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWarpCropMatchesGolden locks the perspective de-skew behaviour of
// WarpCrop against a reference warp (perspective transform with bicubic
// resampling) generated offline by testdata/gen_warp_golden.py. The reference
// uses the same homogeneous mapping as WarpCrop, so this test pins the
// geometry (output size + de-skew) of the implementation. Minor
// resampling-kernel differences between the reference sampler and the Go
// Catmull-Rom sampler are absorbed by the MSE tolerance.
//
// This is the unit-tier (model-free) lock for the warp step: the perspective
// de-skew applied to OCR detection quads before recognition.
func TestWarpCropMatchesGolden(t *testing.T) {
	metaPath := filepath.Join("testdata", "warp_meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta struct {
		Src [4][2]float64 `json:"src"`
		W   int           `json:"w"`
		H   int           `json:"h"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("parse meta: %v", err)
	}
	var pts [4]Pt
	for i := range meta.Src {
		pts[i] = Pt{X: meta.Src[i][0], Y: meta.Src[i][1]}
	}

	src := loadGolden(t, filepath.Join("testdata", "warp_src.b64"))
	expected := loadGolden(t, filepath.Join("testdata", "warp_expected.b64"))

	got := WarpCrop(src, pts)

	// Output size must match the reference contract exactly:
	// W = int(max(|p0-p1|,|p2-p3|)), H = int(max(|p0-p3|,|p1-p2|)).
	if got.Bounds().Dx() != meta.W || got.Bounds().Dy() != meta.H {
		t.Fatalf("output size = %dx%d, want %dx%d",
			got.Bounds().Dx(), got.Bounds().Dy(), meta.W, meta.H)
	}
	if expected.Bounds().Dx() != meta.W || expected.Bounds().Dy() != meta.H {
		t.Fatalf("golden size = %dx%d, want %dx%d",
			expected.Bounds().Dx(), expected.Bounds().Dy(), meta.W, meta.H)
	}

	mse := imageMSE(got, expected)
	t.Logf("WarpCrop vs golden MSE = %.4f (RMSE/channel = %.4f)", mse, math.Sqrt(mse))

	// Generous enough to absorb resampling-kernel differences, tight enough
	// to catch a grossly wrong implementation (e.g. an axis-aligned crop of
	// the same quad would diverge by orders of magnitude on this skewed input).
	const maxMSE = 30.0
	if mse > maxMSE {
		t.Errorf("WarpCrop de-skew diverges from golden: MSE=%.4f > %.4f", mse, maxMSE)
	}

	// Sanity: WarpCrop must actually de-skew, not just return an axis-aligned
	// bbox crop of the quad. On this perspective (non-parallelogram) input the
	// output dimensions differ from the axis-aligned bbox, so the two are
	// trivially unequal — confirm that rather than asserting a number.
	bbox := axisFallback(src, pts)
	if bbox.Bounds().Dx() == meta.W && bbox.Bounds().Dy() == meta.H {
		t.Errorf("WarpCrop output size %dx%d equals the axis-aligned fallback size; warp may not be de-skewing",
			meta.W, meta.H)
	}
}

// TestWarpCropDegenerateQuadIsSafe checks that a collinear (degenerate) quad
// does not panic and returns a non-nil crop (falls back to axis-aligned).
func TestWarpCropDegenerateQuadIsSafe(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 50, 50))
	// All four corners on a single line -> singular homography.
	pts := [4]Pt{{10, 10}, {20, 10}, {30, 10}, {40, 10}}
	got := WarpCrop(src, pts)
	if got == nil {
		t.Fatal("WarpCrop returned nil for degenerate quad")
	}
	if got.Bounds().Dx() <= 0 || got.Bounds().Dy() <= 0 {
		t.Errorf("WarpCrop returned empty crop for degenerate quad: %dx%d",
			got.Bounds().Dx(), got.Bounds().Dy())
	}
}

// TestWarpCropAxisAlignedQuadIsStable checks that an already axis-aligned,
// axis-parallel quad is reproduced (up to bicubic resampling) without
// distortion — i.e. the output matches the source sub-rect.
func TestWarpCropAxisAlignedQuadIsStable(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill with a checkerboard so resampling has signal.
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if ((x/10)+(y/10))%2 == 0 {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				src.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	// Exact axis-aligned rectangle -> output should match the source sub-rect.
	pts := [4]Pt{{20, 20}, {80, 20}, {80, 70}, {20, 70}}
	got := WarpCrop(src, pts)
	if got.Bounds().Dx() != 60 || got.Bounds().Dy() != 50 {
		t.Fatalf("axis-aligned output size = %dx%d, want 60x50",
			got.Bounds().Dx(), got.Bounds().Dy())
	}
	// For an axis-parallel quad the warp is identity (just a sub-rect copy),
	// so it must match FastCrop of the same bbox up to resampling error.
	want := FastCrop(src, 20, 20, 80, 70)
	mse := imageMSE(got, want)
	t.Logf("axis-aligned WarpCrop vs FastCrop MSE = %.4f", mse)
	if mse > 5.0 {
		t.Errorf("axis-aligned warp diverged from the source sub-rect: MSE=%.4f > 5.0", mse)
	}
}

// loadGolden reads a single-line base64-encoded PNG fixture (committed as
// text so pre-commit text filters cannot corrupt the binary signature).
func loadGolden(t *testing.T, path string) *image.RGBA {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("base64 decode %s: %v", path, err)
	}
	img, err := png.Decode(bytes.NewReader(dec))
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return toRGBA(img)
}

// imageMSE returns the mean squared error across all RGBA channels between a
// and b (both must have identical dimensions).
func imageMSE(a, b *image.RGBA) float64 {
	ba, bb := a.Bounds(), b.Bounds()
	if ba.Dx() != bb.Dx() || ba.Dy() != bb.Dy() {
		return math.MaxFloat64
	}
	var acc float64
	n := ba.Dx() * ba.Dy()
	for y := 0; y < ba.Dy(); y++ {
		for x := 0; x < ba.Dx(); x++ {
			ca := a.RGBAAt(x, y)
			cb := b.RGBAAt(x, y)
			acc += sqDiff(ca.R, cb.R) + sqDiff(ca.G, cb.G) + sqDiff(ca.B, cb.B) + sqDiff(ca.A, cb.A)
		}
	}
	return acc / float64(n*4)
}

func sqDiff(x, y uint8) float64 {
	d := float64(x) - float64(y)
	return d * d
}

// TestWarpCropRespectsNonZeroOrigin guards the source-image bounds handling in
// sampleBicubic: a source with a non-zero origin (Min != (0,0)) must be sampled
// at its absolute coordinates, not relative to (0,0). WarpCrop on such an image
// must produce the same crop as WarpCrop on an equivalent (0,0)-origin image
// holding identical pixels at the same absolute coordinates.
//
// The quad is interior to both images so the sampler never reaches either
// image's edge; this isolates the origin handling from edge-replication
// differences and exercises the far-edge clamp where a zero-origin assumption
// would clamp too early.
func TestWarpCropRespectsNonZeroOrigin(t *testing.T) {
	gradient := func(x, y int) color.RGBA {
		return color.RGBA{uint8(x % 256), uint8(y % 256), uint8((x + y) % 256), 255}
	}

	// Non-zero-origin source with its own pixel buffer.
	origin := image.Pt(50, 50)
	sub := image.NewRGBA(image.Rect(origin.X, origin.Y, origin.X+200, origin.Y+200))
	for y := origin.Y; y < origin.Y+200; y++ {
		for x := origin.X; x < origin.X+200; x++ {
			sub.SetRGBA(x, y, gradient(x, y))
		}
	}
	// Equivalent (0,0)-origin image holding the same pixels at the same
	// absolute coordinates.
	flat := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			flat.SetRGBA(x, y, gradient(x, y))
		}
	}

	// Interior quad in absolute coordinates (so sampling stays away from both
	// images' edges).
	pts := [4]Pt{
		{X: 60, Y: 60},
		{X: 240, Y: 60},
		{X: 240, Y: 240},
		{X: 60, Y: 240},
	}

	gotSub := WarpCrop(sub, pts)
	gotFlat := WarpCrop(flat, pts)

	if gotSub.Bounds() != gotFlat.Bounds() {
		t.Fatalf("output size mismatch: sub=%v flat=%v", gotSub.Bounds(), gotFlat.Bounds())
	}
	// With correct origin handling the two are pixel-identical; a zero-origin
	// assumption clamps ~20% of the crop too early and diverges by orders of
	// magnitude.
	if mse := imageMSE(gotSub, gotFlat); mse > 1e-3 {
		t.Errorf("WarpCrop ignored the source image origin: MSE between sub- and flat-frame warps = %v", mse)
	}
}

// TestWarpCropRejectsMalformedQuad guards against a process-crashing panic /
// OOM on an out-of-range or non-finite detector quad. The old FastCrop path
// clamped coordinates to the source bounds before allocating; WarpCrop must be
// equally safe. A finite but absurd coordinate (e.g. 3e18) would otherwise
// reach image.NewRGBA and panic with "huge or negative dimensions", and a
// non-finite coordinate would drive an undefined-size allocation.
//
// This is a regression guard for the untrusted-boundary contract: OCRDetect
// accepts coordinates from the DocAnalyzer backend, and the detector clips its
// points, but that invariant is not enforced at this Go boundary.
func TestWarpCropRejectsMalformedQuad(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))

	cases := []struct {
		name string
		pts  [4]Pt
	}{
		{"huge x", [4]Pt{{0, 0}, {3e18, 0}, {3e18, 2}, {0, 2}}},
		{"huge negative", [4]Pt{{-3e18, 0}, {0, 0}, {0, 2}, {-3e18, 2}}},
		{"nan", [4]Pt{{0, 0}, {math.NaN(), 0}, {10, 10}, {0, 10}}},
		{"inf", [4]Pt{{0, 0}, {math.Inf(1), 0}, {10, 10}, {0, 10}}},
		{"outside bounds", [4]Pt{{-100, -100}, {200, -100}, {200, 200}, {-100, 200}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *image.RGBA
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("WarpCrop panicked on %q: %v", tc.name, r)
					}
				}()
				got = WarpCrop(src, tc.pts)
			}()
			if got == nil {
				t.Fatalf("WarpCrop returned nil on %q", tc.name)
			}
			w, h := got.Bounds().Dx(), got.Bounds().Dy()
			if w <= 0 || h <= 0 {
				t.Errorf("WarpCrop returned an empty crop on %q: %dx%d", tc.name, w, h)
			}
			if w > maxWarpDim || h > maxWarpDim {
				t.Errorf("WarpCrop returned an unbounded crop on %q: %dx%d", tc.name, w, h)
			}
		})
	}
}
