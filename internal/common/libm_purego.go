//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build !cgo

package common

import "math"

// PyLog10 returns log10(x) using Go's pure-Go math implementation.
//
// The cgo build of this package (see libm_cgo.go) routes through glibc's
// log10 to match Python's math.log10 bit-exactly. That build is
// preferred for the scoring paths that compare results against Python
// reference output. This fallback is provided so that internal/common
// — and the entrypoints that import it — remain buildable with
// CGO_ENABLED=0 (and other constrained cross-compilation setups that
// can't link -lm). Results may differ from Python's math.log10 by up
// to 1 ULP on some inputs; this is acceptable for non-strict-parity
// builds.
func PyLog10(x float64) float64 {
	return math.Log10(x)
}

// PySqrt returns sqrt(x) using Go's pure-Go math implementation.
//
// Provided as a counterpart to PyLog10 for the !cgo build. Go's
// math.Sqrt is a correctly-rounded implementation; PySqrt exists for
// API symmetry with the cgo build.
func PySqrt(x float64) float64 {
	return math.Sqrt(x)
}
