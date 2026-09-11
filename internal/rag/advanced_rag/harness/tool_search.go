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

package harness

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

	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/service/nlp"
)

// Search tools: the retrieval legs plus the narrowing that runs on top.
//
// Mirrors Python harness/tools/search.py::hybrid_search (with the vector /
// BM25 legs expressed as a weight on the same call, as the Python module does
// for its three entry points).

// Retrieval defaults. Mirrors Python search.py's _DEFAULT_* constants; callers
// that supply no configuration get exactly these values.
const (
	DefaultSimilarityThreshold   = 0.2
	DefaultTopN                  = 12
	DefaultRerankCandidatesCount = 64
	DefaultTopK                  = 1024
	// maxEffectiveQueryChars caps the expanded query in CODE POINTS: Python
	// slices a str with `[:400]` (search.py:129/216/254), which counts code
	// points. A byte cap would cut a multi-byte rune in half and, for CJK, would
	// apply a ~133-character cap instead of 400.
	maxEffectiveQueryChars = 400
	// maxQueryTerms caps the terms derived from a query (Python
	// _query_to_terms: [:16]).
	maxQueryTerms = 16
)

// Term-splitting pattern for QueryToTerms (Python _query_to_terms).
var (
	// queryMetaRe matches the regex metacharacters Python blanks out before
	// splitting a query into its literal terms (search.py:_query_to_terms). The split that
	// follows is on WHITE SPACE, never on \w+: Go's RE2 \w is ASCII-only, so
	// tokenizing "关羽 斩杀" with it yields nothing at all and the fan-out BM25
	// leg loses the keyed terms it needs to weight the discriminating entity
	// (agentic_rag_graph.py:_search_one).
	queryMetaRe = regexp.MustCompile(`[.*+?^$()\[\]{}]`)

	// DefaultAgenticVectorWeight is the vector leg's weight when agentic
	// retrieval runs keyword-only: zero.
	//
	// This mirrors Python's RAGTools.retrieve, which takes
	// `using_embedding: bool = False` (agentic_rag.py:retrieve) and where no caller
	// passes True, so `embd_mdl` stays None and the weight it forwards is 0.0
	// (agentic_rag.py:retrieve). The agentic loop therefore runs on keyword
	// matches plus reranking; matching that is what keeps Go's recall aligned
	// with Python's. SearchDeps.UsingEmbedding is the Go spelling of that flag.
	DefaultAgenticVectorWeight = 0.0

	// DefaultHybridVectorWeight is the vector leg's weight when embedding IS
	// used (Python RAGTools.retrieve with using_embedding=True). Python reads it
	// from `_setting(self, "vector_similarity_weight", 0.7)` (agentic_rag.py:retrieve);
	// 0.7 is the configured default when a caller turns embedding on.
	DefaultHybridVectorWeight = 0.7

	// HybridSearchDefaultVectorWeight is the vector leg's weight for the
	// standalone hybrid_search tool (Python search_chunks → hybrid_search). It
	// defaults to _DEFAULT_HYBRID_VECTOR_WEIGHT = 0.3 — NOT 0.7 —
	// because the tool reads `_setting(tools, "vector_similarity_weight", 0.3)`.
	HybridSearchDefaultVectorWeight = 0.3

	// VectorSearchDefaultSimilarityThreshold is the engine similarity floor for
	// the pure-vector entry point (search.py:vector_search, 0.2).
	VectorSearchDefaultSimilarityThreshold = 0.2
	// BM25SearchDefaultSimilarityThreshold is the engine similarity floor for
	// bm25_search / grep_search (/395: 0.0).
	BM25SearchDefaultSimilarityThreshold = 0.0
)

// Retriever is the retrieval backend the harness searches through.
//
// Mirrors Python's `settings.retriever.retrieval(...)`, which the harness calls
// as a module-level singleton. The Go version takes it as an interface so the
// session and the orchestrator can be tested without a search backend, and so
// the concrete adapter (the harness root package's RuntimeRetriever) stays the
// only place that knows about the runtime.
type Retriever interface {
	// Retrieve runs one search and returns raw chunk maps. The chunks carry at
	// least chunk_id / content / doc_id / docnm_kwd so the harness accessors
	// (ChunkIDOf, ChunkTextOf, DocIDOf, ...) can read them.
	Retrieve(ctx context.Context, req RetrieveRequest) ([]map[string]any, error)
}

// DocChunkLister returns a document's chunks in reading order, which is what
// the document-level tools need (Python settings.retriever.chunk_list with
// sort_by_position=True, rag/nlp/search.py:_only_strings).
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

// DocIDVerifier reports which of a candidate document id set are known to the
// session's datasets. Mirrors Python RAGTools._filter_known_doc_ids, whose
// lookup is deliberately left to the caller so this package stays free of a
// database dependency.
type DocIDVerifier interface {
	// KnownDocIDs returns the subset of candidates that exist in the given
	// datasets. An error or an unavailable backend yields an empty set, which
	// the caller treats as "verification unavailable" rather than "no match".
	KnownDocIDs(ctx context.Context, datasetIDs, candidates []string) (map[string]bool, error)
}

// DocTenant is the owning (knowledge base, tenant) pair for one document, as
// returned by DocTenantResolver. Mirrors Python tools._resolve_doc_tenant,
// which yields (kb, tenant) for a document id.
type DocTenant struct {
	KBID     string
	TenantID string
}

// DocTenantResolver maps document ids to their owning (kb, tenant), mirroring
// Python tools._resolve_doc_tenant (exploration.py:_kg_scopes). Unlike
// DocIDVerifier, the returned KB/tenant may lie OUTSIDE the caller's
// datasetIDs, so graph_explore can group documents by their real owner and
// search knowledge bases that were not in the original search set. The lookup
// is left to the caller (this package stays database-free); an error or a nil
// resolver makes ExploreGraph fall back to the pre-existing behaviour of
// searching each bound dataset with the whole DocScope.
type DocTenantResolver interface {
	ResolveDocTenants(ctx context.Context, docIDs []string) (map[string]DocTenant, error)
}

