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

package service

import (
	"context"
	"encoding/json"
	"testing"

	modelModule "ragflow/internal/entity/models"
)

// The per-turn cost footer in the chat UI reads these keys off the final SSE
// chunk. A renamed field does not fail loudly — the footer simply never appears,
// which is indistinguishable from "the turn reported nothing" — so the wire
// names are pinned here rather than left to the struct tags alone.
func TestAsyncChatResultCostWireKeys(t *testing.T) {
	result := AsyncChatResult{
		Answer: "an answer",
		Usage: &ChatTurnUsage{
			PromptTokens:     1200,
			CompletionTokens: 340,
			TotalTokens:      1540,
		},
		ElapsedMs: 8421.5,
		Final:     true,
	}

	body, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	usage, ok := wire["usage"].(map[string]any)
	if !ok {
		t.Fatalf("no usage object in %s", body)
	}
	for key, want := range map[string]float64{
		"prompt_tokens":     1200,
		"completion_tokens": 340,
		"total_tokens":      1540,
	} {
		got, ok := usage[key].(float64)
		if !ok {
			t.Errorf("usage.%s missing from %s", key, body)
			continue
		}
		if got != want {
			t.Errorf("usage.%s = %v, want %v", key, got, want)
		}
	}

	if got, ok := wire["elapsed_ms"].(float64); !ok || got != 8421.5 {
		t.Errorf("elapsed_ms = %v, want 8421.5", wire["elapsed_ms"])
	}
}

// A turn that reported no cost must not emit the keys at all: an empty object
// would render a footer reading "0 tokens", which is a measurement, not an
// absence.
func TestAsyncChatResultOmitsCostWhenUnset(t *testing.T) {
	body, err := json.Marshal(AsyncChatResult{Answer: "an answer"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"usage", "elapsed_ms"} {
		if _, ok := wire[key]; ok {
			t.Errorf("%s present in %s, want omitted", key, body)
		}
	}
}

func TestDecorateAnswerPrefersRealUsage(t *testing.T) {
	s := &ChatPipelineService{}
	timer, prompt := newTimerAndPrompt()
	real := &modelModule.TokenUsage{
		PromptTokens:     150,
		CompletionTokens: 80,
		TotalTokens:      230,
	}

	res := s.decorateAnswer(
		context.Background(),
		"Test answer",
		nil,
		prompt,
		[]string{"hello"},
		50, // usedTokenCount (approximation should be superseded)
		timer,
		nil,
		0.3,
		false,
		nil,
		"",
		nil,
		"tenant-1",
		nil,
		false,
		real,
	)

	if res.Usage == nil {
		t.Fatal("res.Usage is nil")
	}
	if res.Usage.PromptTokens != 150 {
		t.Errorf("PromptTokens = %d, want 150", res.Usage.PromptTokens)
	}
	if res.Usage.CompletionTokens != 80 {
		t.Errorf("CompletionTokens = %d, want 80", res.Usage.CompletionTokens)
	}
	if res.Usage.TotalTokens != 230 {
		t.Errorf("TotalTokens = %d, want 230", res.Usage.TotalTokens)
	}
}

func TestDecorateAnswerFallsBackWhenNoRealUsage(t *testing.T) {
	s := &ChatPipelineService{}
	timer, prompt := newTimerAndPrompt()

	res := s.decorateAnswer(
		context.Background(),
		"Test answer",
		nil,
		prompt,
		[]string{"hello"},
		50, // usedTokenCount
		timer,
		nil,
		0.3,
		false,
		nil,
		"",
		nil,
		"tenant-1",
		nil,
		false,
		nil,
	)

	if res.Usage == nil {
		t.Fatal("res.Usage is nil")
	}
	if res.Usage.PromptTokens != 50 {
		t.Errorf("PromptTokens = %d, want 50", res.Usage.PromptTokens)
	}
	if res.Usage.CompletionTokens <= 0 {
		t.Errorf("CompletionTokens = %d, want > 0", res.Usage.CompletionTokens)
	}
	if res.Usage.TotalTokens != res.Usage.PromptTokens+res.Usage.CompletionTokens {
		t.Errorf("TotalTokens = %d, want %d", res.Usage.TotalTokens, res.Usage.PromptTokens+res.Usage.CompletionTokens)
	}
}

