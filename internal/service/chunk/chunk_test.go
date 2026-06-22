package chunk

import (
	"context"
	"errors"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"reflect"
	"strings"
	"testing"

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

func setupChunkTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
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
	deleteChunksErr   error
	deleteChunksCalls int
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

func (e *parseTestDocEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	e.deleteChunksCalls++
	return 0, e.deleteChunksErr
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
