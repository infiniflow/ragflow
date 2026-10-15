// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestNormalizeAgentInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		query  string
		inputs map[string]any
		want   any
	}{
		{name: "empty request"},
		{name: "query only", query: "1", want: "1"},
		{name: "true alongside query", query: "1", inputs: map[string]any{"bool": map[string]any{"type": "boolean", "value": true}}, want: map[string]any{"query": "1", "bool": true}},
		{name: "false without query", inputs: map[string]any{"bool": map[string]any{"type": "boolean", "value": false}}, want: map[string]any{"bool": false}},
		{name: "zero and empty values", inputs: map[string]any{"count": map[string]any{"value": 0}, "text": map[string]any{"value": ""}}, want: map[string]any{"count": 0, "text": ""}},
		{name: "unset optional field", inputs: map[string]any{"text": map[string]any{"type": "line", "optional": true}}, want: map[string]any{"text": nil}},
		{name: "raw object", inputs: map[string]any{"data": map[string]any{"enabled": false}}, want: map[string]any{"data": map[string]any{"enabled": false}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeAgentInput(tc.query, tc.inputs)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("normalized input = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestAgentbotCompletionPreservesBooleanBeginInput(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate canvas versions: %v", err)
	}
	pushServiceDB(t, db)
	dsl := map[string]any{
		"components": map[string]any{
			"begin": map[string]any{
				"obj": map[string]any{"component_name": "Begin", "params": map[string]any{
					"inputs": map[string]any{"bool": map[string]any{"name": "bool", "type": "boolean"}},
				}},
				"downstream": []any{"message"},
			},
			"message": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "{{begin@bool}}|{{sys.query}}"}},
				"upstream": []any{"begin"},
			},
		},
		"path": []any{"begin", "message"},
	}
	makeCanvasWithDSL(t, "canvas-bool", "user-1", "tenant-1", "version-bool", dsl)
	svc := NewBotService(NewAgentServiceWithOptions(nil, nil, nil))
	for _, value := range []bool{true, false} {
		t.Run(strconv.FormatBool(value), func(t *testing.T) {
			body := `{"stream":true,"running_hint_text":"正在运行中...🕞","openai-compatible":false,"query":"1","inputs":{"bool":{"name":"bool","optional":false,"options":[],"type":"boolean","value":` + strconv.FormatBool(value) + `}},"files":[],"session_id":null}`
			var req AgentbotCompletionRequest
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Fatal(err)
			}
			events, code, err := svc.AgentbotCompletion(t.Context(), "user-1", "canvas-bool", req)
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("AgentbotCompletion = (%d, %v)", code, err)
			}
			messages, waiting, errs, _, done := drainAgentEvents(t, events)
			if len(errs) != 0 || len(waiting) != 0 || !done {
				t.Fatalf("unexpected run result: errors=%+v waiting=%+v done=%v", errs, waiting, done)
			}
			want := strconv.FormatBool(value) + "|1"
			if len(messages) != 1 || messages[0].Content != want {
				t.Fatalf("messages = %+v, want content %q", messages, want)
			}
		})
	}
}
