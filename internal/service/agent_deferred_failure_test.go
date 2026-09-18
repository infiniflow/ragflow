package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component" // blank import: registers factories via component.init()
	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestDeferredAgentStreamFailureText(t *testing.T) {
	deferred := &runtime.DeferredStreamError{Text: "**ERROR**: [GraphRunError] model unavailable"}
	if got := deferredAgentStreamFailureText(fmt.Errorf("canvas invoke: component %q invoke: %w", "Message:x", deferred)); got != deferred.Text {
		t.Errorf("deferredAgentStreamFailureText = %q, want the Agent _ERROR text", got)
	}
	embedded := &runtime.DeferredStreamError{Text: "**ERROR**: [GraphRunError] consume deferred Agent stream: model unavailable"}
	if got := deferredAgentStreamFailureText(fmt.Errorf("canvas invoke: %w", embedded)); got != embedded.Text {
		t.Errorf("marker text embedded in the _ERROR value truncated the failure text to %q", got)
	}
	if got := deferredAgentStreamFailureText(errors.New("canvas invoke: Message: consume deferred Agent stream: unrelated")); got != "" {
		t.Errorf("plain error containing the marker text returned %q, want empty", got)
	}
	if got := deferredAgentStreamFailureText(fmt.Errorf("invoke: %w", &runtime.DeferredStreamError{Err: errors.New("stream dropped")})); got != "stream dropped" {
		t.Errorf("underlying stream error returned %q, want its message", got)
	}
	if got := deferredAgentStreamFailureText(fmt.Errorf("invoke: %w", &runtime.DeferredStreamError{Err: context.Canceled})); got != "" {
		t.Errorf("cancellation inside the deferred stream returned %q, want empty", got)
	}
	if got := deferredAgentStreamFailureText(errors.New("canvas invoke: compile error")); got != "" {
		t.Errorf("non-deferred error returned %q, want empty", got)
	}
	if got := deferredAgentStreamFailureText(context.DeadlineExceeded); got != "" {
		t.Errorf("timeout returned %q, want empty", got)
	}
}

// An Agent whose chat model is unavailable fails through its `_ERROR`
// output; the downstream Message records the failure while consuming the
// deferred stream. The run must keep that failure text in the chat message
// flow (like the Python canvas, whose node outputs surface it in the chat
// bubble) instead of terminating the SSE conversation with an error frame
// that the front-end renders as toast popups.
func TestRunAgent_DeferredAgentModelFailureStaysInChatStream(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
				"downstream": []any{"Agent:fail_0"},
			},
			"Agent:fail_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Agent",
					"params": map[string]any{
						"llm_id":   "unavailable-model",
						"driver":   "OpenAI",
						"api_key":  "test-key",
						"base_url": "http://127.0.0.1:59999/v1",
						"prompts":  []any{map[string]any{"content": "{sys.query}", "role": "user"}},
					},
				},
				"upstream":   []any{"begin_0"},
				"downstream": []any{"Message:fail_0"},
			},
			"Message:fail_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"content": []any{"{Agent:fail_0@content}"}},
				},
				"upstream": []any{"Agent:fail_0"},
			},
		},
		"path": []any{"begin_0", "Agent:fail_0", "Message:fail_0"},
	}
	dao.DB.Create(&entity.TenantModelProvider{ID: "prov-deferred-fail", ProviderName: "OpenAI-API-Compatible", TenantID: "tenant-1"})
	dao.DB.Create(&entity.TenantModelInstance{ID: "inst-deferred-fail", InstanceName: "unreachable", ProviderID: "prov-deferred-fail", APIKey: "test-key", Extra: `{"base_url": "http://127.0.0.1:59999/v1"}`})
	dao.DB.Create(&entity.TenantModel{ID: "unavailable-model", ModelName: "unavailable-model", ProviderID: "prov-deferred-fail", InstanceID: "inst-deferred-fail", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"})
	makeCanvasWithDSL(t, "canvas-deferred-fail", "user-1", "tenant-1", "v-deferred-fail", dsl)

	tracker, _ := newAgentCancelTracker(t)
	svc := NewAgentServiceWithOptions(nil, nil, tracker)
	events, err := svc.RunAgent(
		t.Context(),
		"user-1",
		"canvas-deferred-fail",
		"session-deferred-fail",
		"",
		"hello", nil)
	if err != nil {
		t.Fatalf("RunAgent returned sync error: %v", err)
	}
	messages, _, errs, messageEnds, done := drainAgentEvents(t, events)
	if len(errs) != 0 {
		t.Fatalf("model failure produced error frames %+v, want the failure kept in the message stream", errs)
	}
	if !done {
		t.Fatal("stream did not terminate with done")
	}
	if messageEnds == 0 {
		t.Fatal("deferred failure stream ended without message_end")
	}
	var failureText string
	for _, m := range messages {
		if strings.Contains(m.Content, "**ERROR**") {
			failureText = m.Content
			break
		}
	}
	if failureText == "" {
		t.Fatalf("no message event carries the Agent _ERROR text; messages: %+v", messages)
	}
	assertPersistedAssistantFailure(t, "session-deferred-fail", "canvas-deferred-fail", failureText)
	assertRunMarkedFailed(t, tracker, "canvas-deferred-fail", "session-deferred-fail")
}

func assertPersistedAssistantFailure(t *testing.T, sessionID, agentID, wantText string) {
	t.Helper()
	conv, err := dao.NewAPI4ConversationDAO().GetBySessionID(t.Context(), dao.DB, sessionID, agentID)
	if err != nil {
		t.Fatalf("reload conversation: %v", err)
	}
	var history []map[string]any
	if err := json.Unmarshal(conv.Message, &history); err != nil {
		t.Fatalf("parse persisted message history: %v", err)
	}
	for _, turn := range history {
		if turn["role"] != "assistant" {
			continue
		}
		if content, _ := turn["content"].(string); strings.Contains(content, wantText) {
			return
		}
	}
	t.Fatalf("persisted assistant answer does not carry the failure text %q; history: %+v", wantText, history)
}

func assertRunMarkedFailed(t *testing.T, tracker *canvas.RunTracker, canvasID, sessionID string) {
	t.Helper()
	fields, err := tracker.Get(t.Context(), runIDFor(canvasID, map[string]any{"session_id": sessionID}))
	if err != nil {
		t.Fatalf("read run status: %v", err)
	}
	if fields["status"] != "2" {
		t.Fatalf("run status = %q, want 2 (failed); fields: %+v", fields["status"], fields)
	}
}
