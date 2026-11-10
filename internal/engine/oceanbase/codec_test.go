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

	normalized, err := normalizeChunk(document, fullChunkSchema())
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

func fullChunkSchema() map[string]bool {
	columns := make(map[string]bool, len(chunkColumns))
	for _, column := range chunkColumns {
		columns[column.name] = true
	}
	return columns
}

// A table created before a column existed keeps that field in extra. The write
// path must follow the live schema instead of the static definition, or the
// REPLACE names a column the table does not have.
func TestNormalizeChunkRoutesMissingColumnToExtra(t *testing.T) {
	columns := fullChunkSchema()
	delete(columns, "compile_kwd")
	document := map[string]interface{}{
		"id": "chunk-1", "kb_id": "kb-1", "compile_kwd": "wiki", "token_num": 12,
	}
	normalized, err := normalizeChunk(document, columns)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := normalized["compile_kwd"]; ok {
		t.Fatal("compile_kwd was written to a column the table does not have")
	}
	var extra map[string]interface{}
	if err := json.Unmarshal([]byte(normalized["extra"].(string)), &extra); err != nil {
		t.Fatal(err)
	}
	if extra["compile_kwd"] != "wiki" {
		t.Errorf("extra.compile_kwd = %v, want wiki", extra["compile_kwd"])
	}
	if _, ok := normalized["token_num"]; !ok {
		t.Error("token_num has a column but was not written to it")
	}

	normalized, err = normalizeChunk(document, fullChunkSchema())
	if err != nil {
		t.Fatal(err)
	}
	if normalized["compile_kwd"] != "wiki" {
		t.Errorf("compile_kwd = %v, want the physical column value", normalized["compile_kwd"])
	}
}

// extra is the storage of historical values for a field the table has no
// column for, so a read must surface it under the field's own name. A physical
// column is the current value and must win over a stale extra entry.
func TestDecodeChunkExpandsDeclaredFieldsFromExtra(t *testing.T) {
	row := map[string]interface{}{
		"id":          "chunk-1",
		"compile_kwd": "timeline",
		"extra":       `{"compile_kwd":"wiki","token_num":12,"unknown_field":"x"}`,
	}
	decoded := decodeLogicalRow(row, "chunk")
	if decoded["compile_kwd"] != "timeline" {
		t.Errorf("compile_kwd = %v, want the physical value", decoded["compile_kwd"])
	}
	if decoded["token_num"] != float64(12) {
		t.Errorf("token_num = %#v, want the value from extra", decoded["token_num"])
	}
	if _, ok := decoded["unknown_field"]; ok {
		t.Error("a key outside the declared schema was promoted onto the chunk")
	}
}
