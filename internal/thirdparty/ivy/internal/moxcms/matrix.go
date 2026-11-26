// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/matrix.rs.

package moxcms

import "math"

// Port: upstream declares Vector3<T>, Vector4<T> and Matrix3<T> for each
// type T. The port declares only the types of the subset: Vector3f,
// Vector3d, Matrix3f and Matrix3d.

// Vector3f is Vector3f in moxcms src/matrix.rs. It is a vector of three f32.
type Vector3f struct {
	V [3]float32
}

// Vector3d is Vector3d in moxcms src/matrix.rs. It is a vector of three f64.
type Vector3d struct {
	V [3]float64
}

// vector3fFrom is impl From<T> for Vector3<T> in moxcms src/matrix.rs. It
// returns a vector with v in each component.
func vector3fFrom(v float32) Vector3f {
	return Vector3f{V: [3]float32{v, v, v}}
}

// Equal is impl PartialEq for Vector3<T> in moxcms src/matrix.rs. It reports
// whether each component of v differs from the one of o by less than 0.0001.
func (v Vector3f) Equal(o Vector3f) bool {
	const tolerance = float32(0.0001)
	dx := absF32(v.V[0] - o.V[0])
	dy := absF32(v.V[1] - o.V[1])
	dz := absF32(v.V[2] - o.V[2])
	return dx < tolerance && dy < tolerance && dz < tolerance
}

// Equal is impl PartialEq for Vector3<T> in moxcms src/matrix.rs. It compares
// the components of v and o as f32, with a tolerance of 0.0001.
func (v Vector3d) Equal(o Vector3d) bool {
	return v.toF32().Equal(o.toF32())
}

// toF32 is Vector3::cast in moxcms src/matrix.rs, from f64 to f32.
func (v Vector3d) toF32() Vector3f {
	return Vector3f{V: [3]float32{float32(v.V[0]), float32(v.V[1]), float32(v.V[2])}}
}

// mla is impl FusedMultiplyAdd for Vector3<T> in moxcms src/matrix.rs. It
// returns v + b*c, component by component.
func (v Vector3f) mla(b, c Vector3f) Vector3f {
	return Vector3f{V: [3]float32{
		mlaf(v.V[0], b.V[0], c.V[0]),
		mlaf(v.V[1], b.V[1], c.V[1]),
		mlaf(v.V[2], b.V[2], c.V[2]),
	}}
}

// negMla is impl FusedMultiplyNegAdd for Vector3<T> in moxcms
// src/matrix.rs. It returns v - b*c, component by component.
func (v Vector3f) negMla(b, c Vector3f) Vector3f {
	return Vector3f{V: [3]float32{
		negMlaf(v.V[0], b.V[0], c.V[0]),
		negMlaf(v.V[1], b.V[1], c.V[1]),
		negMlaf(v.V[2], b.V[2], c.V[2]),
	}}
}

// Matrix3f is Matrix3f in moxcms src/matrix.rs. It is a 3x3 matrix of f32,
// in rows.
type Matrix3f struct {
	V [3][3]float32
}

// Matrix3d is Matrix3d in moxcms src/matrix.rs. It is a 3x3 matrix of f64,
// in rows.
type Matrix3d struct {
	V [3][3]float64
}

