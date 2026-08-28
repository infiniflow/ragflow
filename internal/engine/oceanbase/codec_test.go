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

package oceanbase

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
)

func TestNormalizeChunkBoundsImportantKeywordsAndRecomputesTokens(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	defer tokenizer.SetEngineType("")

	asciiKeyword := strings.Repeat("x", importantKeywordMaxBytes+1)
	unicodeKeyword := strings.Repeat("中", 100)
	escapedKeyword := strings.Repeat(`\`, 200)
	document := map[string]interface{}{
		"id":            "chunk-1",
		"kb_id":         "kb-1",
		"important_kwd": []string{asciiKeyword, unicodeKeyword, escapedKeyword},
		"important_tks": "stale tokens",
	}

	normalized, err := normalizeChunk(document)
	if err != nil {
		t.Fatal(err)
	}

	var keywords []string
	if err := json.Unmarshal([]byte(normalized["important_kwd"].(string)), &keywords); err != nil {
		t.Fatal(err)
	}
	if len(keywords) != 3 {
		t.Fatalf("important_kwd = %#v, want three keywords", keywords)
	}
	for _, keyword := range keywords {
		if len(keyword) > importantKeywordMaxBytes {
			t.Errorf("important keyword byte length = %d, want <= %d", len(keyword), importantKeywordMaxBytes)
		}
		if strings.ToValidUTF8(keyword, "") != keyword {
			t.Errorf("important keyword is not valid UTF-8: %q", keyword)
		}
	}
	if normalized["important_tks"] != strings.Join(keywords, " ") {
		t.Errorf("important_tks = %q, want tokens from stored keywords", normalized["important_tks"])
	}
	if document["important_tks"] != "stale tokens" {
		t.Errorf("normalizeChunk mutated input important_tks: %q", document["important_tks"])
	}
}

func TestEncodeUpdateValueBoundsImportantKeyword(t *testing.T) {
	keyword := strings.Repeat("中", 100)

	encoded, err := encodeUpdateValue("chunk", "important_kwd", []string{keyword})
	if err != nil {
		t.Fatal(err)
	}

	var keywords []string
	if err := json.Unmarshal([]byte(encoded.(string)), &keywords); err != nil {
		t.Fatal(err)
	}
	if len(keywords) != 1 || len(keywords[0]) > importantKeywordMaxBytes {
		t.Fatalf("important_kwd = %#v, want one keyword bounded to %d bytes", keywords, importantKeywordMaxBytes)
	}
}
