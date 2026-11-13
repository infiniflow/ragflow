package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/common"
)

// TestModelInputCaptureOwnsSnapshotAndSeals checks the field allowlist, snapshot
// ownership, and rejection of writes after finalization.
func TestModelInputCaptureOwnsSnapshotAndSeals(t *testing.T) {
	ctx, capture := WithModelInputCapture(t.Context())
	function := map[string]any{"name": "lookup", "arguments": `{"q":"original"}`}
	messages := []Message{
		{Role: "system", Content: "instruction", Name: "excluded-name", FunctionCall: "excluded-function", Refusal: "excluded-refusal"},
		{Role: "assistant", Content: "", ToolCalls: []map[string]any{{"id": "call-1", "type": "function", "function": function}}},
		{Role: "tool", Content: "original", ToolCallID: "call-1"},
	}
	captureModelInput(ctx, "actual-model", messages)
	messages[0].Content = "changed"
	function["arguments"] = "changed"
	result := capture.Finalize()
	captureModelInput(ctx, "late", messages)
	if len(result.Calls) != 1 || result.Calls[0].Model != "actual-model" || result.Calls[0].Sequence != 1 || result.Calls[0].Status != "captured" {
		t.Fatalf("unexpected calls: %+v", result)
	}
	if strings.Contains(string(result.Calls[0].Messages), "changed") || !strings.Contains(string(result.Calls[0].Messages), "original") {
		t.Fatalf("snapshot changed: %s", result.Calls[0].Messages)
	}
	if strings.Contains(string(result.Calls[0].Messages), "excluded-") {
		t.Fatalf("fields outside the snapshot contract leaked: %s", result.Calls[0].Messages)
	}
	if got := capture.Finalize(); len(got.Calls) != 1 || got.OmittedCount != 0 {
		t.Fatalf("late call changed finalized result: %+v", got)
	}
}

// TestModelInputCaptureReservesLaterOmissionReasons checks that large calls leave
// room for later omission markers and small captured calls.
func TestModelInputCaptureReservesLaterOmissionReasons(t *testing.T) {
	empty, _ := json.Marshal(ModelInputCall{
		Sequence: 1, Model: "fill", Status: "captured",
		Messages: json.RawMessage(`[{"role":"user","content":""}]`),
	})
	for _, tc := range []struct {
		name        string
		textBytes   int
		firstStatus string
		omitted     uint64
	}{
		{"near capture limit", maxModelInputBytes - modelInputSummaryReserve - (maxModelInputCalls-1)*modelInputOmissionReserve - 256, "captured", 1},
		// Without marker headroom this call fills every available byte, leaving
		// later ordinary omissions with no place to explain their reason.
		{"would consume marker headroom", maxModelInputBytes - modelInputSummaryReserve - len(empty), "omitted", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, capture := WithModelInputCapture(t.Context())
			captureModelInput(ctx, "fill", []Message{{Role: "user", Content: strings.Repeat("x", tc.textBytes)}})
			captureModelInput(ctx, "image", []Message{{Role: "user", Content: []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "private"}},
			}}})
			captureModelInput(ctx, "small", []Message{{Role: "user", Content: "still fits"}})
			result := capture.Finalize()
			if len(result.Calls) != 3 || result.Calls[0].Status != tc.firstStatus || result.Calls[1].Reason != "unsupported_content" || result.Calls[2].Status != "captured" || result.OmittedCount != tc.omitted {
				t.Fatalf("near-limit capture lost subsequent diagnostics: calls=%d omitted=%d", len(result.Calls), result.OmittedCount)
			}
			if tc.firstStatus == "omitted" && result.Calls[0].Reason != "size_limit" {
				t.Fatalf("large call reason = %q", result.Calls[0].Reason)
			}
			encoded, err := json.Marshal(result)
			if err != nil || len(encoded) > maxModelInputBytes {
				t.Fatalf("result exceeds total budget: bytes=%d err=%v", len(encoded), err)
			}
		})
	}
}

