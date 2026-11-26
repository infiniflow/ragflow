// Copied from github.com/mgilbir/gowemf, file color_transform.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.
// ivy: the transforms use internal/moxcms, not golittlecms (D24). The names
// and the checks of the envelope stay. A calibrated RGB space becomes a
// small ICC profile that moxcms reads. BlackPointCompensation is ignored,
// because the subset of moxcms has no option for it.

package gowemf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"ragflow/internal/thirdparty/ivy"
	"ragflow/internal/thirdparty/ivy/internal/moxcms"
)

// ColorTransformOptions adds profile/transform limits. Defaults: 16 MiB profile
// bytes (also the sum of referenced tag sizes), 4096 tags, 16 million pixels per
// TransformRGBA call. golittlecms retains its own allocation and parser guards.
// ResolveProfile is optional and is invoked only for a named profile without
// embedded bytes. The application decides whether/how to resolve the opaque name;
// neither this adapter nor the parser opens a path supplied by a metafile.
//
// ivy: moxcms has no black point compensation, so BlackPointCompensation
// changes nothing.
type ColorTransformOptions struct {
	MaxProfileBytes, MaxPixels uint64
	MaxProfileTags             uint32
	BlackPointCompensation     bool
	ResolveProfile             func(ColorSpace) ([]byte, error)
}

func (o ColorTransformOptions) defaults() ColorTransformOptions {
	if o.MaxProfileBytes == 0 {
		o.MaxProfileBytes = 16 << 20
	}
	if o.MaxProfileTags == 0 {
		o.MaxProfileTags = 4096
	}
	if o.MaxPixels == 0 {
		o.MaxPixels = 16_000_000
	}
	return o
}

// NewColorTransform creates a golittlecms RGB-to-sRGB converter. It supports
// embedded RGB ICC profiles, calibrated RGB endpoints/gamma and sRGB. Windows
// intent values are explicitly mapped to ICC intents rather than cast.
//
// ivy: the converter is a transform of moxcms.
func NewColorTransform(space ColorSpace) (RGBTransform, error) {
	return NewColorTransformWithOptions(space, ColorTransformOptions{})
}

func NewColorTransformWithOptions(space ColorSpace, options ColorTransformOptions) (RGBTransform, error) {
	o := options.defaults()
	intent, err := space.ICCIntent()
	if err != nil {
		return nil, err
	}
	input, err := openColorProfile(space, o)
	if err != nil {
		return nil, err
	}
	if input.ColorSpace != moxcms.DataColorSpaceRgb {
		return nil, failure(0, "RGB source profile required", ErrUnsupported)
	}
	transform, err := input.CreateTransform8Bit(moxcms.LayoutRgba, moxcms.NewSrgb(), moxcms.LayoutRgba, transformOptions(intent))
	if err != nil {
		return nil, colorError("color transform", err)
	}
	return &cmsRGBTransform{transform, o.MaxPixels}, nil
}

// ivy: transformOptions returns the options of moxcms for an ICC intent.
func transformOptions(intent uint32) moxcms.TransformOptions {
	opts := moxcms.DefaultTransformOptions()
	opts.RenderingIntent = moxcms.RenderingIntent(intent)
	return opts
}

// ivy: colorError wraps an error of moxcms with the error of gowemf that
// matches it.
func colorError(field string, err error) error {
	switch {
	case errors.Is(err, ivy.ErrUnsupported):
		return fmt.Errorf("gowemf: %s: %w: %w", field, ErrUnsupported, err)
	case errors.Is(err, ivy.ErrLimit):
		return fmt.Errorf("gowemf: %s: %w: %w", field, ErrLimit, err)
	}
	return fmt.Errorf("gowemf: %s: %w: %w", field, ErrMalformed, err)
}

type cmsRGBTransform struct {
	transform moxcms.TransformExecutor
	maxPixels uint64
}

func (t *cmsRGBTransform) TransformRGBA(dst, src []byte) error {
	if len(src)%4 != 0 || len(dst) != len(src) {
		return malformed(0, "RGBA transform buffer length")
	}
	n := uint64(len(src) / 4)
	if n > t.maxPixels || n > uint64(^uint32(0))/4 {
		return failure(0, "color transform pixels", ErrLimit)
	}
	if err := t.transform.Transform(src, dst); err != nil {
		return colorError("color transform", err)
	}
	return nil
}

