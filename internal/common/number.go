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

package common

import "strconv"

// PyFloat64 is a float64 that serializes to JSON using the same format as Python's json.dumps.
// Python uses the "shortest unique representation" algorithm (dtoa) for float64,
// which is equivalent to Go's strconv.FormatFloat with 'g' and precision -1.
// This ensures deterministic and Python-compatible float serialization.
type PyFloat64 float64

// MarshalJSON implements the json.Marshaler interface for PyFloat64.
// Uses strconv.FormatFloat with 'g' format and -1 precision to produce
// the shortest decimal representation that uniquely identifies the float64,
// matching Python's json.dumps behavior.
func (f PyFloat64) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(f), 'g', -1, 64)), nil
}

// ConvertFloatsToPyFormat recursively converts all float64 values in nested
// map[string]interface{} and []interface{} structures to PyFloat64, ensuring
// Python-compatible JSON serialization. Typed float slices ([]float64,
// []float32, and their nested variants) are also handled so common vector
// payload shapes don't fall through to Go's default float formatting.
func ConvertFloatsToPyFormat(v interface{}) interface{} {
	switch val := v.(type) {
	case float64:
		return PyFloat64(val)
	case float32:
		return PyFloat64(val)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			result[k] = ConvertFloatsToPyFormat(v2)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = ConvertFloatsToPyFormat(item)
		}
		return result
	case []map[string]interface{}:
		result := make([]map[string]interface{}, len(val))
		for i, item := range val {
			result[i] = ConvertFloatsToPyFormat(item).(map[string]interface{})
		}
		return result
	case []float64:
		result := make([]PyFloat64, len(val))
		for i, f := range val {
			result[i] = PyFloat64(f)
		}
		return result
	case []float32:
		result := make([]PyFloat64, len(val))
		for i, f := range val {
			result[i] = PyFloat64(f)
		}
		return result
	case [][]float64:
		result := make([][]PyFloat64, len(val))
		for i, inner := range val {
			converted := ConvertFloatsToPyFormat(inner).([]PyFloat64)
			result[i] = converted
		}
		return result
	case [][]float32:
		result := make([][]PyFloat64, len(val))
		for i, inner := range val {
			converted := ConvertFloatsToPyFormat(inner).([]PyFloat64)
			result[i] = converted
		}
		return result
	default:
		return v
	}
}

// PairwiseSum returns the sum of xs computed via pairwise (cascade) summation,
// matching the error behavior of numpy.sum(): O(log n * eps) instead of
// the O(n * eps) of a naive left-to-right loop.
//
// This implementation matches numpy's exact pairwise summation algorithm:
// - For n < 16: uses naive left-to-right sum (matching numpy's small-array optimization)
// - For n >= 16: processes pairs left-to-right, carrying any odd element to the end
//   of the next level. This matches numpy's pairwise reduction in
//   numpy/core/src/umath/reduction.c.
//
// xs is modified in place. Pass a copy if the caller still needs the input.
//
// Empty input returns 0; single-element input returns xs[0].
func PairwiseSum(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}

	// For small arrays (n < 16), numpy uses naive left-to-right sum.
	// This is critical for matching Python's exact float64 results.
	// Empirically verified: numpy's np.sum() uses naive sum for n < 16.
	if n < 16 {
		sum := 0.0
		for _, x := range xs {
			sum += x
		}
		return sum
	}

	// Pairwise summation matching numpy's algorithm:
	// Process pairs left-to-right, carry odd element to the end.
	for n > 1 {
		m := n / 2
		for i := 0; i < m; i++ {
			xs[i] = xs[2*i] + xs[2*i+1]
		}
		// If odd length, carry the last element to position m
		if n%2 != 0 {
			xs[m] = xs[n-1]
			n = m + 1
		} else {
			n = m
		}
	}
	return xs[0]
}

func GetInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
