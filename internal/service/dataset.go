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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	redisengine "ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/server"
	"ragflow/internal/service/nlp"
	"ragflow/internal/utility"
	"sort"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	datasetAllowedChunkMethods = map[string]struct{}{
		"naive":        {},
		"book":         {},
		"email":        {},
		"laws":         {},
		"manual":       {},
		"one":          {},
		"paper":        {},
		"picture":      {},
		"presentation": {},
		"qa":           {},
		"resume":       {},
		"table":        {},
		"tag":          {},
	}
	datasetSupportedAvatarMIMETypes = map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
	}
	datasetAllowedOrderByFields = map[string]struct{}{
		"create_time": {},
		"update_time": {},
	}
	datasetAllowedMetadataTypes = map[string]struct{}{
		"string": {},
		"list":   {},
		"time":   {},
		"number": {},
	}
	datasetChunkMethodErrorMessage = "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'resume', 'table' or 'tag'"
	validIndexTypes                = []string{"graph", "raptor", "mindmap"}
	indexTypeToTaskType            = map[string]string{"graph": "graphrag", "raptor": "raptor", "mindmap": "mindmap"}
	indexTypeToDisplayName         = map[string]string{"graph": "Graph", "raptor": "RAPTOR", "mindmap": "Mindmap"}
)

const (
	// Keep the legacy worker marker in queue payloads; persisted tasks use a real document ID.
	graphRaptorQueueDocID = "graph_raptor_x"
	maximumTaskPageNumber = int64(100000000)
	serverQueueNamePrefix = "te"
)

// DatasetService implements the RESTful dataset APIs from dataset_api.py.
type DatasetService struct {
	kbDAO          *dao.KnowledgebaseDAO
	documentDAO    *dao.DocumentDAO
	connectorDAO   *dao.ConnectorDAO
	tenantDAO      *dao.TenantDAO
	tenantLLMDAO   *dao.TenantLLMDAO
	pipelineLogDAO *dao.PipelineOperationLogDAO
	userTenantDAO  *dao.UserTenantDAO
	taskDAO        *dao.TaskDAO
	searchService  *SearchService
	docEngine      engine.DocEngine
	embeddingCache *utility.EmbeddingLRU
	engineType     server.EngineType
}

// NewDatasetService creates a new datasets service.
func NewDatasetService() *DatasetService {
	cfg := server.GetConfig()
	engineType := server.EngineType("")
	if cfg != nil {
		engineType = cfg.DocEngine.Type
	}
	return &DatasetService{
		kbDAO:          dao.NewKnowledgebaseDAO(),
		documentDAO:    dao.NewDocumentDAO(),
		connectorDAO:   dao.NewConnectorDAO(),
		tenantDAO:      dao.NewTenantDAO(),
		tenantLLMDAO:   dao.NewTenantLLMDAO(),
		pipelineLogDAO: dao.NewPipelineOperationLogDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		taskDAO:        dao.NewTaskDAO(),
		searchService:  NewSearchService(),
		docEngine:      engine.Get(),
		embeddingCache: utility.NewEmbeddingLRU(1000),
		engineType:     engineType,
	}
}

func (s *DatasetService) UpdateDocumentMetadataConfig(userID, datasetID, documentID string, req map[string]interface{}) (*entity.Document, common.ErrorCode, error) {
	if _, err := s.kbDAO.GetByIDAndTenantID(datasetID, userID); err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("You don't own the dataset.")
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("Document %s not found in dataset %s", documentID, datasetID)
		}
		return nil, common.CodeServerError, err
	}

	metadata, ok := req["metadata"]
	if !ok {
		return nil, common.CodeArgumentError, errors.New("metadata is required")
	}

	parserConfig := doc.ParserConfig
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}
	parserConfig["metadata"] = metadata

	if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"parser_config": parserConfig}); err != nil {
		return nil, common.CodeExceptionError, err
	}

	updatedDoc, err := s.documentDAO.GetByID(doc.ID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Document not found!")
		}
		return nil, common.CodeExceptionError, err
	}

	return updatedDoc, common.CodeSuccess, nil
}

func checkType(indexType string) bool {
	haveType := false
	for _, t := range validIndexTypes {
		if indexType == t {
			haveType = true
		}
	}
	return haveType
}

func (s *DatasetService) newRaptorOrGraphRagTask(sampleDoc *entity.Document, taskType string, taskDocID string, queueDocID string, docIDs []string) (*entity.Task, map[string]interface{}, error) {
	if docIDs == nil || len(docIDs) == 0 {
		docIDs = make([]string, 0)
	}
	if !checkIndexTaskType(taskType) {
		return nil, nil, errors.New("type should be graphrag, raptor or mindmap")
	}

	chunkingConfig, err := s.documentDAO.GetChunkingConfig(sampleDoc.ID)
	if err != nil {
		return nil, nil, err
	}

	hasher := xxhash.New()
	keys := make([]string, 0, len(chunkingConfig))
	for key := range chunkingConfig {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		_, _ = hasher.Write([]byte(key))
		_, _ = hasher.Write([]byte{0})
		v, mErr := json.Marshal(chunkingConfig[key])
		if mErr != nil {
			return nil, nil, mErr
		}
		_, _ = hasher.Write(v)
		_, _ = hasher.Write([]byte{0})
	}

	taskID := strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
	beginAt := time.Now().Truncate(time.Second)
	progressMsg := beginAt.Format("15:04:05") + " created task " + taskType

	for _, field := range []interface{}{taskDocID, maximumTaskPageNumber, maximumTaskPageNumber, taskType} {
		_, _ = hasher.Write([]byte(fmt.Sprint(field)))
	}
	digest := fmt.Sprintf("%016x", hasher.Sum64())
	task := &entity.Task{
		ID:          taskID,
		DocID:       taskDocID,
		FromPage:    maximumTaskPageNumber,
		ToPage:      maximumTaskPageNumber,
		TaskType:    taskType,
		ProgressMsg: &progressMsg,
		BeginAt:     &beginAt,
		Digest:      &digest,
	}

	queueMessage := map[string]interface{}{
		"id":           taskID,
		"doc_id":       queueDocID,
		"from_page":    maximumTaskPageNumber,
		"to_page":      maximumTaskPageNumber,
		"task_type":    taskType,
		"progress_msg": progressMsg,
		"begin_at":     beginAt.Format("2006-01-02 15:04:05"),
		"digest":       digest,
		"doc_ids":      docIDs,
	}

	return task, queueMessage, nil
}

func createDatasetIndexTaskInTx(tx *gorm.DB, task *entity.Task, queueDocID string) (*entity.Document, error) {
	if task == nil {
		return nil, errors.New("task is required")
	}
	if err := tx.Create(task).Error; err != nil {
		return nil, err
	}

	if queueDocID == "" {
		return nil, nil
	}

	var document entity.Document
	err := tx.Select("id", "progress_msg", "process_begin_at").Where("id = ?", queueDocID).First(&document).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	beginAt := time.Now().Truncate(time.Second)
	if task.BeginAt != nil {
		beginAt = *task.BeginAt
	}
	if err := tx.Model(&entity.Document{}).Where("id = ?", queueDocID).Updates(map[string]interface{}{
		"progress_msg":     "Task is queued...",
		"process_begin_at": beginAt,
	}).Error; err != nil {
		return nil, err
	}

	return &document, nil
}

func enqueueDatasetIndexTask(priority int, queueMessage map[string]interface{}) error {
	redisClient := redisengine.Get()
	if redisClient == nil || !redisClient.QueueProduct(datasetIndexQueueName(priority), queueMessage) {
		return errors.New("Can't access Redis. Please check the Redis' status")
	}
	return nil
}

func cleanupFailedDatasetIndexTask(taskID string, updatedDocument *entity.Document, kbID string, indexType string) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("id = ?", taskID).Delete(&entity.Task{}).Error; err != nil {
			return fmt.Errorf("delete task %s: %w", taskID, err)
		}

		if column := datasetIndexTaskIDColumn(indexType); kbID != "" && column != "" {
			if err := tx.Model(&entity.Knowledgebase{}).Where("id = ? AND "+column+" = ?", kbID, taskID).Update(column, nil).Error; err != nil {
				return fmt.Errorf("clear dataset task id %s: %w", taskID, err)
			}
		}

		if updatedDocument == nil {
			return nil
		}

		return tx.Model(&entity.Document{}).Where("id = ?", updatedDocument.ID).Updates(map[string]interface{}{
			"progress_msg":     updatedDocument.ProgressMsg,
			"process_begin_at": updatedDocument.ProcessBeginAt,
		}).Error
	})
}

func datasetIndexTaskIDColumn(indexType string) string {
	switch indexType {
	case "graph":
		return "graphrag_task_id"
	case "raptor":
		return "raptor_task_id"
	case "mindmap":
		return "mindmap_task_id"
	default:
		return ""
	}
}

