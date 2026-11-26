// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/transform_lut3_to_3.rs.

package moxcms

// transformLut3x3 is TransformLut3x3 in moxcms
// src/conversions/transform_lut3_to_3.rs, for samples of 8 bits and the
// weights of Low. It transforms through a 3D LUT of gridSize nodes in each
// dimension.
type transformLut3x3 struct {
	lut                 []float32
	srcLayout           Layout
	dstLayout           Layout
	gridSize            int
	bitDepth            int
	interpolationMethod InterpolationMethod
	weights             []barycentricWeight
	colorSpace          DataColorSpace
	isLinear            bool
}

// transformChunk is TransformLut3x3::transform_chunk in moxcms
// src/conversions/transform_lut3_to_3.rs.
func (t *transformLut3x3) transformChunk(src, dst []uint8, interpolator trilinear) {
	srcCn, dstCn := t.srcLayout, t.dstLayout
	srcChannels, dstChannels := srcCn.Channels(), dstCn.Channels()
	valueScale := float32(int(1)<<t.bitDepth - 1)
	maxValue := uint8(uint32(1)<<t.bitDepth - 1)
	// Port: the last color and its result are kept, because the pixels of
	// an image come in runs of one color, such as the white of a page.
	// Upstream interpolates each pixel. The result is the same.
	var lastX, lastY, lastZ, out0, out1, out2 uint8
	hasLast := false
	for i, j := 0, 0; i+srcChannels <= len(src) && j+dstChannels <= len(dst); i, j = i+srcChannels, j+dstChannels {
		s, d := src[i:i+srcChannels], dst[j:j+dstChannels]
		// The reduction of a sample of 8 bits to 256 bins is the sample.
		x := s[srcCn.rI()]
		y := s[srcCn.gI()]
		z := s[srcCn.bI()]
		a := maxValue
		if srcChannels == 4 {
			a = s[srcCn.aI()]
		}
		if !hasLast || x != lastX || y != lastY || z != lastZ {
			v := interpolator.inter3(t.lut, &t.weights[x], &t.weights[y], &t.weights[z])
			r0 := float32(v.V[0]*valueScale) + 0.5
			r1 := float32(v.V[1]*valueScale) + 0.5
			r2 := float32(v.V[2]*valueScale) + 0.5
			out0 = f32ToU8(maxF32(minF32(r0, valueScale), 0))
			out1 = f32ToU8(maxF32(minF32(r1, valueScale), 0))
			out2 = f32ToU8(maxF32(minF32(r2, valueScale), 0))
			lastX, lastY, lastZ, hasLast = x, y, z, true
		}
		d[dstCn.rI()] = out0
		d[dstCn.gI()] = out1
		d[dstCn.bI()] = out2
		if dstChannels == 4 {
			d[dstCn.aI()] = a
		}
	}
}

// Transform is TransformExecutor::transform for TransformLut3x3 in moxcms
// src/conversions/transform_lut3_to_3.rs. The subset has the trilinear
// interpolation only, which upstream also uses for Lab, XYZ and linear RGB.
func (t *transformLut3x3) Transform(src, dst []uint8) error {
	srcChannels, dstChannels := t.srcLayout.Channels(), t.dstLayout.Channels()
	if len(src)%srcChannels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if len(dst)%dstChannels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if len(src)/srcChannels != len(dst)/dstChannels {
		return cmsError(LaneSizeMismatch)
	}
	t.transformChunk(src, dst, trilinear{gridSize: t.gridSize})
	return nil
}

// makeTransform3x3 is Lut3x3Factory::make_transform_3x3 for
// DefaultLut3x3Factory in moxcms src/conversions/transform_lut3_to_3.rs,
// for samples of 8 bits. The subset has the weights of Low only.
func makeTransform3x3(srcLayout, dstLayout Layout, gridSize, bitDepth int, lut []float32, options TransformOptions, colorSpace DataColorSpace, isLinear bool) TransformExecutor {
	return &transformLut3x3{
		lut:                 lut,
		srcLayout:           srcLayout,
		dstLayout:           dstLayout,
		gridSize:            gridSize,
		bitDepth:            bitDepth,
		interpolationMethod: options.InterpolationMethod,
		weights:             createRanged256(gridSize),
		colorSpace:          colorSpace,
		isLinear:            isLinear,
	}
}