// RetrieveRequest is one retrieval call.
//
// Weight is the vector-similarity weight: 0.3 hybrid (default), 1.0 vector-only,
// 0.0 keyword-only (BM25). This is how Python's three search entry points
// (hybrid_search / vector_search / bm25_search) differ.
type RetrieveRequest struct {
	Query                 string
	DatasetIDs            []string
	DocScope              []string
	TopN                  int
	TopK                  int
	RerankCandidatesCount int
	// SimilarityThreshold and KeywordsSimilarityWeight are pointers because ZERO
	// is a valid explicit setting (threshold 0 = no floor; weight 0 = the vector
	// leg gets the whole weight). nil means "not supplied", so the retriever
	// keeps its own default instead of being handed a zero override.
	SimilarityThreshold *float64
	// KeywordsSimilarityWeight is named for keywords but carries the VECTOR
	// weight, as in Python's vector_similarity_weight.
	KeywordsSimilarityWeight *float64
	TenantID                 string
	// MetaDataFilter restricts retrieval to chunks whose metadata matches
	// (Python tools.meta_data_filter). Nil means no filtering.
	MetaDataFilter map[string]any
	// RankFeature mirrors Python RAGTools.retrieve's `rank_feature` argument
	// (agentic_rag.py:retrieve): question-type tags produced by
	// label_question(question, self.kbs) that the retriever uses to boost
	// matching chunks. The Go engine consumes it as a tag → weight map (matching
	// internal/engine/types and the chat pipeline), so it is map[string]float64,
	// not a bare list. Nil means no rank feature (Python passes None).
	RankFeature map[string]float64
	// ExcludeCompiled excludes compiled-product rows from plain retrieval
	// (Python hybrid_search passes must_not={"exists": "compile_kwd"},
	// search.py:hybrid_search). Compiled products have their own expansion step, so the
	// base retrieval here should surface ordinary document chunks only. False
	// matches Python RAGTools.retrieve, which does not exclude them.
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

// SearchChannel identifies which Python search entry point a call mirrors. Python
// has three genuinely different search functions rather than one with a flag;
// collapsing them let a single global UsingEmbedding switch silently disable the
// vector leg for a tool that never consulted using_embedding.
type SearchChannel int

const (
	// ChannelGrep mirrors Python grep_search — the `retrieve`, `grep_search`
	// and `grep_chunks` session tools (action_session.py:_exec_retrieve :737-741). It is
	// keyword-only: grep_search has NO vector leg at all, so the weight is
	// unconditionally 0 and UsingEmbedding must not be consulted.
	ChannelGrep SearchChannel = iota
	// ChannelHybrid mirrors the standalone hybrid_search — the `search_chunks`
	// session tool (action_session.py:_exec_search_chunks, search.py:hybrid_search). Its vector
	// leg is gated on `tools.embed_mdl` being set
	// (`vector_weight = _setting(...) if embd_mdl else 0`), NOT on
	// using_embedding; production always supplies an embed model, so it runs.
	// Default weight 0.3 (search.py:_DEFAULT_HYBRID_VECTOR_WEIGHT).
	ChannelHybrid
	// ChannelRetrieve mirrors Python RAGTools.retrieve,
	// used by the low/naive direct passes. THIS is the only function that
	// honours `using_embedding: bool = False` (:600, :643), which is what
	// UsingEmbedding spells. Default weight 0.7 when embedding is on.
	ChannelRetrieve
)

// resolveVectorWeight mirrors the weight each Python entry point computes. The
// gate differs per channel — that is the whole point:
//
//   - ChannelGrep: always 0 (grep_search has no vector leg).
//   - ChannelHybrid: 0 when no embedder is configured (Python's `if embd_mdl`),
//     otherwise VectorSimilarityWeight ?? 0.3. UsingEmbedding is IRRELEVANT
//     here: Python's hybrid_search has no such parameter.
//   - ChannelRetrieve: 0 unless UsingEmbedding (Python's using_embedding),
//     otherwise VectorSimilarityWeight ?? 0.7.
func resolveVectorWeight(deps SearchDeps, ch SearchChannel) float64 {
	switch ch {
	case ChannelGrep:
		return 0
	case ChannelHybrid:
		// Python: vector_weight = _setting(...) if embd_mdl else 0.
		// HasEmbedder is the Go spelling of `tools.embed_mdl`; the runtime
		// retrieval service resolves the actual model, as Python's retriever
		// does from embd_mdl.
		if !deps.HasEmbedder {
			return 0
		}
		return floatPtrOrDef(deps.VectorSimilarityWeight, HybridSearchDefaultVectorWeight)
	default:
		if !deps.UsingEmbedding {
			return 0
		}
		return floatPtrOrDef(deps.VectorSimilarityWeight, DefaultHybridVectorWeight)
	}
}

// SearchDeps are the dependencies of HybridSearch.
type SearchDeps struct {
	// Backend is the retrieval service.
	Backend Retriever
	// KbIDs are the session's bound dataset ids (Python tools.kb_ids).
	KbIDs []string
	// SQLKBs are the session's structured (SQL-backed) datasets (Python
	// tools.sql_kbs). Python hybrid_search merges them into the target id list
	// (search.py:hybrid_search: `tools.kb_ids + [kb.id for kb in tools.sql_kbs]`), so the
	// keyword/vector search spans the structured tables too. Go keeps them
	// separate on SearchDeps and folds them into targetIDs in runSearch.
	SQLKBs []string
	// TenantID scopes the retrieval.
	TenantID string
	// IndexName is the search index that holds the dataset's document/structure
	// rows (Python search.index_name; navigation.py:_navigate_structure_impl routes the compiled
	// structure read through it). Empty falls back to "ragflow_<TenantID>", the
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
	// Model is the request-scoped chat model (Python tools.chat_mdl). It drives
	// the calculate tool's expression-writing call and the graph_explore
	// structure verdict (AskStructure). Required for `calculate`; nil makes the
	// model-backed tools report a miss.
	Model SessionModel
	// Logger is optional; nil uses the default logger.
	Logger *log.Logger
	// DocIDVerifier resolves which of the caller-supplied document ids actually
	// belong to the session's datasets (Python
	// RAGTools._filter_known_doc_ids). Nil means "no verification available":
	// the doc scope is passed through to the retriever unchanged, which is the
	// pre-existing behaviour.
	//
	// It is injected rather than queried here because resolving document
	// ownership is a database lookup this package deliberately does not depend
	// on; the caller (internal/rag/advanced_rag) supplies the implementation.
	DocIDVerifier DocIDVerifier
	// DocChunks pages through one document's chunks for the document-level
	// tools (fetch_full_document / summarize_document). Nil leaves both
	// unavailable.
	DocChunks DocChunkLister
	// Embedder is the external embedding handle used by graph/structure seed
	// encoding (Python tools.embed_mdl). Nil falls back to this package's
	// internal tenant-default resolver, which needs a database and therefore
	// stays self-contained for callers that cannot or do not want to supply one.
	Embedder nlp.NavEmbedder
	// Retrieval tuning (Python RAGTools.retrieve: _setting(self, "top_n"),
	// similarity_threshold, vector_similarity_weight, rerank_candidates_count,
	// top_k). Zero falls back to this package's Default* constants.
	TopN                int
	SimilarityThreshold float64
	// VectorSimilarityWeight is a pointer so an explicit 0 (keyword-only, the
	// agentic default) is distinguishable from "not configured", which falls
	// back to DefaultAgenticVectorWeight.
	VectorSimilarityWeight *float64
	// UsingEmbedding is the Go spelling of Python RAGTools.retrieve's
	// `using_embedding: bool = False` (agentic_rag.py:retrieve). When false (the
	// agentic default), the vector leg is disabled and retrieval is keyword-only
	// (vector weight 0) — exactly Python's `embd_mdl = None; vector_weight = 0`.
	// When true, the embedder is engaged and the vector weight is applied:
	// VectorSimilarityWeight if set, else DefaultHybridVectorWeight (0.7, Python
	// `_setting(self, "vector_similarity_weight", 0.7)`). Unlike Python, the
	// query embedder is supplied by the runtime retrieval service, so Go carries
	// the flag rather than an embd_mdl handle on the retrieve call itself.
	//
	// SCOPE: this flag governs ChannelRetrieve ONLY. Python's using_embedding
	// lives on RAGTools.retrieve and nowhere else — hybrid_search (search_chunks)
	// has no such parameter, so gating it here is what silently disabled the
	// semantic leg.
	UsingEmbedding bool
	// HasEmbedder is the Go spelling of Python `tools.embed_mdl` being set —
	// the gate hybrid_search applies to its vector leg
	// (search.py:_setting: `vector_weight = _setting(...) if embd_mdl else 0`).
	// Production always supplies one (dialog_service.py:rag_agent embed_mdl=embd_mdl),
	// so the semantic leg of search_chunks runs. False keeps callers that have no
	// embedding model on keyword-only retrieval, as Python does.
	HasEmbedder           bool
	RerankCandidatesCount int
	TopK                  int
	// MetaDataFilter restricts retrieval to matching chunk metadata (Python
	// tools.meta_data_filter). Nil means no filtering.
	MetaDataFilter map[string]any
	// DocScope is the session-wide document restriction (Python
	// RAGTools.doc_scope). It is a CEILING applied by scopedDocIDs before any
	// search: an explicit caller scope is intersected with it. Empty means
	// "search everything".
	DocScope []string
	// DocTenantResolver maps document ids to their owning (kb, tenant),
	// mirroring Python tools._resolve_doc_tenant. Unlike DocIDVerifier (which
	// only answers "belongs to the bound datasets?"), this may return KB/tenant
	// pairs OUTSIDE the caller's datasetIDs, so graph_explore can search
	// documents that belong to other knowledge bases. Nil falls back to the
	// pre-existing behaviour: each bound dataset is searched with the whole
	// DocScope (no per-document re-grouping).
	DocTenantResolver DocTenantResolver
	// DoRefer mirrors Python RAGTools.do_refer: when true, summarize_document
	// prefixes the citation rules so the model cites the blocks it summarises.
	DoRefer bool
	// CiteRules is the optional user-defined citation template. Empty falls back
	// to the embedded citation_prompt.md (mirrors Python user_defined_prompts).
	// summarize_document passes it to the citation header verbatim.
	CiteRules string
	// WebSearch is the optional open-web provider for the `web_search` tool. Nil
	// HIDES the tool from the session surface — Toolset.HasWebSearch is set from
	// mode.HasTool("web_search") && WebSearch != nil, mirroring Python's
	// provider gate (action_session.py:_disable_tool). Should a call reach the handler
	// anyway, it reports StatusError/ReasonInfra with a do-not-retry note, never a
	// query-level MISS (WebSearchTool:826).
	WebSearch WebSearcher
	// WikiRetriever is the optional provider for the `wiki_query` tool. The tool is
	// advertised in NO mode: its spec was dropped from ToolMap to match Python,
	// which never registers wiki_query either (see action_session.go). The
	// handler is kept as an unplugged seam and reports a clean MISS
	// (StatusMiss/ReasonNoDoc) when no retriever is wired.
	WikiRetriever WikiRetriever
	// KBs mirrors Python RAGTools' self.kbs — the resolved
	// Knowledgebase objects (carrying parser_config / tenant_id) the agentic
	// tools run over. The rank feature is derived from these, exactly as
	// Python's retrieve calls label_question(question, self.kbs). Projected
	// from RAGTools.KBs when the search deps are built.
	KBs []*entity.Knowledgebase
	// Tagger mirrors Python RAGTools.retrieve's
	// `rank_feature=label_question(question, self.kbs)` (agentic_rag.py:retrieve): it
	// classifies the query into question-type tags the retriever boosts on. Nil
	// means no tag boost (Python's label_question returning None). The Go
	// implementation lives in internal/service (MetadataService.LabelQuestion),
	// matching Python's rag.app.tag.label_question which queries the tag service.
	Tagger QuestionLabeler
}

// QuestionLabeler mirrors Python rag.app.tag.label_question(question, kbs)
// (agentic_rag.py:retrieve → tag.py). Given the query and the KB objects (which
// carry parser_config.tag_kb_ids), it returns a map of question-type tag →
// weight the retriever uses to rank results. The production implementation is
// internal/service.MetadataService.LabelQuestion; tests supply a stub.
type QuestionLabeler interface {
	LabelQuestion(ctx context.Context, question string, kbs []*entity.Knowledgebase) map[string]float64
}

// CompiledExpander enriches a search result with the dataset's compiled
// structure (page index / tree / knowledge graph / wiki synthesis pages).
//
// Mirrors Python tools/compiled_expansion.py::_expand_with_compiled, which
// hybrid_search calls when use_compiled is set. It is a seam rather than a
// direct call so the search leg stays independent of the compiled-structure
// machinery (and testable without a knowledge graph).
type CompiledExpander interface {
	Expand(ctx context.Context, kb *Kbinfos, query, keywords string, docScope []string) error
}

// QueryToTerms mirrors Python _query_to_terms: derive the
// literal terms a query carries. The alternation's segments are split out (capped
// at maxQueryTerms) so the regex locate and the term-based context window fire on
// the same broad set of names; a plain query simply splits on white space. Terms
// shorter than two characters and duplicates are dropped.
//
// Case is PRESERVED, as in Python (it never lowercases): the downstream consumers
// are case-insensitive — NarrowByTerms compiles with "(?i)"
// (grep_sed_narrow.go, Python re.IGNORECASE) and the BM25 keyword leg is
// tokenized server-side — so lowercasing here would only change the text handed
// to them.
func QueryToTerms(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	// Strip the regex decorations Python removes (search.py:_query_to_terms): an inline
	// flag prefix plus \b and (?i) markers.
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

// FanoutKeyedTerms mirrors Python's fan-out term filter
// (agentic_rag_graph.py:_fanout_search): the subset of terms worth keying a BM25 round on.
// Short stopwords and bare numbers are dropped, but a long number (a year, a
// specimen ID) is discriminative and survives.
func FanoutKeyedTerms(terms []string) []string {
	var out []string
	for _, t := range terms {
		digits := isAllDigits(t)
		// Python compares t.lower() against the (lowercase) stopword set while the
		// terms themselves keep their case (agentic_rag_graph.py:_search_one), so "The"
		// must be dropped like "the".
		if (utf8.RuneCountInString(t) >= 3 && !IsStopword(strings.ToLower(t)) && !digits) ||
			(utf8.RuneCountInString(t) >= 4 && digits) {
			out = append(out, t)
		}
	}
	return out
}

// searchOpts carries the per-entry-point differences that in Python live in
// separate functions (hybrid_search / vector_search / bm25_search / grep_search /
// RAGTools.retrieve). In Go each entry point is its own function that builds a
// searchOpts and delegates to runSearch, so the vector-weight gate, similarity
// threshold and compiled-row exclusion are chosen by the CALLER, never by a
// single shared "channel flag" that a global switch could silently corrupt.
type searchOpts struct {
	// weight is the VECTOR similarity weight (Python vector_similarity_weight).
	weight float64
	// threshold is the engine similarity floor.
	threshold float64
	// excludeCompiled mirrors Python hybrid_search's must_not={"exists":"compile_kwd"}.
	excludeCompiled bool
	// promoteChildren mirrors Python calling settings.retriever.retrieval_by_children
	// after search (search.py:_normalize): child fragments are lifted to their
	// parent chunk. In Python EVERY entry point does this — hybrid_search (L106),
	// vector_search (L239) and bm25_search (L276) all route through _normalize, and
	// grep_search builds on bm25_search. So ALL entry points set promoteChildren:
	// true (including vector/bm25/grep); only a future non-promoting path would
	// leave it false. An empty tenant_id still skips promotion, mirroring Python
	// _normalize.
	promoteChildren bool
	// logLabel / logVerb / logKeywords describe the entry point's own
	// "searching" line, Python-exact:
	//
	//	hybrid → [Hybrid search] Searching the knowledge base for "q" (keywords: k) (search.py:hybrid_search)
	//	vector → [Vector search] Searching by meaning for "q" (keywords: k)          (:215)
	//	bm25   → [BM25 search] Searching by keyword for "q" (keywords: k)            (:252)
	//	grep   → [Grep search] Keyword-first locate for "q"      (NO keywords)       (:418)
	//
	// Python prints the "(keywords: …)" suffix unconditionally on the three legs
	// that carry it; Go prints it only when non-empty (a keyword-less leg is a
	// deliberate state — see the fan-out's narrow bypass —
	// agentic_rag_graph.py:_search_one — and search_chunks takes no keywords at all,
	// action_session.py:execute_tool — so a bare "(keywords: )" reads like a dropped
	// argument). logKeywords also keeps grep from growing a suffix Python never
	// prints on that leg.
	logLabel    string
	logVerb     string
	logKeywords bool
	// logExtra enables the dedup / compiled-expansion / progress lines. Python
	// emits those ONLY inside hybrid_search (:140 / :192 / :203); vector_search
	// and bm25_search log nothing but their searching line (plus a debug line Go
	// has no equivalent for), and grep_search has its own two lines instead.
	logExtra bool
	// cache enables the per-request search cache (SearchCacheLoad/Store). Python
	// keeps that cache on `tools.search_cache` and touches it ONLY inside
	// hybrid_search (:136-141 read, :207 write); vector_search / bm25_search /
	// grep_search / RAGTools.retrieve never consult it. Go's cache lives on the
	// shared deps.KB, so without this gate a keyword-only leg could be served a
	// hybrid result (different vector weight, threshold and compiled policy)
	// and vice versa — Go's "enhanced" typing did not make it hybrid-scoped.
	cache bool
	// narrowLabel tags the narrowing line. Python is NOT self-consistent here:
	// hybrid_search passes the snake_case "hybrid_search", while
	// vector_search / bm25_search pass their display labels "Vector search"
	// (:248) / "BM25 search" (:288). Both forms are reproduced verbatim.
	narrowLabel string
}

// searchLogLine renders a per-leg "searching" line (search.py:hybrid_search/215/252/418):
// both the bracket label and the verb vary by entry point. The keywords suffix
// is appended by the caller, because Python prints it on hybrid/vector/bm25
// only — grep_search's locate line never carries one.
func searchLogLine(label, verb, question string) string {
	return fmt.Sprintf("[%s] %s %q", label, verb, question)
}

// searchLogger returns the run's logger, falling back to the package logger.
func searchLogger(deps SearchDeps) *log.Logger {
	if deps.Logger != nil {
		return deps.Logger
	}
	return _LOG
}

// runSearch is the shared body of every search entry point. It is deliberately
// dumb about WHICH Python function it mirrors: that is encoded by opts, supplied
// by the calling entry-point function.
func runSearch(ctx context.Context, deps SearchDeps, p SearchParams, opts searchOpts) ([]map[string]any, []map[string]any) {
	logger := searchLogger(deps)
	// Python retrieve:614-646 — an explicit argument wins, then the caller's
	// configuration, then this package's own defaults.
	topN := p.TopN
	if topN <= 0 {
		topN = deps.TopN
	}
	if topN <= 0 {
		topN = DefaultTopN
	}
	// Python hybrid_search:117 folds the structured (SQL) datasets into the
	// target id list: `kb_ids or tools.kb_ids + [kb.id for kb in tools.sql_kbs]`.
	// The explicit argument still wins; otherwise the session's kb_ids and
	// sql_kbs are merged so the vector/keyword search spans the structured tables.
	targetIDs := p.KbIDs
	if len(targetIDs) == 0 {
		// Python hybrid_search:117 folds sql_kbs into the id list via
		// `dict.fromkeys(kb_ids + [kb.id for kb in sql_kbs])` — dict.fromkeys
		// de-duplicates. Mirror that here so a KB present in both KbIDs and
		// SQLKBs is not searched twice (and likewise dedup KbIDs themselves).
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
	// Mirrors Python search.py:hybrid_search/216/254 — the 400-char cap applies ONLY to the
	// retrieval_query branch (entity-weighted expansion appended to the question).
	// The keywords and bare-question branches are NOT truncated, matching Python's
	// `f"{query} {keywords}".strip()` / `query` forms, which carry no [:400].
	var effectiveQuery string
	if strings.TrimSpace(p.RetrievalQuery) != "" {
		// Python's cap slices code points (`f"{query} {retrieval_query}".strip()[:400]`,
		// search.py:129/216/254): a byte slice would both cut a multi-byte rune in
		// half — handing the retriever invalid UTF-8 — and, for CJK, stop at
		// ~133 characters, silently dropping two thirds of the expansion terms
		// the fan-out leg weighs on.
		effectiveQuery = truncateRunes(strings.TrimSpace(fmt.Sprintf("%s %s", p.Question, p.RetrievalQuery)), maxEffectiveQueryChars)
	} else if strings.TrimSpace(p.Keywords) != "" {
		effectiveQuery = strings.TrimSpace(fmt.Sprintf("%s %s", p.Question, p.Keywords))
	} else {
		effectiveQuery = p.Question
	}

	// Python's per-leg searching line (search.py:hybrid_search/215/252/418): BOTH the
	// bracket label and the verb vary by entry point — "[Hybrid search]
	// Searching the knowledge base for", "[Vector search] Searching by meaning
	// for", "[BM25 search] Searching by keyword for", "[Grep search]
	// Keyword-first locate for" — and only the three keyword-carrying legs
	// append the keywords (see searchOpts.logLabel / logVerb / logKeywords).
	// The suffix is printed only when non-empty: a keyword-less leg is a
	// deliberate state (fan-out Channel B is a narrow bypass,
	// agentic_rag_graph.py:_search_one; search_chunks takes no keywords at all,
	// action_session.py:execute_tool), so a bare "(keywords: )" reads like a dropped
	// argument rather than an intentional empty.
	searchLine := searchLogLine(opts.logLabel, opts.logVerb, p.Question)
	if kws := strings.TrimSpace(p.Keywords); opts.logKeywords && kws != "" {
		searchLine += fmt.Sprintf(" (keywords: %s)", kws)
	}
	logger.Printf("%s", searchLine)

	// 3. Per-request dedup: an identical query+scope is retrieved at most once,
	// so e.g. pre_search and a claim search asking the same question do not
	// repeat the round-trip, child fetch and narrowing. Python's cache is
	// hybrid-only — both the lookup (:136-141) and the store (:207) sit inside
	// hybrid_search — hence opts.cache.
	if deps.KB != nil && opts.cache {
		if chunks, aggs, ok := deps.KB.SearchCacheLoad(SearchCacheKey(effectiveQuery, targetIDs, topN, docScope)); ok {
			logger.Printf("[%s] Already searched this — reusing the %d passage(s) found earlier.", opts.logLabel, len(chunks))
			return chunks, aggs
		}
	}

	// 4. Retrieve.
	// rank_feature (Python retrieve: rank_feature=label_question(question,
	// self.kbs), agentic_rag.py:retrieve). Go computes it from the KB objects via the
	// injected Tagger; nil Tagger ⇒ no boost (Python's label_question returns
	// None). The result is a tag → weight map the engine understands.
	var rankFeature map[string]float64
	if deps.Tagger != nil {
		rankFeature = deps.Tagger.LabelQuestion(ctx, effectiveQuery, deps.KBs)
	}
	// The per-query progress line is NOT emitted here: runSearch already logs
	// "[<kind>] Searching the knowledge base for …" through the run's logger,
	// which Rag wraps with the think-log forwarder (see think_log.go). Emitting
	// one here as well would show the same search twice in the reasoning block.
	chunks, err := deps.Backend.Retrieve(ctx, RetrieveRequest{
		Query:                 effectiveQuery,
		DatasetIDs:            targetIDs,
		DocScope:              docScope,
		TopN:                  topN,
		TopK:                  intOrDef(deps.TopK, DefaultTopK),
		RerankCandidatesCount: max(intOrDef(deps.RerankCandidatesCount, DefaultRerankCandidatesCount), topN),
		SimilarityThreshold:   &opts.threshold,
		// The field is named for keywords but carries the VECTOR weight, as in
		// Python's vector_similarity_weight. Whether the vector leg runs at all
		// — and its default — is decided by the calling entry-point function
		// (see searchOpts.weight), NOT by a shared channel flag. ExcludeCompiled
		// mirrors Python hybrid_search's must_not={"exists":"compile_kwd"}
		// (search.py:hybrid_search), which keeps compiled products out of plain retrieval.
		KeywordsSimilarityWeight: &opts.weight,
		TenantID:                 deps.TenantID,
		MetaDataFilter:           deps.MetaDataFilter,
		RankFeature:              rankFeature,
		ExcludeCompiled:          opts.excludeCompiled,
	})
	if err != nil {
		// Go-only line: Python lets the retriever's exception propagate untagged.
		logger.Printf("[%s] retrieval failed: %v", opts.logLabel, err)
		return nil, nil
	}

	// 4b. retrieval_by_children: Python promotes child fragments to their parent
	// chunk right after retrieval through _normalize (, also reached
	// by vector_search L239 and bm25_search L276; grep_search rides on bm25_search;
	// RAGTools.retrieve does the same at). ALL entry points do
	// this, so every runSearch caller sets promoteChildren: true. The promotion
	// lives here (gated by opts.promoteChildren) rather than in the shared Backend
	// so a future non-promoting path can opt out; an empty tenant_id skips it,
	// mirroring Python _normalize.
	if opts.promoteChildren {
		de := deps.DocEngine
		if de == nil {
			de = engine.Get()
		}
		if de != nil && len(chunks) > 0 {
			chunks = nlp.RetrievalByChildren(chunks, []string{deps.TenantID}, de, ctx)
		}
	}

	// 5. doc_aggs from the FULL retrieved candidate set, computed here and left
	// untouched by the narrowing / compiled-expansion steps below — mirroring
	// Python, where `_normalize` takes doc_aggs straight from the retriever and
	// _narrow_or_keep / _expand_with_compiled only ever replace kbinfos["chunks"].
	aggs := DocAggs(chunks)

	// Memory BEFORE narrowing: the raw corpus may hold a fact the narrowing
	// drops, and a gap-driven grep over memory recovers it without re-querying.
	if deps.KB != nil {
		MemoryAdd(deps.KB, chunks)
	}

	// 6. Narrow-or-keep (chunks only; doc_aggs stays as retrieved).
	chunks = NarrowOrKeep(chunks, p.Keywords, opts.narrowLabel, logger)

	// 7. Compiled expansion.
	if p.UseCompiled && len(chunks) > 0 && deps.Expand != nil {
		logger.Printf("[%s] Compiled expansion enabled — enriching with page_index/tree/KG navigation.", opts.logLabel)
		if err := deps.Expand.Expand(ctx, deps.KB, p.Question, p.Keywords, docScope); err != nil {
			logger.Printf("[%s] compiled expansion failed: %v", opts.logLabel, err)
		}
	}

	// Python prints this progress line inside hybrid_search only (:203): the
	// bm25/vector legs log nothing but their searching line, and grep_search
	// has its own two lines (see GrepSearch). Gated so a log diff against
	// Python lines up line for line.
	if len(chunks) > 0 && opts.logExtra {
		logger.Printf("[%s] %q -> %d chunk(s): %s", opts.logLabel, trunc(p.Question, 80), len(chunks), docStatsLine(chunks))
	}

	// 8. Cache the result — hybrid leg only, mirroring Python search.py:hybrid_search.
	if deps.KB != nil && opts.cache {
		deps.KB.SearchCacheStore(SearchCacheKey(effectiveQuery, targetIDs, topN, docScope), chunks, aggs)
	}
	return chunks, aggs
}

// HybridSearch mirrors Python hybrid_search. The vector leg is
// gated on an embedder being configured (Python: vector_weight = _setting(...)
// if embd_mdl else 0); default weight 0.3. Compiled rows are excluded.
func HybridSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	return runSearch(ctx, deps, p, searchOpts{
		weight:          resolveVectorWeight(deps, ChannelHybrid),
		threshold:       floatOrDef(deps.SimilarityThreshold, DefaultSimilarityThreshold),
		excludeCompiled: true,
		promoteChildren: true,
		logLabel:        "Hybrid search",
		logVerb:         "Searching the knowledge base for",
		logKeywords:     true,
		logExtra:        true, // Python's dedup/compiled/progress lines live here only
		cache:           true, // Python's search_cache is read/written here only (:136/:207)
		// Python passes the SNAKE_CASE tag to _narrow_or_keep on this leg
		// (search.py:hybrid_search) although its own lines say "Hybrid search" — the
		// inconsistency is Python's and is reproduced verbatim.
		narrowLabel: "hybrid_search",
	})
}

// VectorSearch mirrors Python vector_search. It is the pure
// vector entry point: with no embedder it returns nothing (Python bails when
// embd_mdl is unset), otherwise the vector weight is 1.0 and compiled rows are
// excluded.
func VectorSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	if !deps.HasEmbedder {
		return nil, nil
	}
	return runSearch(ctx, deps, p, searchOpts{
		weight:          1.0,
		threshold:       VectorSearchDefaultSimilarityThreshold,
		excludeCompiled: true,
		promoteChildren: true,
		logLabel:        "Vector search",
		logVerb:         "Searching by meaning for",
		logKeywords:     true,
		logExtra:        false, // Python vector_search logs nothing but its searching line (:215)
		narrowLabel:     "Vector search",
	})
}

// BM25Search mirrors Python bm25_search. Keyword-only: the
// vector weight is unconditionally 0, the similarity floor is 0.0, and compiled
// rows are excluded.
func BM25Search(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	return runSearch(ctx, deps, p, searchOpts{
		weight:          0,
		threshold:       BM25SearchDefaultSimilarityThreshold,
		excludeCompiled: true,
		promoteChildren: true,
		logLabel:        "BM25 search",
		logVerb:         "Searching by keyword for",
		logKeywords:     true,
		logExtra:        false, // Python bm25_search logs nothing but its searching line (:252)
		narrowLabel:     "BM25 search",
	})
}

// GrepSearch mirrors Python grep_search: a keyword-first locate
// that runs bm25_search and then narrows the prose candidates to the term-grep
// window (regex locate + short line-context, like Python's _narrow_by_terms).
//
// Behaviour, faithful to Python:
//   - BM25 candidate pool built from the query's extracted terms — or the
//     explicit keywords hint when one is supplied (search.py:grep_search) — never a
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
	logger.Printf("%s", searchLogLine("Grep search", "Keyword-first locate for", query))
	if query == "" {
		return nil, nil
	}
	terms := GrepTermsFromQuery(query)
	// Python then delegates to bm25_search with an explicit keywords hint
	// (search.py:grep_search): `hint = keywords if keywords else " ".join(terms)`.
	// A long question buries its proper nouns under stopwords; without the hint
	// the noun chunk never enters the candidate pool and grep has nothing to
	// locate. It is a SOFT boost — the pool still spans the corpus. Delegating
	// (rather than calling runSearch) also reproduces Python's log sequence:
	// bm25_search logs "[BM25 search] Searching by keyword for …" and narrows
	// under that same tag (:252 / :288), then GrepSearch logs its own two lines.
	hint := strings.TrimSpace(p.Keywords)
	if hint == "" {
		hint = strings.Join(terms, " ")
	}
	bp := p
	bp.Question = query
	bp.Keywords = hint
	chunks, docAggs := BM25Search(ctx, deps, bp)
	// Python: `if not chunks or not terms: return res` (search.py:grep_search).
	if len(chunks) == 0 || len(terms) == 0 {
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
		return chunks, docAggs
	}

	res := NarrowByTerms(prose, terms, nil, query, NarrowContext{Before: 1, After: 0},
		GrepOutCharsPerChunk, GrepOutTotalChars)
	kept := res.Kept
	if len(kept) == 0 {
		// Nothing matched: keep the raw BM25 candidates so evidence is not dropped.
		return chunks, docAggs
	}
	out := make([]map[string]any, 0, len(table)+len(kept))
	out = append(out, table...)
	out = append(out, kept...)
	// Python logs the post-narrow size and the per-doc breakdown for the
	// combined table+prose set (search.py:grep_search / :469) — the grep leg's own two
	// lines, which is why runSearch keeps logExtra off for it.
	chars := 0
	for _, c := range out {
		chars += len(ChunkTextOf(c))
	}
	logger.Printf("[Grep search] narrowed %d->%d chunk(s), %.1fK chars.", len(chunks), len(out), float64(chars)/1000.0)
	logger.Printf("[Grep search] %q -> %d chunk(s): %s", trunc(query, 80), len(out), docStatsLine(out))
	return out, docAggs
}