func checkIndexTaskType(taskType string) bool {
	switch taskType {
	case "graphrag", "raptor", "mindmap":
		return true
	default:
		return false
	}
}

func datasetIndexTaskID(kb *entity.Knowledgebase, indexType string) string {
	if kb == nil {
		return ""
	}
	switch indexType {
	case "graph":
		if kb.GraphragTaskID != nil {
			return *kb.GraphragTaskID
		}
	case "raptor":
		if kb.RaptorTaskID != nil {
			return *kb.RaptorTaskID
		}
	case "mindmap":
		if kb.MindmapTaskID != nil {
			return *kb.MindmapTaskID
		}
	}
	return ""
}

func datasetIndexTaskIDUpdate(indexType, taskID string) map[string]interface{} {
	switch indexType {
	case "graph":
		return map[string]interface{}{"graphrag_task_id": taskID}
	case "raptor":
		return map[string]interface{}{"raptor_task_id": taskID}
	case "mindmap":
		return map[string]interface{}{"mindmap_task_id": taskID}
	default:
		return map[string]interface{}{}
	}
}

func datasetIndexTaskIDs(kb *entity.Knowledgebase) []string {
	if kb == nil {
		return nil
	}

	taskIDs := make([]string, 0, 3)
	for _, taskID := range []*string{kb.GraphragTaskID, kb.RaptorTaskID, kb.MindmapTaskID} {
		if taskID != nil && *taskID != "" {
			taskIDs = append(taskIDs, *taskID)
		}
	}
	return common.Deduplicate(taskIDs)
}

func datasetIndexQueueName(priority int) string {
	return fmt.Sprintf("%s.%d.common", serverQueueNamePrefix, priority)
}

// RunIndex Run an indexing task (graph/raptor/mindmap) for a dataset.
func (s *DatasetService) RunIndex(userID, datasetID, indexType string) (map[string]interface{}, common.ErrorCode, error) {
	if !checkType(indexType) {
		return nil, common.CodeDataError, fmt.Errorf("Invalid index type '%s'. Must be one of %v", indexType, validIndexTypes)
	}

	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
		}
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	taskType := indexTypeToTaskType[indexType]
	displayName := indexTypeToDisplayName[indexType]

	documents, code, err := s.getDocumentsByDatasetForIndex(datasetID)
	if err != nil {
		return nil, code, err
	}
	_ = documents

	sampleDocument := documents[0]
	documentIDs := make([]string, len(documents))

	for i, doc := range documents {
		documentIDs[i] = doc.ID
	}

	task, queueMessage, err := s.newRaptorOrGraphRagTask(sampleDocument, taskType, sampleDocument.ID, graphRaptorQueueDocID, documentIDs)
	if err != nil {
		common.Warn("Failed to build dataset index task", zap.String("dataset_id", datasetID), zap.String("task_type", taskType), zap.Error(err))
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	var updatedDocument *entity.Document
	var dataErr error
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		var lockedKB entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", kb.ID, string(entity.StatusValid)).
			First(&lockedKB).Error; err != nil {
			return err
		}

		existingTaskID := datasetIndexTaskID(&lockedKB, indexType)
		if existingTaskID != "" {
			var existingTask entity.Task
			taskErr := tx.Where("id = ?", existingTaskID).First(&existingTask).Error
			if taskErr != nil {
				if errors.Is(taskErr, gorm.ErrRecordNotFound) {
				} else {
					return taskErr
				}
			} else if existingTask.Progress != 1 && existingTask.Progress != -1 {
				dataErr = fmt.Errorf("Task %s in progress with status %v. A %s Task is already running.", existingTaskID, existingTask.Progress, displayName)
				return dataErr
			}
		}

		updatedDocument, err = createDatasetIndexTaskInTx(tx, task, graphRaptorQueueDocID)
		if err != nil {
			return err
		}
		return tx.Model(&entity.Knowledgebase{}).Where("id = ?", lockedKB.ID).Updates(datasetIndexTaskIDUpdate(indexType, task.ID)).Error
	})
	if err != nil {
		if dataErr != nil {
			return nil, common.CodeDataError, dataErr
		}
		common.Warn("Failed to create dataset index task", zap.String("dataset_id", datasetID), zap.String("task_type", taskType), zap.Error(err))
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	if err := enqueueDatasetIndexTask(0, queueMessage); err != nil {
		if cleanupErr := cleanupFailedDatasetIndexTask(task.ID, updatedDocument, kb.ID, indexType); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		common.Warn("Failed to queue dataset index task", zap.String("dataset_id", datasetID), zap.String("task_type", taskType), zap.Error(err))
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	return map[string]interface{}{"task_id": task.ID}, common.CodeSuccess, nil
}

func (s *DatasetService) getDocumentsByDatasetForIndex(datasetID string) ([]*entity.Document, common.ErrorCode, error) {
	documents, _, err := s.documentDAO.GetByKBID(datasetID)
	if err != nil {
		common.Warn("Failed to load dataset documents for index", zap.String("dataset_id", datasetID), zap.Error(err))
		return nil, common.CodeDataError, errors.New("Internal server error")
	}
	if len(documents) == 0 {
		return nil, common.CodeDataError, fmt.Errorf("No documents in Dataset %s", datasetID)
	}
	return documents, common.CodeSuccess, nil
}

type TraceIndexRequest struct {
	Type string `json:"type" binding:"required"`
}

// TraceIndex Trace an indexing task (graph/raptor/mindmap) for a dataset.
func (s *DatasetService) TraceIndex(datasetID, userID, indexType string) (*entity.Task, common.ErrorCode, error) {
	if !checkType(indexType) {
		return nil, common.CodeDataError, fmt.Errorf("Invalid index type '%s'. Must be one of %v", indexType, validIndexTypes)
	}

	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
		}
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	taskID := datasetIndexTaskID(kb, indexType)

	var task *entity.Task
	if taskID != "" {
		task, err = s.taskDAO.GetByID(taskID)
		if err != nil {
			if dao.IsNotFoundErr(err) {
				return nil, common.CodeSuccess, nil
			}
			return nil, common.CodeServerError, errors.New("Internal server error")
		}
		if task == nil {
			return nil, common.CodeSuccess, nil
		}
	}

	return task, common.CodeSuccess, nil
}

// SearchDatasetsRequest is the request structure for searching chunks across datasets.
type SearchDatasetsRequest struct {
	DatasetIDs             []string               `json:"dataset_ids" binding:"required"`
	Question               string                 `json:"question" binding:"required"`
	Page                   *int                   `json:"page,omitempty"`
	Size                   *int                   `json:"size,omitempty"`
	DocIDs                 []string               `json:"doc_ids,omitempty"`
	UseKG                  *bool                  `json:"use_kg,omitempty"`
	TopK                   *int                   `json:"top_k,omitempty"`
	CrossLanguages         []string               `json:"cross_languages,omitempty"`
	SearchID               *string                `json:"search_id,omitempty"`
	MetadataFilter         map[string]interface{} `json:"meta_data_filter,omitempty"`
	RerankID               *string                `json:"rerank_id,omitempty"`
	Keyword                *bool                  `json:"keyword,omitempty"`
	SimilarityThreshold    *float64               `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight,omitempty"`
}

// SearchDatasetsResponse is the response structure for dataset search results.
type SearchDatasetsResponse struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs"`
	Labels  *map[string]float64      `json:"labels"`
	Total   int64                    `json:"total"`
}

