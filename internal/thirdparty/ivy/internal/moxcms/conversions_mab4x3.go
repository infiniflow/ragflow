// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/mab4x3.rs.

package moxcms

// aCurves4x3 is ACurves4x3 in moxcms src/conversions/mab4x3.rs. It applies
// the A curves and then the CLUT of an mAB tag with four inputs.
type aCurves4x3 struct {
	curves              [4][]float32
	clut                []float32
	gridSize            [4]uint8
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
	depth               int
}

// Transform is Stage::transform for ACurves4x3 in moxcms
// src/conversions/mab4x3.rs. It reads four values and writes three for each
// pixel.
func (s *aCurves4x3) Transform(src, dst []float32) error {
	lut, err := NewHypercubeGrid(s.clut, s.gridSize, 3)
	if err != nil {
		return err
	}
	scaleValue := float32(s.depth - 1)
	for i, j := 0, 0; i+4 <= len(src) && j+3 <= len(dst); i, j = i+4, j+3 {
		c := s.curves[0][curveIndex(src[i], scaleValue)]
		m := s.curves[1][curveIndex(src[i+1], scaleValue)]
		y := s.curves[2][curveIndex(src[i+2], scaleValue)]
		k := s.curves[3][curveIndex(src[i+3], scaleValue)]
		r := lut.QuadlinearVec3(c, m, y, k)
		dst[j], dst[j+1], dst[j+2] = r.V[0], r.V[1], r.V[2]
	}
	return nil
}

// aCurves4x3Optimized is ACurves4x3Optimized in moxcms
// src/conversions/mab4x3.rs. It applies the CLUT of an mAB tag with four
// inputs whose A curves are linear.
type aCurves4x3Optimized struct {
	clut                []float32
	gridSize            [4]uint8
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
}

// Transform is Stage::transform for ACurves4x3Optimized in moxcms
// src/conversions/mab4x3.rs.
func (s *aCurves4x3Optimized) Transform(src, dst []float32) error {
	lut, err := NewHypercubeGrid(s.clut, s.gridSize, 3)
	if err != nil {
		return err
	}
	for i, j := 0, 0; i+4 <= len(src) && j+3 <= len(dst); i, j = i+4, j+3 {
		r := lut.QuadlinearVec3(src[i], src[i+1], src[i+2], src[i+3])
		dst[j], dst[j+1], dst[j+2] = r.V[0], r.V[1], r.V[2]
	}
	return nil
}

// prepareMab4x3 is prepare_mab_4x3 in moxcms src/conversions/mab4x3.rs. It
// applies an mAB tag with four inputs and three outputs to each node of lut,
// which holds four values for each node. It returns three values for each
// node.
func prepareMab4x3(mab *LutMultidimensionalType, lut []float32, options TransformOptions, pcs DataColorSpace) ([]float32, error) {
	if mab.NumInputChannels != 4 || mab.NumOutputChannels != 3 {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	newLut := make([]float32, len(lut)/4*3)
	if len(mab.ACurves) != 4 || mab.Clut == nil {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	clut := mab.Clut.ToClutF32()
	n := uint64(1)
	var err error
	for _, g := range mab.GridPoints[:4] {
		if n, err = safeMul(n, uint64(g)); err != nil {
			return nil, err
		}
	}
	if n, err = safeMul(n, uint64(mab.NumOutputChannels)); err != nil {
		return nil, err
	}
	if uint64(len(clut)) != n {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(clut), Expected: int(n)}}
	}
	gridSize := [4]uint8{mab.GridPoints[0], mab.GridPoints[1], mab.GridPoints[2], mab.GridPoints[3]}
	if allCurvesLinear(mab.ACurves) {
		l := &aCurves4x3Optimized{clut: clut, gridSize: gridSize, interpolationMethod: options.InterpolationMethod, pcs: pcs}
		if err := l.Transform(lut, newLut); err != nil {
			return nil, err
		}
	} else {
		tables, err := buildCurveTables(mab.ACurves, 4)
		if err != nil {
			return nil, err
		}
		l := &aCurves4x3{
			curves:              [4][]float32{tables[0], tables[1], tables[2], tables[3]},
			clut:                clut,
			gridSize:            gridSize,
			interpolationMethod: options.InterpolationMethod,
			pcs:                 pcs,
			depth:               mabDepth,
		}
		if err := l.Transform(lut, newLut); err != nil {
			return nil, err
		}
	}

	if len(mab.MCurves) == 3 {
		m, err := prepareMCurves(mab, false)
		if err != nil {
			return nil, err
		}
		if m != nil {
			if err := m.Transform(newLut); err != nil {
				return nil, err
			}
		}
	}

	if len(mab.BCurves) != 3 {
		return nil, cmsError(InvalidAtoBLut)
	}
	b, err := prepareBCurves(mab)
	if err != nil {
		return nil, err
	}
	if b != nil {
		if err := b.Transform(newLut); err != nil {
			return nil, err
		}
	}
	return newLut, nil
}
