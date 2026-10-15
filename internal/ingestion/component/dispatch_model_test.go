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

func TestConfiguredAudioModelID(t *testing.T) {
	tests := []struct {
		name  string
		setup schema.ParserSetup
		want  string
	}{
		{
			name:  "audio uses vlm.llm_id",
			setup: schema.ParserSetup{"vlm": map[string]any{"llm_id": "whisper@openai"}},
			want:  "whisper@openai",
		},
		{
			name:  "empty setup",
			setup: schema.ParserSetup{},
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configuredAudioModelID(tc.setup); got != tc.want {
				t.Fatalf("configuredAudioModelID() = %q, want %q", got, tc.want)
			}
		})
	}
}
