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
	knowledgecompiler "ragflow/internal/ingestion/component/knowledge_compiler"
	kc "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/knowledge_compile"
	"ragflow/internal/service"

	appcommon "ragflow/internal/common"

	"gorm.io/gorm"
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
	kc.SetTemplateResolver(newKnowledgeCompilerTemplateResolver())
	knowledge_compile.SetWikiDirtyCompiler(compileDirtyWikiDocument)
}

func compileDirtyWikiDocument(ctx context.Context, request knowledge_compile.WikiDirtyRequest) error {
	doc, err := dao.NewDocumentDAO().GetByID(ctx, dao.DB, request.DocumentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return fmt.Errorf("Wiki dirty compile: load document: %w", err)
	}
	if doc.KbID != request.DatasetID || (doc.Status != nil && *doc.Status == "0") {
		return replaceDirtyWikiProducts(ctx, request, nil, nil, nil, nil, true)
	}
	compilerParams, err := loadWikiCompilerParams(ctx, doc)
	if err != nil {
		return err
	}
	templateIDs, err := resolveWikiTemplateIDs(ctx, request.TenantID, compilerParams)
	if err != nil {
		return err
	}
	if len(templateIDs) == 0 {
		return replaceDirtyWikiProducts(ctx, request, nil, nil, nil, nil, true)
	}
	sourceChunks, err := loadActiveSourceChunks(ctx, request)
	if err != nil {
		return err
	}
	if len(sourceChunks) == 0 {
		return replaceDirtyWikiProducts(ctx, request, nil, nil, nil, nil, true)
	}
	compiled := make([]map[string]any, 0)
	affectedSlugs := make([]string, 0)
	removedSlugs := make([]string, 0)
	activeStates := make([]kc.WikiMapActiveState, 0)
	for _, templateID := range templateIDs {
		params := copyStringAnyMap(compilerParams)
		delete(params, "compilation_template_group_id")
		delete(params, "compilation_template_group_ids")
		params["compilation_template_id"] = templateID
		component, err := knowledgecompiler.NewKnowledgeCompilerComponent("Compiler", params)
		if err != nil {
			return err
		}
		output, err := component.Invoke(ctx, dao.DB, map[string]any{
			"chunks":           sourceChunks,
			"tenant_id":        request.TenantID,
			"dataset_id":       request.DatasetID,
			"kb_id":            request.DatasetID,
			"doc_id":           request.DocumentID,
			"wiki_incremental": true,
		})
		if err != nil {
			return fmt.Errorf("Wiki dirty compile document %s: %w", request.DocumentID, err)
		}
		compiled = append(compiled, wikiCompiledRows(output)...)
		affectedSlugs = append(affectedSlugs, stringValues(output["wiki_affected_slugs"])...)
		removedSlugs = append(removedSlugs, stringValues(output["wiki_removed_slugs"])...)
		states, err := wikiActiveStates(output)
		if err != nil {
			return fmt.Errorf("Wiki dirty compile document %s: %w", request.DocumentID, err)
		}
		activeStates = append(activeStates, states...)
	}
	if len(affectedSlugs) == 0 && len(removedSlugs) == 0 {
		return persistDirtyWikiActiveStates(ctx, request, activeStates)
	}
	return replaceDirtyWikiProducts(ctx, request, compiled, uniqueStrings(affectedSlugs), uniqueStrings(removedSlugs), activeStates, false)
}

