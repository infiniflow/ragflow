// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/chromaticity.rs.

package moxcms

// Chromaticity is Chromaticity in moxcms src/chromaticity.rs. It holds the
// chromaticity coordinates x and y of a color.
type Chromaticity struct {
	X, Y float32
}

// The standard illuminants of Chromaticity in moxcms src/chromaticity.rs.
var (
	// ChromaticityD65 is Chromaticity::D65.
	ChromaticityD65 = Chromaticity{X: 0.31272, Y: 0.32903}
	// ChromaticityD50 is Chromaticity::D50.
	ChromaticityD50 = Chromaticity{X: 0.34567, Y: 0.35850}
)

// ToXyz is Chromaticity::to_xyz in moxcms src/chromaticity.rs. It returns
// the color with the luminance 1.
func (c Chromaticity) ToXyz() Xyz {
	var reciprocal float32
	if c.Y != 0 {
		reciprocal = 1 / c.Y
	}
	return Xyz{
		X: c.X * reciprocal,
		Y: 1,
		Z: (1 - c.X - c.Y) * reciprocal,
	}
}

// ToXyzd is Chromaticity::to_xyzd in moxcms src/chromaticity.rs. It returns
// the color with the luminance 1.
func (c Chromaticity) ToXyzd() Xyzd {
	var reciprocal float32
	if c.Y != 0 {
		reciprocal = 1 / c.Y
	}
	return Xyzd{
		X: float64(c.X) * float64(reciprocal),
		Y: 1,
		Z: (1 - float64(c.X) - float64(c.Y)) * float64(reciprocal),
	}
}

// ToXyYb is Chromaticity::to_xyyb in moxcms src/chromaticity.rs. It returns
// the color in xyY with the luminance 1.
func (c Chromaticity) ToXyYb() XyY {
	return XyY{X: float64(c.X), Y: float64(c.Y), Yb: 1}
}
