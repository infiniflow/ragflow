// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/mab.rs.

package moxcms

// The constants of prepare_mab_3x3 and prepare_mba_3x3 in moxcms
// src/conversions/mab.rs. A curve of an mAB or mBA tag becomes a table of
// lerpDepth entries for samples of mabBp bits, which a stage reads in mabDepth
// steps.
const (
	lerpDepth = 65536
	mabBp     = 13
	mabDepth  = 8192
)

// curveIndex is the index into a curve table of a value from 0 to 1, as the
// stages of moxcms src/conversions/mab.rs compute it.
func curveIndex(v, scaleValue float32) uint16 {
	return f32ToU16(minF32(roundF32(v*scaleValue), scaleValue))
}

// aCurves3 is ACurves3 in moxcms src/conversions/mab.rs. It applies the A
// curves and then the CLUT of an mAB tag.
type aCurves3 struct {
	curves              [3][]float32
	clut                []float32
	gridSize            [3]uint8
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
	depth               int
}

// Transform is InPlaceStage::transform for ACurves3 in moxcms
// src/conversions/mab.rs.
func (s *aCurves3) Transform(dst []float32) error {
	lut, err := NewCubeGrid(s.clut, s.gridSize, 3)
	if err != nil {
		return err
	}
	scaleValue := float32(s.depth - 1)
	for i := 0; i+3 <= len(dst); i += 3 {
		b0 := s.curves[0][curveIndex(dst[i], scaleValue)]
		b1 := s.curves[1][curveIndex(dst[i+1], scaleValue)]
		b2 := s.curves[2][curveIndex(dst[i+2], scaleValue)]
		v := lut.TrilinearVec3(b0, b1, b2)
		dst[i], dst[i+1], dst[i+2] = v.V[0], v.V[1], v.V[2]
	}
	return nil
}

// aCurves3Optimized is ACurves3Optimized in moxcms src/conversions/mab.rs.
// It applies the CLUT of an mAB tag whose A curves are linear.
type aCurves3Optimized struct {
	clut                []float32
	gridSize            [3]uint8
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
}

// Transform is InPlaceStage::transform for ACurves3Optimized in moxcms
// src/conversions/mab.rs.
func (s *aCurves3Optimized) Transform(dst []float32) error {
	lut, err := NewCubeGrid(s.clut, s.gridSize, 3)
	if err != nil {
		return err
	}
	for i := 0; i+3 <= len(dst); i += 3 {
		v := lut.TrilinearVec3(dst[i], dst[i+1], dst[i+2])
		dst[i], dst[i+1], dst[i+2] = v.V[0], v.V[1], v.V[2]
	}
	return nil
}

// aCurves3Inverse is ACurves3Inverse in moxcms src/conversions/mab.rs. It
// applies the CLUT and then the A curves of an mBA tag.
type aCurves3Inverse struct {
	curves              [3][]float32
	clut                []float32
	gridSize            [3]uint8
	interpolationMethod InterpolationMethod
	pcs                 DataColorSpace
	depth               int
}

// Transform is InPlaceStage::transform for ACurves3Inverse in moxcms
// src/conversions/mab.rs.
func (s *aCurves3Inverse) Transform(dst []float32) error {
	lut, err := NewCubeGrid(s.clut, s.gridSize, 3)
	if err != nil {
		return err
	}
	scaleValue := float32(s.depth - 1)
	for i := 0; i+3 <= len(dst); i += 3 {
		v := lut.TrilinearVec3(dst[i], dst[i+1], dst[i+2])
		dst[i] = s.curves[0][curveIndex(v.V[0], scaleValue)]
		dst[i+1] = s.curves[1][curveIndex(v.V[1], scaleValue)]
		dst[i+2] = s.curves[2][curveIndex(v.V[2], scaleValue)]
	}
	return nil
}

// mCurves3 is MCurves3 in moxcms src/conversions/mab.rs. It applies the M
// curves and the matrix of an mAB or mBA tag. inverse puts the matrix
// first, for an mBA tag.
type mCurves3 struct {
	curves  [3][]float32
	matrix  Matrix3f
	bias    Vector3f
	inverse bool
	depth   int
}

