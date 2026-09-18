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

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

// Search tools: the retrieval legs plus the narrowing that runs on top.
//
// One call carries both legs — the vector / BM25 split is a weight on the same call —
// so the three entry points below differ only in the defaults they pass.

// Retrieval defaults: callers that supply no configuration get exactly these values.
const (
	DefaultSimilarityThreshold   = 0.2
	DefaultTopN                  = 12
	DefaultRerankCandidatesCount = 64
	DefaultTopK                  = 1024
	// maxEffectiveQueryChars caps the expanded query in CODE POINTS. A byte cap would cut
	// a multi-byte rune in half and, for CJK, would apply a ~133-character cap instead of
	// 400.
	maxEffectiveQueryChars = 400
	// maxQueryTerms caps the terms derived from a query.
	maxQueryTerms = 16
)

// Term-splitting pattern for QueryToTerms.
var (
	// queryMetaRe matches the regex metacharacters blanked out before splitting a query
	// into its literal terms. The split that follows is on WHITE SPACE, never on \w+:
	// Go's RE2 \w is ASCII-only, so tokenizing a CJK phrase with it yields nothing at all
	// and the fan-out BM25 leg loses the keyed terms it needs to weight the
	// discriminating entity.
	queryMetaRe = regexp.MustCompile(`[.*+?^$()\[\]{}]`)

	// DefaultAgenticVectorWeight is the vector leg's weight when agentic
	// retrieval runs keyword-only: zero.
	//
	// The agentic loop runs on keyword matches plus reranking, and nothing turns the
	// embedded leg on: SearchDeps.UsingEmbedding defaults to false, so the weight
	// forwarded is 0.0. Keeping that default is what keeps recall on keyword matches.
	DefaultAgenticVectorWeight = 0.0

	// DefaultHybridVectorWeight is the vector leg's weight when embedding IS used
	// (SearchDeps.UsingEmbedding true): 0.7, the configured default when a caller turns
	// embedding on.
	DefaultHybridVectorWeight = 0.7

	// HybridSearchDefaultVectorWeight is the vector leg's weight for the standalone
	// hybrid_search tool: 0.3 — NOT 0.7 — because that tool reads its own configured
	// default rather than the retrieve channel's.
	HybridSearchDefaultVectorWeight = 0.3

	// VectorSearchDefaultSimilarityThreshold is the engine similarity floor for
	// the pure-vector entry point (search.py:vector_search, 0.2).
	VectorSearchDefaultSimilarityThreshold = 0.2
	// BM25SearchDefaultSimilarityThreshold is the engine similarity floor for
	// bm25_search / grep_search (/395: 0.0).
	BM25SearchDefaultSimilarityThreshold = 0.0
)

// Retriever is the retrieval backend the runtime searches through.
//
// It is an interface so the session and the orchestrator can be tested without a search
// backend, and so the concrete adapter (the runtime root package's RuntimeRetriever)
// stays the only place that knows about the runtime.
type Retriever interface {
	// Retrieve runs one search and returns raw chunk maps. The chunks carry at
	// least chunk_id / content / doc_id / docnm_kwd so the runtime accessors
	// (ChunkIDOf, ChunkTextOf, DocIDOf, ...) can read them.
	Retrieve(ctx context.Context, req RetrieveRequest) ([]map[string]any, error)
}

// DocChunkLister returns a document's chunks in reading order, which is what the
// document-level tools need (position-sorted, strings only).
//
// Retrieval cannot express it: RetrieveRequest ranks by relevance to a query,
// while these tools want every chunk of one document, page-ordered, paged.
// Like DocIDVerifier it is injected, so this package needs no document-store
// dependency of its own. Nil means the tools that need it stay unavailable.
type DocChunkLister interface {
	// DocChunks returns up to Limit chunks of DocID starting at Offset, ordered
	// by reading position. A short page means the document is exhausted.
	DocChunks(ctx context.Context, req DocChunksRequest) ([]map[string]any, error)
}

// DocChunksRequest is one page of one document's chunks.
type DocChunksRequest struct {
	DocID      string
	DatasetIDs []string
	TenantID   string
	Offset     int
	Limit      int
}

// DocIDVerifier reports which of a candidate document id set are known to the session's
// datasets. The lookup is deliberately left to the caller so this package stays free of a
// database dependency.
type DocIDVerifier interface {
	// KnownDocIDs returns the subset of candidates that exist in the given
	// datasets. An error or an unavailable backend yields an empty set, which
	// the caller treats as "verification unavailable" rather than "no match".
	KnownDocIDs(ctx context.Context, datasetIDs, candidates []string) (map[string]bool, error)
}

// DocTenant is the owning (knowledge base, tenant) pair for one document, as returned by
// DocTenantResolver.
type DocTenant struct {
	KBID     string
	TenantID string
}

// DocTenantResolver maps document ids to their owning (kb, tenant). Unlike
// DocIDVerifier, the returned KB/tenant may lie OUTSIDE the caller's
// datasetIDs, so graph_explore can group documents by their real owner and
// search knowledge bases that were not in the original search set. The lookup
// is left to the caller (this package stays database-free); an error or a nil
// resolver makes ExploreGraph fall back to the pre-existing behaviour of
// searching each bound dataset with the whole DocScope.
type DocTenantResolver interface {
	ResolveDocTenants(ctx context.Context, docIDs []string) (map[string]DocTenant, error)
}

// MetadataResolver resolves document sets from document metadata for the metadata_search
// tool. Like DocIDVerifier / DocTenantResolver the lookup is left to the caller, so the
// search legs hold no metadata-index dependency of their own; the production
// implementation is internal/service.MetadataService. Nil makes the metadata channel
// unavailable (the tool reports a clean miss, the pre-search channel is skipped).
//
// Filters are the tool's own {key, value, op} condition maps, kept as maps so the
// implementation needs no import of this package.
type MetadataResolver interface {
	// FilterDocIDsByMetaPushdown resolves the documents whose metadata matches filters
	// (logic = "and" | "or") by pushing the predicate into the document-metadata index.
	//
	// ok=false means the push-down is NOT viable or errored and the caller must fall back
	// to GetFlattedMetaByKBs + the in-memory filter. ok=true with an empty slice is the
	// definitive "no document matches".
	FilterDocIDsByMetaPushdown(ctx context.Context, kbIDs []string, filters []map[string]any, logic string) ([]string, bool)
	// GetFlattedMetaByKBs returns field → value → doc_ids for the given datasets. It is
	// the in-memory filter's input and the source of the "which fields exist" hint that
	// turns "this dataset has no title" into advice instead of an empty retrieval the
	// model retries forever.
	GetFlattedMetaByKBs(ctx context.Context, kbIDs []string) (common.MetaData, error)
}

// RetrieveRequest is one retrieval call.
//
// Weight is the keyword-similarity weight: 0.7 hybrid (default), 0.0 vector-only,
// 1.0 keyword-only (BM25). This is how Python's three search entry points
// (hybrid_search / vector_search / bm25_search) differ.
type RetrieveRequest struct {
	Query                 string
	DatasetIDs            []string
	DocScope              []string
	TopN                  int
	TopK                  int
	RerankCandidatesCount int
	// SimilarityThreshold is a pointer because ZERO is a valid explicit setting
	// (threshold 0 = no floor); nil means "not supplied", so the retriever keeps
	// its own default instead of being handed a zero override.
	SimilarityThreshold *float64
	// KeywordsSimilarityWeight is the keyword leg's weight. The retrieval
	// adapter derives the complementary vector weight and pure-mode behavior.
	KeywordsSimilarityWeight *float64
	TenantID                 string
	// MetaDataFilter restricts retrieval to chunks whose metadata matches
	// Nil means no filtering.
	MetaDataFilter map[string]any
	// RankFeature: `rank_feature` argument
	// (agentic_rag.py:retrieve): question-type tags produced by
	// label_question(question, self.kbs) that the retriever uses to boost
	// matching chunks. The Go engine consumes it as a tag → weight map (matching
	// internal/engine/types and the chat pipeline), so it is map[string]float64,
	// not a bare list. Nil means no rank feature.
	RankFeature map[string]float64
	// ExcludeCompiled excludes compiled-product rows from plain retrieval (the
	// must_not={"exists": "compile_kwd"} filter the hybrid leg applies). Compiled products
	// have their own expansion step, so the base retrieval here should surface ordinary
	// document chunks only. False is what the retrieve channel passes: it does not exclude
	// them.
	ExcludeCompiled bool
}

