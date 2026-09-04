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
	"fmt"
	"strings"
	"testing"

	modelModule "ragflow/internal/entity/models"
)

// ---- mocks ----

type fakeRetriever struct {
	result *RetrievalTestResponse
	err    error
}

func (r *fakeRetriever) RetrievalTest(ctx context.Context, req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.result, nil
}

type fakeStreamLLM struct {
	chunks []string
	err    error
}

func (f *fakeStreamLLM) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, <-chan error, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	ch := make(chan string, len(f.chunks)+1)
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	errCh := make(chan error, 1)
	close(errCh)
	return ch, errCh, nil
}

// capturingStreamLLM records the ChatConfig actually handed to the LLM.
type capturingStreamLLM struct {
	fakeStreamLLM
	gotConfig *modelModule.ChatConfig
}

func (c *capturingStreamLLM) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, <-chan error, error) {
	c.gotConfig = config
	return c.fakeStreamLLM.ChatStream(ctx, messages, config)
}

// asyncErrStreamLLM returns answer deltas, then reports an async driver error.
type asyncErrStreamLLM struct {
	fakeStreamLLM
}

func (a *asyncErrStreamLLM) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, <-chan error, error) {
	ch := make(chan string, 1)
	ch <- "partial"
	close(ch)
	errCh := make(chan error, 1)
	errCh <- fmt.Errorf("API request failed with status 400: invalid temperature")
	close(errCh)
	return ch, errCh, nil
}

// ---- AskService tests ----

func collect(deltas <-chan AskDelta) []AskDelta {
	var out []AskDelta
	for d := range deltas {
		out = append(out, d)
	}
	return out
}

func TestAskService_RetrievalError(t *testing.T) {
	ret := &fakeRetriever{err: fmt.Errorf("engine down")}
	llm := &fakeStreamLLM{chunks: []string{"answer"}}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))
	if len(deltas) < 1 || deltas[0].Kind != AskDeltaError {
		t.Fatalf("expected error delta, got %+v", deltas)
	}
}

func TestAskService_EmptyResult(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{Chunks: []map[string]interface{}{}}}
	llm := &fakeStreamLLM{chunks: []string{"answer"}}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))
	if len(deltas) < 1 || !strings.Contains(deltas[0].Value, "no relevant information") {
		t.Fatalf("expected 'no relevant information', got %+v", deltas)
	}
}

func TestAskService_StreamingFlow(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "test chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
		DocAggs: []map[string]interface{}{{"doc_id": "d1", "count": 1}},
	}}
	llm := &fakeStreamLLM{chunks: []string{"Hello", " world"}}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))

	var hasAnswer, hasFinal bool
	for _, d := range deltas {
		if d.Kind == AskDeltaAnswer {
			hasAnswer = true
		}
		if d.Kind == AskDeltaFinal {
			hasFinal = true
			if d.Refs == nil {
				t.Error("Final delta should have Refs")
			}
		}
	}
	if !hasAnswer || !hasFinal {
		t.Errorf("expected answer+final deltas, got %+v", deltas)
	}
}

func TestAskService_EmptyLLMStreamReturnsError(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "test chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
	}}
	llm := &fakeStreamLLM{}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))

	if len(deltas) != 1 || deltas[0].Kind != AskDeltaError || !strings.Contains(deltas[0].Value, "LLM call failed") {
		t.Fatalf("expected LLM error delta, got %+v", deltas)
	}
}

func TestAskService_ThinkTags(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
		DocAggs: []map[string]interface{}{},
	}}
	llm := &fakeStreamLLM{chunks: []string{"<think>", "reasoning...", "</think>", "visible answer"}}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))

	var hasMarker bool
	for _, d := range deltas {
		if d.Kind == AskDeltaMarker {
			hasMarker = true
		}
	}
	if !hasMarker {
		t.Error("expected think markers")
	}
}

func TestAskService_LLMError(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk"},
		},
	}}
	llm := &fakeStreamLLM{err: fmt.Errorf("model offline")}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))
	if len(deltas) < 1 || deltas[0].Kind != AskDeltaError {
		t.Fatalf("expected error delta, got %+v", deltas)
	}
}

