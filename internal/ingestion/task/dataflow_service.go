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
	componentpkg "ragflow/internal/ingestion/component"
	"ragflow/internal/utility"
	"regexp"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"
)

type embedder struct {
	model *models.EmbeddingModel
}

func (e *embedder) Encode(texts []string) ([][]float64, error) {
	config := &models.EmbeddingConfig{Dimension: 0}
	embeds, err := e.model.ModelDriver.Embed(e.model.ModelName, texts, e.model.APIConfig, config)
	if err != nil {
		return nil, err
	}
	vecs := make([][]float64, len(embeds))
	for i, v := range embeds {
		vecs[i] = v.Embedding
	}
	return vecs, nil
}

// init sets the package-level EncodeFunc once, avoiding the per-task
// save/restore pattern that previously caused data races when multiple
// workers ran pipelines concurrently (see defaultRunPipeline).
func init() {
	componentpkg.EncodeFunc = func(tenantID, embdID string) componentpkg.Embedder {
		model, err := service.NewModelProviderService().GetEmbeddingModel(tenantID, embdID)
		if err != nil {
			return nil
		}
		return &embedder{model: model}
	}
}

type ProgressFunc func(prog float64, msg string)

type docService interface {
	UpdateDocument(id string, req *service.UpdateDocumentRequest) error
	GetDocumentMetadataByID(docID string) (map[string]any, error)
	SetDocumentMetadata(docID string, meta map[string]any) error
}

type chunkCounter interface {
	IncrementChunkNum(docID, kbID string, chunkNum, tokenConsumption int, duration float64) error
}

type defaultDocService struct{}
type defaultChunkCounter struct{}

func (d *defaultDocService) UpdateDocument(id string, req *service.UpdateDocumentRequest) error {
	return service.NewDocumentService().UpdateDocument(id, req)
}

func (d *defaultDocService) GetDocumentMetadataByID(docID string) (map[string]any, error) {
	return service.NewDocumentService().GetDocumentMetadataByID(docID)
}

func (d *defaultDocService) SetDocumentMetadata(docID string, meta map[string]any) error {
	return service.NewDocumentService().SetDocumentMetadata(docID, meta)
}

func (d *defaultChunkCounter) IncrementChunkNum(docID, kbID string, chunkNum, tokenConsumption int, duration float64) error {
	return service.NewDocumentService().IncrementChunkNum(docID, kbID, chunkNum, tokenConsumption, duration)
}

func encodeTexts(model *models.EmbeddingModel, texts []string) ([][]float64, int, error) {
	texts = TruncateTexts(texts, model.MaxTokens)
	config := &models.EmbeddingConfig{Dimension: 0}
	embeds, err := model.ModelDriver.Embed(model.ModelName, texts, model.APIConfig, config)
	if err != nil {
		return nil, 0, err
	}
	vecs := make([][]float64, len(embeds))
	totalTokens := 0
	for i, v := range embeds {
		vecs[i] = v.Embedding
		totalTokens += v.TokenCount
	}
	return vecs, totalTokens, nil
}

type PipelineExecutor struct {
	taskCtx            *TaskContext
	dataflowID         string
	embeddingBatchSize int
	docBulkSize        int
	progressFunc       ProgressFunc

	docSvc                docService
	chunkCounter          chunkCounter
	insertChunksFunc      func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error)
	logCreateFunc         func(log *entity.PipelineOperationLog) error
	getEmbeddingModelFunc func(tenantID, embdID string) (*models.EmbeddingModel, error)
	loadDSLFunc           func(ctx context.Context, dataflowID string) (string, string, error)
	runPipelineFunc       func(ctx context.Context, dsl string) (map[string]any, string, error)
}

func validateDataflowTaskContext(taskCtx *TaskContext) error {
	if taskCtx == nil {
		return fmt.Errorf("dataflow service: nil task context")
	}
	if taskCtx.Doc.ID == "" {
		return fmt.Errorf("dataflow service: empty document id")
	}
	if taskCtx.Doc.KbID == "" {
		return fmt.Errorf("dataflow service: empty document knowledgebase id")
	}
	if taskCtx.Doc.Name == nil || *taskCtx.Doc.Name == "" {
		return fmt.Errorf("dataflow service: empty document name")
	}
	if taskCtx.KB.ID == "" {
		return fmt.Errorf("dataflow service: empty knowledgebase id")
	}
	if taskCtx.KB.EmbdID == "" {
		return fmt.Errorf("dataflow service: empty embedding model id")
	}
	if taskCtx.Tenant.ID == "" {
		return fmt.Errorf("dataflow service: empty tenant id")
	}
	return nil
}

