// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	"ragflow/internal/agent/chat"
)

func TestStructuredOutputDisabled(t *testing.T) {
	for _, raw := range []any{nil, map[string]any(nil), map[string]any{}, map[string]any{"value": map[string]any{"old": true}}} {
		got, err := newStructuredOutput(raw)
		if err != nil || got != nil {
			t.Fatalf("newStructuredOutput(%v) = %v, %v", raw, got, err)
		}
	}
}

func TestStructuredOutputRejectsInvalidSchema(t *testing.T) {
	cases := map[string]any{
		"boolean root":            false,
		"string root":             "object",
		"array root":              map[string]any{"type": "array"},
		"union root":              map[string]any{"type": []any{"object", "null"}},
		"invalid property type":   map[string]any{"properties": map[string]any{"age": map[string]any{"type": "invalid"}}},
		"negative constraint":     map[string]any{"properties": map[string]any{"name": map[string]any{"minLength": -1}}},
		"duplicate required":      map[string]any{"required": []any{"name", "name"}},
		"invalid required":        map[string]any{"required": "name"},
		"invalid property schema": map[string]any{"properties": map[string]any{"name": 42}},
		"unsupported draft":       map[string]any{"$schema": "https://example.com/custom-schema"},
		"invalid draft":           map[string]any{"$schema": 42},
		"external HTTPS ref":      map[string]any{"$ref": "https://example.com/remote.json"},
		"external file ref":       map[string]any{"$ref": "file:///tmp/private.json"},
		"missing local ref":       map[string]any{"$ref": "#/$defs/missing"},
		"invalid regexp":          map[string]any{"properties": map[string]any{"name": map[string]any{"pattern": "["}}},
		"unencodable schema":      map[string]any{"enum": make(chan int)},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := newStructuredOutput(raw)
			if err == nil || got != nil {
				t.Fatalf("accepted invalid schema: %v, %v", got, err)
			}
		})
	}
}