// SearchDatasets searches chunks across one or more knowledge bases based on a question.
// It retrieves relevant chunks using embedding and optional reranking, applying filters,
// cross-language translation, and keyword extraction as configured.
func (s *DatasetService) SearchDatasets(req *SearchDatasetsRequest, userID string) (*SearchDatasetsResponse, error) {
	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if len(req.DatasetIDs) == 0 {
		return nil, fmt.Errorf("dataset_ids is required")
	}
	common.Info("SearchDatasets started", zap.String("userID", userID), zap.Any("datasets", req.DatasetIDs), zap.String("question", req.Question))

	page := 1
	if req.Page != nil {
		page = *req.Page
	}
	pageSize := 30
	if req.Size != nil {
		pageSize = *req.Size
	}
	useKG := false
	if req.UseKG != nil {
		useKG = *req.UseKG
	}
	similarityThreshold := 0.0
	if req.SimilarityThreshold != nil {
		similarityThreshold = *req.SimilarityThreshold
	}
	vectorSimilarityWeight := 0.3
	if req.VectorSimilarityWeight != nil {
		vectorSimilarityWeight = *req.VectorSimilarityWeight
	}
	topK := 1024
	if req.TopK != nil {
		topK = *req.TopK
	}
	if topK < 1 {
		topK = 1
	} else if topK > 2048 {
		topK = 2048
	}
	keyword := false
	if req.Keyword != nil {
		keyword = *req.Keyword
	}
	searchID := ""
	if req.SearchID != nil {
		searchID = *req.SearchID
	}
	rerankID := ""
	if req.RerankID != nil {
		rerankID = *req.RerankID
	}

	question := req.Question
	datasetIDs := req.DatasetIDs
	metadataFilter := req.MetadataFilter
	crossLanguages := req.CrossLanguages

	common.Debug(fmt.Sprintf("SearchDatasets request:\n"+
		"    datasetIDs=%v\n"+
		"    question=%s\n"+
		"    page=%v, pageSize=%v\n"+
		"    docIDs=%v\n"+
		"    useKG=%v, topK=%v\n"+
		"    crossLanguages=%v\n"+
		"    searchID=%v\n"+
		"    metadataFilter=%v\n"+
		"    rerankID=%v\n"+
		"    keyword=%v\n"+
		"    similarityThreshold=%v, vectorSimilarityWeight=%v",
		datasetIDs, req.Question,
		common.PtrString(req.Page), common.PtrString(req.Size), req.DocIDs,
		useKG, topK, crossLanguages, searchID,
		metadataFilter,
		rerankID,
		keyword,
		similarityThreshold, vectorSimilarityWeight))

	ctx := context.Background()
	modelProviderSvc := NewModelProviderService()

	// Access check for all datasets
	var tenantIDs []string
	var kbRecords []*entity.Knowledgebase
	seenTenants := make(map[string]bool)
	for _, datasetID := range datasetIDs {
		if !s.kbDAO.Accessible(datasetID, userID) {
			common.Warn("SearchDatasets access denied", zap.String("datasetID", datasetID), zap.String("userID", userID))
			return nil, fmt.Errorf("only owner of dataset %s is authorized for this operation", datasetID)
		}

		kb, err := s.kbDAO.GetByID(datasetID)
		if err != nil || kb == nil {
			common.Warn("SearchDatasets dataset not found", zap.String("datasetID", datasetID))
			return nil, fmt.Errorf("dataset %s not found", datasetID)
		}
		if !seenTenants[kb.TenantID] {
			seenTenants[kb.TenantID] = true
			tenantIDs = append(tenantIDs, kb.TenantID)
		}
		kbRecords = append(kbRecords, kb)
	}

	// Check if all kbs have the same embedding model
	if len(kbRecords) > 1 {
		firstEmbdID := kbRecords[0].EmbdID
		for i := 1; i < len(kbRecords); i++ {
			if kbRecords[i].EmbdID != firstEmbdID {
				return nil, fmt.Errorf("cannot retrieve across datasets with different embedding models")
			}
		}
	}

	// Override request fields with values from saved search config (if search_id is provided)
	var chatID string
	if searchID != "" {
		searchDetail, err := s.searchService.GetDetail(searchID)
		if err != nil {
			common.Warn("Failed to get search detail for search_id, proceeding without it", zap.String("searchID", searchID), zap.Error(err))
		} else if searchConfig, ok := searchDetail["search_config"].(map[string]interface{}); ok && searchConfig != nil {
			if scMetadataFilter, ok := searchConfig["meta_data_filter"].(map[string]interface{}); ok {
				metadataFilter = scMetadataFilter
			}
			if scST, ok := searchConfig["similarity_threshold"].(float64); ok {
				similarityThreshold = scST
			}
			if scVSW, ok := searchConfig["vector_similarity_weight"].(float64); ok {
				vectorSimilarityWeight = scVSW
			}
			if scTopK, ok := searchConfig["top_k"].(float64); ok {
				topK = int(scTopK)
				if topK < 1 {
					topK = 1
				} else if topK > 2048 {
					topK = 2048
				}
			}
			if scUseKG, ok := searchConfig["use_kg"].(bool); ok {
				useKG = scUseKG
			}
			if scLangs, ok := searchConfig["cross_languages"].([]interface{}); ok {
				crossLanguages = make([]string, len(scLangs))
				for i, l := range scLangs {
					if s, ok := l.(string); ok {
						crossLanguages[i] = s
					}
				}
			}
			if scKeyword, ok := searchConfig["keyword"].(bool); ok {
				keyword = scKeyword
			}
			if scRerankID, ok := searchConfig["rerank_id"].(string); ok {
				rerankID = scRerankID
			}
			chatID, _ = searchConfig["chat_id"].(string)

			common.Debug("SearchDatasets loaded Search config",
				zap.String("searchID", searchID),
				zap.Strings("datasetIDs", datasetIDs),
				zap.Float64("vectorSimilarityWeight", vectorSimilarityWeight),
				zap.Float64("fullTextWeight", 1-vectorSimilarityWeight),
				zap.Float64("similarityThreshold", similarityThreshold),
				zap.Int("topK", topK),
				zap.Strings("crossLanguages", crossLanguages),
				zap.Bool("keyword", keyword),
				zap.String("rerankID", rerankID),
				zap.String("chatID", chatID),
				zap.Bool("useKG", useKG))
		} else {
			common.Warn("No search_config found in search detail", zap.String("searchID", searchID))
		}
	}

	// If meta_data_filter method is auto/semi_auto, get chat model
	var err error
	var chatModelForFilter *models.ChatModel
	if metadataFilter != nil {
		method, _ := metadataFilter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			if chatID != "" {
				driver, modelName, apiConfig, _, err := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeChat, chatID)
				if err != nil {
					common.Warn("Failed to get chat model config from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(err))
				} else {
					chatModelForFilter = models.NewChatModel(driver, &modelName, apiConfig)
					common.Info("Fetched chat model (from search_config) for metadata filter",
						zap.String("chatID", chatID),
						zap.String("tenantID", tenantIDs[0]))
				}
			}

			if chatModelForFilter == nil {
				driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(tenantIDs[0], entity.ModelTypeChat)
				if err != nil {
					common.Warn("Failed to get tenant default chat model for meta_data_filter", zap.Error(err))
				} else {
					chatModelForFilter = models.NewChatModel(driver, &modelName, apiConfig)
					common.Info("Fetched chat model (tenant default) for metadata filter",
						zap.String("tenantID", tenantIDs[0]))
				}
			}
		}
	}

	// Apply meta_data_filter to get filtered doc_ids
	docIDs := make([]string, len(req.DocIDs))
	copy(docIDs, req.DocIDs)
	if metadataFilter != nil {
		metadataSvc := NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs(datasetIDs)
		if err != nil {
			common.Warn("Failed to get flatted metadata, using empty metadata for filter", zap.Error(err))
			flattedMeta = make(common.MetaData)
		}
		common.Info("Metadata filter conditions", zap.Any("filter", metadataFilter))
		filteredDocIDs, _ := ApplyMetaDataFilter(ctx, metadataFilter, flattedMeta, question, chatModelForFilter, req.DocIDs, datasetIDs)
		docIDs = filteredDocIDs
		common.Info("ApplyMetaDataFilter result", zap.Strings("docIDs", docIDs))
	}

	// Apply cross_languages and keyword extraction
	modifiedQuestion := question
	if len(crossLanguages) > 0 {
		// Pass tenantID and empty llmID so CrossLanguages can fetch default if needed
		// This matches Python's cross_languages(tenant_id, llm_id, query, languages)
		common.Info("CrossLanguages: dispatching translation",
			zap.String("tenantID", tenantIDs[0]),
			zap.String("llmID", ""),
			zap.Strings("crossLanguages", crossLanguages))
		translated, err := CrossLanguages(ctx, tenantIDs[0], "", question, crossLanguages)
		if err != nil {
			common.Warn("Failed to translate question", zap.String("llmID", ""), zap.Error(err))
		} else {
			modifiedQuestion = translated
		}
	}
	if keyword {
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(tenantIDs[0], entity.ModelTypeChat)
		if err != nil {
			common.Warn("Failed to get default chat model for LLM transformations", zap.Error(err))
		} else {
			chatModel := models.NewChatModel(driver, &modelName, apiConfig)
			common.Info("Fetched chat model (tenant default) for keyword_extraction",
				zap.String("tenantID", tenantIDs[0]))

			extractedKeywords, err := KeywordExtraction(ctx, chatModel, modifiedQuestion, 3)
			if err != nil {
				common.Warn("Failed to extract keywords from question", zap.Error(err))
			} else if extractedKeywords != "" {
				modifiedQuestion = modifiedQuestion + extractedKeywords
			}
		}
	}
	if modifiedQuestion != question {
		common.Info("Modified question after transformations",
			zap.String("originalQuestion", question),
			zap.String("modifiedQuestion", modifiedQuestion),
			zap.Strings("crossLanguages", crossLanguages),
			zap.Bool("keywordExtraction", keyword))
	}

	// Get tag-based rank features via LabelQuestion
	metadataSvc := NewMetadataService()
	labels := metadataSvc.LabelQuestion(modifiedQuestion, kbRecords)
	if len(labels) > 0 {
		common.Debug("LabelQuestion result", zap.Any("labels", labels))
	}

	// Determine embedding model
	var embeddingModel *models.EmbeddingModel
	if kbRecords[0].EmbdID != "" {
		driver, modelName, apiConfig, maxTokens, embErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeEmbedding, kbRecords[0].EmbdID)
		if embErr != nil {
			return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", embErr)
		}
		embeddingModel = models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	} else {
		driver, modelName, apiConfig, maxTokens, err := modelProviderSvc.GetTenantDefaultModelByType(tenantIDs[0], entity.ModelTypeEmbedding)
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant default embedding model: %w", err)
		}
		embeddingModel = models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	}
	modelNameStr := ""
	if embeddingModel.ModelName != nil {
		modelNameStr = *embeddingModel.ModelName
	}
	common.Info("Fetched embedding model for retrieval",
		zap.String("tenantID", tenantIDs[0]),
		zap.String("modelName", modelNameStr))

	// Get rerank model if rerankID is specified
	var rerankModel *models.RerankModel

	if rerankID != "" {
		driver, modelName, apiConfig, _, rErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeRerank, rerankID)
		if rErr != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", rErr)
		}
		rerankModel = models.NewRerankModel(driver, &modelName, apiConfig)
	}

	if rerankModel != nil {
		common.Info("Fetched rerank model",
			zap.String("tenantID", tenantIDs[0]),
			zap.String("modelName", *rerankModel.ModelName))
	}

	retrievalReq := &nlp.RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  datasetIDs,
		DocIDs:                 docIDs,
		Page:                   page,
		PageSize:               pageSize,
		Top:                    &topK,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		RerankModel:            rerankModel,
		RankFeature:            &labels,
		EmbeddingModel:         embeddingModel,
	}

	retrievalResult, err := nlp.NewRetrievalService(s.docEngine, s.documentDAO).Retrieval(ctx, retrievalReq)
	if err != nil {
		return nil, fmt.Errorf("retrieval search failed: %w", err)
	}

	filteredChunks := retrievalResult.Chunks

	if useKG {
		common.Warn("use_kg is not yet implemented in Go - skipping KG retrieval")
	}

	filteredChunks = nlp.RetrievalByChildren(filteredChunks, tenantIDs, s.docEngine, ctx)

	for i := range filteredChunks {
		delete(filteredChunks[i], "vector")
	}

	common.Info("SearchDatasets completed", zap.String("userID", userID), zap.Any("kbID", datasetIDs), zap.String("question", question), zap.Int64("chunkCount", int64(len(filteredChunks))))

	// Convert all float64 values to PyFloat64 for Python-compatible JSON serialization
	pyChunks := common.ConvertFloatsToPyFormat(filteredChunks).([]map[string]interface{})

	return &SearchDatasetsResponse{
		Chunks:  pyChunks,
		DocAggs: retrievalResult.DocAggs,
		Labels:  &labels,
		Total:   retrievalResult.Total,
	}, nil
}

