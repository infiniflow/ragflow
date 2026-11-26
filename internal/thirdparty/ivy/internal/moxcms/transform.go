// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/transform.rs.

package moxcms

// Port: upstream makes the transforms generic over the type of a sample,
// and it builds transforms of 8, 10, 12 and 16 bits, and of f32 and f64. The
// subset has the transforms of 8 bits only, so the port has no type
// parameter. The const parameters of create_transform_8bit are the
// constants below.

const (
	// bitDepth8 is BIT_DEPTH of create_transform_8bit.
	bitDepth8 = 8
	// linearCap8 is LINEAR_CAP of create_transform_8bit, the number of
	// entries of a linearization table.
	linearCap8 = 256
	// gammaCap8 is GAMMA_CAP of create_transform_8bit, the number of bins of
	// a gamma table that hold values.
	gammaCap8 = 4096
	// gammaTableSize is the number of entries of a gamma table, as in the
	// boxed arrays of upstream.
	gammaTableSize = 65536
)

// TransformExecutor is TransformExecutor<u8> in moxcms src/transform.rs. It
// is a transform of 8 bits from one buffer to another.
type TransformExecutor interface {
	// Transform is TransformExecutor::transform in moxcms
	// src/transform.rs. It transforms the pixels of src into dst. The two
	// buffers must hold the same number of pixels.
	Transform(src, dst []uint8) error
}

// Transform8BitExecutor is Transform8BitExecutor in moxcms
// src/transform.rs.
type Transform8BitExecutor = TransformExecutor

// InPlaceTransformExecutor is InPlaceTransformExecutor<u8> in moxcms
// src/transform.rs. It is a transform of 8 bits that writes each pixel over
// itself.
type InPlaceTransformExecutor interface {
	// Transform is InPlaceTransformExecutor::transform in moxcms
	// src/transform.rs. It transforms the pixels of inOut in place.
	Transform(inOut []uint8) error
}

// Stage is Stage in moxcms src/transform.rs. It is a step of a transform
// on values of f32, from one buffer to another.
type Stage interface {
	Transform(src, dst []float32) error
}

// InPlaceStage is InPlaceStage in moxcms src/transform.rs. It is a step of a
// transform on values of f32, in place.
type InPlaceStage interface {
	Transform(dst []float32) error
}

// BarycentricWeightScale is BarycentricWeightScale in moxcms
// src/transform.rs. It is the precision of the weights of the interpolation
// in a LUT.
//
// Port: upstream has the variant High with the feature "options" only. hayro
// does not enable it, so the subset has Low only.
type BarycentricWeightScale uint8

// BarycentricWeightScaleLow is BarycentricWeightScale::Low in moxcms
// src/transform.rs.
const BarycentricWeightScaleLow BarycentricWeightScale = 0

// InterpolationMethod is InterpolationMethod in moxcms src/transform.rs. It
// is the interpolation in a 3D LUT.
//
// Port: upstream has the variants Tetrahedral, Pyramid and Prism with the
// feature "options" only. hayro does not enable it, so the subset has Linear
// only.
type InterpolationMethod uint8

// InterpolationMethodLinear is InterpolationMethod::Linear in moxcms
// src/transform.rs. It is the trilinear or quadlinear interpolation.
const InterpolationMethodLinear InterpolationMethod = 0

// TransformOptions is TransformOptions in moxcms src/transform.rs. It holds
// the options of a transform.
type TransformOptions struct {
	RenderingIntent RenderingIntent
	// AllowUseCicpTransfer makes a transform take the transfer curve of the
	// cicp tag of a profile, when the profile has one.
	AllowUseCicpTransfer bool
	// PreferFixedPoint makes a transform use fixed-point arithmetic where it
	// can: Q2.13 for a matrix shaper, and Q0.15 for a LUT.
	PreferFixedPoint       bool
	InterpolationMethod    InterpolationMethod
	BarycentricWeightScale BarycentricWeightScale
}

