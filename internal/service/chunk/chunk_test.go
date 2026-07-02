package chunk

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestIsZeroVector(t *testing.T) {
	if !common.IsZeroVector([]float64{0, 0, 0}) {
		t.Error("all zeros should be true")
	}
	if common.IsZeroVector([]float64{0, 1, 0}) {
		t.Error("non-zero should be false")
	}
	if !common.IsZeroVector([]float64{}) {
		t.Error("empty should be true (treated as zero)")
	}
	if !common.IsZeroVector(nil) {
		t.Error("nil should be true")
	}
}

func TestHydrateChunkVectors_AllNonZero(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "c1", "vector": []float64{1, 2, 3}},
		{"id": "c2", "vector": []float64{4, 5, 6}},
	}
	// No zero vectors → nothing to hydrate.
	hydrateChunkVectors(context.Background(), nil, chunks, nil, nil)
	if !reflect.DeepEqual(chunks[0]["vector"], []float64{1, 2, 3}) {
		t.Error("non-zero vector should not be changed")
	}
	if !reflect.DeepEqual(chunks[1]["vector"], []float64{4, 5, 6}) {
		t.Error("non-zero vector should not be changed")
	}
}

func TestHydrateChunkVectors_EmptyChunks(t *testing.T) {
	// Should not panic on empty or nil.
	hydrateChunkVectors(context.Background(), nil, nil, nil, nil)
	hydrateChunkVectors(context.Background(), nil, []map[string]interface{}{}, nil, nil)
}

func TestHydrateChunkVectors_MissingIDs(t *testing.T) {
	chunks := []map[string]interface{}{
		{"vector": []float64{1.0}}, // no id — skipped
	}
	hydrateChunkVectors(context.Background(), nil, chunks, nil, nil)
	// Should not change anything when engine is nil (FetchChunkVectors returns zero vectors).
	// The function doesn't panic — it just can't hydrate because dim is 0.
	// With nil engine, FetchChunkVectors returns zero vectors, so the zero stays zero.
}

func TestHydrateChunkVectors_NoDim(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "c1", "vector": []float64{}},
	}
	hydrateChunkVectors(context.Background(), nil, chunks, []string{"kb1"}, []string{"t1"})
	// Empty vectors have dim=0 → early return. No crash.
}