// AutoMetadataField mirrors the REST dataset auto metadata field schema.
type AutoMetadataField struct {
	Name           string      `json:"name"`
	Type           string      `json:"type"`
	Description    *string     `json:"description,omitempty"`
	Examples       interface{} `json:"examples,omitempty"`
	RestrictValues bool        `json:"restrict_values,omitempty"`
}

// AutoMetadataConfig mirrors the REST dataset auto metadata schema.
type AutoMetadataConfig struct {
	Enabled *bool               `json:"enabled,omitempty"`
	Fields  []AutoMetadataField `json:"fields,omitempty"`
}

// MetadataConfigField mirrors one field in the dataset metadata config API.
type MetadataConfigField struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Description *string  `json:"description"`
	Enum        []string `json:"enum"`
}

// MetadataConfigRequest mirrors PUT /datasets/:dataset_id/metadata/config.
type MetadataConfigRequest struct {
	Metadata        []MetadataConfigField `json:"metadata"`
	BuiltInMetadata []MetadataConfigField `json:"built_in_metadata"`
}

// CreateDatasetRequest represents the request for creating a dataset.
type CreateDatasetRequest struct {
	Name               string                 `json:"name" binding:"required"`
	Avatar             *string                `json:"avatar,omitempty"`
	Description        *string                `json:"description,omitempty"`
	EmbeddingModel     *string                `json:"embedding_model,omitempty"`
	Permission         *string                `json:"permission,omitempty"`
	ChunkMethod        *string                `json:"chunk_method,omitempty"`
	ParseType          *int                   `json:"parse_type,omitempty"`
	PipelineID         *string                `json:"pipeline_id,omitempty"`
	ParserConfig       map[string]interface{} `json:"parser_config,omitempty"`
	AutoMetadataConfig *AutoMetadataConfig    `json:"auto_metadata_config,omitempty"`
	Ext                map[string]interface{} `json:"ext,omitempty"`
}

// ListDatasets lists datasets with pagination and filtering.
func (s *DatasetService) ListDatasets(id, name string, page, pageSize int, orderby string, desc bool, keywords string, ownerIDs []string, parserID, userID string) ([]map[string]interface{}, int64, common.ErrorCode, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		normalizedID, err := normalizeDatasetUUID1(id)
		if err != nil {
			return nil, 0, common.CodeDataError, err
		}
		id = normalizedID

		kbs, err := s.kbDAO.GetKBByIDAndUserID(id, userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, 0, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, id)
		}
	}

	name = strings.TrimSpace(name)
	if name != "" {
		kbs, err := s.kbDAO.GetKBByNameAndUserID(name, userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, 0, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, name)
		}
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}

	orderby = strings.TrimSpace(orderby)
	if _, ok := datasetAllowedOrderByFields[orderby]; !ok {
		orderby = "create_time"
	}

	keywords = strings.TrimSpace(keywords)
	parserID = strings.TrimSpace(parserID)

	// Empty owner ids do not change the query, so only keep the meaningful ones.
	tenantIDs := make([]string, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID != "" {
			tenantIDs = append(tenantIDs, ownerID)
		}
	}
	if len(tenantIDs) == 0 {
		joinedTenants, err := s.tenantDAO.GetJoinedTenantsByUserID(userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		for _, joinedTenant := range joinedTenants {
			if joinedTenant == nil || joinedTenant.TenantID == "" {
				continue
			}
			tenantIDs = append(tenantIDs, joinedTenant.TenantID)
		}
	}

	kbs, total, err := s.kbDAO.GetByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords, parserID)
	if err != nil {
		return nil, 0, common.CodeServerError, errors.New("Database operation failed")
	}

	data := make([]map[string]interface{}, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		data = append(data, datasetListItemToMap(kb))
	}

	return data, total, common.CodeSuccess, nil
}