// intOrDef returns v when set, else fallback.
func intOrDef(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

// floatOrDef returns v when set, else fallback. Kept separate from the pointer
// form below so both "unset" conventions stay explicit at the call site.
func floatOrDef(v, fallback float64) float64 {
	if v > 0 {
		return v
	}
	return fallback
}

// floatPtrOrDef returns *v when configured, else fallback. A pointer is used so
// a configured 0 is honoured instead of looking like "unset".
func floatPtrOrDef(v *float64, fallback float64) float64 {
	if v != nil {
		return *v
	}
	return fallback
}

// SearchChannel identifies which search entry point a call mirrors. The three channels
// are genuinely different search functions rather than one with a flag; collapsing them
// let a single global UsingEmbedding switch silently disable the vector leg for a tool
// that never consulted it.
type SearchChannel int

const (
	// ChannelGrep: the `retrieve`, `grep_search` and `grep_chunks` session tools. It is
	// keyword-only: the grep leg has NO vector leg at all, so the weight is
	// unconditionally 0 and UsingEmbedding must not be consulted.
	ChannelGrep SearchChannel = iota
	// ChannelHybrid is the standalone hybrid search — the `search_chunks` session tool.
	// Its vector leg is gated on an embedder being available (the configured weight when
	// one is, else 0), NOT on UsingEmbedding; production always supplies an embed model,
	// so it runs. Default weight 0.3.
	ChannelHybrid
	// ChannelRetrieve is used by the low/naive direct passes. THIS is the only channel
	// that honours UsingEmbedding (false by default). Default weight 0.7 when embedding is
	// on.
	ChannelRetrieve
)

// resolveKeywordsSimilarityWeight mirrors the weight each Python entry point computes. The
// gate differs per channel — that is the whole point:
//
//   - ChannelGrep: always 1 (grep_search has no vector leg).
//   - ChannelHybrid: 1 when no embedder is configured (Python's `if embd_mdl`),
//     otherwise KeywordsSimilarityWeight ?? 0.7. UsingEmbedding is IRRELEVANT
//     here: Python's hybrid_search has no such parameter.
//   - ChannelRetrieve: 1 unless UsingEmbedding (Python's using_embedding),
//     otherwise KeywordsSimilarityWeight ?? 0.3.
func resolveKeywordsSimilarityWeight(deps SearchDeps, ch SearchChannel) float64 {
	switch ch {
	case ChannelGrep:
		return 1
	case ChannelHybrid:
		// The configured weight when an embedder is available, else 0. HasEmbedder says
		// only that one is available — the retrieval service resolves the actual model.
		if !deps.HasEmbedder {
			return 1
		}
		return floatPtrOrDef(deps.KeywordsSimilarityWeight, 1-HybridSearchDefaultVectorWeight)
	default:
		if !deps.UsingEmbedding {
			return 1
		}
		return floatPtrOrDef(deps.KeywordsSimilarityWeight, 1-DefaultHybridVectorWeight)
	}
}

// SearchDeps are the dependencies of HybridSearch.
type SearchDeps struct {
	// Backend is the retrieval service.
	Backend Retriever
	// KbIDs are the session's bound dataset ids.
	KbIDs []string
	// SQLKBs are the session's structured (SQL-backed) datasets. The hybrid channel
	// merges them into the target id list (the bound KB ids plus every SQL-KB id), so the
	// keyword/vector search spans the structured tables too. They stay separate on
	// SearchDeps and are folded into targetIDs in runSearch.
	SQLKBs []string
	// TenantID scopes the retrieval.
	TenantID string
	// IndexName is the search index that holds the dataset's document/structure rows (the
	// compiled-structure read is routed through it). Empty falls back to
	// "ragflow_<TenantID>", the
	// RAGFlow default — but a tenant that overrides index_name must surface it
	// here or structure/doc reads would hit the wrong index. The earlier Go port
	// hardcoded "ragflow_<tenantID>" and ignored per-tenant overrides.
	IndexName string
	// DocEngine fetches parent chunks during retrieval_by_children (child
	// fragments are promoted to their parent chunk). Nil falls back to
	// engine.Get(). Kept here so runSearch can promote children for the
	// hybrid/retrieve entry points without the Backend guessing which caller
	// wants promotion.
	DocEngine engine.DocEngine
	// KB receives the raw chunks into the lossless memory store BEFORE
	// narrowing, and carries the per-request search cache.
	KB *Kbinfos
	// Expand runs the compiled-structure expansion when UseCompiled is set.
	// Nil disables compiled expansion.
	Expand CompiledExpander
	// NavRouter descends the dataset's compiled navigation tree for the
	// navigate_tree tool. Nil falls back to nav.NewNavServiceRouter() (the
	// internal/service/nav singleton).
	NavRouter NavTreeRouter
	// Model is the request-scoped chat model. It drives
	// the calculate tool's expression-writing call and the graph_explore
	// structure verdict (AskStructure). Required for `calculate`; nil makes the
	// model-backed tools report a miss.
	Model SessionModel
	// Logger is optional; nil uses the default logger. It is the DEVELOPER log:
	// internal diagnostics (search legs, narrowing, compiled expansion) stay
	// here and are not steps. The tool call and its result ARE steps, reported
	// through the per-request reporter StepsFrom(ctx) carries.
	Logger *log.Logger
	// DocIDVerifier resolves which of the caller-supplied document ids actually belong to
	// the session's datasets. Nil means "no verification available": the doc scope is
	// passed through to the retriever unchanged, which is the pre-existing behaviour.
	//
	// It is injected rather than queried here because resolving document
	// ownership is a database lookup this package deliberately does not depend
	// on; the caller (internal/rag/agentic_rag) supplies the implementation.
	DocIDVerifier DocIDVerifier
	// DocChunks pages through one document's chunks for the document-level
	// tools (fetch_full_document / summarize_document). Nil leaves both
	// unavailable.
	DocChunks DocChunkLister
	// Embedder is the external embedding handle used by graph/structure seed encoding.
	// Nil falls back to this package's internal tenant-default resolver, which needs a
	// database and therefore stays self-contained for callers that cannot or do not want
	// to supply one.
	Embedder nlp.NavEmbedder
	// Retrieval tuning (Python RAGTools.retrieve: _setting(self, "top_n"),
	// similarity_threshold, keywords_similarity_weight, rerank_candidates_count,
	// top_k). Zero falls back to this package's Default* constants.
	TopN                int
	SimilarityThreshold float64
	// KeywordsSimilarityWeight is a pointer so an explicit 0 (vector-only) is
	// distinguishable from "not configured", which falls back to the channel's
	// default keyword weight.
	KeywordsSimilarityWeight *float64
	// UsingEmbedding is the Go spelling of Python RAGTools.retrieve's
	// `using_embedding: bool = False` (agentic_rag.py:retrieve). When false (the
	// agentic default), the vector leg is disabled and retrieval is keyword-only
	// (vector weight 0) — exactly Python's `embd_mdl = None; vector_weight = 0`.
	// When true, the embedder is engaged and the vector weight is applied:
	// KeywordsSimilarityWeight if set, else 0.3 (the complement of Python's
	// default vector weight of 0.7). Unlike Python, the
	// query embedder is supplied by the runtime retrieval service, so Go carries
	// the flag rather than an embd_mdl handle on the retrieve call itself.
	//
	// SCOPE: this flag governs ChannelRetrieve ONLY. The hybrid channel (search_chunks)
	// has no such parameter, so gating it here is what silently disabled the semantic
	// leg.
	UsingEmbedding bool
	// HasEmbedder reports whether an embedding handle is available — the gate the hybrid
	// channel applies to its vector leg (the configured weight when one is, else 0).
	// Production always supplies one, so the semantic leg of search_chunks runs. False
	// keeps callers that have no embedding model on keyword-only retrieval.
	HasEmbedder           bool
	RerankCandidatesCount int
	TopK                  int
	// MetaDataFilter restricts retrieval to matching chunk metadata. Nil means no
	// filtering.
	MetaDataFilter map[string]any
	// DocScope is the session-wide document restriction. It is a CEILING applied by
	// scopedDocIDs before any search: an explicit caller scope is intersected with it.
	// Empty means "search everything".
	DocScope []string
	// DocTenantResolver maps document ids to their owning (kb, tenant). Unlike
	// DocIDVerifier (which only answers "belongs to the bound datasets?"), this may
	// return KB/tenant pairs OUTSIDE the caller's datasetIDs, so graph_explore can search
	// documents that belong to other knowledge bases. Nil falls back to the pre-existing
	// behaviour: each bound dataset is searched with the whole DocScope (no per-document
	// re-grouping).
	DocTenantResolver DocTenantResolver
	// MetadataResolver resolves document sets from document metadata, backing the
	// metadata_search tool and the pre-search metadata channel. Nil leaves both
	// unavailable: the tool reports a clean miss and the channel is skipped.
	MetadataResolver MetadataResolver
	// DoRefer: when true, summarize_document
	// prefixes the citation rules so the model cites the blocks it summarises.
	DoRefer bool
	// CiteRules is the optional user-defined citation template. Empty falls back to the
	// embedded citation_prompt.md. summarize_document passes it to the citation header
	// verbatim.
	CiteRules string
	// WebSearch is the optional open-web provider for the `web_search` tool. Nil HIDES
	// the tool from the session surface — Toolset.HasWebSearch is set from
	// mode.HasTool("web_search") && WebSearch != nil. Should a call reach the handler
	// anyway, it reports StatusError/ReasonInfra with a do-not-retry note, never a
	// query-level MISS.
	WebSearch WebSearcher
	// WikiRetriever is the optional provider for the `wiki_query` tool. The tool is
	// advertised in NO mode: its spec is not in the tool map either (see
	// action_session.go). The handler is kept as an unplugged seam and reports a clean
	// MISS (StatusMiss/ReasonNoDoc) when no retriever is wired.
	WikiRetriever WikiRetriever
	// KBs: the resolved Knowledgebase objects (carrying parser_config / tenant_id) the
	// agentic tools run over. The rank feature is derived from these — the retrieve
	// channel labels the question against them. Projected from RAGTools.KBs when the
	// search deps are built.
	KBs []*entity.Knowledgebase
	// Tagger classifies the query into question-type tags the retriever boosts on. Nil
	// means no tag boost. The implementation lives in internal/service
	// (MetadataService.LabelQuestion), which queries the tag service.
	Tagger QuestionLabeler
}

// QuestionLabeler labels a question against the KB objects: given the query and the KBs
// it returns a map of question-type tag → weight the retriever uses to rank results. The
// extractor (extractor_tag.go) writes tag_kwd (the tag-name list) and tag_feas (per-tag
// weights) onto each chunk; the labeler aggregates tag_kwd to build the vocabulary and the
// retriever ranks with tag_feas. No separate tag-library dataset is needed. The production
// implementation is internal/service.MetadataService.LabelQuestion; tests supply a stub.
type QuestionLabeler interface {
	LabelQuestion(ctx context.Context, question string, kbs []*entity.Knowledgebase) map[string]float64
}

// CompiledExpander enriches a search result with the dataset's compiled
// structure (page index / tree / knowledge graph / wiki synthesis pages).
//
// The compiled expansion the hybrid leg runs when use_compiled is set. It is a seam
// rather than a direct call so the search leg stays independent of the compiled-structure
// machinery (and testable without a knowledge graph).
type CompiledExpander interface {
	Expand(ctx context.Context, kb *Kbinfos, query, keywords string, docScope []string) error
}

// QueryToTerms: derive the
// literal terms a query carries. The alternation's segments are split out (capped
// at maxQueryTerms) so the regex locate and the term-based context window fire on
// the same broad set of names; a plain query simply splits on white space. Terms
// shorter than two characters and duplicates are dropped.
//
// Case is PRESERVED (it is never lowercased): the downstream consumers are
// case-insensitive — NarrowByTerms compiles with "(?i)" (grep_sed_narrow.go) and the
// BM25 keyword leg is tokenized server-side — so lowercasing here would only change the
// text handed to them.
func QueryToTerms(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	// Strip the regex decorations an inline query may carry: a flag prefix plus \b and
	// (?i) markers.
	q = strings.TrimPrefix(q, "(?i)")
	q = strings.ReplaceAll(q, `\b`, "")
	q = strings.ReplaceAll(q, "(?i)", "")

	var (
		out  []string
		seen = make(map[string]bool, len(q)/2)
	)
	for _, part := range strings.Split(q, "|") {
		part = strings.TrimSpace(queryMetaRe.ReplaceAllString(part, " "))
		if part == "" {
			continue
		}
		for _, tok := range strings.Fields(part) {
			if utf8.RuneCountInString(tok) < 2 || seen[tok] {
				continue
			}
			seen[tok] = true
			out = append(out, tok)
		}
		if len(out) >= maxQueryTerms {
			break
		}
	}
	if len(out) > maxQueryTerms {
		out = out[:maxQueryTerms]
	}
	return out
}

// FanoutKeyedTerms is the fan-out term filter: the subset of terms worth keying a BM25
// round on. Short stopwords and bare numbers are dropped, but a long number (a year, a
// specimen ID) is discriminative and survives.
func FanoutKeyedTerms(terms []string) []string {
	var out []string
	for _, t := range terms {
		digits := isAllDigits(t)
		// The stopword set is lowercase while the terms themselves keep their case, so
		// "The" must be dropped like "the".
		if (utf8.RuneCountInString(t) >= 3 && !IsStopword(strings.ToLower(t)) && !digits) ||
			(utf8.RuneCountInString(t) >= 4 && digits) {
			out = append(out, t)
		}
	}
	return out
}

// searchOpts carries the per-entry-point differences. Each entry point is its own
// function that builds a searchOpts and delegates to runSearch, so the vector-weight gate,
// similarity threshold and compiled-row exclusion are chosen by the CALLER, never by a
// single shared "channel flag" that a global switch could silently corrupt.
type searchOpts struct {
	// keywordsSimilarityWeight is the keyword leg's similarity weight.
	keywordsSimilarityWeight float64
	// threshold is the engine similarity floor.
	threshold float64
	// excludeCompiled: must_not={"exists":"compile_kwd"}.
	excludeCompiled bool
	// promoteChildren lifts child fragments to their parent chunk after search. EVERY
	// entry point sets it true (hybrid, vector, bm25 and grep — the grep leg builds on the
	// bm25 leg); only a future non-promoting path would leave it false. An empty tenant_id
	// still skips promotion.
	promoteChildren bool
	// logLabel / logVerb / logKeywords describe the entry point's own
	// "searching" line — the LOG form, terse and grep-friendly, with the keywords
	// suffix and no period:
	//
	//	hybrid   → [Hybrid search] Searching the knowledge base for "q" (keywords: k)
	//	vector   → [Vector search] Searching by meaning for "q" (keywords: k)
	//	bm25     → [BM25 search] Searching by keyword for "q" (keywords: k)
	//	grep     → [Grep search] Keyword-first locate for "q"  (NO keywords)
	//	retrieve → [Retrieve] Searching for "q" (keywords: k)  (Go-only tag)
	//
	// Every verb is Python's own (search.py:122/:216/:254/:421), so a log diff
	// against Python lines up — retrieve is the one Go-only tag, and it no longer
	// carries "the knowledge base" (2026-09-16), which every leg searches and which
	// therefore distinguishes nothing a reader can act on. The shape is otherwise
	// unchanged — same tag, same keywords suffix, same `"%q -> N chunk(s): …"`
	// result line.
	//
	// The think block gets thinkVerb through searchThinkLine instead — one sentence
	// family for every leg, with the method named and the keyword list left out. The
	// two sentences come from the same call (runSearch / GrepSearch), so they can
	// never describe different searches.
	//
	// The RESULT line is the other half of a leg (reportSearchResult): every leg
	// emits one, on every exit, including an empty pool. In the log it keeps Python's
	// `"%q -> N chunk(s): <per-doc>"` shape where Python has one (hybrid :203, grep
	// :471); the vector and bm25 legs get it in Go only.
	//
	// The "(keywords: …)" suffix is printed only when non-empty: a keyword-less leg is a
	// deliberate state (see the fan-out's narrow bypass) and search_chunks takes no keywords
	// at all, so a bare "(keywords: )" would read like a dropped argument. logKeywords also
	// keeps the grep leg from growing a suffix it never carries.
	logLabel    string
	logVerb     string
	logKeywords bool
	// thinkVerb is the same leg's verb for the think block, which reports a
	// SENTENCE: it names the method ("by meaning and keyword") because the
	// "[Hybrid search]" prefix is a developer's label, and it is the reason the two
	// projections differ at all. Every leg sets it explicitly — so the log side can
	// be reworded (the Go-only retrieve leg dropped "the knowledge base", above)
	// without the trace moving with it; the fallback to logVerb only covers a future
	// leg that forgets to set one, where an identical sentence beats an empty one.
	thinkVerb string
	// cache enables the per-request search cache (SearchCacheLoad/Store). Only the hybrid
	// leg touches that cache; the vector / bm25 / grep / retrieve legs never consult it.
	// The cache lives on the shared deps.KB, so without this gate a keyword-only leg could
	// be served a
	// hybrid result (different vector weight, threshold and compiled policy)
	// and vice versa — Go's "enhanced" typing did not make it hybrid-scoped.
	cache bool
	// narrowLabel tags the narrowing line. The forms are deliberately UNEVEN: the hybrid
	// leg passes the snake_case "hybrid_search", while the vector and bm25 legs pass their
	// display labels "Vector search" / "BM25 search". Both forms are reproduced verbatim.
	narrowLabel string
	// rankFeature opts the entry point into the question-type tag boost
	// (rank_feature). ONLY the retrieve channel passes the question-type tags; the three
	// search legs call the retriever WITHOUT them (the grep leg delegates to bm25). Before
	// this gate every leg shipped the boost, which re-ranked search_chunks and retrieve
	// results the same way.
	rankFeature bool
}

// searchLogLine renders the "what this leg is searching for" message, WITHOUT
// the stage prefix: the line is a STEP now (StepReporter.StageLine), and that
// call writes "[<label>] " itself from the same string — so the developer log
// stays byte-identical to Python's line (search.py:hybrid_search/215/252/418)
// while the same sentence reaches the think block.
//
// Both the verb and the label vary by entry point. The keywords suffix is
// appended by the caller, because Python prints it on hybrid/vector/bm25 only —
// grep_search's locate line never carries one.
func searchLogLine(verb, question string) string {
	return fmt.Sprintf("%s %q", verb, question)
}

// searchThinkLine renders the same leg's "searching" sentence for the think block:
// the same verb with the query in quotes, plus the period a sentence needs (the
// log line has none — Python's does not) and a length cap (the slot-research
// driver searches a paragraph of evidence as its "query").
//
// It is the log line's sibling, not a replacement: StageLineDetail reports this
// one and logs searchLogLine's, from the same call.
func searchThinkLine(verb, question string) string {
	return fmt.Sprintf("%s %q.", verb, trunc(question, 80))
}

// searchLogger returns the run's logger, falling back to the package logger.
func searchLogger(deps SearchDeps) *log.Logger {
	if deps.Logger != nil {
		return deps.Logger
	}
	return _LOG
}

// runSearch is the shared body of every search entry point. It is deliberately dumb
// about WHICH entry point it is serving: that is encoded by opts, supplied by the
// calling function.
func runSearch(ctx context.Context, deps SearchDeps, p SearchParams, opts searchOpts) ([]map[string]any, []map[string]any) {
	logger := searchLogger(deps)
	// An explicit argument wins, then the caller's configuration, then this package's own
	// defaults.
	topN := p.TopN
	if topN <= 0 {
		topN = deps.TopN
	}
	if topN <= 0 {
		topN = DefaultTopN
	}
	// The structured (SQL) datasets are folded into the target id list. The explicit
	// argument still wins; otherwise the session's kb ids and sql ids are merged, so the
	// vector/keyword search spans the structured tables.
	targetIDs := p.KbIDs
	if len(targetIDs) == 0 {
		// The merge DE-DUPLICATES, so a KB present in both KbIDs and SQLKBs is not searched
		// twice (and the KbIDs themselves are deduped the same way).
		merged := append(append([]string{}, deps.KbIDs...), deps.SQLKBs...)
		seen := make(map[string]struct{}, len(merged))
		targetIDs = targetIDs[:0]
		for _, id := range merged {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			targetIDs = append(targetIDs, id)
		}
	}
	if len(targetIDs) == 0 {
		return nil, nil
	}
	if deps.Backend == nil {
		return nil, nil
	}
	docScope := resolveDocScope(ctx, deps, p.DocScope, targetIDs, logger)

	// 2. Effective query.
	//
	// The 400-char cap applies ONLY to the retrieval_query branch (the entity-weighted
	// expansion appended to the question). The keywords and bare-question branches are NOT
	// truncated.
	var effectiveQuery string
	if strings.TrimSpace(p.RetrievalQuery) != "" {
		// The cap slices code points: a byte slice would both cut a multi-byte rune in
		// half — handing the retriever invalid UTF-8 — and, for CJK, stop at ~133
		// characters, silently dropping two thirds of the expansion terms the fan-out leg
		// weighs on.
		effectiveQuery = truncateRunes(strings.TrimSpace(fmt.Sprintf("%s %s", p.Question, p.RetrievalQuery)), maxEffectiveQueryChars)
	} else if strings.TrimSpace(p.Keywords) != "" {
		effectiveQuery = strings.TrimSpace(fmt.Sprintf("%s %s", p.Question, p.Keywords))
	} else {
		effectiveQuery = p.Question
	}

	// The per-leg searching line: BOTH the bracket label and the verb vary by entry
	// point — "[Hybrid search] Searching the knowledge base for", "[Vector search]
	// Searching by meaning for", "[BM25 search] Searching by keyword for", "[Grep
	// search] Keyword-first locate for" — and only the three keyword-carrying legs
	// append the keywords (see searchOpts.logLabel / logVerb / logKeywords for the
	// Python source of each verb, and for the one Go-only tag that dropped "the
	// knowledge base").
	// The suffix is printed only when non-empty: a keyword-less leg is a
	// deliberate state (fan-out Channel B is a narrow bypass,
	// agentic_rag_graph.py:_search_one; search_chunks takes no keywords at all,
	// action_session.py:execute_tool), so a bare "(keywords: )" reads like a dropped
	// argument rather than an intentional empty.
	searchLine := searchLogLine(opts.logVerb, p.Question)
	if kws := strings.TrimSpace(p.Keywords); opts.logKeywords && kws != "" {
		searchLine += fmt.Sprintf(" (keywords: %s)", kws)
	}
	// A STEP, not only a log line: which leg ran and what it searched for is
	// exactly what the think block has to show to answer "what did it actually
	// search".
	//
	// The two audiences get different sentences from this one call site. The log
	// keeps Python's shape — label, keyword list and result line, plus Python's verb
	// on every leg that has one (only the Go-only retrieve verb deliberately dropped
	// "the knowledge base"; see logVerb above for what a log diff can still be
	// compared against). The think block gets the sentence family every leg
	// shares, which names the method inside the sentence because "[BM25 search]" is
	// a developer's label, and leaves out the keyword list: that appears only on the
	// legs that carry keywords, so in the block it reads as random noise, and it is
	// a detail a reader would have to parse rather than read.
	thinkVerb := opts.thinkVerb
	if thinkVerb == "" {
		thinkVerb = opts.logVerb
	}
	StepsFrom(ctx).StageLineDetail(logger, opts.logLabel,
		searchThinkLine(thinkVerb, p.Question), searchLine)

	// 3. Per-request dedup: an identical query+scope is retrieved at most once,
	// so e.g. pre_search and a claim search asking the same question do not
	// repeat the round-trip, child fetch and narrowing. The cache is hybrid-only — both the
	// lookup and the store belong to that leg — hence opts.cache.
	if deps.KB != nil && opts.cache {
		if chunks, aggs, ok := deps.KB.SearchCacheLoad(SearchCacheKey(effectiveQuery, targetIDs, topN, docScope)); ok {
			logger.Printf("[%s] Already searched this — reusing the %d passage(s) found earlier.", opts.logLabel, len(chunks))
			// A cache hit still hands a pool back, so it still reports one: the
			// dedup line explains WHY nothing was retrieved, this says WHAT the leg
			// contributed (Go-only: Python returns here silently on the result side).
			reportSearchResult(ctx, logger, opts.logLabel, p.Question, chunks)
			return chunks, aggs
		}
	}

	// 4. Retrieve.
	// rank_feature ONLY on the retrieve leg. The hybrid/vector/bm25 legs never pass it, so
	// the other entry points leave the request's RankFeature nil. It is computed from the
	// KB objects via the injected Tagger; a nil Tagger ⇒ no boost.
	var rankFeature map[string]float64
	if opts.rankFeature && deps.Tagger != nil {
		rankFeature = deps.Tagger.LabelQuestion(ctx, effectiveQuery, deps.KBs)
	}
	// Neither of the leg's two lines is emitted here: the searching line is above
	// and the result line at the end (reportSearchResult). Emitting either one here
	// as well would show the same search twice in the reasoning block.
	// D5: a retrieval that FAILS is retried once before the query is written off. The
	// failure this exists for is the upstream one: measured (2026-09-16) two queries of a
	// run answered `SILICONFLOW API error: 503 Service Unavailable … Model service
	// overloaded`, and the passages those queries would have returned were simply gone
	// from a question whose whole work is coverage. One retry costs one round trip on the
	// failure path only, and it is a retry of the same request (no re-planning, nothing
	// cached, nothing narrowed yet).
	retrieve := func() ([]map[string]any, error) {
		return deps.Backend.Retrieve(ctx, RetrieveRequest{
			Query:                    effectiveQuery,
			DatasetIDs:               targetIDs,
			DocScope:                 docScope,
			TopN:                     topN,
			TopK:                     intOrDef(deps.TopK, DefaultTopK),
			RerankCandidatesCount:    max(intOrDef(deps.RerankCandidatesCount, DefaultRerankCandidatesCount), topN),
			SimilarityThreshold:      &opts.threshold,
			KeywordsSimilarityWeight: &opts.keywordsSimilarityWeight,
			TenantID:                 deps.TenantID,
			MetaDataFilter:           deps.MetaDataFilter,
			RankFeature:              rankFeature,
			ExcludeCompiled:          opts.excludeCompiled,
		})
	}
	chunks, err := retrieve()
	if err != nil {
		// Go-only line: the retriever's failure is reported here instead of propagating.
		logger.Printf("[%s] retrieval failed: %v — retrying once.", opts.logLabel, err)
		chunks, err = retrieve()
	}
	if err != nil {
		logger.Printf("[%s] retrieval failed twice: %v", opts.logLabel, err)
		return nil, nil
	}

	// 4b. retrieval_by_children: child fragments are promoted to their parent chunk right
	// after retrieval. ALL entry points do this, so every runSearch caller sets
	// promoteChildren: true. The promotion lives here (gated by opts.promoteChildren)
	// rather than in the shared Backend so a future non-promoting path can opt out; an
	// empty tenant_id skips it.
	if opts.promoteChildren {
		de := deps.DocEngine
		if de == nil {
			de = engine.Get()
		}
		if de != nil && len(chunks) > 0 {
			chunks = nlp.RetrievalByChildren(chunks, []string{deps.TenantID}, de, ctx)
		}
	}

	// 5. doc_aggs from the FULL retrieved candidate set: computed here and left untouched
	// by the narrowing / compiled-expansion steps below, which only ever replace the chunk
	// list.
	aggs := DocAggs(chunks)

	// Memory BEFORE narrowing: the raw corpus may hold a fact the narrowing
	// drops, and a gap-driven grep over memory recovers it without re-querying.
	if deps.KB != nil {
		MemoryAdd(deps.KB, chunks)
	}

	// 6. Narrow-or-keep (chunks only; doc_aggs stays as retrieved).
	chunks = NarrowOrKeep(ctx, chunks, p.Keywords, opts.narrowLabel, logger)

	// 7. Compiled expansion.
	if p.UseCompiled && len(chunks) > 0 && deps.Expand != nil {
		logger.Printf("[%s] Compiled expansion enabled — enriching with page_index/tree/KG navigation.", opts.logLabel)
		if err := deps.Expand.Expand(ctx, deps.KB, p.Question, p.Keywords, docScope); err != nil {
			logger.Printf("[%s] compiled expansion failed: %v", opts.logLabel, err)
		}
	}

	// What the leg FOUND, reported once, on every path (see reportSearchResult).
	reportSearchResult(ctx, logger, opts.logLabel, p.Question, chunks)

	// 8. Cache the result — hybrid leg only.
	if deps.KB != nil && opts.cache {
		deps.KB.SearchCacheStore(SearchCacheKey(effectiveQuery, targetIDs, topN, docScope), chunks, aggs)
	}
	return chunks, aggs
}

// HybridSearch: the vector leg is gated on an embedder being configured (the configured
// weight when one is, else 0); default weight 0.3. Compiled rows are excluded.
func HybridSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	return runSearch(ctx, deps, p, searchOpts{
		keywordsSimilarityWeight: resolveKeywordsSimilarityWeight(deps, ChannelHybrid),
		threshold:                floatOrDef(deps.SimilarityThreshold, DefaultSimilarityThreshold),
		excludeCompiled:          true,
		promoteChildren:          true,
		logLabel:                 "Hybrid search",
		logVerb:                  "Searching the knowledge base for",
		logKeywords:              true,
		cache:                    true, // Python's search_cache is read/written here only (:136/:207)
		thinkVerb:                "Searching by meaning and keyword for",
		// Python passes the SNAKE_CASE tag to _narrow_or_keep on this leg
		// (search.py:hybrid_search) although its own lines say "Hybrid search" — the
		// inconsistency is Python's and is reproduced verbatim.
		narrowLabel: "hybrid_search",
	})
}