// executeMatrixStage is MCurves3::execute_matrix_stage in moxcms
// src/conversions/mab.rs.
func (s *mCurves3) executeMatrixStage(dst []float32) {
	m, b := s.matrix, s.bias
	if m.TestEquality(Matrix3fIdentity) && b.Equal(Vector3f{}) {
		return
	}
	for i := 0; i+3 <= len(dst); i += 3 {
		x, y, z := dst[i], dst[i+1], dst[i+2]
		dst[i] = mlaf(mlaf(mlaf(b.V[0], x, m.V[0][0]), y, m.V[0][1]), z, m.V[0][2])
		dst[i+1] = mlaf(mlaf(mlaf(b.V[1], x, m.V[1][0]), y, m.V[1][1]), z, m.V[1][2])
		dst[i+2] = mlaf(mlaf(mlaf(b.V[2], x, m.V[2][0]), y, m.V[2][1]), z, m.V[2][2])
	}
}

// Transform is InPlaceStage::transform for MCurves3 in moxcms
// src/conversions/mab.rs.
func (s *mCurves3) Transform(dst []float32) error {
	scaleValue := float32(s.depth - 1)
	if s.inverse {
		s.executeMatrixStage(dst)
	}
	for i := 0; i+3 <= len(dst); i += 3 {
		dst[i] = s.curves[0][curveIndex(dst[i], scaleValue)]
		dst[i+1] = s.curves[1][curveIndex(dst[i+1], scaleValue)]
		dst[i+2] = s.curves[2][curveIndex(dst[i+2], scaleValue)]
	}
	if !s.inverse {
		s.executeMatrixStage(dst)
	}
	return nil
}

// bCurves3 is BCurves3<DEPTH> in moxcms src/conversions/mab.rs. It applies
// the B curves of an mAB or mBA tag.
type bCurves3 struct {
	curves [3][]float32
	depth  int
}

// Transform is InPlaceStage::transform for BCurves3 in moxcms
// src/conversions/mab.rs.
func (s *bCurves3) Transform(dst []float32) error {
	scaleValue := float32(s.depth - 1)
	for i := 0; i+3 <= len(dst); i += 3 {
		dst[i] = s.curves[0][curveIndex(dst[i], scaleValue)]
		dst[i+1] = s.curves[1][curveIndex(dst[i+1], scaleValue)]
		dst[i+2] = s.curves[2][curveIndex(dst[i+2], scaleValue)]
	}
	return nil
}

// allCurvesLinear reports whether each curve is linear.
func allCurvesLinear(curves []*ToneReprCurve) bool {
	for _, c := range curves {
		if !c.isLinear() {
			return false
		}
	}
	return true
}

// buildCurveTables builds the tables of the curves of an mAB or mBA tag for
// a stage. It returns InvalidTrcCurve when a curve has no table, or when the
// tag has not want curves.
func buildCurveTables(curves []*ToneReprCurve, want int) ([][]float32, error) {
	tables := make([][]float32, 0, len(curves))
	for _, c := range curves {
		t, ok := c.buildLinearizeTable(lerpDepth, mabBp)
		if !ok {
			return nil, cmsError(InvalidTrcCurve)
		}
		tables = append(tables, t)
	}
	if len(tables) != want {
		return nil, cmsError(InvalidTrcCurve)
	}
	return tables, nil
}

// clutGrid3 returns the CLUT of an mAB or mBA tag with three inputs, and
// checks its length, as prepare_mab_3x3 and prepare_mba_3x3 in moxcms do.
func clutGrid3(mab *LutMultidimensionalType) ([]float32, error) {
	clut := mab.Clut.ToClutF32()
	n, err := safeMul(uint64(mab.GridPoints[0]), uint64(mab.GridPoints[1]))
	if err == nil {
		n, err = safeMul(n, uint64(mab.GridPoints[2]))
	}
	if err == nil {
		n, err = safeMul(n, uint64(mab.NumOutputChannels))
	}
	if err != nil {
		return nil, err
	}
	if uint64(len(clut)) != n {
		return nil, &CmsError{Kind: MalformedCurveLutTable, Size: MalformedSize{Size: len(clut), Expected: int(n)}}
	}
	return clut, nil
}

// prepareMCurves returns the stage of the M curves and the matrix of mab,
// or nil when the curves are linear and the matrix is the identity.
func prepareMCurves(mab *LutMultidimensionalType, inverse bool) (*mCurves3, error) {
	if allCurvesLinear(mab.MCurves) && mab.Matrix.TestEquality(Matrix3dIdentity) && mab.Bias.Equal(Vector3d{}) {
		return nil, nil
	}
	tables, err := buildCurveTables(mab.MCurves, 3)
	if err != nil {
		return nil, err
	}
	return &mCurves3{
		curves:  [3][]float32{tables[0], tables[1], tables[2]},
		matrix:  mab.Matrix.ToF32(),
		bias:    mab.Bias.toF32(),
		inverse: inverse,
		depth:   mabDepth,
	}, nil
}