// DefaultTransformOptions is impl Default for TransformOptions in moxcms
// src/transform.rs.
func DefaultTransformOptions() TransformOptions {
	return TransformOptions{
		RenderingIntent:        RenderingIntentPerceptual,
		AllowUseCicpTransfer:   true,
		PreferFixedPoint:       true,
		InterpolationMethod:    InterpolationMethodLinear,
		BarycentricWeightScale: BarycentricWeightScaleLow,
	}
}

// Layout is Layout in moxcms src/transform.rs. It is the layout of the
// channels of a pixel. For RGB it is also the order of the channels. CMYK
// of 8 bits uses LayoutRgba.
type Layout uint8

// The layouts of Layout in moxcms src/transform.rs.
const (
	LayoutRgb       Layout = 0
	LayoutRgba      Layout = 1
	LayoutGray      Layout = 2
	LayoutGrayAlpha Layout = 3
	LayoutInks5     Layout = 4
	LayoutInks6     Layout = 5
	LayoutInks7     Layout = 6
	LayoutInks8     Layout = 7
	LayoutInks9     Layout = 8
	LayoutInks10    Layout = 9
	LayoutInks11    Layout = 10
	LayoutInks12    Layout = 11
	LayoutInks13    Layout = 12
	LayoutInks14    Layout = 13
	LayoutInks15    Layout = 14
	LayoutCmyka     Layout = 16
)

// Port: upstream panics in Layout::r_i, g_i, b_i and a_i for a layout
// without such a channel. The transforms of the port call them only for the
// layouts that have the channel.

// rI is Layout::r_i in moxcms src/transform.rs. It is the index of red.
func (l Layout) rI() int { return 0 }

// gI is Layout::g_i in moxcms src/transform.rs. It is the index of green.
func (l Layout) gI() int { return 1 }

// bI is Layout::b_i in moxcms src/transform.rs. It is the index of blue.
func (l Layout) bI() int { return 2 }

// aI is Layout::a_i in moxcms src/transform.rs. It is the index of alpha.
func (l Layout) aI() int {
	if l == LayoutGrayAlpha {
		return 1
	}
	return 3
}

// HasAlpha is Layout::has_alpha in moxcms src/transform.rs.
func (l Layout) HasAlpha() bool {
	return l == LayoutRgba || l == LayoutGrayAlpha
}

// Channels is Layout::channels in moxcms src/transform.rs. It is the number
// of channels of a pixel.
func (l Layout) Channels() int {
	switch l {
	case LayoutRgb:
		return 3
	case LayoutRgba:
		return 4
	case LayoutGray:
		return 1
	case LayoutGrayAlpha:
		return 2
	case LayoutCmyka:
		return 5
	default:
		// Inks5 to Inks15.
		return int(l) + 1
	}
}

// IsMatrixShaper is ColorProfile::is_matrix_shaper in moxcms
// src/transform.rs. It reports whether p is an RGB profile with colorants
// and tone curves.
func (p *ColorProfile) IsMatrixShaper() bool {
	return p.ColorSpace == DataColorSpaceRgb &&
		!p.RedColorant.Equal(Xyzd{}) &&
		!p.GreenColorant.Equal(Xyzd{}) &&
		!p.BlueColorant.Equal(Xyzd{}) &&
		p.RedTrc != nil && p.GreenTrc != nil && p.BlueTrc != nil
}

// CreateTransform8Bit is ColorProfile::create_transform_8bit in moxcms
// src/transform.rs. It returns the transform of samples of 8 bits from p, in
// srcLayout, to dst, in dstLayout.
func (p *ColorProfile) CreateTransform8Bit(srcLayout Layout, dst *ColorProfile, dstLayout Layout, options TransformOptions) (TransformExecutor, error) {
	// The table of Low has one weight for each sample of 8 bits.
	options.BarycentricWeightScale = BarycentricWeightScaleLow
	return p.createTransformNbit(srcLayout, dst, dstLayout, options)
}