// TestModelInputCaptureOmissionsAndLaterSmallCall checks all omission reasons,
// omitted-content exclusion, and retention of a subsequent small call.
func TestModelInputCaptureOmissionsAndLaterSmallCall(t *testing.T) {
	ctx, capture := WithModelInputCapture(t.Context())
	captureModelInput(ctx, "image", []Message{{Role: "user", Content: []any{
		map[string]any{"type": "text", "text": "visible text"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,private"}},
	}}})
	captureModelInput(ctx, "large", []Message{{Role: "user", Content: strings.Repeat("x", maxModelInputBytes+1)}})
	captureModelInput(ctx, "invalid", []Message{{Role: "assistant", Content: "", ToolCalls: []map[string]any{{"bad": json.Number("invalid")}}}})
	captureModelInput(ctx, "small", []Message{{Role: "user", Content: "small"}})
	result := capture.Finalize()
	if len(result.Calls) != 4 || result.OmittedCount != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for i, reason := range []string{"unsupported_content", "size_limit", "serialization_error"} {
		if call := result.Calls[i]; call.Status != "omitted" || call.Reason != reason || len(call.Messages) != 0 {
			t.Fatalf("call %d = %+v, want omission %s", i, call, reason)
		}
	}
	if result.Calls[3].Status != "captured" {
		t.Fatalf("later small call was lost: %+v", result.Calls[3])
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "visible text") {
		t.Fatalf("omitted call leaked content: %s (%v)", encoded, err)
	}
}

// TestModelInputCaptureBoundsWholeResult checks serialized size and call limits,
// including escaped text, oversized model names, and omitted-call accounting.
func TestModelInputCaptureBoundsWholeResult(t *testing.T) {
	ctx, capture := WithModelInputCapture(t.Context())
	// Escaping expands '<' to six bytes; the serialized budget must catch it.
	captureModelInput(ctx, "escaped", []Message{{Role: "user", Content: strings.Repeat("<", maxModelInputBytes/4)}})
	captureModelInput(ctx, strings.Repeat("m", maxModelInputBytes+1), []Message{{Role: "user", Content: "small"}})
	for i := 0; i < 40; i++ {
		captureModelInput(ctx, fmt.Sprintf("model-%d", i), []Message{{Role: "user", Content: strings.Repeat("x", 45000)}})
	}
	result := capture.Finalize()
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxModelInputBytes || len(result.Calls) > maxModelInputCalls {
		t.Fatalf("unbounded result: bytes=%d calls=%d err=%v", len(encoded), len(result.Calls), err)
	}
	captured := uint64(0)
	for _, call := range result.Calls {
		if call.Status == "captured" {
			captured++
		}
	}
	if captured+result.OmittedCount != 42 {
		t.Fatalf("omitted count excludes placeholders or overflow: captured=%d result=%+v", captured, result)
	}
}

// TestModelInputCaptureCountAndRequestIsolation checks concurrent call counting,
// sequence ordering, and separation between request-local collectors.
func TestModelInputCaptureCountAndRequestIsolation(t *testing.T) {
	ctxA, captureA := WithModelInputCapture(t.Context())
	ctxB, captureB := WithModelInputCapture(t.Context())
	var workers sync.WaitGroup
	for i := 0; i < 80; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			captureModelInput(ctxA, "A", []Message{{Role: "user", Content: "only A"}})
			captureModelInput(ctxB, "B", []Message{{Role: "user", Content: "only B"}})
		}()
	}
	workers.Wait()
	for name, result := range map[string]ModelInputs{"A": captureA.Finalize(), "B": captureB.Finalize()} {
		if len(result.Calls) != 32 || result.OmittedCount != 48 {
			t.Fatalf("%s: calls=%d omitted=%d", name, len(result.Calls), result.OmittedCount)
		}
		for i, call := range result.Calls {
			if call.Model != name || call.Sequence != uint64(i+1) || !strings.Contains(string(call.Messages), "only "+name) {
				t.Fatalf("request isolation/order failure: %+v", call)
			}
		}
	}
}

type modelInputDriver struct {
	*captureToolDriver
	invoke func([]Message) error
	calls  int
}

// ChatWithMessages counts the stub invocation, runs its optional hook, and
// returns a fixed answer when the hook succeeds.
func (d *modelInputDriver) ChatWithMessages(_ context.Context, _ string, messages []Message, _ *APIConfig, _ *ChatConfig, _ *common.ModelUsage) (*ChatResponse, error) {
	d.calls++
	if d.invoke != nil {
		if err := d.invoke(messages); err != nil {
			return nil, err
		}
	}
	answer := "ok"
	return &ChatResponse{Answer: &answer}, nil
}