// RetrieveSearch mirrors Python RAGTools.retrieve, the
// low/naive direct pass. Unlike HybridSearch it honours UsingEmbedding and does
// NOT exclude compiled rows (Python's retrieve has no must_not compile_kwd).
func RetrieveSearch(ctx context.Context, deps SearchDeps, p SearchParams) ([]map[string]any, []map[string]any) {
	return runSearch(ctx, deps, p, searchOpts{
		weight:          resolveVectorWeight(deps, ChannelRetrieve),
		threshold:       floatOrDef(deps.SimilarityThreshold, DefaultSimilarityThreshold),
		excludeCompiled: false,
		promoteChildren: true,
		// Go-only identity: Python's L1 RAGTools.retrieve
		// logs nothing of its own, so there is no Python string to mirror. The
		// tag stays human-readable and the extra lines stay on, because this is
		// the only trace the low-mode direct pass leaves.
		logLabel:    "Retrieve",
		logVerb:     "Searching the knowledge base for",
		logKeywords: true,
		logExtra:    true,
		narrowLabel: "retrieve",
	})
}

// webSearchMaxChunks is the hard ceiling on admitted web passages per call:
// Python action_session._exec_web_search caps each query's chunks at [:8] and
// runs at most two queries, so 16 is the maximum.
const webSearchMaxChunks = 16

