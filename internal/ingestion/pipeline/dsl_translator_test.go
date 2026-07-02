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

// Slice 4 tests for port-rag-flow-pipeline-to-go.md Phase 4.
// Pins the keyed-components → PipelineDSL translation contract
// and the upstream-end Pipeline.RunCanvas method (the seam
// the canvas runner drives).

package pipeline

import (
	"errors"
	"reflect"
	"testing"

	_ "ragflow/internal/ingestion/component"         // blank import: registers ingestion factories
	_ "ragflow/internal/ingestion/component/chunker" // blank import: registers chunker factories
)

// TestTemplateToPipelineDSL_Linear pins the keyed-components
// translation for a 3-stage linear chain. The expected Stages[]
// order is id-sorted (deterministic) when no canvas-side
// downstream ordering is consulted.
func TestTemplateToPipelineDSL_Linear(t *testing.T) {
	template := map[string]any{
		"canvas_category": "dataflow_canvas",
		"components": map[string]any{
			"File": map[string]any{
				"obj": map[string]any{
					"component_name": "File",
					"params":         map[string]any{},
				},
			},
			"Parser": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params":         map[string]any{"setups": map[string]any{}},
				},
			},
			"TokenChunker": map[string]any{
				"obj": map[string]any{
					"component_name": "TokenChunker",
					"params":         map[string]any{"chunk_token_size": 512},
				},
			},
		},
	}
	dsl, err := TemplateToPipelineDSL(template)
	if err != nil {
		t.Fatalf("TemplateToPipelineDSL: %v", err)
	}
	if len(dsl.Stages) != 3 {
		t.Fatalf("Stages len = %d, want 3", len(dsl.Stages))
	}
	wantTypes := []string{"File", "Parser", "TokenChunker"}
	for i, want := range wantTypes {
		if dsl.Stages[i].Type != want {
			t.Errorf("Stages[%d].Type = %q, want %q", i, dsl.Stages[i].Type, want)
		}
	}
	// Params from the keyed-components shape survive the round-trip.
	if got, want := dsl.Stages[1].Params["setups"], map[string]any{}; !reflect.DeepEqual(got, want) {
		t.Errorf("Stages[1].Params[setups] = %v, want %v", got, want)
	}
	if got, want := dsl.Stages[2].Params["chunk_token_size"], 512; got != want {
		t.Errorf("Stages[2].Params[chunk_token_size] = %v, want %v", got, want)
	}
}

// TestTemplateToPipelineDSL_RejectsNonIngestionCategory pins the
// canvas_category whitelist. Templates with category="agent" must
// not route through the ingestion translator.
func TestTemplateToPipelineDSL_RejectsNonIngestionCategory(t *testing.T) {
	template := map[string]any{
		"canvas_category": "agent",
		"components":      map[string]any{},
	}
	_, err := TemplateToPipelineDSL(template)
	if err == nil {
		t.Fatal("agent template: want ErrTemplateNotIngestion, got nil")
	}
	if !errors.Is(err, ErrTemplateNotIngestion) {
		t.Errorf("err = %v, want wraps ErrTemplateNotIngestion", err)
	}
}

// TestTemplateToPipelineDSL_RejectsAgentNodes pins the
// per-component rejection. A node referencing "Begin" or
// "Message" must fail with ErrTemplateAgentNode.
func TestTemplateToPipelineDSL_RejectsAgentNodes(t *testing.T) {
	template := map[string]any{
		"canvas_category": "dataflow_canvas",
		"components": map[string]any{
			"Begin": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
			},
		},
	}
	_, err := TemplateToPipelineDSL(template)
	if err == nil {
		t.Fatal("Begin node: want ErrTemplateAgentNode, got nil")
	}
	var agentErr ErrTemplateAgentNode
	if !errors.As(err, &agentErr) {
		t.Errorf("err = %v, want ErrTemplateAgentNode", err)
	} else if agentErr.NodeID != "Begin" || agentErr.ComponentName != "Begin" {
		t.Errorf("ErrTemplateAgentNode = %+v, want {Begin Begin}", agentErr)
	}
}

