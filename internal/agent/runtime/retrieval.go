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

// Retrieval contracts shared by the canvas agent runtime (internal/agent/tool)
// and the smart-reasoning agent (internal/agentic_rag). Keeping these here —
// in the engine-agnostic runtime package — means neither agent layer depends on
// the other: both depend on this shared contract.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// RetrievalChunk is the minimal shape a RetrievalService returns. The full
// Chunk type (with document_id, docnm_kwd, position, etc.) lives in
// internal/entity and is wired in by the retrieval adapters.
type RetrievalChunk struct {
	ID           string
	Content      string
	DocumentID   string
	DocumentName string
	DatasetID    string
	ImageID      string
	URL          string
	Positions    any
	// ChunkIndex is the chunk's 0-based reading-order index within its document
	// (ES `chunk_order_int`). Deep-read tools sort chunks by it so the model reads
	// a document sequentially rather than in arbitrary match order.
	ChunkIndex int
	// PageNum is the chunk's page number within its document (ES `page_num_int`).
	PageNum          int
	Score            float64
	TermSimilarity   float64
	VectorSimilarity float64
}

// RetrievalRequest is the input to RetrievalService.Search.
type RetrievalRequest struct {
	Query                    string
	DatasetIDs               []string
	MemoryIDs                []string
	TopN                     int
	RerankCandidatesCount    int
	TopK                     int
	KeywordsSimilarityWeight *float64
	UseKG                    bool
	SimilarityThreshold      *float64
	RerankID                 string
	CrossLanguages           []string
	TOCEnhance               bool
	MetaDataFilter           map[string]any
	RetrievalFrom            string
	// DocScope restricts retrieval to a set of document ids. Empty = no doc filter.
	DocScope []string
	// TenantID is the calling tenant (== user_id in RAGFlow's data model).
	TenantID string
	// OnlyOriginalText, when true, restricts retrieval to ordinary document
	// text chunks (available_int=1 and no compile_kwd), excluding
	// knowledge-compiled products.
	OnlyOriginalText bool
	// SelectFields limits the ES _source fields returned per hit.
	SelectFields []string
}

// RetrievalService is the knowledge-base search interface. The server installs
// an adapter during boot.
type RetrievalService interface {
	Search(ctx context.Context, db *gorm.DB, req RetrievalRequest) ([]RetrievalChunk, error)
}

// MemoryRetrievalService is the memory-message retrieval surface used when
// retrieval_from=memory.
type MemoryRetrievalService interface {
	Search(ctx context.Context, db *gorm.DB, req RetrievalRequest) ([]RetrievalChunk, error)
}

// KGRetrievalService is the GraphRAG retrieval surface.
type KGRetrievalService interface {
	Search(ctx context.Context, db *gorm.DB, req RetrievalRequest) ([]RetrievalChunk, error)
}

// GrepService is the regex-search surface used by grep_chunks. It is separate
// from RetrievalService because regex matching over chunk content is a distinct
// retrieval mode.
type GrepService interface {
	Grep(ctx context.Context, req GrepRequest) ([]RetrievalChunk, error)
}

// GrepRequest is the input to GrepService.Grep.
type GrepRequest struct {
	Pattern    string   // The regex to match against chunk content (case-insensitive).
	DatasetIDs []string // Knowledge base IDs to restrict to.
	DocScope   []string // Document IDs to restrict to (empty = no doc filter).
	Limit      int      // Max number of chunks to return.
	Offset     int      // Number of chunks to skip (0-based), for pagination.
	// Sort is an ordered list of field names to order results by ascending
	// (e.g. a document's reading order: chunk_order_int, page_num_int, top_int).
	Sort []string // Ordered ascending sort fields.
	// SelectFields limits the ES _source fields returned per hit.
	SelectFields []string
	TenantID     string // Calling tenant (== user_id in RAGFlow's data model).
}

// ErrRetrievalServiceMissing is returned when no RetrievalService is registered.
var ErrRetrievalServiceMissing = errors.New(
	"Retrieval service not yet implemented (service not registered) — " +
		"use Python Canvas or implement internal/service/nlp/retrieval.go",
)