// CreateDataset creates a new dataset.
func (s *DatasetService) CreateDataset(req *CreateDatasetRequest, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	if !isValidString(req.Name) {
		return nil, common.CodeDataError, errors.New("Dataset name must be string.")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
	}
	if len(name) > entity.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), entity.DatasetNameLimit)
	}

	tenant, err := s.tenantDAO.GetByID(tenantID)
	if err != nil || tenant == nil {
		return nil, common.CodeDataError, errors.New("Tenant not found.")
	}

	parserID := ""
	permission := "me"
	embeddingModel := ""
	parserConfig := req.ParserConfig
	pipelineID := req.PipelineID
	description := req.Description
	avatar := req.Avatar
	var language *string

	if req.Description != nil && len(*req.Description) > 65535 {
		return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if req.Permission != nil {
		permission = strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
	}
	if req.ChunkMethod != nil {
		parserID = strings.TrimSpace(*req.ChunkMethod)
		if err := validateDatasetChunkMethod(parserID); err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = nil
	}
	if req.ParseType != nil && (*req.ParseType < 0 || *req.ParseType > 64) {
		return nil, common.CodeDataError, fmt.Errorf("Input should be between 0 and 64")
	}
	if req.PipelineID != nil {
		normalizedPipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = normalizedPipelineID
	}
	if req.EmbeddingModel != nil {
		embeddingModel = strings.TrimSpace(*req.EmbeddingModel)
		if err := validateDatasetEmbeddingModel(embeddingModel); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if err := validateDatasetParserConfigSize(parserConfig); err != nil {
		return nil, common.CodeDataError, err
	}

	// ext mirrors the Python REST implementation and overrides known top-level fields.
	for key, value := range req.Ext {
		switch key {
		case "name":
			nameValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Dataset name must be string.")
			}
			nameValue = strings.TrimSpace(nameValue)
			if nameValue == "" {
				return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
			}
			if len(nameValue) > entity.DatasetNameLimit {
				return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(nameValue), entity.DatasetNameLimit)
			}
			name = nameValue
		case "description":
			descriptionValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Description must be string.")
			}
			if len(descriptionValue) > 65535 {
				return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
			}
			description = &descriptionValue
		case "avatar":
			avatarValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Avatar must be string.")
			}
			if len(avatarValue) > 65535 {
				return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
			}
			if err := validateDatasetAvatar(avatarValue); err != nil {
				return nil, common.CodeDataError, err
			}
			avatar = &avatarValue
		case "language":
			languageValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Language must be string.")
			}
			languageValue = strings.TrimSpace(languageValue)
			language = &languageValue
		case "permission":
			permissionValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Permission must be string.")
			}
			permissionValue = strings.TrimSpace(permissionValue)
			if permissionValue != "me" && permissionValue != "team" {
				return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
			}
			permission = permissionValue
		case "embedding_model", "embd_id":
			embeddingModelValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Embedding model identifier must follow <model_name>@<provider> format")
			}
			embeddingModelValue = strings.TrimSpace(embeddingModelValue)
			if err := validateDatasetEmbeddingModel(embeddingModelValue); err != nil {
				return nil, common.CodeDataError, err
			}
			embeddingModel = embeddingModelValue
		case "chunk_method", "parser_id":
			parserIDValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New(datasetChunkMethodErrorMessage)
			}
			parserIDValue = strings.TrimSpace(parserIDValue)
			if err := validateDatasetChunkMethod(parserIDValue); err != nil {
				return nil, common.CodeDataError, err
			}
			parserID = parserIDValue
			pipelineID = nil
		case "pipeline_id":
			pipelineIDValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("pipeline_id must be 32 hex characters")
			}
			normalizedPipelineID, err := normalizeDatasetPipelineID(pipelineIDValue)
			if err != nil {
				return nil, common.CodeDataError, err
			}
			pipelineID = normalizedPipelineID
		case "parser_config":
			parserConfigValue, ok := value.(map[string]interface{})
			if !ok {
				return nil, common.CodeDataError, errors.New("parser_config must be valid JSON")
			}
			if err := validateDatasetParserConfigSize(parserConfigValue); err != nil {
				return nil, common.CodeDataError, err
			}
			parserConfig = parserConfigValue
		}
	}

	// parser_id wins when it is present; otherwise parse_type and pipeline_id must arrive together.
	if parserID == "" {
		if req.ParseType == nil && pipelineID == nil {
			parserID = "naive"
		} else if req.ParseType == nil || pipelineID == nil {
			missingFields := make([]string, 0, 2)
			if req.ParseType == nil {
				missingFields = append(missingFields, "parse_type")
			}
			if pipelineID == nil {
				missingFields = append(missingFields, "pipeline_id")
			}
			return nil, common.CodeDataError, fmt.Errorf("parser_id omitted -> required fields missing: %s", strings.Join(missingFields, ", "))
		}
	}

	if req.AutoMetadataConfig != nil {
		parserConfig = applyAutoMetadataConfig(parserConfig, req.AutoMetadataConfig)
	}

	parserConfig = common.GetParserConfig(parserID, parserConfig)
	parserConfig["llm_id"] = tenant.LLMID

	embdID := tenant.EmbdID
	if embeddingModel != "" {
		ok, message := s.verifyEmbeddingAvailability(embeddingModel, tenantID)
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		embdID = embeddingModel
	}

	kbID := utility.GenerateToken()

	status := string(entity.StatusValid)
	// Deduplicate name within tenant
	duplicateName, err := common.DuplicateName(func(n, tid string) bool {
		existing, err := s.kbDAO.GetByName(n, tid)
		return err == nil && existing != nil
	}, name, tenantID)
	if err != nil {
		return nil, common.CodeDataError, err
	}

	kb := &entity.Knowledgebase{
		ID:           kbID,
		Name:         duplicateName,
		TenantID:     tenantID,
		CreatedBy:    tenantID,
		ParserID:     parserID,
		PipelineID:   pipelineID,
		ParserConfig: parserConfig,
		Permission:   permission,
		EmbdID:       embdID,
		Status:       &status,
	}

	if description != nil {
		kb.Description = description
	}
	if avatar != nil {
		kb.Avatar = avatar
	}
	if language != nil {
		kb.Language = language
	}

	if err := s.kbDAO.Create(kb); err != nil {
		return nil, common.CodeServerError, errors.New("Failed to save dataset")
	}

	createdKB, err := s.kbDAO.GetByID(kbID)
	if err != nil || createdKB == nil {
		return nil, common.CodeServerError, errors.New("Dataset created failed")
	}

	return datasetToMap(createdKB), common.CodeSuccess, nil
}

// DeleteDatasets deletes multiple datasets.
func (s *DatasetService) DeleteDatasets(ids []string, deleteAll bool, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	normalizedIDs := make([]string, 0, len(ids))
	seenIDs := make(map[string]struct{}, len(ids))

	// Canonicalize ids once so every downstream DAO call sees the same UUID1 hex format.
	for _, id := range ids {
		normalizedID, err := normalizeDatasetUUID1(strings.TrimSpace(id))
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if _, exists := seenIDs[normalizedID]; exists {
			return nil, common.CodeDataError, fmt.Errorf("Duplicate ids: '%s'", normalizedID)
		}
		seenIDs[normalizedID] = struct{}{}
		normalizedIDs = append(normalizedIDs, normalizedID)
	}

	if len(normalizedIDs) == 0 {
		if !deleteAll {
			return map[string]interface{}{"success_count": 0}, common.CodeSuccess, nil
		}

		kbs, err := s.kbDAO.Query(map[string]interface{}{"tenant_id": tenantID})
		if err != nil {
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
		for _, kb := range kbs {
			normalizedIDs = append(normalizedIDs, kb.ID)
		}
	}

	kbs := make([]*entity.Knowledgebase, 0, len(normalizedIDs))
	unauthorizedIDs := make([]string, 0)
	for _, id := range normalizedIDs {
		kb, err := s.kbDAO.GetByIDAndTenantID(id, tenantID)
		if err != nil || kb == nil {
			unauthorizedIDs = append(unauthorizedIDs, id)
			continue
		}
		kbs = append(kbs, kb)
	}
	if len(unauthorizedIDs) > 0 {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for datasets: '%s'", tenantID, strings.Join(unauthorizedIDs, ", "))
	}

	errorsList := make([]string, 0)
	successCount := 0
	for _, kb := range kbs {
		if err := s.deleteDataset(tenantID, kb); err != nil {
			errorsList = append(errorsList, err.Error())
			continue
		}
		successCount++
	}

	if len(errorsList) == 0 {
		return map[string]interface{}{"success_count": successCount}, common.CodeSuccess, nil
	}

	details := strings.Join(errorsList, "; ")
	if len(details) > 128 {
		details = details[:128]
	}
	errorMessage := fmt.Sprintf(
		"Successfully deleted %d datasets, %d failed. Details: %s...",
		successCount,
		len(errorsList),
		details,
	)
	if successCount == 0 {
		return nil, common.CodeDataError, errors.New(errorMessage)
	}

	return map[string]interface{}{
		"success_count": successCount,
		"errors":        limitStrings(errorsList, 5),
	}, common.CodeSuccess, nil
}

// GetDataset gets a single dataset with its size and linked connectors.
func (s *DatasetService) GetDataset(datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}

	normalizedID, err := normalizeDatasetUUID1(datasetID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	datasetID = normalizedID

	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, datasetID)
	}

	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
	}

	data := datasetToMap(kb)

	size, err := s.documentDAO.SumSizeByDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	data["size"] = size

	connectors, err := s.connectorDAO.ListByDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	data["connectors"] = connectors

	return data, common.CodeSuccess, nil
}

type DatasetConnectorRequest struct {
	ID        string `json:"id"`
	AutoParse string `json:"auto_parse,omitempty"`
}

type UpdateDatasetRequest struct {
	Name               *string                    `json:"name,omitempty"`
	Avatar             *string                    `json:"avatar,omitempty"`
	Description        *string                    `json:"description,omitempty"`
	Language           *string                    `json:"language,omitempty"`
	Connectors         *[]DatasetConnectorRequest `json:"connectors,omitempty"`
	EmbdID             *string                    `json:"embd_id,omitempty"`
	EmbeddingModel     *string                    `json:"embedding_model,omitempty"`
	Permission         *string                    `json:"permission,omitempty"`
	ParserID           *string                    `json:"parser_id,omitempty"`
	ChunkMethod        *string                    `json:"chunk_method,omitempty"`
	Pagerank           *int64                     `json:"pagerank,omitempty"`
	ParserConfig       map[string]interface{}     `json:"parser_config,omitempty"`
	PipelineID         *string                    `json:"pipeline_id,omitempty"`
	AutoMetadataConfig *AutoMetadataConfig        `json:"auto_metadata_config,omitempty"`
	Ext                map[string]interface{}     `json:"ext,omitempty"`
}