// ChatStreamlyWithSender counts the stub invocation, runs its optional hook,
// and sends a fixed answer when the hook succeeds.
func (d *modelInputDriver) ChatStreamlyWithSender(_ context.Context, _ string, messages []Message, _ *APIConfig, _ *ChatConfig, _ *common.ModelUsage, sender func(*string, *string) error) error {
	d.calls++
	if d.invoke != nil {
		if err := d.invoke(messages); err != nil {
			return err
		}
	}
	answer := "ok"
	return sender(&answer, nil)
}

// consumeModelInputStream drains and closes a stream, returning any non-EOF error.
func consumeModelInputStream(t *testing.T, stream *schema.StreamReader[*schema.Message]) error {
	t.Helper()
	defer stream.Close()
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// TestModelInputCaptureDriverAttemptsAndFailure checks model names and immutable
// snapshots for failed and successful attempts in both generation paths.
func TestModelInputCaptureDriverAttemptsAndFailure(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", streaming), func(t *testing.T) {
			ctx, capture := WithModelInputCapture(t.Context())
			first, second := "primary", "fallback"
			failed := &modelInputDriver{captureToolDriver: &captureToolDriver{}, invoke: func([]Message) error { return errors.New("unavailable") }}
			succeeded := &modelInputDriver{captureToolDriver: &captureToolDriver{}, invoke: func(messages []Message) error {
				messages[0].Content = "provider mutation"
				return nil
			}}
			model, err := NewFailoverEinoChatModel([]*ChatModel{
				NewChatModel(failed, &first, &APIConfig{}),
				NewChatModel(succeeded, &second, &APIConfig{}),
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			messages := []*schema.Message{schema.SystemMessage("original")}
			if streaming {
				stream, err := model.Stream(ctx, messages)
				if err != nil {
					t.Fatal(err)
				}
				if err := consumeModelInputStream(t, stream); err != nil {
					t.Fatal(err)
				}
			} else if _, err := model.Generate(ctx, messages); err != nil {
				t.Fatal(err)
			}
			result := capture.Finalize()
			if failed.calls != 1 || succeeded.calls != 1 || len(result.Calls) != 2 {
				t.Fatalf("attempts do not match driver calls: failed=%d succeeded=%d result=%+v", failed.calls, succeeded.calls, result)
			}
			for i, name := range []string{first, second} {
				if call := result.Calls[i]; call.Model != name || call.Sequence != uint64(i+1) || !strings.Contains(string(call.Messages), "original") {
					t.Fatalf("wrong attempt snapshot: %+v", call)
				}
			}
		})
	}
}

// TestModelInputCaptureToolStreamDelegatesOnce checks that Stream's delegation
// to Generate records exactly one provider invocation.
func TestModelInputCaptureToolStreamDelegatesOnce(t *testing.T) {
	ctx, capture := WithModelInputCapture(t.Context())
	name := "tool-model"
	driver := &modelInputDriver{captureToolDriver: &captureToolDriver{}}
	model := NewEinoChatModel(NewChatModel(driver, &name, &APIConfig{}), nil)
	bound, err := model.WithTools([]*schema.ToolInfo{{Name: "lookup"}})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := bound.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	if err := consumeModelInputStream(t, stream); err != nil {
		t.Fatal(err)
	}
	if result := capture.Finalize(); driver.calls != 1 || len(result.Calls) != 1 {
		t.Fatalf("Stream-to-Generate duplicated capture: driver=%d result=%+v", driver.calls, result)
	}
}

// TestModelInputOmissionDoesNotPreventProviderCall checks that unsupported
// diagnostic content does not prevent the provider from receiving the call.
func TestModelInputOmissionDoesNotPreventProviderCall(t *testing.T) {
	ctx, capture := WithModelInputCapture(t.Context())
	name := "vision"
	driver := &modelInputDriver{captureToolDriver: &captureToolDriver{}}
	model := NewEinoChatModel(NewChatModel(driver, &name, &APIConfig{}), nil)
	url := "data:image/png;base64,private"
	_, err := model.Generate(ctx, []*schema.Message{{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{{
		Type:  schema.ChatMessagePartTypeImageURL,
		Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{URL: &url}},
	}}}})
	if err != nil || driver.calls != 1 {
		t.Fatalf("capture affected provider execution: calls=%d err=%v", driver.calls, err)
	}
	if result := capture.Finalize(); len(result.Calls) != 1 || result.Calls[0].Reason != "unsupported_content" {
		t.Fatalf("unexpected capture: %+v", result)
	}
}