// MetadataDocIDs resolves the documents a metadata filter matches: metadata-index
// push-down (ES / Infinity) -> in-memory filter when the push-down is not viable ->
// intersect with the session document scope.
//
// ok=false means NO document is eligible — nothing matched, or the session scope removed
// every match. That is a normal empty result, never an error: the caller reports a
// query-level miss. The resolution is query-independent, so callers resolve once and then
// search inside the returned documents.
func MetadataDocIDs(ctx context.Context, deps SearchDeps, filters []map[string]any, logic string) ([]string, bool) {
	logger := searchLogger(deps)
	if len(filters) == 0 || deps.MetadataResolver == nil {
		return nil, false
	}
	targetIDs := metadataTargetIDs(deps)
	if len(targetIDs) == 0 {
		return nil, false
	}
	docIDs, pushdownOK := deps.MetadataResolver.FilterDocIDsByMetaPushdown(ctx, targetIDs, filters, logic)
	if !pushdownOK {
		metas, err := deps.MetadataResolver.GetFlattedMetaByKBs(ctx, targetIDs)
		if err != nil {
			logger.Printf("[Metadata search] push-down unavailable and the in-memory fallback failed: %v", err)
			return nil, false
		}
		docIDs = metaFilterDocIDs(metas, filters, logic)
	}
	if len(docIDs) == 0 {
		logger.Printf("[Metadata search] no documents matched filters=%v logic=%s", filters, logic)
		return nil, false
	}
	// The session's document scope is a ceiling: a document outside it stays unreachable
	// even when its metadata matches.
	scoped := scopedDocIDs(deps.DocScope, docIDs)
	if len(scoped) == 0 {
		logger.Printf("[Metadata search] 0 documents after the doc-scope intersection")
		return nil, false
	}
	return scoped, true
}