// UpdateDataset Update a dataset
func (s *DatasetService) UpdateDataset(datasetID, tenantID string, req UpdateDatasetRequest) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Dataset not found")
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	if kb == nil || kb.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	connectorsProvided := req.Connectors != nil
	connectors := make([]DatasetConnectorRequest, 0)
	if req.Connectors != nil {
		connectors = *req.Connectors
	}

	updates := make(map[string]interface{})
	extUpdates := normalizeDatasetUpdateExt(req.Ext)

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, common.CodeDataError, errors.New("String should have at least 1 character")
		}
		if len(name) > 128 {
			return nil, common.CodeDataError, errors.New("String should have at most 128 characters")
		}
		updates["name"] = name
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return nil, common.CodeDataError, err
		}
		updates["avatar"] = *req.Avatar
	}
	if req.Description != nil {
		if len(*req.Description) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		updates["description"] = *req.Description
	}
	if req.Language != nil {
		language := strings.TrimSpace(*req.Language)
		if len(language) > 32 {
			return nil, common.CodeDataError, errors.New("String should have at most 32 characters")
		}
		updates["language"] = language
	}
	if req.Permission != nil {
		permission := strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
		updates["permission"] = permission
	}
	if req.PipelineID != nil {
		pipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if pipelineID != nil {
			updates["pipeline_id"] = *pipelineID
		}
	}

	for key, value := range extUpdates {
		if _, exists := updates[key]; !exists {
			updates[key] = value
		}
	}

	parserID, parserIDProvided, err := datasetUpdateParserID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if !parserIDProvided {
		if extParserID, ok := updates["parser_id"]; ok {
			parserIDValue, ok := extParserID.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New(datasetChunkMethodErrorMessage)
			}
			parserID = strings.TrimSpace(parserIDValue)
			if err := validateDatasetChunkMethod(parserID); err != nil {
				return nil, common.CodeDataError, err
			}
			parserIDProvided = true
		}
	}
	if parserIDProvided {
		updates["parser_id"] = parserID
	}

	embdID, embdIDProvided, err := datasetUpdateEmbeddingID(req)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if !embdIDProvided {
		if extEmbdID, ok := updates["embd_id"]; ok {
			embdIDValue, ok := extEmbdID.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Embedding model identifier must follow <model_name>@<provider> format")
			}
			embdID = strings.TrimSpace(embdIDValue)
			if embdID != "" {
				if err := validateDatasetEmbeddingModel(embdID); err != nil {
					return nil, common.CodeDataError, err
				}
			}
			embdIDProvided = true
		}
	}
	if embdIDProvided {
		if embdID == "" {
			embdID = kb.EmbdID
		}
		ok, message := s.verifyEmbeddingAvailability(embdID, tenantID)
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		updates["embd_id"] = embdID
	}

	if req.AutoMetadataConfig != nil {
		req.ParserConfig = applyAutoMetadataConfig(req.ParserConfig, req.AutoMetadataConfig)
	}
	if req.ParserConfig != nil {
		if err := validateDatasetParserConfigSize(req.ParserConfig); err != nil {
			return nil, common.CodeDataError, err
		}
		if len(req.ParserConfig) > 0 {
			parserConfig := normalizeDatasetUpdateParserConfig(req.ParserConfig)
			updates["parser_config"] = entity.JSONMap(common.DeepMergeMaps(kb.ParserConfig, parserConfig))
		}
	}

	if req.Pagerank != nil && *req.Pagerank != kb.Pagerank {
		if *req.Pagerank < 0 || *req.Pagerank > 100 {
			return nil, common.CodeDataError, errors.New("Input should be less than or equal to 100")
		}
		if s.engineType == server.EngineInfinity {
			return nil, common.CodeDataError, errors.New("'pagerank' can only be set when doc_engine is elasticsearch")
		}
		indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
		if *req.Pagerank > 0 {
			err = s.docEngine.UpdateChunks(context.Background(), map[string]interface{}{"kb_id": kb.ID}, map[string]interface{}{common.PAGERANK_FLD: *req.Pagerank}, indexName, kb.ID)
		} else {
			err = s.docEngine.UpdateChunks(context.Background(), map[string]interface{}{"exists": common.PAGERANK_FLD}, map[string]interface{}{"remove": common.PAGERANK_FLD}, indexName, kb.ID)
		}
		if err != nil {
			return nil, common.CodeServerError, err
		}
		updates["pagerank"] = *req.Pagerank
	}

	if parserIDProvided && parserID != kb.ParserID {
		if _, ok := updates["parser_config"]; !ok {
			updates["parser_config"] = entity.JSONMap(common.GetParserConfig(parserID, nil))
		}
	}
	if kb.PipelineID != nil && parserIDProvided {
		if _, ok := updates["pipeline_id"]; !ok {
			updates["pipeline_id"] = ""
		}
	}

	if nameValue, ok := updates["name"].(string); ok && strings.ToLower(nameValue) != strings.ToLower(kb.Name) {
		existing, lookupErr := s.kbDAO.GetByName(nameValue, tenantID)
		if lookupErr != nil && !dao.IsNotFoundErr(lookupErr) {
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
		if existing != nil {
			return nil, common.CodeDataError, fmt.Errorf("Dataset name '%s' already exists", nameValue)
		}
	}

	if len(updates) == 0 && !connectorsProvided {
		return nil, common.CodeDataError, errors.New("No properties were modified")
	}

	if len(updates) > 0 {
		if err = s.kbDAO.UpdateByID(kb.ID, updates); err != nil {
			return nil, common.CodeServerError, errors.New("Update dataset error.(Database error)")
		}
	}

	if connectorsProvided {
		connectorLinks := make([]dao.DatasetConnectorLink, 0, len(connectors))
		for _, connector := range connectors {
			connectorID := strings.TrimSpace(connector.ID)
			if connectorID == "" {
				return nil, common.CodeDataError, errors.New("connector id is required")
			}
			connectorLinks = append(connectorLinks, dao.DatasetConnectorLink{
				ID:        connectorID,
				AutoParse: connector.AutoParse,
			})
		}
		if err = s.connectorDAO.LinkDatasetConnectors(kb.ID, connectorLinks); err != nil {
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
	}

	updatedKB, err := s.kbDAO.GetByID(kb.ID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Dataset updated failed")
	}

	data := datasetToMap(updatedKB)
	linkedConnectors, err := s.connectorDAO.ListByDatasetID(kb.ID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	data["connectors"] = linkedConnectors
	return data, common.CodeSuccess, nil
}

func datasetUpdateParserID(req UpdateDatasetRequest) (string, bool, error) {
	parserID := ""
	provided := false
	if req.ParserID != nil {
		parserID = strings.TrimSpace(*req.ParserID)
		provided = true
	}
	if req.ChunkMethod != nil {
		parserID = strings.TrimSpace(*req.ChunkMethod)
		provided = true
	}
	if !provided {
		return "", false, nil
	}
	if err := validateDatasetChunkMethod(parserID); err != nil {
		return "", true, err
	}
	return parserID, true, nil
}

func datasetUpdateEmbeddingID(req UpdateDatasetRequest) (string, bool, error) {
	embdID := ""
	provided := false
	if req.EmbdID != nil {
		embdID = strings.TrimSpace(*req.EmbdID)
		provided = true
	}
	if req.EmbeddingModel != nil {
		embdID = strings.TrimSpace(*req.EmbeddingModel)
		provided = true
	}
	if !provided {
		return "", false, nil
	}
	if embdID != "" {
		if err := validateDatasetEmbeddingModel(embdID); err != nil {
			return "", true, err
		}
	}
	return embdID, true, nil
}

func normalizeDatasetUpdateExt(ext map[string]interface{}) map[string]interface{} {
	if ext == nil {
		return nil
	}

	updates := make(map[string]interface{}, len(ext))
	for key, value := range ext {
		switch key {
		case "embedding_model":
			updates["embd_id"] = value
		case "chunk_method":
			updates["parser_id"] = value
		case "connectors", "auto_metadata_config", "ext", "parse_type":
			continue
		default:
			updates[key] = value
		}
	}
	return updates
}

func normalizeDatasetUpdateParserConfig(parserConfig map[string]interface{}) map[string]interface{} {
	normalized := common.DeepMergeMaps(nil, parserConfig)
	parentChild, _ := normalized["parent_child"].(map[string]interface{})
	if parentChild == nil {
		parentChild = map[string]interface{}{}
	}

	if datasetBoolValue(parentChild["use_parent_child"]) {
		childrenDelimiter, ok := parentChild["children_delimiter"]
		if !ok {
			childrenDelimiter = "\n"
		}
		normalized["children_delimiter"] = childrenDelimiter
		enableChildren, ok := parentChild["use_parent_child"]
		if !ok {
			enableChildren = true
		}
		normalized["enable_children"] = enableChildren
	} else {
		normalized["children_delimiter"] = ""
		normalized["enable_children"] = false
		normalized["parent_child"] = map[string]interface{}{}
	}

	if extFields, ok := normalized["ext"].(map[string]interface{}); ok {
		delete(normalized, "ext")
		for key, value := range extFields {
			normalized[key] = value
		}
	}

	return normalized
}

func datasetBoolValue(value interface{}) bool {
	switch typedValue := value.(type) {
	case bool:
		return typedValue
	case string:
		return typedValue == "1" || strings.EqualFold(typedValue, "true")
	case int:
		return typedValue != 0
	case int64:
		return typedValue != 0
	case float64:
		return typedValue != 0
	default:
		return false
	}
}

// GetMetadataConfig gets the auto-metadata configuration for a dataset.
func (s *DatasetService) GetMetadataConfig(datasetID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	if kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	return map[string]interface{}{
		"metadata":          parserConfigValueOrEmptyList(kb.ParserConfig, "metadata"),
		"built_in_metadata": parserConfigValueOrEmptyList(kb.ParserConfig, "built_in_metadata"),
	}, common.CodeSuccess, nil
}

// UpdateMetadataConfig updates the auto-metadata configuration for a dataset.
func (s *DatasetService) UpdateMetadataConfig(datasetID, tenantID string, req *MetadataConfigRequest) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	tenantID = strings.TrimSpace(tenantID)

	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}
	if kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", tenantID, datasetID)
	}

	if req == nil {
		req = &MetadataConfigRequest{}
	}

	metadata, err := normalizeMetadataConfigFields(req.Metadata, "metadata")
	if err != nil {
		return nil, common.CodeDataError, err
	}
	builtInMetadata, err := normalizeMetadataConfigFields(req.BuiltInMetadata, "built_in_metadata")
	if err != nil {
		return nil, common.CodeDataError, err
	}

	parserConfig := kb.ParserConfig
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}
	parserConfig["metadata"] = metadata
	parserConfig["built_in_metadata"] = builtInMetadata

	if err := s.kbDAO.UpdateByID(kb.ID, map[string]interface{}{"parser_config": parserConfig}); err != nil {
		return nil, common.CodeServerError, errors.New("Update auto-metadata error.(Database error)")
	}

	return map[string]interface{}{
		"metadata":          metadata,
		"built_in_metadata": builtInMetadata,
	}, common.CodeSuccess, nil
}