func TestKnowledgebaseEmbeddingKey(t *testing.T) {
	tenantEmbdID := int64(42)

	tests := []struct {
		name     string
		kb       *entity.Knowledgebase
		tenantID string
		want     string
	}{
		{
			name: "uses tenant embedding id before embd id",
			kb: &entity.Knowledgebase{
				EmbdID:       "shared-model",
				TenantEmbdID: &tenantEmbdID,
			},
			want: "tenant:42",
		},
		{
			name: "uses embd id without tenant embedding id",
			kb: &entity.Knowledgebase{
				EmbdID: "shared-model",
			},
			want: "embd:shared-model",
		},
		{
			name:     "uses tenant default when embedding id is empty",
			kb:       &entity.Knowledgebase{},
			tenantID: "tenant-1",
			want:     "default:tenant-1",
		},
		{
			name: "ignores non-positive tenant embedding id",
			kb: &entity.Knowledgebase{
				EmbdID:       "shared-model",
				TenantEmbdID: new(int64),
			},
			want: "embd:shared-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := knowledgebaseEmbeddingKey(tt.kb, tt.tenantID); got != tt.want {
				t.Fatalf("knowledgebaseEmbeddingKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePrevalidatesDocumentsBeforeMutating(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)

	userID := "user-1"
	datasetID := "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestKB(t, "kb-2", userID)
	insertChunkTestDoc(t, "doc-1", datasetID)
	insertChunkTestDoc(t, "doc-2", "kb-2")
	insertChunkTestTask(t, "task-1", "doc-1")

	svc := &ChunkService{
		docEngine:   nil,
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
	}

	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{
		DocumentIDs: []string{"doc-1", "missing-doc", "doc-2"},
	})
	if err == nil {
		t.Fatal("expected parse to fail")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %v", code)
	}
	if !strings.Contains(err.Error(), "missing-doc") || !strings.Contains(err.Error(), "doc-2") {
		t.Fatalf("expected missing and foreign documents in error, got %q", err.Error())
	}

	var taskCount int64
	if err := dao.DB.Model(&entity.Task{}).Where("doc_id = ?", "doc-1").Count(&taskCount).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("expected existing task to remain, got %d tasks", taskCount)
	}

	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc.Run != nil {
		t.Fatalf("expected doc run to remain nil, got %q", *doc.Run)
	}
	if doc.ChunkNum != 7 {
		t.Fatalf("expected chunk_num to remain 7, got %d", doc.ChunkNum)
	}
}

func TestParseRejectsInaccessibleDataset(t *testing.T) {
	svc := newParseTestService(t)
	svc.accessibleFunc = func(string, string) bool { return false }

	_, code, err := svc.Parse("user-1", "kb-1", &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if err == nil {
		t.Fatal("expected parse to fail")
	}
	if code != common.CodeOperatingError {
		t.Fatalf("expected CodeOperatingError, got %v", code)
	}
	if !strings.Contains(err.Error(), "don't own the dataset") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRequiresDocumentIDs(t *testing.T) {
	svc := newParseTestService(t)
	svc.accessibleFunc = func(string, string) bool { return true }

	for _, req := range []*service.ParseFileRequest{nil, {DocumentIDs: nil}, {DocumentIDs: []string{}}} {
		_, code, err := svc.Parse("user-1", "kb-1", req)
		if err == nil {
			t.Fatal("expected parse to fail")
		}
		if code != common.CodeDataError {
			t.Fatalf("expected CodeDataError, got %v", code)
		}
		if !strings.Contains(err.Error(), "document_ids") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestParseReturnsDataErrorWhenDatasetMissing(t *testing.T) {
	svc := newParseTestService(t)
	svc.accessibleFunc = func(string, string) bool { return true }
	svc.getKnowledgebaseByIDFunc = func(string) (*entity.Knowledgebase, error) { return nil, nil }

	_, code, err := svc.Parse("user-1", "kb-1", &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if err == nil {
		t.Fatal("expected parse to fail")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %v", code)
	}
	if !strings.Contains(err.Error(), "dataset not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReturnsServerErrorWhenDocumentsQueryFails(t *testing.T) {
	svc := newParseTestService(t)
	queryErr := errors.New("documents query failed")
	svc.accessibleFunc = func(string, string) bool { return true }
	svc.getKnowledgebaseByIDFunc = func(string) (*entity.Knowledgebase, error) {
		return &entity.Knowledgebase{ID: "kb-1", TenantID: "user-1"}, nil
	}
	svc.getDocumentsByIDsFunc = func([]string) ([]*entity.Document, error) {
		return nil, queryErr
	}

	_, code, err := svc.Parse("user-1", "kb-1", &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if !errors.Is(err, queryErr) {
		t.Fatalf("expected query error, got %v", err)
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
}

func TestParseRejectsRunningDocument(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)

	userID := "user-1"
	datasetID := "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)
	running := string(entity.TaskStatusRunning)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("run", running).Error; err != nil {
		t.Fatalf("mark doc running: %v", err)
	}

	svc := newParseTestService(t)
	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if err == nil {
		t.Fatal("expected parse to fail")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %v", code)
	}
	if !strings.Contains(err.Error(), "currently being processed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReturnsServerErrorWhenDeleteChunksFails(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)

	deleteErr := errors.New("delete chunks failed")
	engine := &parseTestDocEngine{deleteChunksErr: deleteErr}
	svc := newParseTestService(t)
	svc.docEngine = engine

	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if !errors.Is(err, deleteErr) {
		t.Fatalf("expected delete chunks error, got %v", err)
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
	if engine.deleteChunksCalls != 1 {
		t.Fatalf("expected DeleteChunks once, got %d", engine.deleteChunksCalls)
	}
}

func TestParseReturnsServerErrorWhenDeleteTasksFails(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)

	deleteErr := errors.New("delete tasks failed")
	svc := newParseTestService(t)
	svc.deleteTasksByDocIDsFunc = func([]string) (int64, error) { return 0, deleteErr }

	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if !errors.Is(err, deleteErr) {
		t.Fatalf("expected delete tasks error, got %v", err)
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
}

func TestParseReturnsServerErrorWhenStorageAddressFails(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)

	storageErr := errors.New("storage address failed")
	svc := newParseTestService(t)
	svc.getDocumentStorageAddressFunc = func(*entity.Document) (string, string, error) {
		return "", "", storageErr
	}

	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storage error, got %v", err)
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
}

func TestParseReturnsServerErrorWhenQueueFails(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)

	queueErr := errors.New("queue failed")
	svc := newParseTestService(t)
	svc.queueParseTasksFunc = func(*entity.Document, string, string, int64) error {
		return queueErr
	}

	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if !errors.Is(err, queueErr) {
		t.Fatalf("expected queue error, got %v", err)
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
}

func TestParseCleansTasksWhenBeginParseFails(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)

	beginErr := errors.New("begin parse failed")
	deleteCalls := 0
	svc := newParseTestService(t)
	svc.beginParseDocumentFunc = func(string) error { return beginErr }
	svc.deleteTasksByDocIDsFunc = func([]string) (int64, error) {
		deleteCalls++
		return 1, nil
	}

	_, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if !errors.Is(err, beginErr) {
		t.Fatalf("expected begin error, got %v", err)
	}
	if code != common.CodeServerError {
		t.Fatalf("expected CodeServerError, got %v", code)
	}
	if deleteCalls != 2 {
		t.Fatalf("expected initial delete and cleanup delete, got %d calls", deleteCalls)
	}
}

func TestParseReturnsPartialSuccessForDuplicateDocumentIDs(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)

	svc := newParseTestService(t)
	result, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{
		DocumentIDs: []string{"doc-1", "doc-1"},
	})
	if err == nil {
		t.Fatal("expected duplicate warning error")
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected CodeSuccess, got %v", code)
	}
	if result["success_count"] != 1 {
		t.Fatalf("expected one successful parse, got %v", result["success_count"])
	}
	errorsValue, ok := result["errors"].([]string)
	if !ok || len(errorsValue) != 1 || !strings.Contains(errorsValue[0], "Duplicate document ids: doc-1") {
		t.Fatalf("unexpected duplicate errors: %#v", result["errors"])
	}
}

func TestParseQueuesAndBeginsDocument(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID := "user-1", "kb-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, "doc-1", datasetID)
	insertChunkTestTask(t, "task-1", "doc-1")

	queueCalls := 0
	svc := newParseTestService(t)
	svc.queueParseTasksFunc = func(doc *entity.Document, bucket, objectName string, priority int64) error {
		queueCalls++
		if doc.ID != "doc-1" || bucket != datasetID || objectName != "doc-1.txt" || priority != 0 {
			t.Fatalf("unexpected queue args: doc=%s bucket=%s object=%s priority=%d", doc.ID, bucket, objectName, priority)
		}
		return nil
	}

	result, code, err := svc.Parse(userID, datasetID, &service.ParseFileRequest{DocumentIDs: []string{"doc-1"}})
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected CodeSuccess, got %v", code)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if queueCalls != 1 {
		t.Fatalf("expected queue once, got %d", queueCalls)
	}

	var taskCount int64
	if err := dao.DB.Model(&entity.Task{}).Where("doc_id = ?", "doc-1").Count(&taskCount).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("expected old tasks to be deleted, got %d", taskCount)
	}

	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusRunning) {
		t.Fatalf("expected document to be running, got %v", doc.Run)
	}
	if doc.ChunkNum != 0 {
		t.Fatalf("expected chunk_num reset to 0, got %d", doc.ChunkNum)
	}
}

func TestAddChunkSuccess(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID, documentID := "user-1", "kb-1", "doc-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, documentID, datasetID)

	engine := &addChunkTestEngine{}
	var incrementTokenNum, incrementChunkNum int64
	svc := &ChunkService{
		docEngine:   engine,
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
		accessibleFunc: func(datasetIDArg, userIDArg string) bool {
			return datasetIDArg == datasetID && userIDArg == userID
		},
		getKnowledgebaseByIDFunc: func(id string) (*entity.Knowledgebase, error) {
			return &entity.Knowledgebase{ID: id, TenantID: userID, EmbdID: "embed-1"}, nil
		},
		getEmbeddingModelFunc: func(string, string) (*models.EmbeddingModel, error) {
			driver := &stubEmbeddingDriver{
				embeddings: []models.EmbeddingData{
					{Embedding: []float64{1, 2}},
					{Embedding: []float64{3, 4}},
				},
			}
			modelName := "embed-1"
			return models.NewEmbeddingModel(driver, &modelName, &models.APIConfig{}, 0), nil
		},
		incrementChunkStatsFunc: func(docID, kbID string, tokenNum, chunkNum int64, duration float64) error {
			if docID != documentID || kbID != datasetID || duration != 0 {
				t.Fatalf("unexpected increment args doc=%s kb=%s duration=%v", docID, kbID, duration)
			}
			incrementTokenNum = tokenNum
			incrementChunkNum = chunkNum
			return nil
		},
		tokenizeFunc:            func(text string) (string, error) { return text, nil },
		fineGrainedTokenizeFunc: func(text string) (string, error) { return text + "_fg", nil },
		numTokensFunc:           func(text string) int { return len(text) },
	}

	resp, err := svc.AddChunk(&service.AddChunkRequest{
		DatasetID:         datasetID,
		DocumentID:        documentID,
		Content:           "chunk body",
		ImportantKeywords: []string{"k1"},
		Questions:         []string{" q1 ", ""},
		TagKwd:            []string{"tag1"},
		TagFeas:           map[string]interface{}{"tag1": float64(0.5)},
	}, userID)
	if err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}
	if resp == nil || resp.Chunk == nil {
		t.Fatalf("expected chunk response, got %#v", resp)
	}
	if resp.Chunk["dataset_id"] != datasetID || resp.Chunk["document_id"] != documentID {
		t.Fatalf("unexpected response chunk: %#v", resp.Chunk)
	}
	if resp.Chunk["content"] != "chunk body" {
		t.Fatalf("content = %v, want chunk body", resp.Chunk["content"])
	}
	if resp.Chunk["document"] != "doc-1.txt" {
		t.Fatalf("document = %v, want doc-1.txt", resp.Chunk["document"])
	}
	if incrementChunkNum != 1 {
		t.Fatalf("increment chunk num = %d, want 1", incrementChunkNum)
	}
	if incrementTokenNum <= 0 {
		t.Fatalf("increment token num = %d, want > 0", incrementTokenNum)
	}
	if len(engine.insertedChunks) != 1 {
		t.Fatalf("inserted chunks = %d, want 1", len(engine.insertedChunks))
	}
	inserted := engine.insertedChunks[0]
	if inserted["doc_id"] != documentID || inserted["kb_id"] != datasetID {
		t.Fatalf("unexpected inserted chunk: %#v", inserted)
	}
	if inserted["img_id"] != nil {
		t.Fatalf("did not expect image id in inserted chunk: %#v", inserted)
	}
	vec, ok := inserted["q_2_vec"].([]float64)
	if !ok {
		t.Fatalf("expected q_2_vec []float64, got %T", inserted["q_2_vec"])
	}
	if len(vec) != 2 || vec[0] < 2.7999 || vec[0] > 2.8001 || vec[1] < 3.7999 || vec[1] > 3.8001 {
		t.Fatalf("vector = %v, want approximately [2.8 3.8]", vec)
	}
}

func TestAddChunkValidationErrors(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestKB(t, "kb-1", "user-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")

	svc := &ChunkService{
		docEngine:   &addChunkTestEngine{},
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
	}

	tests := []struct {
		name    string
		req     *service.AddChunkRequest
		setup   func(*ChunkService)
		wantMsg string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantMsg: "invalid request payload",
		},
		{
			name: "empty content",
			req:  &service.AddChunkRequest{DatasetID: "kb-1", DocumentID: "doc-1", Content: " "},
			setup: func(svc *ChunkService) {
				svc.accessibleFunc = func(string, string) bool { return true }
				svc.getKnowledgebaseByIDFunc = func(string) (*entity.Knowledgebase, error) {
					return &entity.Knowledgebase{ID: "kb-1", TenantID: "user-1", EmbdID: "embed-1"}, nil
				}
			},
			wantMsg: "`content` is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(svc)
			}
			_, err := svc.AddChunk(tt.req, "user-1")
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantMsg)
			}
		})
	}
}

