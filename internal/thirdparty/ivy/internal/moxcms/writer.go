// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/writer.rs.

package moxcms

import "math"

// toU8Fixed8 is FloatToFixedU8Fixed8::to_u8_fixed8 for f32 in moxcms
// src/writer.rs. It encodes v as a u8Fixed8Number of ICC.1.
func toU8Fixed8(v float32) uint16 {
	switch {
	case v > 255.0+255.0/256:
		return 0xffff
	case v < 0:
		return 0
	default:
		return f32ToU16(float32(math.Floor(float64(float32(v*256) + 0.5))))
	}
}