// WebSearchTool mirrors Python web_search / _exec_web_search
// (action_session.py:_exec_web_search): the `web_search` tool. The web_search tool schema
// takes a 1-2 element array of query strings (see webSearchToolSpec), so args is
// the positional query list, not a key/value object. Results share the RAGFlow
// chunk shape so they merge into the SAME shared evidence pool as corpus hits
// (Python _exec_web_search admits every web chunk via _admit_evidence), and the
// model sees the same REDUNDANT/OK/MISS outcome as the corpus tools.
func WebSearchTool(ctx context.Context, deps SearchDeps, args map[string]any) (ToolOutcome, error) {
	if deps.WebSearch == nil {
		// Mirror Python action_session._exec_web_search: an absent provider is
		// an infra ERROR with an explicit do-not-retry note (returning MISS
		// would let the model retry the same dead tool and burn turns), never a
		// query-level miss.
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "web_search",
				"note": "web_search is not configured in this deployment. Do not use this tool again; use the corpus tools (retrieve / search_chunks / navigate_*) instead.",
			}},
			Status:  StatusError,
			Reason:  ReasonInfra,
			Metrics: map[string]any{},
		}, nil
	}
	queries := toolStringList(args, "")
	if len(queries) == 0 {
		// Fall back to a single positional element pushed under the empty key.
		if raw, ok := args[""]; ok {
			queries = toolStringList(map[string]any{"q": raw}, "q")
		}
	}
	if len(queries) == 0 {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	// Cap at two queries (Python action_session.execute_tool passes
	// _arg_query_list(args, 2) for web_search).
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
	// Python caps each query's chunks at [:8]; with up to two queries that is a
	// hard ceiling of 16 web passages per tool call.
	if len(results) > webSearchMaxChunks {
		results = results[:webSearchMaxChunks]
	}

	// Build RAGFlow-shaped chunk maps and admit them to the shared evidence pool
	// (Python _admit_evidence), mirroring the corpus tools' merge + outcome.
	// Python de-duplicates web passages across the (up to two) queries by
	// chunk_id; the Go provider surfaces raw strings, so de-dup by content to
	// keep the same "each distinct passage appears once" behaviour.
	var payload []any
	var evidenceIDs []string
	newChunks := 0
	seen := make(map[string]bool, len(results))
	for i, r := range results {
		if r == "" || seen[r] {
			continue
		}
		// Python _admit_evidence early-stops at the pool cap, BEFORE it records
		// the chunk as seen, so a rejected passage is retried once room frees.
		if evidencePoolFull(deps.KB) {
			continue
		}
		seen[r] = true
		chunkID := fmt.Sprintf("web_%d", i)
		c := map[string]any{
			"chunk_id": chunkID,
			"content":  r,
			"doc_id":   "web",
		}
		if deps.KB != nil {
			before := len(deps.KB.Chunks)
			deps.KB.Merge([]map[string]any{c}, nil)
			newChunks += len(deps.KB.Chunks) - before
		}
		// Evidence references the chunk id (Python ids.append(cid)), not a pool
		// position.
		evidenceIDs = append(evidenceIDs, chunkID)
		// Passage shape from _admit_evidence(include_doc_id=False): {"id","content"},
		// non-table text cut to 1200 code points (plain slice, no ellipsis).
		content := r
		if !IsTableChunk(c) {
			content = truncateRunes(content, 1200)
		}
		payload = append(payload, map[string]any{"id": chunkID, "content": content})
	}

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

// list_chunks deep-read caps (mirror Python action_session.py / tools/search.py).
const (
	// listChunksMaxDeep caps the deep-read pool admission (Python
	// tools/search.py _LIST_CHUNKS_MAX_CHUNKS = 80).
	listChunksMaxDeep = 80
	// listChunksMaxOut caps the model-facing passage list (Python
	// action_session.py _exec_list_chunks [:30]).
	listChunksMaxOut = 30
)

// listChunks deep-reads one document's FULL text and admits it to the shared
// evidence pool — the mirror of Python _exec_list_chunks
// → tools.list_chunks → tools.fetch_full_document.
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
		// Mirror Python list_chunks("") → empty → _search_outcome → MISS.
		return ToolOutcome{Payload: []any{}, Status: StatusMiss, Reason: ReasonNoDoc, Metrics: map[string]any{"hits": 0}}, nil
	}

	var chunks []map[string]any
	if e.deps.DocChunks != nil {
		// Real doc-store deep read (Python fetch_full_document), budgeted by the
		// model window like Python's max_length gate.
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
		// Unknown doc_id / out-of-scope doc, or nothing readable: a query-level
		// MISS (Python list_chunks returns {"chunks": []} and _search_outcome
		// turns that into MISS/no_doc — there is no "graceful OK empty" branch).
		return ToolOutcome{Payload: []any{}, Status: StatusMiss, Reason: ReasonNoDoc, Metrics: map[string]any{"hits": 0, "new_evidence": 0}}, nil
	}

	// Python _seed_evidence CREATES the pool when it
	// is absent, so the admittance below always has somewhere to write.
	if e.deps.KB == nil {
		e.deps.KB = &Kbinfos{}
	}

	// Python _exec_list_chunks (:809-833) admits only the first listChunksMaxOut
	// (30) of the listChunksMaxDeep (80) fetched passages: no-id chunks and in-call
	// duplicates are skipped, evidence references the chunk id, and only chunks new
	// to the shared pool are appended.
	admit := chunks
	if len(admit) > listChunksMaxOut {
		admit = admit[:listChunksMaxOut]
	}
	kbSeen := make(map[string]bool, len(e.deps.KB.Chunks))
	for _, c := range e.deps.KB.Chunks {
		kbSeen[chunkKey(c)] = true
	}
	seen := map[string]bool{}
	var payload []any
	var evidenceIDs []string
	newChunks := 0
	for _, c := range admit {
		cid := ChunkIDOf(c)
		if cid == "" {
			// Python _exec_list_chunks skips a blank cid in the caller, BEFORE
			// _admit_evidence.
			continue
		}
		// Python _admit_evidence early-stops at the pool cap, BEFORE the
		// per-call dedup.
		if evidencePoolFull(e.deps.KB) {
			continue
		}
		if seen[cid] {
			continue
		}
		seen[cid] = true
		evidenceIDs = append(evidenceIDs, cid)
		// Shape like _admit_evidence(include_doc_id=False): {"id","content"} with
		// non-table text cut to 1200 code points (a plain slice, no ellipsis).
		content := ChunkTextOf(c)
		if !IsTableChunk(c) {
			content = truncateRunes(content, 1200)
		}
		payload = append(payload, map[string]any{"id": cid, "content": content})
		key := chunkKey(c)
		if !kbSeen[key] {
			kbSeen[key] = true
			e.deps.KB.Chunks = append(e.deps.KB.Chunks, c)
			newChunks++
		}
	}

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
		Reason:      ReasonNone,
		Metrics:     map[string]any{"hits": len(payload), "new_evidence": newChunks},
	}, nil
}

