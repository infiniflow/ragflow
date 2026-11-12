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

package bootstrap

import (
	"errors"
	"testing"

	"ragflow/internal/agent/retrievalbridge"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/agentic_rag"
	"ragflow/internal/service"
)

func TestInitializeRetrievalServices(t *testing.T) {
	retrieval := runtime.GetRetrievalService()
	memory := runtime.GetMemoryRetrievalService()
	grep := runtime.GetGrepService()
	bm25 := runtime.GetBm25Service()
	kg := agentic_rag.GetKgService()
	t.Cleanup(func() {
		runtime.SetRetrievalService(retrieval)
		runtime.SetMemoryRetrievalService(memory)
		runtime.SetGrepService(grep)
		runtime.SetBm25Service(bm25)
		agentic_rag.SetKgService(kg)
	})

	memoryService := service.NewMemoryService(nil, nil)
	InitializeRetrievalServices(service.NewMetadataService(), memoryService, service.NewModelFactory())

	if _, ok := runtime.GetRetrievalService().(*retrievalbridge.RuntimeAdapter); !ok {
		t.Fatalf("retrieval service = %T, want runtime bridge", runtime.GetRetrievalService())
	}
	if _, ok := runtime.GetMemoryRetrievalService().(*retrievalbridge.MemoryAdapter); !ok {
		t.Errorf("memory service = %T, want memory adapter", runtime.GetMemoryRetrievalService())
	}
	if _, ok := runtime.GetGrepService().(*agentic_rag.GrepAdapter); !ok {
		t.Errorf("grep service = %T, want grep adapter", runtime.GetGrepService())
	}
	if _, ok := runtime.GetBm25Service().(*agentic_rag.Bm25Adapter); !ok {
		t.Errorf("BM25 service = %T, want BM25 adapter", runtime.GetBm25Service())
	}
	if _, ok := agentic_rag.GetKgService().(*agentic_rag.KgAdapter); !ok {
		t.Errorf("graph service = %T, want graph adapter", agentic_rag.GetKgService())
	}

	// Both registries share the runtime bridge, which must call the concrete NLP
	// adapter rather than look itself up and recurse.
	for _, svc := range []runtime.RetrievalService{runtime.GetRetrievalService(), agenttool.GetRetrievalService()} {
		chunks, err := svc.Search(t.Context(), nil, runtime.RetrievalRequest{})
		if err != nil || len(chunks) != 0 {
			t.Errorf("empty retrieval = %v, %v; want no chunks or error", chunks, err)
		}
		_, err = svc.Search(t.Context(), nil, runtime.RetrievalRequest{UseKG: true})
		if !errors.Is(err, agenttool.ErrGraphRAGNotSupported) {
			t.Errorf("GraphRAG retrieval error = %v, want NLP adapter validation", err)
		}
	}

	// A configured memory service reaches request validation, not the missing
	// service sentinel returned when the adapter has no memory backend.
	_, err := runtime.GetMemoryRetrievalService().Search(t.Context(), nil, runtime.RetrievalRequest{})
	if err == nil || err.Error() != "memory retrieval: memory_ids is required" {
		t.Errorf("memory retrieval error = %v, want missing memory IDs", err)
	}
}
