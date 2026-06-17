package chunk

import (
	"context"
	"ragflow/internal/common"
	"ragflow/internal/dao"
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
