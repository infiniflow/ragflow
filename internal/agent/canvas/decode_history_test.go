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

package canvas

import (
	"reflect"
	"testing"
)

func TestDecodeFromDSLHistoryRoundTrip(t *testing.T) {
	dslHistory := []any{
		[]any{"user", "你好"},
		[]any{"assistant", map[string]any{"content": "您好", "downloads": []any{"file-1"}}},
	}
	dslMemory := []any{
		[]any{"question", "answer", "searched the knowledge base"},
	}
	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
			},
		},
		"history": dslHistory,
		"memory":  dslMemory,
	}

	decoded, err := DecodeFromDSL(dsl)
	if err != nil {
		t.Fatalf("DecodeFromDSL: %v", err)
	}
	if len(decoded.History) != 2 {
		t.Fatalf("history length = %d, want 2", len(decoded.History))
	}
	if got := decoded.History[1]["content"]; got != "您好" {
		t.Fatalf("assistant content = %v, want 您好", got)
	}
	if !reflect.DeepEqual(EncodeHistory(decoded.History), dslHistory) {
		t.Fatalf("history round trip = %#v, want %#v", EncodeHistory(decoded.History), dslHistory)
	}
	if !reflect.DeepEqual(EncodeMemory(decoded.Memory), dslMemory) {
		t.Fatalf("memory round trip = %#v, want %#v", EncodeMemory(decoded.Memory), dslMemory)
	}
}

func TestDecodeFromDSLSkipsMalformedHistoryEntries(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
			},
		},
		"history": []any{
			[]any{"user"},
			42,
			[]any{"assistant", "valid"},
		},
	}

	decoded, err := DecodeFromDSL(dsl)
	if err != nil {
		t.Fatalf("DecodeFromDSL: %v", err)
	}
	if len(decoded.History) != 1 || decoded.History[0]["role"] != "assistant" {
		t.Fatalf("decoded history = %#v, want one assistant entry", decoded.History)
	}
}
