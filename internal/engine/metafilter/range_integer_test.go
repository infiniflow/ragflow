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

package metafilter

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Exercise the public query builder and the JSON encoding used by both search
// engines, so a numerically adjacent range boundary cannot pass unnoticed.
func TestBuildMetaFilterQueryPreservesIntegerRangeBounds(t *testing.T) {
	bounds := []struct {
		name, literal string
		integer       int64
	}{
		{"positive_above_float_precision", "9007199254740993", 9007199254740993},
		{"negative_above_float_precision", "-9007199254740993", -9007199254740993},
		{"maximum_int64", "9223372036854775807", 9223372036854775807},
		{"minimum_int64", "-9223372036854775808", -9223372036854775808},
	}
	for _, bound := range bounds {
		values := []struct {
			name  string
			value interface{}
		}{
			{"string", bound.literal}, {"int64", bound.integer}, {"json_number", json.Number(bound.literal)},
		}
		for _, input := range values {
			for _, op := range []struct{ input, dsl string }{{">", "gt"}, {"<", "lt"}, {"≥", "gte"}, {"≤", "lte"}} {
				t.Run(bound.name+"/"+input.name+"/"+op.dsl, func(t *testing.T) {
					assertEncodedRangeBound(t, op.input, op.dsl, input.value, bound.literal)
				})
			}
		}
	}
}

func TestBuildMetaFilterQueryPreservesNonIntegerRangeCoercion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    interface{}
		wantJSON string
	}{
		{"fraction", "1.25", "1.25"},
		{"scientific_notation", "1e3", "1000"},
		{"decimal_integer", "1.0", "1"},
		{"date", "2026-11-24", `"2026-11-24"`},
		{"text_case", "UpperBound", `"UpperBound"`},
		{"already_rounded_float", float64(9007199254740993), "9007199254740992"},
		{"outside_int64", "9223372036854775808", "9223372036854776000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertEncodedRangeBound(t, ">", "gt", tc.input, tc.wantJSON)
		})
	}
}

func assertEncodedRangeBound(t *testing.T, op, dsl string, input interface{}, wantJSON string) {
	t.Helper()
	query, err := BuildMetaFilterQuery([]map[string]interface{}{{"key": "sequence", "op": op, "value": input}}, "and", []string{"kb-1"})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(query); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&body)
	decoder.UseNumber()
	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	bound, ok := findEncodedRangeBound(decoded, dsl)
	if !ok {
		t.Fatalf("encoded query has no %s range boundary", dsl)
	}
	encodedBound, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedBound) != wantJSON {
		t.Fatalf("encoded range boundary = %s, want %s", encodedBound, wantJSON)
	}
}

func findEncodedRangeBound(value interface{}, dsl string) (interface{}, bool) {
	switch node := value.(type) {
	case map[string]interface{}:
		if ranges, ok := node["range"].(map[string]interface{}); ok {
			if bounds, ok := ranges["meta_fields.sequence"].(map[string]interface{}); ok {
				bound, exists := bounds[dsl]
				return bound, exists
			}
		}
		for _, child := range node {
			if bound, ok := findEncodedRangeBound(child, dsl); ok {
				return bound, true
			}
		}
	case []interface{}:
		for _, child := range node {
			if bound, ok := findEncodedRangeBound(child, dsl); ok {
				return bound, true
			}
		}
	}
	return nil, false
}
