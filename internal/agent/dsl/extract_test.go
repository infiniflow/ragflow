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

package dsl

import (
	"errors"
	"testing"
)

// happyDSL returns a 2-component dsl suitable for the happy-path
// extractors.
func happyDSL() map[string]any {
	return map[string]any{
		"components": map[string]any{
			"begin": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params": map[string]any{
						"mode": "Manual",
					},
					"input_form": map[string]any{
						"query": map[string]any{
							"type": "string",
						},
					},
				},
			},
			"answer": map[string]any{
				"obj": map[string]any{
					"component_name": "Answer",
				},
			},
		},
	}
}

func TestExtractComponentInputForm_HappyPath(t *testing.T) {
	got, err := ExtractComponentInputForm(happyDSL(), "begin")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if q, ok := got["query"].(map[string]any); !ok || q["type"] != "string" {
		t.Errorf("query type = %v, want string", got["query"])
	}
}

func TestExtractComponentInputForm_NotFound(t *testing.T) {
	_, err := ExtractComponentInputForm(happyDSL(), "missing")
	if !errors.Is(err, ErrComponentNotFound) {
		t.Errorf("err = %v, want ErrComponentNotFound", err)
	}
}

func TestExtractComponentInputForm_MissingObj(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"bare": map[string]any{}, // no obj
		},
	}
	_, err := ExtractComponentInputForm(dsl, "bare")
	if !errors.Is(err, ErrMalformedDSL) {
		t.Errorf("err = %v, want ErrMalformedDSL", err)
	}
}

func TestExtractComponentInputForm_MissingInputForm(t *testing.T) {
	// "answer" has obj but no input_form.
	_, err := ExtractComponentInputForm(happyDSL(), "answer")
	if !errors.Is(err, ErrMissingInputForm) {
		t.Errorf("err = %v, want ErrMissingInputForm", err)
	}
}

func TestExtractComponentInputForm_NilDSL(t *testing.T) {
	_, err := ExtractComponentInputForm(nil, "anything")
	if !errors.Is(err, ErrMalformedDSL) {
		t.Errorf("err = %v, want ErrMalformedDSL", err)
	}
}

func TestExtractComponentParams_HappyPath(t *testing.T) {
	got, err := ExtractComponentParams(happyDSL(), "begin")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got["mode"] != "Manual" {
		t.Errorf("mode = %v, want Manual", got["mode"])
	}
}

func TestExtractComponentParams_NoParams(t *testing.T) {
	got, err := ExtractComponentParams(happyDSL(), "answer")
	if err != nil {
		t.Fatalf("err = %v, want nil (params is optional)", err)
	}
	if got != nil && len(got) != 0 {
		t.Errorf("params = %v, want empty/nil", got)
	}
}

// TestExtractComponentParams_WrongType pins that a present-but-
// wrongly-typed params field is ErrMalformedDSL. CodeRabbit PR #1.
func TestExtractComponentParams_WrongType(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"bad": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         "this is a string, not a dict",
				},
			},
		},
	}
	_, err := ExtractComponentParams(dsl, "bad")
	if !errors.Is(err, ErrMalformedDSL) {
		t.Errorf("err = %v, want ErrMalformedDSL", err)
	}
}

func TestExtractComponentName_HappyPath(t *testing.T) {
	got, err := ExtractComponentName(happyDSL(), "begin")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "Begin" {
		t.Errorf("name = %q, want Begin", got)
	}
}

func TestExtractComponentName_NotFound(t *testing.T) {
	_, err := ExtractComponentName(happyDSL(), "missing")
	if !errors.Is(err, ErrComponentNotFound) {
		t.Errorf("err = %v, want ErrComponentNotFound", err)
	}
}

// TestFindBeginComponentID_HappyPath covers the common case where the
// component ID is literally "begin" (mirrors the
// internal/agent/dsl/testdata fixtures).
func TestFindBeginComponentID_HappyPath(t *testing.T) {
	id, err := FindBeginComponentID(happyDSL())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if id != "begin" {
		t.Errorf("id = %q, want begin", id)
	}
}

// TestFindBeginComponentID_DifferentID ensures the helper resolves
// the name to whatever ID the canvas uses (mirrors real-world
// canvases where IDs are sally:0 / jack:0 etc.).
func TestFindBeginComponentID_DifferentID(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"sally:0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
				},
			},
			"jack:0": map[string]any{
				"obj": map[string]any{
					"component_name": "LLM",
				},
			},
		},
	}
	id, err := FindBeginComponentID(dsl)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if id != "sally:0" {
		t.Errorf("id = %q, want sally:0", id)
	}
}

// TestFindBeginComponentID_NotFound pins that a canvas with no begin
// component returns ErrComponentNotFound. The service layer maps this
// to an empty fallback (degrades gracefully, no panic).
func TestFindBeginComponentID_NotFound(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"jack:0": map[string]any{
				"obj": map[string]any{
					"component_name": "LLM",
				},
			},
		},
	}
	_, err := FindBeginComponentID(dsl)
	if !errors.Is(err, ErrComponentNotFound) {
		t.Errorf("err = %v, want ErrComponentNotFound", err)
	}
}

// TestFindBeginComponentID_NilDSL pins that a nil dsl returns
// ErrMalformedDSL (not a nil-deref panic).
func TestFindBeginComponentID_NilDSL(t *testing.T) {
	_, err := FindBeginComponentID(nil)
	if !errors.Is(err, ErrMalformedDSL) {
		t.Errorf("err = %v, want ErrMalformedDSL", err)
	}
}

// TestExtractPrologue_HappyPath pins the prologue lookup path.
func TestExtractPrologue_HappyPath(t *testing.T) {
	dsl := happyDSL()
	dsl["components"].(map[string]any)["begin"].(map[string]any)["obj"].(map[string]any)["prologue"] = "hello"
	got, err := ExtractPrologue(dsl)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "hello" {
		t.Errorf("prologue = %q, want hello", got)
	}
}

// TestExtractPrologue_NotFound pins that a missing begin component
// returns ErrComponentNotFound (the service layer turns this into
// empty-string fallback).
func TestExtractPrologue_NotFound(t *testing.T) {
	_, err := ExtractPrologue(map[string]any{
		"components": map[string]any{},
	})
	if !errors.Is(err, ErrComponentNotFound) {
		t.Errorf("err = %v, want ErrComponentNotFound", err)
	}
}

// TestExtractMode_HappyPath pins the mode lookup path.
func TestExtractMode_HappyPath(t *testing.T) {
	dsl := happyDSL()
	dsl["components"].(map[string]any)["begin"].(map[string]any)["obj"].(map[string]any)["mode"] = "Agent"
	got, err := ExtractMode(dsl)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "Agent" {
		t.Errorf("mode = %q, want Agent", got)
	}
}

// TestExtractMode_NotFound pins that a missing begin component
// returns ErrComponentNotFound.
func TestExtractMode_NotFound(t *testing.T) {
	_, err := ExtractMode(map[string]any{
		"components": map[string]any{},
	})
	if !errors.Is(err, ErrComponentNotFound) {
		t.Errorf("err = %v, want ErrComponentNotFound", err)
	}
}
