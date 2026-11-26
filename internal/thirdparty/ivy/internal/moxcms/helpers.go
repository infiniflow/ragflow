// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/helpers.rs.

package moxcms

import (
	"encoding/binary"
	"math"
)

// ToClutF32 is LutStore::to_clut_f32 in moxcms src/helpers.rs.
func (s LutStore8) ToClutF32() []float32 {
	r := make([]float32, len(s))
	for i, x := range s {
		r[i] = uint8NumberToFloatFast(x)
	}
	return r
}

// ToClutF32 is LutStore::to_clut_f32 in moxcms src/helpers.rs.
func (s LutStore16) ToClutF32() []float32 {
	r := make([]float32, len(s))
	for i, x := range s {
		r[i] = uint16NumberToFloatFast(uint32(x))
	}
	return r
}

// isLinear is ToneReprCurve::is_linear in moxcms src/helpers.rs.
func (c *ToneReprCurve) isLinear() bool {
	if c.Kind == CurveLut {
		lut := c.Lut
		if len(lut) == 0 {
			return true
		}
		if len(lut) == 1 {
			gamma := 1 / u8Fixed8NumberToFloat(lut[0])
			if math.Abs(float64(gamma-1)) < 1e-4 {
				return true
			}
		}
		return isCurveLinear16(lut)
	}
	p := c.Parametric
	if len(p) == 0 {
		return true
	}
	return len(p) == 1 && p[0] == 1
}

// readMatrix3d is read_matrix_3d in moxcms src/helpers.rs. It reads nine
// s15Fixed16Number values in rows.
func readMatrix3d(b []byte) (Matrix3d, error) {
	if len(b) < 36 {
		return Matrix3d{}, cmsError(InvalidProfile)
	}
	var m Matrix3d
	for i := range 3 {
		for j := range 3 {
			m.V[i][j] = s15Fixed16NumberToDouble(int32(binary.BigEndian.Uint32(b[(3*i+j)*4:])))
		}
	}
	return m, nil
}

// readVector3d is read_vector_3d in moxcms src/helpers.rs. It reads three
// s15Fixed16Number values.
func readVector3d(b []byte) (Vector3d, error) {
	if len(b) < 12 {
		return Vector3d{}, cmsError(InvalidProfile)
	}
	var v Vector3d
	for i := range 3 {
		v.V[i] = s15Fixed16NumberToDouble(int32(binary.BigEndian.Uint32(b[i*4:])))
	}
	return v, nil
}
