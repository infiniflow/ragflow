// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/math/mod.rs.

package moxcms

import "math"

// mClamp is m_clamp in moxcms src/math/mod.rs. It returns min when a is NaN.
func mClamp(a, lo, hi float32) float32 {
	switch {
	case a > hi:
		return hi
	case a >= lo:
		return a
	default:
		// a < lo or a is NaN.
		return lo
	}
}

// Port: the functions below give Go the semantics of the Rust operations on
// floating-point numbers that moxcms uses. They have no item in moxcms. Rust
// f32::max and f32::min return the other value when one value is NaN. A
// cast with "as" from a float to an integer saturates, and it gives 0 for
// NaN. Go leaves such a cast undefined, so the port never uses one on a value
// that is out of range.

// maxF32 is f32::max of Rust.
func maxF32(a, b float32) float32 {
	if isNaN32(a) {
		return b
	}
	if isNaN32(b) {
		return a
	}
	if a > b {
		return a
	}
	return b
}

// minF32 is f32::min of Rust.
func minF32(a, b float32) float32 {
	if isNaN32(a) {
		return b
	}
	if isNaN32(b) {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// maxF64 is f64::max of Rust.
func maxF64(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	if a > b {
		return a
	}
	return b
}

// minF64 is f64::min of Rust.
func minF64(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// isNaN32 reports whether v is NaN.
func isNaN32(v float32) bool {
	return math.IsNaN(float64(v))
}

// clampF32 is f32::clamp of Rust. It returns NaN when v is NaN.
func clampF32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// roundF32 is f32::round of Rust. It rounds half away from zero.
func roundF32(v float32) float32 {
	return float32(math.Round(float64(v)))
}

// floorF32 is f32::floor of Rust.
func floorF32(v float32) float32 {
	return float32(math.Floor(float64(v)))
}

// ceilF32 is f32::ceil of Rust.
func ceilF32(v float32) float32 {
	return float32(math.Ceil(float64(v)))
}

// absF32 is f32::abs of Rust.
func absF32(v float32) float32 {
	return float32(math.Abs(float64(v)))
}

// powF32 is f_powf of the crate pxfm. Port: the port computes the power in
// float64 and rounds it to float32. The last bit can differ from pxfm.
func powF32(x, y float32) float32 {
	return float32(math.Pow(float64(x), float64(y)))
}

// cbrtF32 is f_cbrtf of the crate pxfm.
func cbrtF32(x float32) float32 {
	return float32(math.Cbrt(float64(x)))
}

// floatToInt is the saturated cast "as" of Rust from a float to an integer,
// with the range of the integer.
func floatToInt(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v <= lo {
		return lo
	}
	if v >= hi {
		return hi
	}
	return math.Trunc(v)
}

// f32ToU8 is "v as u8" of Rust for an f32.
func f32ToU8(v float32) uint8 {
	return uint8(floatToInt(float64(v), 0, math.MaxUint8))
}

// f32ToU16 is "v as u16" of Rust for an f32.
func f32ToU16(v float32) uint16 {
	return uint16(floatToInt(float64(v), 0, math.MaxUint16))
}

// f32ToU32 is "v as u32" of Rust for an f32.
func f32ToU32(v float32) uint32 {
	return uint32(floatToInt(float64(v), 0, math.MaxUint32))
}

// f32ToI16 is "v as i16" of Rust for an f32.
func f32ToI16(v float32) int16 {
	return int16(floatToInt(float64(v), math.MinInt16, math.MaxInt16))
}

// f32ToI32 is "v as i32" of Rust for an f32.
func f32ToI32(v float32) int32 {
	return int32(floatToInt(float64(v), math.MinInt32, math.MaxInt32))
}

// f64ToU16 is "v as u16" of Rust for an f64.
func f64ToU16(v float64) uint16 {
	return uint16(floatToInt(v, 0, math.MaxUint16))
}

// f64ToI32 is "v as i32" of Rust for an f64.
func f64ToI32(v float64) int32 {
	return int32(floatToInt(v, math.MinInt32, math.MaxInt32))
}
