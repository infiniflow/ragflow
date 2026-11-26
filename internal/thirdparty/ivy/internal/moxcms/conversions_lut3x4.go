// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/lut3x4.rs.

package moxcms

// Port: the subset has no transform to CMYK, so it leaves out the LUTs with
// three inputs and four outputs of this file. It keeps the two functions
// that make the samples of a grid.

// createLut3Samples is create_lut3_samples in moxcms
// src/conversions/lut3x4.rs, for samples of 8 bits. It returns the nodes of
// a grid of n nodes in each dimension, as integers.
func createLut3Samples(n int) []uint8 {
	src := make([]uint8, 0, 3*n*n*n)
	for x := range n {
		for y := range n {
			for z := range n {
				src = append(src, uint8(x), uint8(y), uint8(z))
			}
		}
	}
	return src
}

// createLut3SamplesNorm is create_lut3_samples_norm in moxcms
// src/conversions/lut3x4.rs. It returns the nodes of a grid of n nodes in
// each dimension, each coordinate from 0 to 1.
func createLut3SamplesNorm(n int) []float32 {
	scale := 1 / (float32(n) - 1)
	src := make([]float32, 0, 3*n*n*n)
	for x := range n {
		for y := range n {
			for z := range n {
				src = append(src, float32(x)*scale, float32(y)*scale, float32(z)*scale)
			}
		}
	}
	return src
}
