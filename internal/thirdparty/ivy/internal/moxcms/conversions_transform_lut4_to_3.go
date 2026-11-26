// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/transform_lut4_to_3.rs.

package moxcms

// defaultVector3fLerp is DefaultVector3fLerp::interpolate in moxcms
// src/conversions/transform_lut4_to_3.rs. It interpolates from a to b at
// t, and scales the result to the range from 0 to scale, rounded.
func defaultVector3fLerp(a, b Vector3f, t, scale float32) Vector3f {
	tv := vector3fFrom(t)
	inter := a.negMla(a, tv).mla(b, tv)
	v := vector3fFrom(0.5).mla(inter, vector3fFrom(scale))
	v.V[0] = mClamp(v.V[0], 0, scale)
	v.V[1] = mClamp(v.V[1], 0, scale)
	v.V[2] = mClamp(v.V[2], 0, scale)
	return v
}

// transformLut4To3 is TransformLut4To3 in moxcms
// src/conversions/transform_lut4_to_3.rs, for samples of 8 bits and the
// weights of Low. It transforms CMYK through a 4D LUT of gridSize nodes in
// each dimension.
type transformLut4To3 struct {
	lut                 []float32
	layout              Layout
	gridSize            int
	bitDepth            int
	interpolationMethod InterpolationMethod
	weights             []barycentricWeight
	colorSpace          DataColorSpace
	isLinear            bool
}

// transformChunk is TransformLut4To3::transform_chunk in moxcms
// src/conversions/transform_lut4_to_3.rs, with DefaultVector3fLerp.
func (t *transformLut4To3) transformChunk(src, dst []uint8, interpolator trilinear) {
	cn := t.layout
	channels := cn.Channels()
	gridSize3 := t.gridSize * t.gridSize * t.gridSize
	valueScale := float32(int(1)<<t.bitDepth - 1)
	maxValue := uint8(uint32(1)<<t.bitDepth - 1)
	// Port: the last color and its result are kept, as in
	// transformLut3x3.transformChunk. The result is the same.
	var last [4]uint8
	var out [3]uint8
	hasLast := false
	for i, j := 0, 0; i+4 <= len(src) && j+channels <= len(dst); i, j = i+4, j+channels {
		s, d := src[i:i+4], dst[j:j+channels]
		c, m, y, k := s[0], s[1], s[2], s[3]
		if !hasLast || [4]uint8{c, m, y, k} != last {
			kWeights := t.weights[k]
			table1 := t.lut[int(kWeights.x)*gridSize3*3:]
			table2 := t.lut[int(kWeights.xn)*gridSize3*3:]
			r1 := interpolator.inter3(table1, &t.weights[c], &t.weights[m], &t.weights[y])
			r2 := interpolator.inter3(table2, &t.weights[c], &t.weights[m], &t.weights[y])
			r := defaultVector3fLerp(r1, r2, kWeights.w, valueScale)
			out = [3]uint8{f32ToU8(r.V[0]), f32ToU8(r.V[1]), f32ToU8(r.V[2])}
			last, hasLast = [4]uint8{c, m, y, k}, true
		}
		d[cn.rI()] = out[0]
		d[cn.gI()] = out[1]
		d[cn.bI()] = out[2]
		if channels == 4 {
			d[cn.aI()] = maxValue
		}
	}
}

// Transform is TransformExecutor::transform for TransformLut4To3 in moxcms
// src/conversions/transform_lut4_to_3.rs. The subset has the trilinear
// interpolation only, which upstream also uses for Lab, XYZ and linear RGB.
func (t *transformLut4To3) Transform(src, dst []uint8) error {
	channels := t.layout.Channels()
	if len(src)%4 != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if len(dst)%channels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if len(src)/4 != len(dst)/channels {
		return cmsError(LaneSizeMismatch)
	}
	t.transformChunk(src, dst, trilinear{gridSize: t.gridSize})
	return nil
}