// SearchCacheKey mirrors Python _search_cache_key: the tuple of what actually
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

// docInDatasets reports whether docID belongs to the datasets being searched,
// mirroring the `kb_id.in_(self.kb_ids)` guard of Python
// RAGTools._resolve_doc_tenant.
//
// The second result is false only when ownership could NOT be determined — no
// verifier injected, or the lookup errored. It is true when the lookup
// completed, including the "document absent from the bound datasets" case:
// KnownDocIDs returns a known-subset map, so a foreign/stale doc_id yields an
// empty map (never an error), and is reported as belongs=false / verified=true
// so the caller rejects it instead of reading it unscoped — exactly as Python
// returns None from _resolve_doc_tenant and refuses to fetch.
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

// scopedDocIDs mirrors Python RAGTools.scoped_doc_ids,
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

// resolveDocScope mirrors Python retrieve:620-635 — apply the session doc_scope
// ceiling, then keep only the requested document ids that actually belong to the
// session's datasets.
//
// The ceiling is Python's scoped_doc_ids: a None request scope falls back to the
// session's fixed doc_scope (deps.DocScope), and an explicit request scope is
// intersected with it, so a session bound to a fixed document set keeps
// searching within it whichever scope the tool passes.
//
// Three outcomes are tuned to match Python, because the document scope is
// supplied by model-generated tool arguments and must never silently change what
// is searched:
//
//   - verification unavailable (no verifier injected, or it errored): the scope
//     is passed through unchanged (fail open);
//   - every candidate unknown AND a fixed session base scope (self.doc_scope is
//     not None): return an EMPTY result (Python retrieve:631-632 returns
//     {"chunks": [], "doc_aggs": []}), never unfiltered retrieval;
//   - every candidate unknown with NO session base scope: drop the scope and
//     search UNFILTERED (Python retrieve:633-635 — doc_scope = None).
//
// The empty-vs-unfiltered distinction is encoded as a non-nil empty slice vs
// nil: the backend treats a nil DocScope as "search everything" and a non-nil
// empty DocScope as "match nothing".
func resolveDocScope(ctx context.Context, deps SearchDeps, scope, kbIDs []string, logger *log.Logger) []string {
	scope = scopedDocIDs(deps.DocScope, scope)
	if len(scope) == 0 && len(deps.DocScope) > 0 {
		// The session ceiling removed every requested id. Python's scoped_doc_ids
		// yields an empty list here, which its downstream `if doc_scope:` reads as
		// "no filter" and searches unfiltered; Go deliberately keeps the safer
		// "match nothing" instead of letting a model-supplied scope escape the
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
		// No usable candidate ids (request scope None and no session base scope,
		// or only empty strings): Python leaves doc_scope = None and searches
		// unfiltered. Return nil, not an empty set, so the backend does not filter
		// to zero documents.
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
		// Every candidate is unknown. Python retrieve:631-635 distinguishes two
		// outcomes by the SESSION base scope:
		//   - a fixed session doc_scope (self.doc_scope is not None) → return an
		//     empty result (no documents match);
		//   - no session base scope → fall back to UNFILTERED retrieval (None).
		if len(deps.DocScope) > 0 {
			return []string{}
		}
		logger.Printf("[Hybrid search] every supplied doc ID was unknown; falling back to unfiltered retrieval")
		return nil
	}
	return valid
}

