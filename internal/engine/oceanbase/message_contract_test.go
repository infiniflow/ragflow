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
	"reflect"
	"strings"
	"testing"
)

func TestMessageSubstringFilterKeepsExactFilters(t *testing.T) {
	got, args, err := buildFilter(map[string]interface{}{"session_id_contains": "part_%'x"}, "memory")
	if err != nil || got != "INSTR(`session_id`, ?) > 0" || !reflect.DeepEqual(args, []interface{}{"part_%'x"}) {
		t.Fatalf("%s %v %v", got, args, err)
	}
	exact, _, err := buildFilter(map[string]interface{}{"session_id": "part"}, "memory")
	if err != nil || !strings.Contains(exact, " = ?") {
		t.Fatal(exact, err)
	}
}

func TestMemoryMessageIDsDecodeAsIntegers(t *testing.T) {
	for _, tc := range []struct {
		stored string
		want   int64
	}{{"42", 42}, {"9223372036854775807", 9223372036854775807}} {
		row := decodeLogicalRow(map[string]interface{}{"message_id": tc.stored}, "memory")
		if row["message_id"] != tc.want {
			t.Fatalf("message_id = %#v, want %d", row["message_id"], tc.want)
		}
	}
}

func TestMemoryContentIncludesUnsetForgetTime(t *testing.T) {
	row := decodeLogicalRow(map[string]interface{}{"message_id": "42", "content_ltks": "content"}, "memory")
	if value, ok := row["forget_at"]; !ok || value != nil {
		t.Fatalf("unset forget_at = %#v, present=%v", value, ok)
	}
}