// Accessible checks if a knowledge base is accessible by a user
func (s *DatasetService) Accessible(kbID, userID string) bool {
	return s.kbDAO.Accessible(kbID, userID)
}

// GetIngestionSummary returns dataset-level ingestion counters together with
// the aggregated document parsing status, mirroring
// dataset_api_service.get_ingestion_summary.
func (s *DatasetService) GetIngestionSummary(datasetID, userID string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}

	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, datasetID)
	}

	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
	}

	status, err := s.documentDAO.GetParsingStatusByKBID(datasetID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	return map[string]interface{}{
		"doc_num":   kb.DocNum,
		"chunk_num": kb.ChunkNum,
		"token_num": kb.TokenNum,
		"status":    status,
	}, common.CodeSuccess, nil
}

// ListIngestionLogs lists ingestion logs for a dataset, mirroring
// dataset_api_service.list_ingestion_logs. log_type selects between
// dataset-level logs ("dataset") and per-file logs ("file").
func (s *DatasetService) ListIngestionLogs(datasetID, userID string, page, pageSize int, orderby string, desc bool, operationStatus []string, createDateFrom, createDateTo, logType, keywords string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}

	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	if logType != "dataset" && logType != "file" {
		return nil, common.CodeDataError, errors.New("Invalid \"log_type\", expected \"dataset\" or \"file\"")
	}

	var (
		logs  []*entity.PipelineOperationLog
		total int64
		err   error
	)
	if logType == "file" {
		logs, total, err = s.pipelineLogDAO.GetFileLogsByKBID(datasetID, page, pageSize, orderby, desc, keywords, operationStatus, createDateFrom, createDateTo)
	} else {
		logs, total, err = s.pipelineLogDAO.GetDatasetLogsByKBID(datasetID, page, pageSize, orderby, desc, operationStatus, createDateFrom, createDateTo, keywords)
	}
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	items := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		if logType == "file" {
			items = append(items, fileIngestionLogToMap(log))
		} else {
			items = append(items, datasetIngestionLogToMap(log))
		}
	}

	return map[string]interface{}{
		"total": total,
		"logs":  items,
	}, common.CodeSuccess, nil
}

// GetIngestionLog returns a single ingestion log, mirroring
// dataset_api_service.get_ingestion_log. It returns the full record (including
// the `dsl`, `document_id`, `parser_id`, etc.) so that the front-end
// dataflow-result page can render the pipeline timeline and chunks. The
// file-level converter is a superset of the dataset-level fields, so it is
// correct for both dataset-level (graph/raptor/mindmap) and per-file logs.
func (s *DatasetService) GetIngestionLog(datasetID, userID, logID string) (map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}

	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	log, err := s.pipelineLogDAO.GetByIDAndKBID(logID, datasetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, errors.New("Log not found")
		}
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	return fileIngestionLogToMap(log), common.CodeSuccess, nil
}

func datasetIngestionLogToMap(log *entity.PipelineOperationLog) map[string]interface{} {
	return map[string]interface{}{
		"id":               log.ID,
		"tenant_id":        log.TenantID,
		"kb_id":            log.KbID,
		"progress":         log.Progress,
		"progress_msg":     stringPointerValue(log.ProgressMsg),
		"process_begin_at": timePointerValue(log.ProcessBeginAt),
		"process_duration": log.ProcessDuration,
		"task_type":        log.TaskType,
		"operation_status": log.OperationStatus,
		"avatar":           stringPointerValue(log.Avatar),
		"status":           stringPointerValue(log.Status),
		"create_time":      int64PointerValue(log.CreateTime),
		"create_date":      timePointerValue(log.CreateDate),
		"update_time":      int64PointerValue(log.UpdateTime),
		"update_date":      timePointerValue(log.UpdateDate),
	}
}

func fileIngestionLogToMap(log *entity.PipelineOperationLog) map[string]interface{} {
	return map[string]interface{}{
		"id":               log.ID,
		"document_id":      log.DocumentID,
		"tenant_id":        log.TenantID,
		"kb_id":            log.KbID,
		"pipeline_id":      stringPointerValue(log.PipelineID),
		"pipeline_title":   stringPointerValue(log.PipelineTitle),
		"parser_id":        log.ParserID,
		"document_name":    log.DocumentName,
		"document_suffix":  log.DocumentSuffix,
		"document_type":    log.DocumentType,
		"source_from":      log.SourceFrom,
		"progress":         log.Progress,
		"progress_msg":     stringPointerValue(log.ProgressMsg),
		"process_begin_at": timePointerValue(log.ProcessBeginAt),
		"process_duration": log.ProcessDuration,
		"dsl":              jsonMapValue(log.DSL),
		"task_type":        log.TaskType,
		"operation_status": log.OperationStatus,
		"avatar":           stringPointerValue(log.Avatar),
		"status":           stringPointerValue(log.Status),
		"create_time":      int64PointerValue(log.CreateTime),
		"create_date":      timePointerValue(log.CreateDate),
		"update_time":      int64PointerValue(log.UpdateTime),
		"update_date":      timePointerValue(log.UpdateDate),
	}
}