func loadWikiCompilerParams(ctx context.Context, doc *entity.Document) (map[string]any, error) {
	if params := findCompilerParams(map[string]any(doc.ParserConfig)); params != nil {
		return params, nil
	}
	if doc.PipelineID == nil || strings.TrimSpace(*doc.PipelineID) == "" {
		return map[string]any{}, nil
	}
	canvas, err := dao.NewUserCanvasDAO().GetByID(ctx, dao.DB, *doc.PipelineID)
	if err != nil {
		if err == dao.ErrUserCanvasNotFound {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if params := findCompilerParams(map[string]any(canvas.DSL)); params != nil {
		return params, nil
	}
	return map[string]any{}, nil
}

func findCompilerParams(value any) map[string]any {
	switch typed := value.(type) {
	case entity.JSONMap:
		return findCompilerParams(map[string]any(typed))
	case map[string]any:
		if _, hasGroup := typed["compilation_template_group_id"]; hasGroup {
			return copyStringAnyMap(typed)
		}
		if _, hasGroups := typed["compilation_template_group_ids"]; hasGroups {
			return copyStringAnyMap(typed)
		}
		if _, hasTemplate := typed["compilation_template_id"]; hasTemplate {
			return copyStringAnyMap(typed)
		}
		for _, child := range typed {
			if params := findCompilerParams(child); params != nil {
				return params
			}
		}
	case []any:
		for _, child := range typed {
			if params := findCompilerParams(child); params != nil {
				return params
			}
		}
	}
	return nil
}

func resolveWikiTemplateIDs(ctx context.Context, tenantID string, params map[string]any) ([]string, error) {
	ids := make([]string, 0)
	if templateID, ok := params["compilation_template_id"].(string); ok && strings.TrimSpace(templateID) != "" {
		ids = append(ids, strings.TrimSpace(templateID))
	}
	groupIDs := stringValues(params["compilation_template_group_id"])
	groupIDs = append(groupIDs, stringValues(params["compilation_template_group_ids"])...)
	if len(groupIDs) > 0 {
		resolved, err := dao.NewCompilationTemplateDAO().ResolveGroupTemplateIDs(ctx, dao.DB, tenantID, uniqueStrings(groupIDs))
		if err != nil {
			return nil, err
		}
		ids = append(ids, resolved...)
	}
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	var templates []entity.CompilationTemplate
	if err := dao.DB.WithContext(ctx).
		Where("id IN ? AND status = ?", ids, string(entity.StatusValid)).Find(&templates).Error; err != nil {
		return nil, err
	}
	wikiIDs := make([]string, 0, len(templates))
	for _, template := range templates {
		kind := template.Kind
		if configured, ok := template.Config["kind"].(string); ok && strings.TrimSpace(configured) != "" {
			kind = configured
		}
		if strings.EqualFold(strings.TrimSpace(kind), "wiki") || strings.EqualFold(strings.TrimSpace(kind), "wiki_page") {
			wikiIDs = append(wikiIDs, template.ID)
		}
	}
	return uniqueStrings(wikiIDs), nil
}

func loadActiveSourceChunks(ctx context.Context, request knowledge_compile.WikiDirtyRequest) ([]map[string]any, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, fmt.Errorf("Wiki dirty compile: document engine is unavailable")
	}
	chunks := make([]map[string]any, 0)
	for offset := 0; ; offset += 1000 {
		result, err := docEngine.Search(ctx, &enginetypes.SearchRequest{
			IndexNames: []string{fmt.Sprintf("ragflow_%s", request.TenantID)},
			KbIDs:      []string{request.DatasetID},
			Offset:     offset,
			Limit:      1000,
			SelectFields: []string{
				"id", "doc_id", "docnm_kwd", "content_with_weight", "available_int", "compile_kwd",
			},
			Filter: map[string]any{
				"doc_id":        []string{request.DocumentID},
				"available_int": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			break
		}
		for _, row := range result.Chunks {
			if strings.TrimSpace(anyString(row["compile_kwd"])) != "" {
				continue
			}
			chunks = append(chunks, row)
		}
		if len(result.Chunks) == 0 || int64(offset+len(result.Chunks)) >= result.Total {
			break
		}
	}
	return chunks, nil
}

func wikiCompiledRows(output map[string]any) []map[string]any {
	rows := make([]map[string]any, 0)
	items, _ := output["chunks"].([]any)
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		compileKWD := strings.TrimSpace(anyString(row["compile_kwd"]))
		if compileKWD != "wiki_page" && compileKWD != "wiki_section" {
			continue
		}
		row["available_int"] = 0
		rows = append(rows, row)
	}
	return rows
}

func replaceDirtyWikiProducts(ctx context.Context, request knowledge_compile.WikiDirtyRequest, rows []map[string]any, affectedSlugs, removedSlugs []string, activeStates []kc.WikiMapActiveState, fullReplace bool) error {
	var dirty entity.WikiDocumentDirty
	if err := dao.DB.WithContext(ctx).Where("document_id = ?", request.DocumentID).First(&dirty).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if dirty.Revision != request.Revision {
		return nil
	}
	docEngine := engine.Get()
	if docEngine == nil {
		return fmt.Errorf("Wiki dirty compile: document engine is unavailable")
	}
	indexName := fmt.Sprintf("ragflow_%s", request.TenantID)
	existing, err := docEngine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:   []string{indexName},
		KbIDs:        []string{request.DatasetID},
		Limit:        10000,
		SelectFields: []string{"id", "compile_kwd", "slug_kwd", "parent_id"},
		Filter: map[string]any{
			"doc_id":        []string{request.DocumentID},
			"compile_kwd":   []string{"wiki_page", "wiki_section"},
			"available_int": 0,
		},
	})
	if err != nil {
		return err
	}
	newIDs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if id := anyString(row["id"]); id != "" {
			newIDs[id] = struct{}{}
		}
	}
	if len(rows) > 0 {
		if _, err := docEngine.InsertChunks(ctx, rows, indexName, request.DatasetID); err != nil {
			return err
		}
	}
	affected := make(map[string]struct{}, len(affectedSlugs)+len(removedSlugs))
	for _, slug := range append(append([]string(nil), affectedSlugs...), removedSlugs...) {
		affected[slug] = struct{}{}
	}
	affectedPageIDs := make(map[string]struct{})
	if existing != nil && !fullReplace {
		for _, row := range existing.Chunks {
			if anyString(row["compile_kwd"]) != "wiki_page" {
				continue
			}
			if _, ok := affected[anyString(row["slug_kwd"])]; ok {
				affectedPageIDs[anyString(row["id"])] = struct{}{}
			}
		}
	}
	if existing != nil && fullReplace {
		for _, row := range existing.Chunks {
			if anyString(row["compile_kwd"]) == "wiki_page" {
				removedSlugs = append(removedSlugs, anyString(row["slug_kwd"]))
			}
		}
		removedSlugs = uniqueStrings(removedSlugs)
	}
	staleIDs := make([]string, 0)
	if existing != nil {
		for _, row := range existing.Chunks {
			id := anyString(row["id"])
			_, pageAffected := affectedPageIDs[id]
			_, sectionAffected := affectedPageIDs[anyString(row["parent_id"])]
			inScope := fullReplace || pageAffected || sectionAffected
			if _, keep := newIDs[id]; id != "" && inScope && !keep {
				staleIDs = append(staleIDs, id)
			}
		}
	}
	if len(staleIDs) > 0 {
		if _, err := docEngine.DeleteChunks(ctx, map[string]any{"id": staleIDs, "kb_id": request.DatasetID}, indexName, request.DatasetID); err != nil {
			return err
		}
	}
	if fullReplace {
		if err := clearWikiActiveStates(ctx, docEngine, request); err != nil {
			return err
		}
	}
	if err := putWikiActiveStates(ctx, docEngine, activeStates); err != nil {
		return err
	}
	// Do not wake the dataset consumer when this document never had any Wiki
	// products and the dirty replacement produced none. A full replacement with
	// existing products still publishes so the consumer can retract them.
	if len(rows) == 0 && (existing == nil || len(existing.Chunks) == 0) {
		return nil
	}
	return knowledge_compile.PublishCompleted(ctx, request.TenantID, request.DatasetID, request.DocumentID, []string{"wiki"})
}

