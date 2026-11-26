// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/lut4.rs.

package moxcms

// lut4x3 is Lut4x3 in moxcms src/conversions/lut4.rs. It is a stage of a
// lut8Type or lut16Type tag with four inputs and three outputs.
type lut4x3 struct {
	linearization       [4][]float32
	clut                []float32
	gridSize            uint8
	output              [3][]float32
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
}

// Transform is Stage::transform for Lut4x3 in moxcms
// src/conversions/lut4.rs. It reads four values and writes three for each
// pixel. The subset has the quadlinear interpolation only, which upstream
// also uses for a PCS of Lab or XYZ.
func (l *lut4x3) Transform(src, dst []float32) error {
	tbl, err := NewHypercube(l.clut, int(l.gridSize), 3)
	if err != nil {
		return err
	}
	for i, j := 0, 0; i+4 <= len(src) && j+3 <= len(dst); i, j = i+4, j+3 {
		linearX := lutInterpLinearFloat(src[i], l.linearization[0])
		linearY := lutInterpLinearFloat(src[i+1], l.linearization[1])
		linearZ := lutInterpLinearFloat(src[i+2], l.linearization[2])
		linearW := lutInterpLinearFloat(src[i+3], l.linearization[3])

		clut := tbl.QuadlinearVec3(linearX, linearY, linearZ, linearW)

		dst[j] = lutInterpLinearFloat(clut.V[0], l.output[0])
		dst[j+1] = lutInterpLinearFloat(clut.V[1], l.output[1])
		dst[j+2] = lutInterpLinearFloat(clut.V[2], l.output[2])
	}
	return nil
}

// makeLut4x3 is make_lut_4x3 in moxcms src/conversions/lut4.rs.
func makeLut4x3(lut *LutDataType, options TransformOptions, pcs DataColorSpace) (*lut4x3, error) {
	clutLen, err := clutLength(lut)
	if err != nil {
		return nil, err
	}
	clutTable := lut.ClutTable.ToClutF32()
	if uint64(len(clutTable)) != clutLen {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(clutTable), Expected: int(min(clutLen, 1<<31))}}
	}
	linCurves, err := splitCurves(lut.InputTable.ToClutF32(), int(lut.NumInputTableEntries), 4)
	if err != nil {
		return nil, err
	}
	gammaCurves, err := splitCurves(lut.OutputTable.ToClutF32(), int(lut.NumOutputTableEntries), 3)
	if err != nil {
		return nil, err
	}
	return &lut4x3{
		linearization:       [4][]float32{linCurves[0], linCurves[1], linCurves[2], linCurves[3]},
		interpolationMethod: options.InterpolationMethod,
		pcs:                 pcs,
		clut:                clutTable,
		gridSize:            lut.NumClutGridPoints,
		output:              [3][]float32{gammaCurves[0], gammaCurves[1], gammaCurves[2]},
	}, nil
}

// stageLut4x3 is stage_lut_4x3 in moxcms src/conversions/lut4.rs.
func stageLut4x3(lut *LutDataType, options TransformOptions, pcs DataColorSpace) (Stage, error) {
	l, err := makeLut4x3(lut, options, pcs)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// createLut4NormSamples is create_lut4_norm_samples in moxcms
// src/conversions/lut4.rs. It returns the nodes of a grid of n nodes in
// each of four dimensions, each coordinate from 0 to 1. The fourth
// coordinate changes the slowest.
func createLut4NormSamples(n int) []float32 {
	src := make([]float32, 0, 4*n*n*n*n)
	recpeq := 1 / float32(n-1)
	for k := range n {
		for c := range n {
			for m := range n {
				for y := range n {
					src = append(src, float32(c)*recpeq, float32(m)*recpeq, float32(y)*recpeq, float32(k)*recpeq)
				}
			}
		}
	}
	return src
}

// createLut4 is create_lut4 in moxcms src/conversions/lut4.rs. It returns the
// result of the LUT on each node of a grid of n nodes in each dimension.
func createLut4(n int, lut *LutDataType, options TransformOptions, pcs DataColorSpace) ([]float32, error) {
	if lut.NumInputChannels != 4 {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	lutSize := 4 * n * n * n * n
	src := createLut4NormSamples(n)
	dest := make([]float32, lutSize/4*3)
	stage, err := stageLut4x3(lut, options, pcs)
	if err != nil {
		return nil, err
	}
	if err := stage.Transform(src, dest); err != nil {
		return nil, err
	}
	return dest, nil
}
