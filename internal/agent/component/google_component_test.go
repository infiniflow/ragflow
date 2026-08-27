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
	"strings"
	"testing"
)

func TestGoogleComponent_RegisteredAndInputForm(t *testing.T) {
	c, err := New("Google", map[string]any{
		"api_key":  "",
		"country":  "us",
		"language": "en",
	})
	if err != nil {
		t.Fatalf("New(Google): %v", err)
	}
	if got := c.Name(); got != "Google" {
		t.Fatalf("Name() = %q, want Google", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("Google component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	if _, ok := form["q"]; !ok {
		t.Fatalf("GetInputForm missing q: %+v", form)
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestGoogleComponent_MissingAPIKeyMatchesToolError(t *testing.T) {
	c, err := New("Google", map[string]any{})
	if err != nil {
		t.Fatalf("New(Google): %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{"q": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if got, _ := out["_ERROR"].(string); !strings.Contains(got, "api_key") {
		t.Fatalf("_ERROR = %q, want api_key error (out=%+v)", got, out)
	}
	if got, ok := out["formalized_content"].(string); !ok || got != "" {
		t.Fatalf("formalized_content = %#v, want empty string", out["formalized_content"])
	}
	if got := anySlice(out["json"]); len(got) != 0 {
		t.Fatalf("json len = %d, want 0", len(got))
	}
}
