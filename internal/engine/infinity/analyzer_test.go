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

package infinity

import "testing"

func TestAnalyzerForLanguage(t *testing.T) {
	tests := []struct {
		name     string
		analyzer string
		language string
		want     string
	}{
		{"slovak suffixes the coarse rag analyzer", "rag-coarse", "Slovak", "rag-coarse-slovak"},
		{"slovak suffixes the fine rag analyzer", "rag-fine", "Slovak", "rag-fine-slovak"},
		{"czech is handled the same way", "rag-coarse", "czech", "rag-coarse-czech"},
		{"the bare rag analyzer takes a suffix too", "rag", "Slovak", "rag-slovak"},
		{"case and surrounding space do not matter", "rag-fine", "  CZECH  ", "rag-fine-czech"},

		// Languages the analyzer treats no differently must not move: their
		// datasets are already indexed under the default analyzer, and
		// switching a Snowball stemmer on would silently change tokenization.
		{"english is left alone", "rag-coarse", "English", "rag-coarse"},
		{"dutch is left alone", "rag-coarse", "Dutch", "rag-coarse"},
		{"an unset language is left alone", "rag-coarse", "", "rag-coarse"},

		// Keyword analyzers are language independent.
		{"whitespace is left alone", "whitespace-#", "Slovak", "whitespace-#"},
		{"rankfeatures is left alone", "rankfeatures", "Slovak", "rankfeatures"},
		{"a rag prefix is not a rag analyzer", "ragged", "Slovak", "ragged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := analyzerForLanguage(tt.analyzer, tt.language); got != tt.want {
				t.Errorf("analyzerForLanguage(%q, %q) = %q, want %q", tt.analyzer, tt.language, got, tt.want)
			}
		})
	}
}

// Queries select the index whose analyzer matches the dataset language.
func TestMatchingFieldSelectsLanguageIndex(t *testing.T) {
	for _, tt := range []struct{ field, analyzer, ref string }{
		{"content", "rag-coarse", "content_ltks"},
		{"content", "rag-fine", "content_sm_ltks"},
		{"docnm", "rag-coarse", "docnm_kwd"},
	} {
		for _, lang := range []struct{ language, suffix string }{{"", ""}, {"English", ""}, {"Czech", "-czech"}, {"Slovak", "-slovak"}} {
			analyzer := tt.analyzer + lang.suffix
			want := tt.field + "@" + fulltextIndexName(tt.field, analyzer)
			if lang.language == "English" {
				want += "_default"
			}
			if got := convertMatchingFieldForLanguage(tt.ref, lang.language); got != want {
				t.Errorf("convertMatchingFieldForLanguage(%q, %q) = %q, want %q", tt.ref, lang.language, got, want)
			}
		}
	}
}

func TestMatchingFieldPreservesExplicitWeights(t *testing.T) {
	for field, want := range map[string]string{
		"content_ltks^2":              "content@ft_content_rag_coarse_czech^2",
		"entity_kwd^10":               "entity_kwd^10",
		"tag_kwd^5":                   "tag_kwd@ft_tag_kwd_whitespace__^5",
		"content@custom_rag_coarse^3": "content@custom_rag_coarse^3",
	} {
		if got := convertMatchingFieldForLanguage(field, "Czech"); got != want {
			t.Errorf("convertMatchingFieldForLanguage(%q, Czech) = %q, want %q", field, got, want)
		}
	}
}

func TestFullTextIndexSpecsSeparateDefaultFromLegacyName(t *testing.T) {
	schema := orderedFields{
		Keys: []string{"content"},
		Fields: map[string]fieldInfo{
			"content": {Type: "varchar", Analyzer: []interface{}{"rag-coarse"}},
		},
	}
	for language, want := range map[string]fullTextIndexSpec{
		"":        {field: "content", name: "ft_content_rag_coarse", analyzer: "rag-coarse"},
		"English": {field: "content", name: "ft_content_rag_coarse_default", analyzer: "rag-coarse"},
		"Czech":   {field: "content", name: "ft_content_rag_coarse_czech", analyzer: "rag-coarse-czech"},
	} {
		got := fullTextIndexSpecs(schema, language)
		if len(got) != 1 || got[0] != want {
			t.Errorf("fullTextIndexSpecs(%q) = %#v, want %#v", language, got, want)
		}
	}
}