// MetadataSearch: hybrid retrieval restricted to the documents a metadata filter matches
// (MetadataDocIDs + HybridSearch), the leg the pre-search metadata channel runs.
//
// Compiled expansion stays OFF: it would pull in out-of-scope chunks and break the "only
// these documents" contract.
func MetadataSearch(ctx context.Context, deps SearchDeps, p SearchParams, filters []map[string]any, logic string) ([]map[string]any, []map[string]any) {
	docIDs, ok := MetadataDocIDs(ctx, deps, filters, logic)
	if !ok {
		return nil, nil
	}
	searchLogger(deps).Printf("[Metadata search] %q searching %d matched doc(s) via filters=%v", trunc(p.Question, 80), len(docIDs), filters)
	return HybridSearch(ctx, deps, SearchParams{
		Question:    p.Question,
		Keywords:    p.Keywords,
		DocScope:    docIDs,
		TopN:        p.TopN,
		UseCompiled: false,
	})
}

// metadataTargetIDs is the deduplicated dataset id list a metadata lookup spans: the
// session's bound datasets plus its structured (SQL-backed) ones.
func metadataTargetIDs(deps SearchDeps) []string {
	merged := append(append([]string{}, deps.KbIDs...), deps.SQLKBs...)
	out := make([]string, 0, len(merged))
	seen := make(map[string]struct{}, len(merged))
	for _, id := range merged {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// metaFilterDocIDs is the in-memory metadata filter used when the index push-down is not
// viable. The condition conversion and the operator normalisation stay in
// internal/service, where the chat pipeline's own filter shares them.
func metaFilterDocIDs(metas common.MetaData, filters []map[string]any, logic string) []string {
	if len(metas) == 0 || len(filters) == 0 {
		return nil
	}
	conditions := make([]service.MetaFilterCondition, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			continue
		}
		conditions = append(conditions, service.MetaFilterCondition{
			Key:   asString(f["key"]),
			Value: f["value"],
			Op:    asString(f["op"]),
		})
	}
	if len(conditions) == 0 {
		return nil
	}
	return service.ApplyMetaFilter(metas, conditions, logic)
}

// VectorSearch: the pure vector entry point. With no embedder it returns nothing,
// otherwise the vector weight is 1.0 and compiled rows are excluded.
func VectorSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	if !deps.HasEmbedder {
		return nil, nil
	}
	return runSearch(ctx, deps, p, searchOpts{
		keywordsSimilarityWeight: 0,
		threshold:                VectorSearchDefaultSimilarityThreshold,
		excludeCompiled:          true,
		promoteChildren:          true,
		logLabel:                 "Vector search",
		logVerb:                  "Searching by meaning for",
		logKeywords:              true,
		thinkVerb:                "Searching by meaning for",
		narrowLabel:              "Vector search",
	})
}

