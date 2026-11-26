// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/lib.rs.

// Package moxcms is a port of a subset of the Rust crate moxcms, a color
// management system for ICC profiles.
//
// The subset is the part of moxcms that hayro uses (D16), and the part that
// the package icc uses (D61). It reads an ICC profile, makes the sRGB
// profile, a gray profile and the profile of CICP code points, and builds
// transforms of 8 bits from a profile to another profile. A transform is a
// TransformExecutor or an InPlaceTransformExecutor. SUBSET.md lists each item
// of the subset and its user in hayro.
//
// The port has the scalar code of moxcms only. It has no SIMD code.
//
// A profile is untrusted input. A malformed profile gives an error, and it
// never makes the package panic. Each error is a CmsError, and each CmsError
// wraps ivy.ErrFormat, ivy.ErrUnsupported or ivy.ErrLimit.
//
// Port: Rust never fuses a multiplication and an addition into one
// instruction. Go can fuse x*y + z on some processors, and then the last bit
// of a result can differ from moxcms. The functions of mlaf.go round each
// product, so that the transforms do not fuse.
package moxcms