func TestStructuredOutputDrafts(t *testing.T) {
	for _, draft := range []string{
		"http://json-schema.org/draft-04/schema#",
		"http://json-schema.org/draft-06/schema#",
		"https://json-schema.org/draft-07/schema#",
		"https://json-schema.org/draft/2019-09/schema",
		"https://json-schema.org/draft/2020-12/schema",
	} {
		t.Run(draft, func(t *testing.T) {
			validator, err := newStructuredOutput(map[string]any{"$schema": draft, "required": []any{"name"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := validator.parse(`{"name":"Alice"}`); err != nil {
				t.Fatal(err)
			}
			if _, err := validator.parse(`{}`); err == nil {
				t.Fatal("missing required property accepted")
			}
		})
	}
}

func TestStructuredOutputConstraints(t *testing.T) {
	definition := map[string]any{
		"properties": map[string]any{
			"person": map[string]any{
				"$ref": "#/$defs/person",
			},
			"items": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		},
		"required":             []any{"person", "items"},
		"additionalProperties": false,
		"$defs": map[string]any{
			"person": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"age":  map[string]any{"type": "integer", "minimum": 0},
					"role": map[string]any{"enum": []any{"user", "admin"}},
				},
				"required": []any{"age", "role"},
			},
		},
		"value": map[string]any{"previous": "secret"},
	}
	before, _ := json.Marshal(definition)
	validator, err := newStructuredOutput(definition)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(definition)
	if string(before) != string(after) {
		t.Fatal("schema input mutated")
	}
	if strings.Contains(validator.prompt(), "secret") || !strings.Contains(validator.prompt(), `"type":"object"`) {
		t.Fatalf("unexpected prompt: %s", validator.prompt())
	}
	for _, input := range []string{
		`{"person":{"age":12,"role":"user"},"items":[1,2]}`,
	} {
		if _, err := validator.parse(input); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{
		`{"person":{"age":"12","role":"user"},"items":[1,2]}`,
		`{"person":{"age":-1,"role":"user"},"items":[1,2]}`,
		`{"person":{"age":1,"role":"guest"},"items":[1,2]}`,
		`{"person":{"age":1,"role":"user"},"items":["1"]}`,
		`{"person":{"age":1,"role":"user"},"items":[],"extra":true}`,
		`{"person":{"age":1},"items":[]}`,
		`{"items":[]}`,
		`{"person":{"age":NaN,"role":"user"},"items":[]}`,
		`{"person":{"age":Infinity,"role":"user"},"items":[]}`,
		`{} {}`,
		`[]`, `null`, `"text"`,
	} {
		if _, err := validator.parse(input); err == nil {
			t.Errorf("accepted invalid output: %s", input)
		}
	}
}

func TestStructuredOutputSchemaWithoutProperties(t *testing.T) {
	for _, raw := range []map[string]any{
		{"type": "object", "additionalProperties": false},
		{"type": "object", "maxProperties": 0},
	} {
		validator, err := newStructuredOutput(raw)
		if err != nil || validator == nil {
			t.Fatalf("schema not enabled: %v", err)
		}
		if _, err := validator.parse(`{}`); err != nil {
			t.Fatal(err)
		}
		if _, err := validator.parse(`{"extra":1}`); err == nil {
			t.Fatal("constraint without properties was ignored")
		}
	}
}

func TestStructuredOutputFrom(t *testing.T) {
	cases := []struct {
		name    string
		params  map[string]any
		want    any
		present bool
	}{
		{"absent", nil, nil, false},
		{"flat schema", map[string]any{"output_structure": map[string]any{"type": "object"}}, map[string]any{"type": "object"}, true},
		{"saved schema wins", map[string]any{"outputs": map[string]any{"structured": map[string]any{"maxProperties": 0}}, "output_structure": map[string]any{"type": "array"}}, map[string]any{"maxProperties": 0}, true},
		{"explicit disable wins", map[string]any{"outputs": map[string]any{"structured": nil}, "output_structure": map[string]any{"type": "object"}}, nil, true},
		{"invalid schema retained", map[string]any{"outputs": map[string]any{"structured": false}}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, present := structuredOutputFrom(tc.params)
			if present != tc.present || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got (%v,%v), want (%v,%v)", got, present, tc.want, tc.present)
			}
		})
	}
}

type structuredRepairInvoker func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error)

func (f structuredRepairInvoker) Invoke(ctx context.Context, _ *gorm.DB, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	return f(ctx, req)
}

