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

package pipeline

import (
	"reflect"
	"testing"
)

// TestPipelineToCanvas_LinearChain pins the linear-stage
// conversion contract. The 5-stage production flow
// (File -> Parser -> TokenChunker -> Tokenizer -> Extractor)
// is the only path the production runtime drives today; this
// test makes sure the conversion preserves the order AND the
// per-stage Params (so a TokenChunker stage configured with
// chunk_token_size=1024 keeps that key after the round-trip).
func TestPipelineToCanvas_LinearChain(t *testing.T) {
	dsl := &PipelineDSL{
		Version:     "1",
		Name:        "default-ingestion",
		Description: "5-stage ingestion flow",
		StageCount:  5,
		Stages: []StageDSL{
			{Type: "File", Params: map[string]any{}},
			{Type: "Parser", Params: map[string]any{}},
			{Type: "TokenChunker", Params: map[string]any{"chunk_token_size": 1024}},
			{Type: "Tokenizer", Params: map[string]any{}},
			{Type: "Extractor", Params: map[string]any{}},
		},
	}

	c, err := PipelineToCanvas(dsl)
	if err != nil {
		t.Fatalf("PipelineToCanvas: %v", err)
	}

	// Path preserves stage order.
	wantPath := []string{"File", "Parser", "TokenChunker", "Tokenizer", "Extractor"}
	if !reflect.DeepEqual(c.Path, wantPath) {
		t.Errorf("Path = %v, want %v", c.Path, wantPath)
	}

	// Each stage's params survive the round-trip.
	if got, want := c.Components["TokenChunker"].Obj.Params["chunk_token_size"], 1024; got != want {
		t.Errorf("TokenChunker params[chunk_token_size] = %v, want %v", got, want)
	}

	// Edges encode the linear chain.
	chain := []struct {
		id         string
		upstream   []string
		downstream []string
	}{
		{"File", nil, []string{"Parser"}},
		{"Parser", []string{"File"}, []string{"TokenChunker"}},
		{"TokenChunker", []string{"Parser"}, []string{"Tokenizer"}},
		{"Tokenizer", []string{"TokenChunker"}, []string{"Extractor"}},
		{"Extractor", []string{"Tokenizer"}, nil},
	}
	for _, want := range chain {
		got := c.Components[want.id]
		if !reflect.DeepEqual(got.Upstream, want.upstream) {
			t.Errorf("%s Upstream = %v, want %v", want.id, got.Upstream, want.upstream)
		}
		if !reflect.DeepEqual(got.Downstream, want.downstream) {
			t.Errorf("%s Downstream = %v, want %v", want.id, got.Downstream, want.downstream)
		}
	}
}

// TestPipelineToCanvas_ComponentNameIsStageType pins the
// id == Type invariant. A future refactor that switches to
// per-instance ids must update this test AND the follow-up
// Runner.Run wiring; this test is the canary.
func TestPipelineToCanvas_ComponentNameIsStageType(t *testing.T) {
	dsl := &PipelineDSL{
		Version: "1",
		Stages: []StageDSL{
			{Type: "File", Params: map[string]any{}},
			{Type: "Parser", Params: map[string]any{}},
		},
	}
	c, err := PipelineToCanvas(dsl)
	if err != nil {
		t.Fatalf("PipelineToCanvas: %v", err)
	}
	for _, id := range c.Path {
		comp, ok := c.Components[id]
		if !ok {
			t.Errorf("Path id %q not in Components map", id)
			continue
		}
		if comp.Obj.ComponentName != id {
			t.Errorf("ComponentName = %q, want id = %q", comp.Obj.ComponentName, id)
		}
	}
}

// TestPipelineToCanvas_NilDSL is the input-validation pin. The
// runner trusts PipelineToCanvas to be a no-op on nil so the
// caller can defer validation to the adapter.
func TestPipelineToCanvas_NilDSL(t *testing.T) {
	if _, err := PipelineToCanvas(nil); err == nil {
		t.Fatal("nil DSL: want error, got nil")
	}
}

// TestPipelineToCanvas_EmptyStages pins the IsValid delegation
// contract. The adapter forwards to d.IsValid so a DSL with no
// stages fails before the conversion runs (a future "convert
// partial DSL" path can short-circuit on this error).
func TestPipelineToCanvas_EmptyStages(t *testing.T) {
	dsl := &PipelineDSL{Version: "1", Stages: nil}
	if _, err := PipelineToCanvas(dsl); err == nil {
		t.Fatal("empty Stages: want error, got nil")
	}
}

// TestPipelineToCanvas_DuplicateStage pins the duplicate-stage
// rejection. Two stages with the same Type would collide in the
// canvas Components map (which is keyed by id == Type today);
// rejecting the input is safer than silently dropping one.
func TestPipelineToCanvas_DuplicateStage(t *testing.T) {
	dsl := &PipelineDSL{
		Version: "1",
		Stages: []StageDSL{
			{Type: "File", Params: map[string]any{}},
			{Type: "File", Params: map[string]any{}},
		},
	}
	if _, err := PipelineToCanvas(dsl); err == nil {
		t.Fatal("duplicate stage type: want error, got nil")
	}
}

// TestMustPipelineToCanvas_PanicsOnError pins the panic-on-error
// helper used by test fixtures. Production code must not call
// the Must variant; this test exists to guarantee the panic
// path stays loud so a misuse surfaces immediately.
func TestMustPipelineToCanvas_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustPipelineToCanvas(nil): expected panic, got none")
		}
	}()
	MustPipelineToCanvas(nil)
}