func NewDataflowService(
	taskCtx *TaskContext,
	dataflowID string,
	embeddingBatchSize int,
	docBulkSize int,
) (*PipelineExecutor, error) {
	if err := validateDataflowTaskContext(taskCtx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(dataflowID) == "" {
		return nil, fmt.Errorf("dataflow service: empty dataflow id")
	}
	progressFn := func(prog float64, msg string) {}
	if taskCtx != nil && taskCtx.ProgressFunc != nil {
		progressFn = taskCtx.ProgressFunc
	}
	svc := &PipelineExecutor{
		taskCtx:            taskCtx,
		dataflowID:         dataflowID,
		embeddingBatchSize: embeddingBatchSize,
		docBulkSize:        docBulkSize,
		progressFunc:       progressFn,
		docSvc:             &defaultDocService{},
		chunkCounter:       &defaultChunkCounter{},
		insertChunksFunc: func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error) {
			return engine.Get().InsertChunks(ctx, chunks, baseName, datasetID)
		},
		logCreateFunc:         dao.NewPipelineOperationLogDAO().Create,
		getEmbeddingModelFunc: service.NewModelProviderService().GetEmbeddingModel,
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

func (s *PipelineExecutor) WithGetEmbeddingModelFunc(f func(tenantID, embdID string) (*models.EmbeddingModel, error)) *PipelineExecutor {
	s.getEmbeddingModelFunc = f
	return s
}

func (s *PipelineExecutor) WithDocService(d docService) *PipelineExecutor {
	s.docSvc = d
	return s
}

func (s *PipelineExecutor) WithChunkCounter(c chunkCounter) *PipelineExecutor {
	s.chunkCounter = c
	return s
}

func (s *PipelineExecutor) WithLoadDSLFunc(f func(ctx context.Context, dataflowID string) (string, string, error)) *PipelineExecutor {
	s.loadDSLFunc = f
	return s
}

func (s *PipelineExecutor) WithRunPipelineFunc(f func(ctx context.Context, dsl string) (map[string]any, string, error)) *PipelineExecutor {
	s.runPipelineFunc = f
	return s
}

func (s *PipelineExecutor) KB() *entity.Knowledgebase { return &s.taskCtx.KB }
func (s *PipelineExecutor) Doc() *entity.Document     { return &s.taskCtx.Doc }
func (s *PipelineExecutor) Tenant() *entity.Tenant    { return &s.taskCtx.Tenant }

func (s *PipelineExecutor) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dsl, correctedID, err := s.loadDSLFunc(ctx, s.dataflowID)
	if err != nil {
		return err
	}
	if correctedID != "" {
		s.dataflowID = correctedID
	}

	pipelineOutput, pipelineDSL, err := s.runPipelineFunc(ctx, dsl)
	if err != nil {
		return err
	}

	if s.taskCtx.Doc.ID == CANVAS_DEBUG_DOC_ID {
		s.recordPipelineLog(s.taskCtx.Doc.ID, pipelineDSL, "done")
		return nil
	}

	if err := s.RunDataflow(ctx, pipelineOutput); err != nil {
		return err
	}

	if pipelineDSL != "" {
		s.recordPipelineLog(s.taskCtx.Doc.ID, pipelineDSL, "done")
	}
	return nil
}

func (s *PipelineExecutor) RunDataflow(ctx context.Context, pipelineOutput map[string]any) error {
	taskStart := time.Now()
	if pipelineOutput == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	chunks := s.normalizeChunks(pipelineOutput)
	if chunks == nil {
		return nil
	}

	embeddingTokenConsumption := GetEmbeddingTokenConsumption(pipelineOutput)

	metadata := s.processChunks(chunks)
	if err := s.prepareChunkAssets(chunks); err != nil {
		return err
	}

	if len(metadata) > 0 {
		if err := s.updateDocumentMetadata(s.taskCtx.Doc.ID, metadata); err != nil {
			common.Warn(fmt.Sprintf("failed to update document metadata: %v", err))
		}
	}

	// Generate embeddings before indexing — this was missing in the initial
	// Go migration and caused all chunks to be inserted without vector
	// embeddings, making semantic/vector search completely inoperable.
	chunks, embeddingTokenConsumption, err := s.embedChunks(ctx, chunks, embeddingTokenConsumption)
	if err != nil {
		return err
	}

	indexStart := time.Now()
	s.progress(0.82, "[DOC Engine]:\nStart to index...")
	if err := s.insertChunks(ctx, chunks); err != nil {
		return err
	}

	if err := s.incrementChunkNum(s.taskCtx.Doc.ID, s.taskCtx.Doc.KbID, len(chunks), embeddingTokenConsumption, 0); err != nil {
		common.Warn(fmt.Sprintf("failed to increment chunk num: %v", err))
	}
	indexDuration := time.Since(indexStart).Seconds()
	taskDuration := time.Since(taskStart).Seconds()
	s.progress(1.0, fmt.Sprintf("Indexing done (%.2fs). Task done (%.2fs)", indexDuration, taskDuration))

	return nil
}

func (s *PipelineExecutor) normalizeChunks(output map[string]any) []map[string]any {
	return NormalizeChunks(output)
}

func (s *PipelineExecutor) embedChunks(ctx context.Context, chunks []map[string]any, tokenConsumption int) ([]map[string]any, int, error) {
	if len(chunks) == 0 {
		return nil, 0, nil
	}
	s.progress(0.82, "\n-------------------------------------\nStart to embedding...")
	model, err := s.getEmbeddingModel(s.taskCtx.Tenant.ID, s.taskCtx.KB.EmbdID)
	if err != nil {
		s.progress(-1, fmt.Sprintf("[ERROR]: %v", err))
		return nil, tokenConsumption, err
	}
	texts := PrepareTextsForDataflowEmbedding(chunks)
	batchSize := s.embeddingBatchSize
	if batchSize <= 0 {
		batchSize = 16
	}
	delta := 0.20 / float64(len(texts)/batchSize+1)
	prog := 0.8
	var allVects [][]float64
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		if lim := s.taskCtx.EmbedLimiter; lim != nil {
			if err := lim.Acquire(ctx, 1); err != nil {
				s.progress(-1, fmt.Sprintf("[ERROR]: %v", err))
				return nil, tokenConsumption, err
			}
		}
		vecs, tc, err := encodeTexts(model, batch)
		if err != nil {
			if lim := s.taskCtx.EmbedLimiter; lim != nil {
				lim.Release(1)
			}
			s.progress(-1, fmt.Sprintf("[ERROR]: %v", err))
			return nil, tokenConsumption, err
		}
		if lim := s.taskCtx.EmbedLimiter; lim != nil {
			lim.Release(1)
		}
		allVects = append(allVects, vecs...)
		tokenConsumption += tc
		prog += delta
		s.progress(prog, fmt.Sprintf("%d / %d", i+1, len(texts)/batchSize))
	}
	if len(allVects) != len(chunks) {
		panic(fmt.Sprintf("vector count mismatch: %d vs %d", len(allVects), len(chunks)))
	}
	AttachVectors(chunks, allVects)
	return chunks, tokenConsumption, nil
}

