// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/mlaf.rs.

package moxcms

// mlaf is mlaf in moxcms src/mlaf.rs, for f32. It returns acc + a*b.
//
// Port: upstream uses a fused multiply-add when the target has one. The port
// uses the scalar path of upstream, which rounds the product before the
// addition. The conversion to float32 keeps Go from a fused operation.
func mlaf(acc, a, b float32) float32 {
	return acc + float32(a*b)
}

// mlafF64 is mlaf in moxcms src/mlaf.rs, for f64. It returns acc + a*b.
func mlafF64(acc, a, b float64) float64 {
	return acc + float64(a*b)
}

// negMlaf is neg_mlaf in moxcms src/mlaf.rs, for f32. It returns acc - a*b.
func negMlaf(acc, a, b float32) float32 {
	return mlaf(acc, a, -b)
}

// fmla is fmla in moxcms src/mlaf.rs, for f32. It returns a*b + acc.
func fmla(a, b, acc float32) float32 {
	return mlaf(acc, a, b)
}

// fmlaF64 is fmla in moxcms src/mlaf.rs, for f64. It returns a*b + acc.
func fmlaF64(a, b, acc float64) float64 {
	return mlafF64(acc, a, b)
}