// BM25Search: Keyword-only: the
// vector weight is unconditionally 0, the similarity floor is 0.0, and compiled
// rows are excluded.
func BM25Search(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	return runSearch(ctx, deps, p, searchOpts{
		keywordsSimilarityWeight: 1,
		threshold:                BM25SearchDefaultSimilarityThreshold,
		excludeCompiled:          true,
		promoteChildren:          true,
		logLabel:                 "BM25 search",
		logVerb:                  "Searching by keyword for",
		logKeywords:              true,
		thinkVerb:                "Searching by keyword for",
		narrowLabel:              "BM25 search",
	})
}

// patternRecallTopN is how wide ONE operand's recall goes when the query is a
// STRUCTURAL pattern — `关公.*斩`, a question about how a thing is written rather
// than about a name.
//
// A pattern is a LOCATOR, and a locator is only as good as the ground it is given:
// recalling ten passages answers "does this pattern occur in those ten", not
// "where does it occur". Recalling only a small topN per operand lets a pattern match
// inside a handful of passages of a corpus where the subject alone occurs in a large part
// of the text. That is the one retrieval path that does not depend on the model already
// knowing the name, and a narrow recall has it looking through a keyhole.
//
// It is the widest per-operand number the pipeline already uses elsewhere
// (SCAViewCap), and only the MATCHED windows travel onwards — the grep output cap
// (GrepOutTotalChars) still bounds what the model pays for.
//
// 200, because the binding constraint is the recall bound rather than the corpus: an
// operand's own match count runs well past a small topN, so a pattern's recall stops before
// the corpus does and the passages past a ranking's head are never matched against the
// pattern at all — and those are exactly the windows that hold the members nobody has
// named. The number is a RECALL bound on candidates that are only matched and narrowed (the
// model sees the matched windows, capped by GrepOutTotalChars), so widening it costs
// retrieval, not prompt — and a pattern whose operand hits the bound is logged rather than
// silently truncated (see retrieveGrepCandidates).
const patternRecallTopN = 200

// isStructuralPattern reports whether a query asks for structure (any-wildcard
// pattern) rather than listing terms: `关公.*斩` does, `华雄|颜良` does not.
func isStructuralPattern(query string) bool {
	return strings.Contains(query, ".*") || strings.Contains(query, ".+")
}

// callerBatch reports whether the caller wrote a BATCH — several terms in one
// query — rather than a sentence.
//
// Three shapes count, and they are the three the model actually writes: an
// alternation, a pattern (whose operands are the terms), and whitespace-separated CJK
// pieces. The last one is the reason this predicate exists rather than a `|` test: the model
// writes that batch shape and never writes a `|`, so a `|`-only gate would keep the
// per-term seat allocation permanently off.
//
// A sentence is excluded on purpose, in both languages: an English question
// splits on whitespace but carries no CJK, and a Chinese question has no
// whitespace to split on. Those are the questions that are not enumerating
// anything, and they must not pay for per-term searches.
func callerBatch(query string) bool {
	if strings.Contains(query, "|") || grepPatternOf(query) != nil {
		return true
	}
	pieces := 0
	for _, field := range strings.Fields(query) {
		if !hasCJK(field) {
			continue
		}
		pieces++
		if pieces >= 2 {
			return true
		}
	}
	return false
}

// retrieveGrepCandidates fetches the candidate set the locate step then narrows.
//
// One ranked search answers "what does this query match", which is the wrong question for
// an alternation. A batch of names asks WHICH of them the corpus carries, and a single
// top-N hands nearly every seat to the passages that match many of the terms at once: the
// rarest names — the ones the probe was made for — never reach the pool, and the round
// answers short while the probe itself had done its job.
//
// So an alternation searches each term on its own, and the results are woven
// term-by-term, best hit first. The caller's per-query cap then keeps ONE hit
// per term before it keeps a second hit for any single term — which is the
// shape an enumeration needs: one passage per name, and a name that comes back
// empty is a name the corpus does not carry. That empty answer is the point of
// the probe, so it is returned, not filled with the hits the other terms found.
//
// The weave is bounded by the terms (GrepTermsMax) and by each search's own
// TopN, and the narrowing stage's char budget still decides how much of the
// woven set survives.
//
// The trigger is the CALLER's BATCH, not the `|` character: the model writes batches of
// names separated by spaces and no `|`, so a `|`-only trigger would leave this whole
// function dead code while the batches it was written for went to a single ranked top-N.
// A batch is an alternation, a pattern, or two
// or more CJK pieces separated by whitespace; a sentence in either language is
// none of those, which is what keeps the extra searches off the questions that
// are not enumerating anything.
func retrieveGrepCandidates(
	ctx context.Context,
	deps SearchDeps,
	bp SearchParams,
	query string,
	terms []string,
) ([]map[string]any, []map[string]any) {
	if !callerBatch(query) || len(terms) < 2 {
		return BM25Search(ctx, deps, bp)
	}

	perTerm := make([][]map[string]any, 0, len(terms))
	aggs := make([]map[string]any, 0, len(terms))
	perTopN := bp.TopN
	if isStructuralPattern(query) && perTopN < patternRecallTopN {
		perTopN = patternRecallTopN
	}
	for _, term := range terms {
		// The term alone is the query: on a keyword leg one rare token is the
		// strongest query there is, and the rest of the alternation can only
		// dilute it.
		sub := bp
		sub.Question = term
		sub.Keywords = term
		sub.TopN = perTopN
		chunks, docAggs := BM25Search(ctx, deps, sub)
		if isStructuralPattern(query) && len(chunks) >= perTopN {
			// A pattern is a LOCATOR over what the keyword leg recalled, so a full page means
			// it was matched against a TRUNCATED candidate set: matches past this bound are not
			// "absent from the corpus", they were never looked at. Said out loud because the
			// difference decides whether a member the pattern did not show is a corpus fact or
			// a ceiling (see patternRecallTopN for the measurement).
			searchLogger(deps).Printf("[Grep search] operand %q filled its recall bound (%d passage(s)); matches beyond it were not matched against the pattern.", term, perTopN)
		}
		perTerm = append(perTerm, chunks)
		aggs = append(aggs, docAggs...)
		// A term searched ON ITS OWN is the caller's probe of ONE individual, so
		// a passage that carries it is a confirmed member with its evidence —
		// recorded as such, because the round needs the members (and the passages
		// behind them), not just the number of chunks it holds.
		//
		// Except when the caller IS the runtime: the completeness pass searches the
		// actor and the act words, which are not names and must not enter the record's
		// to-do list (see SearchParams.SkipReachLedger).
		if !bp.SkipReachLedger {
			for _, c := range chunks {
				if strings.Contains(strings.ToLower(ChunkTextOf(c)), strings.ToLower(term)) {
					deps.KB.RecordReachedTerm(term, ChunkIDOf(c))
					break
				}
			}
		}
	}

	seen := make(map[string]bool)
	out := make([]map[string]any, 0, len(terms))
	for depth := 0; ; depth++ {
		any := false
		for _, chunks := range perTerm {
			if depth >= len(chunks) {
				continue
			}
			any = true
			c := chunks[depth]
			if id := ChunkIDOf(c); id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			out = append(out, c)
		}
		if !any {
			break
		}
	}
	// One line, only for a batch — this is the fingerprint that says the per-term
	// seats were actually allocated, so a run can be read for whether the trigger
	// fired at all (the failure mode this replaced was a mechanism that never ran).
	searchLogger(deps).Printf("[Grep search] batch of %d term(s): each searched on its own, %d candidate(s) woven",
		len(terms), len(out))
	return out, aggs
}

// ProbeSeatTopN bounds how many candidates one term's seat search takes before
// the window is picked: a seat exists to carry the name, not to rank it.
const ProbeSeatTopN = 3

// TermSeat runs ONE cheap keyword search for a single named term and returns the
// passage that carries it, narrowed to that term's own window, or (nil, false)
// when nothing reached it.
//
// This is the retrieval unit an enumeration needs, and it is deliberately not a
// query SYNTAX: the caller's terms may arrive as an alternation ("A|B|C"), as a
// space-separated list inside one string, or as a list of query strings, and the
// seat is the same thing in all three cases. What it replaces is asking for
// several individuals at once: one ranked search hands its seats to the passages
// that match MANY of the named terms, so the rarest name — the reason the call
// was made — is the one that loses: most queries in a call are cut by the per-query cap,
// and the candidates that did come back were flattened to one hit each, so the rarest names
// are missing from an answer whose retrieval had returned tens of candidates per query.
//
// The term alone is the query, on the keyword leg only: no vector leg, no
// compiled expansion, no model call — a few hundred milliseconds, which is what
// lets the caller afford one per named term.
//
// The boolean is the OTHER half of the answer: false means this corpus reached
// nothing for that term, which is a fact about the corpus the run must keep
// (Kbinfos.RecordProbedAbsent) rather than a failed lookup to retry.
func TermSeat(ctx context.Context, deps SearchDeps, base SearchParams, term string) ([]map[string]any, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, false
	}
	sub := base
	sub.Question = term
	sub.Keywords = term
	sub.TopN = ProbeSeatTopN
	sub.UseCompiled = false
	chunks, _ := BM25Search(ctx, deps, sub)
	if len(chunks) == 0 {
		return nil, false
	}
	res := NarrowByTerms(chunks, []string{term}, nil, term,
		NarrowContext{Before: 1, After: 0}, GrepOutCharsPerChunk, GrepOutTotalChars)
	if len(res.Kept) > 0 {
		return res.Kept[:1], true
	}
	// The keyword leg returned candidates that do not carry the term: on a
	// keyword leg that is the corpus answering "not here", so the seat is empty
	// rather than filled with the nearest passages.
	return nil, false
}

