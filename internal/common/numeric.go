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

// CoalesceInt returns *val if val is non-nil and positive; otherwise returns
// defaultVal. It is useful for optional int parameters (e.g. pagination)
// where nil or a value <= 0 means "use the default".
func CoalesceInt(val *int, defaultVal int) int {
	if val != nil && *val > 0 {
		return *val
	}
	return defaultVal
}

// IsZeroVector reports whether every element of v is zero. An empty or nil
// slice is considered a zero vector.
func IsZeroVector(v []float64) bool {
	for _, x := range v {
		if x != 0 {
			return false
		}
	}
	return true
}
