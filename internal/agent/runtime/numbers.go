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

package runtime

import (
	"encoding/json"
	"math/big"
)

// ExactNumberRatio represents a numeric value without rounding preserved JSON
// integers. Floating-point inputs retain the exact value they already represent.
// It does not accept numeric strings or change the original value.
func ExactNumberRatio(value any) *big.Rat {
	switch number := value.(type) {
	case json.Number:
		if !json.Valid([]byte(number)) {
			return nil
		}
		ratio, _ := new(big.Rat).SetString(number.String())
		return ratio
	case float64:
		return new(big.Rat).SetFloat64(number)
	case float32:
		return new(big.Rat).SetFloat64(float64(number))
	case int:
		return new(big.Rat).SetInt64(int64(number))
	case int32:
		return new(big.Rat).SetInt64(int64(number))
	case int64:
		return new(big.Rat).SetInt64(number)
	}
	return nil
}
