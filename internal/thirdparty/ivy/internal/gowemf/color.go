// Copied from github.com/mgilbir/gowemf, file color.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"image"
	"image/color"
)

const (
	ColorCalibratedRGB         uint32 = 0
	ColorSRGB                  uint32 = 0x73524742
	ColorWindows               uint32 = 0x57696e20
	ColorProfileLinked         uint32 = 0x4c494e4b
	ColorProfileEmbedded       uint32 = 0x4d424544
	IntentBusiness             uint32 = 1
	IntentGraphics             uint32 = 2
	IntentImages               uint32 = 4
	IntentAbsoluteColorimetric uint32 = 8
)

// XYZ holds a CIE XYZ tristimulus vector, decoded from signed 2.30 fixed point.
type XYZ struct{ X, Y, Z float64 }

// ColorSpace describes source/profile colors. Calibration fields are RGB-only;
// embedded ICC profiles can describe RGB, gray or CMYK. Name and Profile borrow the input.
// Name is an opaque profile name (UTF-16LE when Unicode is true); the library
// never opens it. A supplied in-memory profile takes precedence over calibrated
// endpoints. sRGB/Windows spaces ignore calibration/profile fields per MS-WMF.
// Gamma uses unsigned 16.16 values, including LOGCOLORSPACE's 8.8 << 8 encoding.
type ColorSpace struct {
	Type, Intent  uint32
	Endpoints     [3]XYZ
	Gamma         [3]float64
	Name, Profile []byte
	Unicode       bool
}

func (s ColorSpace) IsSRGB() bool { return s.Type == ColorSRGB || s.Type == ColorWindows }

// ICCIntent converts Windows LCS_GM_* intent values to ICC intent numbers.
// Intent zero is an unspecified value used by V4 DIBs; it defaults to perceptual.
func (s ColorSpace) ICCIntent() (uint32, error) {
	switch s.Intent {
	case 0, IntentImages:
		return 0, nil
	case IntentGraphics:
		return 1, nil
	case IntentBusiness:
		return 2, nil
	case IntentAbsoluteColorimetric:
		return 3, nil
	}
	return 0, malformed(0, "color rendering intent")
}

// RGBTransform consumes straight, interleaved RGBA8 and produces straight sRGB
// RGBA8 of the same length, preserving alpha. Buffers must not overlap. An
// implementation must return errors for invalid buffers rather than silently
// clipping writes. ConvertToSRGB calls it once per row, with bounded buffers.
type RGBTransform interface{ TransformRGBA(dst, src []byte) error }
type ColorTransformFactory func(ColorSpace) (RGBTransform, error)

// ConvertToSRGB applies an explicit source color space. It can be used for a
// selected EMF logical color space as well as DIB metadata. No profile paths are
// opened. The output is a new zero-origin image; source and output allocations
// are separate. A nil factory is valid only for sRGB/Windows source colors.
func ConvertToSRGB(src image.Image, space ColorSpace, factory ColorTransformFactory, limits ImageLimits) (*image.NRGBA, error) {
	if src == nil {
		return nil, malformed(0, "nil color source image")
	}
	l := limits.defaults()
	bounds := src.Bounds()
	if bounds.Max.X <= bounds.Min.X || bounds.Max.Y <= bounds.Min.Y {
		return nil, malformed(0, "color source dimensions")
	}
	w, h := uint64(bounds.Max.X)-uint64(bounds.Min.X), uint64(bounds.Max.Y)-uint64(bounds.Min.Y)
	if w > l.MaxPixels/h || w > uint64(int(^uint(0)>>1))/4/h {
		return nil, failure(0, "color output pixels", ErrLimit)
	}
	if uint64(len(space.Profile))+uint64(len(space.Name)) > l.MaxBytes {
		return nil, failure(0, "color profile bytes", ErrLimit)
	}
	var transform RGBTransform
	if !space.IsSRGB() {
		if factory == nil {
			return nil, failure(0, "color conversion requires a transform", ErrUnsupported)
		}
		var err error
		transform, err = factory(space)
		if err != nil {
			return nil, err
		}
		if transform == nil {
			return nil, malformed(0, "nil color transform")
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
	row := make([]byte, int(w)*4)
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			v := color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			i := x * 4
			row[i], row[i+1], row[i+2], row[i+3] = v.R, v.G, v.B, v.A
		}
		dst := out.Pix[y*out.Stride : y*out.Stride+len(row)]
		if transform == nil {
			copy(dst, row)
		} else {
			if err := transform.TransformRGBA(dst, row); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// ColorSpace returns the parsed DIB color description. Borrowed metadata must
// remain immutable. Ordinary INFO/CORE DIBs use the library's RGB/sRGB default;
// use ConvertToSRGB explicitly when their enclosing DC selects another space.
func (d *DIB) ColorSpace() ColorSpace {
	if d == nil {
		return ColorSpace{Type: ColorSRGB}
	}
	return d.colorSpace
}

// ImageWithColorTransform converts a color-managed DIB into sRGB. Use
// NewColorTransform for the golittlecms backend, or supply another implementation.
func (d *DIB) ImageWithColorTransform(factory ColorTransformFactory) (image.Image, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	if !d.colorSpace.IsSRGB() && factory == nil {
		return nil, failure(0, "DIB color conversion", ErrUnsupported)
	}
	im, err := d.rawImage()
	if err != nil {
		return nil, err
	}
	if d.colorSpace.IsSRGB() {
		return im, nil
	}
	return ConvertToSRGB(im, d.colorSpace, factory, d.limits)
}

// AlphaImageWithColorTransform interprets the input as premultiplied BGRA before
// applying color conversion. Converted output is straight-alpha NRGBA; the
// returned image.Image interface preserves normal Go RGBA semantics either way.
func (d *DIB) AlphaImageWithColorTransform(factory ColorTransformFactory) (image.Image, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	if !d.colorSpace.IsSRGB() && factory == nil {
		return nil, failure(0, "alpha DIB color conversion", ErrUnsupported)
	}
	im, err := d.rawAlphaImage()
	if err != nil {
		return nil, err
	}
	if d.colorSpace.IsSRGB() {
		return im, nil
	}
	return ConvertToSRGB(im, d.colorSpace, factory, d.limits)
}
