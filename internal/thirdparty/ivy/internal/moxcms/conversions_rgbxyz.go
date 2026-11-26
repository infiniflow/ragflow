// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/rgbxyz.rs.

package moxcms

// transformMatrixShaper is TransformMatrixShaper in moxcms
// src/conversions/rgbxyz.rs. It holds the tables and the matrix of a
// transform between two matrix shapers with a curve for each channel.
type transformMatrixShaper struct {
	rLinear          []float32
	gLinear          []float32
	bLinear          []float32
	rGamma           []uint8
	gGamma           []uint8
	bGamma           []uint8
	adaptationMatrix Matrix3f
}

// transformMatrixShaperOptimized is TransformMatrixShaperOptimized in moxcms
// src/conversions/rgbxyz.rs. It holds the tables and the matrix of a
// transform between two matrix shapers whose channels have the same curve.
type transformMatrixShaperOptimized struct {
	linear           []float32
	gamma            []uint8
	adaptationMatrix Matrix3f
}

// toQ213N is TransformMatrixShaperOptimized::to_q2_13_n in moxcms
// src/conversions/rgbxyz.rs, for R = i16 and samples of 8 bits. It converts
// the linear table and the matrix to fixed point with precision bits.
func (s *transformMatrixShaperOptimized) toQ213N(precision uint, gammaLut, bitDepth int) transformMatrixShaperFixedPointOpt {
	lutScale := float32(gammaLut-1) / float32(int(1)<<bitDepth-1)
	linearScale := float32(int(1)<<bitDepth-1) * lutScale
	linear := make([]int16, len(s.linear))
	for i, v := range s.linear {
		linear[i] = f32ToI16(roundF32(v * linearScale))
	}
	scale := float32(int32(1) << precision)
	var m [3][3]int16
	for i := range 3 {
		for j := range 3 {
			m[i][j] = f32ToI16(s.adaptationMatrix.V[i][j] * scale)
		}
	}
	return transformMatrixShaperFixedPointOpt{
		linear:           linear,
		gamma:            s.gamma,
		adaptationMatrix: m,
	}
}

// checkLanes makes the checks of the length of the buffers that each
// transform of upstream makes before it starts.
func checkLanes(srcLen, srcChannels, dstLen, dstChannels int) error {
	if srcLen/srcChannels != dstLen/dstChannels {
		return cmsError(LaneSizeMismatch)
	}
	if srcLen%srcChannels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	if dstLen%dstChannels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	return nil
}

// isRgbLayout reports whether l is LayoutRgb or LayoutRgba, the layouts of
// the transforms of a matrix shaper.
func isRgbLayout(l Layout) bool {
	return l == LayoutRgb || l == LayoutRgba
}

// shaperChannel is the arithmetic of one output channel of
// TransformMatrixShaperScalar::transform in moxcms
// src/conversions/rgbxyz.rs. It returns the index into a gamma table.
func shaperChannel(r, g, b float32, row [3]float32, scale float32) uint16 {
	v := mlaf(mlaf(float32(r*row[0]), g, row[1]), b, row[2])
	return f32ToU16(mlaf(0.5, minF32(maxF32(v, 0), 1), scale))
}

// transformMatrixShaperScalar is TransformMatrixShaperScalar in moxcms
// src/conversions/rgbxyz.rs.
//
// Port: upstream makes the layouts const parameters. The port keeps them in
// fields.
type transformMatrixShaperScalar struct {
	profile   transformMatrixShaper
	srcLayout Layout
	dstLayout Layout
	gammaLut  int
	bitDepth  int
}

// Transform is TransformExecutor::transform for TransformMatrixShaperScalar
// in moxcms src/conversions/rgbxyz.rs.
func (t *transformMatrixShaperScalar) Transform(src, dst []uint8) error {
	srcCn, dstCn := t.srcLayout, t.dstLayout
	srcChannels, dstChannels := srcCn.Channels(), dstCn.Channels()
	if err := checkLanes(len(src), srcChannels, len(dst), dstChannels); err != nil {
		return err
	}
	m := t.profile.adaptationMatrix
	scale := float32(t.gammaLut - 1)
	maxColors := uint8(1<<t.bitDepth - 1)
	for i, j := 0, 0; i+srcChannels <= len(src) && j+dstChannels <= len(dst); i, j = i+srcChannels, j+dstChannels {
		s, d := src[i:i+srcChannels], dst[j:j+dstChannels]
		r := t.profile.rLinear[s[srcCn.rI()]]
		g := t.profile.gLinear[s[srcCn.gI()]]
		b := t.profile.bLinear[s[srcCn.bI()]]
		a := maxColors
		if srcChannels == 4 {
			a = s[srcCn.aI()]
		}
		newR := shaperChannel(r, g, b, m.V[0], scale)
		newG := shaperChannel(r, g, b, m.V[1], scale)
		newB := shaperChannel(r, g, b, m.V[2], scale)
		d[dstCn.rI()] = t.profile.rGamma[newR]
		d[dstCn.gI()] = t.profile.gGamma[newG]
		d[dstCn.bI()] = t.profile.bGamma[newB]
		if dstChannels == 4 {
			d[dstCn.aI()] = a
		}
	}
	return nil
}

