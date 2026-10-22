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

import (
	"encoding/json"
	"testing"
)

// Linked-content ingestion (#21032) attributes fetched pages with
// source_url/parent_doc_id inside the chunk's `extra` map rather than as
// top-level keys, because Infinity drops fields that are not columns. That
// design rests on the write path below, so pin it here rather than leaving it
// to a comment: `extra` must survive as the declared varchar column with both
// keys intact, while a top-level provenance key must not.
func TestLinkedContentProvenanceSurvivesInfinityWritePath(t *testing.T) {
	provenance := map[string]interface{}{
		"source_url":    "https://example.com/linked",
		"parent_doc_id": "doc-1",
	}

	nested := transformChunkFields(map[string]interface{}{
		"id":                  "c1",
		"content_with_weight": "linked page body",
		"docnm_kwd":           "https://example.com/linked",
		"extra":               provenance,
	}, nil)
	flat := transformChunkFields(map[string]interface{}{
		"id":                  "c2",
		"content_with_weight": "linked page body",
		"docnm_kwd":           "https://example.com/linked",
		"source_url":          "https://example.com/linked",
		"parent_doc_id":       "doc-1",
	}, nil)
	rows := []map[string]interface{}{nested, flat}

	if err := (&Engine{mappingFileName: "infinity_mapping.json"}).dropUndeclaredFields("ragflow_t_kb", rows, map[string]bool{}); err != nil {
		t.Fatalf("dropUndeclaredFields: %v", err)
	}

	encoded, ok := rows[0]["extra"].(string)
	if !ok {
		t.Fatalf("extra = %#v (%T), want the JSON string the varchar column stores", rows[0]["extra"], rows[0]["extra"])
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("extra %q is not a JSON object: %v", encoded, err)
	}
	if decoded["source_url"] != "https://example.com/linked" || decoded["parent_doc_id"] != "doc-1" {
		t.Fatalf("extra = %#v, want both provenance keys", decoded)
	}

	// The control: the same keys as top-level fields are dropped, which is why
	// the feature cannot put them there.
	if _, exists := rows[1]["source_url"]; exists {
		t.Fatal("top-level source_url survived the projection; the extra-column design would be unnecessary")
	}
	if _, exists := rows[1]["parent_doc_id"]; exists {
		t.Fatal("top-level parent_doc_id survived the projection")
	}
}
