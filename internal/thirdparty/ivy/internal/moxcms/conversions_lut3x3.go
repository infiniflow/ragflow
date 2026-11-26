// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/lut3x3.rs.

package moxcms

// lut3x3 is Lut3x3 in moxcms src/conversions/lut3x3.rs. It is a stage of a
// lut8Type or lut16Type tag with three inputs and three outputs.
type lut3x3 struct {
	input               [3][]float32
	clut                []float32
	gridSize            uint8
	gamma               [3][]float32
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
}

// splitCurves splits table into n curves of entries values each. It
// returns MalformedCurveLutTable when table is too short.
func splitCurves(table []float32, entries, n int) ([][]float32, error) {
	if len(table) < entries*n {
		return nil, &CmsError{Kind: MalformedCurveLutTable, Size: MalformedSize{Size: len(table), Expected: entries * n}}
	}
	curves := make([][]float32, n)
	for i := range curves {
		curves[i] = table[i*entries : (i+1)*entries]
	}
	return curves, nil
}

// clutLength returns the number of values of the CLUT of a lut8Type or
// lut16Type tag, as make_lut_3x3 and make_lut_4x3 in moxcms compute it.
func clutLength(lut *LutDataType) (uint64, error) {
	n, err := safePowi(uint64(lut.NumClutGridPoints), uint32(lut.NumInputChannels))
	if err != nil {
		return 0, err
	}
	return safeMul(n, uint64(lut.NumOutputChannels))
}

// makeLut3x3 is make_lut_3x3 in moxcms src/conversions/lut3x3.rs.
func makeLut3x3(lut *LutDataType, options TransformOptions, pcs DataColorSpace) (*lut3x3, error) {
	clutLen, err := clutLength(lut)
	if err != nil {
		return nil, err
	}
	linCurves, err := splitCurves(lut.InputTable.ToClutF32(), int(lut.NumInputTableEntries), 3)
	if err != nil {
		return nil, err
	}
	clutTable := lut.ClutTable.ToClutF32()
	if uint64(len(clutTable)) != clutLen {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(clutTable), Expected: int(min(clutLen, 1<<31))}}
	}
	gammaCurves, err := splitCurves(lut.OutputTable.ToClutF32(), int(lut.NumOutputTableEntries), 3)
	if err != nil {
		return nil, err
	}
	return &lut3x3{
		input:               [3][]float32{linCurves[0], linCurves[1], linCurves[2]},
		gamma:               [3][]float32{gammaCurves[0], gammaCurves[1], gammaCurves[2]},
		interpolationMethod: options.InterpolationMethod,
		clut:                clutTable,
		gridSize:            lut.NumClutGridPoints,
		pcs:                 pcs,
	}, nil
}

// stageLut3x3 is stage_lut_3x3 in moxcms src/conversions/lut3x3.rs.
func stageLut3x3(lut *LutDataType, options TransformOptions, pcs DataColorSpace) (Stage, error) {
	l, err := makeLut3x3(lut, options, pcs)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Transform is Stage::transform for Lut3x3 in moxcms
// src/conversions/lut3x3.rs. The subset has the trilinear interpolation
// only, which upstream also uses for a PCS of Lab or XYZ.
func (l *lut3x3) Transform(src, dst []float32) error {
	tbl, err := NewCube(l.clut, int(l.gridSize), 3)
	if err != nil {
		return err
	}
	for i := 0; i+3 <= len(src) && i+3 <= len(dst); i += 3 {
		linearX := lutInterpLinearFloat(src[i], l.input[0])
		linearY := lutInterpLinearFloat(src[i+1], l.input[1])
		linearZ := lutInterpLinearFloat(src[i+2], l.input[2])

		clut := tbl.TrilinearVec3(linearX, linearY, linearZ)

		dst[i] = lutInterpLinearFloat(clut.V[0], l.gamma[0])
		dst[i+1] = lutInterpLinearFloat(clut.V[1], l.gamma[1])
		dst[i+2] = lutInterpLinearFloat(clut.V[2], l.gamma[2])
	}
	return nil
}

// createLut3x3 is create_lut3x3 in moxcms src/conversions/lut3x3.rs. It
// returns the result of the LUT on each triplet of src.
func createLut3x3(lut *LutDataType, src []float32, options TransformOptions, pcs DataColorSpace) ([]float32, error) {
	if lut.NumInputChannels != 3 || lut.NumOutputChannels != 3 {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	dest := make([]float32, len(src))
	stage, err := stageLut3x3(lut, options, pcs)
	if err != nil {
		return nil, err
	}
	if err := stage.Transform(src, dest); err != nil {
		return nil, err
	}
	return dest, nil
}
