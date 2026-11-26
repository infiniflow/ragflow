// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/bpc.rs.

package moxcms

// transformDirection is TransformDirection in moxcms
// src/conversions/bpc.rs.
type transformDirection uint8

// The directions of TransformDirection in moxcms src/conversions/bpc.rs.
const (
	deviceToPcs transformDirection = iota
	pcsToDevice
)

// defaultV4BlackPoint is DEFAULT_V4_BLACK_POINT in moxcms
// src/conversions/bpc.rs.
var defaultV4BlackPoint = Xyz{X: 0.003357, Y: 0.003479, Z: 0.002869}

// detectBlackPoint is ColorProfile::detect_black_point in moxcms
// src/conversions/bpc.rs. It reports false when the profile has no black
// point for the intent and the direction.
func (p *ColorProfile) detectBlackPoint(intent RenderingIntent, direction transformDirection) (Xyz, bool) {
	if intent != RenderingIntentPerceptual && intent != RenderingIntentSaturation {
		return Xyz{}, false
	}
	if p.ColorSpace != DataColorSpaceRgb {
		// Upstream detects no black point of a CMYK profile either.
		return Xyz{}, false
	}
	if p.versionInternal < ProfileVersionV40 {
		return Xyz{}, false
	}
	if direction == deviceToPcs {
		if p.hasDeviceToPcsLut() {
			return defaultV4BlackPoint, true
		}
		return Xyz{}, true
	}
	if p.hasPcsToDeviceLut() {
		return defaultV4BlackPoint, true
	}
	return Xyz{}, true
}

// compensateBpcInLut is compensate_bpc_in_lut in moxcms
// src/conversions/bpc.rs. It maps the black point srcBp of each XYZ triplet
// to dstBp, and keeps the white point.
func compensateBpcInLut(lutXyz []float32, srcBp, dstBp Xyz) {
	if srcBp.Equal(Xyz{}) && dstBp.Equal(Xyz{}) {
		return
	}
	wp := ChromaticityD50.ToXyz()

	// a = (wp - dstBp) / (wp - srcBp) is the scale.
	// b = dstBp - a*srcBp is the offset.
	// The result is a*in + b.
	compute := func(srcBpC, dstBpC, wpC float32) (float32, float32) {
		denom := wpC - srcBpC
		if absF32(denom) < 1e-10 {
			// The values are degenerate, so pass them through.
			return 1, 0
		}
		a := (wpC - dstBpC) / denom
		b := dstBpC - float32(a*srcBpC)
		return a, b
	}

	ax, bx := compute(srcBp.X, dstBp.X, wp.X)
	ay, by := compute(srcBp.Y, dstBp.Y, wp.Y)
	az, bz := compute(srcBp.Z, dstBp.Z, wp.Z)

	s := float32(1.0 / (1.0 + 32767.0/32768.0))
	bx *= s
	by *= s
	bz *= s

	for i := 0; i+3 <= len(lutXyz); i += 3 {
		lutXyz[i] = fmla(lutXyz[i], ax, bx)
		lutXyz[i+1] = fmla(lutXyz[i+1], ay, by)
		lutXyz[i+2] = fmla(lutXyz[i+2], az, bz)
	}
}