func clearWikiActiveStates(ctx context.Context, docEngine engine.DocEngine, request knowledge_compile.WikiDirtyRequest) error {
	_, err := docEngine.DeleteChunks(ctx, map[string]any{
		"kb_id":          request.DatasetID,
		"compile_kwd":    "wiki_map_active",
		"available_int":  0,
		"source_doc_ids": []string{request.DocumentID},
	}, fmt.Sprintf("ragflow_%s", request.TenantID), request.DatasetID)
	if err != nil {
		return fmt.Errorf("clear Wiki active MAP state for document %s: %w", request.DocumentID, err)
	}
	return nil
}

func persistDirtyWikiActiveStates(ctx context.Context, request knowledge_compile.WikiDirtyRequest, states []kc.WikiMapActiveState) error {
	if len(states) == 0 {
		return nil
	}
	var dirty entity.WikiDocumentDirty
	if err := dao.DB.WithContext(ctx).Where("document_id = ?", request.DocumentID).First(&dirty).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if dirty.Revision != request.Revision {
		return nil
	}
	docEngine := engine.Get()
	if docEngine == nil {
		return fmt.Errorf("Wiki dirty compile: document engine is unavailable")
	}
	return putWikiActiveStates(ctx, docEngine, states)
}

func putWikiActiveStates(ctx context.Context, docEngine engine.DocEngine, states []kc.WikiMapActiveState) error {
	if len(states) == 0 {
		return nil
	}
	store, ok := knowledge_compile.NewWikiMapVersionStore(docEngine).(kc.WikiMapActiveStateStore)
	if !ok {
		return fmt.Errorf("Wiki dirty compile: active MAP store is unavailable")
	}
	for _, state := range states {
		if err := store.PutWikiMapActiveState(ctx, state); err != nil {
			return err
		}
	}
	return nil
}

func copyStringAnyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				result = append(result, value)
			}
		}
		return result
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// newKnowledgeCompilerGroupResolver builds the production GroupResolver backed by
// the compilation_template DAO. Without it, any config carrying
// compilation_template_group_id would fail loud at runtime (the component
// refuses to silently drop the compilation_template_ids stamp). It resolves each
// group id to its child template ids so group-based configs stamp the full set
// on every compiled unit.
func newKnowledgeCompilerGroupResolver() kc.GroupResolver {
	tmplDAO := dao.NewCompilationTemplateDAO()
	return func(ctx context.Context, db *gorm.DB, tenantID string, groupIDs []string) ([]string, error) {
		return tmplDAO.ResolveGroupTemplateIDs(ctx, db, tenantID, groupIDs)
	}
}

// newKnowledgeCompilerTemplateResolver builds the production TemplateResolver
// backed by the compilation_template DAO. It loads a single template by id and
// returns its id, kind (which selects the Go variant via common.KindToVariant),
// and config (the template "content"). Without it, any config carrying
// compilation_template_id would fail loudly at runtime.
func newKnowledgeCompilerTemplateResolver() kc.TemplateResolver {
	tmplDAO := dao.NewCompilationTemplateDAO()
	return func(ctx context.Context, db *gorm.DB, tenantID, templateID string) (kc.TemplateInfo, error) {
		t, err := tmplDAO.GetTemplate(ctx, db, tenantID, templateID)
		if err != nil {
			return kc.TemplateInfo{}, err
		}
		return kc.TemplateInfo{
			ID:     t.ID,
			Kind:   t.Kind,
			Config: map[string]any(t.Config),
		}, nil
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
			// No explicit chat model was supplied (e.g. the dataset-level deduper
			// is seeded with a global default that may be empty). Resolve the
			// tenant's default chat model so cross-document LLM merging can
			// actually run instead of failing / falling back to a no-op. Use the
			// composite "<model>@<instance>@<provider>" reference (not the bare
			// model name) so later Chat/ResolveModelConfig round-trips can locate
			// the provider.
			defaultRef, derr := svc.GetTenantDefaultModelRef(context.Background(), tenantID, entity.ModelTypeChat)
			if derr != nil || strings.TrimSpace(defaultRef) == "" {
				return kc.Deps{}, fmt.Errorf("knowledge_compiler: llm_id is empty and no tenant default chat model available: %w", derr)
			}
			llmID = defaultRef
			// Keep the fall-through path below for ModelContextLen resolution so
			// both explicit and default model refs share one context-window path.
		}
		// Resolve the chat model's context window so RAPTOR can truncate each
		// cluster's texts to fit the LLM context (mirrors Python self._llm_model.max_length).
		// This uses content_length (PR #17839) — the total context window — not
		// max_output. max_output is only the generation cap; using it as the
		// budget source would collapse per-chunk input quotas.
		llmMax := kc.DefaultLLMContextLength
		// Bound the model-config lookup so a stalled provider/instance DB read
		// cannot block document ingestion indefinitely.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if ml, merr := svc.ResolveModelContextLength(ctx, tenantID, llmID); merr == nil && ml > 0 {
			llmMax = ml
		}
		// Resolve the model's generation cap (max_output). Cross-document merge
		// judging packs many pairs into one LLM call; the batch must be bounded by
		// BOTH the input window and this output cap, so a large candidate set
		// never overflows max_output and yields a truncated/non-JSON reply. This
		// uses max_tokens (the generation cap), NOT content_length — see
		// ResolveModelContextLength's comment.
		llmMaxOutput := 0
		if _, _, _, mo, merr := svc.ResolveModelConfig(ctx, tenantID, entity.ModelTypeChat, llmID); merr == nil && mo > 0 {
			llmMaxOutput = mo
		}

		return kc.Deps{
			Chat:            &kcChatInvoker{svc: svc, tenantID: tenantID, llmID: llmID},
			Embed:           &kcEmbedder{svc: svc, tenantID: tenantID, embdID: embeddingModel},
			WikiPages:       &kcWikiPageStore{docEngine: engine.Get()},
			WikiMapVersions: knowledge_compile.NewWikiMapVersionStore(engine.Get()),
			// HistoricalKNN / Redis are optional (wiki historical dedup,
			// datasetnav lock). They are wired separately when the
			// surrounding pipeline supplies the backing services.
			ModelContextLen: llmMax,
			ModelMaxOutput:  llmMaxOutput,
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
	// Retry transient transport/provider failures (HTTP timeout, reset,
	// connection refused, 5xx, 429) with exponential backoff. A single
	// external-LLM hiccup must not abort the whole knowledge compile — the reply
	// is never cached, so each attempt issues a fresh request. Permanent
	// configuration/model errors (auth, unknown model) are not retried.
	var resp *models.ChatResponse
	call := func() error {
		// Bound each attempt to a short deadline so a stalled LLM provider (e.g.
		// MiniMax hanging on a large merge-judge prompt) surfaces a timeout
		// quickly instead of blocking a compile sub-batch for minutes; the
		// retry/backoff loop above then handles it as a transient failure.
		attemptCtx, cancel := context.WithTimeout(ctx, kcChatAttemptTimeout)
		defer cancel()
		r, err := c.svc.Chat(attemptCtx, c.tenantID, llmID, msgs, config)
		if err != nil {
			return err
		}
		resp = r
		return nil
	}
	if req.DisableRetry {
		if err := call(); err != nil {
			return nil, err
		}
	} else if retryErr := appcommon.RetryWithBackoff(ctx, kcChatRetryMax, kcChatRetryDelay, call, appcommon.IsTransientError); retryErr != nil {
		return nil, retryErr
	}
	content := ""
	if resp != nil && resp.Answer != nil {
		content = *resp.Answer
	}
	return &kc.ChatResponse{Content: content}, nil
}

