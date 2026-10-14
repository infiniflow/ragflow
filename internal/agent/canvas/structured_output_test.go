package canvas

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/agent/chat"
	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

type structuredCanvasInvoker struct {
	responses []string
	calls     int
}

func (s *structuredCanvasInvoker) Invoke(_ context.Context, _ *gorm.DB, _ chat.Request) (*chat.Response, error) {
	i := s.calls
	s.calls++
	return &chat.Response{Content: s.responses[i], Model: "stub"}, nil
}

func TestCanvasStructuredOutputBlocksInvalidDownstream(t *testing.T) {
	for _, valid := range []bool{false, true} {
		name := "exhausted"
		if valid {
			name = "corrected"
		}
		t.Run(name, func(t *testing.T) {
			second := `{"count":"invalid"}`
			if valid {
				second = `{"count":2}`
			}
			inv := &structuredCanvasInvoker{responses: []string{`{"count":"invalid"}`, second}}
			previous := component.GetDefaultChatInvokerForTest()
			component.SetDefaultChatInvoker(inv)
			t.Cleanup(func() { component.SetDefaultChatInvoker(previous) })
			dsl := &Canvas{Components: map[string]CanvasComponent{
				"begin": {Obj: CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}}, Downstream: []string{"llm"}},
				"llm": {Obj: CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{
					"llm_id": "stub", "user_prompt": "count", "max_retries": 1,
					"outputs": map[string]any{"structured": map[string]any{
						"type": "object", "properties": map[string]any{"count": map[string]any{"type": "integer"}}, "required": []any{"count"},
					}},
				}}, Upstream: []string{"begin"}, Downstream: []string{"message"}},
				"message": {Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{"text": "{llm@content}"}}, Upstream: []string{"llm"}},
			}, Path: []string{"begin", "llm", "message"}}
			var emitted strings.Builder
			ctx := runtime.WithCanvasMessageEmitter(t.Context(), func(content string) { emitted.WriteString(content) })
			compiled, err := Compile(ctx, dsl)
			if err != nil {
				t.Fatal(err)
			}
			state := NewCanvasState("structured-run", "structured-session")
			_, err = compiled.Workflow.Invoke(withState(ctx, state), map[string]any{"query": "count"})
			if inv.calls != 2 {
				t.Fatalf("calls=%d", inv.calls)
			}
			if !valid {
				if err == nil {
					t.Fatal("invalid structured output completed canvas")
				}
				for _, ref := range []string{"llm@content", "llm@structured", "message@content"} {
					if v, e := state.GetVar(ref); e == nil && v != nil {
						t.Fatalf("invalid value published to %s: %#v", ref, v)
					}
				}
				if emitted.Len() != 0 {
					t.Fatalf("invalid message emitted: %q", emitted.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			value, err := state.GetVar("llm@structured")
			if err != nil || value == nil {
				t.Fatalf("structured=%v err=%v", value, err)
			}
			content, err := state.GetVar("message@content")
			if err != nil || content != `{"count":2}` {
				t.Fatalf("message=%v err=%v", content, err)
			}
		})
	}
}

func TestCanvasStructuredOutputPreservesNumericDownstream(t *testing.T) {
	inv := &structuredCanvasInvoker{responses: []string{`{"items":[10,2,0.5]}`}}
	previous := component.GetDefaultChatInvokerForTest()
	component.SetDefaultChatInvoker(inv)
	t.Cleanup(func() { component.SetDefaultChatInvoker(previous) })
	dsl := &Canvas{Components: map[string]CanvasComponent{
		"begin": {Obj: CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}}, Downstream: []string{"llm"}},
		"llm": {Obj: CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{
			"llm_id": "stub", "user_prompt": "numbers", "max_retries": 0,
			"outputs": map[string]any{"structured": map[string]any{
				"properties": map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{"type": "number"}}},
				"required":   []any{"items"},
			}},
		}}, Upstream: []string{"begin"}, Downstream: []string{"sort"}},
		"sort": {Obj: CanvasComponentObj{ComponentName: "ListOperations", Params: map[string]any{
			"query": "llm@structured.items", "operations": "sort", "sort_method": "asc",
		}}, Upstream: []string{"llm"}},
	}, Path: []string{"begin", "llm", "sort"}}
	ctx := t.Context()
	compiled, err := Compile(ctx, dsl)
	if err != nil {
		t.Fatal(err)
	}
	state := NewCanvasState("structured-numbers", "session")
	if _, err := compiled.Workflow.Invoke(withState(ctx, state), map[string]any{"query": "numbers"}); err != nil {
		t.Fatal(err)
	}
	got, err := state.GetVar("sort@result")
	if err != nil || !reflect.DeepEqual(got, []any{float64(0.5), float64(2), float64(10)}) {
		t.Fatalf("numeric output changed downstream semantics: %v, %v", got, err)
	}
}
