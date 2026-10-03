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

	"ragflow/internal/dao"
	"ragflow/internal/entity"
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

func TestResolveModelConfig_MaxTokensOverrideIsContextWindowOnly(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		ProviderID:   "provider-openai",
		InstanceName: "default",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := db.Model(&entity.TenantModel{}).
		Where("id = ?", "0123456789abcdef0123456789abcdef").
		Update("extra", `{"max_tokens": 2000}`).Error; err != nil {
		t.Fatalf("set model extra: %v", err)
	}

	providerModel, err := dao.GetModelProviderManager().GetModelByName("OpenAI", "gpt-4o")
	if err != nil {
		t.Fatalf("resolve provider model: %v", err)
	}
	if providerModel.MaxOutput == nil {
		t.Fatal("provider model has no max_output")
	}
	wantMaxOutput := *providerModel.MaxOutput

	cases := []struct {
		name    string
		resolve func() (int, error)
	}{
		{
			name: "tenant model id",
			resolve: func() (int, error) {
				_, _, _, maxOutput, err := resolveModelConfigByID(
					t.Context(), db, "tenant-1", entity.ModelTypeChat,
					"0123456789abcdef0123456789abcdef",
				)
				return maxOutput, err
			},
		},
		{
			name: "composite reference",
			resolve: func() (int, error) {
				_, _, _, maxOutput, err := resolveModelConfigFromProviderInstance(
					t.Context(), db, "tenant-1", entity.ModelTypeChat, "gpt-4o@default@OpenAI",
				)
				return maxOutput, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.resolve()
			if err != nil {
				t.Fatalf("resolve model config: %v", err)
			}
			if got != wantMaxOutput {
				t.Fatalf("max output = %d, want catalog max_output %d", got, wantMaxOutput)
			}
		})
	}

	if got := dao.ResolveModelContentLength(
		t.Context(), db, "tenant-1", "0123456789abcdef0123456789abcdef", "", "",
	); got != 2000 {
		t.Fatalf("context length = %d, want tenant max_tokens override 2000", got)
	}
}
