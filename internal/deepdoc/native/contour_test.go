//go:build cgo

package native

import "testing"

// makeSeg builds a w x h seg from a list of filled (x,y) pixels.
func makeSeg(w, h int, fill []struct{ x, y int }) []bool {
	seg := make([]bool, w*h)
	for _, p := range fill {
		seg[p.y*w+p.x] = true
	}
	return seg
}

func rect(w, h int) []struct{ x, y int } {
	var pts []struct{ x, y int }
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pts = append(pts, struct{ x, y int }{x, y})
		}
	}
	return pts
}

func TestFindContoursSynthetic(t *testing.T) {
	cases := []struct {
		name     string
		w, h     int
		fill     []struct{ x, y int }
		wantCons int
	}{
		{"solid4x4", 4, 4, rect(4, 4), 1},
		{"solid6x6", 6, 6, rect(6, 6), 1},
		{"singlePixel", 3, 3, []struct{ x, y int }{{1, 1}}, 0},
		{"twoSquares", 6, 3, append(rect(2, 2), []struct{ x, y int }{{4, 0}, {5, 0}, {4, 1}, {5, 1}}...), 2},
		// 1x3 horizontal line.
		{"hline", 3, 1, []struct{ x, y int }{{0, 0}, {1, 0}, {2, 0}}, 1},
		// 2x2 block.
		{"block2x2", 2, 2, rect(2, 2), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seg := makeSeg(tc.w, tc.h, tc.fill)
			got := findContours(seg, tc.w, tc.h, 0)
			if len(got) != tc.wantCons {
				t.Fatalf("%s: got %d contours, want %d", tc.name, len(got), tc.wantCons)
			}
			// Every contour must have >=3 points and stay in bounds.
			for i, c := range got {
				if len(c) < 3 {
					t.Fatalf("%s: contour %d has %d points (<3)", tc.name, i, len(c))
				}
				for _, p := range c {
					if p.X < 0 || p.X >= float64(tc.w) || p.Y < 0 || p.Y >= float64(tc.h) {
						t.Fatalf("%s: contour %d point out of bounds: %v", tc.name, i, p)
					}
				}
			}
		})
	}
}
