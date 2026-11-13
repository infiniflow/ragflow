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
	"strings"
	"testing"
)

func TestMessageSubstringFilterKeepsExactFilters(t *testing.T) {
	got := equivalentConditionToStrRaw(map[string]interface{}{"session_id_contains": "part_%'x", "agent_id": "exact"}, nil, true)
	if !strings.Contains(got, "regex(session_id") || !strings.Contains(got, `part_%\x27x`) || !strings.Contains(got, "agent_id='exact'") {
		t.Fatal(got)
	}
	exact := buildFilterFromConditionRaw(map[string]interface{}{"session_id": "part"}, nil)
	if strings.Contains(exact, "LIKE") {
		t.Fatal(exact)
	}
}
func TestMemoryEmbeddingPublicField(t *testing.T) {
	row := map[string]interface{}{"q_3_vec": []float32{1, 2, 3}, "message_type_kwd": "raw"}
	normalizeMemoryMessageRows([]map[string]interface{}{row})
	if _, ok := row["q_3_vec"]; ok {
		t.Fatal(row)
	}
	if row["content_embed"] == nil || row["message_type"] != "raw" {
		t.Fatal(row)
	}
}