func TestAddChunkImageAndTagFeatureValidation(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID, documentID := "user-1", "kb-1", "doc-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, documentID, datasetID)

	storeCalls := 0
	svc := &ChunkService{
		docEngine:      &addChunkTestEngine{},
		kbDAO:          dao.NewKnowledgebaseDAO(),
		documentDAO:    dao.NewDocumentDAO(),
		accessibleFunc: func(string, string) bool { return true },
		getKnowledgebaseByIDFunc: func(id string) (*entity.Knowledgebase, error) {
			return &entity.Knowledgebase{ID: id, TenantID: userID, EmbdID: "embed-1"}, nil
		},
		tokenizeFunc:            func(text string) (string, error) { return text, nil },
		fineGrainedTokenizeFunc: func(text string) (string, error) { return text + "_fg", nil },
		numTokensFunc:           func(text string) int { return len(text) },
		getEmbeddingModelFunc: func(string, string) (*models.EmbeddingModel, error) {
			driver := &stubEmbeddingDriver{
				embeddings: []models.EmbeddingData{
					{Embedding: []float64{1, 1}},
					{Embedding: []float64{1, 1}},
				},
			}
			modelName := "embed-1"
			return models.NewEmbeddingModel(driver, &modelName, &models.APIConfig{}, 0), nil
		},
		incrementChunkStatsFunc: func(string, string, int64, int64, float64) error { return nil },
		storeChunkImageFunc: func(bucket, chunkID string, imageBinary []byte) error {
			storeCalls++
			if bucket != datasetID || chunkID == "" || len(imageBinary) == 0 {
				t.Fatalf("unexpected store args bucket=%s chunkID=%s len=%d", bucket, chunkID, len(imageBinary))
			}
			return nil
		},
	}

	_, err := svc.AddChunk(&service.AddChunkRequest{
		DatasetID:   datasetID,
		DocumentID:  documentID,
		Content:     "chunk body",
		ImageBase64: strPtr("not-base64"),
	}, userID)
	if err == nil || !strings.Contains(err.Error(), "Invalid `image_base64`") {
		t.Fatalf("expected invalid image error, got %v", err)
	}

	validJPEG := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2pRZ0AAAAASUVORK5CYII="

	resp, err := svc.AddChunk(&service.AddChunkRequest{
		DatasetID:  datasetID,
		DocumentID: documentID,
		Content:    "chunk body",
		TagFeas:    map[string]interface{}{"tag1": "bad"},
	}, userID)
	if err == nil || !strings.Contains(err.Error(), "`tag_feas` values must be finite numbers") || resp != nil {
		t.Fatalf("expected tag_feas validation error, got resp=%#v err=%v", resp, err)
	}

	resp, err = svc.AddChunk(&service.AddChunkRequest{
		DatasetID:   datasetID,
		DocumentID:  documentID,
		Content:     "chunk body",
		TagFeas:     map[string]interface{}{"tag1": float64(1)},
		ImageBase64: strPtr(validJPEG),
	}, userID)
	if err != nil {
		t.Fatalf("AddChunk() with image error = %v", err)
	}
	if storeCalls != 1 {
		t.Fatalf("store image calls = %d, want 1", storeCalls)
	}
	if _, ok := resp.Chunk["image_id"]; !ok {
		t.Fatalf("expected image_id in response, got %#v", resp.Chunk)
	}
}