func TestAskService_TemperatureFromLLMSetting(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
		DocAggs: []map[string]interface{}{},
	}}
	llm := &capturingStreamLLM{fakeStreamLLM: fakeStreamLLM{chunks: []string{"answer"}}}
	svc := NewAskService(ret, nil, 0, 0)

	opts := askOptionsFromSearchConfig("s1", map[string]interface{}{
		"llm_setting": map[string]interface{}{
			"temperature":         0.6,
			"temperature_enabled": true,
		},
	})
	deltas := collect(svc.StreamWithOptions(t.Context(), llm, "user1", "test", []string{"kb1"}, opts))

	var hasFinal bool
	for _, d := range deltas {
		if d.Kind == AskDeltaFinal {
			hasFinal = true
		}
	}
	if !hasFinal {
		t.Fatalf("expected final delta, got %+v", deltas)
	}
	if llm.gotConfig == nil || llm.gotConfig.Temperature == nil || *llm.gotConfig.Temperature != 0.6 {
		t.Errorf("expected configured temperature 0.6, got %+v", llm.gotConfig)
	}
}

func TestAskService_TemperatureDisabledUsesDefault(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
		DocAggs: []map[string]interface{}{},
	}}
	llm := &capturingStreamLLM{fakeStreamLLM: fakeStreamLLM{chunks: []string{"answer"}}}
	svc := NewAskService(ret, nil, 0, 0)

	opts := askOptionsFromSearchConfig("s1", map[string]interface{}{
		"llm_setting": map[string]interface{}{
			"temperature":         0.6,
			"temperature_enabled": false,
		},
	})
	deltas := collect(svc.StreamWithOptions(t.Context(), llm, "user1", "test", []string{"kb1"}, opts))

	var hasFinal bool
	for _, d := range deltas {
		if d.Kind == AskDeltaFinal {
			hasFinal = true
		}
	}
	if !hasFinal {
		t.Fatalf("expected final delta, got %+v", deltas)
	}
	if llm.gotConfig == nil || llm.gotConfig.Temperature == nil || *llm.gotConfig.Temperature != 0.1 {
		t.Errorf("expected default temperature 0.1 when disabled, got %+v", llm.gotConfig)
	}
}

func TestAskService_TemperatureMissingUsesLLMSettingDefaults(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
		DocAggs: []map[string]interface{}{},
	}}
	llm := &capturingStreamLLM{fakeStreamLLM: fakeStreamLLM{chunks: []string{"answer"}}}
	svc := NewAskService(ret, nil, 0, 0)

	// Flag enabled but value absent (and a completely absent flag) must
	// substitute the Python LLM_SETTING_DEFAULTS, matching resolve_llm_setting.
	opts := askOptionsFromSearchConfig("s1", map[string]interface{}{
		"llm_setting": map[string]interface{}{
			"temperature_enabled": true,
		},
	})
	collect(svc.StreamWithOptions(t.Context(), llm, "user1", "test", []string{"kb1"}, opts))

	if llm.gotConfig == nil || llm.gotConfig.Temperature == nil || *llm.gotConfig.Temperature != DefaultAskTemperature {
		t.Errorf("expected LLM_SETTING_DEFAULTS temperature %v, got %+v", DefaultAskTemperature, llm.gotConfig)
	}
	if llm.gotConfig.TopP == nil || *llm.gotConfig.TopP != DefaultAskTopP {
		t.Errorf("expected LLM_SETTING_DEFAULTS top_p %v, got %+v", DefaultAskTopP, llm.gotConfig)
	}
}

