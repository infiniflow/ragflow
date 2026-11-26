// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/interpolator.rs.

package moxcms

// Port: upstream has the interpolations Tetrahedral, Pyramidal and
// Prismatic with the feature "options" only, and their weights of more than
// 256 bins. hayro does not enable it, so the subset has Trilinear only.

// barycentricWeight is BarycentricWeight<f32> in moxcms
// src/conversions/interpolator.rs. For a sample, it holds the node below,
// the node above, and the weight of the node above.
type barycentricWeight struct {
	x, xn int32
	w     float32
}

// barycentricWeightI16 is BarycentricWeight<i16> in moxcms
// src/conversions/interpolator.rs. Its weight is in Q0.15.
type barycentricWeightI16 struct {
	x, xn int32
	w     int16
}

// createRanged256 is BarycentricWeight<f32>::create_ranged_256 in moxcms
// src/conversions/interpolator.rs. It returns the weights of the 256
// samples of 8 bits in a grid of gridSize nodes.
func createRanged256(gridSize int) []barycentricWeight {
	weights := make([]barycentricWeight, 256)
	const scale = float32(1.0) / lutSampling
	for index := range weights {
		x := int32(index) * int32(gridSize-1) / lutSampling
		xn := min(x+1, int32(gridSize-1))
		s := float32(int32(gridSize-1)) * scale
		dr := fmla(float32(index), s, -float32(x))
		weights[index] = barycentricWeight{x: x, xn: xn, w: dr}
	}
	return weights
}

// createRanged256I16 is BarycentricWeight<i16>::create_ranged_256 in moxcms
// src/conversions/interpolator.rs.
func createRanged256I16(gridSize int) []barycentricWeightI16 {
	weights := make([]barycentricWeightI16, 256)
	const scale = float32(1.0) / lutSampling
	const q = float32(1<<15 - 1)
	for index := range weights {
		x := int32(index) * int32(gridSize-1) / lutSampling
		xn := min(x+1, int32(gridSize-1))
		s := float32(int32(gridSize-1)) * scale
		dr := f32ToI16(maxF32(minF32(roundF32(fmla(float32(index), s, -float32(x))*q), 32767), -32767))
		weights[index] = barycentricWeightI16{x: x, xn: xn, w: dr}
	}
	return weights
}

// trilinear is Trilinear<GRID_SIZE> in moxcms
// src/conversions/interpolator.rs. It interpolates in a cube of gridSize
// nodes in each dimension, with three values in each node.
type trilinear struct {
	gridSize int
}

// fetch is TetrahedralFetchVector3f::fetch in moxcms
// src/conversions/interpolator.rs.
func (t trilinear) fetch(cube []float32, x, y, z int32) Vector3f {
	g := uint32(t.gridSize)
	offset := int(uint32(x)*(g*g)+uint32(y)*g+uint32(z)) * 3
	j := cube[offset : offset+3]
	return Vector3f{V: [3]float32{j[0], j[1], j[2]}}
}

// inter3 is MultidimensionalInterpolation::inter3 for Trilinear in moxcms
// src/conversions/interpolator.rs.
func (t trilinear) inter3(cube []float32, lutR, lutG, lutB *barycentricWeight) Vector3f {
	x, y, z := lutR.x, lutG.x, lutB.x
	xn, yn, zn := lutR.xn, lutG.xn, lutB.xn

	w0 := vector3fFrom(lutR.w)
	w1 := vector3fFrom(lutG.w)
	w2 := vector3fFrom(lutB.w)

	c000 := t.fetch(cube, x, y, z)
	c100 := t.fetch(cube, xn, y, z)
	c010 := t.fetch(cube, x, yn, z)
	c110 := t.fetch(cube, xn, yn, z)
	c001 := t.fetch(cube, x, y, zn)
	c101 := t.fetch(cube, xn, y, zn)
	c011 := t.fetch(cube, x, yn, zn)
	c111 := t.fetch(cube, xn, yn, zn)

	c00 := c000.negMla(c000, w0).mla(c100, w0)
	c10 := c010.negMla(c010, w0).mla(c110, w0)
	c01 := c001.negMla(c001, w0).mla(c101, w0)
	c11 := c011.negMla(c011, w0).mla(c111, w0)

	c0 := c00.negMla(c00, w1).mla(c10, w1)
	c1 := c01.negMla(c01, w1).mla(c11, w1)

	return c0.negMla(c0, w2).mla(c1, w2)
}