// prepareBCurves returns the stage of the B curves of mab, or nil when the
// curves are linear.
func prepareBCurves(mab *LutMultidimensionalType) (*bCurves3, error) {
	if allCurvesLinear(mab.BCurves) {
		return nil, nil
	}
	tables, err := buildCurveTables(mab.BCurves, 3)
	if err != nil {
		return nil, err
	}
	return &bCurves3{curves: [3][]float32{tables[0], tables[1], tables[2]}, depth: mabDepth}, nil
}

// prepareMab3x3 is prepare_mab_3x3 in moxcms src/conversions/mab.rs. It
// applies an mAB tag with three inputs and three outputs to each triplet of
// lut.
func prepareMab3x3(mab *LutMultidimensionalType, lut []float32, options TransformOptions, pcs DataColorSpace) error {
	if mab.NumInputChannels != 3 || mab.NumOutputChannels != 3 {
		return cmsError(UnsupportedProfileConnection)
	}
	if len(mab.ACurves) == 3 && mab.Clut != nil {
		clut, err := clutGrid3(mab)
		if err != nil {
			return err
		}
		gridSize := [3]uint8{mab.GridPoints[0], mab.GridPoints[1], mab.GridPoints[2]}
		if allCurvesLinear(mab.ACurves) {
			l := &aCurves3Optimized{clut: clut, gridSize: gridSize, interpolationMethod: options.InterpolationMethod, pcs: pcs}
			if err := l.Transform(lut); err != nil {
				return err
			}
		} else {
			tables, err := buildCurveTables(mab.ACurves, 3)
			if err != nil {
				return err
			}
			l := &aCurves3{
				curves:              [3][]float32{tables[0], tables[1], tables[2]},
				clut:                clut,
				gridSize:            gridSize,
				interpolationMethod: options.InterpolationMethod,
				pcs:                 pcs,
				depth:               mabDepth,
			}
			if err := l.Transform(lut); err != nil {
				return err
			}
		}
	}

	if len(mab.MCurves) == 3 {
		m, err := prepareMCurves(mab, false)
		if err != nil {
			return err
		}
		if m != nil {
			if err := m.Transform(lut); err != nil {
				return err
			}
		}
	}

	if len(mab.BCurves) != 3 {
		return cmsError(InvalidAtoBLut)
	}
	b, err := prepareBCurves(mab)
	if err != nil {
		return err
	}
	if b != nil {
		return b.Transform(lut)
	}
	return nil
}

// prepareMba3x3 is prepare_mba_3x3 in moxcms src/conversions/mab.rs. It
// applies an mBA tag with three inputs and three outputs to each triplet of
// lut.
func prepareMba3x3(mab *LutMultidimensionalType, lut []float32, options TransformOptions, pcs DataColorSpace) error {
	if mab.NumInputChannels != 3 || mab.NumOutputChannels != 3 {
		return cmsError(UnsupportedProfileConnection)
	}

	if len(mab.BCurves) != 3 {
		return cmsError(InvalidAtoBLut)
	}
	b, err := prepareBCurves(mab)
	if err != nil {
		return err
	}
	if b != nil {
		if err := b.Transform(lut); err != nil {
			return err
		}
	}

	if len(mab.MCurves) == 3 {
		m, err := prepareMCurves(mab, true)
		if err != nil {
			return err
		}
		if m != nil {
			if err := m.Transform(lut); err != nil {
				return err
			}
		}
	}

	if len(mab.ACurves) == 3 && mab.Clut != nil {
		clut, err := clutGrid3(mab)
		if err != nil {
			return err
		}
		gridSize := [3]uint8{mab.GridPoints[0], mab.GridPoints[1], mab.GridPoints[2]}
		if allCurvesLinear(mab.ACurves) {
			l := &aCurves3Optimized{clut: clut, gridSize: gridSize, interpolationMethod: options.InterpolationMethod, pcs: pcs}
			return l.Transform(lut)
		}
		tables, err := buildCurveTables(mab.ACurves, 3)
		if err != nil {
			return err
		}
		l := &aCurves3Inverse{
			curves:              [3][]float32{tables[0], tables[1], tables[2]},
			clut:                clut,
			gridSize:            gridSize,
			interpolationMethod: options.InterpolationMethod,
			pcs:                 pcs,
			depth:               mabDepth,
		}
		return l.Transform(lut)
	}
	return nil
}