// ErrMemoryRetrievalServiceMissing is returned when no MemoryRetrievalService is registered.
var ErrMemoryRetrievalServiceMissing = errors.New("memory retrieval service not registered")

// ErrKGRetrievalServiceMissing is returned when no KGRetrievalService is registered.
var ErrKGRetrievalServiceMissing = errors.New(
	"GraphRAG (kg) retrieval service not yet wired",
)

// ErrGrepServiceMissing is returned when no GrepService has been registered.
var ErrGrepServiceMissing = errors.New(
	"grep service not registered — call runtime.SetGrepService(...) at boot",
)

// ErrRegexpNotSupported is returned when the underlying doc engine does not
// implement regex matching on chunk content (e.g. Infinity).
var ErrRegexpNotSupported = errors.New(
	"grep_chunks: regex matching is not supported by this document engine",
)

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

var (
	memoryRetrievalServiceMu   sync.RWMutex
	memoryRetrievalServiceImpl MemoryRetrievalService = stubMemoryRetrievalService{}
)

func SetMemoryRetrievalService(svc MemoryRetrievalService) {
	memoryRetrievalServiceMu.Lock()
	defer memoryRetrievalServiceMu.Unlock()
	if svc == nil {
		memoryRetrievalServiceImpl = stubMemoryRetrievalService{}
		return
	}
	memoryRetrievalServiceImpl = svc
}

func GetMemoryRetrievalService() MemoryRetrievalService {
	memoryRetrievalServiceMu.RLock()
	defer memoryRetrievalServiceMu.RUnlock()
	return memoryRetrievalServiceImpl
}

var (
	kgRetrievalServiceMu   sync.RWMutex
	kgRetrievalServiceImpl KGRetrievalService = stubKGRetrievalService{}
)

func SetKGRetrievalService(svc KGRetrievalService) {
	kgRetrievalServiceMu.Lock()
	defer kgRetrievalServiceMu.Unlock()
	if svc == nil {
		kgRetrievalServiceImpl = stubKGRetrievalService{}
		return
	}
	kgRetrievalServiceImpl = svc
}

func GetKGRetrievalService() KGRetrievalService {
	kgRetrievalServiceMu.RLock()
	defer kgRetrievalServiceMu.RUnlock()
	return kgRetrievalServiceImpl
}

var (
	grepServiceMu   sync.RWMutex
	grepServiceImpl GrepService = stubGrepService{}
)

func SetGrepService(svc GrepService) {
	grepServiceMu.Lock()
	defer grepServiceMu.Unlock()
	if svc == nil {
		grepServiceImpl = stubGrepService{}
		return
	}
	grepServiceImpl = svc
}

func GetGrepService() GrepService {
	grepServiceMu.RLock()
	defer grepServiceMu.RUnlock()
	return grepServiceImpl
}

type stubRetrievalService struct{}

func (stubRetrievalService) Search(_ context.Context, _ *gorm.DB, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return nil, ErrRetrievalServiceMissing
}

type stubMemoryRetrievalService struct{}

func (stubMemoryRetrievalService) Search(_ context.Context, _ *gorm.DB, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return nil, ErrMemoryRetrievalServiceMissing
}

type stubKGRetrievalService struct{}

func (stubKGRetrievalService) Search(_ context.Context, _ *gorm.DB, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return nil, ErrKGRetrievalServiceMissing
}

type stubGrepService struct{}

func (stubGrepService) Grep(_ context.Context, _ GrepRequest) ([]RetrievalChunk, error) {
	return nil, ErrGrepServiceMissing
}

// simpleRetrievalService is a deterministic test implementation that returns
// synthetic chunks based on the query.
type simpleRetrievalService struct{}

func (simpleRetrievalService) Search(_ context.Context, _ *gorm.DB, req RetrievalRequest) ([]RetrievalChunk, error) {
	if req.Query == "" {
		return nil, nil
	}
	topN := req.TopN
	if topN <= 0 {
		topN = 8
	}
	const maxSimpleTopN = 1024
	if topN > maxSimpleTopN {
		topN = maxSimpleTopN
	}
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

// SetSimpleRetrievalService installs deterministic synthetic retrieval for
// tests and local demos.
func SetSimpleRetrievalService() {
	SetRetrievalService(simpleRetrievalService{})
}