func openColorProfile(s ColorSpace, o ColorTransformOptions) (*moxcms.ColorProfile, error) {
	if uint64(len(s.Name))+uint64(len(s.Profile)) > o.MaxProfileBytes {
		return nil, failure(0, "color profile bytes", ErrLimit)
	}
	if s.IsSRGB() {
		return moxcms.NewSrgb(), nil
	}
	if s.Type != ColorCalibratedRGB && s.Type != ColorProfileEmbedded && s.Type != ColorProfileLinked {
		return nil, failure(0, "source color space", ErrUnsupported)
	}
	data := s.Profile
	hasName := len(s.Name) > 0 && s.Name[0] != 0
	if s.Unicode {
		if len(s.Name)%2 != 0 {
			return nil, malformed(0, "UTF-16 profile name")
		}
		hasName = len(s.Name) >= 2 && u16(s.Name) != 0
	}
	if len(data) == 0 && hasName {
		if o.ResolveProfile == nil {
			return nil, failure(0, "named color profile requires a resolver", ErrUnsupported)
		}
		var err error
		data, err = o.ResolveProfile(s)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, malformed(0, "empty resolved profile")
		}
	}
	if len(data) != 0 {
		checked, err := checkICCEnvelope(data, o)
		if err != nil {
			return nil, err
		}
		return readProfile(checked, o)
	}
	if s.Type != ColorCalibratedRGB {
		return nil, malformed(0, "missing ICC profile")
	}
	var white XYZ
	for i, p := range s.Endpoints {
		sum := p.X + p.Y + p.Z
		if !finite(p.X) || !finite(p.Y) || !finite(p.Z) || !finite(sum) || sum == 0 || !finite(s.Gamma[i]) || s.Gamma[i] <= 0 {
			return nil, malformed(0, "calibrated RGB endpoints/gamma")
		}
		white.X += p.X
		white.Y += p.Y
		white.Z += p.Z
	}
	if !finite(white.X+white.Y+white.Z) || white.X+white.Y+white.Z == 0 || white.Y <= 0 {
		return nil, malformed(0, "calibrated RGB white point")
	}
	data, err := calibratedProfile(s.Endpoints, white, s.Gamma)
	if err != nil {
		return nil, err
	}
	return readProfile(data, o)
}

// ivy: readProfile reads an ICC profile with moxcms, under the limits of o.
func readProfile(data []byte, o ColorTransformOptions) (*moxcms.ColorProfile, error) {
	opts := moxcms.DefaultParsingOptions()
	opts.MaxProfileSize = int(min(o.MaxProfileBytes, math.MaxInt32-1)) + 1
	p, err := moxcms.NewFromSliceWithOptions(data, opts)
	if err != nil {
		return nil, colorError("ICC profile", err)
	}
	return p, nil
}

// ivy: calibratedProfile returns an ICC profile of version 4 for a
// calibrated RGB space. It replaces CreateRGBProfile of golittlecms. The
// endpoints are scaled so that the white has a Y of 1, and adapted from the
// white to the white of the PCS with the Bradford transform, as ICC.1 Annex E
// says. Each channel has a parametric curve of type 0, a pure gamma.
func calibratedProfile(endpoints [3]XYZ, white XYZ, gamma [3]float64) ([]byte, error) {
	d50 := XYZ{0.9642, 1, 0.8249}
	adapt := bradford(XYZ{white.X / white.Y, 1, white.Z / white.Y}, d50)
	var colorants [3]XYZ
	for i, p := range endpoints {
		colorants[i] = adapt.apply(XYZ{p.X / white.Y, p.Y / white.Y, p.Z / white.Y})
		if !finite(colorants[i].X) || !finite(colorants[i].Y) || !finite(colorants[i].Z) {
			return nil, malformed(0, "calibrated RGB chromaticity")
		}
	}
	xyzTag := func(v XYZ) []byte {
		b := make([]byte, 20)
		copy(b, "XYZ ")
		for i, c := range []float64{v.X, v.Y, v.Z} {
			binary.BigEndian.PutUint32(b[8+4*i:], uint32(s15Fixed16(c)))
		}
		return b
	}
	paraTag := func(g float64) []byte {
		b := make([]byte, 16)
		copy(b, "para")
		binary.BigEndian.PutUint32(b[12:], uint32(s15Fixed16(g)))
		return b
	}
	tags := []struct {
		sig  string
		data []byte
	}{
		{"wtpt", xyzTag(d50)},
		{"rXYZ", xyzTag(colorants[0])},
		{"gXYZ", xyzTag(colorants[1])},
		{"bXYZ", xyzTag(colorants[2])},
		{"rTRC", paraTag(gamma[0])},
		{"gTRC", paraTag(gamma[1])},
		{"bTRC", paraTag(gamma[2])},
	}
	off := 128 + 4 + 12*len(tags)
	b := make([]byte, off, off+20*4+16*3)
	be := binary.BigEndian
	be.PutUint32(b[8:], 0x04300000)
	copy(b[12:], "mntrRGB XYZ ")
	copy(b[36:], "acsp")
	for i, c := range []float64{d50.X, d50.Y, d50.Z} {
		be.PutUint32(b[68+4*i:], uint32(s15Fixed16(c)))
	}
	be.PutUint32(b[128:], uint32(len(tags)))
	for i, t := range tags {
		entry := b[132+12*i:]
		copy(entry, t.sig)
		be.PutUint32(entry[4:], uint32(len(b)))
		be.PutUint32(entry[8:], uint32(len(t.data)))
		b = append(b, t.data...)
	}
	be.PutUint32(b, uint32(len(b)))
	return b, nil
}

