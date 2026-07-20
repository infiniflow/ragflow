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
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// PipelineResult is the outcome of a pipeline run: chunks have been
// indexed, and these bookkeeping inputs remain for the caller to apply to
// document state (metadata merge + chunk/token counter bumps).
type PipelineResult struct {
	DocID            string
	KbID             string
	Metadata         map[string]any
	ChunkCount       int
	TokenConsumption int
	Duration         float64 // pipeline wall-clock seconds
}

type PipelineExecutor struct {
	taskCtx     *TaskContext
	canvasID    string
	docBulkSize int

	indexWriter     *chunkIndexWriter
	logCreateFunc   func(log *entity.PipelineOperationLog) error
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
	if taskCtx.Doc.KbID == "" {
		return fmt.Errorf("pipeline executor: empty document knowledgebase id")
	}
	if taskCtx.Doc.Name == nil || *taskCtx.Doc.Name == "" {
		return fmt.Errorf("pipeline executor: empty document name")
	}
	if taskCtx.KB.ID == "" {
		return fmt.Errorf("pipeline executor: empty knowledgebase id")
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

func (s *PipelineExecutor) WithLogCreateFunc(f func(log *entity.PipelineOperationLog) error) *PipelineExecutor {
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

	if s.taskCtx.Doc.ID == CANVAS_DEBUG_DOC_ID {
		s.recordPipelineLog(s.taskCtx.Doc.ID, pipelineDSL, "done")
		return nil, nil
	}

	result, err := s.processOutput(ctx, pipelineOutput, start)
	if err != nil {
		return nil, err
	}

	if pipelineDSL != "" {
		s.recordPipelineLog(s.taskCtx.Doc.ID, pipelineDSL, "done")
	}
	return result, nil
}

func (s *PipelineExecutor) processOutput(ctx context.Context, pipelineOutput map[string]any, start time.Time) (*PipelineResult, error) {
	if pipelineOutput == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	chunks := NormalizeChunks(pipelineOutput)
	if len(chunks) == 0 {
		return nil, nil
	}

	embeddingTokenConsumption := GetEmbeddingTokenConsumption(pipelineOutput)
	metadata, err := ProcessChunksForPipeline(
		chunks,
		s.taskCtx.Doc.ID,
		s.taskCtx.Doc.KbID,
		*s.taskCtx.Doc.Name,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	tableMeta := AggregateTableDocMetadata(chunks, map[string]interface{}(s.taskCtx.Doc.ParserConfig))
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

	if err := s.indexWriter.Write(ctx, chunks); err != nil {
		return nil, err
	}

	return &PipelineResult{
		DocID:            s.taskCtx.Doc.ID,
		KbID:             s.taskCtx.Doc.KbID,
		Metadata:         metadata,
		ChunkCount:       len(chunks),
		TokenConsumption: embeddingTokenConsumption,
		Duration:         time.Since(start).Seconds(),
	}, nil
}

func (s *PipelineExecutor) recordPipelineLog(docID, dsl, status string) {
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
	if err := s.logCreateFunc(log); err != nil {
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
	canvas, err := dao.NewUserCanvasDAO().GetByID(canvasID)
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
	schemas, err := pipelinepkg.ExtractAllComponentParams([]byte(dsl))
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
	common.InjectExtractorLLMID(parserConfig, s.taskCtx.Tenant.LLMID)

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
	if s.taskCtx.File != nil {
		inputs["file"] = s.taskCtx.File
	}
	inputs["tenant_id"] = s.taskCtx.Tenant.ID
	inputs["kb_id"] = s.taskCtx.KB.ID

	// Component params from Doc.ParserConfig — including the tenant LLM id
	// injected into Extractor components above — are passed to Run as
	// override_params, keyed by cpnID with override-wins. The DSL itself is
	// compiled unchanged.
	output, err := pipe.Run(ctx, inputs, parserConfig)
	if err != nil {
		return nil, dsl, err
	}
	payload, err := pipelinepkg.ExtractPayload(dsl, output)
	if err != nil {
		return nil, dsl, err
	}
	return payload, dsl, nil
}