// GrepSearch: a keyword-first locate that runs the bm25 leg and then narrows the prose
// candidates to the term-grep window (regex locate + short line-context).
//
// Behaviour:
//   - BM25 candidate pool built from the query's extracted terms — or the explicit
//     keywords hint when one is supplied — never a
//     hard filter: the pool still spans the corpus.
//   - Table chunks pass through UN-narrowed (full text): the grep window would
//     truncate them to a header-only snippet and hide mid/late-table answer rows.
//   - Prose chunks are narrowed via NarrowByTerms with context {before:1,
//     after:0}, per-chunk 700-char and total 8000-char caps.
//   - When grep matches nothing, the raw BM25 candidates are returned unchanged so
//     evidence is never dropped (enumeration / multi-hop must not lose candidates).
func GrepSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	logger := searchLogger(deps)
	query := strings.TrimSpace(p.Question)
	// Python logs the locate line BEFORE extracting the terms and before the
	// empty-query bail-out (search.py:grep_search).
	//
	// Same split as runSearch: the log keeps Python's phrasing ("Keyword-first
	// locate for", search.py:418) and the think block says what the leg does — find
	// these exact words — in the family the other legs use. "Keyword-first locate"
	// was a noun phrase naming the implementation, not an action a reader could read.
	StepsFrom(ctx).StageLineDetail(logger, "Grep search",
		searchThinkLine("Searching for the exact words", query),
		searchLogLine("Keyword-first locate for", query))
	// Python prints its result line at the END of grep_search, reading whatever the
	// function is about to return (`_g = res.get("chunks"); if _g:`, :471-477) — so
	// every early return below gets one too. Reporting through this closure is what
	// keeps that true: the count is always of the RETURNED set, which for the
	// fallbacks below is the raw candidate pool or the table chunks, not the pieces
	// the term window happened to keep.
	report := func(returned []map[string]any) {
		reportSearchResult(ctx, logger, "Grep search", query, returned)
	}
	if query == "" {
		// Nothing was searched, so there is no result to report: the searching line
		// above already says the query was empty, and "Found nothing for \"\"" would
		// read like a search that ran and came back empty.
		return nil, nil
	}
	terms := GrepTermsFromQuery(query)
	pattern := grepPatternOf(query)
	if pattern != nil {
		// The operands of a pattern ARE its recall terms (see GrepPatternOperands):
		// the phrase reading of the same string would treat "关公.*斩" as one clause
		// and lose half of it.
		terms = GrepPatternOperands(query)
	}
	// The WEAVE searches the caller's own WORDS; `terms` above is the LOCATOR's
	// list, and for an unbroken CJK clause it is a set of two-rune windows (see
	// GrepWordsFromQuery).
	//
	// A window searched on its own spends a retrieval on a fragment nobody
	// proposed — and, because a term searched on its own records what it reached,
	// it also entered the reach ledger. The session then reads that ledger back as
	// its to-do list: the windows of an unbroken clause are recorded as if they were probes,
	// so the ledger fills with fragments the question is made of, while the names the round is
	// actually missing are not on it at all.
	//
	// A pattern keeps its operands: those are already the caller's words, and its
	// recall needs the long ones (`关公.*斩` searches 关公 and 斩).
	weave := terms
	if pattern == nil {
		weave = GrepWordsFromQuery(query)
	}
	// The grep leg then delegates to the bm25 leg with an explicit keywords hint: the
	// caller's keywords when supplied, else the extracted terms joined. A long question
	// buries its proper nouns under stopwords; without the hint the noun chunk never enters
	// the candidate pool and grep has nothing to locate. It is a SOFT boost — the pool
	// still spans the corpus. Delegating (rather than calling runSearch) is also what
	// produces the log sequence: the bm25 leg logs its searching line and narrows under
	// that tag, then this leg logs its own two lines.
	hint := strings.TrimSpace(p.Keywords)
	if hint == "" {
		hint = strings.Join(terms, " ")
	}
	bp := p
	if grepPatternOf(query) != nil {
		// A pattern is an expression for the MATCHER, not for the engine: handed
		// over, `|` and `.*` become either literals to escape or syntax of their
		// own, and a batch probe that the engine echoes back empty is then read as
		// "the corpus does not carry it". So recall gets the OPERANDS, which is
		// what a keyword leg can actually search for, and the pattern stays here
		// and decides which of the candidates are evidence.
		bp.Question = strings.Join(terms, " ")
		bp.Keywords = bp.Question
	} else {
		bp.Question = query
		bp.Keywords = hint
	}
	chunks, docAggs := retrieveGrepCandidates(ctx, deps, bp, query, weave)
	// No candidates or no terms: there is nothing to locate.
	if len(chunks) == 0 || len(terms) == 0 {
		report(chunks)
		return chunks, docAggs
	}

	var table, prose []map[string]any
	for _, c := range chunks {
		if IsTableChunk(c) {
			table = append(table, c)
		} else {
			prose = append(prose, c)
		}
	}
	if len(prose) == 0 {
		// All tables: nothing to locate, and the tables ARE the evidence.
		report(chunks)
		return chunks, docAggs
	}

	// A query that carries pattern syntax is read as a PATTERN and matched against
	// the candidates the keyword leg returned (see matchGrepPattern): `A|B` is how
	// a batch of names is asked about, `A.*B` is how the way a thing was done is
	// asked about, and neither can be expressed by locating terms one at a time.
	// The pattern itself never leaves this function — the engine only ever saw the
	// operands — so `|` and `.*` are operators here rather than characters somebody
	// has to escape.
	if pattern != nil {
		kept, matched := matchGrepPattern(prose, pattern, contextCharBudget, GrepOutTotalChars)
		if matched == 0 {
			// The pattern matched nothing in what was reached: keep the raw
			// candidates so evidence is not dropped, and SAY what happened — a
			// silent return is how "the pattern did not occur here" gets read as
			// "the corpus does not carry it".
			logGrepReach(logger, query, chunks, terms)
			return chunks, docAggs
		}
		out := make([]map[string]any, 0, len(table)+len(kept))
		out = append(out, table...)
		out = append(out, kept...)
		chars := 0
		for _, c := range out {
			chars += len(ChunkTextOf(c))
		}
		logger.Printf("[Grep search] pattern %q matched %d/%d candidate(s) -> %d chunk(s), %.1fK chars.",
			trunc(query, 80), matched, len(prose), len(out), float64(chars)/1000.0)
		logGrepReach(logger, query, chunks, terms)
		return out, docAggs
	}

	res := NarrowByTerms(prose, terms, nil, query, NarrowContext{Before: 1, After: 0},
		GrepOutCharsPerChunk, GrepOutTotalChars)
	kept := res.Kept
	if len(kept) == 0 {
		// Nothing matched: keep the raw BM25 candidates so evidence is not dropped
		// — and report THOSE, because they are what the caller receives.
		report(chunks)
		return chunks, docAggs
	}
	out := make([]map[string]any, 0, len(table)+len(kept))
	out = append(out, table...)
	out = append(out, kept...)
	// Python logs the post-narrow size and the per-doc breakdown for the
	// combined table+prose set (search.py:grep_search / :469) — the grep leg's own
	// line, which only exists on this path (the five above return before it). It is
	// a LOG line, not a step: the think block's result sentence comes from report
	// below, like every other leg's.
	chars := 0
	for _, c := range out {
		chars += len(ChunkTextOf(c))
	}
	logger.Printf("[Grep search] narrowed %d->%d chunk(s), %.1fK chars.", len(chunks), len(out), float64(chars)/1000.0)
	report(out)
	return out, docAggs
}

// RetrieveSearch: the low/naive direct pass. Unlike HybridSearch it honours
// UsingEmbedding and does NOT exclude compiled rows.
func RetrieveSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	return runSearch(ctx, deps, p, searchOpts{
		keywordsSimilarityWeight: resolveKeywordsSimilarityWeight(deps, ChannelRetrieve),
		threshold:                floatOrDef(deps.SimilarityThreshold, DefaultSimilarityThreshold),
		excludeCompiled:          false,
		promoteChildren:          true,
		// Go-only identity: Python's L1 RAGTools.retrieve
		// logs nothing of its own, so there is no Python string to mirror. The
		// tag stays human-readable and the extra lines stay on, because this is
		// the only trace the low-mode direct pass leaves.
		logLabel:    "Retrieve",
		logVerb:     "Searching for",
		logKeywords: true,
		thinkVerb:   "Searching for",
		narrowLabel: "retrieve",
		// The ONLY entry point that passes the question-type tag boost.
		rankFeature: true,
	})
}

// webSearchMaxChunks is the hard ceiling on admitted web passages per call: each query
// is capped at 8 chunks and at most two queries run, so 16 is the maximum.
const webSearchMaxChunks = 16

