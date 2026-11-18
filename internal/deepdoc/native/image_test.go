//go:build cgo

package native

import (
	"image"
	"testing"
)

func TestCheckImageBounds(t *testing.T) {
	ok := func(w, h int) bool {
		return checkImageBounds(image.Rect(0, 0, w, h)) == nil
	}
	if !ok(100, 100) {
		t.Fatal("normal image rejected")
	}
	if !ok(10000, 10000) { // == maxImagePixels, within both caps
		t.Fatal("max-sized image rejected")
	}

	cases := []struct {
		name string
		w, h int
	}{
		{"empty width", 0, 100},
		{"empty height", 100, 0},
		{"oversized width", maxImageDim + 1, 100},
		{"oversized height", 100, maxImageDim + 1},
		{"too many pixels", maxImageDim, maxImageDim + 1}, // exceeds maxImagePixels
	}
	for _, c := range cases {
		if err := checkImageBounds(image.Rect(0, 0, c.w, c.h)); err == nil {
			t.Errorf("%s: expected rejection for %dx%d", c.name, c.w, c.h)
		}
	}
}