// kcChatRetryMax bounds how many times a transient LLM transport failure is
// retried. Each attempt may run up to the driver's HTTP timeout, so the count
// stays small to avoid unbounded wall-clock latency inside one compile.
const kcChatRetryMax = 5

// kcChatAttemptTimeout bounds a single Chat call (per retry attempt). A stalled
// provider must surface a timeout promptly rather than hold a compile sub-batch;
// 3 minutes is long enough for a big merge-judge prompt yet short enough that
// several failed attempts do not stall the pipeline for many minutes.
const kcChatAttemptTimeout = 3 * time.Minute

// kcChatRetryDelay is the initial exponential-backoff delay between retries.
const kcChatRetryDelay = 2 * time.Second

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
	mdl, err := e.resolveModel(ctx)
	if err != nil {
		return nil, err
	}
	config := &models.EmbeddingConfig{}
	// Slice inputs into per-provider batches: providers cap the per-request input
	// count and reject larger batches rather than chunking internally. The batch
	// size is resolved from the model's capability (all_models.json batch_size,
	// added by #17877/#17878) via EmbeddingModel.ResolveBatchSize, which falls
	// back to a conservative default. Batches are fanned out on the shared compiler
	// pool and concatenated back in input order.
	batchSize := mdl.ResolveBatchSize()
	numBatches := (len(texts) + batchSize - 1) / batchSize
	slots := make([][][]float32, numBatches) // per-batch vector lists, distinct indices => no race
	jobs := make([]knowledge_compile.CompilerJob, 0, numBatches)
	for b := 0; b < numBatches; b++ {
		b := b
		start := b * batchSize
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchTexts := texts[start:end]
		jobs = append(jobs, func() error {
			if err = ctx.Err(); err != nil {
				return err
			}
			var embeds []models.EmbeddingData
			embeds, err = mdl.ModelDriver.Embed(ctx, mdl.ModelName, models.EmbedRequest{Texts: batchTexts}, mdl.APIConfig, config, nil)
			if err != nil {
				return fmt.Errorf("knowledge_compiler: embed: %w", err)
			}
			vecs := make([][]float32, len(embeds))
			for i, v := range embeds {
				vecs[i] = float64sToFloat32(v.Embedding)
			}
			slots[b] = vecs
			return nil
		})
	}
	if err = knowledge_compile.SubmitCompilerJobs(ctx, jobs); err != nil {
		return nil, err
	}
	// Flatten in input order and derive the vector dimension from the first
	// batch's first vector.
	out := make([][]float32, 0, len(texts))
	var batchDim int
	for _, slot := range slots {
		for _, vec := range slot {
			out = append(out, vec)
			if batchDim == 0 {
				batchDim = len(vec)
			}
		}
	}
	if batchDim > 0 {
		e.dim.CompareAndSwap(0, int64(batchDim))
	}
	return out, nil
}

