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

type ProgressFunc func(prog float64, msg string)

// PipelineResult is the outcome of a pipeline run: chunks have been
// indexed, and these bookkeeping inputs remain for the caller to apply to
// document state (metadata merge + chunk/token counter bumps).
type PipelineResult struct {
	DocID            string
	KbID             string
	Metadata         map[string]any
	ChunkCount       int
	TokenConsumption int
}

type PipelineExecutor struct {
	taskCtx      *TaskContext
	canvasID     string
	docBulkSize  int
	progressFunc ProgressFunc

	insertChunksFunc func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error)
	logCreateFunc    func(log *entity.PipelineOperationLog) error
	loadDSLFunc      func(ctx context.Context, canvasID string) (string, string, error)
	runPipelineFunc  func(ctx context.Context, dsl string) (map[string]any, string, error)
	progressSink     pipelinepkg.ProgressSink
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
	progressFn := func(prog float64, msg string) {}
	if taskCtx.ProgressFunc != nil {
		progressFn = taskCtx.ProgressFunc
	}
	svc := &PipelineExecutor{
		taskCtx:      taskCtx,
		canvasID:     canvasID,
		docBulkSize:  docBulkSize,
		progressFunc: progressFn,
		insertChunksFunc: func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error) {
			return engine.Get().InsertChunks(ctx, chunks, baseName, datasetID)
		},
		logCreateFunc: dao.NewPipelineOperationLogDAO().Create,
	}
	svc.loadDSLFunc = svc.defaultLoadDSL
	svc.runPipelineFunc = svc.defaultRunPipeline
	return svc, nil
}

func (s *PipelineExecutor) WithProgressFunc(fn ProgressFunc) *PipelineExecutor {
	s.progressFunc = fn
	return s
}

func (s *PipelineExecutor) WithInsertChunksFunc(f func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error)) *PipelineExecutor {
	s.insertChunksFunc = f
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

func (s *PipelineExecutor) KB() *entity.Knowledgebase { return &s.taskCtx.KB }
func (s *PipelineExecutor) Doc() *entity.Document     { return &s.taskCtx.Doc }
func (s *PipelineExecutor) Tenant() *entity.Tenant    { return &s.taskCtx.Tenant }

func (s *PipelineExecutor) Execute(ctx context.Context) (*PipelineResult, error) {
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

	result, err := s.processOutput(ctx, pipelineOutput)
	if err != nil {
		return nil, err
	}

	if pipelineDSL != "" {
		s.recordPipelineLog(s.taskCtx.Doc.ID, pipelineDSL, "done")
	}
	return result, nil
}

func (s *PipelineExecutor) processOutput(ctx context.Context, pipelineOutput map[string]any) (*PipelineResult, error) {
	taskStart := time.Now()
	if pipelineOutput == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	chunks := s.normalizeChunks(pipelineOutput)
	if chunks == nil {
		return nil, nil
	}

	embeddingTokenConsumption := GetEmbeddingTokenConsumption(pipelineOutput)
	metadata := s.processChunks(chunks)

	indexStart := time.Now()
	s.progress(0.82, "[DOC Engine]:\nStart to index...")
	if err := s.insertChunks(ctx, chunks); err != nil {
		return nil, err
	}
	indexDuration := time.Since(indexStart).Seconds()
	taskDuration := time.Since(taskStart).Seconds()
	s.progress(1.0, fmt.Sprintf("Indexing done (%.2fs). Task done (%.2fs)", indexDuration, taskDuration))

	return &PipelineResult{
		DocID:            s.taskCtx.Doc.ID,
		KbID:             s.taskCtx.Doc.KbID,
		Metadata:         metadata,
		ChunkCount:       len(chunks),
		TokenConsumption: embeddingTokenConsumption,
	}, nil
}

func (s *PipelineExecutor) normalizeChunks(output map[string]any) []map[string]any {
	return NormalizeChunks(output)
}

func (s *PipelineExecutor) processChunks(chunks []map[string]any) map[string]any {
	return ProcessChunksForPipeline(
		chunks,
		s.taskCtx.Doc.ID,
		s.taskCtx.Doc.KbID,
		*s.taskCtx.Doc.Name,
		time.Now(),
	)
}

func (s *PipelineExecutor) insertChunks(ctx context.Context, chunks []map[string]any) error {
	baseName := fmt.Sprintf("ragflow_%s", s.taskCtx.Tenant.ID)
	if len(chunks) == 0 {
		_, err := s.insertChunksFunc(ctx, chunks, baseName, s.taskCtx.Doc.KbID)
		return err
	}
	bulkSize := s.docBulkSize
	if bulkSize <= 0 {
		bulkSize = len(chunks)
	}
	for b := 0; b < len(chunks); b += bulkSize {
		end := b + bulkSize
		if end > len(chunks) {
			end = len(chunks)
		}
		if _, err := s.insertChunksFunc(ctx, chunks[b:end], baseName, s.taskCtx.Doc.KbID); err != nil {
			return err
		}
		if (b/bulkSize)%128 == 0 {
			s.progress(0.8+0.1*float64(b+1)/float64(len(chunks)), "")
		}
	}
	return nil
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

func (s *PipelineExecutor) progress(prog float64, msg string) {
	if s.progressFunc != nil {
		s.progressFunc(prog, msg)
	}
}
