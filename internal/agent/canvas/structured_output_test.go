package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/agent/chat"
	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity/models"

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

func TestCanvasAgentStructuredOutputFeedsMessage(t *testing.T) {
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatal(err)
	}
	previousAllowHost := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true
	t.Cleanup(func() { common.AllowAnyHostForTest = previousAllowHost })

	for _, tc := range []struct {
		name        string
		initial     string
		corrected   string
		corrections int
		wantErr     bool
	}{
		{"valid", `{"count":7}`, `{"count":7}`, 0, false},
		{"corrected", `{"count":"bad"}`, `{"count":7}`, 1, false},
		{"exhausted", `{"count":"bad"}`, `{"count":"still bad"}`, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var streamCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				streamCalls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
					t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				var request struct {
					Stream bool `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !request.Stream {
					t.Errorf("expected a streaming request: stream=%v err=%v", request.Stream, err)
					http.Error(w, "expected a streaming request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w,
					"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
					tc.initial,
				)
			}))
			t.Cleanup(server.Close)

			inv := &structuredCanvasInvoker{responses: []string{tc.corrected}}
			previous := component.GetDefaultChatInvokerForTest()
			component.SetDefaultChatInvoker(inv)
			t.Cleanup(func() { component.SetDefaultChatInvoker(previous) })

			dsl := &Canvas{Components: map[string]CanvasComponent{
				"begin": {
					Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
					Downstream: []string{"agent"},
				},
				"agent": {
					Obj: CanvasComponentObj{ComponentName: "Agent", Params: map[string]any{
						"llm_id": "structured-test", "driver": "OpenAI",
						"api_key": "test", "base_url": server.URL,
						"user_prompt": "count", "max_rounds": 1, "max_retries": 1,
						"outputs": map[string]any{"structured": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"count": map[string]any{"type": "integer"},
							},
							"required": []any{"count"}, "additionalProperties": false,
						}},
					}},
					Upstream: []string{"begin"}, Downstream: []string{"message"},
				},
				"message": {
					Obj: CanvasComponentObj{
						ComponentName: "Message",
						Params:        map[string]any{"text": "{agent@structured.count}"},
					},
					Upstream: []string{"agent"},
				},
			}, Path: []string{"begin", "agent", "message"}}

			var emitted strings.Builder
			ctx := runtime.WithCanvasMessageEmitter(t.Context(), func(content string) {
				emitted.WriteString(content)
			})
			compiled, err := Compile(ctx, dsl)
			if err != nil {
				t.Fatal(err)
			}
			state := NewCanvasState("structured-agent-"+tc.name, "session")
			_, err = compiled.Workflow.Invoke(withState(ctx, state), map[string]any{"query": "count"})

			if streamCalls.Load() != 1 || inv.calls != tc.corrections {
				t.Fatalf("provider calls=%d corrections=%d", streamCalls.Load(), inv.calls)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid Agent output completed Canvas")
				}
				for _, ref := range []string{"agent@content", "agent@structured", "message@content"} {
					if value, getErr := state.GetVar(ref); getErr == nil && value != nil {
						t.Fatalf("invalid output published to %s: %#v", ref, value)
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
			value, err := state.GetVar("agent@structured")
			if err != nil || !reflect.DeepEqual(value, map[string]any{"count": float64(7)}) {
				t.Fatalf("structured output=%#v err=%v", value, err)
			}
			content, err := state.GetVar("message@content")
			if err != nil || content != "7" || emitted.String() != "7" {
				t.Fatalf("message=%#v err=%v emitted=%q", content, err, emitted.String())
			}
		})
	}
}
