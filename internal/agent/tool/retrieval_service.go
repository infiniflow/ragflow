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

// RetrievalService is the abstract interface for retrieval.
// The full Python `Dealer.search()` surface (KB + memory +
// rerank + cross-language + toc_enhance + metadata filter +
// GraphRAG) lands incrementally as the corresponding
// enhancements fill in. For now the interface is minimal —
// just the entry point the RetrievalTool calls.
package tool

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// RetrievalChunk is the minimal shape RetrievalService returns. The
// full Chunk type (with document_id, docnm_kwd, position, etc.)
// lives in internal/entity and is wired in by a follow-up phase.
type RetrievalChunk struct {
	ID         string
	Content    string
	DocumentID string
	Score      float64
}

// RetrievalRequest is the input to RetrievalService.Search.
type RetrievalRequest struct {
	Query               string
	DatasetIDs          []string
	TopN                int
	UseKG               bool
	UseRerank           bool
	RerankID            string
	TOCEnhance          bool
	MetadataFilter      map[string]string
	SimilarityThreshold float64
	// TenantID is the calling tenant (== user_id in RAGFlow's data model).
	// Optional for the nlp adapter; the KG adapter uses it to resolve the
	// tenant's default chat + embedding models. Reads from
	// CanvasState.Sys["user_id"] when empty (set by the Begin component at
	// internal/agent/component/begin.go:82).
	TenantID string
}

// RetrievalService is the interface the Retrieval tool uses.
// Today only the stub impl exists; production code can register
// a real impl via SetRetrievalService during boot.
type RetrievalService interface {
	Search(ctx context.Context, req RetrievalRequest) ([]RetrievalChunk, error)
}

// KGRetrievalService is the GraphRAG retrieval surface. The
// KBRetrieval service and the KGRetrieval service are kept
// separate on purpose: the kg backend's signature requires
// per-tenant chat + embedding model handles, while the nlp
// backend resolves them lazily through RetrievalRequest's
// EmbeddingModel field. Splitting the registries means each
// adapter can be tested in isolation and wired independently
// at boot.
type KGRetrievalService interface {
	Search(ctx context.Context, req RetrievalRequest) ([]RetrievalChunk, error)
}

// ErrRetrievalServiceMissing is declared in retrieval.go (kept
// for backward compat with retrieval_test.go). Callers using
// the RetrievalService interface here can match against the
// same sentinel by referring to the same package-level var.

var (
	retrievalServiceMu   sync.RWMutex
	retrievalServiceImpl RetrievalService = stubRetrievalService{}
)

func SetRetrievalService(svc RetrievalService) {
	retrievalServiceMu.Lock()
	defer retrievalServiceMu.Unlock()
	if svc == nil {
		retrievalServiceImpl = stubRetrievalService{}
		return
	}
	retrievalServiceImpl = svc
}

func GetRetrievalService() RetrievalService {
	retrievalServiceMu.RLock()
	defer retrievalServiceMu.RUnlock()
	return retrievalServiceImpl
}

type stubRetrievalService struct{}

func (stubRetrievalService) Search(_ context.Context, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return nil, ErrRetrievalServiceMissing
}

// simpleRetrievalService is a deterministic test/demo impl that
// returns synthetic chunks based on the query. Useful for
// development and integration tests; the production impl lands
// when the boot path wires internal/service.ChunkService into
// SetRetrievalService.
type simpleRetrievalService struct{}

func (simpleRetrievalService) Search(_ context.Context, req RetrievalRequest) ([]RetrievalChunk, error) {
	if req.Query == "" {
		return nil, nil
	}
	topN := req.TopN
	if topN <= 0 {
		topN = 8
	}
	// Cap topN to a sane upper bound so a hostile canvas can't force
	// a giant preallocation here. Real callers honor this cap; the
	// production service has its own server-side limits as well.
	const maxSimpleTopN = 1024
	if topN > maxSimpleTopN {
		topN = maxSimpleTopN
	}
	// codeql[go/uncontrolled-allocation-size] False positive: topN
	// is bounded to maxSimpleTopN (1024) above, so the resulting
	// slice cannot exceed ~1 MiB (chunk items are small structs).
	chunks := make([]RetrievalChunk, 0, topN)
	for i := 0; i < topN && i < 3; i++ {
		chunks = append(chunks, RetrievalChunk{
			ID:         fmt.Sprintf("simple-%d", i),
			Content:    fmt.Sprintf("Chunk %d matching %q", i, req.Query),
			DocumentID: "simple-doc",
			Score:      0.9 - float64(i)*0.1,
		})
	}
	return chunks, nil
}

// SetSimpleRetrievalService installs the simpleRetrievalService
// (deterministic synthetic chunks). Useful for development and
// integration tests. Production code should call
// SetRetrievalService with a real implementation backed by
// internal/service.ChunkService — see design doc §4.2
// RetrievalService.
func SetSimpleRetrievalService() {
	SetRetrievalService(simpleRetrievalService{})
}

// ErrKGRetrievalServiceMissing is returned when the agent's
// RetrievalTool dispatches use_kg=true but no KGRetrievalService
// has been registered via SetKGRetrievalService. This is the
// expected "kg not yet wired" state — distinct from
// ErrRetrievalServiceMissing (which signals the nlp adapter is
// un-wired).
var ErrKGRetrievalServiceMissing = errors.New(
	"GraphRAG (kg) retrieval service not yet wired — " +
		"call tool.SetKGRetrievalService(tool.NewKGRetrievalAdapter(...)) at boot",
)

var (
	kgRetrievalServiceMu   sync.RWMutex
	kgRetrievalServiceImpl KGRetrievalService = stubKGRetrievalService{}
)

// SetKGRetrievalService installs the GraphRAG adapter. Passing
// nil reverts to the stub that returns ErrKGRetrievalServiceMissing.
// Idempotent: safe to call from cmd/server_main.go once at boot
// and from tests that want to swap the impl.
func SetKGRetrievalService(svc KGRetrievalService) {
	kgRetrievalServiceMu.Lock()
	defer kgRetrievalServiceMu.Unlock()
	if svc == nil {
		kgRetrievalServiceImpl = stubKGRetrievalService{}
		return
	}
	kgRetrievalServiceImpl = svc
}

// GetKGRetrievalService returns the registered KGRetrievalService.
// Always non-nil — defaults to the stub.
func GetKGRetrievalService() KGRetrievalService {
	kgRetrievalServiceMu.RLock()
	defer kgRetrievalServiceMu.RUnlock()
	return kgRetrievalServiceImpl
}

type stubKGRetrievalService struct{}

func (stubKGRetrievalService) Search(_ context.Context, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return nil, ErrKGRetrievalServiceMissing
}
