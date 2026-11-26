// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/defaults.rs.

package moxcms

// whitePointFromTemperature is white_point_from_temperature in moxcms
// src/defaults.rs, which is cmsWhitePointFromTemp of lcms. The temperature
// tempK must be from 4000 to 25000 kelvin. For another temperature it
// returns (-1, -1, -1).
func whitePointFromTemperature(tempK int32) XyY {
	t := float64(tempK)
	t2 := t * t
	t3 := t2 * t
	var x float64
	switch {
	case t > 4000 && t <= 7000:
		x = float64(-4.6070*(1e9/t3)) + float64(2.9678*(1e6/t2)) + float64(0.09911*(1e3/t)) + 0.244063
	case t > 7000 && t <= 25000:
		x = float64(-2.0064*(1e9/t3)) + float64(1.9018*(1e6/t2)) + float64(0.24748*(1e3/t)) + 0.237040
	default:
		return XyY{X: -1, Y: -1, Yb: -1}
	}
	y := float64(-3.000*(x*x)) + float64(2.870*x) - 0.275
	return XyY{X: x, Y: y, Yb: 1}
}

// whitePointD65 is white_point_d65 in moxcms src/defaults.rs.
func whitePointD65() XyY {
	return whitePointFromTemperature(6504)
}

// srgbColorants is srgb_colorants in moxcms src/defaults.rs.
func srgbColorants() Matrix3d {
	return colorantsMatrix(whitePointD65(), ColorPrimariesBt709)
}

// basicRgbProfile is ColorProfile::basic_rgb_profile in moxcms
// src/defaults.rs.
func basicRgbProfile(whitePoint XyY) *ColorProfile {
	p := defaultColorProfile()
	p.ProfileClass = ProfileClassDisplayDevice
	p.RenderingIntent = RenderingIntentPerceptual
	p.ColorSpace = DataColorSpaceRgb
	p.Pcs = DataColorSpaceXyz
	p.updateChromaticAdaptation(whitePoint)
	return &p
}

// NewFromCicp is ColorProfile::new_from_cicp in moxcms src/defaults.rs. It
// returns a new RGB profile with the primaries and the transfer
// characteristics of cicp. When cicp has no primaries or no transfer curve,
// the profile has no colorants or no tone curves, and a transform from it
// fails.
func NewFromCicp(cicp CicpProfile) *ColorProfile {
	p := basicRgbProfile(whitePointD65())
	p.updateRgbColorimetryFromCicp(cicp)
	return p
}

// NewSrgb is ColorProfile::new_srgb in moxcms src/defaults.rs. It returns a
// new sRGB profile.
//
// Port: the subset leaves out the description and the copyright of the
// profile.
func NewSrgb() *ColorProfile {
	p := basicRgbProfile(whitePointD65())
	p.updateColorants(srgbColorants())
	p.RedTrc = &ToneReprCurve{Kind: CurveParametric, Parametric: srgbParametric()}
	p.GreenTrc = &ToneReprCurve{Kind: CurveParametric, Parametric: srgbParametric()}
	p.BlueTrc = &ToneReprCurve{Kind: CurveParametric, Parametric: srgbParametric()}
	p.Cicp = &CicpProfile{
		ColorPrimaries:          CicpColorPrimariesBt709,
		TransferCharacteristics: TransferCharacteristicsSrgb,
		MatrixCoefficients:      MatrixCoefficientsBt709,
		FullRange:               false,
	}
	return p
}

// NewGrayWithGamma is ColorProfile::new_gray_with_gamma in moxcms
// src/defaults.rs. It returns a new gray profile with a pure gamma.
//
// Port: the subset leaves out the copyright of the profile.
func NewGrayWithGamma(gamma float32) *ColorProfile {
	p := defaultColorProfile()
	p.GrayTrc = curveFromGamma(gamma)
	p.ProfileClass = ProfileClassDisplayDevice
	p.RenderingIntent = RenderingIntentPerceptual
	p.ColorSpace = DataColorSpaceGray
	p.updateChromaticAdaptation(whitePointD65())
	return &p
}
