// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/xyy.rs.

package moxcms

// XyY is XyY in moxcms src/xyy.rs. It holds a color in CIE xyY. Yb is the
// luminance Y.
type XyY struct {
	X, Y, Yb float64
}

// NewXyY is XyY::new in moxcms src/xyy.rs.
func NewXyY(x, y, yb float64) XyY {
	return XyY{X: x, Y: y, Yb: yb}
}

// ToXyz is XyY::to_xyz in moxcms src/xyy.rs.
func (c XyY) ToXyz() Xyz {
	reciprocal := 0.0
	if c.Y != 0 {
		reciprocal = 1 / c.Y * c.Yb
	}
	return Xyz{
		X: float32(c.X * reciprocal),
		Y: float32(c.Yb),
		Z: float32((1 - c.X - c.Y) * reciprocal),
	}
}

// ToXyzd is XyY::to_xyzd in moxcms src/xyy.rs.
func (c XyY) ToXyzd() Xyzd {
	reciprocal := 0.0
	if c.Y != 0 {
		reciprocal = 1 / c.Y * c.Yb
	}
	return Xyzd{
		X: c.X * reciprocal,
		Y: c.Yb,
		Z: (1 - c.X - c.Y) * reciprocal,
	}
}
