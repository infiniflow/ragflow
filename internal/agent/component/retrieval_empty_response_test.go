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

package component

import (
	"context"
	"reflect"
	"testing"

	agenttool "ragflow/internal/agent/tool"

	"gorm.io/gorm"
)

type emptyRetrievalService struct{}

func (emptyRetrievalService) Search(context.Context, *gorm.DB, agenttool.RetrievalRequest) ([]agenttool.RetrievalChunk, error) {
	return []agenttool.RetrievalChunk{}, nil
}

// assertEmptyRetrievalOutputs pins the declared outputs of a Retrieval node
// that produced no chunks: both names must resolve to an empty array so a
// downstream {{<id>@json}} / {{<id>@chunks}} template does not fail with
// "Can't find variable".
func assertEmptyRetrievalOutputs(t *testing.T, output map[string]any) {
	t.Helper()
	for _, key := range []string{"json", "chunks"} {
		value, ok := output[key].([]any)
		if !ok {
			t.Fatalf("%s = %#v, want an empty array", key, output[key])
		}
		if len(value) != 0 {
			t.Fatalf("%s = %#v, want an empty array", key, value)
		}
	}
}

// TestRetrievalComponent_EmptyResultPreservesConfiguredEmptyResponse covers
// the tool's early-return envelope (no retrieval_from configured): the
// configured empty_response reaches formalized_content and the chunk outputs
// stay resolvable.
func TestRetrievalComponent_EmptyResultPreservesConfiguredEmptyResponse(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	agenttool.SetRetrievalService(emptyRetrievalService{})
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"empty_response": "No relevant content.",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}
	output, err := component.Invoke(t.Context(), nil, map[string]any{"query": "love"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if output["formalized_content"] != "No relevant content." {
		t.Fatalf("formalized_content = %#v", output["formalized_content"])
	}
	assertEmptyRetrievalOutputs(t, output)
}

// TestRetrievalComponent_ZeroHitsExposeChunkOutputs covers the search path
// that returns no hit: the tool envelope drops its `chunks` key (omitempty),
// which previously left `json` unset and broke every downstream reference.
func TestRetrievalComponent_ZeroHitsExposeChunkOutputs(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	agenttool.SetRetrievalService(emptyRetrievalService{})
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"retrieval_from": "dataset",
		"kb_ids":         []any{"kb-1"},
		"empty_response": "No relevant content.",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}
	output, err := component.Invoke(t.Context(), nil, map[string]any{"query": "love"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if output["formalized_content"] != "No relevant content." {
		t.Fatalf("formalized_content = %#v", output["formalized_content"])
	}
	assertEmptyRetrievalOutputs(t, output)
}

// TestRetrievalComponent_MissingDatasetExposeChunkOutputs covers the
// no-dataset-selected early return: the node reports the configuration error
// while still resolving every declared output.
func TestRetrievalComponent_MissingDatasetExposeChunkOutputs(t *testing.T) {
	component, err := newRetrievalComponent(map[string]any{
		"retrieval_from": "dataset",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}
	output, err := component.Invoke(t.Context(), nil, map[string]any{"query": "love"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if output["_ERROR"] != "No dataset is selected." {
		t.Fatalf("_ERROR = %#v", output["_ERROR"])
	}
	if output["formalized_content"] != "" {
		t.Fatalf("formalized_content = %#v, want empty string", output["formalized_content"])
	}
	assertEmptyRetrievalOutputs(t, output)
}

// TestRetrievalComponent_JSONOutputMirrorsChunks pins the alias itself: `json`
// is the DSL-declared name for the same chunk array the tool reports as
// `chunks`, so downstream templates see identical data under either name.
func TestRetrievalComponent_JSONOutputMirrorsChunks(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	agenttool.SetSimpleRetrievalService()
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"retrieval_from": "dataset",
		"kb_ids":         []any{"kb-1"},
		"top_n":          1,
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}
	output, err := component.Invoke(t.Context(), nil, map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := output["chunks"].([]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one chunk", output["chunks"])
	}
	first, ok := chunks[0].(map[string]any)
	if !ok || first["id"] != "simple-0" || first["content"] == "" {
		t.Fatalf("chunk payload = %#v", chunks[0])
	}
	jsonOutput, ok := output["json"].([]any)
	if !ok {
		t.Fatalf("json = %#v, want an array", output["json"])
	}
	if !reflect.DeepEqual(jsonOutput, chunks) {
		t.Fatalf("json = %#v, want the chunks array %#v", jsonOutput, chunks)
	}
}