// CreateInPlaceTransform8Bit is ColorProfile::create_in_place_transform_8bit
// in moxcms src/transform.rs. It returns the transform of samples of 8 bits
// from p to dst, in place, in layout. It transforms RGB to RGB and gray to
// gray only, with the same channels.
func (p *ColorProfile) CreateInPlaceTransform8Bit(layout Layout, dst *ColorProfile, options TransformOptions) (InPlaceTransformExecutor, error) {
	return p.createTransformInPlaceNbit(layout, dst, options)
}

// createTransformInPlaceNbit is ColorProfile::create_transform_in_place_nbit
// in moxcms src/transform.rs, for samples of 8 bits.
func (p *ColorProfile) createTransformInPlaceNbit(layout Layout, dst *ColorProfile, options TransformOptions) (InPlaceTransformExecutor, error) {
	// An in-place transform is a transform of matrix shapers only.
	isRgbTransform := p.ColorSpace == DataColorSpaceRgb &&
		dst.Pcs == DataColorSpaceXyz &&
		dst.ColorSpace == DataColorSpaceRgb &&
		p.Pcs == DataColorSpaceXyz &&
		p.IsMatrixShaper() &&
		dst.IsMatrixShaper()
	isGrayTransform := p.ColorSpace == DataColorSpaceGray && p.GrayTrc != nil &&
		dst.ColorSpace == DataColorSpaceGray && dst.GrayTrc != nil &&
		p.Pcs == DataColorSpaceXyz &&
		dst.Pcs == DataColorSpaceXyz

	if isRgbTransform {
		transform := p.TransformMatrix(dst)

		if p.areAllTrcTheSame() && dst.areAllTrcTheSame() {
			linear, err := p.buildRLinearizeTable(linearCap8, bitDepth8, options.AllowUseCicpTransfer)
			if err != nil {
				return nil, err
			}
			gamma, err := dst.buildGammaTable(dst.RedTrc, gammaCap8, bitDepth8, options.AllowUseCicpTransfer)
			if err != nil {
				return nil, err
			}
			profileTransform := transformMatrixShaperOptimized{
				linear:           linear,
				gamma:            gamma,
				adaptationMatrix: transform.ToF32(),
			}
			return makeInPlaceOptimizedTransform(layout, profileTransform, options)
		}

		shaper, err := p.buildTransformMatrixShaper(dst, transform, options)
		if err != nil {
			return nil, err
		}
		return makeInPlaceTransform(layout, shaper, options)
	}

	if isGrayTransform {
		// Gray to gray.
		grayLinear, err := p.buildGrayLinearizeTable(linearCap8, bitDepth8)
		if err != nil {
			return nil, err
		}
		grayGamma, err := dst.buildGammaTable(dst.GrayTrc, gammaCap8, bitDepth8, options.AllowUseCicpTransfer)
		if err != nil {
			return nil, err
		}
		return makeGrayToGrayInPlace(layout, grayLinear, grayGamma, bitDepth8, gammaCap8)
	}

	return nil, cmsError(UnsupportedProfileConnection)
}

// buildTransformMatrixShaper builds the TransformMatrixShaper of a matrix
// shaper with a curve for each channel, as create_transform_nbit and
// create_transform_in_place_nbit in moxcms src/transform.rs do.
func (p *ColorProfile) buildTransformMatrixShaper(dst *ColorProfile, transform Matrix3d, options TransformOptions) (transformMatrixShaper, error) {
	var s transformMatrixShaper
	var err error
	useCicp := options.AllowUseCicpTransfer
	if s.rLinear, err = p.buildRLinearizeTable(linearCap8, bitDepth8, useCicp); err != nil {
		return s, err
	}
	if s.gLinear, err = p.buildGLinearizeTable(linearCap8, bitDepth8, useCicp); err != nil {
		return s, err
	}
	if s.bLinear, err = p.buildBLinearizeTable(linearCap8, bitDepth8, useCicp); err != nil {
		return s, err
	}
	if s.rGamma, err = dst.buildGammaTable(dst.RedTrc, gammaCap8, bitDepth8, useCicp); err != nil {
		return s, err
	}
	if s.gGamma, err = dst.buildGammaTable(dst.GreenTrc, gammaCap8, bitDepth8, useCicp); err != nil {
		return s, err
	}
	if s.bGamma, err = dst.buildGammaTable(dst.BlueTrc, gammaCap8, bitDepth8, useCicp); err != nil {
		return s, err
	}
	s.adaptationMatrix = transform.ToF32()
	return s, nil
}

