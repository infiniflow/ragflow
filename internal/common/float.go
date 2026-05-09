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

const epsilon32 = 1e-6
const epsilon64 = 1e-9

func Float64IsZero(f float64) bool {
	if f < 0 && f >= -epsilon64 {
		return true
	}
	if f > 0 && f <= epsilon64 {
		return true
	}
	return false
}

func Float32IsNotZero(f float32) bool {
	if f < 0 && f >= -epsilon32 {
		return true
	}
	if f > 0 && f <= epsilon32 {
		return true
	}
	return false
}