// TestTemplateToPipelineDSL_RejectsDuplicateComponent pins the
// singleton-per-component rule. The keyed-components shape uses
// per-instance ids like "Parser:HipSignsRhyme"; the legacy
// pipeline shape is one-stage-per-component-name, so two
// instances of the same component collide.
func TestTemplateToPipelineDSL_RejectsDuplicateComponent(t *testing.T) {
	template := map[string]any{
		"canvas_category": "dataflow_canvas",
		"components": map[string]any{
			"Parser:A": map[string]any{
				"obj": map[string]any{"component_name": "Parser", "params": map[string]any{}},
			},
			"Parser:B": map[string]any{
				"obj": map[string]any{"component_name": "Parser", "params": map[string]any{}},
			},
		},
	}
	_, err := TemplateToPipelineDSL(template)
	if err == nil {
		t.Fatal("duplicate Parser: want errDuplicateStage, got nil")
	}
	wantErr := errDuplicateStage("Parser")
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestTemplateToPipelineDSL_EmptyComponents pins the empty-input
// rejection (matches pipeline.go:IsValid behaviour).
func TestTemplateToPipelineDSL_EmptyComponents(t *testing.T) {
	template := map[string]any{
		"canvas_category": "dataflow_canvas",
		"components":      map[string]any{},
	}
	_, err := TemplateToPipelineDSL(template)
	if err == nil {
		t.Fatal("empty components: want error, got nil")
	}
	if !errors.Is(err, errEmptyStages) {
		t.Errorf("err = %v, want wraps errEmptyStages", err)
	}
}

// TestTemplateToPipelineDSL_NilTemplate pins the nil-input
// rejection.
func TestTemplateToPipelineDSL_NilTemplate(t *testing.T) {
	_, err := TemplateToPipelineDSL(nil)
	if err == nil {
		t.Fatal("nil template: want error, got nil")
	}
	if !errors.Is(err, errNilDSL) {
		t.Errorf("err = %v, want wraps errNilDSL", err)
	}
}

// TestTemplateToPipelineDSL_RoundTripsGeneralProduction pins the
// real-world general.json template shape. The fixture is a
// in-memory stub mirroring the production schema: 5-stage
// File → Parser → TokenChunker → Tokenizer with two parser nodes
// (one per family). Both must round-trip without dropping params.
func TestTemplateToPipelineDSL_RoundTripsGeneralProduction(t *testing.T) {
	template := buildGeneralTemplateFixture()
	dsl, err := TemplateToPipelineDSL(template)
	if err != nil {
		t.Fatalf("TemplateToPipelineDSL: %v", err)
	}
	if len(dsl.Stages) != 4 {
		t.Errorf("Stages len = %d, want 4 (File, Parser, TokenChunker, Tokenizer)", len(dsl.Stages))
	}
}

// buildGeneralTemplateFixture returns an in-memory template that
// mirrors the production general.json shape. Used by
// TestTemplateToPipelineDSL_RoundTripsGeneralProduction.
func buildGeneralTemplateFixture() map[string]any {
	return map[string]any{
		"canvas_category": "dataflow_canvas",
		"components": map[string]any{
			"File": map[string]any{
				"obj": map[string]any{
					"component_name": "File",
					"params":         map[string]any{},
				},
			},
			"Parser": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params": map[string]any{
						"setups": map[string]any{
							"markdown": map[string]any{"output_format": "json"},
						},
					},
				},
			},
			"TokenChunker": map[string]any{
				"obj": map[string]any{
					"component_name": "TokenChunker",
					"params": map[string]any{
						"chunk_token_size": 512,
						"delimiter_mode":   "token_size",
					},
				},
			},
			"Tokenizer": map[string]any{
				"obj": map[string]any{
					"component_name": "Tokenizer",
					"params": map[string]any{
						"search_method": []any{"full_text"},
					},
				},
			},
		},
	}
}
