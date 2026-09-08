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

// The fulltext index name is derived from the analyzer, and Infinity analyzes a
// query with the first index it finds for a field. Index names must therefore
// keep sorting coarse-before-fine once the language suffix is appended, so the
// coarse analyzer stays the one a query is analyzed with.
func TestLanguageSuffixKeepsCoarseBeforeFine(t *testing.T) {
	coarse := fulltextIndexName("content", analyzerForLanguage("rag-coarse", "slovak"))
	fine := fulltextIndexName("content", analyzerForLanguage("rag-fine", "slovak"))
	if coarse != "ft_content_rag_coarse_slovak" {
		t.Errorf("coarse index name = %q", coarse)
	}
	if coarse >= fine {
		t.Errorf("expected %q to sort before %q", coarse, fine)
	}
}
