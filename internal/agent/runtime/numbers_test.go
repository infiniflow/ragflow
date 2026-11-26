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
	"math"
	"testing"
)

func TestExactNumberRatio(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input any
		ratio string
	}{
		{"preserved unsigned ID", json.Number("11099848653006015581"), "11099848653006015581"},
		{"exact native integer", int64(9007199254740993), "9007199254740993"},
		{"float represented integer", float64(9007199254740992), "9007199254740992"},
		{"float32", float32(0.5), "1/2"},
		{"int32", int32(-2), "-2"},
		{"int", int(2), "2"},
		{"fraction", json.Number("-1.5"), "-3/2"},
		{"exponent", json.Number("1e3"), "1000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ExactNumberRatio(tc.input)
			if got == nil || got.RatString() != tc.ratio {
				t.Fatalf("ratio=%v, want %s", got, tc.ratio)
			}
		})
	}
	for _, value := range []any{"2", true, nil, json.Number("bad"), json.Number("1/2"), math.NaN(), math.Inf(1)} {
		if got := ExactNumberRatio(value); got != nil {
			t.Errorf("accepted %v (%T): %s", value, value, got)
		}
	}
}