func TestAddChunkIncrementsStatsAfterInsert(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	userID, datasetID, documentID := "user-1", "kb-1", "doc-1"
	insertChunkTestKB(t, datasetID, userID)
	insertChunkTestDoc(t, documentID, datasetID)

	var incrementCalls int
	engine := &addChunkTestEngine{}
	svc := &ChunkService{
		docEngine:      engine,
		kbDAO:          dao.NewKnowledgebaseDAO(),
		documentDAO:    dao.NewDocumentDAO(),
		accessibleFunc: func(string, string) bool { return true },
		getKnowledgebaseByIDFunc: func(id string) (*entity.Knowledgebase, error) {
			return &entity.Knowledgebase{ID: id, TenantID: userID, EmbdID: "embed-1"}, nil
		},
		getEmbeddingModelFunc: func(string, string) (*models.EmbeddingModel, error) {
			driver := &stubEmbeddingDriver{
				embeddings: []models.EmbeddingData{
					{Embedding: []float64{1, 2}},
					{Embedding: []float64{3, 4}},
				},
			}
			modelName := "embed-1"
			return models.NewEmbeddingModel(driver, &modelName, &models.APIConfig{}, 0), nil
		},
		incrementChunkStatsFunc: func(string, string, int64, int64, float64) error {
			incrementCalls++
			return nil
		},
		tokenizeFunc:            func(text string) (string, error) { return text, nil },
		fineGrainedTokenizeFunc: func(text string) (string, error) { return text + "_fg", nil },
		numTokensFunc:           func(text string) int { return len(text) },
	}

	_, err := svc.AddChunk(&service.AddChunkRequest{
		DatasetID:  datasetID,
		DocumentID: documentID,
		Content:    "chunk body",
	}, userID)
	if err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}
	if incrementCalls != 1 {
		t.Fatalf("increment calls = %d, want 1", incrementCalls)
	}
	if len(engine.insertedChunks) != 1 {
		t.Fatalf("inserted chunks = %d, want 1", len(engine.insertedChunks))
	}
	importantKwd, ok := engine.insertedChunks[0]["important_kwd"].([]string)
	if !ok || len(importantKwd) != 0 {
		t.Fatalf("important_kwd = %#v, want empty []string", engine.insertedChunks[0]["important_kwd"])
	}
}

