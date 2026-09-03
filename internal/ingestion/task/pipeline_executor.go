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
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/utility"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/globals"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/knowledge_compile"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	indexdoc "ragflow/internal/ingestion/task/indexdoc"

	"gorm.io/gorm"
)

// PipelineResult is the outcome of a pipeline run: chunks have been
// indexed, and these bookkeeping inputs remain for the caller to apply to
// document state (metadata merge + chunk/token counter bumps).
type PipelineResult struct {
	DocID            string
	KbID             string
	Metadata         map[string]any
	Chunks           []map[string]any // populated only in debug (dry-run) mode
	ChunkCount       int
	TokenConsumption int
	Duration         float64 // pipeline wall-clock seconds
	// DocName, BuiltInMetadataConfig and AutoMetadataEnabled carry what the
	// document-state finalizer needs to apply built-in metadata
	// (update_time / file_name), mirroring Python apply_built_in_metadata.
	DocName               string
	BuiltInMetadataConfig []any
	AutoMetadataEnabled   bool
	// MessageID is the polling key for the debug-run log. The front-end reads
	// it from the run response and polls GET /agents/:id/logs/:message_id to
	// render progress; it is empty for non-debug (persist) runs.
	MessageID string
}

type PipelineExecutor struct {
	taskCtx     *TaskContext
	canvasID    string
	docBulkSize int

	indexWriter     *chunkIndexWriter
	logCreateFunc   func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error
	loadDSLFunc     func(ctx context.Context, canvasID string) (string, string, error)
	runPipelineFunc func(ctx context.Context, dsl string) (map[string]any, string, error)
	progressSink    pipelinepkg.ProgressSink
	requireResume   bool // when true, the pipeline run passes WithRequireResume
}

func validateTaskContext(taskCtx *TaskContext) error {
	if taskCtx == nil {
		return fmt.Errorf("pipeline executor: nil task context")
	}
	if taskCtx.Doc.ID == "" {
		return fmt.Errorf("pipeline executor: empty document id")
	}
	// A debug (dry-run) context carries no knowledgebase (see
	// TaskContext.IsDebug), so it must not be required to supply one.
	if !taskCtx.IsDebug() && taskCtx.Doc.KbID == "" {
		return fmt.Errorf("pipeline executor: empty document knowledgebase id")
	}
	if taskCtx.Doc.Name == nil || *taskCtx.Doc.Name == "" {
		return fmt.Errorf("pipeline executor: empty document name")
	}
	if taskCtx.Tenant.ID == "" {
		return fmt.Errorf("pipeline executor: empty tenant id")
	}
	return nil
}

func NewPipelineExecutor(
	taskCtx *TaskContext,
	canvasID string,
	docBulkSize int,
) (*PipelineExecutor, error) {
	if err := validateTaskContext(taskCtx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(canvasID) == "" {
		return nil, fmt.Errorf("pipeline executor: empty canvas id")
	}
	svc := &PipelineExecutor{
		taskCtx:     taskCtx,
		canvasID:    canvasID,
		docBulkSize: docBulkSize,
		indexWriter: newChunkIndexWriter(
			func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error) {
				return engine.Get().InsertChunks(ctx, chunks, baseName, datasetID)
			},
			fmt.Sprintf("ragflow_%s", taskCtx.Tenant.ID),
			taskCtx.Doc.KbID,
			docBulkSize,
		),
		logCreateFunc: dao.NewPipelineOperationLogDAO().Create,
	}
	svc.loadDSLFunc = svc.loadDSLFromCanvas
	svc.runPipelineFunc = svc.runPipelineWithDSL
	return svc, nil
}

func (s *PipelineExecutor) WithInsertFunc(f InsertFunc) *PipelineExecutor {
	s.indexWriter.insertFunc = f
	return s
}

func (s *PipelineExecutor) WithLogCreateFunc(f func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error) *PipelineExecutor {
	s.logCreateFunc = f
	return s
}

func (s *PipelineExecutor) WithLoadDSLFunc(f func(ctx context.Context, canvasID string) (string, string, error)) *PipelineExecutor {
	s.loadDSLFunc = f
	return s
}

func (s *PipelineExecutor) WithRunPipelineFunc(f func(ctx context.Context, dsl string) (map[string]any, string, error)) *PipelineExecutor {
	s.runPipelineFunc = f
	return s
}

// WithProgressSink injects a sink that receives pipeline component progress
// events. The sink owns all document/ingestion_task_log persistence; when
// unset, the pipeline runs DB-independent (progress events are dropped).
func (s *PipelineExecutor) WithProgressSink(sink pipelinepkg.ProgressSink) *PipelineExecutor {
	s.progressSink = sink
	return s
}

// WithRequireResume makes the pipeline refuse to start when no checkpoint
// store is resolvable (Redis down or not configured). Production ingestion
// sets this; tests skip it so they can exercise runPlain without Redis.
func (s *PipelineExecutor) WithRequireResume() *PipelineExecutor {
	s.requireResume = true
	return s
}

