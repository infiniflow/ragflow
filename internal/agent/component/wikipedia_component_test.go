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
	"context"
	"testing"
)

func TestWikipedia_RegisteredFactory(t *testing.T) {
	c, err := New("Wikipedia", map[string]any{"top_n": float64(3), "language": "en"})
	if err != nil {
		t.Fatalf("New(Wikipedia) errored: %v", err)
	}
	if got := c.Name(); got != "Wikipedia" {
		t.Fatalf("Name() = %q, want Wikipedia", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("Wikipedia component does not expose GetInputForm")
	}
	query, ok := formGetter.GetInputForm()["query"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[query] has type %T, want map", formGetter.GetInputForm()["query"])
	}
	if query["type"] != "line" {
		t.Fatalf("GetInputForm()[query][type] = %v, want line", query["type"])
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestWikipedia_InvokeEmptyQueryMatchesPython(t *testing.T) {
	c, err := New("Wikipedia", nil)
	if err != nil {
		t.Fatalf("New(Wikipedia) errored: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{"query": "  "})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got, _ := out["formalized_content"].(string); got != "" {
		t.Fatalf("formalized_content = %q, want empty", got)
	}
	if _, ok := out["json"].([]any); !ok {
		t.Fatalf("json output has type %T, want []any", out["json"])
	}
}
