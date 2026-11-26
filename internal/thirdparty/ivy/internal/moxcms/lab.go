// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/lab.rs.

package moxcms

// Lab is Lab in moxcms src/lab.rs. It holds a color in CIE L*a*b*.
type Lab struct {
	// L is the lightness, from 0 to 100.
	L float32
	// A is the component from green, negative, to red, positive.
	A float32
	// B is the component from blue, negative, to yellow, positive.
	B float32
}

// NewLab is Lab::new in moxcms src/lab.rs.
func NewLab(l, a, b float32) Lab {
	return Lab{L: l, A: a, B: b}
}

// labF1 is f_1 in moxcms src/lab.rs. It is the inverse of labF.
func labF1(t float32) float32 {
	if t <= 24.0/116.0 {
		return (108.0 / 841.0) * (t - 16.0/116.0)
	}
	return t * t * t
}

// labF is f in moxcms src/lab.rs.
func labF(t float32) float32 {
	// Port: the variable keeps Go from an exact product of constants.
	k := float32(24.0 / 116.0)
	if t <= k*k*k {
		return float32(841.0/108.0*t) + 16.0/116.0
	}
	return cbrtF32(t)
}

// labPcsScale is 1 + 32767/32768, the scale of the encoding of XYZ in the
// profile connection space.
const labPcsScale = 1.0 + 32767.0/32768.0

// FromPcsXyz is Lab::from_pcs_xyz in moxcms src/lab.rs. It converts a color
// of the XYZ encoding of the profile connection space to the Lab encoding of
// the profile connection space, with each component from 0 to 1.
func FromPcsXyz(xyz Xyz) Lab {
	wp := ChromaticityD50.ToXyz()
	deviceX := float32(float64(xyz.X) * labPcsScale / float64(wp.X))
	deviceY := float32(float64(xyz.Y) * labPcsScale / float64(wp.Y))
	deviceZ := float32(float64(xyz.Z) * labPcsScale / float64(wp.Z))

	fx := labF(deviceX)
	fy := labF(deviceY)
	fz := labF(deviceZ)

	lb := mlaf(-16.0, 116.0, fy)
	a := 500.0 * (fx - fy)
	b := 200.0 * (fy - fz)

	return NewLab(lb/100.0, (a+128.0)/255.0, (b+128.0)/255.0)
}

// ToPcsXyz is Lab::to_pcs_xyz in moxcms src/lab.rs. It converts a color of
// the Lab encoding of the profile connection space to the XYZ encoding of
// the profile connection space.
func (c Lab) ToPcsXyz() Xyz {
	deviceL := c.L * 100.0
	deviceA := fmla(c.A, 255.0, -128.0)
	deviceB := fmla(c.B, 255.0, -128.0)

	y := (deviceL + 16.0) / 116.0

	wp := ChromaticityD50.ToXyz()

	x := labF1(mlaf(y, 0.002, deviceA)) * wp.X
	y1 := labF1(y) * wp.Y
	z := labF1(mlaf(y, -0.005, deviceB)) * wp.Z

	return NewXyz(
		float32(float64(x)/labPcsScale),
		float32(float64(y1)/labPcsScale),
		float32(float64(z)/labPcsScale),
	)
}