func newStructuredTestValidator(t *testing.T) *structuredOutput {
	t.Helper()
	validator, err := newStructuredOutput(map[string]any{
		"properties": map[string]any{"count": map[string]any{"type": "integer"}},
		"required":   []any{"count"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

func TestStructuredOutputRepair(t *testing.T) {
	validator := newStructuredTestValidator(t)
	history := make([]schema.Message, 1, 8)
	history[0] = schema.Message{Role: schema.System, Content: "original system prompt"}
	req := ChatInvokeRequest{Messages: history, Tools: []chat.Tool{{Type: "function"}}, ToolChoice: chat.ToolChoiceRequired, ModelName: "fake"}
	initial := &ChatInvokeResponse{Content: `{"count":"wrong"}`}
	calls := 0
	inv := structuredRepairInvoker(func(_ context.Context, got ChatInvokeRequest) (*ChatInvokeResponse, error) {
		calls++
		if len(got.Tools) != 0 || got.ToolChoice != chat.ToolChoiceNone || got.ModelName != "fake" {
			t.Fatalf("unsafe correction request: %+v", got)
		}
		if len(got.Messages) != 3 || got.Messages[1].Role != schema.Assistant || got.Messages[1].Content != initial.Content {
			t.Fatalf("previous answer missing from correction: %+v", got.Messages)
		}
		if !strings.Contains(got.Messages[2].Content, "count") || !strings.Contains(got.Messages[2].Content, "integer") {
			t.Fatalf("precise validation error missing: %s", got.Messages[2].Content)
		}
		return &ChatInvokeResponse{Content: "```json\n{\"count\":2}\n```", Model: "fixed", Tokens: 7}, nil
	})
	resp, parsed, err := validator.validateAndRepair(t.Context(), nil, inv, req, initial, 1)
	if err != nil || calls != 1 || parsed["count"] != float64(2) || resp.Content != `{"count":2}` || resp.Model != "fixed" || resp.Tokens != 7 {
		t.Fatalf("unexpected repair: resp=%+v parsed=%v calls=%d err=%v", resp, parsed, calls, err)
	}
	if initial.Content != `{"count":"wrong"}` || len(req.Tools) != 1 || req.ToolChoice != chat.ToolChoiceRequired || history[:cap(history)][1].Content != "" {
		t.Fatal("caller request or response mutated")
	}
}

func TestStructuredOutputRepairBudget(t *testing.T) {
	validator := newStructuredTestValidator(t)
	for _, retries := range []int{0, 1, 3} {
		calls := 0
		inv := structuredRepairInvoker(func(_ context.Context, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
			calls++
			return &ChatInvokeResponse{Content: `{"count":"wrong"}`}, nil
		})
		resp, parsed, err := validator.validateAndRepair(t.Context(), nil, inv, ChatInvokeRequest{}, &ChatInvokeResponse{Content: `{}`}, retries)
		if err == nil || resp != nil || parsed != nil || calls != retries {
			t.Fatalf("retries=%d response=%v parsed=%v calls=%d err=%v", retries, resp, parsed, calls, err)
		}
	}
}

func TestStructuredOutputRepairFailures(t *testing.T) {
	validator := newStructuredTestValidator(t)
	providerErr := errors.New("provider unavailable")
	cases := []struct {
		name      string
		response  *ChatInvokeResponse
		inv       ChatInvoker
		retries   int
		wantError string
	}{
		{"negative budget", &ChatInvokeResponse{Content: `{}`}, nil, -1, "nonnegative"},
		{"nil initial response", nil, nil, 1, "nil model response"},
		{"initial tool call", &ChatInvokeResponse{Content: `{"count":1}`, ToolCalls: []chat.ToolCall{{Name: "write"}}}, nil, 1, "tool calls"},
		{"missing invoker", &ChatInvokeResponse{Content: `{}`}, nil, 1, "requires a chat invoker"},
		{"provider error", &ChatInvokeResponse{Content: `{}`}, structuredRepairInvoker(func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error) { return nil, providerErr }), 1, "provider unavailable"},
		{"nil correction response", &ChatInvokeResponse{Content: `{}`}, structuredRepairInvoker(func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error) { return nil, nil }), 1, "nil model response"},
		{"correction tool call", &ChatInvokeResponse{Content: `{}`}, structuredRepairInvoker(func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error) {
			return &ChatInvokeResponse{ToolCalls: []chat.ToolCall{{Name: "write"}}}, nil
		}), 1, "tool calls"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, parsed, err := validator.validateAndRepair(t.Context(), nil, tc.inv, ChatInvokeRequest{}, tc.response, tc.retries)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) || resp != nil || parsed != nil {
				t.Fatalf("got (%v,%v,%v), want error containing %s", resp, parsed, err, tc.wantError)
			}
			if tc.name == "provider error" && !errors.Is(err, providerErr) {
				t.Fatal("provider error identity lost")
			}
		})
	}
}

func TestStructuredOutputRepairDoesNotRetryProviderError(t *testing.T) {
	validator := newStructuredTestValidator(t)
	calls := 0
	inv := newRetryInvoker(structuredRepairInvoker(func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error) {
		calls++
		return nil, errors.New("provider unavailable")
	}), 3, 0)
	_, _, err := validator.validateAndRepair(t.Context(), nil, inv, ChatInvokeRequest{}, &ChatInvokeResponse{Content: `{}`}, 3)
	if err == nil || calls != 1 {
		t.Fatalf("provider correction retries multiplied: calls=%d err=%v", calls, err)
	}
}

