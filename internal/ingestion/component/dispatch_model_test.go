//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestConfiguredMediaModelID(t *testing.T) {
	tests := []struct {
		name   string
		family string
		setup  schema.ParserSetup
		want   string
	}{
		{
			name:   "image parse_method is the VLM model ref",
			family: "image",
			setup:  schema.ParserSetup{"parse_method": "gpt-4-vision@openai"},
			want:   "gpt-4-vision@openai",
		},
		{
			name:   "image ocr falls back to vlm.llm_id",
			family: "image",
			setup: schema.ParserSetup{
				"parse_method": "ocr",
				"vlm":          map[string]any{"llm_id": "gpt-4-vision@openai"},
			},
			want: "gpt-4-vision@openai",
		},
		{
			name:   "image ocr falls back to top-level llm_id",
			family: "image",
			setup:  schema.ParserSetup{"parse_method": "ocr", "llm_id": "qwen-vl@dashscope"},
			want:   "qwen-vl@dashscope",
		},
		{
			name:   "pdf uses vlm.llm_id",
			family: "pdf",
			setup: schema.ParserSetup{
				"parse_method": "deepdoc",
				"vlm":          map[string]any{"llm_id": "custom-vlm@provider"},
			},
			want: "custom-vlm@provider",
		},
		{
			name:   "audio uses vlm.llm_id",
			family: "audio",
			setup:  schema.ParserSetup{"vlm": map[string]any{"llm_id": "whisper@openai"}},
			want:   "whisper@openai",
		},
		{
			name:   "empty setup",
			family: "pdf",
			setup:  schema.ParserSetup{},
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configuredMediaModelID(tc.setup, tc.family); got != tc.want {
				t.Fatalf("configuredMediaModelID() = %q, want %q", got, tc.want)
			}
		})
	}
}