func (s *PipelineExecutor) KB() *entity.Knowledgebase { return &s.taskCtx.KB }
func (s *PipelineExecutor) Doc() *entity.Document     { return &s.taskCtx.Doc }
func (s *PipelineExecutor) Tenant() *entity.Tenant    { return &s.taskCtx.Tenant }

func (s *PipelineExecutor) Execute(ctx context.Context) (*PipelineResult, error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dsl, correctedID, err := s.resolveDSL(ctx)
	if err != nil {
		return nil, err
	}
	if correctedID != "" {
		s.canvasID = correctedID
	}

	pipelineOutput, pipelineDSL, err := s.runPipelineFunc(ctx, dsl)
	if err != nil {
		return nil, err
	}

	// A debug (dry-run) run produces no persistent side effect (no MinIO
	// image upload, no index insert, no pipeline log); see TaskContext.IsDebug.
	if s.taskCtx.IsDebug() {
		return s.collectDebugOutput(ctx, pipelineOutput, start)
	}

	result, err := s.processOutput(ctx, pipelineOutput, start)
	if err != nil {
		return nil, err
	}

	if pipelineDSL != "" {
		s.recordPipelineLog(context.WithoutCancel(ctx), dao.DB, s.taskCtx.Doc.ID, pipelineDSL, "")
	}

	return result, nil
}

func (s *PipelineExecutor) resolveDSL(ctx context.Context) (string, string, error) {
	if s.taskCtx != nil && s.taskCtx.IngestionTask != nil {
		if rerun, ok := s.taskCtx.IngestionTask.RerunInfo(); ok && rerun.DSL != nil {
			raw, err := json.Marshal(rerun.DSL)
			if err != nil {
				return "", "", fmt.Errorf("marshal rerun dsl: %w", err)
			}
			return string(raw), s.canvasID, nil
		}
	}
	return s.loadDSLFunc(ctx, s.canvasID)
}

// collectDebugOutput builds a PipelineResult for a debug (dry-run) run.
// It surfaces the pipeline's chunks so a debug endpoint can render them, but
// performs no DB/index writes — the embedding vectors already computed by the
// pipeline run are left on the chunks. This keeps debug runs side-effect free.
func (s *PipelineExecutor) collectDebugOutput(ctx context.Context, pipelineOutput map[string]any, start time.Time) (*PipelineResult, error) {
	chunks := indexdoc.NormalizeChunks(pipelineOutput)
	return &PipelineResult{
		DocID:            s.taskCtx.Doc.ID,
		KbID:             s.taskCtx.Doc.KbID,
		Chunks:           chunks,
		ChunkCount:       countOriginalChunkIDs(chunks),
		TokenConsumption: indexdoc.GetEmbeddingTokenConsumption(pipelineOutput),
		Duration:         time.Since(start).Seconds(),
	}, nil
}

