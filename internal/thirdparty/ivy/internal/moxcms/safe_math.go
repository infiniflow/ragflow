// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/safe_math.rs.

package moxcms

import "math"

// Port: upstream declares the traits SafeAdd, SafeMul and SafePowi for
// several integer types. The port needs them for usize and u32 only. Go has
// no usize, so the port uses uint64 for it, and Go keeps a value of an
// integer on 64 bits.

// safeAdd is SafeAdd::safe_add in moxcms src/safe_math.rs, for usize.
func safeAdd(a, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, cmsError(OverflowingError)
	}
	return a + b, nil
}

// safeMul is SafeMul::safe_mul in moxcms src/safe_math.rs, for usize.
func safeMul(a, b uint64) (uint64, error) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, cmsError(OverflowingError)
	}
	return a * b, nil
}

// safeMulU32 is SafeMul::safe_mul in moxcms src/safe_math.rs, for u32.
func safeMulU32(a, b uint32) (uint32, error) {
	if a != 0 && b > math.MaxUint32/a {
		return 0, cmsError(OverflowingError)
	}
	return a * b, nil
}

// safeMulU16 is SafeMul::safe_mul in moxcms src/safe_math.rs, for u16.
func safeMulU16(a, b uint16) (uint16, error) {
	if a != 0 && b > math.MaxUint16/a {
		return 0, cmsError(OverflowingError)
	}
	return a * b, nil
}

// safePowi is SafePowi::safe_powi in moxcms src/safe_math.rs, for usize.
func safePowi(base uint64, power uint32) (uint64, error) {
	r := uint64(1)
	for range power {
		var err error
		if r, err = safeMul(r, base); err != nil {
			return 0, err
		}
	}
	return r, nil
}

// safePowiU32 is SafePowi::safe_powi in moxcms src/safe_math.rs, for u32.
func safePowiU32(base, power uint32) (uint32, error) {
	r := uint32(1)
	for range power {
		var err error
		if r, err = safeMulU32(r, base); err != nil {
			return 0, err
		}
	}
	return r, nil
}
