// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/rgb_xyz_factory.rs.

package moxcms

// fixedPointScale is FIXED_POINT_SCALE in moxcms
// src/conversions/rgb_xyz_factory.rs. It is the precision of Q2.13.
const fixedPointScale = 13

// Port: upstream declares the traits RgbXyzFactory and RgbXyzFactoryOpt,
// and implements them for each type of sample. The port has the
// implementations for u8 only, and each one takes the scalar path, where
// upstream can take a path of SIMD.

// makeTransform is RgbXyzFactory::make_transform for u8 in moxcms
// src/conversions/rgb_xyz_factory.rs.
func makeTransform(srcLayout, dstLayout Layout, profile transformMatrixShaper, _ TransformOptions) (TransformExecutor, error) {
	return makeRgbXyzRgbTransform(srcLayout, dstLayout, profile, gammaCap8, bitDepth8)
}

// makeInPlaceTransform is RgbXyzFactory::make_in_place_transform for u8 in
// moxcms src/conversions/rgb_xyz_factory.rs.
func makeInPlaceTransform(layout Layout, profile transformMatrixShaper, _ TransformOptions) (InPlaceTransformExecutor, error) {
	return makeInPlaceRgbXyzTransform(layout, profile, gammaCap8, bitDepth8)
}

// makeOptimizedTransform is RgbXyzFactoryOpt::make_optimized_transform for
// u8 in moxcms src/conversions/rgb_xyz_factory.rs.
func makeOptimizedTransform(srcLayout, dstLayout Layout, profile transformMatrixShaperOptimized, options TransformOptions) (TransformExecutor, error) {
	if options.PreferFixedPoint {
		return makeRgbXyzQ213Opt(srcLayout, dstLayout, profile, fixedPointScale, gammaCap8, bitDepth8)
	}
	return makeRgbXyzRgbTransformOpt(srcLayout, dstLayout, profile, gammaCap8, bitDepth8)
}

// makeInPlaceOptimizedTransform is
// RgbXyzFactoryOpt::make_in_place_optimized_transform for u8 in moxcms
// src/conversions/rgb_xyz_factory.rs. Upstream has a path of fixed point
// for SIMD only, so the scalar path computes in f32 even when the options
// prefer fixed point.
func makeInPlaceOptimizedTransform(layout Layout, profile transformMatrixShaperOptimized, _ TransformOptions) (InPlaceTransformExecutor, error) {
	return makeRgbXyzInPlaceTransformOpt(layout, profile, gammaCap8, bitDepth8)
}