func TestExtractChunkVectors_Empty(t *testing.T) {
	if got := ExtractChunkVectors(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	if got := ExtractChunkVectors([]map[string]interface{}{}); len(got) != 0 {
		t.Errorf("expected empty for empty input, got %v", got)
	}
}

func TestExtractChunkVectors_Float64Slice(t *testing.T) {
	chunks := []map[string]interface{}{
		{"vector": []float64{1.0, 2.0, 3.0}},
		{"vector": []float64{0.0, 0.0, 0.0}},
	}
	result := ExtractChunkVectors(chunks)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if len(result[0]) != 3 || result[0][0] != 1.0 {
		t.Errorf("first vector should be [1,2,3]: %v", result[0])
	}
	if result[1] != nil {
		t.Errorf("zero vector should be nil: %v", result[1])
	}
}

func TestExtractChunkVectors_InterfaceSlice(t *testing.T) {
	chunks := []map[string]interface{}{
		{"vector": []interface{}{float64(4.0), float64(5.0)}},
	}
	result := ExtractChunkVectors(chunks)
	if len(result) != 1 || len(result[0]) != 2 || result[0][1] != 5.0 {
		t.Errorf("expected [4,5]: %v", result)
	}
}

func TestExtractChunkVectors_MissingField(t *testing.T) {
	chunks := []map[string]interface{}{{"id": "c1"}}
	result := ExtractChunkVectors(chunks)
	if len(result) != 1 || result[0] != nil {
		t.Errorf("missing vector field should give nil entry, got %v", result)
	}
}

func TestToFloat64Slice_Types(t *testing.T) {
	if got := toFloat64Slice(nil); got != nil {
		t.Error("nil should return nil")
	}
	if got := toFloat64Slice([]float64{1.0, 2.0}); len(got) != 2 || got[1] != 2.0 {
		t.Error("[]float64 should be copied")
	}
	if got := toFloat64Slice([]interface{}{float64(3.0)}); len(got) != 1 || got[0] != 3.0 {
		t.Error("[]interface{} containing float64 should work")
	}
	if got := toFloat64Slice("string"); got != nil {
		t.Error("unknown type should return nil")
	}
}

func TestToFloat64Slice_Independence(t *testing.T) {
	orig := []float64{1.0, 2.0, 3.0}
	result := toFloat64Slice(orig)
	result[0] = 999.0
	if orig[0] != 1.0 {
		t.Error("returned slice should be independent copy")
	}
}

func TestAskService_ContextCancel(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
	}}
	llm := &fakeStreamLLM{chunks: []string{"<think>", "reasoning...", "</think>", "visible answer"}}
	svc := NewAskService(ret, nil, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	deltas := collect(svc.Stream(ctx, llm, "user1", "test", []string{"kb1"}))
	// Should get no deltas (or very few) since context is cancelled.
	_ = deltas
}

func TestAskService_AsyncLLMErrorSurfaces(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk", "docnm_kwd": "Doc", "kb_id": "kb1", "doc_id": "d1"},
		},
		DocAggs: []map[string]interface{}{},
	}}
	llm := &asyncErrStreamLLM{fakeStreamLLM: fakeStreamLLM{chunks: []string{"partial"}}}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))

	var errDelta *AskDelta
	for i := range deltas {
		if deltas[i].Kind == AskDeltaError {
			errDelta = &deltas[i]
			break
		}
	}
	if errDelta == nil {
		t.Fatalf("expected error delta, got %+v", deltas)
	}
	if !strings.Contains(errDelta.Value, "invalid temperature") {
		t.Errorf("expected real driver error surfaced, got %q", errDelta.Value)
	}
}

func TestAskService_LLMErrorValueIsRealError(t *testing.T) {
	ret := &fakeRetriever{result: &RetrievalTestResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk"},
		},
	}}
	llm := &fakeStreamLLM{err: fmt.Errorf("provider name missing in model name: gpt-4o")}
	svc := NewAskService(ret, nil, 0, 0)
	deltas := collect(svc.Stream(t.Context(), llm, "user1", "test", []string{"kb1"}))
	if len(deltas) < 1 || deltas[0].Kind != AskDeltaError {
		t.Fatalf("expected error delta, got %+v", deltas)
	}
	if !strings.Contains(deltas[0].Value, "provider name missing in model name") {
		t.Errorf("expected real sync error surfaced, got %q", deltas[0].Value)
	}
}