func TestStoreChunkImageMergesExistingImage(t *testing.T) {
	oldImage := mustEncodePNG(t, image.Rect(0, 0, 2, 2))
	newImage := mustEncodePNG(t, image.Rect(0, 0, 1, 1))
	mockStorage := &chunkImageStorage{
		exists:    true,
		oldBinary: oldImage,
	}

	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() {
		factory.SetStorage(originalStorage)
	})

	svc := &ChunkService{}
	if err := svc.storeChunkImage("kb-1", "chunk-1", newImage); err != nil {
		t.Fatalf("storeChunkImage() error = %v", err)
	}
	if mockStorage.putCalls != 1 {
		t.Fatalf("put calls = %d, want 1", mockStorage.putCalls)
	}
}

func TestRemoveChunksDecrementsStatsAfterDelete(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")
	setChunkTestStats(t, "doc-1", "kb-1", 70, 7, 100, 10)

	engine := &parseTestDocEngine{deleteChunksCount: 3}
	svc := &ChunkService{
		docEngine:     engine,
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}

	deletedCount, err := svc.RemoveChunks(&service.RemoveChunksRequest{
		DocID:    "doc-1",
		ChunkIDs: []string{"chunk-1", "chunk-2", "chunk-3"},
	}, "user-1")
	if err != nil {
		t.Fatalf("RemoveChunks() error = %v", err)
	}
	if deletedCount != 3 {
		t.Fatalf("deleted count = %d, want 3", deletedCount)
	}
	if engine.deleteChunksCalls != 1 {
		t.Fatalf("DeleteChunks calls = %d, want 1", engine.deleteChunksCalls)
	}
	if engine.deleteIndexName != "ragflow_tenant-1" || engine.deleteDatasetID != "kb-1" {
		t.Fatalf("unexpected delete target index=%q dataset=%q", engine.deleteIndexName, engine.deleteDatasetID)
	}
	if engine.deleteChunksCondition["doc_id"] != "doc-1" {
		t.Fatalf("delete doc_id condition = %#v, want doc-1", engine.deleteChunksCondition["doc_id"])
	}
	if !reflect.DeepEqual(engine.deleteChunksCondition["id"], []interface{}{"chunk-1", "chunk-2", "chunk-3"}) {
		t.Fatalf("delete id condition = %#v", engine.deleteChunksCondition["id"])
	}

	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc.TokenNum != 70 {
		t.Fatalf("document token_num = %d, want 70", doc.TokenNum)
	}
	if doc.ChunkNum != 4 {
		t.Fatalf("document chunk_num = %d, want 4", doc.ChunkNum)
	}

	kb, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get kb: %v", err)
	}
	if kb.TokenNum != 100 {
		t.Fatalf("knowledgebase token_num = %d, want 100", kb.TokenNum)
	}
	if kb.ChunkNum != 7 {
		t.Fatalf("knowledgebase chunk_num = %d, want 7", kb.ChunkNum)
	}
}

func TestRemoveChunksSkipsStatsWhenNothingDeleted(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")
	setChunkTestStats(t, "doc-1", "kb-1", 70, 7, 100, 10)

	var decrementCalls int
	svc := &ChunkService{
		docEngine:     &parseTestDocEngine{deleteChunksCount: 0},
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
		decrementChunkStatsFunc: func(string, string, int64, int64, float64) error {
			decrementCalls++
			return nil
		},
	}

	deletedCount, err := svc.RemoveChunks(&service.RemoveChunksRequest{
		DocID:     "doc-1",
		DeleteAll: true,
	}, "user-1")
	if err != nil {
		t.Fatalf("RemoveChunks() error = %v", err)
	}
	if deletedCount != 0 {
		t.Fatalf("deleted count = %d, want 0", deletedCount)
	}
	if decrementCalls != 0 {
		t.Fatalf("decrement calls = %d, want 0", decrementCalls)
	}
}