// DocAggs builds the per-document aggregation from a chunk list: each entry
// carries doc_id (the key direct.py dedups on) plus the counts the reporting layer
// reads. Python gets these from the retriever (retrieval(..., aggs=True)); the Go
// Retriever interface returns only chunks, so they are derived here.
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
// Mirrors the shaping the Python harness applies to _exec_web_search results so
// they enter kbinfos with the same field names every other chunk carries.

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

// ---------------------------------------------------------------------------
// Per-request retrieval cache (Python tools.search_cache).
//
// The Python side keeps a dict on the tools object keyed by the search
// parameters; an identical (query, scope, limits) search is served from cache
// instead of hitting the retriever again. Go attaches the same cache to
// Kbinfos and exposes it as methods here so the search tool and its cache live
// in one file, mirroring tools/search.py.
// ---------------------------------------------------------------------------

// searchCache is the per-request retrieval cache. Mirrors Python
// tools.search_cache (a dict on the tools object).
type searchCache struct {
	mu      sync.Mutex
	entries map[string]cachedSearch
}

// cachedSearch is one cache entry. Mirrors a Python tools.search_cache value.
type cachedSearch struct {
	chunks []map[string]any
	aggs   []map[string]any
}

// SearchCacheLoad returns a cached retrieval for key, or (nil, nil, false) when
// the key is absent. Mirrors Python `if cache_key in cache: return cache[cache_key]`.
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

// SearchCacheStore records a retrieval under key. Mirrors Python
// `cache[cache_key] = {...}`.
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
