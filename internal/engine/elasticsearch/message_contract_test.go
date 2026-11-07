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

package elasticsearch

import (
	"reflect"
	"testing"
)

func TestMessageSubstringEscapesWildcardCharacters(t *testing.T) {
	got := buildBoolQueryFromConditionRaw(map[string]interface{}{"session_id_contains": `a*b?c\d`}, nil, false, true)
	want := map[string]interface{}{"filter": []interface{}{map[string]interface{}{"wildcard": map[string]interface{}{"session_id": `*a\*b\?c\\d*`}}}}
	if !reflect.DeepEqual(got, map[string]interface{}{"bool": want}) {
		t.Fatalf("query = %#v, want %#v", got, want)
	}
}

func TestMemorySessionFilterMatchesLiteralSubstring(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "alpha", want: "*alpha*"},
		{input: "lph", want: "*lph*"},
		{input: "alpha OR beta", want: "*alpha OR beta*"},
		{input: "alpha* OR session_id:*", want: `*alpha\* OR session_id:\**`},
		{input: `a*b?c\d`, want: `*a\*b\?c\\d*`},
		{input: "x:(y)", want: "*x:(y)*"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := buildBoolQueryFromCondition(map[string]interface{}{"session_id": tc.input}, nil, false, true)
			want := map[string]interface{}{"bool": map[string]interface{}{"filter": []interface{}{
				map[string]interface{}{"wildcard": map[string]interface{}{"session_id": tc.want}},
			}}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("query = %#v, want %#v", got, want)
			}
		})
	}
}

func TestSessionFilterPreservesOtherQueryForms(t *testing.T) {
	cases := []struct {
		name   string
		value  interface{}
		memory bool
		clause map[string]interface{}
	}{
		{name: "non-memory string", value: "alpha", clause: map[string]interface{}{"term": map[string]interface{}{"session_id": "alpha"}}},
		{name: "memory list", value: []string{"alpha", "beta"}, memory: true, clause: map[string]interface{}{"terms": map[string]interface{}{"session_id": []string{"alpha", "beta"}}}},
		{name: "empty memory string", value: "", memory: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildBoolQueryFromCondition(map[string]interface{}{"session_id": tc.value}, nil, false, tc.memory)
			var want map[string]interface{}
			if tc.clause != nil {
				want = map[string]interface{}{"bool": map[string]interface{}{"filter": []interface{}{tc.clause}}}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("query = %#v, want %#v", got, want)
			}
		})
	}
}