func (s *PipelineExecutor) processOutput(ctx context.Context, pipelineOutput map[string]any, start time.Time) (*PipelineResult, error) {
	if pipelineOutput == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	chunks := indexdoc.NormalizeChunks(pipelineOutput)
	if len(chunks) == 0 {
		return nil, nil
	}

	embeddingTokenConsumption := indexdoc.GetEmbeddingTokenConsumption(pipelineOutput)
	metadata, err := indexdoc.ProcessChunksForPipeline(
		chunks,
		s.taskCtx.Doc.ID,
		*s.taskCtx.Doc.Name,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	tableMeta := indexdoc.AggregateTableDocMetadata(chunks, map[string]interface{}(s.taskCtx.Doc.ParserConfig))
	if tableMeta != nil {
		if metadata == nil {
			metadata = make(map[string]any)
		}
		for k, v := range tableMeta {
			if _, exists := metadata[k]; !exists {
				metadata[k] = v
			}
		}
	}

	// Per-document compiled knowledge products (those emitted by the
	// KnowledgeCompiler component and stamped with `compile_kwd`) are persisted
	// as available_int=0: invisible to the normal retriever until the dataset-level
	// post-processing consumer (§11) merges them into available_int=1 products.
	// Ordinary source chunks stay available_int=1 (the index default) unless the
	// document itself is disabled (status=0).
	markCompiledProductsHidden(chunks)
	// Reload status immediately before write: LoadFromIngestionTask copied a
	// snapshot that may be stale if BatchUpdateDocumentStatus ran mid-pipeline.
	docStatus := s.taskCtx.Doc.Status
	if dao.DB != nil {
		if persisted, err := dao.NewDocumentDAO().GetByID(ctx, dao.DB, s.taskCtx.Doc.ID); err == nil && persisted != nil {
			docStatus = persisted.Status
			s.taskCtx.Doc.Status = persisted.Status
		} else if err != nil {
			common.Warn(fmt.Sprintf("failed to reload document %s status before availability stamp: %v", s.taskCtx.Doc.ID, err))
		}
	}
	applyDocumentAvailability(chunks, docStatus)

	oldCompiledProductIDs, oldCompiledVariants, err := s.loadDocumentCompiledState(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.indexWriter.Write(ctx, chunks); err != nil {
		return nil, err
	}
	if err := s.reconcileDocumentCompiledProducts(ctx, oldCompiledProductIDs, chunks); err != nil {
		return nil, err
	}
	activeStates, err := wikiActiveStates(pipelineOutput)
	if err != nil {
		return nil, err
	}
	if err := putWikiActiveStates(ctx, engine.Get(), activeStates); err != nil {
		return nil, fmt.Errorf("persist Wiki active MAP state: %w", err)
	}

	// All chunks are now persisted. Notify the dataset-level post-processing consumer
	// (§11) that this document is complete: its compiled products were written
	// available_int=0 and the consumer later merges them into dataset-level products
	// (available_int=1). The notification is sent only after a successful persist
	// and is best-effort / non-fatal — a delivery failure is logged but does not
	// fail the pipeline task.
	//
	// The variants passed to PublishCompleted are the union of the compile types
	// in the previous and current document generations. They are derived from the
	// authoritative `compilation_template_kind_kwd` the KnowledgeCompiler
	// component stamps on each compiled product (the resolved template's kind →
	// KindToVariant, O2a whitelist). Keeping the previous types lets the dataset
	// consumer retract stale merged products when a template is removed.
	eventVariants := mergeCompiledVariants(oldCompiledVariants, compiledVariants(chunks))
	if len(eventVariants) > 0 {
		if err := knowledge_compile.PublishCompleted(ctx, s.taskCtx.Tenant.ID, s.taskCtx.Doc.KbID, s.taskCtx.Doc.ID, eventVariants); err != nil {
			common.Logger.Warn(fmt.Sprintf("knowledge_compile: publish doc_completed for %s failed: %v", s.taskCtx.Doc.ID, err))
		}
	}

	// Compilation products are derived artifacts and must not inflate the
	// document's source chunk counter shown by the document list API.
	chunkCount := countOriginalChunkIDs(chunks)

	builtInMetadata, autoMetaEnabled := builtInMetadataFromParserConfig(
		s.taskCtx.Doc.ParserConfig,
	)

	return &PipelineResult{
		DocID:                 s.taskCtx.Doc.ID,
		KbID:                  s.taskCtx.Doc.KbID,
		Metadata:              metadata,
		ChunkCount:            chunkCount,
		TokenConsumption:      embeddingTokenConsumption,
		Duration:              time.Since(start).Seconds(),
		DocName:               docNameValue(s.taskCtx.Doc.Name),
		BuiltInMetadataConfig: builtInMetadata,
		AutoMetadataEnabled:   autoMetaEnabled,
	}, nil
}

// builtInMetadataFromParserConfig extracts the built-in metadata config
// (update_time / file_name) and whether auto-metadata is enabled from the
// component-scoped Extractor node's modular metadata config. Legacy flat
// fields (enable_metadata / metadata_config / built_in_metadata at either the
// top level or on the node) are intentionally not supported.
func builtInMetadataFromParserConfig(parserConfig entity.JSONMap) ([]any, bool) {
	var extractorKeys []string
	for k := range parserConfig {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "extractor:") || strings.HasPrefix(lower, "extractor_") {
			extractorKeys = append(extractorKeys, k)
		}
	}
	sort.Strings(extractorKeys)

	for _, k := range extractorKeys {
		nodeRaw := parserConfig[k]
		if node, ok := nodeRaw.(map[string]any); ok {
			if metaObj, ok := node["metadata"].(map[string]any); ok {
				arr := metadataFieldSlice(metaObj["built_in_metadata"])
				return arr, parserConfigBool(metaObj["enabled"])
			}
		}
	}
	return nil, false
}

// metadataFieldSlice normalizes a built_in_metadata / metadata value that may
// arrive as []interface{} (DB round-trip) or []map[string]interface{} (in-memory
// construction) into a []any.
func metadataFieldSlice(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	if list, ok := value.([]map[string]any); ok {
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, item)
		}
		return out
	}
	return nil
}

// parserConfigBool coerces a parser_config boolean-like value (bool / number)
// to bool, mirroring the frontend's enable_metadata handling.
func parserConfigBool(v any) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case float64:
		return typed > 0
	case int:
		return typed > 0
	}
	return false
}

func docNameValue(name *string) string {
	if name == nil {
		return ""
	}
	return *name
}