func TestStructuredOutputRepairCancellation(t *testing.T) {
	validator := newStructuredTestValidator(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := validator.validateAndRepair(ctx, nil, nil, ChatInvokeRequest{}, &ChatInvokeResponse{Content: `{"count":1}`}, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled output published: %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	inv := structuredRepairInvoker(func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error) {
		cancel()
		return &ChatInvokeResponse{Content: `{"count":1}`}, nil
	})
	resp, parsed, err := validator.validateAndRepair(ctx, nil, inv, ChatInvokeRequest{}, &ChatInvokeResponse{Content: `{}`}, 1)
	if !errors.Is(err, context.Canceled) || resp != nil || parsed != nil {
		t.Fatalf("canceled correction published: %v, %v, %v", resp, parsed, err)
	}
}

func TestStructuredOutputValidWithoutCorrection(t *testing.T) {
	validator := newStructuredTestValidator(t)
	initial := &ChatInvokeResponse{Content: "<think>private</think>```json\n{\"count\":1}\n```"}
	resp, parsed, err := validator.validateAndRepair(t.Context(), nil, nil, ChatInvokeRequest{}, initial, 0)
	if err != nil || parsed["count"] != float64(1) || resp.Content != `{"count":1}` || initial.Content == resp.Content {
		t.Fatalf("valid initial answer failed: %v, %v, %v", resp, parsed, err)
	}
}

func TestStructuredOutputPreservesNumberPrecision(t *testing.T) {
	validator, err := newStructuredOutput(map[string]any{
		"properties": map[string]any{"count": map[string]any{"const": json.Number("9007199254740992")}},
		"required":   []any{"count"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.parse(`{"count":9007199254740993}`); err == nil {
		t.Fatal("rounded a distinct integer into the allowed value")
	}
	parsed, err := validator.parse(`{"count":9007199254740992}`)
	if err != nil || parsed["count"] != float64(9007199254740992) {
		t.Fatalf("number precision lost: %v, %v", parsed, err)
	}
}

func TestStructuredOutputRejectsLossyRuntimeNumbers(t *testing.T) {
	validator, err := newStructuredOutput(map[string]any{"type": "object"})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		`{"count":9007199254740993}`,
		`{"nested":{"count":9007199254740993}}`,
		`{"items":[9007199254740993]}`,
		`{"count":1e309}`,
		`{"count":1e-1000}`,
		`{"count":0.1234567890123456789}`,
	} {
		if parsed, err := validator.parse(content); err == nil || parsed != nil {
			t.Fatalf("accepted a lossy runtime number: %s => %v, %v", content, parsed, err)
		}
	}
	parsed, err := validator.parse(`{"items":[2,10],"nested":{"ratio":0.1},"text":"2","flag":true,"empty":null}`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"items":  []any{float64(2), float64(10)},
		"nested": map[string]any{"ratio": float64(0.1)},
		"text":   "2", "flag": true, "empty": nil,
	}
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("runtime number contract changed: got %v, want %v", parsed, want)
	}
}

func TestStructuredOutputPreservesMarkersInJSONStrings(t *testing.T) {
	validator, err := newStructuredOutput(map[string]any{
		"properties": map[string]any{"text": map[string]any{"type": "string"}},
		"required":   []any{"text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"keep </think> as text", "keep ```json as text", "</think>```json"} {
		content, err := json.Marshal(map[string]any{"text": value})
		if err != nil {
			t.Fatal(err)
		}
		resp, parsed, err := validator.validateAndRepair(t.Context(), nil, nil, ChatInvokeRequest{}, &ChatInvokeResponse{Content: string(content)}, 0)
		if err != nil || parsed["text"] != value || resp.Content != string(content) {
			t.Fatalf("JSON marker value changed: %v, %v, %v", resp, parsed, err)
		}
	}
}
