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

package service

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// TestAddSkillTokenFields_PreLowercasesAllFourFields is the regression test
// for the SkillIndexer / Skill Hub sibling of the cycle 85/86
// content_ltks / title_tks fold (#20304): every tokenised value written to
// name_tks / tags_tks / description_tks / content_tks must be pre-lowercased
// so the canvas's query-side lowercase matches the whitespace-analyzed
// indexed value.
func TestAddSkillTokenFields_PreLowercasesAllFourFields(t *testing.T) {
	skill := SkillInfo{
		ID:          "skill-1",
		Name:        "Hello World",
		Description: "Acoustic Assessment",
		Tags:        []string{"Docker Compose", "NLP"},
		Content:     "Deploy via Compose",
	}
	cfg := entity.FieldConfig{
		Name:        entity.FieldWeight{Enabled: true},
		Tags:        entity.FieldWeight{Enabled: true},
		Description: entity.FieldWeight{Enabled: true},
		Content:     entity.FieldWeight{Enabled: true},
	}

	doc := map[string]interface{}{}
	addSkillTokenFields(doc, skill, cfg)

	cases := []struct {
		field string
	}{
		{"name_tks"},
		{"tags_tks"},
		{"description_tks"},
		{"content_tks"},
	}
	for _, tc := range cases {
		raw, ok := doc[tc.field]
		if !ok {
			t.Errorf("expected %q in doc, missing", tc.field)
			continue
		}
		s, ok := raw.(string)
		if !ok {
			t.Errorf("%q: want string, got %T", tc.field, raw)
			continue
		}
		if s != strings.ToLower(s) {
			t.Errorf("%q is not pre-lowercased: %q", tc.field, s)
		}
	}
}

// TestAddSkillTokenFields_DisabledFieldsAreOmitted guards the operator
// toggle: when Description or Content is disabled, the corresponding
// *_tks field must NOT appear in the doc, so a later search against the
// index does not match ghost tokens.
func TestAddSkillTokenFields_DisabledFieldsAreOmitted(t *testing.T) {
	skill := SkillInfo{
		ID:          "skill-1",
		Name:        "Hello",
		Description: "Desc",
		Tags:        []string{"x"},
		Content:     "Body",
	}
	cfg := entity.FieldConfig{
		Name:        entity.FieldWeight{Enabled: true},
		Tags:        entity.FieldWeight{Enabled: true},
		Description: entity.FieldWeight{Enabled: false},
		Content:     entity.FieldWeight{Enabled: false},
	}
	doc := map[string]interface{}{}
	addSkillTokenFields(doc, skill, cfg)

	if _, has := doc["description_tks"]; has {
		t.Errorf("description_tks should be omitted when Description.Enabled=false")
	}
	if _, has := doc["content_tks"]; has {
		t.Errorf("content_tks should be omitted when Content.Enabled=false")
	}
	if _, has := doc["name_tks"]; !has {
		t.Errorf("name_tks must always be present when Name.Enabled=true")
	}
	if _, has := doc["tags_tks"]; !has {
		t.Errorf("tags_tks must always be present when Tags.Enabled=true")
	}
}

// TestAddSkillTokenFields_PreLowercaseMatchesLowercasedQuery is the
// behavioural test: an indexed capitalised skill name must share the same
// BM25 token set as a lowercased query. The Tokenize path may emit
// punctuation; we assert the strict-substring property the cycle 85/86
// fold establishes (every ASCII letter in the indexed value is lower-case,
// so a strict-lower-case query can match).
func TestAddSkillTokenFields_PreLowercaseMatchesLowercasedQuery(t *testing.T) {
	skill := SkillInfo{Name: "GitHub Actions Runner", Tags: []string{"CI", "DevOps"}}
	cfg := entity.FieldConfig{
		Name: entity.FieldWeight{Enabled: true},
		Tags: entity.FieldWeight{Enabled: true},
	}
	doc := map[string]interface{}{}
	addSkillTokenFields(doc, skill, cfg)

	nameTks := doc["name_tks"].(string)
	tagsTks := doc["tags_tks"].(string)

	// The indexed tokens must not contain any capital ASCII letters — that
	// is the exact precondition for the canvas's query-side lowercase
	// (`strings.ToLower(query)` in chunk.go:2096) to match.
	for _, r := range nameTks {
		if r >= 'A' && r <= 'Z' {
			t.Errorf("name_tks still contains capital letter %q in %q", r, nameTks)
		}
	}
	for _, r := range tagsTks {
		if r >= 'A' && r <= 'Z' {
			t.Errorf("tags_tks still contains capital letter %q in %q", r, tagsTks)
		}
	}
}
