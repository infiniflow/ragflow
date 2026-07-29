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
	"testing"

	agenttool "ragflow/internal/agent/tool"

	"gorm.io/gorm"
)

type emptyRetrievalService struct{}

func (emptyRetrievalService) Search(context.Context, *gorm.DB, agenttool.RetrievalRequest) ([]agenttool.RetrievalChunk, error) {
	return []agenttool.RetrievalChunk{}, nil
}

func TestRetrievalComponent_PreservesConfiguredEmptyResponse(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	agenttool.SetRetrievalService(emptyRetrievalService{})
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	component, err := newRetrievalComponent(map[string]any{
		"empty_response": "No relevant content.",
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}
	output, err := component.Invoke(context.Background(), nil, map[string]any{"query": "love"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if output["formalized_content"] != "No relevant content." {
		t.Fatalf("formalized_content = %#v", output["formalized_content"])
	}
}
