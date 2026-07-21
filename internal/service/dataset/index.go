package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	redisengine "ragflow/internal/engine/redis"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/utility"

	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func checkType(indexType string) bool {
	haveType := false
	for _, t := range validIndexTypes {
		if indexType == t {
			haveType = true
		}
	}
	return haveType
}

func (d *DatasetService) newRaptorOrGraphRagTask(sampleDoc *entity.Document, taskType string, taskDocID string, queueDocID string, docIDs []string) (*entity.Task, map[string]interface{}, error) {
	if docIDs == nil || len(docIDs) == 0 {
		docIDs = make([]string, 0)
	}
	if !checkIndexTaskType(taskType) {
		return nil, nil, errors.New("type should be graphrag, raptor or mindmap")
	}

	chunkingConfig, err := d.documentDAO.GetChunkingConfig(sampleDoc.ID)
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

	taskID := utility.GenerateUUID()
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

func datasetIndexTaskFinishAtColumn(indexType string) string {
	switch indexType {
	case "graph":
		return "graphrag_task_finish_at"
	case "raptor":
		return "raptor_task_finish_at"
	case "mindmap":
		return "mindmap_task_finish_at"
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

func clearGraphPhaseMarkers(redisClient *redisengine.Client, datasetID string) {
	if redisClient == nil || datasetID == "" {
		return
	}
	for _, phase := range []string{graphPhaseResolutionDone, graphPhaseCommunityDone} {
		if !redisClient.Delete(fmt.Sprintf("graphrag:phase:%s:%s", datasetID, phase)) {
			common.Warn("Failed to clear GraphRAG phase marker", zap.String("dataset_id", datasetID), zap.String("phase", phase))
		}
	}
}

func (d *DatasetService) RunIndex(userID, datasetID, indexType string) (map[string]interface{}, common.ErrorCode, error) {
	if !checkType(indexType) {
		return nil, common.CodeDataError, fmt.Errorf("Invalid index type '%s'. Must be one of %v", indexType, validIndexTypes)
	}

	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
		}
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	taskType := indexTypeToTaskType[indexType]
	displayName := indexTypeToDisplayName[indexType]

	documents, code, err := d.getDocumentsByDatasetForIndex(datasetID)
	if err != nil {
		return nil, code, err
	}
	_ = documents

	sampleDocument := documents[0]
	documentIDs := make([]string, len(documents))

	for i, doc := range documents {
		documentIDs[i] = doc.ID
	}

	task, queueMessage, err := d.newRaptorOrGraphRagTask(sampleDocument, taskType, sampleDocument.ID, graphRaptorQueueDocID, documentIDs)
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

func (d *DatasetService) getDocumentsByDatasetForIndex(datasetID string) ([]*entity.Document, common.ErrorCode, error) {
	documents, _, err := d.documentDAO.GetByKBID(datasetID)
	if err != nil {
		common.Warn("Failed to load dataset documents for index", zap.String("dataset_id", datasetID), zap.Error(err))
		return nil, common.CodeDataError, errors.New("Internal server error")
	}
	if len(documents) == 0 {
		return nil, common.CodeDataError, fmt.Errorf("No documents in Dataset %s", datasetID)
	}
	return documents, common.CodeSuccess, nil
}

func (d *DatasetService) TraceIndex(datasetID, userID, indexType string) (*entity.Task, common.ErrorCode, error) {
	if !checkType(indexType) {
		return nil, common.CodeDataError, fmt.Errorf("Invalid index type '%s'. Must be one of %v", indexType, validIndexTypes)
	}

	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
		}
		return nil, common.CodeDataError, errors.New("Internal server error")
	}

	taskID := datasetIndexTaskID(kb, indexType)

	var task *entity.Task
	if taskID != "" {
		task, err = d.taskDAO.GetByID(taskID)
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

type embeddingCheckSample struct {
	ChunkID           string
	KbID              string
	DocID             string
	DocName           string
	VectorField       string
	Vector            []float64
	PageNum           interface{}
	Position          interface{}
	Top               interface{}
	ContentWithWeight string
	QuestionKeywords  []string
}

func (d *DatasetService) CheckEmbedding(userID, datasetID string, req *service.CheckEmbeddingRequest) (*service.EmbeddingCheckResponse, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
		}
		return nil, common.CodeServerError, errors.New("Internal server error")
	}

	if req == nil || strings.TrimSpace(req.EmbeddingID) == "" {
		return nil, common.CodeDataError, errors.New("`embd_id` is required.")
	}
	embeddingID := strings.TrimSpace(req.EmbeddingID)
	if ok, message := d.verifyEmbeddingAvailability(embeddingID, userID); !ok {
		return nil, common.CodeDataError, errors.New(message)
	}
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("doc engine not initialized")
	}

	driver, modelName, apiConfig, maxTokens, err := service.NewModelProviderService().ResolveModelConfig(kb.TenantID, entity.ModelTypeEmbedding, embeddingID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	embeddingModel := modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)

	checkNum := defaultEmbeddingCheckNum
	if req.CheckNum != nil {
		checkNum = *req.CheckNum
	}
	if checkNum <= 0 {
		checkNum = defaultEmbeddingCheckNum
	}

	samples, err := d.sampleRandomChunksWithVectors(context.Background(), kb.TenantID, datasetID, checkNum)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(samples) == 0 {
		return &service.EmbeddingCheckResponse{
			Summary: datasetEmbeddingCheckSummary(datasetID, embeddingID, 0, nil, ""),
			Results: nil,
		}, common.CodeSuccess, nil
	}

	results := make([]service.EmbeddingCheckResult, 0, len(samples))
	effectiveSimilarities := make([]float64, 0, len(samples))
	matchMode := "content_only"
	for _, sample := range samples {
		if sample.Vector == nil || len(sample.Vector) == 0 {
			continue
		}

		rawChunk, err := d.docEngine.GetChunk(context.Background(), fmt.Sprintf("ragflow_%s", kb.TenantID), sample.ChunkID, []string{datasetID})
		if err != nil {
			continue
		}
		chunkMap := datasetMap(rawChunk)
		if len(chunkMap) == 0 {
			continue
		}

		title := datasetString(chunkMap["title_tks"])
		content := datasetString(chunkMap["content_ltks"])

		var titleVector [][]float64
		if title != "" {
			titleVector, err = datasetEncodeEmbedding(embeddingModel, []string{title})
			if err != nil {
				return nil, common.CodeServerError, err
			}
		}
		var contentVector [][]float64
		if content != "" {
			contentVector, err = datasetEncodeEmbedding(embeddingModel, []string{content})
			if err != nil {
				return nil, common.CodeServerError, err
			}
		}

		var vectors [][]float64
		if len(titleVector) > 0 && len(contentVector) > 0 {
			vectors = [][]float64{titleVector[0], contentVector[0]}
			matchMode = "title_and_content"
		} else if len(titleVector) > 0 {
			vectors = titleVector
		} else if len(contentVector) > 0 {
			vectors = contentVector
		} else {
			continue
		}

		if len(vectors[0]) != len(sample.Vector) {
			return nil, common.CodeDataError, fmt.Errorf("Embedding failure. The dimension (%d) of given embedding model is different from the original (%d)", len(vectors[0]), len(sample.Vector))
		}

		var sim float64
		if len(vectors) == 2 {
			simContent := datasetCosSim(vectors[1], sample.Vector)
			simMix := datasetCosSim(datasetMixVectors(vectors[0], vectors[1], 0.1), sample.Vector)
			sim = simContent
			if simMix > sim {
				sim = simMix
				matchMode = "title+content"
			}
		} else {
			sim = datasetCosSim(vectors[0], sample.Vector)
		}
		sim = datasetRoundFloat(sim, 6)

		effectiveSimilarities = append(effectiveSimilarities, sim)
		results = append(results, service.EmbeddingCheckResult{
			ChunkID:     sample.ChunkID,
			DocID:       sample.DocID,
			DocName:     sample.DocName,
			VectorField: sample.VectorField,
			VectorDim:   len(sample.Vector),
			CosSim:      sim,
		})
	}

	summary := datasetEmbeddingCheckSummary(datasetID, embeddingID, len(samples), effectiveSimilarities, matchMode)
	response := &service.EmbeddingCheckResponse{Summary: summary, Results: results}
	if len(effectiveSimilarities) == 0 {
		return nil, common.CodeDataError, errors.New("No embedded chunks are available to compare.")
	}
	if summary.AvgCosSim >= 0.9 {
		return response, common.CodeSuccess, nil
	}
	return response, common.CodeNotEffective, errors.New("Embedding model switch failed: the average similarity between old and new vectors is below 0.9, indicating incompatible vector spaces.")
}