// resolveModel returns the embedding model to embed with. It prefers the
// explicitly configured embedding_model; when the caller left it unset, it falls
// back to the tenant's default embedding model (mirrors Python, which uses the
// KB/tenant's configured embedding model for wiki compilation). A clear error is
// returned only when neither is available, so a KB with no embedding model fails
// loudly instead of silently producing empty vectors.
func (e *kcEmbedder) resolveModel(ctx context.Context) (*models.EmbeddingModel, error) {
	if embdID := strings.TrimSpace(e.embdID); embdID != "" {
		mdl, err := e.svc.GetEmbeddingModel(ctx, e.tenantID, embdID)
		if err != nil {
			return nil, fmt.Errorf("knowledge_compiler: resolve embedding model: %w", err)
		}
		if mdl == nil || mdl.ModelDriver == nil {
			return nil, fmt.Errorf("knowledge_compiler: embedding model %q is unavailable", embdID)
		}
		return mdl, nil
	}
	driver, name, apiConfig, _, err := e.svc.GetTenantDefaultModelByType(ctx, e.tenantID, entity.ModelTypeEmbedding)
	if err != nil {
		return nil, fmt.Errorf("knowledge_compiler: embedding_model is required and no tenant default embedding model is set: %w", err)
	}
	if driver == nil || name == "" {
		return nil, fmt.Errorf("knowledge_compiler: embedding_model is required (tenant default embedding model unavailable)")
	}
	return &models.EmbeddingModel{ModelDriver: driver, ModelName: &name, APIConfig: apiConfig}, nil
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
		SelectFields: []string{"id", "slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "plan_group_kwd", "summary_with_weight", "content_with_weight", "entity_names_kwd", "related_kb_pages_kwd", "outlinks_kwd", "source_chunk_ids", "kc_content_md_raw", "_score"},
		// compile_kwd="wiki_page" is the schema-backed discriminator for wiki
		// pages (sections carry compile_kwd="wiki_section"); there is no
		// "kc_kind" column in the chunk schema, so filtering on it would return
		// empty on Infinity.
		Filter: map[string]interface{}{
			"compile_kwd": "wiki_page",
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
		SelectFields: []string{"id", "slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "plan_group_kwd", "summary_with_weight", "content_with_weight", "entity_names_kwd", "related_kb_pages_kwd", "outlinks_kwd", "source_chunk_ids", "kc_content_md_raw", "_score"},
		Filter: map[string]interface{}{
			"compile_kwd": "wiki_page",
			"slug_kwd":    slug,
		},
	}
	res, err := s.docEngine.Search(ctx, req)
	if err != nil || res == nil || len(res.Chunks) == 0 {
		return nil, err
	}
	page := wikiPageCandidateFromRow(res.Chunks[0])
	return &page, nil
}

func (s *kcWikiPageStore) FindPagesBySourceChunks(ctx context.Context, tenantID, datasetID string, chunkIDs []string, k int) ([]kc.WikiPageCandidate, error) {
	if s == nil || s.docEngine == nil || len(chunkIDs) == 0 || k <= 0 || strings.TrimSpace(datasetID) == "" {
		return nil, nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        []string{datasetID},
		Limit:        k,
		SelectFields: []string{"id", "slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "summary_with_weight", "content_with_weight", "entity_names_kwd", "related_kb_pages_kwd", "outlinks_kwd", "source_chunk_ids", "kc_content_md_raw", "_score"},
		Filter: map[string]interface{}{
			"compile_kwd":      "wiki_page",
			"source_chunk_ids": chunkIDs,
		},
	}
	res, err := s.docEngine.Search(ctx, req)
	if err != nil || res == nil {
		return nil, err
	}
	out := make([]kc.WikiPageCandidate, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		candidate := wikiPageCandidateFromRow(row)
		if candidate.Score == 0 {
			candidate.Score = 0.68
		}
		out = append(out, candidate)
	}
	return out, nil
}

func wikiPageCandidateFromRow(row map[string]interface{}) kc.WikiPageCandidate {
	return kc.WikiPageCandidate{
		ID:             strings.TrimSpace(anyString(row["id"])),
		Slug:           strings.TrimSpace(anyString(row["slug_kwd"])),
		Title:          strings.TrimSpace(anyString(row["title_kwd"])),
		PageType:       strings.TrimSpace(anyString(row["page_type_kwd"])),
		Topic:          strings.TrimSpace(anyString(row["topic_kwd"])),
		PlanGroup:      strings.TrimSpace(anyString(row["plan_group_kwd"])),
		Summary:        strings.TrimSpace(anyString(row["summary_with_weight"])),
		ContentMD:      strings.TrimSpace(anyString(row["content_with_weight"])),
		ContentMDRaw:   strings.TrimSpace(anyString(row["kc_content_md_raw"])),
		EntityNames:    anyStrings(row["entity_names_kwd"]),
		RelatedKBPages: anyStrings(row["related_kb_pages_kwd"]),
		Outlinks:       anyStrings(row["outlinks_kwd"]),
		SourceChunkIDs: anyStrings(row["source_chunk_ids"]),
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