// createTransformNbit is ColorProfile::create_transform_nbit in moxcms
// src/transform.rs, for samples of 8 bits.
//
// Port: the subset leaves out three transforms of upstream that hayro never
// builds, because hayro transforms to sRGB only. They are gray to RGB when
// the RGB profile has a different curve for each channel, RGB to gray, and
// RGB or Lab to CMYK. The port returns UnsupportedProfileConnection for
// them.
func (p *ColorProfile) createTransformNbit(srcLayout Layout, dst *ColorProfile, dstLayout Layout, options TransformOptions) (TransformExecutor, error) {
	switch {
	case p.ColorSpace == DataColorSpaceRgb &&
		dst.Pcs == DataColorSpaceXyz &&
		dst.ColorSpace == DataColorSpaceRgb &&
		p.Pcs == DataColorSpaceXyz &&
		p.IsMatrixShaper() &&
		dst.IsMatrixShaper():
		if srcLayout == LayoutGray || srcLayout == LayoutGrayAlpha {
			return nil, cmsError(InvalidLayout)
		}
		if dstLayout == LayoutGray || dstLayout == LayoutGrayAlpha {
			return nil, cmsError(InvalidLayout)
		}

		if p.hasDeviceToPcsLut() || dst.hasPcsToDeviceLut() {
			return makeLutTransform(srcLayout, p, dstLayout, dst, options)
		}

		transform := p.TransformMatrix(dst)

		if p.areAllTrcTheSame() && dst.areAllTrcTheSame() {
			linear, err := p.buildRLinearizeTable(linearCap8, bitDepth8, options.AllowUseCicpTransfer)
			if err != nil {
				return nil, err
			}
			gamma, err := dst.buildGammaTable(dst.RedTrc, gammaCap8, bitDepth8, options.AllowUseCicpTransfer)
			if err != nil {
				return nil, err
			}
			profileTransform := transformMatrixShaperOptimized{
				linear:           linear,
				gamma:            gamma,
				adaptationMatrix: transform.ToF32(),
			}
			return makeOptimizedTransform(srcLayout, dstLayout, profileTransform, options)
		}

		shaper, err := p.buildTransformMatrixShaper(dst, transform, options)
		if err != nil {
			return nil, err
		}
		return makeTransform(srcLayout, dstLayout, shaper, options)

	case p.ColorSpace == DataColorSpaceGray && p.GrayTrc != nil &&
		(dst.ColorSpace == DataColorSpaceRgb || (dst.ColorSpace == DataColorSpaceGray && dst.GrayTrc != nil)) &&
		p.Pcs == DataColorSpaceXyz &&
		dst.Pcs == DataColorSpaceXyz:
		if srcLayout != LayoutGrayAlpha && srcLayout != LayoutGray {
			return nil, cmsError(InvalidLayout)
		}

		if p.hasDeviceToPcsLut() || dst.hasPcsToDeviceLut() {
			return makeLutTransform(srcLayout, p, dstLayout, dst, options)
		}

		grayLinear, err := p.buildGrayLinearizeTable(linearCap8, bitDepth8)
		if err != nil {
			return nil, err
		}

		if dst.ColorSpace == DataColorSpaceGray {
			// Gray to gray.
			grayGamma, err := dst.buildGammaTable(dst.GrayTrc, gammaCap8, bitDepth8, options.AllowUseCicpTransfer)
			if err != nil {
				return nil, err
			}
			return makeGrayToX(srcLayout, dstLayout, grayLinear, grayGamma, bitDepth8, gammaCap8)
		}
		if dst.areAllTrcTheSame() {
			// Gray to RGB, where each channel has the same curve.
			rgbGamma, err := dst.buildGammaTable(dst.RedTrc, gammaCap8, bitDepth8, options.AllowUseCicpTransfer)
			if err != nil {
				return nil, err
			}
			return makeGrayToX(srcLayout, dstLayout, grayLinear, rgbGamma, bitDepth8, gammaCap8)
		}
		// Gray to RGB, where the channels have different curves.
		return nil, cmsError(UnsupportedProfileConnection)

	case p.ColorSpace == DataColorSpaceRgb &&
		dst.ColorSpace == DataColorSpaceGray && dst.GrayTrc != nil &&
		dst.Pcs == DataColorSpaceXyz &&
		p.Pcs == DataColorSpaceXyz:
		// RGB to gray.
		return nil, cmsError(UnsupportedProfileConnection)

	case (p.ColorSpace.isThreeChannels() || p.ColorSpace == DataColorSpaceCmyk || p.ColorSpace == DataColorSpaceColor4) &&
		(dst.ColorSpace.isThreeChannels() || dst.ColorSpace == DataColorSpaceCmyk || dst.ColorSpace == DataColorSpaceColor4) &&
		(dst.Pcs == DataColorSpaceXyz || dst.Pcs == DataColorSpaceLab) &&
		(p.Pcs == DataColorSpaceXyz || p.Pcs == DataColorSpaceLab):
		if srcLayout == LayoutGray || srcLayout == LayoutGrayAlpha {
			return nil, cmsError(InvalidLayout)
		}
		if dstLayout == LayoutGray || dstLayout == LayoutGrayAlpha {
			return nil, cmsError(InvalidLayout)
		}
		return makeLutTransform(srcLayout, p, dstLayout, dst, options)

	default:
		return makeLutTransform(srcLayout, p, dstLayout, dst, options)
	}
}