// countOriginalChunkIDs returns the number of distinct source chunk IDs in the
// slice. Knowledge-compiler products are also emitted as index chunks, but
// carry compile_kwd and must not be included in the document's chunk_count.
// After ProcessChunksForPipeline, every chunk carries an "id" field computed
// from xxhash(text+docID). Chunks with identical text share the same id and
// the search engine treats them as upserts, so the effective stored chunk count
// is the number of unique ids — not len(chunks). Mirrors the index-side
// deduplication that happens at write time.
func countOriginalChunkIDs(chunks []map[string]any) int {
	seen := make(map[string]struct{}, len(chunks))
	for _, ck := range chunks {
		if _, compiled := ck["compile_kwd"]; compiled {
			continue
		}
		id, _ := ck["id"].(string)
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	return len(seen)
}

// markCompiledProductsHidden sets available_int=0 on the per-document compiled
// knowledge products so they are hidden from the retriever until the dataset-level
// post-processing consumer merges them into available_int=1 products (§11). A
// chunk is a compiled product iff it carries the compile_kwd discriminator the
// KnowledgeCompiler component stamps; ordinary source chunks (no compile_kwd)
// keep the index default available_int=1 and remain immediately searchable.
// Merged dataset-level products are written by the consumer, never here, so they are
// never double-marked.
func markCompiledProductsHidden(chunks []map[string]any) {
	for _, ck := range chunks {
		if _, ok := ck["compile_kwd"]; !ok {
			continue
		}
		ck["available_int"] = 0
	}
}

// loadDocumentCompiledState snapshots the previous successful document
// compiler generation before the new pipeline output is written. Reading first
// avoids relying on immediate search visibility after a bulk index write.
func (s *PipelineExecutor) loadDocumentCompiledState(ctx context.Context) ([]string, []string, error) {
	docEngine := engine.Get()
	if docEngine == nil || s == nil || s.taskCtx == nil {
		return nil, nil, nil
	}
	const pageSize = 1000
	indexName := fmt.Sprintf("ragflow_%s", s.taskCtx.Tenant.ID)
	oldIDs := make([]string, 0)
	oldProducts := make([]map[string]any, 0)
	for offset := 0; ; offset += pageSize {
		result, err := docEngine.Search(ctx, &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{s.taskCtx.Doc.KbID},
			Offset:       offset,
			Limit:        pageSize,
			SelectFields: []string{"id", "compile_kwd", "compilation_template_kind_kwd"},
			Filter:       map[string]any{"doc_id": []string{s.taskCtx.Doc.ID}},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("load document compiler products: %w", err)
		}
		if result == nil || len(result.Chunks) == 0 {
			break
		}
		for _, row := range result.Chunks {
			if strings.TrimSpace(asCompiledKwd(row)) == "" {
				continue
			}
			oldProducts = append(oldProducts, row)
			if id := strings.TrimSpace(anyString(row["id"])); id != "" {
				oldIDs = append(oldIDs, id)
			}
		}
		if int64(offset+len(result.Chunks)) >= result.Total {
			break
		}
	}
	return oldIDs, compiledVariants(oldProducts), nil
}

// reconcileDocumentCompiledProducts advances the document-level compiler
// generation only after the new pipeline output has been persisted. Reparse
// preparation keeps the previous compiled rows so a failed run does not erase
// the last usable document Wiki; this method removes rows absent from the new
// successful generation immediately before the dataset completion event.
func (s *PipelineExecutor) reconcileDocumentCompiledProducts(ctx context.Context, oldIDs []string, chunks []map[string]any) error {
	docEngine := engine.Get()
	if docEngine == nil || s == nil || s.taskCtx == nil {
		return nil
	}
	newIDs := make(map[string]struct{})
	for _, chunk := range chunks {
		if strings.TrimSpace(asCompiledKwd(chunk)) == "" {
			continue
		}
		if id := strings.TrimSpace(anyString(chunk["id"])); id != "" {
			newIDs[id] = struct{}{}
		}
	}
	staleIDs := make([]string, 0, len(oldIDs))
	for _, id := range oldIDs {
		if _, keep := newIDs[id]; !keep {
			staleIDs = append(staleIDs, id)
		}
	}
	const pageSize = 1000
	indexName := fmt.Sprintf("ragflow_%s", s.taskCtx.Tenant.ID)
	for start := 0; start < len(staleIDs); start += pageSize {
		end := min(start+pageSize, len(staleIDs))
		if _, err := docEngine.DeleteChunks(ctx, map[string]any{
			"id":    staleIDs[start:end],
			"kb_id": s.taskCtx.Doc.KbID,
		}, indexName, s.taskCtx.Doc.KbID); err != nil {
			return fmt.Errorf("delete stale document compiler products: %w", err)
		}
	}
	return nil
}

// applyDocumentAvailability stamps ordinary source chunks with available_int=0
// when Document.status is "0" (disabled). Disabling before any chunks exist only
// updates MySQL; without this, later parsing would still write searchable
// available_int=1 rows. Compiled products (compile_kwd) stay at 0 regardless.
func applyDocumentAvailability(chunks []map[string]any, status *string) {
	if status == nil || *status != "0" {
		return
	}
	for _, ck := range chunks {
		if _, ok := ck["compile_kwd"]; ok {
			continue
		}
		ck["available_int"] = 0
	}
}

// compiledVariants returns the sorted, de-duplicated set of compile types a
// document's compiled products carry. It reads the authoritative
// `compilation_template_kind_kwd` the KnowledgeCompiler component stamps on each
// compiled product (the resolved template's kind) and maps it through
// common.KindToVariant (O2a whitelist: unknown kinds are skipped). Products
// without an authoritative kind fall back to their `compile_kwd`-derived variant.
// This surfaces the compiler's runtime variant inference to PublishCompleted so
// the consumer can route the dataset-level re-compile per compile type.
func compiledVariants(chunks []map[string]any) []string {
	seen := map[string]struct{}{}
	for _, ck := range chunks {
		if _, ok := ck["compile_kwd"]; !ok {
			continue
		}
		var v kccommon.Variant
		if kind, ok := ck["compilation_template_kind_kwd"].(string); ok && kind != "" {
			mapped, err := kccommon.KindToVariant(kind)
			if err != nil {
				continue // unknown template kind (O2a): skip, do not mis-route
			}
			v = mapped
		} else {
			// Fallback on compile_kwd via the shared knowledge_compile mapping
			// (which folds the inferred structure kinds list/set/hypergraph into
			// VariantStructure before the KindToVariant whitelist lookup).
			mapped, err := knowledge_compile.KwdToVariant(asCompiledKwd(ck))
			if err != nil {
				continue
			}
			v = mapped
		}
		if _, dup := seen[string(v)]; dup {
			continue
		}
		seen[string(v)] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeCompiledVariants returns the union of the previous and current
// document-level compiler variants. A reparse that removes a compiler template
// still needs to notify the dataset consumer so it can retract stale merged
// products; a document that never had compiler products produces an empty set
// and does not wake the dataset consumer at all.
func mergeCompiledVariants(previous, current []string) []string {
	seen := make(map[string]struct{}, len(previous)+len(current))
	for _, variants := range [][]string{previous, current} {
		for _, variant := range variants {
			variant = strings.TrimSpace(variant)
			if variant != "" {
				seen[variant] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	merged := make([]string, 0, len(seen))
	for variant := range seen {
		merged = append(merged, variant)
	}
	sort.Strings(merged)
	return merged
}

// asCompiledKwd extracts the compile_kwd keyword value from a chunk map,
// tolerating both a bare string and a list-wrapped keyword column from the
// engine. It returns "" when absent.
func asCompiledKwd(c map[string]any) string {
	switch v := c["compile_kwd"].(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func wikiActiveStates(output map[string]any) ([]kccommon.WikiMapActiveState, error) {
	if output == nil || output["wiki_active_map_states"] == nil {
		return nil, nil
	}
	appendState := func(states []kccommon.WikiMapActiveState, value map[string]any) ([]kccommon.WikiMapActiveState, error) {
		state := kccommon.WikiMapActiveState{
			Key:        strings.TrimSpace(anyString(value["key"])),
			TenantID:   strings.TrimSpace(anyString(value["tenant_id"])),
			DatasetID:  strings.TrimSpace(anyString(value["dataset_id"])),
			DocumentID: strings.TrimSpace(anyString(value["document_id"])),
		}
		switch payload := value["payload"].(type) {
		case string:
			state.Payload = []byte(payload)
		case []byte:
			state.Payload = append([]byte(nil), payload...)
		default:
			return nil, fmt.Errorf("decode Wiki active MAP state: unsupported payload type %T", value["payload"])
		}
		if state.Key == "" || state.TenantID == "" || state.DatasetID == "" || state.DocumentID == "" {
			return nil, fmt.Errorf("decode Wiki active MAP state: incomplete scope")
		}
		return append(states, state), nil
	}

	switch values := output["wiki_active_map_states"].(type) {
	case []kccommon.WikiMapActiveState:
		return append([]kccommon.WikiMapActiveState(nil), values...), nil
	case []map[string]any:
		states := make([]kccommon.WikiMapActiveState, 0, len(values))
		for _, value := range values {
			var err error
			states, err = appendState(states, value)
			if err != nil {
				return nil, err
			}
		}
		return states, nil
	case []any:
		states := make([]kccommon.WikiMapActiveState, 0, len(values))
		for _, raw := range values {
			value, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("decode Wiki active MAP state: unsupported item type %T", raw)
			}
			var err error
			states, err = appendState(states, value)
			if err != nil {
				return nil, err
			}
		}
		return states, nil
	default:
		return nil, fmt.Errorf("decode Wiki active MAP states: unsupported type %T", output["wiki_active_map_states"])
	}
}

// PipelineLogInput contains the identifiers and optional snapshots needed to
// persist a pipeline operation log without constructing a PipelineExecutor.
type PipelineLogInput struct {
	TenantID   string
	KbID       string
	DocumentID string
	PipelineID string
	DSL        string
	Status     string
	Document   entity.Document
}

// RecordPipelineLog persists a pipeline operation log without requiring
// executor setup. Callers that already know a terminal state should pass it in
// Status; otherwise the writer falls back to the latest document.run value.
func RecordPipelineLog(ctx context.Context, db *gorm.DB, input PipelineLogInput) error {
	return recordPipelineLog(ctx, db, input, dao.NewPipelineOperationLogDAO().Create)
}

func recordPipelineLog(
	ctx context.Context,
	db *gorm.DB,
	input PipelineLogInput,
	createFunc func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error,
) error {
	var dslMap entity.JSONMap
	if strings.TrimSpace(input.DSL) == "" {
		dslMap = entity.JSONMap{}
	} else if err := json.Unmarshal([]byte(input.DSL), &dslMap); err != nil {
		dslMap = entity.JSONMap{"raw": input.DSL}
	}

	// The task context contains the document snapshot loaded when the task
	// started. Reload it here so the operation log reflects the final progress
	// state written by the progress sink, matching the Python operation-log
	// creation path.
	doc := input.Document
	if doc.ID == "" {
		doc.ID = input.DocumentID
		doc.KbID = input.KbID
	}
	if db != nil {
		if persisted, err := dao.NewDocumentDAO().GetByID(ctx, db, input.DocumentID); err == nil && persisted != nil {
			doc = *persisted
		} else if err != nil {
			common.Warn(fmt.Sprintf("failed to reload document %s for pipeline log: %v", input.DocumentID, err))
		}
	}
	if input.KbID == "" {
		input.KbID = doc.KbID
	}
	if input.PipelineID == "" && doc.PipelineID != nil {
		input.PipelineID = strings.TrimSpace(*doc.PipelineID)
	}
	if input.TenantID == "" && db != nil && input.KbID != "" {
		if kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, input.KbID); err == nil && kb != nil {
			input.TenantID = kb.TenantID
		} else if err != nil {
			return fmt.Errorf("load knowledgebase %s for pipeline log: %w", input.KbID, err)
		}
	}

	// Pipeline identity for the log row. A document without a user pipeline
	// selection runs on a builtin registry pipeline: its canvasID is the
	// parser_id, not a canvas row, so the log is titled with the document's
	// parser_id, reuses the document thumbnail as avatar, and leaves
	// pipeline_id empty.
	pipelineTitle := doc.ParserID
	pipelineAvatar := doc.Thumbnail
	var pipelineID *string
	if input.PipelineID != "" {
		pipelineID = &input.PipelineID
		if db != nil && strings.TrimSpace(input.DSL) != "" {
			if canvas, err := dao.NewUserCanvasDAO().GetByID(ctx, db, input.PipelineID); err == nil && canvas != nil {
				if canvas.Title != nil {
					pipelineTitle = *canvas.Title
				}
				pipelineAvatar = canvas.Avatar
			} else if err != nil && !errors.Is(err, dao.ErrUserCanvasNotFound) {
				common.Warn(fmt.Sprintf("failed to reload pipeline %s for operation log: %v", input.PipelineID, err))
			}
		}
	}

	operationStatus := input.Status
	if operationStatus == "" && doc.Run != nil && *doc.Run != "" {
		operationStatus = *doc.Run
	}
	statusValue := "1"
	if doc.Status != nil && *doc.Status != "" {
		statusValue = *doc.Status
	}
	sourceFrom := doc.SourceType
	if parts := strings.SplitN(sourceFrom, "/", 2); len(parts) > 0 {
		sourceFrom = parts[0]
	}
	documentName := ""
	if doc.Name != nil {
		documentName = *doc.Name
	}
	log := &entity.PipelineOperationLog{
		ID:              utility.GenerateUUID(),
		TenantID:        input.TenantID,
		KbID:            input.KbID,
		DocumentID:      input.DocumentID,
		PipelineID:      pipelineID,
		PipelineTitle:   &pipelineTitle,
		TaskType:        string(entity.PipelineTaskTypeParse),
		DSL:             dslMap,
		ParserID:        doc.ParserID,
		DocumentName:    documentName,
		DocumentSuffix:  doc.Suffix,
		DocumentType:    doc.Type,
		SourceFrom:      sourceFrom,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessBeginAt:  doc.ProcessBeginAt,
		ProcessDuration: doc.ProcessDuration,
		OperationStatus: operationStatus,
		Avatar:          pipelineAvatar,
		Status:          &statusValue,
	}
	return createFunc(ctx, db, log)
}

func (s *PipelineExecutor) recordPipelineLog(ctx context.Context, db *gorm.DB, docID, dsl, status string) {
	pipelineID := ""
	if s.taskCtx.PipelineID != "" {
		pipelineID = s.canvasID
	}
	if err := recordPipelineLog(ctx, db, PipelineLogInput{
		TenantID:   s.Tenant().ID,
		KbID:       s.KB().ID,
		DocumentID: docID,
		PipelineID: pipelineID,
		DSL:        dsl,
		Status:     status,
		Document:   s.taskCtx.Doc,
	}, s.logCreateFunc); err != nil {
		common.Warn(fmt.Sprintf("failed to record pipeline log: %v", err))
	}
}

func (s *PipelineExecutor) loadDSLFromCanvas(ctx context.Context, canvasID string) (string, string, error) {
	if s == nil || s.taskCtx == nil {
		return "", "", fmt.Errorf("pipeline executor: nil task context")
	}
	if canvasID == "" {
		return "", "", fmt.Errorf("pipeline executor: empty canvas id")
	}
	canvas, err := dao.NewUserCanvasDAO().GetByID(ctx, dao.DB, canvasID)
	if err != nil {
		return "", "", fmt.Errorf("load canvas %s: %w", canvasID, err)
	}

	canvasTitle := ""
	if canvas.Title != nil {
		canvasTitle = *canvas.Title
	}
	common.Info(fmt.Sprintf("load canvas %s, name %s", canvasID, canvasTitle))

	raw, err := json.Marshal(canvas.DSL)
	if err != nil {
		return "", "", fmt.Errorf("marshal canvas dsl %s: %w", canvasID, err)
	}
	return string(raw), canvasID, nil
}

// warnUnknownComponentParams logs a warning for any component id in the
// parserConfig whose id is absent from the pipeline DSL. The runtime merge
// (component params -> override_params) silently drops such entries, so we
// surface them here for operability. API-side validation
// already rejects unknown ids on write; this is purely a defensive guard
// for legacy/stale rows.
func warnUnknownComponentParams(dsl string, parserConfig map[string]any) {
	if len(parserConfig) == 0 {
		return
	}
	// dsl arrives as the canvas ENVELOPE ({ "dsl": { "components": ... } }) in
	// production, so it must be unwrapped before ExtractAllComponentParams
	// runs (that helper expects the inner DSL). The previous direct call
	// passed the enveloped DSL, whose "components" key is nested under "dsl",
	// so it silently returned an error and made this guard a no-op.
	inner, err := pipelinepkg.UnwrapCanvasDSL([]byte(dsl))
	if err != nil {
		common.Warn(fmt.Sprintf("warnUnknownComponentParams: cannot parse DSL to validate component params: %v", err))
		return
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		common.Warn(fmt.Sprintf("warnUnknownComponentParams: cannot re-encode DSL: %v", err))
		return
	}
	schemas, err := pipelinepkg.ExtractAllComponentParams(innerJSON)
	if err != nil {
		common.Warn(fmt.Sprintf("warnUnknownComponentParams: cannot parse DSL to validate component params: %v", err))
		return
	}
	dslCPNs := make(map[string]struct{}, len(schemas))
	for _, s := range schemas {
		dslCPNs[s.CpnID] = struct{}{}
	}
	for cpnID := range parserConfig {
		if _, ok := dslCPNs[cpnID]; !ok {
			common.Warn(fmt.Sprintf(
				"parser_config references cpnID %q not present in the pipeline DSL; it will be ignored at runtime", cpnID))
		}
	}
}

func (s *PipelineExecutor) runPipelineWithDSL(ctx context.Context, dsl string) (map[string]any, string, error) {
	if s == nil || s.taskCtx == nil {
		return nil, dsl, fmt.Errorf("pipeline executor: nil task context")
	}

	parserConfig := map[string]interface{}(s.taskCtx.Doc.ParserConfig)
	if parserConfig == nil {
		// Debug (dataflow dry-run) contexts intentionally carry no
		// ParserConfig; start from an empty map so the debug page cap can be
		// injected in place below without a nil-map assignment panic.
		parserConfig = map[string]interface{}{}
	}

	// Surface component params whose cpnID is absent from the DSL. The
	// runtime merge (override_params) silently drops such entries;
	// API-side validation already rejects unknown ids on write, so this is a
	// defensive guard for legacy/stale rows.
	warnUnknownComponentParams(dsl, parserConfig)

	pipelineID := "pipeline_" + s.taskCtx.Doc.ID
	if s.taskCtx.IngestionTask != nil && s.taskCtx.IngestionTask.ID != "" {
		pipelineID = s.taskCtx.IngestionTask.ID
	}
	pipe, err := pipelinepkg.NewPipelineFromDSL([]byte(dsl), pipelineID,
		pipelinepkg.WithProgressSink(s.progressSink),
		pipelinepkg.WithDocumentID(s.taskCtx.Doc.ID))
	if err != nil {
		return nil, dsl, fmt.Errorf("compile pipeline dsl: %w", err)
	}
	inputs := map[string]any{}
	if s.taskCtx.Doc.ID != "" {
		inputs["doc_id"] = s.taskCtx.Doc.ID
	}
	// Run-level metadata shared by both persist and debug (dataflow
	// dry-run) runs. In debug the KB is absent (NewDebugTaskContext forces
	// KB.ID == ""); in persist it is the document's own KB. Either way the
	// Tokenizer reads kb_id from CanvasState.Globals.
	inputs["tenant_id"] = s.taskCtx.Tenant.ID
	inputs["kb_id"] = s.taskCtx.KB.ID
	if s.taskCtx.KB.Language != nil {
		inputs["lang"] = *s.taskCtx.KB.Language
	}

	// File delivery and doc metadata differ between the two run modes.
	debug := s.taskCtx.IsDebug()
	if debug {
		// A debug (dry-run) run has no DB document row, so the parser
		// cannot resolve its bytes via doc_id → storage. Deliver the
		// uploaded bytes directly as `binary` (what the parser actually
		// reads) and surface the doc name/type so family detection works.
		if s.taskCtx.File != nil {
			inputs["file"] = s.taskCtx.File
			inputs["binary"] = s.taskCtx.File
		}
		if s.taskCtx.Doc.Name != nil && *s.taskCtx.Doc.Name != "" {
			inputs["name"] = *s.taskCtx.Doc.Name
		}
		if s.taskCtx.Doc.Type != "" {
			inputs["file_type"] = s.taskCtx.Doc.Type
		}
		// A debug (dry-run) run with a chunker node keeps only the leading
		// N chunks for preview. The cap is delivered through pipeline inputs
		// (seeded into CanvasState.Globals by the pipeline run, read by the
		// chunker decorator via globals.DebugChunkCap) — the same run-level
		// channel as the other shared metadata, not override_params (the
		// decorator is built at compile time and cannot read run-time
		// override_params). An explicit caller-supplied cap is respected.
		inputs = injectDebugChunkCap(inputs)
	} else {
		if s.taskCtx.File != nil {
			inputs["file"] = s.taskCtx.File
		}
	}

	// A canvas-debug (dataflow dry-run) must return a fast preview, so it
	// caps the parser to the first few pages. The cap is delivered through
	// override_params (Run's 3rd argument) — the SAME channel the
	// production ParserConfig uses — keyed by the Parser component's cpnID
	// and the document's filetype family. It is NOT passed through pipeline
	// inputs: the parser selects pages from ParserConfig[cpnID][family]
	// ["pages"] (a list of 1-indexed inclusive ranges), exactly mirroring
	// NormalizeParserConfigPages / pdf_pages_test.go. The DSL/parser-family
	// knowledge now lives in pipeline.BuildParserPageCapOverride.
	if debug {
		parserConfig = pipelinepkg.BuildParserPageCapOverride(
			parserConfig, []byte(dsl), s.taskCtx.Doc.Type,
			debugPageCapPages, component.ComponentNameParser, component.ParserFileFamily)
	}

	// Component params from Doc.ParserConfig — including the tenant LLM id
	// injected into Extractor components above — are passed to Run as
	// override_params, keyed by cpnID with override-wins. The DSL itself is
	// compiled unchanged.
	output, err := pipe.Run(ctx, inputs, parserConfig)
	if err != nil {
		return nil, dsl, err
	}

	logDSL := s.buildLogDSL(dsl, output)

	payload, err := pipelinepkg.ExtractPayload(dsl, output)
	if err != nil {
		return nil, dsl, err
	}
	return payload, logDSL, nil
}

// buildLogDSL returns the DSL string recorded for a pipeline run: the
// run-result DSL (static canvas structure + each component's runtime outputs
// merged into obj.params.outputs) when it can be built and marshaled,
// otherwise the static dsl unchanged — log recording must never fail a run.
//
// BuildDebugResultDSL needs the full run output, which is in scope only inside
// runPipelineWithDSL: the extracted payload returned there no longer carries
// output["state"]. Two consumers:
//   - canvas-debug runs hand the map to the DebugLogSink END marker via the
//     ResultSink capability (END-marker `dsl` attachment,
//     rag/flow/pipeline.py:98);
//   - persist (dataset parse) runs return its JSON as the log DSL so the
//     pipeline operation log carries every component's output
//     (dsl=str(pipeline), rag/svr/task_executor_refactor/dataflow_service.py)
//     — without it the dataset log "View result" page renders blank panels.
//
// The sink probe stays an optional capability: non-debug (DB-backed) sinks
// ignore it and the ProgressSink contract is unchanged.
func (s *PipelineExecutor) buildLogDSL(dsl string, output map[string]any) string {
	logDSL := dsl
	if resultDSL, e := BuildDebugResultDSL(dsl, output); e == nil {
		if rs, ok := s.progressSink.(ResultSink); ok {
			rs.SetResult(resultDSL, output)
		}
		if raw, e := json.Marshal(resultDSL); e == nil {
			logDSL = string(raw)
		} else {
			common.Warn(fmt.Sprintf("marshal run-result dsl for pipeline log: %v", e))
		}
	} else {
		common.Warn(fmt.Sprintf("build run-result dsl for pipeline log: %v", e))
	}
	return logDSL
}

// debugPageCapPages is the number of leading pages a canvas-debug
// (dataflow dry-run) parses. The debug preview must return fast, so we cap
// the parser to the first few pages. The cap is expressed as the 1-indexed
// inclusive range [1, debugPageCapPages], matching the production
// ParserConfig[cpnID][filetype]["pages"] shape (see NormalizeParserConfigPages).
const debugPageCapPages = 2

// injectDebugChunkCap sets the canvas-debug chunk cap on the run inputs when
// not already present. The cap is read by the chunker decorator (via
// CanvasState.Globals / globals.DebugChunkCap) and limits a debug (dry-run)
// preview to the leading N chunks. An existing value (a future caller-supplied
// override) is respected, mirroring BuildParserPageCapOverride's respect for an
// explicit page cap. A nil inputs map is initialized, so callers may pass a
// concrete or nil map.
func injectDebugChunkCap(inputs map[string]any) map[string]any {
	if inputs == nil {
		inputs = map[string]any{}
	}
	if _, ok := inputs[globals.DebugChunkCapKey]; !ok {
		inputs[globals.DebugChunkCapKey] = DebugChunkCapDefault
	}
	return inputs
}
