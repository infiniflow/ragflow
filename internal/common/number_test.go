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

import (
	"encoding/json"
	"math"
	"testing"
)

// The expected strings are what Python's json.dumps prints for the same float.
func TestPyFloat64MarshalJSONMatchesPythonJSONDumps(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{1, "1.0"},
		{-1, "-1.0"},
		{100, "100.0"},
		{123456, "123456.0"},
		{1e6, "1000000.0"},
		{1763800000, "1763800000.0"},
		{12345678.9, "12345678.9"},
		{9999999999999998, "9999999999999998.0"},
		{0.5, "0.5"},
		{0.123456789, "0.123456789"},
		{0.0001, "0.0001"},
		{1e-05, "1e-05"},
		{1.5e-07, "1.5e-07"},
		{1e16, "1e+16"},
		{1.5e16, "1.5e+16"},
		{-2.5e20, "-2.5e+20"},
		{1e100, "1e+100"},
	}
	for _, c := range cases {
		got, err := PyFloat64(c.in).MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("MarshalJSON(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

// NaN and infinities have no JSON form; MarshalJSON keeps returning the same
// text as before so json.Marshal still reports them as an error.
func TestPyFloat64MarshalJSONNonFinite(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
	}
	for _, c := range cases {
		got, err := PyFloat64(c.in).MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("MarshalJSON(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestPyFloat64InConvertedChunk(t *testing.T) {
	chunk := map[string]interface{}{
		"similarity":       1.0,
		"create_timestamp": 1763800000.0,
		"vector":           []float64{0, 0.25},
	}
	got, err := json.Marshal(ConvertFloatsToPyFormat(chunk))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"create_timestamp":1763800000.0,"similarity":1.0,"vector":[0.0,0.25]}`
	if string(got) != want {
		t.Fatalf("json.Marshal = %s, want %s", got, want)
	}
}
