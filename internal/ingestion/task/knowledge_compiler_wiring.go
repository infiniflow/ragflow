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

package task

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	kc "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service"
)

// This file is the composition-root wiring for the KnowledgeCompiler ingestion
// component. The component package (internal/ingestion/component/knowledge_compiler)
// is deliberately DB-independent: it owns the compile schema but not the model
// resolution or the storage engine. The DepsResolver seam is injected here, at
// the task-package level, so the component never imports internal/service
// directly (which would invert the dependency direction — see PORT_PLAN.md §4).
//
// The component returns its compiled knowledge units as chunk-aligned docs merged
// into the upstream chunk stream, so it needs no separate writer: the caller
// (pipeline / downstream tokenizer) handles any persistence, exactly as it does
// for ordinary chunks.

func init() {
	kc.SetDepsResolver(newKnowledgeCompilerDepsResolver())
	kc.SetGroupResolver(newKnowledgeCompilerGroupResolver())
}

// newKnowledgeCompilerGroupResolver builds the production GroupResolver backed by
// the compilation_template DAO. Without it, any config carrying
// compilation_template_group_ids would fail loud at runtime (the component
// refuses to silently drop the compilation_template_ids stamp). It resolves each
// group id to its child template ids so group-based configs stamp the full set
// on every compiled unit.
func newKnowledgeCompilerGroupResolver() kc.GroupResolver {
	tmplDAO := dao.NewCompilationTemplateDAO()
	return func(ctx context.Context, tenantID string, groupIDs []string) ([]string, error) {
		return tmplDAO.ResolveGroupTemplateIDs(ctx, tenantID, groupIDs)
	}
}

// newKnowledgeCompilerDepsResolver builds the production DepsResolver. Each call
// yields a fresh Deps whose ChatInvoker / Embedder are bound to the resolved
// tenant + model ids (captured in the closure), mirroring how the Tokenizer
// component resolves its embedder.
func newKnowledgeCompilerDepsResolver() kc.DepsResolver {
	svc := service.NewModelProviderService()
	return func(tenantID, llmID, embeddingModel string) (kc.Deps, error) {
		if strings.TrimSpace(llmID) == "" {
			return kc.Deps{}, fmt.Errorf("knowledge_compiler: llm_id is required for production deps resolution")
		}
		// Resolve the chat model's context window so RAPTOR can truncate each
		// cluster's texts to fit the LLM context (mirrors Python self._llm_model.max_length).
		llmMax := kc.DefaultLLMContextLength
		// Bound the model-config lookup so a stalled provider/instance DB read
		// cannot block document ingestion indefinitely.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, _, _, ml, merr := svc.ResolveModelConfig(ctx, tenantID, entity.ModelTypeChat, llmID); merr == nil && ml > 0 {
			llmMax = ml
		}

		return kc.Deps{
			Chat:      &kcChatInvoker{svc: svc, tenantID: tenantID, llmID: llmID},
			Embed:     &kcEmbedder{svc: svc, tenantID: tenantID, embdID: embeddingModel},
			WikiPages: &kcWikiPageStore{docEngine: engine.Get()},
			// HistoricalKNN / Redis are optional (wiki historical dedup,
			// datasetnav lock). They are wired separately when the
			// surrounding pipeline supplies the backing services.
			LLMMaxLength: llmMax,
		}, nil
	}
}

// kcChatInvoker adapts service.ModelProviderService.Chat to the
// knowledge_compiler ChatInvoker seam.
type kcChatInvoker struct {
	svc      *service.ModelProviderService
	tenantID string
	llmID    string
}

func (c *kcChatInvoker) Chat(ctx context.Context, req kc.ChatRequest) (*kc.ChatResponse, error) {
	llmID := c.llmID
	if req.LLMID != "" {
		llmID = req.LLMID
	}
	msgs := []models.Message{
		{Role: "system", Content: req.SystemPrompt},
		{Role: "user", Content: req.UserPrompt},
	}
	// Python's knowledge compilation pins per-call-site temperatures
	// (extraction 0.1, merge judging 0.0); nil leaves the driver default.
	var config *models.ChatConfig
	if req.Temperature != nil || req.MaxTokens != nil {
		config = &models.ChatConfig{}
		if req.Temperature != nil {
			config.Temperature = req.Temperature
		}
		// MaxTokens caps the generated summary length (mirrors Python's
		// {"max_tokens": max(self._max_token, 512)}, issue #10235).
		if req.MaxTokens != nil {
			config.MaxTokens = req.MaxTokens
		}
	}
	resp, err := c.svc.Chat(ctx, c.tenantID, llmID, msgs, config)
	if err != nil {
		return nil, err
	}
	content := ""
	if resp != nil && resp.Answer != nil {
		content = *resp.Answer
	}
	return &kc.ChatResponse{Content: content}, nil
}

// kcEmbedder adapts service.ModelProviderService.GetEmbeddingModel to the
// knowledge_compiler Embedder seam. Vectors are returned as []float32 to match
// the component's product schema.
type kcEmbedder struct {
	svc      *service.ModelProviderService
	tenantID string
	embdID   string
	dim      atomic.Int64
}

