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
		if _, ok2 := form["query"]; !ok2 {
			t.Fatalf("GetInputForm missing both q and query: %+v", form)
		}
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestGoogleComponent_MissingAPIKeyReturnsError(t *testing.T) {
	c, err := New("Google", map[string]any{})
	if err != nil {
		t.Fatalf("New(Google): %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{"q": "ragflow"})
	if err != nil {
		if !strings.Contains(err.Error(), "api_key") {
			t.Fatalf("error = %v, want api_key error", err)
		}
		return
	}
	// Component may embed error in output map instead of returning Go error.
	if errStr, ok := out["_ERROR"].(string); ok && strings.Contains(errStr, "api_key") {
		return
	}
	t.Fatalf("expected api_key error, got out=%+v err=%v", out, err)
}
