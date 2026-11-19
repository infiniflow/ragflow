//go:build integration

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

package models

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNovaCodeLiveChat exercises the real configured NovaCode endpoint.
// The default is Responses; NOVACODE_WIRE_API=chat_completions selects chat.
func TestNovaCodeLiveChat(t *testing.T) {
	key := os.Getenv("NOVACODE_API_KEY")
	model := os.Getenv("NOVACODE_MODEL")
	if key == "" || model == "" {
		t.Skip("requires NOVACODE_API_KEY and NOVACODE_MODEL")
	}
	baseURL := os.Getenv("NOVACODE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://ai.novacode.top/v1"
	}
	protocol := os.Getenv("NOVACODE_WIRE_API")
	if protocol == "" {
		protocol = "responses"
	}
	credentials, _ := json.Marshal(novaCodeCredentials{APIKey: key, WireAPI: protocol, ReasoningEffort: "medium"})
	raw := string(credentials)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	driver := NewNovaCodeModel(map[string]string{"default": baseURL}, URLSuffix{})
	response, err := driver.ChatWithMessages(ctx, model, []Message{{Role: "user", Content: "Reply with exactly OK."}}, &APIConfig{ApiKey: &raw}, nil, nil)
	if err != nil {
		t.Fatalf("NovaCode %s request failed: %v", protocol, err)
	}
	if response == nil || response.Answer == nil || strings.TrimSpace(*response.Answer) == "" {
		t.Fatal("NovaCode returned no text")
	}
}

