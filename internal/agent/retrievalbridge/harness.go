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

package retrievalbridge

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ragflow/internal/agent/component"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	et "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/rag/agentic-rag"
	agenticruntime "ragflow/internal/rag/agentic-rag/runtime"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// NewHarnessRetriever connects chat requests to the agentic runtime using the
// shared model, metadata and document services initialized by the API server.
func NewHarnessRetriever(modelProviderService *service.ModelProviderService, metadataService *service.MetadataService, docEngine engineDocEngine) func(context.Context, service.HarnessRequest) (service.HarnessResult, error) {
	return func(ctx context.Context, req service.HarnessRequest) (service.HarnessResult, error) {
		// Resolve the tenant's actual chat model, mirroring Python where RAGTools
		// receives a fully-resolved LLMBundle: chat.LLMID may be a UUID/tenant_model
		// id that the default invoker cannot split into a provider, so resolving here
		// avoids falling through to a dummy driver. If resolution fails we leave model
		// nil so the runtime degrades to a direct search (no dummy fallback).
		var model agenticruntime.SessionModel
		// outerModel mirrors Python RAGTools.chat_mdl: the fully-resolved chat
		// model that drives the outer rag_agent react loop (tools=[rag,
		// summarize_document], terminal_tools={"rag"}). Wired into RAGTools.Outer
		// so Rag() can run that loop instead of the direct graph.
		var outerModel *modelModule.ChatModel
		// resolvedModelName is the resolved chat model's own name (e.g.
		// "gpt-4o"), i.e. Python chat_mdl.llm_name. It is the gen_json reply-cache
		// key's model component; empty disables that cache.
		resolvedModelName := ""
		// The resolution goes through the same entry point the pipeline's own
		// capability probe uses, so the model decided there is the model wired
		// here: req.ModelID may be empty (a dialog without an llm_id), and both
		// layers must then fall back to the tenant default rather than resolving
		// nothing here and judging capability there.
		if target, mErr := modelProviderService.ResolveChatModelTarget(ctx, req.TenantID, req.ModelID); mErr == nil {
			if inv := component.NewResolvedInvoker(target.Driver, target.ModelName, target.APIConfig); inv != nil {
				// MaxLength mirrors Python LLMBundle.max_length (the model's
				// context window in tokens); message-fitting nodes (calculate,
				// structure_qa) use it as their chat.FitMessages budget.
				model = &agenticruntime.InvokerSessionModel{Invoker: inv, DB: dao.DB, MaxLength: target.ContextLength}
				resolvedModelName = target.ModelName
				// Reuse the same resolved driver/name/api as the invoker so
				// the outer loop and the inner tool calls share one model.
				outerModel = modelModule.NewChatModel(target.Driver, &target.ModelName, target.APIConfig)
			}
		} else {
			common.Warn("runtime: failed to resolve chat model for reasoning; runtime will degrade to direct search", zap.Error(mErr))
		}

		// Load the KB objects (mirroring Python RAGTools' self.kbs via
		// KnowledgebaseService.get_by_ids(kb_ids)) so the agentic tool can
		// derive rank features. The Go tag extractor (extractor_tag.go) writes
		// both tag_kwd (the list of tag names) and tag_feas (per-tag weights)
		// onto each chunk at parse time; the labeler aggregates tag_kwd to build
		// the tag vocabulary and the retriever ranks with tag_feas.
		var kbs []*entity.Knowledgebase
		if len(req.DatasetIDs) > 0 {
			var err error
			kbs, err = dao.NewKnowledgebaseDAO().GetByIDs(ctx, dao.DB, req.DatasetIDs)
			if err != nil {
				return service.HarnessResult{}, err
			}
			if err := validateLoadedDatasets(req.DatasetIDs, kbs); err != nil {
				return service.HarnessResult{}, err
			}
		}
		// validate_dataset_embedding_models runs FIRST upstream
		// (dialog_service.py:358-360) and mixing is a hard failure there, not a
		// fallback to keyword-only retrieval.
		if err := validateDatasetEmbeddingModels(ctx, kbs); err != nil {
			return service.HarnessResult{}, err
		}
		// HasEmbedder mirrors Python `embd_mdl = ... if kbs and kbs[0].embd_id else
		// None` (dialog_service.py:362): the gate is the FIRST dataset's embd_id. It
		// is what hybrid_search applies to its vector leg (search.py:143), so
		// getting it wrong silently degrades search_chunks to keyword-only.
		hasEmbedder := hasEmbedderFor(kbs)
		// Retrieval tuning mirrors the dialog's own settings, so the agentic
		// searches run with the same budget as the standard path. Without it the
		// harness falls back to its package defaults (top_n=12) and the dialog's
		// configured top_n is silently ignored.
		//
		// The runtime carries the KEYWORD leg's weight and derives the vector leg
		// as its complement (runtime.resolveKeywordsSimilarityWeight, and the
		// RetrieveRequest contract: 0.7 hybrid / 0.0 vector-only / 1.0
		// keyword-only). The dialog configures the VECTOR weight, so it is inverted
		// here; the dialog's default 0.3 therefore yields the runtime's own 0.7.
		keywordsWeight := 1 - req.Retrieval.VectorSimilarityWeight
		deps := agentic_rag.RAGTools{
			Model:                    model,
			ModelName:                resolvedModelName,
			Outer:                    outerModel,
			TopN:                     req.Retrieval.TopN,
			SimilarityThreshold:      req.Retrieval.SimilarityThreshold,
			KeywordsSimilarityWeight: &keywordsWeight,
			RerankCandidatesCount:    req.Retrieval.RerankCandidatesCount,
			// OriginalQuestion mirrors Python RAGTools(original_user_question=...):
			// the user's own, unrewritten question as received from the chat
			// layer. The outer model's `rag(question=...)` argument is
			// model-generated and often compresses a multi-hop question to its
			// first hop; resolveEffectiveQuestion (agentic_rag.py:865) prefers
			// this original over that rewrite when both describe the same turn.
			OriginalQuestion: req.Question,
			// DocScope mirrors Python RAGTools(doc_scope=...): the narrowed doc_ids
			// (chat-level doc_ids + meta_data_filter) restrict every agentic
			// retrieval to the user-selected documents instead of the whole kb.
			DocScope:      req.DocIDs,
			DocIDVerifier: agentic_rag.NewDocIDLookup(),
			// MetadataResolver backs the metadata_search tool and the pre-search
			// metadata channel (Python DocMetadataService push-down + meta_filter
			// fallback). metadataService already carries the doc engine and the DAO
			// the metadata index reads need.
			MetadataResolver: metadataService,
			// DeclaredMetadata lets the metadata_search catalog describe each field
			// (meaning + allowed values) from the dataset's own parser_config, not just
			// name it. Same service: it also carries the KB DAO that read needs.
			DeclaredMetadata: metadataService,
			// DocChunks pages one document's chunks in reading order off the
			// chunk index (Python retriever.chunk_list), backing the
			// document-level tools (summarize_document / fetch_full_document).
			DocChunks: &docChunkPager{docEngine: docEngine},
			// Expand runs search_chunks' compiled-structure expansion
			// (Python hybrid_search use_compiled=True → _expand_with_compiled).
			// Backed by the same document engine the chunk reads use, with the
			// dense seed leg wired to the bound dataset's own embedding model.
			// The scope config lets each dataset be scanned under its own tenant
			// and a doc scope be grouped by real owner, like graph_explore.
			Expand: agenticruntime.NewCompiledExpander(
				newDatasetCompiledStore(docEngine, modelProviderService, kbs),
				agenticruntime.CompiledScopeConfig{
					DatasetIDs:        req.DatasetIDs,
					TenantID:          req.TenantID,
					KBs:               kbs,
					DocTenantResolver: agentic_rag.NewDocTenantResolver(),
				},
			),
			// WebSearch backs the web_search tool (Python RAGTools.web_search).
			// The pipeline supplies a callback only when the chat enables
			// internet web search; a nil one also keeps the tool off the surface.
			WebSearch:   harnessWebSearcher(req.WebSearch),
			KBs:         kbs,
			HasEmbedder: hasEmbedder,
			// Embedder backs claim recall's KNN leg (Python
			// recall_dataset_claims, navigation.py:1837-1909, which embeds the
			// query with tools.embed_mdl — the FIRST dataset's embedding model,
			// dialog_service.py:362-366) and the structure-drill seed vector.
			// Without it the claim leg silently degrades to BM25-only and
			// paraphrase-phrased claims are never recalled. Bound to
			// kbs[0].EmbdID (validateDatasetEmbeddingModels above guarantees
			// every bound dataset shares it), query-side encoded.
			Embedder: embedderForDatasets(kbs, modelProviderService),
			// Tagger is the Go equivalent of Python's label_question
			// (agentic_rag.py:668): classifies the query into question-type
			// tags the retriever boosts on. metadataService implements it
			// (service.MetadataService.LabelQuestion).
			Tagger: metadataService,
			// SystemPrompt mirrors Python RAGTools(system_prompt=
			// _render_reasoning_system_prompt(dialog, prompt_config, kwargs),
			// dialog_service.py:2084) — the dialog-level UI configuration the
			// final-answer compose appends after the agentic contract
			// (agentic_rag_graph.py:923-933).
			SystemPrompt: req.SystemPrompt,
			// CiteRules and EvidenceMaxTokens deliberately stay unset: Python's
			// dialog path constructs RAGTools WITHOUT user_defined_prompts
			// (dialog_service.py:2072-2090), so citation_prompt defaults apply,
			// and Python has no evidence-token override (the compose always
			// uses min(chat_mdl.max_length, _EVIDENCE_BUDGET_TOKENS=8000),
			// which EvidenceMaxTokens<=0 reproduces).
		}
		for _, message := range req.Messages {
			content, err := service.NormalizeOpenAIMessageContent(message["content"])
			if err != nil {
				return service.HarnessResult{}, err
			}
			role, _ := message["role"].(string)
			deps.Messages = append(deps.Messages, schema.Message{Role: schema.RoleType(role), Content: content})
		}
		// Diagnose WHY compiled expansion is disabled: NewCompiledExpander
		// returns nil for three reasons (store==nil / no datasets / no tenant)
		// and RAGTools.Expand==nil silences the whole channel with no other
		// trace — the observed "Compiled expansion enabled = 0" was invisible.
		if deps.Expand == nil {
			switch {
			case docEngine == nil:
				common.Warn("compiled expansion disabled: document engine unavailable (store==nil)")
			case len(req.DatasetIDs) == 0:
				common.Warn("compiled expansion disabled: no bound dataset (DatasetIDs empty)")
			case strings.TrimSpace(req.TenantID) == "":
				common.Warn("compiled expansion disabled: no tenant (TenantID empty)")
			default:
				common.Warn("compiled expansion disabled: unknown reason")
			}
		}
		// The two projections of one reasoning step: the sentence the chat UI
		// appends to its think block, and the structured event a step-rendering
		// client consumes. Steps.Stage/Emit feeds both from one call, so the
		// trace cannot drift from its structured twin. Each step is an
		// isThink=true delta, i.e. think-block content rather than answer text.
		if req.AnswerSink != nil {
			answerSink := req.AnswerSink
			deps.AnswerSink = &agentic_rag.AnswerSink{
				OnDelta: req.AnswerSink,
			}
			deps.Steps.Text = func(line string) { answerSink(line, true) }
		}
		if req.ThinkSink != nil {
			// One type on both sides — agenticruntime.ThinkEvent is an alias of
			// service.ThinkEvent — so the sink passes straight through: there is
			// nothing to copy field by field, and no way to forget a field that
			// was added on one side only.
			deps.Steps.Events = req.ThinkSink
		}
		r := agentic_rag.Rag(ctx, deps, agenticruntime.RunRequest{
			Question:        req.Question,
			ThinkingMode:    req.ThinkingMode,
			DatasetIDs:      req.DatasetIDs,
			TenantID:        req.TenantID,
			SessionID:       req.SessionID,
			Images:          req.Images,
			TextAttachments: req.TextAttachments,
		})
		res := service.HarnessResult{Chunks: r.Chunks, DocAggs: r.DocAggs, Answer: r.Answer, SlotCitations: r.SlotCitations, CiteChunkIDs: r.CiteChunkIDs}
		if r.Kbinfos != nil {
			res.Memory = r.Kbinfos.Memory
			res.PreSummary = r.Kbinfos.PreSummary
		}
		return res, nil
	}
}