// transformMatrixShaperOptScalar is TransformMatrixShaperOptScalar in moxcms
// src/conversions/rgbxyz.rs.
type transformMatrixShaperOptScalar struct {
	profile   transformMatrixShaperOptimized
	srcLayout Layout
	dstLayout Layout
	gammaLut  int
	bitDepth  int
}

// Transform is TransformExecutor::transform for
// TransformMatrixShaperOptScalar in moxcms src/conversions/rgbxyz.rs.
func (t *transformMatrixShaperOptScalar) Transform(src, dst []uint8) error {
	srcCn, dstCn := t.srcLayout, t.dstLayout
	srcChannels, dstChannels := srcCn.Channels(), dstCn.Channels()
	if err := checkLanes(len(src), srcChannels, len(dst), dstChannels); err != nil {
		return err
	}
	m := t.profile.adaptationMatrix
	scale := float32(t.gammaLut - 1)
	maxColors := uint8(1<<t.bitDepth - 1)
	for i, j := 0, 0; i+srcChannels <= len(src) && j+dstChannels <= len(dst); i, j = i+srcChannels, j+dstChannels {
		s, d := src[i:i+srcChannels], dst[j:j+dstChannels]
		r := t.profile.linear[s[srcCn.rI()]]
		g := t.profile.linear[s[srcCn.gI()]]
		b := t.profile.linear[s[srcCn.bI()]]
		a := maxColors
		if srcChannels == 4 {
			a = s[srcCn.aI()]
		}
		newR := shaperChannel(r, g, b, m.V[0], scale)
		newG := shaperChannel(r, g, b, m.V[1], scale)
		newB := shaperChannel(r, g, b, m.V[2], scale)
		d[dstCn.rI()] = t.profile.gamma[newR]
		d[dstCn.gI()] = t.profile.gamma[newG]
		d[dstCn.bI()] = t.profile.gamma[newB]
		if dstChannels == 4 {
			d[dstCn.aI()] = a
		}
	}
	return nil
}

// inPlace is the InPlaceTransformExecutor of a TransformExecutor whose
// source and destination have the same layout.
//
// Port: upstream implements InPlaceTransformExecutor for
// TransformMatrixShaperScalar, TransformMatrixShaperOptScalar and
// TransformGray2RgbFusedExecutor with a copy of the loop of
// TransformExecutor. Each such loop reads all the channels of a pixel
// before it writes the pixel. So the transform from a buffer into the same
// buffer gives the same result, and the port uses it.
type inPlace struct {
	t        TransformExecutor
	channels int
}

// Transform is InPlaceTransformExecutor::transform.
func (p inPlace) Transform(inOut []uint8) error {
	if len(inOut)%p.channels != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	return p.t.Transform(inOut, inOut)
}

// makeRgbXyzRgbTransform is make_rgb_xyz_rgb_transform in moxcms
// src/conversions/rgbxyz.rs, for the scalar path.
func makeRgbXyzRgbTransform(srcLayout, dstLayout Layout, profile transformMatrixShaper, gammaLut, bitDepth int) (TransformExecutor, error) {
	if !isRgbLayout(srcLayout) || !isRgbLayout(dstLayout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	return &transformMatrixShaperScalar{
		profile:   profile,
		srcLayout: srcLayout,
		dstLayout: dstLayout,
		gammaLut:  gammaLut,
		bitDepth:  bitDepth,
	}, nil
}

// makeRgbXyzRgbTransformOpt is make_rgb_xyz_rgb_transform_opt in moxcms
// src/conversions/rgbxyz.rs, for the scalar path.
func makeRgbXyzRgbTransformOpt(srcLayout, dstLayout Layout, profile transformMatrixShaperOptimized, gammaLut, bitDepth int) (TransformExecutor, error) {
	if !isRgbLayout(srcLayout) || !isRgbLayout(dstLayout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	return &transformMatrixShaperOptScalar{
		profile:   profile,
		srcLayout: srcLayout,
		dstLayout: dstLayout,
		gammaLut:  gammaLut,
		bitDepth:  bitDepth,
	}, nil
}

// makeInPlaceRgbXyzTransform is make_in_place_rgb_xyz_transform in moxcms
// src/conversions/rgbxyz.rs.
func makeInPlaceRgbXyzTransform(layout Layout, profile transformMatrixShaper, gammaLut, bitDepth int) (InPlaceTransformExecutor, error) {
	if !isRgbLayout(layout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	t := &transformMatrixShaperScalar{
		profile:   profile,
		srcLayout: layout,
		dstLayout: layout,
		gammaLut:  gammaLut,
		bitDepth:  bitDepth,
	}
	return inPlace{t: t, channels: layout.Channels()}, nil
}

// makeRgbXyzInPlaceTransformOpt is make_rgb_xyz_in_place_transform_opt in
// moxcms src/conversions/rgbxyz.rs.
func makeRgbXyzInPlaceTransformOpt(layout Layout, profile transformMatrixShaperOptimized, gammaLut, bitDepth int) (InPlaceTransformExecutor, error) {
	if !isRgbLayout(layout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	t := &transformMatrixShaperOptScalar{
		profile:   profile,
		srcLayout: layout,
		dstLayout: layout,
		gammaLut:  gammaLut,
		bitDepth:  bitDepth,
	}
	return inPlace{t: t, channels: layout.Channels()}, nil
}
