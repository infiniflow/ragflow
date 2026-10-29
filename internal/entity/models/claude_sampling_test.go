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
	"encoding/json"
	"io"
	"net/http"
	"ragflow/internal/common"
	"testing"
)

func TestClaudeSamplingPolicy(t *testing.T) {
	for _, tc := range []struct {
		name            string
		model           string
		temperature     *float64
		topP            *float64
		wantTemperature bool
		wantTopP        bool
	}{
		{"bedrock sonnet 4.6 keeps temperature only", "eu.anthropic.claude-sonnet-4-6", f64(0.7), f64(0.9), true, false},
		{"top_p alone is kept", "eu.anthropic.claude-sonnet-4-6", nil, f64(0.9), false, true},
		{"temperature alone is kept", "claude-sonnet-4-6", f64(0.7), nil, true, false},
		{"opus 4.8 accepts no sampling", "claude-opus-4-8", f64(0.7), f64(0.9), false, false},
		{"bedrock opus 4.7 accepts no sampling", "anthropic.claude-opus-4-7-v1:0", f64(0.7), nil, false, false},
		{"haiku 5.5 accepts no sampling", "claude-haiku-5-5", f64(0.7), f64(0.9), false, false},
		{"sonnet 5 accepts no sampling", "claude-sonnet-5", f64(0.7), f64(0.9), false, false},
		{"sonnet 4.5 is not sonnet 5", "claude-sonnet-4-5-20250929", f64(0.7), f64(0.9), true, false},
		{"opus 4.1 rejects the pair", "anthropic.claude-opus-4-1-20250805-v1:0", f64(0.7), f64(0.9), true, false},
		{"claude 4.0 accepts the pair", "claude-sonnet-4-20250514", f64(0.7), f64(0.9), true, true},
		{"claude 3.5 accepts the pair", "anthropic.claude-3-5-sonnet-20241022-v2:0", f64(0.7), f64(0.9), true, true},
		{"unparsable claude name counts as recent", "claude-custom", f64(0.7), f64(0.9), true, false},
		{"non-claude model is untouched", "meta.llama3-70b-instruct-v1:0", f64(0.7), f64(0.9), true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &ChatConfig{Temperature: tc.temperature, TopP: tc.topP}

			got := withClaudeSamplingPolicy(tc.model, cfg)

			if (got.Temperature != nil) != tc.wantTemperature {
				t.Errorf("temperature=%v, want present=%v", got.Temperature, tc.wantTemperature)
			}
			if (got.TopP != nil) != tc.wantTopP {
				t.Errorf("top_p=%v, want present=%v", got.TopP, tc.wantTopP)
			}
			if cfg.Temperature != tc.temperature || cfg.TopP != tc.topP {
				t.Errorf("the caller's config was modified: %+v", cfg)
			}
		})
	}
}

func TestClaudeSamplingPolicyNilConfig(t *testing.T) {
	if got := withClaudeSamplingPolicy("claude-sonnet-4-6", nil); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestAnthropicChatDropsTopPWhenTemperatureIsSet(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		model           string
		wantTemperature bool
		wantTopP        bool
	}{
		{"claude-sonnet-4-6", true, false},
		{"claude-opus-4-8", false, false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
				if _, ok := body["temperature"]; ok != tc.wantTemperature {
					t.Errorf("temperature present=%v, want %v (body=%v)", ok, tc.wantTemperature, body)
				}
				if _, ok := body["top_p"]; ok != tc.wantTopP {
					t.Errorf("top_p present=%v, want %v (body=%v)", ok, tc.wantTopP, body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
			})
			defer srv.Close()

			apiKey := "test-key"
			_, err := newAnthropicForTest(srv.URL).ChatWithMessages(
				t.Context(),
				tc.model,
				[]Message{{Role: "user", Content: "hi"}},
				&APIConfig{ApiKey: &apiKey},
				&ChatConfig{Temperature: f64(0.5), TopP: f64(0.9)},
				&common.ModelUsage{},
			)
			if err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
		})
	}
}

func f64(v float64) *float64 { return &v }

func TestBedrockChatDropsTopPForClaudeWhenTemperatureIsSet(t *testing.T) {
	withSSRFBypass(t)
	const modelID = "eu.anthropic.claude-sonnet-4-6"
	srv := newBedrockServer(t, http.MethodPost, "/model/"+modelID+"/converse",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var body bedrockConverseRequest
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal body: %v", err)
				return
			}
			inf := body.InferenceConfig
			if inf == nil || inf.Temperature == nil || *inf.Temperature != 0.5 {
				t.Errorf("inferenceConfig=%+v, want temperature 0.5", inf)
			} else if inf.TopP != nil {
				t.Errorf("topP=%v, want it dropped next to temperature", *inf.TopP)
			}
			resp := bedrockConverseResponse{}
			resp.Output.Message.Role = "assistant"
			resp.Output.Message.Content = []bedrockContentBlock{{Text: "pong"}}
			resp.StopReason = "end_turn"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})
	defer srv.Close()

	key := validBedrockKey()
	_, err := newBedrockForTest(srv.URL).ChatWithMessages(t.Context(), modelID,
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &key}, &ChatConfig{Temperature: f64(0.5), TopP: f64(0.9)}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}