// getDeviceToPcs is ColorProfile::get_device_to_pcs in moxcms
// src/transform.rs. It returns the A2B LUT of an intent. It falls back to
// A2B0 when the intent has no LUT, as ICC.1:2022 section 8.10.2 c) says.
func (p *ColorProfile) getDeviceToPcs(intent RenderingIntent) LutWarehouse {
	var lut LutWarehouse
	switch intent {
	case RenderingIntentAbsoluteColorimetric, RenderingIntentRelativeColorimetric:
		lut = p.LutAToBColorimetric
	case RenderingIntentSaturation:
		lut = p.LutAToBSaturation
	default:
		lut = p.LutAToBPerceptual
	}
	if lut == nil {
		return p.LutAToBPerceptual
	}
	return lut
}

// getPcsToDevice is ColorProfile::get_pcs_to_device in moxcms
// src/transform.rs. It returns the B2A LUT of an intent. It falls back to
// B2A0 when the intent has no LUT, as ICC.1:2022 section 8.10.2 c) says.
func (p *ColorProfile) getPcsToDevice(intent RenderingIntent) LutWarehouse {
	var lut LutWarehouse
	switch intent {
	case RenderingIntentAbsoluteColorimetric, RenderingIntentRelativeColorimetric:
		lut = p.LutBToAColorimetric
	case RenderingIntentSaturation:
		lut = p.LutBToASaturation
	default:
		lut = p.LutBToAPerceptual
	}
	if lut == nil {
		return p.LutBToAPerceptual
	}
	return lut
}

// deviceToPcsIsLut16 is ColorProfile::device_to_pcs_is_lut16 in moxcms
// src/transform.rs. Only a table of the type lut16Type encodes Lab as
// version 2 does, in profiles of version 2 and 4 alike (ICC.1 section
// 10.8).
func (p *ColorProfile) deviceToPcsIsLut16(intent RenderingIntent) bool {
	lut, ok := p.getDeviceToPcs(intent).(*LutDataType)
	return ok && lut.LutType == LutTypeLut16
}

// pcsToDeviceIsLut16 is ColorProfile::pcs_to_device_is_lut16 in moxcms
// src/transform.rs.
func (p *ColorProfile) pcsToDeviceIsLut16(intent RenderingIntent) bool {
	lut, ok := p.getPcsToDevice(intent).(*LutDataType)
	return ok && lut.LutType == LutTypeLut16
}