// TestNovaCodeLiveStream verifies that text deltas and the terminal event are
// consumed for the selected wire API, including any upstream reasoning events.
func TestNovaCodeLiveStream(t *testing.T) {
	key, model := os.Getenv("NOVACODE_API_KEY"), os.Getenv("NOVACODE_MODEL")
	if key == "" || model == "" {
		t.Skip("requires NOVACODE_API_KEY and NOVACODE_MODEL")
	}
	baseURL := os.Getenv("NOVACODE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://ai.novacode.top/v1"
	}
	protocol := os.Getenv("NOVACODE_WIRE_API")
	if protocol == "" {
		protocol = "responses"
	}
	credentials, _ := json.Marshal(novaCodeCredentials{APIKey: key, WireAPI: protocol, ReasoningEffort: "medium"})
	raw := string(credentials)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	driver := NewNovaCodeModel(map[string]string{"default": baseURL}, URLSuffix{})
	var content strings.Builder
	done := false
	cfg := &ChatConfig{}
	err := driver.ChatStreamlyWithSender(ctx, model, []Message{{Role: "user", Content: "Reply with exactly OK."}}, &APIConfig{ApiKey: &raw}, cfg, nil, func(text, reason *string) error {
		if text != nil {
			if *text == "[DONE]" {
				done = true
			} else {
				content.WriteString(*text)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NovaCode %s stream failed: %v", protocol, err)
	}
	if !done || strings.TrimSpace(content.String()) == "" {
		t.Fatal("NovaCode returned no text or terminal event")
	}
}

// TestNovaCodeLiveTools checks stateless Responses continuation with the
// provider's real encrypted reasoning items. It is an explicit paid opt-in.
func TestNovaCodeLiveTools(t *testing.T) {
	if os.Getenv("NOVACODE_LIVE_TOOLS") != "1" {
		t.Skip("requires NOVACODE_LIVE_TOOLS=1")
	}
	key, model := os.Getenv("NOVACODE_API_KEY"), os.Getenv("NOVACODE_MODEL")
	if key == "" || model == "" {
		t.Skip("requires NOVACODE_API_KEY and NOVACODE_MODEL")
	}
	baseURL := os.Getenv("NOVACODE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://ai.novacode.top/v1"
	}
	credentials, _ := json.Marshal(novaCodeCredentials{APIKey: key, WireAPI: "responses", ReasoningEffort: "medium"})
	raw := string(credentials)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	driver := NewNovaCodeModel(map[string]string{"default": baseURL}, URLSuffix{})
	api := &APIConfig{ApiKey: &raw}
	tools := []map[string]any{{"type": "function", "function": map[string]any{
		"name": "get_test_value", "description": "Return a fixed test value", "parameters": map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}, "additionalProperties": false}, "strict": true,
	}}}
	required := "required"
	history := []Message{{Role: "user", Content: "Call get_test_value once, then repeat only its returned value."}}
	response, err := driver.ChatWithMessages(ctx, model, history, api, &ChatConfig{Tools: tools, ToolChoice: &required}, nil)
	if err != nil {
		t.Fatalf("NovaCode tool request failed: %v", err)
	}
	if len(response.ToolCalls) == 0 || len(response.ProviderState) == 0 {
		t.Fatal("NovaCode returned no tool call or replay state")
	}
	history = append(history, Message{Role: "assistant", ToolCalls: response.ToolCalls, ProviderState: response.ProviderState})
	for _, call := range response.ToolCalls {
		id, _ := call["id"].(string)
		history = append(history, Message{Role: "tool", ToolCallID: id, Content: "NOVACODE_TOOL_OK"})
	}
	none := "none"
	response, err = driver.ChatWithMessages(ctx, model, history, api, &ChatConfig{Tools: tools, ToolChoice: &none}, nil)
	if err != nil {
		t.Fatalf("NovaCode tool continuation failed: %v", err)
	}
	if response.Answer == nil || !strings.Contains(*response.Answer, "NOVACODE_TOOL_OK") {
		t.Fatal("NovaCode did not return the tool result")
	}
}

// TestNovaCodeLiveVision verifies image input against the real Responses
// endpoint. The prompt does not disclose the generated image's colors.
func TestNovaCodeLiveVision(t *testing.T) {
	key := os.Getenv("NOVACODE_API_KEY")
	selectedModels := os.Getenv("NOVACODE_VISION_MODELS")
	if key == "" || strings.TrimSpace(selectedModels) == "" {
		t.Skip("requires NOVACODE_API_KEY and NOVACODE_VISION_MODELS")
	}
	baseURL := os.Getenv("NOVACODE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://ai.novacode.top/v1"
	}
	credentials, err := json.Marshal(novaCodeCredentials{APIKey: key, WireAPI: "responses", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal("status=configuration_failed")
	}
	raw := string(credentials)
	driver := NewNovaCodeModel(map[string]string{"default": baseURL}, URLSuffix{})

	fixture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			pixel := color.RGBA{G: 255, A: 255}
			if x >= 32 {
				pixel = color.RGBA{B: 255, A: 255}
			}
			fixture.SetRGBA(x, y, pixel)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, fixture); err != nil {
		t.Fatal("status=fixture_failed")
	}
	imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	messages := []Message{{Role: "user", Content: []map[string]any{
		{"type": "text", "text": `Inspect the attached image. Identify the main color in its left half and its right half. Reply only with a JSON object containing the keys "left" and "right", using one lowercase English color name for each value.`},
		{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
	}}}
	var modelNames []string
	for _, selected := range strings.Split(selectedModels, ",") {
		if model := strings.TrimSpace(selected); model != "" {
			modelNames = append(modelNames, model)
		}
	}
	if len(modelNames) == 0 {
		t.Skip("requires at least one model in NOVACODE_VISION_MODELS")
	}
	listed, err := driver.ListModels(t.Context(), &APIConfig{ApiKey: &raw})
	if err != nil {
		t.Fatal("status=model_discovery_failed")
	}
	listedTypes := map[string][]string{}
	for _, model := range listed {
		listedTypes[model.Name] = model.ModelTypes
	}
	for _, model := range modelNames {
		t.Run(model, func(t *testing.T) {
			if !containsModelType(listedTypes[model], "vision") {
				t.Fatalf("model=%s status=vision_metadata_missing", model)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			response, err := driver.ChatWithMessages(ctx, model, messages, &APIConfig{ApiKey: &raw}, nil, nil)
			if err != nil {
				var statusError *APIStatusError
				if errors.As(err, &statusError) {
					t.Fatalf("model=%s status=http_%d", model, statusError.Status)
				}
				t.Fatalf("model=%s status=request_failed", model)
			}
			if response == nil || response.Answer == nil {
				t.Fatalf("model=%s status=no_text", model)
			}
			var description struct {
				Left  string `json:"left"`
				Right string `json:"right"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(*response.Answer)), &description); err != nil {
				t.Fatalf("model=%s status=invalid_description", model)
			}
			if strings.ToLower(strings.TrimSpace(description.Left)) != "green" || strings.ToLower(strings.TrimSpace(description.Right)) != "blue" {
				t.Fatalf("model=%s status=incorrect_image_description left=%q right=%q", model, description.Left, description.Right)
			}
			t.Logf("model=%s status=vision_confirmed", model)
		})
	}
}