func (d *DatasetService) sampleRandomChunksWithVectors(ctx context.Context, tenantID, datasetID string, n int) ([]embeddingCheckSample, error) {
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	totalResult, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		Offset:     0,
		Limit:      1,
		Filter: map[string]interface{}{
			"kb_id":         datasetID,
			"available_int": 1,
		},
	})
	if err != nil {
		return nil, err
	}
	if totalResult == nil || totalResult.Total <= 0 {
		return []embeddingCheckSample{}, nil
	}

	total := int(totalResult.Total)
	const maxEmbeddingSamples = 1024
	if n < 0 {
		return nil, fmt.Errorf("invalid sample size: %d", n)
	}
	if n > maxEmbeddingSamples {
		n = maxEmbeddingSamples
	}
	if n > total {
		n = total
	}
	limit := total
	if limit > 1000 {
		limit = 1000
	}
	if n > limit {
		n = limit
	}
	offsets := rand.Perm(limit)
	offsets = offsets[:n]
	sort.Ints(offsets)

	baseFields := []string{"docnm_kwd", "doc_id", "content_with_weight", "page_num_int", "position_int", "top_int"}
	samples := make([]embeddingCheckSample, 0, n)
	for _, offset := range offsets {
		searchResult, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{datasetID},
			Offset:       offset,
			Limit:        1,
			SelectFields: baseFields,
			Filter: map[string]interface{}{
				"kb_id":         datasetID,
				"available_int": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		if searchResult == nil || len(searchResult.Chunks) == 0 {
			continue
		}
		chunkID := datasetChunkID(searchResult.Chunks[0])
		if chunkID == "" {
			continue
		}
		fullChunk, err := d.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
		if err != nil {
			return nil, err
		}
		chunkMap := datasetMap(fullChunk)
		if len(chunkMap) == 0 {
			continue
		}
		vectorField := datasetGuessVecField(chunkMap)
		vector := datasetAsFloatVec(chunkMap[vectorField])
		samples = append(samples, embeddingCheckSample{
			ChunkID:           chunkID,
			KbID:              datasetID,
			DocID:             datasetString(chunkMap["doc_id"]),
			DocName:           datasetString(chunkMap["docnm_kwd"]),
			VectorField:       vectorField,
			Vector:            vector,
			PageNum:           chunkMap["page_num_int"],
			Position:          chunkMap["position_int"],
			Top:               chunkMap["top_int"],
			ContentWithWeight: datasetString(chunkMap["content_with_weight"]),
			QuestionKeywords:  datasetStringSlice(chunkMap["question_keywords"]),
		})
	}

	if len(samples) == 0 {
		return nil, errors.New("no valid chunks with vectors found")
	}
	return samples, nil
}

