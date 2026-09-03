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

//go:build cgo

package common

/*
#cgo LDFLAGS: -lm
#include <math.h>
*/
import "C"

// PyLog10 calls the C library's log10 (matching Python's math.log10).
//
// Go's pure-Go math.Log10 can differ from glibc's log10 by 1 ULP on
// some inputs (e.g. log10(0.1) returns 0xbfefffffffffffff in Go vs
// 0xbff0000000000000 in glibc), which breaks bit-exact parity tests
// against Python scoring code. PyLog10 routes through cgo so the
// result matches Python's math.log10 exactly.
func PyLog10(x float64) float64 {
	return float64(C.log10(C.double(x)))
}

// PySqrt calls the C library's sqrt (matching Python's math.sqrt).
//
// Go's math.Sqrt is a correctly-rounded pure-Go implementation, but
// PySqrt exists for symmetry with PyLog10 and as a defensive guarantee
// against the rare inputs where Go's implementation diverges from
// glibc's. The cgo overhead is negligible on the scoring paths that
// use it.
func PySqrt(x float64) float64 {
	return float64(C.sqrt(C.double(x)))
}
