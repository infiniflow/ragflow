// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file has no source in moxcms. It holds the CMYK profile that ivy
// uses for CMYK without a profile, so that ivy keeps one copy of it (D61).

package moxcms

import (
	_ "embed"
)

// CmykProfile is the file assets/CGATS001Compat-v2-micro.icc, a compact
// profile of CGATS TR 001 (SWOP) from Compact-ICC-Profiles, under CC0 1.0
// Universal. hayro-interpret and hayro-jpeg2000 embed the same file, and
// their ports, the PDF color space DeviceCMYK and the package icc read it
// from here. A caller must not change it.
//
//go:embed assets/CGATS001Compat-v2-micro.icc
var CmykProfile []byte
