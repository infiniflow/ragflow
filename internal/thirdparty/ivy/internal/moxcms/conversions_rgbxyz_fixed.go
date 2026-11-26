// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/rgbxyz_fixed.rs.

package moxcms

// transformMatrixShaperFixedPointOpt is TransformMatrixShaperFixedPointOpt
// in moxcms src/conversions/rgbxyz_fixed.rs, for R = i16 and W = i16. It
// holds a transform between two matrix shapers whose channels have the same
// curve, in fixed point Q2.13.
//
// Port: upstream declares Matrix3<W>. The port holds the matrix in an
// array.
type transformMatrixShaperFixedPointOpt struct {
	linear           []int16
	gamma            []uint8
	adaptationMatrix [3][3]int16
}

// transformMatrixShaperQ213Optimized is TransformMatrixShaperQ2_13Optimized
// in moxcms src/conversions/rgbxyz_fixed.rs.
type transformMatrixShaperQ213Optimized struct {
	profile   transformMatrixShaperFixedPointOpt
	srcLayout Layout
	dstLayout Layout
	bitDepth  int
	gammaLut  int
	precision uint
}

// q213Channel is the arithmetic of one output channel of
// TransformMatrixShaperQ2_13Optimized::transform in moxcms
// src/conversions/rgbxyz_fixed.rs. It returns the index into the gamma
// table.
//
// Port: upstream adds the products in i32, and the sum wraps on overflow in
// a release build. Go wraps an int32 in the same way.
func q213Channel(r, g, b int16, row [3]int16, rnd int32, precision uint, gammaMax int32) uint16 {
	v := int32(r)*int32(row[0]) + int32(g)*int32(row[1]) + int32(b)*int32(row[2]) + rnd
	return uint16(max(min(v>>precision, gammaMax), 0))
}

// Transform is TransformExecutor::transform for
// TransformMatrixShaperQ2_13Optimized in moxcms
// src/conversions/rgbxyz_fixed.rs.
func (t *transformMatrixShaperQ213Optimized) Transform(src, dst []uint8) error {
	srcCn, dstCn := t.srcLayout, t.dstLayout
	srcChannels, dstChannels := srcCn.Channels(), dstCn.Channels()
	if err := checkLanes(len(src), srcChannels, len(dst), dstChannels); err != nil {
		return err
	}
	m := t.profile.adaptationMatrix
	maxColors := uint8(1<<t.bitDepth - 1)
	rnd := int32(1) << (t.precision - 1)
	gammaMax := int32(t.gammaLut) - 1
	for i, j := 0, 0; i+srcChannels <= len(src) && j+dstChannels <= len(dst); i, j = i+srcChannels, j+dstChannels {
		s, d := src[i:i+srcChannels], dst[j:j+dstChannels]
		r := t.profile.linear[s[srcCn.rI()]]
		g := t.profile.linear[s[srcCn.gI()]]
		b := t.profile.linear[s[srcCn.bI()]]
		a := maxColors
		if srcChannels == 4 {
			a = s[srcCn.aI()]
		}
		d[dstCn.rI()] = t.profile.gamma[q213Channel(r, g, b, m[0], rnd, t.precision, gammaMax)]
		d[dstCn.gI()] = t.profile.gamma[q213Channel(r, g, b, m[1], rnd, t.precision, gammaMax)]
		d[dstCn.bI()] = t.profile.gamma[q213Channel(r, g, b, m[2], rnd, t.precision, gammaMax)]
		if dstChannels == 4 {
			d[dstCn.aI()] = a
		}
	}
	return nil
}

// makeRgbXyzQ213Opt is make_rgb_xyz_q2_13_opt in moxcms
// src/conversions/rgbxyz_fixed.rs, for the scalar path.
func makeRgbXyzQ213Opt(srcLayout, dstLayout Layout, profile transformMatrixShaperOptimized, precision uint, gammaLut, bitDepth int) (TransformExecutor, error) {
	q213 := profile.toQ213N(precision, gammaLut, bitDepth)
	if !isRgbLayout(srcLayout) || !isRgbLayout(dstLayout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	return &transformMatrixShaperQ213Optimized{
		profile:   q213,
		srcLayout: srcLayout,
		dstLayout: dstLayout,
		bitDepth:  bitDepth,
		gammaLut:  gammaLut,
		precision: precision,
	}, nil
}
