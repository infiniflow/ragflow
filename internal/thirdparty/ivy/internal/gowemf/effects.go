// Copied from github.com/mgilbir/gowemf, file effects.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"image"
	"image/color"
	"math"
)

// ApplyImageEffect executes the exact per-channel lookup and affine color-matrix
// effects defined by MS-EMFPLUS. Values are interpreted as straight RGBA, matrix
// inputs/outputs are normalized to [0,1], and results are clamped and rounded to
// RGBA8. Other effect algorithms return ErrUnsupported rather than an approximate
// rendering. The source remains unchanged and nonzero image origins are handled.
func ApplyImageEffect(src image.Image, effect PlusEffect, limits ImageLimits) (*image.NRGBA, error) {
	if src == nil {
		return nil, malformed(0, "nil effect image")
	}
	l := limits.defaults()
	bounds := src.Bounds()
	if bounds.Max.X <= bounds.Min.X || bounds.Max.Y <= bounds.Min.Y {
		return nil, malformed(0, "effect image dimensions")
	}
	w, h := uint64(bounds.Max.X)-uint64(bounds.Min.X), uint64(bounds.Max.Y)-uint64(bounds.Min.Y)
	if w > l.MaxPixels/h || w > uint64(int(^uint(0)>>1))/4/h || w > l.MaxDecodedBytes/4/h {
		return nil, failure(0, "effect pixel budget", ErrLimit)
	}
	var table ColorLookupTableEffect
	var matrix ColorMatrixEffect
	switch effect.Kind {
	case EffectColorLookupTable:
		var ok bool
		table, ok = effect.Parameters.(ColorLookupTableEffect)
		if !ok || len(table.Red) != 256 || len(table.Green) != 256 || len(table.Blue) != 256 || len(table.Alpha) != 256 {
			return nil, malformed(0, "lookup effect parameters")
		}
	case EffectColorMatrix:
		var ok bool
		matrix, ok = effect.Parameters.(ColorMatrixEffect)
		if !ok {
			return nil, malformed(0, "matrix effect parameters")
		}
		for row := range matrix.Rows {
			for _, v := range matrix.Rows[row] {
				if !finite(v) {
					return nil, malformed(0, "non-finite effect matrix")
				}
			}
			if row < 4 && matrix.Rows[row][4] != 0 {
				return nil, malformed(0, "non-affine effect matrix")
			}
		}
	default:
		return nil, failure(0, "image effect playback", ErrUnsupported)
	}
	out := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			p := color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			if effect.Kind == EffectColorLookupTable {
				p = color.NRGBA{table.Red[p.R], table.Green[p.G], table.Blue[p.B], table.Alpha[p.A]}
			} else {
				in := [4]float64{float64(p.R) / 255, float64(p.G) / 255, float64(p.B) / 255, float64(p.A) / 255}
				var channels [4]byte
				for col := 0; col < 4; col++ {
					v := matrix.Rows[4][col]
					for row := 0; row < 4; row++ {
						v += in[row] * matrix.Rows[row][col]
					}
					if !finite(v) {
						return nil, malformed(0, "effect matrix arithmetic overflow")
					}
					if v < 0 {
						v = 0
					}
					if v > 1 {
						v = 1
					}
					channels[col] = byte(math.Round(v * 255))
				}
				p = color.NRGBA{channels[0], channels[1], channels[2], channels[3]}
			}
			out.SetNRGBA(x, y, p)
		}
	}
	return out, nil
}