func (d *DatasetService) verifyEmbeddingAvailability(embdID string, tenantID string) (bool, string) {
	_, _, _, _, err := service.NewModelProviderService().ResolveModelConfig(tenantID, entity.ModelTypeEmbedding, embdID)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (d *DatasetService) DeleteIndex(userID, datasetID, indexType string, wipe bool) (common.ErrorCode, error) {
	if !checkType(indexType) {
		return common.CodeArgumentError, fmt.Errorf("Invalid index type '%s'", indexType)
	}

	if datasetID == "" {
		return common.CodeDataError, errors.New(`Lack of "Dataset ID"`)
	}

	if !d.kbDAO.Accessible(datasetID, userID) {
		return common.CodeDataError, errors.New("No authorization.")
	}

	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return common.CodeDataError, errors.New("Invalid Dataset ID")
		}
		return common.CodeDataError, errors.New("Internal server error")
	}

	taskFinishAtField := datasetIndexTaskFinishAtColumn(indexType)
	taskID := datasetIndexTaskID(kb, indexType)

	common.Info("delete_index", zap.String("dataset_id", datasetID), zap.String("index_type", indexType), zap.Bool("wipe", wipe))

	if taskID != "" {
		redisClient := redisengine.Get()
		if redisClient == nil || !redisClient.Set(fmt.Sprintf("%s-cancel", taskID), "x", 0) {
			common.Warn("Failed to set dataset index cancellation marker", zap.String("dataset_id", datasetID), zap.String("task_id", taskID))
		}
		if err := dao.DB.Unscoped().Where("id = ?", taskID).Delete(&entity.Task{}).Error; err != nil {
			common.Warn("Failed to delete dataset index task", zap.String("dataset_id", datasetID), zap.String("task_id", taskID), zap.Error(err))
			return common.CodeDataError, errors.New("Internal server error")
		}
	}

	if wipe && indexType == "graph" {
		if d.docEngine == nil {
			return common.CodeServerError, errors.New("Document engine is not initialized")
		}
		indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
		_, err = d.docEngine.DeleteChunks(context.Background(), map[string]interface{}{
			"knowledge_graph_kwd": []interface{}{"graph", "subgraph", "entity", "relation", "community_report"},
			"kb_id":               datasetID,
		}, indexName, datasetID)
		if err != nil {
			common.Warn("Failed to delete GraphRAG artefacts", zap.String("dataset_id", datasetID), zap.Error(err))
			return common.CodeDataError, errors.New("Internal server error")
		}
		clearGraphPhaseMarkers(redisengine.Get(), datasetID)
		common.Info("delete_index: cleared GraphRAG artefacts and phase markers", zap.String("dataset_id", datasetID))
	} else if wipe && indexType == "raptor" {
		if d.docEngine == nil {
			return common.CodeServerError, errors.New("Document engine is not initialized")
		}
		indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
		_, err = d.docEngine.DeleteChunks(context.Background(), map[string]interface{}{
			"raptor_kwd": []interface{}{"raptor"},
			"kb_id":      datasetID,
		}, indexName, datasetID)
		if err != nil {
			common.Warn("Failed to delete RAPTOR artefacts", zap.String("dataset_id", datasetID), zap.Error(err))
			return common.CodeDataError, errors.New("Internal server error")
		}
	}

	updates := datasetIndexTaskIDUpdate(indexType, "")
	if taskFinishAtField != "" {
		updates[taskFinishAtField] = nil
	}
	if len(updates) > 0 {
		if err := d.kbDAO.UpdateByID(kb.ID, updates); err != nil {
			common.Warn("Failed to clear KB index task refs", zap.String("dataset_id", datasetID), zap.Error(err))
		}
	}

	return common.CodeSuccess, nil
}
