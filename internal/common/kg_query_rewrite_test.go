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

package common

import (
	"testing"
)

func TestBuildQueryRewritePrompt(t *testing.T) {
	question := "What is the capital of France?"
	ty2ents := `{"LOCATION": ["Paris", "London"]}`
	prompt := BuildQueryRewritePrompt(question, ty2ents)
	if !contains(prompt, question) {
		t.Error("expected question in prompt")
	}
	if !contains(prompt, ty2ents) {
		t.Error("expected type pool in prompt")
	}
	if contains(prompt, "{query}") {
		t.Error("placeholder {query} should have been replaced")
	}
	if contains(prompt, "{TYPE_POOL}") {
		t.Error("placeholder {TYPE_POOL} should have been replaced")
	}
}

func TestParseQueryRewriteResponse_ValidJSON(t *testing.T) {
	resp := `{
		"answer_type_keywords": ["LOCATION", "ORGANIZATION"],
		"entities_from_query": ["France", "Paris", "Capital"]
	}`
	result, err := ParseQueryRewriteResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.TypeKeywords) != 2 || result.TypeKeywords[0] != "LOCATION" {
		t.Errorf("expected [LOCATION ORGANIZATION], got %v", result.TypeKeywords)
	}
	if len(result.Entities) != 3 || result.Entities[0] != "France" {
		t.Errorf("expected [France Paris Capital], got %v", result.Entities)
	}
}

func TestParseQueryRewriteResponse_MarkdownBlock(t *testing.T) {
	resp := "Here is the result:\n```json\n{\n\t\"answer_type_keywords\": [\"DATE\"],\n\t\"entities_from_query\": [\"SpaceX\", \"launch\"]\n}\n```"
	result, err := ParseQueryRewriteResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.TypeKeywords) != 1 || result.TypeKeywords[0] != "DATE" {
		t.Errorf("expected [DATE], got %v", result.TypeKeywords)
	}
}

func TestParseQueryRewriteResponse_ExtraText(t *testing.T) {
	resp := `Some text before
{
	"answer_type_keywords": ["PERSON"],
	"entities_from_query": ["Einstein"]
}
Some text after`
	result, err := ParseQueryRewriteResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entities) != 1 || result.Entities[0] != "Einstein" {
		t.Errorf("expected [Einstein], got %v", result.Entities)
	}
}

func TestParseQueryRewriteResponse_Invalid(t *testing.T) {
	resp := "This is not valid JSON"
	_, err := ParseQueryRewriteResponse(resp)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseQueryRewriteResponse_EmptyEntities(t *testing.T) {
	resp := `{"answer_type_keywords": ["LOCATION"], "entities_from_query": []}`
	result, err := ParseQueryRewriteResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entities) != 0 {
		t.Errorf("expected empty entities, got %v", result.Entities)
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
