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

package service

import (
	"reflect"
	"testing"
)

func TestCompletionMessages_KeepsClientHistoryWithoutShorthand(t *testing.T) {
	history := []map[string]interface{}{
		{"role": "user", "content": "Which products support SSO?"},
		{"role": "assistant", "content": "A, B and C."},
		{"role": "user", "content": "What about the second one?"},
	}

	got := CompletionMessages("", "", history)

	want := []map[string]interface{}{
		{"role": "user", "content": "Which products support SSO?"},
		{"role": "assistant", "content": "A, B and C."},
		{"role": "user", "content": "What about the second one?"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history must be kept untouched, got %#v", got)
	}
}

func TestCompletionMessages_ShorthandCollapsesToSingleTurn(t *testing.T) {
	history := []map[string]interface{}{
		{"role": "user", "content": "old"},
		{"role": "assistant", "content": "reply"},
	}
	cases := []struct {
		name, question, query, want string
	}{
		{"question", "q", "", "q"},
		{"query", "", "s", "s"},
		{"question wins over query", "q", "s", "q"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompletionMessages(tc.question, tc.query, history)

			want := []map[string]interface{}{{"role": "user", "content": tc.want}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestCompletionMessages_EmptyInputStaysEmpty(t *testing.T) {
	if got := CompletionMessages("", "", nil); len(got) != 0 {
		t.Fatalf("expected no messages, got %#v", got)
	}
}