// WebSearchTool: the `web_search` tool. Its schema takes a 1-2 element array of query
// strings (see webSearchToolSpec), so args is the positional query list, not a key/value
// object. Results share the RAGFlow chunk shape so they merge into the SAME shared
// evidence pool as corpus hits, and the model sees the same REDUNDANT/OK/MISS outcome as
// the corpus tools.
func WebSearchTool(ctx context.Context, deps SearchDeps, args map[string]any) (ToolOutcome, error) {
	if deps.WebSearch == nil {
		// An absent provider is an infra ERROR with an explicit do-not-retry note (returning
		// MISS would let the model retry the same dead tool and burn turns), never a
		// query-level miss.
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "web_search",
				"note": "Web search is NOT configured for this session. Do not use this tool again; use the corpus tools (retrieve / search_chunks / navigate_*) instead.",
			}},
			Status:  StatusError,
			Reason:  ReasonInfra,
			Metrics: map[string]any{},
		}, nil
	}
	// The query list is read from args["query"]. The old ""-key read never found anything,
	// so every call with a proper {"query": [...]} payload was rejected as bad_args.
	queries := toolStringList(args, "query")
	if len(queries) == 0 {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	// Cap at two queries.
	if len(queries) > 2 {
		queries = queries[:2]
	}
	results, err := deps.WebSearch.Search(ctx, queries)
	if err != nil || len(results) == 0 {
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "web_search",
				"note": "No web result returned.",
			}},
			Status:  StatusMiss,
			Reason:  ReasonNoDoc,
			Metrics: map[string]any{},
		}, nil
	}
	// Each query's chunks are capped at 8; with up to two queries that is a hard ceiling of
	// 16 web passages per tool call.
	if len(results) > webSearchMaxChunks {
		results = results[:webSearchMaxChunks]
	}

	// Build RAGFlow-shaped chunk maps and admit them to the shared evidence pool, mirroring
	// the corpus tools' merge + outcome. Corpus passages are de-duplicated across the (up to
	// two) queries by chunk_id; the web provider surfaces raw strings, so de-dup by content
	// to keep the same "each distinct passage appears once" behaviour.
	var payload []any
	var evidenceIDs []string
	newChunks := 0
	seen := make(map[string]bool, len(results))
	// The whole batch is ONE critical section (Kbinfos.Admit): the admit loop has no await
	// (the retrieval itself is awaited above it), so two sessions can never interleave
	// here.
	deps.KB.Admit(func(p *PoolAdmitter) {
		// The claim-coverage skip is provably unreachable here: web passages carry synthetic
		// "web_N" ids, which no claim's source_chunk_ids can reference.
		for i, r := range results {
			if r == "" || seen[r] {
				continue
			}
			// Admission early-stops at the pool cap, BEFORE the chunk is recorded as seen, so
			// a rejected passage is retried once room frees.
			if p.Full() {
				continue
			}
			chunkID := fmt.Sprintf("web_%d", i)
			seen[r] = true
			c := map[string]any{
				"chunk_id": chunkID,
				"content":  r,
				"doc_id":   "web",
			}
			// p.Add, not Merge: Merge takes this same pool lock and would deadlock
			// inside the critical section. It also answers "new to the pool" per
			// chunk, so the count no longer has to be inferred from a length delta
			// that other sessions' appends inflate.
			if p.Add(c) {
				newChunks++
			}
			// Evidence references the chunk id, not a pool position.
			evidenceIDs = append(evidenceIDs, chunkID)
			// Passage shape: {"id","content"} (no doc_id), non-table text cut to 1200 code
			// points (plain slice, no ellipsis).
			content := r
			if !IsTableChunk(c) {
				content = truncateRunes(content, 1200)
			}
			payload = append(payload, map[string]any{"id": chunkID, "content": content})
		}
	})

	if len(payload) == 0 {
		return ToolOutcome{Payload: []any{}, Status: StatusMiss, Reason: ReasonNoDoc, Metrics: map[string]any{"hits": 0, "new_evidence": 0}}, nil
	}
	status := StatusOK
	if newChunks == 0 {
		status = StatusRedundant
	}
	return ToolOutcome{
		Payload:     payload,
		EvidenceIDs: evidenceIDs,
		Status:      status,
		Metrics:     map[string]any{"hits": len(payload), "new_evidence": newChunks},
	}, nil
}

// list_chunks deep-read caps.
const (
	// listChunksMaxDeep caps the deep-read pool admission.
	listChunksMaxDeep = 80
	// listChunksMaxOut caps the model-facing passage list.
	listChunksMaxOut = 30
)

// listChunks deep-reads one document's FULL text and admits it to the shared evidence
// pool.
//
// The read goes through deps.DocChunks (the same reader fetch_full_document /
// summarize_document use), not the evidence pool, so navigate_tree can route a
// doc_id whose chunks were never retrieved; when DocChunks is unwired it falls
// back to the pool so the tool still works in unit tests.
//
// Outcome semantics mirror _search_outcome: an empty result (blank doc_id, unknown
// doc, or nothing read) is a MISS — not an error; a non-empty result is REDUNDANT
// when nothing new entered the pool, else OK. The deep read is capped at
// listChunksMaxDeep and the model-facing output at listChunksMaxOut.
func (e *searchExecutor) listChunks(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	docID := argString(args, "doc_id")
	if docID == "" {
		// An empty doc_id is a query-level MISS.
		return ToolOutcome{Payload: []any{}, Status: StatusMiss, Reason: ReasonNoDoc, Metrics: map[string]any{"hits": 0}}, nil
	}

	var chunks []map[string]any
	if e.deps.DocChunks != nil {
		// Real doc-store deep read, budgeted by the model window.
		deep, _ := fetchFullDocument(ctx, e.deps, docID, e.req.MaxLength)
		if len(deep) > listChunksMaxDeep {
			deep = deep[:listChunksMaxDeep]
		}
		chunks = deep
	} else if e.deps.KB != nil {
		// Fallback: scan the in-memory evidence pool (unit-test / unwired path).
		for _, c := range e.deps.KB.Chunks {
			if DocIDOf(c) == docID {
				chunks = append(chunks, c)
			}
		}
	}

	if len(chunks) == 0 {
		// Unknown doc_id / out-of-scope doc, or nothing readable: a query-level MISS. There
		// is no "graceful OK empty" branch.
		return ToolOutcome{Payload: []any{}, Status: StatusMiss, Reason: ReasonNoDoc, Metrics: map[string]any{"hits": 0, "new_evidence": 0}}, nil
	}

	// The pool is CREATED when absent, so the admittance below always has somewhere to
	// write.
	if e.deps.KB == nil {
		e.deps.KB = &Kbinfos{}
	}

	// Only the first listChunksMaxOut (30) of the listChunksMaxDeep (80) fetched passages
	// are admitted: no-id chunks and in-call duplicates are skipped, evidence references
	// the chunk id, and only chunks new to the shared pool are appended.
	admit := chunks
	if len(admit) > listChunksMaxOut {
		admit = admit[:listChunksMaxOut]
	}
	seen := map[string]bool{}
	var payload []any
	var evidenceIDs []string
	newChunks := 0
	// The batch is ONE critical section (Kbinfos.Admit): the admit stretch has no await, so
	// two sessions can never interleave here, and the pool-side dedup must see the LIVE
	// pool rather than a snapshot taken upfront.
	e.deps.KB.Admit(func(p *PoolAdmitter) {
		covered := p.ClaimCoveredIDs()
		for _, c := range admit {
			cid := ChunkIDOf(c)
			if cid == "" {
				// A blank cid is skipped in the caller, BEFORE admission.
				continue
			}
			// Admission early-stops at the pool cap, BEFORE the per-call dedup.
			if p.Full() {
				continue
			}
			if seen[cid] {
				continue
			}
			// already quoted verbatim by a pooled claim →
			// skip the full passage (table chunks exempt).
			if p.CoveredByClaim(cid, covered, IsTableChunk(c)) {
				continue
			}
			seen[cid] = true
			evidenceIDs = append(evidenceIDs, cid)
			// Pool shape: {"id","content"} (no doc_id), with non-table text cut to 1200 code
			// points (a plain slice, no ellipsis).
			content := ChunkTextOf(c)
			if !IsTableChunk(c) {
				content = truncateRunes(content, 1200)
			}
			payload = append(payload, map[string]any{"id": cid, "content": content})
			if p.Add(c) {
				newChunks++
			}
		}
	})

	if len(payload) == 0 {
		return ToolOutcome{Payload: []any{}, Status: StatusMiss, Reason: ReasonNoDoc, Metrics: map[string]any{"hits": 0, "new_evidence": 0}}, nil
	}
	// A deep read COVERS its claims: once the full chunk text is in the pool, the claim's
	// 1200-char quote of the same passage is duplicated tokens in every later prompt.
	// Retire claim pseudo-chunks whose source chunk was just read.
	if retired := e.deps.KB.RetireClaimsCoveredBy(evidenceIDs); retired > 0 {
		searchLogger(e.deps).Printf("[list_chunks] retired %d covered claim(s) from the evidence pool", retired)
	}
	status := StatusOK
	if newChunks == 0 {
		status = StatusRedundant
	}
	return ToolOutcome{
		Payload:     payload,
		EvidenceIDs: evidenceIDs,
		Status:      status,
		Reason:      ReasonNone,
		Metrics:     map[string]any{"hits": len(payload), "new_evidence": newChunks},
	}, nil
}

// SearchCacheKey: the tuple of what actually
// determines a retrieval result. Scope and limits are part of the key so only a
// genuinely identical query is served from cache.
func SearchCacheKey(effectiveQuery string, targetIDs []string, topN int, docScope []string) string {
	query := normalizeSpace(effectiveQuery)
	ids := append([]string(nil), targetIDs...)
	sort.Strings(ids)
	docs := append([]string(nil), docScope...)
	sort.Strings(docs)
	return fmt.Sprintf("%s|%v|%d|%v", query, ids, topN, docs)
}

