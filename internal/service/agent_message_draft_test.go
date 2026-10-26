package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func messageDraftDSL(params map[string]any) entity.JSONMap {
	return entity.JSONMap{"graph": map[string]any{"nodes": []any{map[string]any{"id": "begin"}, map[string]any{"id": "message"}}, "edges": []any{}}, "components": map[string]any{
		"begin":   map[string]any{"obj": map[string]any{"component_name": "Begin", "params": map[string]any{}}, "downstream": []any{"message"}},
		"message": map[string]any{"obj": map[string]any{"component_name": "Message", "params": params}, "upstream": []any{"begin"}},
	}, "path": []any{}}
}

func TestMessageDraftRoundTripAndExecution(t *testing.T) {
	setupAgentSessionServiceTest(t)
	tests := []struct {
		name   string
		params map[string]any
		valid  bool
	}{
		{"missing", map[string]any{}, false},
		{"empty text", map[string]any{"text": ""}, false},
		{"blank text", map[string]any{"text": " \t"}, false},
		{"empty string", map[string]any{"content": ""}, false},
		{"null", map[string]any{"content": nil}, false},
		{"empty array", map[string]any{"content": []any{}}, false},
		{"empty rows", map[string]any{"content": []any{"", " "}}, false},
		{"partial", map[string]any{"content": []any{"hello", ""}}, false},
		{"valid", map[string]any{"content": []any{"hello", "world"}}, true},
		{"valid text", map[string]any{"text": "hello"}, true},
	}
	svc := NewAgentService()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := "Message " + tt.name
			ctx := serviceTestContext(t, "user-1", "user-1")
			row, code, err := svc.CreateAgent(ctx, "user-1", &CreateAgentRequest{Title: &title, DSL: messageDraftDSL(tt.params)})
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("create: code=%v err=%v", code, err)
			}
			for range 2 {
				if err := svc.UpdateAgent(ctx, "user-1", row.ID, map[string]any{"dsl": messageDraftDSL(tt.params)}); err != nil {
					t.Fatalf("save: %v", err)
				}
				saved, err := svc.GetAgent(ctx, "user-1", row.ID)
				if err != nil {
					t.Fatal(err)
				}
				got := saved.DSL["components"].(map[string]any)["message"].(map[string]any)["obj"].(map[string]any)["params"]
				if !reflect.DeepEqual(got, tt.params) {
					t.Fatalf("params=%#v want %#v", got, tt.params)
				}
				versions, err := dao.NewUserCanvasVersionDAO().ListByCanvasID(t.Context(), dao.DB, row.ID)
				if err != nil || len(versions) != 1 {
					t.Fatalf("versions=%v err=%v", len(versions), err)
				}
				if !reflect.DeepEqual(versions[0].DSL["components"].(map[string]any)["message"].(map[string]any)["obj"].(map[string]any)["params"], tt.params) {
					t.Fatal("draft version changed params")
				}
			}
			c, err := canvas.DecodeFromDSL(messageDraftDSL(tt.params))
			if err != nil {
				t.Fatal(err)
			}
			compiled, err := canvas.Compile(t.Context(), c)
			if !tt.valid {
				_, runErr := svc.buildRunFunc(row.ID, nil, messageDraftDSL(tt.params))(t.Context(), map[string]any{"user_id": "user-1"})
				_, userFacing := errors.AsType[*runtime.UserFacingError](runErr)
				if !userFacing || !strings.Contains(runErr.Error(), "content does not support empty") {
					t.Fatalf("run must return clear validation error: %v", runErr)
				}
				if err == nil || !strings.Contains(err.Error(), "content does not support empty") {
					t.Fatalf("compile error=%v", err)
				}
				if err := svc.UpdateAgent(t.Context(), "user-1", row.ID, map[string]any{"dsl": messageDraftDSL(tt.params), "release": true}); err == nil {
					t.Fatal("release accepted incomplete Message")
				}
				if _, err := svc.PublishAgent(t.Context(), "user-1", row.ID, &PublishAgentRequest{DSL: messageDraftDSL(tt.params)}); err == nil {
					t.Fatal("publish accepted incomplete Message")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			state := canvas.NewCanvasState("message-test", "message-test")
			if _, err := compiled.Workflow.Invoke(canvas.WithState(t.Context(), state), map[string]any{}); err != nil {
				t.Fatalf("invoke: %v", err)
			}
			want := "hello"
			if tt.name == "valid" {
				want = "helloworld"
			}
			if got, err := state.GetVar("message@content"); err != nil || got != want {
				t.Fatalf("content = %#v err=%v, want %q", got, err, want)
			}
		})
	}
}