func (s *PipelineExecutor) processChunks(chunks []map[string]any) map[string]any {
	return ProcessChunksForDataflow(
		chunks,
		s.taskCtx.Doc.ID,
		s.taskCtx.Doc.KbID,
		*s.taskCtx.Doc.Name,
		time.Now(),
	)
}

func (s *PipelineExecutor) prepareChunkAssets(chunks []map[string]any) error {
	return PrepareDataflowChunkAssets(chunks)
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

func (s *PipelineExecutor) updateDocumentMetadata(docID string, metadata map[string]any) error {
	if len(metadata) == 0 {
		return nil
	}
	existing, err := s.docSvc.GetDocumentMetadataByID(docID)
	if err != nil {
		existing = make(map[string]any)
	}
	for k, v := range metadata {
		if _, exists := existing[k]; !exists {
			existing[k] = v
		}
	}
	return s.docSvc.SetDocumentMetadata(docID, existing)
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
		PipelineID:      &s.dataflowID,
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

func (s *PipelineExecutor) incrementChunkNum(docID, kbID string, chunkNum, tokenConsumption int, duration float64) error {
	if s.chunkCounter == nil {
		return fmt.Errorf("dataflow service: chunk counter is nil")
	}
	return s.chunkCounter.IncrementChunkNum(docID, kbID, chunkNum, tokenConsumption, duration)
}

func (s *PipelineExecutor) progress(prog float64, msg string) {
	if s.progressFunc != nil {
		s.progressFunc(prog, msg)
	}
}

func (s *PipelineExecutor) getEmbeddingModel(tenantID, embdID string) (*models.EmbeddingModel, error) {
	return s.getEmbeddingModelFunc(tenantID, embdID)
}

func hasVectors(chunks []map[string]any) bool {
	for _, ck := range chunks {
		for k := range ck {
			if matchQVec.MatchString(k) {
				return true
			}
		}
	}
	return false
}

var matchQVec = regexp.MustCompile(`^q_\d+_vec$`)

func (s *PipelineExecutor) defaultLoadDSL(ctx context.Context, dataflowID string) (string, string, error) {
	if s == nil || s.taskCtx == nil {
		return "", "", fmt.Errorf("dataflow service: nil task context")
	}
	if dataflowID == "" {
		return "", "", fmt.Errorf("dataflow service: empty dataflow id")
	}
	if strings.HasPrefix(s.taskCtx.TaskType, "dataflow") {
		canvas, err := dao.NewUserCanvasDAO().GetByID(dataflowID)
		if err != nil {
			return "", "", fmt.Errorf("load dataflow canvas %s: %w", dataflowID, err)
		}
		raw, err := json.Marshal(canvas.DSL)
		if err != nil {
			return "", "", fmt.Errorf("marshal canvas dsl %s: %w", dataflowID, err)
		}
		return string(raw), dataflowID, nil
	}
	var pipelineLog entity.PipelineOperationLog
	if err := dao.DB.Where("id = ?", dataflowID).First(&pipelineLog).Error; err != nil {
		return "", "", fmt.Errorf("load pipeline log %s: %w", dataflowID, err)
	}
	raw, err := json.Marshal(pipelineLog.DSL)
	if err != nil {
		return "", "", fmt.Errorf("marshal pipeline log dsl %s: %w", dataflowID, err)
	}
	correctedID := dataflowID
	if pipelineLog.PipelineID != nil && *pipelineLog.PipelineID != "" {
		correctedID = *pipelineLog.PipelineID
	}
	return string(raw), correctedID, nil
}

func (s *PipelineExecutor) defaultRunPipeline(ctx context.Context, dsl string) (map[string]any, string, error) {
	if s == nil || s.taskCtx == nil {
		return nil, dsl, fmt.Errorf("dataflow service: nil task context")
	}

	// Use doc ID as pipeline ID if available, otherwise a placeholder
	pipelineID := "pipeline_" + s.taskCtx.Doc.ID
	if s.taskCtx.IngestionTask != nil && s.taskCtx.IngestionTask.ID != "" {
		pipelineID = s.taskCtx.IngestionTask.ID
	}
	pipe, err := pipelinepkg.NewPipelineFromDSL([]byte(dsl), pipelineID)
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
	inputs["model_id"] = s.taskCtx.KB.EmbdID

	output, err := pipe.Run(ctx, inputs)
	if err != nil {
		return nil, dsl, err
	}
	payload, err := extractDataflowPipelinePayload(dsl, output)
	if err != nil {
		return nil, dsl, err
	}
	return payload, dsl, nil
}

func extractDataflowPipelinePayload(dsl string, out map[string]any) (map[string]any, error) {
	if out == nil {
		return nil, nil
	}
	if _, ok := out["output_format"]; ok {
		return out, nil
	}
	terminalIDs, err := terminalComponentIDsFromDSL([]byte(dsl))
	if err != nil {
		return nil, err
	}
	if len(terminalIDs) != 1 {
		return nil, fmt.Errorf("dataflow pipeline requires exactly 1 terminal, got %d: %v", len(terminalIDs), terminalIDs)
	}
	payload, ok := out[terminalIDs[0]].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("run output missing terminal payload %q", terminalIDs[0])
	}
	return payload, nil
}

func terminalComponentIDsFromDSL(raw []byte) ([]string, error) {
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return nil, fmt.Errorf("unmarshal dataflow dsl: %w", err)
	}
	root := tpl
	if nested, ok := tpl["dsl"].(map[string]any); ok {
		root = nested
	}
	components, ok := root["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dataflow dsl missing components map")
	}
	terminals := make([]string, 0, len(components))
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("component %q has invalid type %T", id, rawComp)
		}
		switch downstream := comp["downstream"].(type) {
		case nil:
			terminals = append(terminals, id)
		case []any:
			if len(downstream) == 0 {
				terminals = append(terminals, id)
			}
		default:
			// Non-slice downstream means the component is connected; ignore it here.
		}
	}
	sort.Strings(terminals)
	return terminals, nil
}