// alignedI16x4 is AlignedI16x4 in moxcms
// src/conversions/transform_lut4_to_3.rs. It holds a node of a LUT in Q0.15,
// with a fourth value for the alignment.
type alignedI16x4 [4]int16

// transformLut4To3Q015 is TransformLut4To3Q0_15 in moxcms
// src/conversions/transform_lut4_to_3.rs, for samples of 8 bits and the
// weights of Low. It transforms CMYK through a 4D LUT in fixed point.
type transformLut4To3Q015 struct {
	lut      []alignedI16x4
	layout   Layout
	gridSize int
	bitDepth int
	weights  []barycentricWeightI16
}

// q015Mul is q0_15_mul in moxcms src/conversions/transform_lut4_to_3.rs. It
// multiplies two values in Q0.15, rounded as the instruction pmulhrsw
// rounds.
func q015Mul(a, b int16) int16 {
	product := int32(a) * int32(b)
	return int16(((product >> 14) + 1) >> 1)
}

// q015Lerp is q0_15_lerp in moxcms src/conversions/transform_lut4_to_3.rs.
//
// Port: the sum of upstream wraps on overflow in a release build. Go wraps
// an int16 in the same way.
func q015Lerp(a, b, t int16) int16 {
	const q = int16(1<<15 - 1)
	return q015Mul(a, q-t) + q015Mul(b, t)
}

// q015Lerp3 is q0_15_lerp3 in moxcms
// src/conversions/transform_lut4_to_3.rs.
func q015Lerp3(a, b [3]int16, t int16) [3]int16 {
	return [3]int16{q015Lerp(a[0], b[0], t), q015Lerp(a[1], b[1], t), q015Lerp(a[2], b[2], t)}
}

// fetch is TransformLut4To3Q0_15::fetch in moxcms
// src/conversions/transform_lut4_to_3.rs.
func (t *transformLut4To3Q015) fetch(table []alignedI16x4, x, y, z int32) [3]int16 {
	g := uint32(t.gridSize)
	offset := int(uint32(x)*(g*g) + uint32(y)*g + uint32(z))
	n := table[offset]
	return [3]int16{n[0], n[1], n[2]}
}

// inter3 is TransformLut4To3Q0_15::inter3 in moxcms
// src/conversions/transform_lut4_to_3.rs.
func (t *transformLut4To3Q015) inter3(table []alignedI16x4, c, m, y uint8) [3]int16 {
	cw := t.weights[c]
	mw := t.weights[m]
	yw := t.weights[y]

	c000 := t.fetch(table, cw.x, mw.x, yw.x)
	c100 := t.fetch(table, cw.xn, mw.x, yw.x)
	c010 := t.fetch(table, cw.x, mw.xn, yw.x)
	c110 := t.fetch(table, cw.xn, mw.xn, yw.x)
	c001 := t.fetch(table, cw.x, mw.x, yw.xn)
	c101 := t.fetch(table, cw.xn, mw.x, yw.xn)
	c011 := t.fetch(table, cw.x, mw.xn, yw.xn)
	c111 := t.fetch(table, cw.xn, mw.xn, yw.xn)

	c00 := q015Lerp3(c000, c100, cw.w)
	c10 := q015Lerp3(c010, c110, cw.w)
	c01 := q015Lerp3(c001, c101, cw.w)
	c11 := q015Lerp3(c011, c111, cw.w)
	c0 := q015Lerp3(c00, c10, mw.w)
	c1 := q015Lerp3(c01, c11, mw.w)
	return q015Lerp3(c0, c1, yw.w)
}