// Matrix3fIdentity is Matrix3f::IDENTITY in moxcms src/matrix.rs.
var Matrix3fIdentity = Matrix3f{V: [3][3]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

// Matrix3dIdentity is Matrix3d::IDENTITY in moxcms src/matrix.rs.
var Matrix3dIdentity = Matrix3d{V: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

// TestEquality is Matrix3f::test_equality in moxcms src/matrix.rs. It reports
// whether no element of m differs from the one of o by more than 0.001.
func (m Matrix3f) TestEquality(o Matrix3f) bool {
	const tolerance = float32(0.001)
	for i := range 3 {
		for j := range 3 {
			if absF32(m.V[i][j]-o.V[i][j]) > tolerance {
				return false
			}
		}
	}
	return true
}

// TestEquality is Matrix3d::test_equality in moxcms src/matrix.rs. It reports
// whether no element of m differs from the one of o by more than 0.001.
func (m Matrix3d) TestEquality(o Matrix3d) bool {
	const tolerance = 0.001
	for i := range 3 {
		for j := range 3 {
			if math.Abs(m.V[i][j]-o.V[i][j]) > tolerance {
				return false
			}
		}
	}
	return true
}

// Determinant is Matrix3d::determinant in moxcms src/matrix.rs. It reports
// false when the determinant is zero.
func (m Matrix3d) Determinant() (float64, bool) {
	v := m.V
	a0 := float64(v[0][0]*v[1][1]) * v[2][2]
	a1 := float64(v[0][1]*v[1][2]) * v[2][0]
	a2 := float64(v[0][2]*v[1][0]) * v[2][1]

	s0 := float64(v[0][2]*v[1][1]) * v[2][0]
	s1 := float64(v[0][1]*v[1][0]) * v[2][2]
	s2 := float64(v[0][0]*v[1][2]) * v[2][1]

	j := a0 + a1 + a2 - s0 - s1 - s2
	if j == 0 {
		return 0, false
	}
	return j, true
}

// Inverse is Matrix3d::inverse in moxcms src/matrix.rs. It returns the
// identity when m has no inverse.
func (m Matrix3d) Inverse() Matrix3d {
	v := m.V
	determinant, ok := m.Determinant()
	if !ok {
		return Matrix3dIdentity
	}
	det := 1 / determinant
	a, b, c := v[0][0], v[0][1], v[0][2]
	d, e, f := v[1][0], v[1][1], v[1][2]
	g, h, i := v[2][0], v[2][1], v[2][2]
	return Matrix3d{V: [3][3]float64{
		{
			(float64(e*i) - float64(f*h)) * det,
			(float64(c*h) - float64(b*i)) * det,
			(float64(b*f) - float64(c*e)) * det,
		},
		{
			(float64(f*g) - float64(d*i)) * det,
			(float64(a*i) - float64(c*g)) * det,
			(float64(c*d) - float64(a*f)) * det,
		},
		{
			(float64(d*h) - float64(e*g)) * det,
			(float64(b*g) - float64(a*h)) * det,
			(float64(a*e) - float64(b*d)) * det,
		},
	}}
}

// mulRowVector is Matrix3d::mul_row_vector in moxcms src/matrix.rs. It
// multiplies row r of m by rhs, component by component.
func (m Matrix3d) mulRowVector(r int, rhs Vector3d) Matrix3d {
	for j := range 3 {
		m.V[r][j] *= rhs.V[j]
	}
	return m
}

// MulVector is Matrix3d::mul_vector in moxcms src/matrix.rs. It returns the
// product of m and the column vector o.
func (m Matrix3d) MulVector(o Vector3d) Vector3d {
	x := float64(m.V[0][1]*o.V[1]) + float64(m.V[0][2]*o.V[2]) + float64(m.V[0][0]*o.V[0])
	y := float64(m.V[1][0]*o.V[0]) + float64(m.V[1][1]*o.V[1]) + float64(m.V[1][2]*o.V[2])
	z := float64(m.V[2][0]*o.V[0]) + float64(m.V[2][1]*o.V[1]) + float64(m.V[2][2]*o.V[2])
	return Vector3d{V: [3]float64{x, y, z}}
}

// MatMul is Matrix3d::mat_mul in moxcms src/matrix.rs. It returns the
// product m*o.
func (m Matrix3d) MatMul(o Matrix3d) Matrix3d {
	var r Matrix3d
	for i := range 3 {
		for j := range 3 {
			r.V[i][j] = mlafF64(mlafF64(float64(m.V[i][0]*o.V[0][j]), m.V[i][1], o.V[1][j]), m.V[i][2], o.V[2][j])
		}
	}
	return r
}

// matMulConst is Matrix3d::mat_mul_const in moxcms src/matrix.rs. It returns
// the product m*o.
func (m Matrix3d) matMulConst(o Matrix3d) Matrix3d {
	var r Matrix3d
	for i := range 3 {
		for j := range 3 {
			r.V[i][j] = float64(m.V[i][0]*o.V[0][j]) + float64(m.V[i][1]*o.V[1][j]) + float64(m.V[i][2]*o.V[2][j])
		}
	}
	return r
}

// ToF32 is Matrix3d::to_f32 in moxcms src/matrix.rs.
func (m Matrix3d) ToF32() Matrix3f {
	var r Matrix3f
	for i := range 3 {
		for j := range 3 {
			r.V[i][j] = float32(m.V[i][j])
		}
	}
	return r
}

// Xyz is Xyz in moxcms src/matrix.rs. It holds a color in CIE XYZ, in f32.
type Xyz struct {
	X, Y, Z float32
}

// Xyzd is Xyzd in moxcms src/matrix.rs. It holds a color in CIE XYZ, in f64.
type Xyzd struct {
	X, Y, Z float64
}

// NewXyz is Xyz::new in moxcms src/matrix.rs.
func NewXyz(x, y, z float32) Xyz {
	return Xyz{X: x, Y: y, Z: z}
}

// NewXyzd is Xyzd::new in moxcms src/matrix.rs.
func NewXyzd(x, y, z float64) Xyzd {
	return Xyzd{X: x, Y: y, Z: z}
}

// Equal is impl PartialEq for Xyz in moxcms src/matrix.rs. It reports
// whether each component of a differs from the one of b by less than 0.0001.
func (a Xyz) Equal(b Xyz) bool {
	const tolerance = float32(0.0001)
	return absF32(a.X-b.X) < tolerance && absF32(a.Y-b.Y) < tolerance && absF32(a.Z-b.Z) < tolerance
}

// Equal is impl PartialEq for Xyzd in moxcms src/matrix.rs. It reports
// whether each component of a differs from the one of b by less than 0.0001.
func (a Xyzd) Equal(b Xyzd) bool {
	const tolerance = 0.0001
	return math.Abs(a.X-b.X) < tolerance && math.Abs(a.Y-b.Y) < tolerance && math.Abs(a.Z-b.Z) < tolerance
}

// ToVectorD is Xyzd::to_vector_d in moxcms src/matrix.rs.
func (a Xyzd) ToVectorD() Vector3d {
	return Vector3d{V: [3]float64{a.X, a.Y, a.Z}}
}

// ToXyzd is Xyz::to_xyzd in moxcms src/matrix.rs.
func (a Xyz) ToXyzd() Xyzd {
	return Xyzd{X: float64(a.X), Y: float64(a.Y), Z: float64(a.Z)}
}

// ToXyz is Xyzd::to_xyz in moxcms src/matrix.rs.
func (a Xyzd) ToXyz() Xyz {
	return Xyz{X: float32(a.X), Y: float32(a.Y), Z: float32(a.Z)}
}
