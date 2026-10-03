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

// Retrieval contracts shared by the canvas agent runtime (internal/agent/tool).
// Keeping these in the engine-agnostic runtime package means the tool layer
// re-exports them instead of owning a second copy.
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
	PageNum int
	// MomID is the parent chunk id when this chunk is a child fragment; empty
	// for top-level chunks. It is threaded through so the harness can run
	// retrieval_by_children (child fragments are promoted to their parent chunk)
	// after search, mirroring Python settings.retriever.retrieval_by_children.
	MomID            string
	Score            float64
	TermSimilarity   float64
	VectorSimilarity float64
	// DocType is the engine's doc_type_kwd ("text" / "image" / "table").
	// Carried so an image chunk reaches the harness evidence pool with its type
	// intact and the answer reference card can still render it as an image.
	DocType string
}

// RetrievalRequest is the input to RetrievalService.Search.
type RetrievalRequest struct {
	Query      string
	DatasetIDs []string
	MemoryIDs  []string
	TopN       int
	// RerankCandidatesCount caps the candidate set pulled for reranking. Zero
	// means "use the backend default".
	RerankCandidatesCount    int
	TopK                     int
	KeywordsSimilarityWeight *float64
	UseKG                    bool
	SimilarityThreshold      *float64
	AllowDenseFallback       *bool
	RerankID                 string
	CrossLanguages           []string
	TOCEnhance               bool
	MetaDataFilter           map[string]any
	RetrievalFrom            string
	// DocScope restricts retrieval to a set of document ids (the doc_id list
	// routed by the dataset_navigation_by_tree tool). Empty = no doc filter.
	DocScope []string
	// TenantID is the calling tenant (== user_id in RAGFlow's data model).
	TenantID string
	// RankFeature is the label_question term→weight map passed through to the
	// engine so retrieval is biased toward the query's predicted topic class.
	// Mirrors engine nlp.RetrievalRequest.RankFeature.
	RankFeature *map[string]float64
	// UserID optionally filters memory messages by the user_id they were
	// recorded with (the Retrieval node's "User ID" field, e.g. resolved
	// from sys.user_id). Empty = no user filter. Only meaningful for
	// retrieval_from=memory.
	UserID string
	// ExcludeCompiled excludes compiled-product rows from plain retrieval.
	ExcludeCompiled bool
	// OnlyOriginalText, when true, restricts retrieval to ordinary document
	// text chunks (available_int=1 and no compile_kwd), excluding
	// knowledge-compiled products.
	OnlyOriginalText bool
	// VectorOnly, when true, issues ONLY the dense (vector) leg: no text match
	// expression is built or sent, no fusion expression is attached, and the
	// kNN candidate set is filtered by SCOPE alone (kb_id, doc_id,
	// available_int, ...) instead of by the query's own words.
	//
	// It exists because a hybrid request with KeywordsSimilarityWeight = 0 is
	// NOT a pure vector search: the engine still builds the BM25 clause and
	// passes it as the kNN `filter`, so only chunks matching the query text can
	// come back — the exact restriction a wording-gap search must not have. A
	// vector-only request also skips the 4x candidate over-fetch, the fusion
	// step and the engine's second-pass KNN scoring round trip.
	VectorOnly bool
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
	// ChunkScope restricts to specific chunk ids (term filter) when non-empty;
	// used by meta lookups like ResolveChunkOrder, not by grep itself.
	ChunkScope []string
	Limit      int // Max number of chunks to return.
	Offset     int // Number of chunks to skip (0-based), for pagination.
	// Sort is an ordered list of field names to order results by ascending
	// (e.g. a document's reading order: chunk_order_int, page_num_int, top_int).
	Sort []string // Ordered ascending sort fields.
	// SelectFields limits the ES _source fields returned per hit.
	SelectFields []string
	TenantID     string // Calling tenant (== user_id in RAGFlow's data model).
}

// Bm25Service is the lexical full-text (BM25) search surface used by
// search_bm25_chunks. Like GrepService it is separate from RetrievalService
// because BM25 ranking over tokenized chunk fields is a distinct retrieval mode
// with no vector component.
type Bm25Service interface {
	SearchBm25(ctx context.Context, req Bm25Request) ([]RetrievalChunk, error)
}

// Bm25Request is the input to Bm25Service.SearchBm25.
type Bm25Request struct {
	// Queries are 1-5 keyword/phrase queries; each is scored independently and
	// results are merged and deduplicated by chunk id.
	Queries    []string
	DatasetIDs []string // Knowledge base IDs to restrict to.
	DocScope   []string // Document IDs to restrict to (empty = no doc filter).
	TopN       int      // Max chunks per query before merging.
	TenantID   string   // Calling tenant (== user_id in RAGFlow's data model).
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

// ErrBm25ServiceMissing is returned when no Bm25Service has been registered.
var ErrBm25ServiceMissing = errors.New(
	"bm25 service not registered — call runtime.SetBm25Service(...) at boot",
)

// ErrRegexpNotSupported is returned when the underlying doc engine does not
// implement regex matching on chunk content (e.g. Infinity).
var ErrRegexpNotSupported = errors.New(
	"grep_chunks: regex matching is not supported by this document engine",
)

// ErrRegexpPushdown is wrapped around a native regex search the engine refused
// to run — an unsupported construct (Lucene regexp has no \b, no lookaround) or
// an automaton that blew the engine's state budget (long alternations of `.*`).
// It is a sentinel so the caller can tell "this pattern is beyond the engine"
// apart from a transport or scope failure and degrade deliberately instead of
// surfacing an opaque backend error.
var ErrRegexpPushdown = errors.New("grep_chunks: regexp pushdown failed")

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

var (
	bm25ServiceMu   sync.RWMutex
	bm25ServiceImpl Bm25Service = stubBm25Service{}
)

func SetBm25Service(svc Bm25Service) {
	bm25ServiceMu.Lock()
	defer bm25ServiceMu.Unlock()
	if svc == nil {
		bm25ServiceImpl = stubBm25Service{}
		return
	}
	bm25ServiceImpl = svc
}

func GetBm25Service() Bm25Service {
	bm25ServiceMu.RLock()
	defer bm25ServiceMu.RUnlock()
	return bm25ServiceImpl
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

type stubBm25Service struct{}

func (stubBm25Service) SearchBm25(_ context.Context, _ Bm25Request) ([]RetrievalChunk, error) {
	return nil, ErrBm25ServiceMissing
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
