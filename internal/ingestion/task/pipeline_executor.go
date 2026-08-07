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
	"fmt"
	"ragflow/internal/utility"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
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

	dsl, correctedID, err := s.loadDSLFunc(ctx, s.canvasID)
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
		s.recordPipelineLog(ctx, dao.DB, s.taskCtx.Doc.ID, pipelineDSL, "done")
	}

	return result, nil
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
		ChunkCount:       countDistinctChunkIDs(chunks),
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
	// Ordinary source chunks stay available_int=1 (the index default).
	markCompiledProductsHidden(chunks)

	if err := s.indexWriter.Write(ctx, chunks); err != nil {
		return nil, err
	}

	// All chunks are now persisted. Notify the dataset-level post-processing consumer
	// (§11) that this document is complete: its compiled products were written
	// available_int=0 and the consumer later merges them into dataset-level products
	// (available_int=1). The notification is sent only after a successful persist
	// and is best-effort / non-fatal — a delivery failure is logged but does not
	// fail the pipeline task.
	if err := knowledge_compile.PublishCompleted(ctx, s.taskCtx.Tenant.ID, s.taskCtx.Doc.KbID, s.taskCtx.Doc.ID, 0); err != nil {
		common.Logger.Warn(fmt.Sprintf("knowledge_compile: publish doc_completed for %s failed: %v", s.taskCtx.Doc.ID, err))
	}

	chunkCount := countDistinctChunkIDs(chunks)

	return &PipelineResult{
		DocID:            s.taskCtx.Doc.ID,
		KbID:             s.taskCtx.Doc.KbID,
		Metadata:         metadata,
		ChunkCount:       chunkCount,
		TokenConsumption: embeddingTokenConsumption,
		Duration:         time.Since(start).Seconds(),
	}, nil
}

// countDistinctChunkIDs returns the number of distinct chunk IDs in the slice.
// After ProcessChunksForPipeline, every chunk carries an "id" field computed
// from xxhash(text+docID). Chunks with identical text share the same id and
// the search engine treats them as upserts, so the effective stored chunk count
// is the number of unique ids — not len(chunks). Mirrors the index-side
// deduplication that happens at write time.
func countDistinctChunkIDs(chunks []map[string]any) int {
	seen := make(map[string]struct{}, len(chunks))
	for _, ck := range chunks {
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

func (s *PipelineExecutor) recordPipelineLog(ctx context.Context, db *gorm.DB, docID, dsl, status string) {
	var dslMap entity.JSONMap
	if err := json.Unmarshal([]byte(dsl), &dslMap); err != nil {
		dslMap = entity.JSONMap{"raw": dsl}
	}
	log := &entity.PipelineOperationLog{
		ID:              utility.GenerateUUID(),
		TenantID:        s.Tenant().ID,
		KbID:            s.KB().ID,
		DocumentID:      docID,
		PipelineID:      &s.canvasID,
		TaskType:        string(entity.PipelineTaskTypeParse),
		DSL:             dslMap,
		ParserID:        s.taskCtx.Doc.ParserID,
		DocumentName:    *s.Doc().Name,
		DocumentSuffix:  s.taskCtx.Doc.Suffix,
		DocumentType:    s.taskCtx.Doc.Type,
		SourceFrom:      s.taskCtx.Doc.SourceType,
		OperationStatus: status,
	}
	if err := s.logCreateFunc(ctx, db, log); err != nil {
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

	// Surface the debug-run result DSL to any sink that implements ResultSink
	// (the DebugLogSink used by canvas-debug runs). This mirrors Python's
	// END-marker `dsl` attachment (rag/flow/pipeline.py:98) so the front-end
	// "View result" page can render parsed chunks. The probe is an optional
	// capability: non-debug (DB-backed) sinks ignore it and the ProgressSink
	// contract is unchanged, keeping the coupling one-directional.
	if rs, ok := s.progressSink.(ResultSink); ok {
		if resultDSL, e := BuildDebugResultDSL(dsl, output); e == nil {
			rs.SetResult(resultDSL, output)
		}
	}

	payload, err := pipelinepkg.ExtractPayload(dsl, output)
	if err != nil {
		return nil, dsl, err
	}
	return payload, dsl, nil
}

// debugPageCapPages is the number of leading pages a canvas-debug
// (dataflow dry-run) parses. The debug preview must return fast, so we cap
// the parser to the first few pages. The cap is expressed as the 1-indexed
// inclusive range [1, debugPageCapPages], matching the production
// ParserConfig[cpnID][filetype]["pages"] shape (see NormalizeParserConfigPages).
const debugPageCapPages = 2
