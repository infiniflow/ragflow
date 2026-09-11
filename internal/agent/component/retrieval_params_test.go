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

type retrievalRequestRecorder struct {
	request agenttool.RetrievalRequest
}

func (r *retrievalRequestRecorder) Search(_ context.Context, _ *gorm.DB, request agenttool.RetrievalRequest) ([]agenttool.RetrievalChunk, error) {
	r.request = request
	return []agenttool.RetrievalChunk{}, nil
}

func TestRetrievalComponentAppliesAdvancedNodeParams(t *testing.T) {
	component, err := newRetrievalComponent(map[string]any{
		"dataset_ids":      []any{"dataset-1"},
		"memory_ids":       []any{"memory-1", "memory-2"},
		"cross_languages":  []any{"English", "Chinese", "Spanish"},
		"toc_enhance":      true,
		"use_kg":           true,
		"meta_data_filter": map[string]any{"method": "manual"},
		"retrieval_from":   "memory",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}

	merged := component.(*retrievalComponent).applyDefaults(nil)
	if got := merged["memory_ids"]; !reflect.DeepEqual(got, []string{"memory-1", "memory-2"}) {
		t.Errorf("memory_ids = %#v", got)
	}
	if got := merged["cross_languages"]; !reflect.DeepEqual(got, []string{"English", "Chinese", "Spanish"}) {
		t.Errorf("cross_languages = %#v", got)
	}
	if got := merged["toc_enhance"]; got != true {
		t.Errorf("toc_enhance = %#v", got)
	}
	if got := merged["use_kg"]; got != true {
		t.Errorf("use_kg = %#v", got)
	}
	if got := merged["meta_data_filter"]; !reflect.DeepEqual(got, map[string]any{"method": "manual"}) {
		t.Errorf("meta_data_filter = %#v", got)
	}
	if got := merged["retrieval_from"]; got != "memory" {
		t.Errorf("retrieval_from = %#v", got)
	}
}

func TestRetrievalComponentForwardsAdvancedNodeParams(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	recorder := &retrievalRequestRecorder{}
	agenttool.SetRetrievalService(recorder)
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"dataset_ids":      []any{"dataset-1"},
		"memory_ids":       []any{"memory-1"},
		"cross_languages":  []any{"English", "Chinese", "Spanish"},
		"toc_enhance":      true,
		"use_kg":           false,
		"meta_data_filter": map[string]any{"method": "manual"},
		"retrieval_from":   "dataset",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}

	if _, err = component.Invoke(t.Context(), nil, map[string]any{"query": "爱"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	request := recorder.request
	if !reflect.DeepEqual(request.MemoryIDs, []string{"memory-1"}) {
		t.Errorf("MemoryIDs = %#v", request.MemoryIDs)
	}
	if !reflect.DeepEqual(request.CrossLanguages, []string{"English", "Chinese", "Spanish"}) {
		t.Errorf("CrossLanguages = %#v", request.CrossLanguages)
	}
	if !request.TOCEnhance {
		t.Error("TOCEnhance = false")
	}
	if request.UseKG {
		t.Error("UseKG = true")
	}
	if !reflect.DeepEqual(request.MetaDataFilter, map[string]any{"method": "manual"}) {
		t.Errorf("MetaDataFilter = %#v", request.MetaDataFilter)
	}
	if request.RetrievalFrom != "dataset" {
		t.Errorf("RetrievalFrom = %q", request.RetrievalFrom)
	}
}

func TestRetrievalComponentForwardsUserIDNodeParam(t *testing.T) {
	previous := agenttool.GetMemoryRetrievalService()
	recorder := &retrievalRequestRecorder{}
	agenttool.SetMemoryRetrievalService(recorder)
	t.Cleanup(func() { agenttool.SetMemoryRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"memory_ids":     []any{"memory-1"},
		"retrieval_from": "memory",
		"user_id":        "user-7",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}

	merged := component.(*retrievalComponent).applyDefaults(nil)
	if got := merged["user_id"]; got != "user-7" {
		t.Errorf("user_id = %#v, want user-7", got)
	}

	if _, err = component.Invoke(t.Context(), nil, map[string]any{"query": "remember"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if recorder.request.UserID != "user-7" {
		t.Errorf("UserID = %q, want user-7", recorder.request.UserID)
	}
	if recorder.request.RetrievalFrom != "memory" {
		t.Errorf("RetrievalFrom = %q, want memory", recorder.request.RetrievalFrom)
	}
}

func TestRetrievalComponentForwardsExplicitZeroSimilarityParams(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	recorder := &retrievalRequestRecorder{}
	agenttool.SetRetrievalService(recorder)
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"dataset_ids":                []any{"dataset-1"},
		"similarity_threshold":       0,
		"keywords_similarity_weight": 0,
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}

	merged := component.(*retrievalComponent).applyDefaults(nil)
	if value, ok := merged["similarity_threshold"]; !ok || value != float64(0) {
		t.Fatalf("similarity_threshold = %#v, present = %v; want explicit zero", value, ok)
	}
	if value, ok := merged["keywords_similarity_weight"]; !ok || value != float64(0) {
		t.Fatalf("keywords_similarity_weight = %#v, present = %v; want explicit zero", value, ok)
	}

	if _, err := component.Invoke(t.Context(), nil, map[string]any{"query": "zero"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if recorder.request.SimilarityThreshold == nil || *recorder.request.SimilarityThreshold != 0 {
		t.Fatalf("SimilarityThreshold = %v; want explicit zero", recorder.request.SimilarityThreshold)
	}
	if recorder.request.KeywordsSimilarityWeight == nil || *recorder.request.KeywordsSimilarityWeight != 0 {
		t.Fatalf("KeywordsSimilarityWeight = %v; want explicit zero", recorder.request.KeywordsSimilarityWeight)
	}
}
