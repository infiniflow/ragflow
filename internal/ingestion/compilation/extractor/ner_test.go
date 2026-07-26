//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package extractor

import "testing"

// ---------------------------------------------------------------------------
// Test data — 21 English test cases (ground truth from Python+spaCy)
// ---------------------------------------------------------------------------

type EnTestSpec struct {
	name         string
	text         string
	wantEntities [][2]string // (text, label) pairs that MUST be found
	wantRels     []relSpec   // typed relations that MUST be found
}

type relSpec struct {
	subj string
	pred string
	obj  string
}

var enTests = []EnTestSpec{
	{name: "founded_by_simple", text: "Apple Inc. was founded by Steve Jobs.",
		wantEntities: [][2]string{{"Steve Jobs", "PERSON"}},
		wantRels:     []relSpec{{"Apple Inc.", "founded_by", "Steve Jobs"}}},
	{name: "founded_by_multi", text: "Google was founded by Larry Page and Sergey Brin.",
		wantEntities: [][2]string{{"Larry Page", "PERSON"}, {"Sergey Brin", "PERSON"}},
		wantRels:     []relSpec{{"Google", "founded_by", "Larry Page"}}},
	{name: "cofounder_of", text: "Elon Musk is a co-founder of Tesla.",
		wantEntities: [][2]string{{"Elon Musk", "PERSON"}, {"Tesla", "ORG"}},
		wantRels:     []relSpec{{"Elon Musk", "founded_by", "Tesla"}}},
	{name: "works_for_simple", text: "John works for Microsoft.",
		wantEntities: [][2]string{{"John", "PERSON"}, {"Microsoft", "ORG"}},
		wantRels:     []relSpec{{"John", "works_for", "Microsoft"}}},
	{name: "employee_of", text: "Mary is an employee of Google.",
		wantEntities: [][2]string{{"Mary", "PERSON"}, {"Google", "ORG"}},
		wantRels:     []relSpec{{"Mary", "works_for", "Google"}}},
	{name: "joined_company", text: "Sundar Pichai joined Google in 2004.",
		wantEntities: [][2]string{{"Sundar Pichai", "PERSON"}, {"Google", "ORG"}},
		wantRels:     []relSpec{{"Sundar Pichai", "works_for", "Google"}}},
	{name: "headquartered_in", text: "The company is headquartered in San Francisco.",
		wantEntities: [][2]string{{"San Francisco", "GPE"}},
		wantRels:     nil},
	{name: "based_in", text: "Microsoft is based in Redmond.",
		wantEntities: [][2]string{{"Microsoft", "ORG"}, {"Redmond", "GPE"}},
		wantRels:     []relSpec{{"Microsoft", "located_in", "Redmond"}}},
	{name: "born_in", text: "Albert Einstein was born in Germany.",
		wantEntities: [][2]string{{"Albert Einstein", "PERSON"}, {"Germany", "GPE"}},
		wantRels:     []relSpec{{"Albert Einstein", "born_in", "Germany"}}},
	{name: "ceo_of", text: "Sundar Pichai is the CEO of Google.",
		wantEntities: [][2]string{{"Sundar Pichai", "PERSON"}, {"Google", "ORG"}},
		wantRels:     []relSpec{{"Sundar Pichai", "works_for", "Google"}, {"Sundar Pichai", "ceo_of", "Google"}}},
	{name: "acquired_by", text: "Instagram was acquired by Facebook.",
		wantEntities: nil, // en_core_web_sm doesn't tag these
		wantRels:     nil},
	{name: "acquired_active", text: "Facebook acquired Instagram.",
		wantEntities: [][2]string{{"Instagram", "PERSON"}}, // en_core_web_sm: Instagram→PERSON
		wantRels:     nil},
	{name: "multi_founded_ceo", text: "Google was founded by Larry Page. Sundar Pichai is the CEO of Google.",
		wantEntities: [][2]string{{"Larry Page", "PERSON"}, {"Sundar Pichai", "PERSON"}},
		wantRels:     nil},
	{name: "multi_works_located", text: "John works for Microsoft. Microsoft is based in Redmond.",
		wantEntities: [][2]string{{"John", "PERSON"}, {"Microsoft", "ORG"}, {"Redmond", "GPE"}},
		wantRels:     []relSpec{{"Microsoft", "located_in", "Redmond"}}},
	{name: "no_entities", text: "The cat sat on the mat.",
		wantEntities: nil,
		wantRels:     nil},
	{name: "org_with_inc", text: "Microsoft Corporation was founded by Bill Gates.",
		wantEntities: [][2]string{{"Bill Gates", "PERSON"}},
		wantRels:     []relSpec{{"Microsoft Corporation", "founded_by", "Bill Gates"}}},
	{name: "located_city", text: "The restaurant is located in Paris.",
		wantEntities: [][2]string{{"Paris", "GPE"}},
		wantRels:     nil},
}

// ---------------------------------------------------------------------------
// Fast unit tests (pure Go, no Python dependency)
// ---------------------------------------------------------------------------

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"Hello world", "en"},
		{"你好世界", "zh"},
		{"こんにちは世界", "ja"},
		{"阿里巴巴由马云创立", "zh"},
		{"アップルは", "ja"},
	}
	for _, tt := range tests {
		got := DetectLanguage(tt.text)
		if got != tt.want {
			t.Errorf("DetectLanguage(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
