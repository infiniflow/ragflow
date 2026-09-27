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
	"strings"
	"testing"
)

func TestResolveVisionLanguage(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]any
		fallback string
		want     string
	}{
		{
			name:     "dataset language takes precedence",
			inputs:   map[string]any{"lang": " Japanese "},
			fallback: "Chinese",
			want:     "Japanese",
		},
		{
			name:     "configured fallback is used",
			inputs:   map[string]any{"lang": ""},
			fallback: " Korean ",
			want:     "Korean",
		},
		{
			name:     "empty values default to English",
			inputs:   nil,
			fallback: "",
			want:     defaultVisionLanguage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVisionLanguage(tt.inputs, tt.fallback); got != tt.want {
				t.Fatalf("resolveVisionLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderFigureVisionLanguage(t *testing.T) {
	prompt := renderFigureVisionLanguage("spaced={{ language }} compact={{language}}", "Japanese")
	if prompt != "spaced=Japanese compact=Japanese" {
		t.Fatalf("renderFigureVisionLanguage() = %q", prompt)
	}

	prompt = renderFigureVisionLanguage("language={{ language }}", "")
	if prompt != "language=English" {
		t.Fatalf("empty-language rendering = %q, want English fallback", prompt)
	}
}

func TestBuildFigureVisionPromptRendersLanguage(t *testing.T) {
	prompt, err := buildFigureVisionPrompt("Japanese")
	if err != nil {
		t.Fatalf("buildFigureVisionPrompt: %v", err)
	}

	if !strings.Contains(prompt, "Write all descriptions and field values in Japanese.") {
		t.Fatalf("prompt does not contain language directive, got: %q", prompt)
	}
	for _, unresolved := range []string{"{{ language }}", "{{language}}"} {
		if strings.Contains(prompt, unresolved) {
			t.Fatalf("prompt contains unresolved placeholder %q", unresolved)
		}
	}

	// Second call uses cache — verify language is re-rendered correctly per call.
	secondPrompt, err := buildFigureVisionPrompt("Chinese")
	if err != nil {
		t.Fatalf("buildFigureVisionPrompt with cached template: %v", err)
	}
	if !strings.Contains(secondPrompt, "Write all descriptions and field values in Chinese.") {
		t.Fatalf("cached prompt did not render the second request language")
	}
	if strings.Contains(secondPrompt, "in Japanese.") {
		t.Fatalf("cached prompt leaked the previous request language")
	}
}
