package models

import (
	"context"
	"encoding/json"
	"sync"
)

const (
	maxModelInputCalls = 32
	maxModelInputBytes = 1 << 20
	// Reserve the result envelope, commas, and the largest uint64 summary.
	modelInputSummaryReserve = 128 + maxModelInputCalls
	// Keep space for ordinary omission markers after a large captured call.
	modelInputOmissionReserve = 1024
)

type modelInputCaptureKey struct{}

// ModelInputCall describes one attempted provider-driver invocation. Captured
// messages are the normalized input, before provider-specific serialization.
type ModelInputCall struct {
	Sequence uint64          `json:"sequence"`
	Model    string          `json:"model"`
	Status   string          `json:"status"`
	Messages json.RawMessage `json:"messages,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

// ModelInputs is a bounded, request-local diagnostic result.
type ModelInputs struct {
	Calls        []ModelInputCall `json:"calls"`
	OmittedCount uint64           `json:"omitted_count"`
}

type modelInputMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []map[string]any `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// ModelInputCapture owns its JSON snapshots and never retains provider objects.
// It is enabled only by the single-component debug endpoint.
type ModelInputCapture struct {
	mu       sync.Mutex
	sealed   bool
	sequence uint64
	bytes    int
	result   ModelInputs
}

// WithModelInputCapture creates an isolated collector for this request.
func WithModelInputCapture(ctx context.Context) (context.Context, *ModelInputCapture) {
	capture := &ModelInputCapture{result: ModelInputs{Calls: []ModelInputCall{}}}
	return context.WithValue(ctx, modelInputCaptureKey{}, capture), capture
}

// Finalize seals the collector. Work completing after the response is assembled
// cannot mutate its result or retain more messages.
func (c *ModelInputCapture) Finalize() ModelInputs {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sealed = true
	result := ModelInputs{Calls: make([]ModelInputCall, len(c.result.Calls)), OmittedCount: c.result.OmittedCount}
	for i, call := range c.result.Calls {
		call.Messages = append(json.RawMessage(nil), call.Messages...)
		result.Calls[i] = call
	}
	return result
}

// captureModelInput records an attempt only when the context has a collector.
func captureModelInput(ctx context.Context, model string, messages []Message) {
	capture, _ := ctx.Value(modelInputCaptureKey{}).(*ModelInputCapture)
	if capture == nil {
		return
	}
	capture.record(model, messages)
}

// record serializes one attempt within the count and byte limits, retaining
// omission reasons when space permits and ignoring calls after finalization.
func (c *ModelInputCapture) record(model string, messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return
	}
	c.sequence++
	if c.sequence > maxModelInputCalls {
		c.result.OmittedCount++
		return
	}
	// Reserve 1 KiB for each remaining attempt, so ordinary model identifiers
	// can still retain an omission reason when captured content fills its budget.
	futureMarkers := (maxModelInputCalls - int(c.sequence)) * modelInputOmissionReserve
	remaining := maxModelInputBytes - modelInputSummaryReserve - c.bytes - futureMarkers
	call := ModelInputCall{Sequence: c.sequence, Model: model, Status: "captured"}
	call.Messages, call.Reason = snapshotModelInput(messages, remaining-len(model))
	if call.Reason == "" {
		encoded, err := json.Marshal(call)
		if err == nil && len(encoded) <= remaining {
			c.bytes += len(encoded)
			c.result.Calls = append(c.result.Calls, call)
			return
		}
		call.Reason = "size_limit"
	}
	c.result.OmittedCount++
	call.Status, call.Messages = "omitted", nil
	// An exceptionally long model identifier may exceed its remaining budget.
	// Keep only the count, preserving the reserved space for subsequent markers.
	if len(model) > remaining {
		return
	}
	encoded, err := json.Marshal(call)
	if err == nil && len(encoded) <= remaining {
		c.bytes += len(encoded)
		c.result.Calls = append(c.result.Calls, call)
	}
}

// snapshotModelInput copies the allowed text-message fields into owned JSON or
// returns an omission reason without propagating serialization failures.
func snapshotModelInput(messages []Message, budget int) (snapshot json.RawMessage, reason string) {
	// Diagnostic serialization must never prevent a model call.
	defer func() {
		if recover() != nil {
			snapshot, reason = nil, "serialization_error"
		}
	}()
	if budget <= 0 || len(messages) > budget/16 {
		return nil, "size_limit"
	}
	remaining := budget
	retained := make([]modelInputMessage, 0, len(messages))
	for _, message := range messages {
		if parts, ok := message.Content.([]any); ok && len(parts) > remaining/2 {
			return nil, "size_limit"
		}
		if !modelInputTextContent(message.Content) || message.Audio != nil {
			return nil, "unsupported_content"
		}
		for _, value := range []any{message.Role, message.Content, message.ToolCallID, message.ToolCalls} {
			if reason := preflightModelInput(value, &remaining, 0); reason != "" {
				return nil, reason
			}
		}
		retained = append(retained, modelInputMessage{
			Role: message.Role, Content: message.Content,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		})
	}
	snapshot, err := json.Marshal(retained)
	if err != nil {
		return nil, "serialization_error"
	}
	if len(snapshot) > budget {
		return nil, "size_limit"
	}
	return snapshot, ""
}

// modelInputTextContent accepts strings, nil, and lists of text-only blocks.
func modelInputTextContent(content any) bool {
	switch value := content.(type) {
	case string, nil:
		return true
	case []any:
		for _, part := range value {
			block, ok := part.(map[string]any)
			if !ok || len(block) != 2 || block["type"] != "text" {
				return false
			}
			if _, ok := block["text"].(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// preflightModelInput bounds traversal and string sizes before allocating JSON.
// It accepts only plain JSON values produced by message normalization.
func preflightModelInput(value any, budget *int, depth int) string {
	if depth > 32 {
		return "serialization_error"
	}
	*budget -= 2
	if *budget < 0 {
		return "size_limit"
	}
	switch value := value.(type) {
	case nil, bool, float64, int, int64:
		return ""
	case json.Number:
		*budget -= len(value)
	case string:
		*budget -= len(value)
	case []any:
		for _, child := range value {
			if reason := preflightModelInput(child, budget, depth+1); reason != "" {
				return reason
			}
		}
	case []map[string]any:
		for _, child := range value {
			if reason := preflightModelInput(child, budget, depth+1); reason != "" {
				return reason
			}
		}
	case map[string]any:
		for key, child := range value {
			*budget -= len(key)
			if reason := preflightModelInput(child, budget, depth+1); reason != "" {
				return reason
			}
		}
	default:
		return "serialization_error"
	}
	if *budget < 0 {
		return "size_limit"
	}
	return ""
}