// transformChunk is TransformLut4To3Q0_15::transform_chunk in moxcms
// src/conversions/transform_lut4_to_3.rs.
func (t *transformLut4To3Q015) transformChunk(src, dst []uint8) {
	cn := t.layout
	channels := cn.Channels()
	gridSize3 := t.gridSize * t.gridSize * t.gridSize
	maxValue := uint8(uint32(1)<<t.bitDepth - 1)
	maxI16 := int16(int32(1)<<t.bitDepth - 1)
	// Port: the last color and its result are kept, as in
	// transformLut3x3.transformChunk. The result is the same.
	var last [4]uint8
	var out [3]uint8
	hasLast := false
	for i, j := 0, 0; i+4 <= len(src) && j+channels <= len(dst); i, j = i+4, j+channels {
		s, d := src[i:i+4], dst[j:j+channels]
		c, m, y, k := s[0], s[1], s[2], s[3]
		if !hasLast || [4]uint8{c, m, y, k} != last {
			kWeights := t.weights[k]
			table1 := t.lut[int(kWeights.x)*gridSize3:]
			table2 := t.lut[int(kWeights.xn)*gridSize3:]
			r1 := t.inter3(table1, c, m, y)
			r2 := t.inter3(table2, c, m, y)
			r := q015Lerp3(r1, r2, kWeights.w)
			out = [3]uint8{
				uint8(uint32(max(min(r[0], maxI16), 0))),
				uint8(uint32(max(min(r[1], maxI16), 0))),
				uint8(uint32(max(min(r[2], maxI16), 0))),
			}
			last, hasLast = [4]uint8{c, m, y, k}, true
		}
		d[cn.rI()] = out[0]
		d[cn.gI()] = out[1]
		d[cn.bI()] = out[2]
		if channels == 4 {
			d[cn.aI()] = maxValue
		}
	}
}

// Transform is TransformExecutor::transform for TransformLut4To3Q0_15 in
// moxcms src/conversions/transform_lut4_to_3.rs.
func (t *transformLut4To3Q015) Transform(src, dst []uint8) error {
	channels := t.layout.Channels()
	if len(src)%4 != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if len(dst)%channels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if len(src)/4 != len(dst)/channels {
		return cmsError(LaneSizeMismatch)
	}
	t.transformChunk(src, dst)
	return nil
}

// makeTransform4x3 is Lut4x3Factory::make_transform_4x3 for
// DefaultLut4x3Factory in moxcms src/conversions/transform_lut4_to_3.rs, for
// samples of 8 bits. The subset has the weights of Low only.
func makeTransform4x3(layout Layout, gridSize, bitDepth int, lut grid4x3, options TransformOptions, colorSpace DataColorSpace, isLinear bool) (TransformExecutor, error) {
	useFixedPoint := options.PreferFixedPoint &&
		bitDepth < 16 &&
		(colorSpace == DataColorSpaceLab ||
			(isLinear && colorSpace == DataColorSpaceRgb) ||
			colorSpace == DataColorSpaceXyz ||
			options.InterpolationMethod == InterpolationMethodLinear)

	if useFixedPoint {
		q := float32(int32(1)<<bitDepth - 1)
		nodes, err := lut.intoF32()
		if err != nil {
			return nil, err
		}
		fixed := make([]alignedI16x4, len(nodes)/3)
		for i := range fixed {
			x := nodes[3*i : 3*i+3]
			fixed[i] = alignedI16x4{
				f32ToI16(roundF32(x[0] * q)),
				f32ToI16(roundF32(x[1] * q)),
				f32ToI16(roundF32(x[2] * q)),
				0,
			}
		}
		return &transformLut4To3Q015{
			lut:      fixed,
			layout:   layout,
			gridSize: gridSize,
			bitDepth: bitDepth,
			weights:  createRanged256I16(gridSize),
		}, nil
	}

	nodes, err := lut.intoF32()
	if err != nil {
		return nil, err
	}
	return &transformLut4To3{
		lut:                 nodes,
		layout:              layout,
		gridSize:            gridSize,
		bitDepth:            bitDepth,
		interpolationMethod: options.InterpolationMethod,
		weights:             createRanged256(gridSize),
		colorSpace:          colorSpace,
		isLinear:            isLinear,
	}, nil
}