func TestRemoveChunksReturnsStatsError(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")

	svc := &ChunkService{
		docEngine:     &parseTestDocEngine{deleteChunksCount: 2},
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
		decrementChunkStatsFunc: func(docID, kbID string, tokenNum, chunkNum int64, duration float64) error {
			if docID != "doc-1" || kbID != "kb-1" || tokenNum != 0 || chunkNum != 2 || duration != 0 {
				t.Fatalf("unexpected decrement args doc=%s kb=%s token=%d chunk=%d duration=%v", docID, kbID, tokenNum, chunkNum, duration)
			}
			return errors.New("stats update failed")
		},
	}

	deletedCount, err := svc.RemoveChunks(&service.RemoveChunksRequest{
		DocID:    "doc-1",
		ChunkIDs: []string{"chunk-1", "chunk-2"},
	}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "failed to update chunk stats") {
		t.Fatalf("expected stats update error, got count=%d err=%v", deletedCount, err)
	}
	if deletedCount != 2 {
		t.Fatalf("deleted count on error = %d, want 2", deletedCount)
	}
}

func TestDecrementChunkStatsClampsCounters(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")
	setChunkTestStats(t, "doc-1", "kb-1", 2, 1, 3, 2)
	if err := dao.DB.Model(&entity.Document{}).
		Where("id = ?", "doc-1").
		Update("process_duration", 0.5).Error; err != nil {
		t.Fatalf("set process duration: %v", err)
	}

	svc := &ChunkService{}
	if err := svc.decrementChunkStats("doc-1", "kb-1", 5, 7, -1); err != nil {
		t.Fatalf("decrementChunkStats() error = %v", err)
	}

	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc.TokenNum != 0 || doc.ChunkNum != 0 || doc.ProcessDuration != 0 {
		t.Fatalf("document stats token=%d chunk=%d duration=%v, want zeros", doc.TokenNum, doc.ChunkNum, doc.ProcessDuration)
	}

	kb, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get kb: %v", err)
	}
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("knowledgebase stats token=%d chunk=%d, want zeros", kb.TokenNum, kb.ChunkNum)
	}
}

func TestDecrementChunkStatsRollsBackWhenKnowledgebaseMissing(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestDoc(t, "doc-1", "missing-kb")

	svc := &ChunkService{}
	err := svc.decrementChunkStats("doc-1", "missing-kb", 0, 3, 0)
	if err == nil || !strings.Contains(err.Error(), "knowledgebase not found") {
		t.Fatalf("expected missing knowledgebase error, got %v", err)
	}

	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc.ChunkNum != 7 {
		t.Fatalf("document chunk_num after rollback = %d, want 7", doc.ChunkNum)
	}
}

func setupChunkTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.UserTenant{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.Task{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func pushChunkTestDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func newParseTestService(t *testing.T) *ChunkService {
	t.Helper()

	return &ChunkService{
		docEngine:   nil,
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
		getDocumentStorageAddressFunc: func(doc *entity.Document) (string, string, error) {
			if doc.Location == nil {
				return doc.KbID, "", nil
			}
			return doc.KbID, *doc.Location, nil
		},
		queueParseTasksFunc: func(*entity.Document, string, string, int64) error {
			return nil
		},
	}
}

func insertChunkTestUserTenant(t *testing.T, userID, tenantID string) {
	t.Helper()

	status := "1"
	userTenant := &entity.UserTenant{
		ID:        userID + "-" + tenantID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "owner",
		InvitedBy: userID,
		Status:    &status,
	}
	if err := dao.DB.Create(userTenant).Error; err != nil {
		t.Fatalf("insert user tenant: %v", err)
	}
}

func insertChunkTestKB(t *testing.T, id, tenantID string) {
	t.Helper()

	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:           id,
		TenantID:     tenantID,
		Name:         id,
		EmbdID:       "embedding-model",
		Permission:   string(entity.TenantPermissionMe),
		CreatedBy:    tenantID,
		ParserConfig: entity.JSONMap{},
		Status:       &status,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
}

func insertChunkTestDoc(t *testing.T, id, kbID string) {
	t.Helper()

	location := id + ".txt"
	name := id + ".txt"
	status := string(entity.StatusValid)
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{},
		SourceType:   string(entity.FileSourceLocal),
		Type:         "txt",
		CreatedBy:    "user-1",
		Name:         &name,
		Location:     &location,
		ChunkNum:     7,
		Suffix:       ".txt",
		Status:       &status,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert doc: %v", err)
	}
}

func setChunkTestStats(t *testing.T, docID, kbID string, docTokenNum, docChunkNum, kbTokenNum, kbChunkNum int64) {
	t.Helper()

	if err := dao.DB.Model(&entity.Document{}).
		Where("id = ?", docID).
		Updates(map[string]interface{}{"token_num": docTokenNum, "chunk_num": docChunkNum}).Error; err != nil {
		t.Fatalf("set doc stats: %v", err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", kbID).
		Updates(map[string]interface{}{"token_num": kbTokenNum, "chunk_num": kbChunkNum}).Error; err != nil {
		t.Fatalf("set kb stats: %v", err)
	}
}

func insertChunkTestTask(t *testing.T, id, docID string) {
	t.Helper()

	digest := "digest"
	task := &entity.Task{
		ID:     id,
		DocID:  docID,
		Digest: &digest,
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

type parseTestDocEngine struct {
	deleteChunksErr       error
	deleteChunksCount     int64
	deleteChunksCalls     int
	deleteChunksCondition map[string]interface{}
	deleteIndexName       string
	deleteDatasetID       string
}

func (e *parseTestDocEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}

func (e *parseTestDocEngine) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
	return nil, nil
}

func (e *parseTestDocEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	return nil
}

func (e *parseTestDocEngine) DeleteChunks(_ context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	e.deleteChunksCalls++
	e.deleteChunksCondition = copyMap(condition)
	e.deleteIndexName = indexName
	e.deleteDatasetID = datasetID
	return e.deleteChunksCount, e.deleteChunksErr
}

func (e *parseTestDocEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return nil, nil
}

func (e *parseTestDocEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}

func (e *parseTestDocEngine) DropChunkStore(context.Context, string, string) error {
	return nil
}

func (e *parseTestDocEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (e *parseTestDocEngine) CreateMetadataStore(context.Context, string) error {
	return nil
}

func (e *parseTestDocEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}

func (e *parseTestDocEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}

func (e *parseTestDocEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}

func (e *parseTestDocEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}

func (e *parseTestDocEngine) DropMetadataStore(context.Context, string) error {
	return nil
}

func (e *parseTestDocEngine) MetadataStoreExists(context.Context, string) (bool, error) {
	return false, nil
}

func (e *parseTestDocEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}

func (e *parseTestDocEngine) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}

func (e *parseTestDocEngine) DeleteDocument(context.Context, string, string) error {
	return nil
}

func (e *parseTestDocEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}

func (e *parseTestDocEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}

func (e *parseTestDocEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}

func (e *parseTestDocEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}

func (e *parseTestDocEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (e *parseTestDocEngine) GetChunkIDs([]map[string]interface{}) []string {
	return nil
}

type addChunkTestEngine struct {
	parseTestDocEngine
	insertedChunks []map[string]interface{}
	insertIndex    string
	insertDataset  string
	insertErr      error
}

func (e *addChunkTestEngine) InsertChunks(_ context.Context, chunks []map[string]interface{}, baseName string, datasetID string) ([]string, error) {
	e.insertedChunks = chunks
	e.insertIndex = baseName
	e.insertDataset = datasetID
	return nil, e.insertErr
}

type chunkImageStorage struct {
	exists    bool
	oldBinary []byte
	putCalls  int
}

func (s *chunkImageStorage) Health() bool { return true }
func (s *chunkImageStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	s.putCalls++
	return nil
}
func (s *chunkImageStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	return s.oldBinary, nil
}
func (s *chunkImageStorage) Remove(bucket, fnm string, tenantID ...string) error  { return nil }
func (s *chunkImageStorage) ObjExist(bucket, fnm string, tenantID ...string) bool { return s.exists }
func (s *chunkImageStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	return "", nil
}
func (s *chunkImageStorage) BucketExists(bucket string) bool                           { return true }
func (s *chunkImageStorage) RemoveBucket(bucket string) error                          { return nil }
func (s *chunkImageStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool { return false }
func (s *chunkImageStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool { return false }

func mustEncodePNG(t *testing.T, rect image.Rectangle) []byte {
	t.Helper()

	img := image.NewRGBA(rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.Set(x, y, color.White)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

type stubEmbeddingDriver struct {
	embeddings []models.EmbeddingData
	embedErr   error
}

func (d *stubEmbeddingDriver) NewInstance(map[string]string) models.ModelDriver { return d }
func (d *stubEmbeddingDriver) Name() string                                     { return "stub" }
func (d *stubEmbeddingDriver) ChatWithMessages(string, []models.Message, *models.APIConfig, *models.ChatConfig) (*models.ChatResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) ChatStreamlyWithSender(string, []models.Message, *models.APIConfig, *models.ChatConfig, func(*string, *string) error) error {
	return nil
}
func (d *stubEmbeddingDriver) Embed(*string, []string, *models.APIConfig, *models.EmbeddingConfig) ([]models.EmbeddingData, error) {
	return d.embeddings, d.embedErr
}
func (d *stubEmbeddingDriver) Rerank(*string, string, []string, *models.APIConfig, *models.RerankConfig) (*models.RerankResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) TranscribeAudio(*string, *string, *models.APIConfig, *models.ASRConfig) (*models.ASRResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) TranscribeAudioWithSender(*string, *string, *models.APIConfig, *models.ASRConfig, func(*string, *string) error) error {
	return nil
}
func (d *stubEmbeddingDriver) AudioSpeech(*string, *string, *models.APIConfig, *models.TTSConfig) (*models.TTSResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) AudioSpeechWithSender(*string, *string, *models.APIConfig, *models.TTSConfig, func(*string, *string) error) error {
	return nil
}
func (d *stubEmbeddingDriver) OCRFile(*string, []byte, *string, *models.APIConfig, *models.OCRConfig) (*models.OCRFileResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) ParseFile(*string, []byte, *string, *models.APIConfig, *models.ParseFileConfig) (*models.ParseFileResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) ListModels(*models.APIConfig) ([]models.ListModelResponse, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) Balance(*models.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) CheckConnection(*models.APIConfig) error { return nil }
func (d *stubEmbeddingDriver) ListTasks(*models.APIConfig) ([]models.ListTaskStatus, error) {
	return nil, nil
}
func (d *stubEmbeddingDriver) ShowTask(string, *models.APIConfig) (*models.TaskResponse, error) {
	return nil, nil
}

func strPtr(v string) *string {
	return &v
}

func (e *parseTestDocEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}

func (e *parseTestDocEngine) GetScores(map[string]interface{}) map[string]float64 {
	return nil
}

func (e *parseTestDocEngine) Ping(context.Context) error {
	return nil
}

func (e *parseTestDocEngine) Close() error {
	return nil
}

func (e *parseTestDocEngine) GetType() string {
	return "test"
}

func (e *parseTestDocEngine) FilterDocIdsByMetaPushdown(context.Context, []string, []map[string]interface{}, string) []string {
	return nil
}

func TestSwitchChunksUpdatesDocEngineWithAvailableInt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}, &entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("failed to migrate sqlite: %v", err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })

	valid := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID:       "ut-1",
		UserID:   "user-1",
		TenantID: "tenant-1",
		Role:     "owner",
		Status:   &valid,
	}).Error; err != nil {
		t.Fatalf("failed to create user_tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		Name:         "dataset",
		EmbdID:       "embed",
		Permission:   string(entity.TenantPermissionMe),
		CreatedBy:    "user-1",
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{},
		Status:       &valid,
	}).Error; err != nil {
		t.Fatalf("failed to create knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "doc",
		CreatedBy:    "user-1",
		Suffix:       "txt",
	}).Error; err != nil {
		t.Fatalf("failed to create document: %v", err)
	}

	engine := &switchChunksEngineMock{}
	svc := &ChunkService{
		docEngine:     engine,
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}

	if err := svc.SwitchChunks("user-1", "kb-1", "doc-1", 0, []string{"chunk-1", "chunk-2"}); err != nil {
		t.Fatalf("SwitchChunks() error = %v", err)
	}

	if len(engine.updateCalls) != 2 {
		t.Fatalf("UpdateChunks calls = %d, want 2", len(engine.updateCalls))
	}
	for i, call := range engine.updateCalls {
		if call.indexName != "ragflow_tenant-1" {
			t.Fatalf("call %d indexName = %q", i, call.indexName)
		}
		if call.datasetID != "kb-1" {
			t.Fatalf("call %d datasetID = %q", i, call.datasetID)
		}
		wantID := []string{"chunk-1", "chunk-2"}[i]
		if !reflect.DeepEqual(call.condition, map[string]interface{}{
			"id":     wantID,
			"doc_id": "doc-1",
		}) {
			t.Fatalf("call %d condition = %#v", i, call.condition)
		}
		if !reflect.DeepEqual(call.newValue, map[string]interface{}{"id": wantID, "available_int": 0}) {
			t.Fatalf("call %d newValue = %#v", i, call.newValue)
		}
	}
}

type updateChunksCall struct {
	condition map[string]interface{}
	newValue  map[string]interface{}
	indexName string
	datasetID string
}

type switchChunksEngineMock struct {
	updateCalls []updateChunksCall
}

func (m *switchChunksEngineMock) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (m *switchChunksEngineMock) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) UpdateChunks(_ context.Context, condition map[string]interface{}, newValue map[string]interface{}, indexName string, datasetID string) error {
	m.updateCalls = append(m.updateCalls, updateChunksCall{
		condition: copyMap(condition),
		newValue:  copyMap(newValue),
		indexName: indexName,
		datasetID: datasetID,
	})
	return nil
}
func (m *switchChunksEngineMock) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	return 0, nil
}
func (m *switchChunksEngineMock) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) DropChunkStore(context.Context, string, string) error { return nil }
func (m *switchChunksEngineMock) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (m *switchChunksEngineMock) CreateMetadataStore(context.Context, string) error { return nil }
func (m *switchChunksEngineMock) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (m *switchChunksEngineMock) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (m *switchChunksEngineMock) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (m *switchChunksEngineMock) DropMetadataStore(context.Context, string) error { return nil }
func (m *switchChunksEngineMock) MetadataStoreExists(context.Context, string) (bool, error) {
	return false, nil
}
func (m *switchChunksEngineMock) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}
func (m *switchChunksEngineMock) DeleteDocument(context.Context, string, string) error { return nil }
func (m *switchChunksEngineMock) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (m *switchChunksEngineMock) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (m *switchChunksEngineMock) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (m *switchChunksEngineMock) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (m *switchChunksEngineMock) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (m *switchChunksEngineMock) GetScores(map[string]interface{}) map[string]float64 { return nil }
func (m *switchChunksEngineMock) Ping(context.Context) error                          { return nil }
func (m *switchChunksEngineMock) Close() error                                        { return nil }
func (m *switchChunksEngineMock) GetType() string                                     { return "elasticsearch" }
func (m *switchChunksEngineMock) FilterDocIdsByMetaPushdown(context.Context, []string, []map[string]interface{}, string) []string {
	return nil
}

func copyMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
