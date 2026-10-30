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

package service

import (
	"ragflow/internal/entity"
	"reflect"
	"testing"
)

func TestChatConfigurationRejectsInvalidTypes(t *testing.T) {
	for _, operation := range []string{"create", "patch"} {
		for _, payload := range []map[string]any{
			{"llm_setting": "invalid"},
			{"llm_setting": map[string]any{"temperature": "bad"}},
			{"llm_setting": map[string]any{"top_p": true}},
			{"llm_setting": map[string]any{"presence_penalty": nil}},
			{"llm_setting": map[string]any{"frequency_penalty": []any{}}},
			{"llm_setting": map[string]any{"max_tokens": "1024"}},
			{"prompt_config": map[string]any{"empty_response": 123}},
			{"prompt_config": map[string]any{"prologue": true}},
			{"prompt_config": map[string]any{"system": nil}},
			{"prompt_config": map[string]any{"parameters": "invalid"}},
			{"prompt_config": map[string]any{"quote": "invalid"}},
		} {
			t.Run(operation, func(t *testing.T) {
				db := setupChatRESTUpdateServiceTestDB(t)
				svc := NewChatService()
				ctx := serviceTestContext(t, "user-1", "user-1")
				var err error
				if operation == "create" {
					payload["name"] = "validation"
					_, _, err = svc.Create(ctx, "user-1", payload)
				} else {
					createChatRESTUpdateServiceTestChat(t, db, "chat", "user-1")
					_, err = svc.PatchChat(ctx, "user-1", "chat", payload)
				}
				if err == nil {
					t.Fatalf("accepted invalid configuration: %#v", payload)
				}
			})
		}
	}
}

func TestMergeChatConfigurationPreservesNestedValues(t *testing.T) {
	base := entity.JSONMap{"reference_metadata": map[string]any{"include": false, "fields": []any{"source"}}, "system": "preserved"}
	merged := mergeJSONMap(base, map[string]any{"reference_metadata": map[string]any{"include": true}})
	want := map[string]any{"include": true, "fields": []any{"source"}}
	if !reflect.DeepEqual(merged["reference_metadata"], entity.JSONMap(want)) && !reflect.DeepEqual(merged["reference_metadata"], want) {
		t.Fatalf("nested configuration=%#v", merged)
	}
	if merged["system"] != "preserved" || base["reference_metadata"].(map[string]any)["include"] != false {
		t.Fatalf("merge lost unrelated data or mutated base")
	}
}