// engineDocEngine is the small slice of the engine surface the doc-chunk pager
// needs, kept as a local interface so it is trivially unit-testable without a
// live engine (the production value is engine.Get()).
type engineDocEngine interface {
	Search(ctx context.Context, req *et.SearchRequest) (*et.SearchResult, error)
}

// docChunkPager is the production agenticruntime.DocChunkLister. It pages one
// document's chunks in reading order straight off the chunk index, mirroring
// Python settings.retriever.chunk_list with sort_by_position=True
// (rag/nlp/search.py:824). This is what the document-level tools
// (summarize_document / fetch_full_document) consume.
//
// The index name is derived from the tenant id (ragflow_<tenantID>), exactly as
// retrieval does, so it needs no user-scope resolution: DocChunksRequest
// already carries the tenant and the bound datasets.
type docChunkPager struct {
	docEngine engineDocEngine
}

// DocChunks implements agenticruntime.DocChunkLister.
func (p *docChunkPager) DocChunks(ctx context.Context, req agenticruntime.DocChunksRequest) ([]map[string]any, error) {
	if p == nil || p.docEngine == nil {
		return nil, nil
	}
	if req.DocID == "" {
		return nil, nil
	}
	orderBy := (&et.OrderByExpr{}).
		Asc("chunk_order_int").
		Asc("page_num_int").
		Asc("top_int").
		Desc("create_timestamp_flt")

	resp, err := p.docEngine.Search(ctx, &et.SearchRequest{
		IndexNames: []string{tenantChunkIndexName(req.TenantID)},
		KbIDs:      req.DatasetIDs,
		Offset:     req.Offset,
		Limit:      req.Limit,
		OrderBy:    orderBy,
		SelectFields: []string{
			"id", "doc_id", "docnm", "content_with_weight", "content", "available_int",
		},
		Filter: map[string]interface{}{
			"doc_id":        req.DocID,
			"available_int": 1,
			"must_not":      map[string]interface{}{"exists": "compile_kwd"},
		},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	rows := make([]map[string]any, 0, len(resp.Chunks))
	for _, ck := range resp.Chunks {
		row := make(map[string]any, 8)
		// The runtime chunk rows need the canonical keys its citation pipeline
		// reads: chunk_id (dedup), docnm_kwd (doc title), doc_id and the text
		// under content/content_with_weight (chunkText prefers content first).
		if id, ok := ck["id"].(string); ok && id != "" {
			row["chunk_id"] = id
		}
		if d, ok := ck["doc_id"]; ok {
			row["doc_id"] = d
		}
		if d, ok := ck["docnm"]; ok {
			row["docnm_kwd"] = d
		}
		if c, ok := ck["content"]; ok {
			row["content"] = c
		}
		if c, ok := ck["content_with_weight"]; ok {
			row["content_with_weight"] = c
		}
		// Keep whatever else the engine returned (e.g. position_int, img) so the
		// merged Kbinfos rows stay rich, as retrieval does.
		for k, v := range ck {
			if _, seen := row[k]; !seen {
				row[k] = v
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// hasEmbedderFor mirrors Python `embd_mdl = LLMBundle(...) if kbs and
// kbs[0].embd_id else None` (dialog_service.py:362). The gate is the FIRST bound
// dataset's embd_id, not "any dataset has one": a dialog whose first dataset
// carries no embedding model yields embd_mdl=None upstream even if later ones do.
//
// Callers must run validateDatasetEmbeddingModels first — with validation
// passing, all datasets share one model and this equals "all have one".
func hasEmbedderFor(kbs []*entity.Knowledgebase) bool {
	return len(kbs) > 0 && kbs[0] != nil && kbs[0].EmbdID != ""
}

// embedderForDatasets builds the embedding handle the agentic runtime uses for
// query-side encoding outside the main retrieval leg: claim recall's KNN leg
// (Python recall_dataset_claims, navigation.py:1837-1909, embedding with
// tools.embed_mdl) and the structure-drill seed vector. The model is the FIRST
// dataset's embedding (Python dialog_service.py:362-366 resolves
// kbs[0].embd_id under kbs[0].tenant_id; validateDatasetEmbeddingModels
// guarantees the rest share it). Nil when no dataset carries an embedding
// model — the claim leg then degrades to BM25-only, matching a Python run
// without embed_mdl.
func embedderForDatasets(kbs []*entity.Knowledgebase, modelSvc *service.ModelProviderService) nlp.NavEmbedder {
	if len(kbs) == 0 || kbs[0] == nil || kbs[0].EmbdID == "" {
		return nil
	}
	return service.NewNavEmbedder(modelSvc, kbs[0].EmbdID)
}

func validateLoadedDatasets(datasetIDs []string, kbs []*entity.Knowledgebase) error {
	loaded := make(map[string]struct{}, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			loaded[kb.ID] = struct{}{}
		}
	}
	for _, id := range datasetIDs {
		if _, ok := loaded[id]; !ok {
			return fmt.Errorf("dataset %q was not found", id)
		}
	}
	return nil
}

// validateDatasetEmbeddingModels mirrors Python validate_dataset_embedding_models
// (knowledgebase_service.py:62-94): every bound dataset must use the same embedding
// model, or none at all. Upstream this runs before embd_mdl is resolved
// (dialog_service.py:358-360) and a violation RAISES — it is not a "no embedding
// model" fallback: treating mixing as HasEmbedder=false would silently serve
// keyword-only results where Python errors.
func validateDatasetEmbeddingModels(ctx context.Context, kbs []*entity.Knowledgebase) error {
	var withEmbd int
	for _, kb := range kbs {
		if kb != nil && kb.EmbdID != "" {
			withEmbd++
		}
	}
	hasEmbd := withEmbd > 0
	if hasEmbd && withEmbd != len(kbs) {
		return errors.New("cannot search across datasets where some have embedding models and others do not")
	}
	if !hasEmbd {
		return nil
	}
	// Mirror Python's grouping key exactly:
	//
	//	ref = tenant_embd_id or (embd_id when it is a raw tenant_model id)
	//	composite = resolved_names.get(ref)      # "model@instance@provider"
	//	key = _base_model_name(composite)        # == tenant_model.model_name
	//
	// The @instance@provider suffix is stripped by _base_model_name
	// (knowledgebase_service.py:33), so only the MODEL NAME is compared — the
	// instance and provider tables are not consulted at all.
	refs := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || kb.EmbdID == "" {
			continue
		}
		ref := ""
		if kb.TenantEmbdID != nil {
			ref = strings.TrimSpace(*kb.TenantEmbdID)
		}
		if ref == "" && !strings.Contains(kb.EmbdID, "@") {
			ref = strings.TrimSpace(kb.EmbdID)
		}
		if ref != "" {
			refs = append(refs, ref)
		}
	}

	// Python resolves the refs with one batched lookup
	// (tenant_model_service.py:562 get_by_ids) and silently skips ids that no
	// longer resolve — a dangling id must not fail the request.
	resolved := make(map[string]string, len(refs))
	// A nil/absent DB or any query error must not fail the request: Python skips
	// unresolvable ids (tenant_model_service.py:557) and falls back to the raw
	// id, so validation degrades to a stricter but safe comparison.
	if len(refs) > 0 && dao.DB != nil {
		if models, err := dao.NewTenantModelDAO().GetByIDs(ctx, dao.DB, refs); err == nil {
			for _, m := range models {
				if m != nil {
					resolved[m.ID] = m.ModelName
				}
			}
		}
	}

	keys := make(map[string]struct{}, len(kbs))
	for _, kb := range kbs {
		if kb == nil || kb.EmbdID == "" {
			continue
		}
		embdID := strings.TrimSpace(kb.EmbdID)
		ref := ""
		if kb.TenantEmbdID != nil {
			ref = strings.TrimSpace(*kb.TenantEmbdID)
		}
		if ref == "" && !strings.Contains(embdID, "@") {
			ref = embdID
		}
		key := ""
		if name, ok := resolved[ref]; ok && name != "" {
			key = name
		} else if embdID != "" && embdID != ref {
			key = common.BaseModelName(embdID)
		} else {
			key = ref
		}
		if key != "" {
			keys[key] = struct{}{}
		}
	}
	if len(keys) > 1 {
		return fmt.Errorf("datasets use different embedding models")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Harness seams (compiled expansion + open-web search)
// ---------------------------------------------------------------------------

// engineCompiledStore implements agenticruntime.CompiledStore over the document
// engine, backing search_chunks' compiled-structure expansion (Python
// tools/compiled_expansion.py). The runtime owns the expansion strategy; this
// adapter only maps its (scope, filters, free text) request onto one engine
// search, mirroring Python's settings.docStoreConn.search calls.
type engineCompiledStore struct {
	engine engineDocEngine
	// embed encodes the seed query for the dense leg. It must be the QUERY
	// encoding of the dataset's own embedding model (Python
	// _search_compiled_rows → _dense_expr → settings.retriever.get_vector →
	// emb_mdl.encode_queries). Nil leaves the keyword leg only.
	embed compiledQueryEncoder
	// embedTenantID is the tenant owning the embedder's model (Python
	// embd_owner_tenant_id = kbs[0].tenant_id). It is distinct from the searched
	// scope's tenant, because Python builds ONE embedder from the leading bound
	// dataset and reuses it for every scope.
	embedTenantID string
}

var _ agenticruntime.CompiledStore = (*engineCompiledStore)(nil)

// compiledQueryEncoder is the query-side embedding seam used for the compiled
// dense seed leg. *service.NavEmbedder implements it via EncodeQueries.
type compiledQueryEncoder interface {
	EncodeQueries(ctx context.Context, tenantID string, texts []string) ([][]float32, error)
}

// Mirror Python compiled_expansion._VECTOR_NUM_CANDIDATES / _VECTOR_SIMILARITY.
// The HNSW ef_search floor (num_candidates) must be >= topN or the ANN search is
// bounded by the candidate list instead of by relevance.
const (
	compiledVectorNumCandidates = 256
	compiledVectorSimilarity    = 0.1
)

// newEngineCompiledStore returns nil when no engine is available so
// agenticruntime.NewCompiledExpander disables expansion instead of searching nowhere.
// A nil embed (or empty embedTenantID) keeps the keyword-only leg.
func newEngineCompiledStore(e engineDocEngine, embed compiledQueryEncoder, embedTenantID string) agenticruntime.CompiledStore {
	if e == nil {
		return nil
	}
	return &engineCompiledStore{engine: e, embed: embed, embedTenantID: embedTenantID}
}

// newDatasetCompiledStore builds the per-request compiled-row store, wiring the
// dense seed leg to the FIRST bound dataset's own embedding model. Python builds
// embd_mdl from kbs[0] (dialog_service.py:362-365) and reuses it for the whole
// expansion; using the tenant-default model instead would compare the query
// against rows embedded by a different model — different vector spaces, so the
// dense hits would be noise.
func newDatasetCompiledStore(e engineDocEngine, modelSvc *service.ModelProviderService, kbs []*entity.Knowledgebase) agenticruntime.CompiledStore {
	if e == nil {
		return nil
	}
	if hasEmbedderFor(kbs) {
		return newEngineCompiledStore(e, service.NewNavEmbedder(modelSvc, kbs[0].EmbdID), kbs[0].TenantID)
	}
	return newEngineCompiledStore(e, nil, "")
}

const tenantChunkIndexPrefix = "ragflow_"

func tenantChunkIndexName(tenantID string) string {
	return tenantChunkIndexPrefix + tenantID
}

// compiledSynthesisKinds are the standalone synthesis-page compile_kwd values.
// Python scopes those rows by source_doc_ids, while entity/relation rows are
// scoped by doc_id (compiled_expansion.py _search_compiled_rows /
// _search_synthesis_pages).
var compiledSynthesisKinds = map[string]bool{
	"wiki_page":     true,
	"artifact_page": true,
	"essence":       true,
}

// compiledRowSelectFields is the projection the expander reads off a compiled
// row: content_with_weight (seed name parsing), the entity/relation endpoints,
// exact name lookups, and provenance back to source chunks.
var compiledRowSelectFields = []string{
	"id", "kb_id", "doc_id", "docnm_kwd",
	"content_with_weight", "summary_with_weight", "source_chunk_ids",
	"name_kwd", "from_entity_kwd", "to_entity_kwd", "available_int",
}

// compiledChunkSelectFields is the projection for source chunks promoted into
// evidence by compiled expansion.
var compiledChunkSelectFields = []string{
	"id", "kb_id", "doc_id", "docnm_kwd",
	"content_with_weight", "source_chunk_ids", "similarity",
}

// SearchCompiled implements agenticruntime.CompiledStore.
func (s *engineCompiledStore) SearchCompiled(ctx context.Context, kbID, tenantID string, docIDs []string, filters map[string][]string, matchText string, topN int) ([]map[string]any, error) {
	if s == nil || s.engine == nil || kbID == "" || strings.TrimSpace(tenantID) == "" {
		return nil, nil
	}
	if topN <= 0 {
		topN = 1
	}
	filter := compiledEngineFilter(filters)
	if len(docIDs) > 0 {
		filter[compiledDocScopeKey(filters)] = docIDs
	}
	req := &et.SearchRequest{
		IndexNames:   []string{tenantChunkIndexName(tenantID)},
		KbIDs:        []string{kbID},
		Limit:        topN,
		SelectFields: compiledRowSelectFields,
		Filter:       filter,
	}
	// Python prefers a dense seed leg and only falls back to a keyword match
	// when no embedder is available or the vector build fails
	// (compiled_expansion.py _search_compiled_rows:180-194 /
	// _search_synthesis_pages:423-438). Exactly ONE expr is appended.
	if text := strings.TrimSpace(matchText); text != "" {
		if dense := s.denseExpr(ctx, text, topN); dense != nil {
			req.MatchExprs = []interface{}{dense}
		} else {
			req.MatchExprs = []interface{}{&et.MatchTextExpr{
				Fields:       []string{"content_ltks", "content_sm_ltks"},
				MatchingText: text,
				TopN:         topN,
			}}
		}
	}
	res, err := s.engine.Search(ctx, req)
	if err != nil || res == nil {
		return nil, err
	}
	return res.Chunks, nil
}

// denseExpr builds the compiled-row dense match for the seed text, mirroring
// Python _dense_expr (compiled_expansion.py:31-47): the query is embedded with
// the dataset model's QUERY encoding, and the match carries the HNSW
// num_candidates floor and similarity threshold. Returns nil when no embedder is
// wired, the encode fails, or it panics — every caller then uses the keyword leg,
// exactly like Python's `except Exception` around get_vector.
func (s *engineCompiledStore) denseExpr(ctx context.Context, text string, topN int) *et.MatchDenseExpr {
	if s == nil || s.embed == nil || strings.TrimSpace(s.embedTenantID) == "" {
		return nil
	}
	var (
		vecs [][]float32
		err  error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				common.Warn("compiled expansion: seed encode panicked; falling back to keyword match")
			}
		}()
		vecs, err = s.embed.EncodeQueries(ctx, s.embedTenantID, []string{text})
	}()
	if err != nil || len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil
	}
	vec := make([]float64, len(vecs[0]))
	for i, v := range vecs[0] {
		vec[i] = float64(v)
	}
	return &et.MatchDenseExpr{
		VectorColumnName:  fmt.Sprintf("q_%d_vec", len(vec)),
		EmbeddingData:     vec,
		EmbeddingDataType: "float",
		DistanceType:      "cosine",
		TopN:              topN,
		ExtraOptions: map[string]interface{}{
			"similarity":     compiledVectorSimilarity,
			"num_candidates": max(topN, compiledVectorNumCandidates),
		},
	}
}

// LoadChunks implements agenticruntime.CompiledStore: fetch the referenced source
// chunks by id (Python compiled_expansion.py:_load_chunks_for_doc searches the
// tenant index with an {"id": chunk_ids} condition).
func (s *engineCompiledStore) LoadChunks(ctx context.Context, kbID, tenantID string, chunkIDs []string) ([]map[string]any, error) {
	if s == nil || s.engine == nil || kbID == "" || strings.TrimSpace(tenantID) == "" || len(chunkIDs) == 0 {
		return nil, nil
	}
	res, err := s.engine.Search(ctx, &et.SearchRequest{
		IndexNames:   []string{tenantChunkIndexName(tenantID)},
		KbIDs:        []string{kbID},
		Limit:        len(chunkIDs),
		SelectFields: compiledChunkSelectFields,
		Filter:       map[string]interface{}{"id": chunkIDs},
	})
	if err != nil || res == nil {
		return nil, err
	}
	return res.Chunks, nil
}

// Vectorize implements agenticruntime.CompiledStore. The ported expander assigns
// compiled chunks the similarity stored on the row and blends by that field, so
// expansion itself never needs a query embedding.
func (s *engineCompiledStore) Vectorize(context.Context, string) ([]float64, error) {
	return nil, fmt.Errorf("compiled store: vectorize is not wired")
}

// compiledDocScopeKey mirrors Python's doc-scope column choice: synthesis pages
// are scoped by source_doc_ids, entity/relation rows by doc_id.
func compiledDocScopeKey(filters map[string][]string) string {
	for _, ck := range filters["compile_kwd"] {
		if compiledSynthesisKinds[strings.ToLower(strings.TrimSpace(ck))] {
			return "source_doc_ids"
		}
	}
	return "doc_id"
}

// compiledEngineFilter maps the runtime' OR-list filters onto the engine filter
// shape. available_int is a numeric column, so its string values are coerced to
// ints (Python passes the literal 1).
func compiledEngineFilter(filters map[string][]string) map[string]interface{} {
	out := make(map[string]interface{}, len(filters))
	for k, vals := range filters {
		if k == "available_int" {
			ints := make([]int, 0, len(vals))
			for _, v := range vals {
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
					ints = append(ints, n)
				}
			}
			if len(ints) > 0 {
				out[k] = ints
			}
			continue
		}
		out[k] = vals
	}
	return out
}

// harnessWebSearcherFunc adapts the chat pipeline's web-search callback to the
// agenticruntime.WebSearcher seam consumed by the web_search tool. The callback is a
// plain func so internal/service does not have to depend on the runtime package.
type harnessWebSearcherFunc func(ctx context.Context, queries []string) ([]string, error)

// Search implements agenticruntime.WebSearcher.
func (f harnessWebSearcherFunc) Search(ctx context.Context, queries []string) ([]string, error) {
	return f(ctx, queries)
}

var _ agenticruntime.WebSearcher = harnessWebSearcherFunc(nil)

// harnessWebSearcher wraps a non-nil callback, returning a nil WebSearcher when
// the pipeline did not supply one — which is what hides the web_search tool.
func harnessWebSearcher(fn func(context.Context, []string) ([]string, error)) agenticruntime.WebSearcher {
	if fn == nil {
		return nil
	}
	return harnessWebSearcherFunc(fn)
}