func stringPointerValue(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func int64PointerValue(i *int64) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

func timePointerValue(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02 15:04:05")
}

func jsonMapValue(m entity.JSONMap) interface{} {
	if m == nil {
		return nil
	}
	return m
}

func (s *DatasetService) deleteDataset(tenantID string, kb *entity.Knowledgebase) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		if taskIDs := datasetIndexTaskIDs(kb); len(taskIDs) > 0 {
			if err := tx.Where("id IN ?", taskIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
		}

		var documents []entity.Document
		if err := tx.Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		docIDs := make([]string, 0, len(documents))
		for _, document := range documents {
			docIDs = append(docIDs, document.ID)
		}

		if len(docIDs) > 0 {
			var mappings []entity.File2Document
			if err := tx.Where("document_id IN ?", docIDs).Find(&mappings).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}

			fileIDs := make([]string, 0, len(mappings))
			seenFileIDs := make(map[string]struct{}, len(mappings))
			for _, mapping := range mappings {
				if mapping.FileID == nil || *mapping.FileID == "" {
					continue
				}
				if _, exists := seenFileIDs[*mapping.FileID]; exists {
					continue
				}
				seenFileIDs[*mapping.FileID] = struct{}{}
				fileIDs = append(fileIDs, *mapping.FileID)
			}

			if err := tx.Where("doc_id IN ?", docIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&entity.File2Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if len(fileIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ?", fileIDs).Delete(&entity.File{}).Error; err != nil {
					return fmt.Errorf("Delete dataset error for %s", kb.ID)
				}
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&entity.Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
		}

		if err := tx.Unscoped().
			Where("source_type = ? AND type = ? AND name = ? AND tenant_id = ?", string(entity.FileSourceKnowledgebase), "folder", kb.Name, tenantID).
			Delete(&entity.File{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		if err := tx.Where("id = ?", kb.ID).Delete(&entity.Knowledgebase{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		return nil
	})
}

func validateDatasetChunkMethod(chunkMethod string) error {
	if _, ok := datasetAllowedChunkMethods[chunkMethod]; !ok {
		return errors.New(datasetChunkMethodErrorMessage)
	}
	return nil
}

func validateDatasetAvatar(avatar string) error {
	if !strings.Contains(avatar, ",") {
		return errors.New("Missing MIME prefix. Expected format: data:<mime>;base64,<data>")
	}

	prefix, _, _ := strings.Cut(avatar, ",")
	if !strings.HasPrefix(prefix, "data:") {
		return errors.New("Invalid MIME prefix format. Must start with 'data:'")
	}

	mimeType, _, _ := strings.Cut(strings.TrimPrefix(prefix, "data:"), ";")
	if _, ok := datasetSupportedAvatarMIMETypes[mimeType]; !ok {
		return errors.New("Unsupported MIME type. Allowed: [image/jpeg image/png]")
	}

	return nil
}

func validateDatasetEmbeddingModel(embeddingModel string) error {
	if embeddingModel == "" {
		return errors.New("Embedding model identifier must follow <model_name>@<provider> format")
	}

	modelName, provider, ok := strings.Cut(embeddingModel, "@")
	if !ok {
		return errors.New("Embedding model identifier must follow <model_name>@<provider> format")
	}
	if strings.TrimSpace(modelName) == "" || strings.TrimSpace(provider) == "" {
		return errors.New("Both model_name and provider must be non-empty strings")
	}

	return nil
}

func normalizeDatasetPipelineID(pipelineID string) (*string, error) {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return nil, nil
	}
	if len(pipelineID) != 32 {
		return nil, errors.New("pipeline_id must be 32 hex characters")
	}
	for _, char := range pipelineID {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return nil, errors.New("pipeline_id must be hexadecimal")
		}
	}

	normalized := strings.ToLower(pipelineID)
	return &normalized, nil
}

func validateDatasetParserConfigSize(parserConfig map[string]interface{}) error {
	if len(parserConfig) == 0 {
		return nil
	}

	data, err := json.Marshal(parserConfig)
	if err != nil {
		return errors.New("parser_config must be valid JSON")
	}
	if len(data) > 65535 {
		return fmt.Errorf("Parser config exceeds size limit (max 65,535 characters). Current size: %d", len(data))
	}

	return nil
}

func normalizeDatasetUUID1(id string) (string, error) {
	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("Invalid UUID1 format")
	}
	if parsedUUID.Version() != 1 {
		return "", errors.New("Must be a UUID1 format")
	}
	return strings.ReplaceAll(parsedUUID.String(), "-", ""), nil
}

func (s *DatasetService) verifyEmbeddingAvailability(embdID string, tenantID string) (bool, string) {
	_, _, _, _, err := NewModelProviderService().GetModelConfigFromProviderInstance(tenantID, entity.ModelTypeEmbedding, embdID)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

func applyAutoMetadataConfig(parserConfig map[string]interface{}, config *AutoMetadataConfig) map[string]interface{} {
	if parserConfig == nil {
		parserConfig = make(map[string]interface{})
	}

	fields := make([]map[string]interface{}, 0, len(config.Fields))
	for _, field := range config.Fields {
		fields = append(fields, map[string]interface{}{
			"name":            field.Name,
			"type":            field.Type,
			"description":     field.Description,
			"examples":        field.Examples,
			"restrict_values": field.RestrictValues,
		})
	}
	parserConfig["metadata"] = fields
	enableMetadata := true
	if config.Enabled != nil {
		enableMetadata = *config.Enabled
	}
	parserConfig["enable_metadata"] = enableMetadata
	return parserConfig
}

func parserConfigValueOrEmptyList(parserConfig map[string]interface{}, key string) interface{} {
	if parserConfig == nil {
		return []interface{}{}
	}

	value, ok := parserConfig[key]
	if !ok || value == nil {
		return []interface{}{}
	}

	return value
}

func normalizeMetadataConfigFields(fields []MetadataConfigField, fieldName string) ([]map[string]interface{}, error) {
	normalizedFields := make([]map[string]interface{}, 0, len(fields))
	for i, field := range fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			return nil, fmt.Errorf("%s[%d].key is required", fieldName, i)
		}
		if len(key) > 255 {
			return nil, fmt.Errorf("%s[%d].key should have at most 255 characters", fieldName, i)
		}

		fieldType := strings.TrimSpace(field.Type)
		if _, ok := datasetAllowedMetadataTypes[fieldType]; !ok {
			return nil, fmt.Errorf("%s[%d].type should be one of 'string', 'list', 'time' or 'number'", fieldName, i)
		}

		if field.Description != nil && len(*field.Description) > 65535 {
			return nil, fmt.Errorf("%s[%d].description should have at most 65535 characters", fieldName, i)
		}

		normalizedFields = append(normalizedFields, map[string]interface{}{
			"key":         key,
			"type":        fieldType,
			"description": field.Description,
			"enum":        field.Enum,
		})
	}

	return normalizedFields, nil
}

func datasetListItemToMap(kb *entity.KnowledgebaseListItem) map[string]interface{} {
	item := map[string]interface{}{
		"id":              kb.ID,
		"name":            kb.Name,
		"tenant_id":       kb.TenantID,
		"permission":      kb.Permission,
		"document_count":  kb.DocNum,
		"token_num":       kb.TokenNum,
		"chunk_count":     kb.ChunkNum,
		"chunk_method":    kb.ParserID,
		"embedding_model": kb.EmbdID,
		"nickname":        kb.Nickname,
	}

	if kb.Avatar != nil {
		item["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		item["language"] = *kb.Language
	}
	if kb.Description != nil {
		item["description"] = *kb.Description
	}
	if kb.TenantAvatar != nil {
		item["tenant_avatar"] = *kb.TenantAvatar
	}
	if kb.UpdateTime != nil {
		item["update_time"] = *kb.UpdateTime
	}

	return item
}

func datasetToMap(kb *entity.Knowledgebase) map[string]interface{} {
	item := map[string]interface{}{
		"id":                       kb.ID,
		"tenant_id":                kb.TenantID,
		"name":                     kb.Name,
		"embedding_model":          kb.EmbdID,
		"permission":               kb.Permission,
		"created_by":               kb.CreatedBy,
		"document_count":           kb.DocNum,
		"token_num":                kb.TokenNum,
		"chunk_count":              kb.ChunkNum,
		"similarity_threshold":     kb.SimilarityThreshold,
		"vector_similarity_weight": kb.VectorSimilarityWeight,
		"chunk_method":             kb.ParserID,
		"parser_config":            kb.ParserConfig,
		"pagerank":                 kb.Pagerank,
		"create_time":              kb.CreateTime,
	}

	if kb.Avatar != nil {
		item["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		item["language"] = *kb.Language
	}
	if kb.Description != nil {
		item["description"] = *kb.Description
	}
	if kb.PipelineID != nil {
		item["pipeline_id"] = *kb.PipelineID
	}
	if kb.GraphragTaskID != nil {
		item["graphrag_task_id"] = *kb.GraphragTaskID
	}
	if kb.GraphragTaskFinishAt != nil {
		item["graphrag_task_finish_at"] = kb.GraphragTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.RaptorTaskID != nil {
		item["raptor_task_id"] = *kb.RaptorTaskID
	}
	if kb.RaptorTaskFinishAt != nil {
		item["raptor_task_finish_at"] = kb.RaptorTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.MindmapTaskID != nil {
		item["mindmap_task_id"] = *kb.MindmapTaskID
	}
	if kb.MindmapTaskFinishAt != nil {
		item["mindmap_task_finish_at"] = kb.MindmapTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.UpdateTime != nil {
		item["update_time"] = *kb.UpdateTime
	}

	return item
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
