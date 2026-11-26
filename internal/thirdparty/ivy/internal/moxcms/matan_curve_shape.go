// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/matan/curve_shape.rs.

package moxcms

// isCurveLinear16 is is_curve_linear16 in moxcms src/matan/curve_shape.rs.
// It reports whether no entry of curve differs from a straight line from 0
// to 65535 by more than 15.
func isCurveLinear16(curve []uint16) bool {
	scale := 1 / float32(len(curve)-1) * 65535
	for index, value := range curve {
		quantized := f32ToU16(roundF32(float32(index) * scale))
		diff := int32(quantized) - int32(value)
		if diff < 0 {
			diff = -diff
		}
		if diff > 0x0f {
			return false
		}
	}
	return true
}
