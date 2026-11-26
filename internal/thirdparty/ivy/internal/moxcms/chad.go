// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/chad.rs.

package moxcms

// pcsD50 is PCS_D50 in moxcms src/chad.rs. It is the illuminant of the
// profile connection space in ICC.1:2022 section 7.2.16.
var pcsD50 = Xyzd{X: 0.9642, Y: 1.0, Z: 0.8249}

// bradfordD is Matrix3d::bradford in moxcms src/chad.rs.
func bradfordD() Matrix3d {
	return Matrix3d{V: [3][3]float64{
		{0.8951, 0.2664, -0.1614},
		{-0.7502, 1.7135, 0.0367},
		{0.0389, -0.0685, 1.0296},
	}}
}

// computeChromaticAdaptionD is compute_chromatic_adaption_d in moxcms
// src/chad.rs. It returns the matrix that adapts a color from the white
// point src to the white point dst.
func computeChromaticAdaptionD(src, dst Xyz, chad Matrix3d) Matrix3d {
	coneSourceXyz := Vector3d{V: [3]float64{float64(src.X), float64(src.Y), float64(src.Z)}}
	coneSourceRgb := chad.MulVector(coneSourceXyz)

	coneDestXyz := Vector3d{V: [3]float64{float64(dst.X), float64(dst.Y), float64(dst.Z)}}
	coneDestRgb := chad.MulVector(coneDestXyz)

	cone := Matrix3d{V: [3][3]float64{
		{coneDestRgb.V[0] / coneSourceRgb.V[0], 0, 0},
		{0, coneDestRgb.V[1] / coneSourceRgb.V[1], 0},
		{0, 0, coneDestRgb.V[2] / coneSourceRgb.V[2]},
	}}

	chadInv := chad.Inverse()
	p0 := cone.MatMul(chad)
	return chadInv.MatMul(p0)
}

// adaptionMatrixD is adaption_matrix_d in moxcms src/chad.rs.
func adaptionMatrixD(sourceIllumination, targetIllumination Xyz) Matrix3d {
	return computeChromaticAdaptionD(sourceIllumination, targetIllumination, bradfordD())
}

// adaptToD50D is adapt_to_d50_d in moxcms src/chad.rs.
func adaptToD50D(r Matrix3d, sourceWhitePt XyY) Matrix3d {
	return adaptToIlluminantD(r, sourceWhitePt, pcsD50.ToXyz())
}

// adaptToIlluminantD is adapt_to_illuminant_d in moxcms src/chad.rs.
func adaptToIlluminantD(r Matrix3d, sourceWhitePt XyY, illuminantXyz Xyz) Matrix3d {
	bradford := adaptionMatrixD(sourceWhitePt.ToXyz(), illuminantXyz)
	return bradford.matMulConst(r)
}