// ivy: s15Fixed16 encodes v as an s15Fixed16Number of ICC.1, clamped.
func s15Fixed16(v float64) int32 {
	return int32(math.Round(math.Max(-32768, math.Min(32767, v)) * 65536))
}

// ivy: matrix3 is a 3 by 3 matrix, by rows.
type matrix3 [3][3]float64

func (m matrix3) apply(v XYZ) XYZ {
	return XYZ{
		m[0][0]*v.X + m[0][1]*v.Y + m[0][2]*v.Z,
		m[1][0]*v.X + m[1][1]*v.Y + m[1][2]*v.Z,
		m[2][0]*v.X + m[2][1]*v.Y + m[2][2]*v.Z,
	}
}

func (m matrix3) mul(n matrix3) matrix3 {
	var r matrix3
	for i := range 3 {
		for j := range 3 {
			for k := range 3 {
				r[i][j] += m[i][k] * n[k][j]
			}
		}
	}
	return r
}

// ivy: bradford returns the Bradford transform that adapts the white src to
// the white dst.
func bradford(src, dst XYZ) matrix3 {
	b := matrix3{{0.8951, 0.2664, -0.1614}, {-0.7502, 1.7135, 0.0367}, {0.0389, -0.0685, 1.0296}}
	bi := matrix3{{0.9869929, -0.1470543, 0.1599627}, {0.4323053, 0.5183603, 0.0492912}, {-0.0085287, 0.0400428, 0.9684867}}
	s, d := b.apply(src), b.apply(dst)
	scale := matrix3{{d.X / s.X, 0, 0}, {0, d.Y / s.Y, 0}, {0, 0, d.Z / s.Z}}
	return bi.mul(scale).mul(b)
}

// This is an ICC envelope/resource check, not a second ICC implementation.
// Tag semantics and transform construction are delegated to golittlecms.
//
// ivy: they are delegated to moxcms.
func checkICCEnvelope(data []byte, o ColorTransformOptions) ([]byte, error) {
	if uint64(len(data)) > o.MaxProfileBytes {
		return nil, failure(0, "ICC bytes", ErrLimit)
	}
	if len(data) < 132 {
		return nil, malformed(0, "ICC header")
	}
	be := binary.BigEndian
	n := uint64(be.Uint32(data))
	count := uint64(be.Uint32(data[128:]))
	if n < 132 || n > uint64(len(data)) || be.Uint32(data[36:]) != 0x61637370 {
		return nil, malformed(0, "ICC size/signature")
	}
	if count > uint64(o.MaxProfileTags) {
		return nil, failure(128, "ICC tag count", ErrLimit)
	}
	end := uint64(132) + count*12
	if end > n {
		return nil, malformed(128, "ICC tag table")
	}
	var referenced uint64
	for i := uint64(0); i < count; i++ {
		entry := data[132+int(i)*12:]
		off, size := uint64(be.Uint32(entry[4:])), uint64(be.Uint32(entry[8:]))
		if off < end || off%4 != 0 || size < 8 || off > n || size > n-off {
			return nil, malformed(132+int(i)*12, "ICC tag span")
		}
		if size > o.MaxProfileBytes-referenced {
			return nil, failure(132+int(i)*12, "ICC referenced tag bytes", ErrLimit)
		}
		referenced += size
	}
	return data[:int(n):int(n)], nil
}
