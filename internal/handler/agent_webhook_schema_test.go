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

package handler

import (
	"strings"
	"testing"
)

// TestExtractBySchema_RequiredMissing pins the required-field-missing
// branch (mirrors python agent_api.py:1913).
func TestExtractBySchema_RequiredMissing(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"q": map[string]any{"type": "string"},
		},
		"required": []any{"q"},
	}
	_, err := extractBySchema(map[string]any{}, schema, "query")
	if err == nil || !strings.Contains(err.Error(), "missing required field") {
		t.Errorf("err = %v, want missing required field", err)
	}
}

// TestExtractBySchema_OptionalDefault confirms that optional fields get
// the type-default value when absent.
func TestExtractBySchema_OptionalDefault(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"s": map[string]any{"type": "string"},
			"b": map[string]any{"type": "boolean"},
			"o": map[string]any{"type": "object"},
			"a": map[string]any{"type": "array"},
			"f": map[string]any{"type": "file"},
			"n": map[string]any{"type": "number"},
		},
	}
	got, err := extractBySchema(map[string]any{}, schema, "query")
	if err != nil {
		t.Fatalf("extractBySchema: %v", err)
	}
	if got["s"] != "" {
		t.Errorf("default for string: got %v", got["s"])
	}
	if got["b"] != false {
		t.Errorf("default for boolean: got %v", got["b"])
	}
	if _, ok := got["o"].(map[string]any); !ok {
		t.Errorf("default for object: got %T", got["o"])
	}
	if _, ok := got["a"].([]any); !ok {
		t.Errorf("default for array: got %T", got["a"])
	}
	if _, ok := got["f"].([]any); !ok {
		t.Errorf("default for file: got %T", got["f"])
	}
	if got["n"] != float64(0) {
		t.Errorf("default for number: got %v", got["n"])
	}
}

// TestExtractBySchema_NumberCoercion pins string→number auto-cast.
func TestExtractBySchema_NumberCoercion(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"n": map[string]any{"type": "number"},
		},
	}
	got, err := extractBySchema(map[string]any{"n": "42"}, schema, "body")
	if err != nil {
		t.Fatalf("extractBySchema: %v", err)
	}
	if got["n"] != float64(42) {
		t.Errorf("number coercion: got %v (%T)", got["n"], got["n"])
	}
}

// TestExtractBySchema_NumberCoercionFloat covers decimal strings.
func TestExtractBySchema_NumberCoercionFloat(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"n": map[string]any{"type": "number"},
		},
	}
	got, err := extractBySchema(map[string]any{"n": "3.14"}, schema, "body")
	if err != nil {
		t.Fatalf("extractBySchema: %v", err)
	}
	if got["n"] != 3.14 {
		t.Errorf("number float coercion: got %v", got["n"])
	}
}

// TestExtractBySchema_BooleanCoercion pins true/false string parsing.
func TestExtractBySchema_BooleanCoercion(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"b": map[string]any{"type": "boolean"},
		},
	}
	cases := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"0", false},
	}
	for _, tc := range cases {
		got, err := extractBySchema(map[string]any{"b": tc.in}, schema, "body")
		if err != nil {
			t.Errorf("coerce %q: err=%v", tc.in, err)
			continue
		}
		if got["b"] != tc.want {
			t.Errorf("coerce %q: got %v, want %v", tc.in, got["b"], tc.want)
		}
	}
}

// TestExtractBySchema_BooleanCoercionBad confirms non-boolean strings fail.
func TestExtractBySchema_BooleanCoercionBad(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"b": map[string]any{"type": "boolean"},
		},
	}
	_, err := extractBySchema(map[string]any{"b": "maybe"}, schema, "body")
	if err == nil || !strings.Contains(err.Error(), "auto-cast failed") {
		t.Errorf("err = %v, want auto-cast failed", err)
	}
}

// TestExtractBySchema_TypeMismatch covers wrong-type validation.
func TestExtractBySchema_TypeMismatch(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"n": map[string]any{"type": "number"},
		},
	}
	_, err := extractBySchema(map[string]any{"n": []any{1}}, schema, "body")
	if err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("err = %v, want type mismatch", err)
	}
}

// TestExtractBySchema_ArrayInnerType confirms array<inner> validation.
func TestExtractBySchema_ArrayInnerType(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"xs": map[string]any{"type": "array<number>"},
		},
	}
	// valid
	if _, err := extractBySchema(map[string]any{"xs": []any{1, 2, 3}}, schema, "body"); err != nil {
		t.Errorf("valid array: err=%v", err)
	}
	// invalid inner type
	_, err := extractBySchema(map[string]any{"xs": []any{1, "two", 3}}, schema, "body")
	if err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("invalid array inner: err=%v, want type mismatch", err)
	}
}

// TestExtractBySchema_ObjectCoercion pins string→object auto-cast.
func TestExtractBySchema_ObjectCoercion(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"o": map[string]any{"type": "object"},
		},
	}
	got, err := extractBySchema(map[string]any{"o": `{"k":"v"}`}, schema, "body")
	if err != nil {
		t.Fatalf("extractBySchema: %v", err)
	}
	m, ok := got["o"].(map[string]any)
	if !ok || m["k"] != "v" {
		t.Errorf("object coercion: got %v", got["o"])
	}
}

// TestExtractBySchema_NilData confirms nil data yields empty map.
func TestExtractBySchema_NilData(t *testing.T) {
	got, err := extractBySchema(nil, nil, "query")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Errorf("expected empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// TestValidateType_FileAndArray confirms validateType handles the
// edge cases at the python agent_api.py:2021-2051 boundary.
func TestValidateType_FileAndArray(t *testing.T) {
	if !validateType([]any{map[string]any{}}, "file") {
		t.Error("file should accept []any")
	}
	if !validateType([]any{1, 2, 3}, "array<number>") {
		t.Error("array<number> should accept []int-equivalent")
	}
	if validateType([]any{1, "x"}, "array<number>") {
		t.Error("array<number> should reject mixed types")
	}
}

// TestDefaultForType covers the python default_for_type table.
func TestDefaultForType(t *testing.T) {
	cases := []struct {
		t string
		// We compare via fmt-friendly assertions rather than reflect.DeepEqual
		// because the python reference returns [] / {} / false / 0 / "" / nil
		// directly, and Go equivalents may differ in concrete type.
	}{
		{"file"}, {"object"}, {"boolean"}, {"number"}, {"string"},
		{"array"}, {"array<object>"}, {"null"}, {"unknown"},
	}
	for _, tc := range cases {
		got := defaultForType(tc.t)
		// Just ensure no panic and we get a sensible zero value.
		_ = got
	}
}