func normalizeSpace(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// docInDatasets reports whether docID belongs to the datasets being searched.
//
// The second result is false only when ownership could NOT be determined — no
// verifier injected, or the lookup errored. It is true when the lookup
// completed, including the "document absent from the bound datasets" case:
// KnownDocIDs returns a known-subset map, so a foreign/stale doc_id yields an
// empty map (never an error), and is reported as belongs=false / verified=true so the
// caller rejects it rather than fetching it unscoped.
func docInDatasets(ctx context.Context, deps SearchDeps, docID string) (belongs, verified bool) {
	if deps.DocIDVerifier == nil || docID == "" {
		return true, false
	}
	known, err := deps.DocIDVerifier.KnownDocIDs(ctx, deps.KbIDs, []string{docID})
	if err != nil {
		// Fail open only on an actual lookup failure; never on "not found".
		return true, false
	}
	return known[docID], true
}

// scopedDocIDs
// the ceiling every search entry point applies before touching the doc store:
// with no session scope the caller's scope passes through; with a session scope
// an absent caller scope falls back to it and an explicit one is INTERSECTED
// with it, so a model-generated doc_scope can never escape the session's
// document restriction.
func scopedDocIDs(sessionScope, scope []string) []string {
	if len(sessionScope) == 0 {
		return scope
	}
	if len(scope) == 0 {
		return append([]string(nil), sessionScope...)
	}
	allowed := make(map[string]bool, len(sessionScope))
	for _, d := range sessionScope {
		allowed[d] = true
	}
	out := make([]string, 0, len(scope))
	for _, d := range scope {
		if allowed[d] {
			out = append(out, d)
		}
	}
	return out
}

// resolveDocScope: apply the session doc_scope ceiling, then keep only the requested
// document ids that actually belong to the session's datasets.
//
// The ceiling: a None request scope falls back to the session's fixed doc_scope
// (deps.DocScope), and an explicit request scope is intersected with it, so a session
// bound to a fixed document set keeps searching within it whichever scope the tool
// passes.
//
// Three outcomes are deliberate, because the document scope is supplied by
// model-generated tool arguments and must never silently change what is searched:
//
//   - verification unavailable (no verifier injected, or it errored): the scope is
//     passed through unchanged (fail open);
//   - every candidate unknown AND a fixed session base scope: return an EMPTY result,
//     never unfiltered retrieval;
//   - every candidate unknown with NO session base scope: drop the scope and search
//     UNFILTERED.
//
// The empty-vs-unfiltered distinction is encoded as a non-nil empty slice vs
// nil: the backend treats a nil DocScope as "search everything" and a non-nil
// empty DocScope as "match nothing".
func resolveDocScope(ctx context.Context, deps SearchDeps, scope, kbIDs []string, logger *log.Logger) []string {
	scope = scopedDocIDs(deps.DocScope, scope)
	if len(scope) == 0 && len(deps.DocScope) > 0 {
		// The session ceiling removed every requested id. Passing the empty scope through
		// would be read as "no filter" and search unfiltered; Go deliberately keeps the
		// safer "match nothing" instead of letting a model-supplied scope escape the
		// session's document restriction.
		return []string{}
	}
	if deps.DocIDVerifier == nil {
		// No verifier: trust the scope as given (fail open).
		return scope
	}
	candidates := make([]string, 0, len(scope))
	for _, d := range scope {
		if d != "" {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		// No usable candidate ids (request scope None and no session base scope, or only
		// empty strings): search unfiltered. Return nil, not an empty set, so the backend
		// does not filter to zero documents.
		return nil
	}
	known, err := deps.DocIDVerifier.KnownDocIDs(ctx, kbIDs, candidates)
	if err != nil {
		logger.Printf("[Hybrid search] document-id verification unavailable (%d candidate(s)); searching the scope as given", len(candidates))
		return scope
	}
	valid := make([]string, 0, len(candidates))
	for _, d := range candidates {
		if known[d] {
			valid = append(valid, d)
		}
	}
	if len(valid) == 0 {
		// Every candidate is unknown. Two outcomes are distinguished by the SESSION base
		// scope:
		//   - a fixed session doc_scope → return an empty result (no documents match);
		//   - no session base scope → fall back to UNFILTERED retrieval.
		if len(deps.DocScope) > 0 {
			return []string{}
		}
		logger.Printf("[Hybrid search] every supplied doc ID was unknown; falling back to unfiltered retrieval")
		return nil
	}
	return valid
}

// DocAggs builds the per-document aggregation from a chunk list: each entry carries
// doc_id (the key the reporting layer dedups on) plus the counts it reads. The Retriever
// interface returns only chunks, so the aggregations are derived here.
func DocAggs(chunks []map[string]any) []map[string]any {
	if len(chunks) == 0 {
		return nil
	}
	type stat struct {
		count int
		chars int
		name  string
	}
	order := make([]string, 0, 8)
	stats := map[string]*stat{}
	for _, c := range chunks {
		if c == nil {
			continue
		}
		id := DocIDOf(c)
		if id == "" {
			id = DocTitleOf(c)
		}
		if id == "" {
			id = "?"
		}
		s, ok := stats[id]
		if !ok {
			s = &stat{name: DocTitleOf(c)}
			stats[id] = s
			order = append(order, id)
		}
		s.count++
		s.chars += len(ChunkTextOf(c))
	}
	out := make([]map[string]any, 0, len(order))
	for _, id := range order {
		s := stats[id]
		// Field names match runtime's referenceDocAggsFromRetrieval
		// (doc_id / doc_name / count) so these aggregations can be handed
		// straight to CanvasState.SetRetrievalReferences without translation.
		out = append(out, map[string]any{
			"doc_id":     id,
			"doc_name":   s.name,
			"count":      s.count,
			"char_count": s.chars,
		})
	}
	return out
}

// searchResultLine is the think-block form of a search leg's result: how many
// passages came back and how many documents they came from, as a sentence. The
// query comes last (that is where a reader looks after the numbers) and there is
// no arrow: "->" is log shorthand, not English, and this line is read by users.
// An empty result says so in words ("Found nothing for …") rather than counting
// zero documents.
//
// It is deliberately NOT docStatsLine — that one is keyed by 32-hex document id
// (a developer's handle, and the line Python prints), which is exactly what a
// reader cannot use. Both are emitted by the same step (StageLineDetail), from
// the same chunks, so the sentence and the id breakdown always describe the same
// call.
func searchResultLine(query string, chunks []map[string]any) string {
	if len(chunks) == 0 {
		return fmt.Sprintf("Found nothing for %q.", trunc(query, 80))
	}
	return fmt.Sprintf("Found %s in %s for %q.", CountOf(len(chunks), "passage"),
		CountOf(len(DocAggs(chunks)), "document"), trunc(query, 80))
}

// searchResultLogLine renders the same result in the log's shape:
// `"%q -> N chunk(s): <per-document breakdown>"` (Python search.py:203 for hybrid,
// :471 for grep). The breakdown is dropped when the leg returned nothing, so a
// zero-result line does not end on a dangling colon.
func searchResultLogLine(query string, chunks []map[string]any) string {
	line := fmt.Sprintf("%q -> %d chunk(s)", trunc(query, 80), len(chunks))
	if stats := docStatsLine(chunks); stats != "" {
		line += ": " + stats
	}
	return line
}

// reportSearchResult is how a leg ends: it reports what the leg found, to both
// audiences, from one place.
//
// It runs on EVERY exit — an empty result and a cached one included — because a leg
// with no result line cannot be told apart from a leg that found nothing, and that
// is the one question the block exists to answer.
//
// The log keeps Python's line shape where Python has one. The vector and bm25 legs
// are the exception: Python logs nothing there beyond the searching line
// (search.py:215/252), and Go deliberately adds the result line, because those legs
// are the ones whose pool size a reader most needs.
func reportSearchResult(ctx context.Context, logger *log.Logger, label, query string, chunks []map[string]any) {
	StepsFrom(ctx).StageLineDetail(logger, label,
		searchResultLine(query, chunks), searchResultLogLine(query, chunks))
}

// docStatsLine renders the per-document breakdown used by the search log line.
func docStatsLine(chunks []map[string]any) string {
	parts := make([]string, 0, 8)
	for _, a := range DocAggs(chunks) {
		parts = append(parts, fmt.Sprintf("%s:%vchunk(%vchars)",
			a["doc_id"], a["count"], a["char_count"]))
	}
	return strings.Join(parts, "; ")
}

// Normalisation of web-search payloads.
//
// Web results are shaped so they enter kbinfos with the same field names every other
// chunk carries.

// normalizeWebResults converts a raw web-search JSON payload into chunk maps.
// Accepts either {"chunks": [...]} or {"results": [...]}; entries without both a
// URL and content are dropped.
func normalizeWebResults(raw []byte) []map[string]any {
	var res struct {
		Chunks  []map[string]any `json:"chunks"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil
	}
	src := res.Chunks
	if len(src) == 0 {
		src = res.Results
	}
	out := make([]map[string]any, 0, len(src))
	for i, c := range src {
		url := firstNonEmpty(anyString(c["url"]), anyString(c["link"]), anyString(c["source"]))
		if url == "" {
			continue
		}
		content := firstNonEmpty(anyString(c["content"]), anyString(c["raw_content"]), anyString(c["text"]))
		if content == "" {
			continue
		}
		docID := anyString(c["doc_id"])
		if docID == "" {
			docID = url
		}
		// doc_id stays the source URL; chunk_id must be UNIQUE per snippet so
		// Kbinfos.Merge (dedup by chunkKey) does not collapse several snippets
		// from the same URL, or the same URL across two retrieval rounds.
		chunkID := fmt.Sprintf("%s#%d", docID, i)
		out = append(out, map[string]any{
			"chunk_id":            chunkID,
			"content_with_weight": content,
			"doc_id":              docID,
			"docnm_kwd":           firstNonEmpty(anyString(c["title"]), anyString(c["source"])),
			"dataset_id":          anyString(c["dataset_id"]),
			"url":                 url,
			"source":              "web",
		})
	}
	return out
}

// firstNonEmpty returns the first argument that is not blank.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// Per-request retrieval cache.
//
// The cache is keyed by the search parameters; an identical (query, scope, limits)
// search is served from cache instead of hitting the retriever again. It is attached to
// Kbinfos and exposed as methods here, so the search tool and its cache live in one
// file.

// searchCache is the per-request retrieval cache.
type searchCache struct {
	mu      sync.Mutex
	entries map[string]cachedSearch
}

// cachedSearch is one cache entry.
type cachedSearch struct {
	chunks []map[string]any
	aggs   []map[string]any
}

// SearchCacheLoad returns a cached retrieval for key, or (nil, nil, false) when
// the key is absent.
func (k *Kbinfos) SearchCacheLoad(key string) ([]map[string]any, []map[string]any, bool) {
	c := k.ensureCache()
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, nil, false
	}
	return e.chunks, e.aggs, true
}

// SearchCacheStore records a retrieval under key.
func (k *Kbinfos) SearchCacheStore(key string, chunks, aggs []map[string]any) {
	c := k.ensureCache()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]cachedSearch)
	}
	c.entries[key] = cachedSearch{chunks: chunks, aggs: aggs}
}

// ensureCache lazily initialises the per-request cache so a zero-value Kbinfos
// is usable without an explicit setup step.
func (k *Kbinfos) ensureCache() *searchCache {
	k.cacheOnce.Do(func() {
		if k.cache == nil {
			k.cache = &searchCache{}
		}
	})
	return k.cache
}
