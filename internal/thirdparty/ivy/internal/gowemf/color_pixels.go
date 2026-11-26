// Copied from github.com/mgilbir/gowemf, file color_pixels.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.
// ivy: the transforms use internal/moxcms, not golittlecms (D24). moxcms has
// no proofing transform, so NewProofingColorTransform returns
// ErrUnsupported.

package gowemf

import (
	"ragflow/internal/thirdparty/ivy/internal/moxcms"
)

// ColorPixelFormat specifies interleaved, unsigned eight-bit source channels.
// CMYK8 uses 0=no ink and 255=full ink. Output is always straight sRGB RGBA8.
type ColorPixelFormat uint8

const (
	ColorRGB8 ColorPixelFormat = iota + 1
	ColorRGBA8
	ColorGray8
	ColorCMYK8
)

type PixelColorTransform struct {
	transform moxcms.TransformExecutor
	channels  int
	copyAlpha bool
	maxPixels uint64
}

func (t *PixelColorTransform) InputChannels() int {
	if t == nil {
		return 0
	}
	return t.channels
}
func (t *PixelColorTransform) TransformPixels(dst, src []byte) error {
	if t == nil || t.transform == nil || len(src)%t.channels != 0 {
		return malformed(0, "color pixel source")
	}
	n := uint64(len(src) / t.channels)
	if n > t.maxPixels || n > uint64(^uint32(0))/4 || n > uint64(int(^uint(0)>>1))/4 {
		return failure(0, "color pixel count", ErrLimit)
	}
	if uint64(len(dst)) != n*4 {
		return malformed(0, "color pixel destination")
	}
	if err := t.transform.Transform(src, dst); err != nil {
		return colorError("pixel color transform", err)
	}
	if !t.copyAlpha {
		for i := 3; i < len(dst); i += 4 {
			dst[i] = 255
		}
	}
	return nil
}

// NewPixelColorTransform builds an explicit 8-bit RGB/RGBA/gray/CMYK-to-sRGB
// converter. The ICC profile must agree with the declared source channel model.
func NewPixelColorTransform(space ColorSpace, format ColorPixelFormat, options ColorTransformOptions) (*PixelColorTransform, error) {
	o := options.defaults()
	intent, err := space.ICCIntent()
	if err != nil {
		return nil, err
	}
	// ivy: moxcms takes CMYK of 8 bits in the layout of RGBA.
	var layout moxcms.Layout
	var signature moxcms.DataColorSpace
	channels := 0
	alpha := false
	switch format {
	case ColorRGB8:
		layout, signature, channels = moxcms.LayoutRgb, moxcms.DataColorSpaceRgb, 3
	case ColorRGBA8:
		layout, signature, channels, alpha = moxcms.LayoutRgba, moxcms.DataColorSpaceRgb, 4, true
	case ColorGray8:
		layout, signature, channels = moxcms.LayoutGray, moxcms.DataColorSpaceGray, 1
	case ColorCMYK8:
		layout, signature, channels = moxcms.LayoutRgba, moxcms.DataColorSpaceCmyk, 4
	default:
		return nil, failure(0, "color pixel format", ErrUnsupported)
	}
	input, err := openColorProfile(space, o)
	if err != nil {
		return nil, err
	}
	if input.ColorSpace != signature {
		return nil, malformed(0, "profile/input channel mismatch")
	}
	transform, err := input.CreateTransform8Bit(layout, moxcms.NewSrgb(), moxcms.LayoutRgba, transformOptions(intent))
	if err != nil {
		return nil, colorError("pixel color transform", err)
	}
	return &PixelColorTransform{transform, channels, alpha, o.MaxPixels}, nil
}

// NewProofingColorTransform converts source RGB through an explicitly supplied
// proofing profile to sRGB. The proof profile can describe a CMYK printer, gray
// device or RGB output. Rendering/proof intents are mapped from each ColorSpace;
// no profile names are opened except through the supplied resolver callback.
//
// ivy: moxcms has no proofing transform. The function checks its arguments
// as upstream does, and then returns ErrUnsupported.
func NewProofingColorTransform(source, proof ColorSpace, options ColorTransformOptions) (RGBTransform, error) {
	o := options.defaults()
	if _, err := source.ICCIntent(); err != nil {
		return nil, err
	}
	if _, err := proof.ICCIntent(); err != nil {
		return nil, err
	}
	in, err := openColorProfile(source, o)
	if err != nil {
		return nil, err
	}
	if in.ColorSpace != moxcms.DataColorSpaceRgb {
		return nil, malformed(0, "proofing source must be RGB")
	}
	if _, err := openColorProfile(proof, o); err != nil {
		return nil, err
	}
	return nil, failure(0, "proofing transform", ErrUnsupported)
}
