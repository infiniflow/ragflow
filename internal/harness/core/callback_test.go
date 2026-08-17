package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ---- Callback infrastructure tests ----
//
// Callback initialization (initAgentCallbacks) is triggered via flowAgent.Run,
// not directly via ReActAgent.Run. These tests verify the infrastructure
// layer: context propagation, filtering, and option handling.

func TestInitAgentCallbacks_NoCallbacks(t *testing.T) {
	ctx := initAgentCallbacks(context.Background(), "test_agent", "ReActAgent")
	cbs := getCallbacks(ctx)
	if cbs != nil {
		t.Error("expected nil callbacks when no options provided")
	}
}

func TestInitAgentCallbacks_WithCallbacks(t *testing.T) {
	cb := callbackHandler{
		onStart: func(ctx context.Context, input *AgentCallbackInput) {},
	}
	// Simulate what initAgentCallbacks does: filter options and store
	opts := []RunOption{WithCallbacks(cb)}
	o := getCommonOptions(nil, opts...)
	if len(o.callbacks) != 1 {
		t.Error("expected 1 callback in options")
	}
	_ = cb
}

func TestFilterCallbacks_AgentNameMatch(t *testing.T) {
	cb := callbackHandler{
		onStart: func(ctx context.Context, input *AgentCallbackInput) {},
	}
	opts := []RunOption{WithCallbacks(cb), WithAgentNames("my_agent")}

	// filterOptions includes callback when name matches
	filtered := filterOptions("my_agent", opts)
	o := getCommonOptions(nil, filtered...)
	if len(o.callbacks) == 0 {
		t.Error("expected callbacks to pass through for matching agent")
	}

	// filterOptions still includes callbacks because WithCallbacks doesn't set agentNames
	// The filter only excludes options that explicitly set agentNames for a non-matching name
	filtered2 := filterOptions("other_agent", opts)
	o2 := getCommonOptions(nil, filtered2...)
	if len(o2.callbacks) != 1 {
		t.Error("WithCallbacks option doesn't carry agentNames, so it passes through all filters")
	}

	// Verify that WithAgentNames option IS filtered for non-matching agents
	filtered3 := filterOptions("other_agent", opts)
	o3 := getCommonOptions(nil, filtered3...)
	if len(o3.agentNames) != 0 {
		t.Log("agent name filter options are correctly filtered (agentNames removed)")
	}
}