func (e *kcEmbedder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	embdID := strings.TrimSpace(e.embdID)
	if embdID == "" {
		return nil, fmt.Errorf("knowledge_compiler: embedding_model is required for production embedding")
	}
	mdl, err := e.svc.GetEmbeddingModel(ctx, e.tenantID, embdID)
	if err != nil {
		return nil, fmt.Errorf("knowledge_compiler: resolve embedding model: %w", err)
	}
	if mdl == nil || mdl.ModelDriver == nil {
		return nil, fmt.Errorf("knowledge_compiler: embedding model %q is unavailable", embdID)
	}
	config := &models.EmbeddingConfig{}
	// Embed expects *string for the model name; nil ModelUsage (not tracked here).
	embeds, err := mdl.ModelDriver.Embed(ctx, mdl.ModelName, texts, mdl.APIConfig, config, nil)
	if err != nil {
		return nil, fmt.Errorf("knowledge_compiler: embed: %w", err)
	}
	out := make([][]float32, len(embeds))
	for i, v := range embeds {
		out[i] = float64sToFloat32(v.Embedding)
	}
	if len(out) > 0 {
		e.dim.CompareAndSwap(0, int64(len(out[0])))
	}
	return out, nil
}

func (e *kcEmbedder) Dimensions() int { return int(e.dim.Load()) }

// float64sToFloat32 converts an embedding vector to the product schema's
// []float32 representation.
func float64sToFloat32(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, x := range in {
		out[i] = float32(x)
	}
	return out
}

type kcWikiPageStore struct {
	docEngine engine.DocEngine
}

func (s *kcWikiPageStore) FindSimilarPages(ctx context.Context, tenantID, datasetID string, queryVec []float32, k int) ([]kc.WikiPageCandidate, error) {
	if s == nil || s.docEngine == nil || len(queryVec) == 0 || k <= 0 || strings.TrimSpace(datasetID) == "" {
		return nil, nil
	}
	vec := make([]float64, len(queryVec))
	for i, v := range queryVec {
		vec[i] = float64(v)
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        []string{datasetID},
		Limit:        k,
		SelectFields: []string{"id", "slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "summary_with_weight", "content_with_weight", "entity_names_kwd", "related_kb_pages_kwd", "outlinks_kwd", "kc_content_md_raw", "_score"},
		Filter: map[string]interface{}{
			"compile_kwd": "artifact_page",
			"kc_kind":     "page",
		},
		MatchExprs: []interface{}{&enginetypes.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(vec)),
			EmbeddingData:     vec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              k,
			ExtraOptions:      map[string]interface{}{"similarity": 0.0},
		}},
	}
	res, err := s.docEngine.Search(ctx, req)
	if err != nil || res == nil {
		return nil, err
	}
	out := make([]kc.WikiPageCandidate, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		out = append(out, wikiPageCandidateFromRow(row))
	}
	return out, nil
}

func (s *kcWikiPageStore) GetPageBySlug(ctx context.Context, tenantID, datasetID, slug string) (*kc.WikiPageCandidate, error) {
	if s == nil || s.docEngine == nil || strings.TrimSpace(datasetID) == "" || strings.TrimSpace(slug) == "" {
		return nil, nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        []string{datasetID},
		Limit:        1,
		SelectFields: []string{"id", "slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "summary_with_weight", "content_with_weight", "entity_names_kwd", "related_kb_pages_kwd", "outlinks_kwd", "kc_content_md_raw", "_score"},
		Filter: map[string]interface{}{
			"compile_kwd": "artifact_page",
			"slug_kwd":    slug,
			"kc_kind":     "page",
		},
	}
	res, err := s.docEngine.Search(ctx, req)
	if err != nil || res == nil || len(res.Chunks) == 0 {
		return nil, err
	}
	page := wikiPageCandidateFromRow(res.Chunks[0])
	return &page, nil
}

func wikiPageCandidateFromRow(row map[string]interface{}) kc.WikiPageCandidate {
	return kc.WikiPageCandidate{
		ID:             strings.TrimSpace(anyString(row["id"])),
		Slug:           strings.TrimSpace(anyString(row["slug_kwd"])),
		Title:          strings.TrimSpace(anyString(row["title_kwd"])),
		PageType:       strings.TrimSpace(anyString(row["page_type_kwd"])),
		Topic:          strings.TrimSpace(anyString(row["topic_kwd"])),
		Summary:        strings.TrimSpace(anyString(row["summary_with_weight"])),
		ContentMD:      strings.TrimSpace(anyString(row["content_with_weight"])),
		ContentMDRaw:   strings.TrimSpace(anyString(row["kc_content_md_raw"])),
		EntityNames:    anyStrings(row["entity_names_kwd"]),
		RelatedKBPages: anyStrings(row["related_kb_pages_kwd"]),
		Outlinks:       anyStrings(row["outlinks_kwd"]),
		Score:          anyFloat(row["_score"]),
	}
}

func anyString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func anyStrings(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func anyFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}
