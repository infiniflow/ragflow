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

package component

import (
	"encoding/json"
	"testing"
)

// TestFloatFromKeepsNarrowContract pins the pre-convergence contract: a native
// float64/float32/int/int64 is a valid param value, everything else (notably
// strings and json.Number) is ignored. common.ToFloat accepts strings, so a
// naive delegation would widen this contract.
func TestFloatFromKeepsNarrowContract(t *testing.T) {
	accepted := map[string]any{
		"float64": 0.7,
		"float32": float32(0.5),
		"int":     1,
		"int64":   int64(2),
	}
	want := map[string]float64{"float64": 0.7, "float32": 0.5, "int": 1, "int64": 2}
	for name, v := range accepted {
		got, ok := floatFrom(map[string]any{"v": v}, "v")
		if !ok || got != want[name] {
			t.Fatalf("floatFrom(%s) = %v,%v, want %v,true", name, got, ok, want[name])
		}
	}

	for name, v := range map[string]any{
		"numeric string": "0.7",
		"NaN string":     "NaN",
		"json.Number":    json.Number("0.7"),
		"bool":           true,
		"int32":          int32(1),
	} {
		if got, ok := floatFrom(map[string]any{"v": v}, "v"); ok {
			t.Fatalf("floatFrom(%s) = %v,true, want rejected", name, got)
		}
	}

	if got, ok := floatFrom(map[string]any{}, "missing"); ok || got != 0 {
		t.Fatalf("floatFrom(missing) = %v,%v, want 0,false", got, ok)
	}
}

func TestIntFromKeepsNarrowContract(t *testing.T) {
	accepted := map[string]any{"int": 1, "int64": int64(2), "float64": 3.9}
	want := map[string]int{"int": 1, "int64": 2, "float64": 3}
	for name, v := range accepted {
		got, ok := intFrom(map[string]any{"v": v}, "v")
		if !ok || got != want[name] {
			t.Fatalf("intFrom(%s) = %v,%v, want %v,true", name, got, ok, want[name])
		}
	}

	for name, v := range map[string]any{
		"numeric string": "7",
		"json.Number":    json.Number("7"),
		"bool":           true,
		"int32":          int32(1),
		"float32":        float32(1),
	} {
		if got, ok := intFrom(map[string]any{"v": v}, "v"); ok {
			t.Fatalf("intFrom(%s) = %v,true, want rejected", name, got)
		}
	}

	if got, ok := intFrom(map[string]any{}, "missing"); ok || got != 0 {
		t.Fatalf("intFrom(missing) = %v,%v, want 0,false", got, ok)
	}
}
