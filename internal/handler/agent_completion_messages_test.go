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

package handler

import (
	"reflect"
	"strings"
	"testing"
)

func agentMultiTurnMessages() []map[string]interface{} {
	return []map[string]interface{}{
		{"role": "user", "content": "Which products support SSO?"},
		{"role": "assistant", "content": "A, B and C."},
		{"role": "user", "content": "What about the second one?"},
	}
}

func TestPrepareAgentCompletionMessages_OpenAICompatKeepsHistory(t *testing.T) {
	req := &agentChatCompletionsRequest{OpenAICompat: true, Messages: agentMultiTurnMessages()}

	if err := prepareAgentCompletionMessages(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(req.Messages, agentMultiTurnMessages()) {
		t.Fatalf("history must reach the run, got %#v", req.Messages)
	}
	if req.Query != "What about the second one?" {
		t.Fatalf("query = %q, want the last user turn", req.Query)
	}
}

func TestPrepareAgentCompletionMessages_ExplicitTextCollapsesHistory(t *testing.T) {
	cases := []struct {
		name, question, query string
	}{
		{"query", "", "standalone question"},
		{"question", "standalone question", ""},
		{"question wins over query", "standalone question", "ignored"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &agentChatCompletionsRequest{
				OpenAICompat: true,
				Question:     tc.question,
				Query:        tc.query,
				Messages:     agentMultiTurnMessages(),
			}

			if err := prepareAgentCompletionMessages(req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := []map[string]interface{}{{"role": "user", "content": "standalone question"}}
			if !reflect.DeepEqual(req.Messages, want) {
				t.Fatalf("got %#v, want %#v", req.Messages, want)
			}
			if req.Query != "standalone question" {
				t.Fatalf("query = %q, want the explicit text", req.Query)
			}
		})
	}
}

func TestPrepareAgentCompletionMessages_NoMessagesStaysEmpty(t *testing.T) {
	req := &agentChatCompletionsRequest{OpenAICompat: true}

	if err := prepareAgentCompletionMessages(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) != 0 || req.Query != "" {
		t.Fatalf("expected no messages and no query, got %#v / %q", req.Messages, req.Query)
	}
}

func TestPrepareAgentCompletionMessages_RejectsLastMessageNotFromUser(t *testing.T) {
	messages := append(agentMultiTurnMessages(), map[string]interface{}{"role": "assistant", "content": "done"})
	req := &agentChatCompletionsRequest{OpenAICompat: true, Messages: messages}

	err := prepareAgentCompletionMessages(req)

	if err == nil || !strings.Contains(err.Error(), "not from user") {
		t.Fatalf("expected a last-message validation error, got %v", err)
	}
}