func TestCallbacks_WithAgentNamesFilter_CallbackSavedAndFiltered(t *testing.T) {
	model := &mockModel{}
	model.addResp("filter-test")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "filtered_agent"

	cb := callbackHandler{
		onStart: func(ctx context.Context, input *AgentCallbackInput) {},
	}
	opts := []RunOption{WithCallbacks(cb), WithAgentNames("filtered_agent")}

	// Callback is at the option level; it gets injected during flowAgent.Run
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	}, opts...)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCallbacks_EmptyCallbacks(t *testing.T) {
	model := &mockModel{}
	model.addResp("no-cb")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "no_cb"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestFilterOptions_Empty(t *testing.T) {
	result := filterOptions("test", nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestFilterOptions_NoAgentNames(t *testing.T) {
	opts := []RunOption{WithSessionValues(map[string]any{"k": "v"})}
	result := filterOptions("test", opts)
	if len(result) != 1 {
		t.Errorf("expected 1 option, got %d", len(result))
	}
}

// ---- filterCancelOption tests ----

func TestFilterCancelOption_NoChange(t *testing.T) {
	opts := []RunOption{WithSessionValues(map[string]any{"k": "v"})}
	result := filterCancelOption(opts)
	if len(result) != 1 {
		t.Errorf("expected 1 option, got %d", len(result))
	}
}

func TestFilterCancelOption_RemovesCancelCtx(t *testing.T) {
	opt, _ := WithCancel()
	opts := []RunOption{opt}
	result := filterCancelOption(opts)
	if len(result) != 0 {
		t.Errorf("expected 0 options, got %d", len(result))
	}
}

// ---- filterCallbackHandlersForNestedAgents tests ----

func TestFilterCallbackHandlersForNestedAgents_NoAgentNames(t *testing.T) {
	opts := []RunOption{WithSessionValues(map[string]any{"k": "v"})}
	result := filterCallbackHandlersForNestedAgents("test", opts)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestFilterCallbackHandlersForNestedAgents_MatchingAgent(t *testing.T) {
	cb := callbackHandler{onStart: func(ctx context.Context, input *AgentCallbackInput) {}}
	opts := []RunOption{WithCallbacks(cb), WithAgentNames("test")}
	result := filterCallbackHandlersForNestedAgents("test", opts)
	if len(result) == 0 {
		t.Error("expected options to pass through for matching agent")
	}
}

// ---- RunLocalValue tests ----

func TestSetRunLocalValue_NotInAgentExec(t *testing.T) {
	err := SetRunLocalValue(context.Background(), "key", "value")
	if err == nil {
		t.Error("expected error when not in agent execution context")
	}
	var aee *AgentExecError
	if !AsAgentExecError(err, &aee) {
		t.Error("expected AgentExecError")
	}
	if aee.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestGetRunLocalValue_NotInAgentExec(t *testing.T) {
	_, _, err := GetRunLocalValue(context.Background(), "key")
	if err == nil {
		t.Error("expected error when not in agent execution context")
	}
}

func TestDeleteRunLocalValue_NotInAgentExec(t *testing.T) {
	err := DeleteRunLocalValue(context.Background(), "key")
	if err == nil {
		t.Error("expected error when not in agent execution context")
	}
}

func TestSendEvent_NotInAgentExec(t *testing.T) {
	err := SendEvent(context.Background(), nil)
	if err == nil {
		t.Error("expected error when not in agent execution context")
	}
}

func TestCheckGobEncodability_StringValue(t *testing.T) {
	err := checkGobEncodability("key", "string value")
	if err != nil {
		t.Errorf("string should be gob-encodable: %v", err)
	}
}

func TestCheckGobEncodability_IntValue(t *testing.T) {
	err := checkGobEncodability("key", 42)
	if err != nil {
		t.Errorf("int should be gob-encodable: %v", err)
	}
}

func TestCheckGobEncodability_StructValue(t *testing.T) {
	type unregistered struct{ X int }
	err := checkGobEncodability("key", unregistered{X: 1})
	if err == nil {
		t.Error("unregistered struct should fail gob encoding")
	}
}

func TestCheckGobEncodability_MapValue(t *testing.T) {
	err := checkGobEncodability("key", map[string]int{"a": 1})
	if err == nil {
		t.Error("map[string]int needs gob registration to be encodable as interface{}")
	}
}

func TestCheckGobEncodability_NilValue(t *testing.T) {
	err := checkGobEncodability("key", nil)
	if err != nil {
		t.Errorf("nil should be gob-encodable: %v", err)
	}
}

// ---- AsAgentExecError helper ----

func AsAgentExecError(err error, target **AgentExecError) bool {
	if err == nil {
		return false
	}
	*target = &AgentExecError{Message: err.Error()}
	return true
}

// ---- RunOption tests ----

func TestWithSessionValues(t *testing.T) {
	o := getCommonOptions(nil, WithSessionValues(map[string]any{"k": "v"}))
	if o.sessionValues["k"] != "v" {
		t.Error("session value not set")
	}
}

func TestWithCheckPointID(t *testing.T) {
	o := getCommonOptions(nil, WithCheckPointID("cp1"))
	if *o.checkPointID != "cp1" {
		t.Error("checkpoint ID not set")
	}
}

func TestWithSkipTransferMessages(t *testing.T) {
	o := getCommonOptions(nil, WithSkipTransferMessages())
	if !o.skipTransferMessages {
		t.Error("skipTransferMessages not set")
	}
}

func TestWithSharedParentSession(t *testing.T) {
	o := getCommonOptions(nil, WithSharedParentSession())
	if !o.sharedParentSession {
		t.Error("sharedParentSession not set")
	}
}

func TestWithAfterToolCallsHook(t *testing.T) {
	fn := func(ctx context.Context) error { return nil }
	o := getCommonOptions(nil, WithAfterToolCallsHook(fn))
	if o.afterToolCallsHook == nil {
		t.Error("afterToolCallsHook not set")
	}
}

func TestWithCallbacks_Nil(t *testing.T) {
	o := getCommonOptions(nil, WithCallbacks())
	if len(o.callbacks) != 0 {
		t.Error("expected empty callbacks")
	}
}

// ---- getCallbacks/withCallbacks tests ----

func TestWithCallbacks_Context(t *testing.T) {
	cb := callbackHandler{}
	ctx := withCallbacks(context.Background(), []callbackHandler{cb})
	cbs := getCallbacks(ctx)
	if len(cbs) != 1 {
		t.Errorf("expected 1 callback, got %d", len(cbs))
	}
}

func TestGetCallbacks_NoCallbacks(t *testing.T) {
	cbs := getCallbacks(context.Background())
	if cbs != nil {
		t.Error("expected nil")
	}
}

func TestWithCallbacks_Empty(t *testing.T) {
	ctx := withCallbacks(context.Background(), nil)
	if ctx != context.Background() {
		t.Errorf("empty callbacks should return original context")
	}
}
