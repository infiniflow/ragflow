package chunk

import (
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/service"
)

func TestUpdateChunkEmbeddingInput(t *testing.T) {
	cases := []struct {
		name          string
		storedContent string
		contentField  string
		storedQueries interface{}
		content       *string
		questions     []string
		unrelated     bool
		noTitle       bool
		wantEmbed     bool
		wantText      string
		wantQueries   []string
	}{
		{
			name: "content changes without questions", storedContent: "old content",
			content: strPtr("new content"), wantEmbed: true, wantText: "new content",
		},
		{
			name: "blank stored questions fall back to content", storedContent: "old content",
			storedQueries: []string{" ", ""}, content: strPtr("new content"), wantEmbed: true, wantText: "new content",
		},
		{
			name: "legacy content field is preserved", storedContent: "old content", contentField: "content",
			questions: []string{" new question "}, wantEmbed: true, wantText: "new question", wantQueries: []string{"new question"},
		},
		{
			name: "questions change while omitted content is preserved", storedContent: "old content",
			storedQueries: []string{"old question"}, questions: []string{" new question ", " ", "second"},
			wantEmbed: true, wantText: "new question\nsecond", wantQueries: []string{"new question", "second"},
		},
		{
			name: "explicit empty questions fall back to content", storedContent: "old content",
			storedQueries: []string{"old question"}, questions: []string{}, wantEmbed: true, wantText: "old content", wantQueries: []string{},
		},
		{
			name: "explicit empty questions clear interface slice", storedContent: "old content",
			storedQueries: []interface{}{"old question"}, questions: []string{}, wantEmbed: true, wantText: "old content", wantQueries: []string{},
		},
		{
			name: "blank new questions fall back to updated content", storedContent: "old content",
			storedQueries: []string{"old question"}, content: strPtr("new content"), questions: []string{" "},
			wantEmbed: true, wantText: "new content", wantQueries: []string{},
		},
		{
			name: "empty content is a real replacement", storedContent: "old content", content: strPtr(""), wantEmbed: true,
		},
		{
			name: "missing document name uses empty title", storedContent: "old content",
			content: strPtr("new content"), noTitle: true, wantEmbed: true, wantText: "new content",
		},
		{
			name: "omitted questions preserve stored string slice", storedContent: "old content",
			storedQueries: []string{" saved ", " ", "question"}, content: strPtr("new content"),
		},
		{
			name: "omitted questions preserve stored interface slice", storedContent: "old content",
			storedQueries: []interface{}{" saved ", " ", "question"}, content: strPtr("new content"),
		},
		{
			name: "unchanged content skips embedding", storedContent: "old content", content: strPtr("old content"),
		},
		{
			name: "trim equivalent questions skip embedding", storedContent: "old content",
			storedQueries: []interface{}{"saved", "question"}, questions: []string{" saved ", " ", "question "},
			wantQueries: []string{"saved", "question"},
		},
		{
			name: "questions equal previous content skip embedding", storedContent: "saved\nquestion",
			questions: []string{"saved", "question"}, wantQueries: []string{"saved", "question"},
		},
		{
			name: "cleared questions equal content skip embedding", storedContent: "saved\nquestion",
			storedQueries: []string{"saved", "question"}, questions: []string{}, wantQueries: []string{},
		},
		{
			name: "empty questions with none stored skip embedding", storedContent: "old content",
			questions: []string{}, wantQueries: []string{},
		},
		{
			name: "unrelated fields skip embedding", storedContent: "old content", unrelated: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, engine, driver := newUpdateChunkEmbeddingTestService(t)
			if tc.noTitle {
				if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-a").Update("name", nil).Error; err != nil {
					t.Fatalf("clear document name: %v", err)
				}
			}
			contentField := tc.contentField
			if contentField == "" {
				contentField = "content_with_weight"
			}
			engine.existingChunk = map[string]interface{}{
				"doc_id": "doc-a", contentField: tc.storedContent,
				"question_kwd": tc.storedQueries, "q_3_vec": []float64{1, 1, 1},
			}
			modelCalls := 0
			svc.getEmbeddingModelFunc = func(tenantID, modelID string) (*models.EmbeddingModel, error) {
				modelCalls++
				if tenantID != "tenant-1" || modelID != "embedding-model" {
					t.Fatalf("embedding model = %q/%q, want tenant-1/embedding-model", tenantID, modelID)
				}
				return models.NewEmbeddingModel(driver, nil, &models.APIConfig{}, 0), nil
			}
			req := &service.UpdateChunkRequest{
				DatasetID: "kb-1", DocumentID: "doc-a", ChunkID: "chunk-1", Content: tc.content, Questions: tc.questions,
			}
			if tc.unrelated {
				available := false
				req.Available = &available
				req.Positions = []interface{}{[]interface{}{1, 2, 3, 4, 5}}
				req.TagFeas = map[string]interface{}{"tag": float64(0.5)}
				req.ImportantKwd = []string{"keyword"}
			}
			if err := svc.UpdateChunk(t.Context(), req, "user-1"); err != nil {
				t.Fatalf("UpdateChunk() error = %v", err)
			}
			if len(engine.updateCalls) != 1 {
				t.Fatalf("UpdateChunks calls = %d, want 1", len(engine.updateCalls))
			}
			call := engine.updateCalls[0]
			if !reflect.DeepEqual(call.condition, map[string]interface{}{"id": "chunk-1", "doc_id": "doc-a"}) || call.indexName != "ragflow_tenant-1" || call.datasetID != "kb-1" {
				t.Fatalf("unexpected update target: %#v", call)
			}
			wantContent := tc.storedContent
			if tc.content != nil {
				wantContent = *tc.content
			}
			if call.newValue["content_with_weight"] != wantContent {
				t.Fatalf("content = %#v, want %q", call.newValue["content_with_weight"], wantContent)
			}
			if tc.questions == nil {
				if _, ok := call.newValue["question_kwd"]; ok {
					t.Fatal("omitted questions must preserve stored question_kwd")
				}
			} else if !reflect.DeepEqual(call.newValue["question_kwd"], tc.wantQueries) {
				t.Fatalf("question_kwd = %#v, want %#v", call.newValue["question_kwd"], tc.wantQueries)
			}
			if !tc.wantEmbed {
				if modelCalls != 0 || len(driver.embedRequests) != 0 {
					t.Fatalf("model lookups=%d embedding calls=%d, want 0", modelCalls, len(driver.embedRequests))
				}
				for key := range call.newValue {
					if strings.HasPrefix(key, "q_") && strings.HasSuffix(key, "_vec") {
						t.Fatalf("unchanged embedding input must not update vector %q", key)
					}
				}
				return
			}
			if modelCalls != 1 || len(driver.embedRequests) != 1 {
				t.Fatalf("model lookups=%d embedding calls=%d, want 1", modelCalls, len(driver.embedRequests))
			}
			title := "doc-a.txt"
			if tc.noTitle {
				title = ""
			}
			if got := driver.embedRequests[0].Texts; !reflect.DeepEqual(got, []string{title, tc.wantText}) {
				t.Fatalf("embedding texts = %#v, want title and %q", got, tc.wantText)
			}
			if len(driver.embedConfigs) != 1 || driver.embedConfigs[0].Dimension != 0 {
				t.Fatalf("embedding config = %#v, want Dimension 0", driver.embedConfigs)
			}
			if got := call.newValue["q_3_vec"]; !reflect.DeepEqual(got, []float64{28, 38, 48}) {
				t.Fatalf("weighted vector = %#v, want [28 38 48]", got)
			}
		})
	}
}

func TestUpdateChunkEmbeddingFailureDoesNotWrite(t *testing.T) {
	cases := []struct {
		name       string
		modelErr   error
		embedErr   error
		embeddings []models.EmbeddingData
		wantError  string
	}{
		{name: "model lookup", modelErr: errors.New("model unavailable"), wantError: "model unavailable"},
		{name: "provider error", embedErr: errors.New("provider unavailable"), wantError: "provider unavailable"},
		{name: "zero vectors", wantError: "unexpected embedding count"},
		{name: "one vector", embeddings: []models.EmbeddingData{{Embedding: []float64{1, 2}}}, wantError: "unexpected embedding count"},
		{name: "three vectors", embeddings: []models.EmbeddingData{{Embedding: []float64{1, 2}}, {Embedding: []float64{3, 4}}, {Embedding: []float64{5, 6}}}, wantError: "unexpected embedding count"},
		{name: "empty dimensions", embeddings: []models.EmbeddingData{{}, {}}, wantError: "unexpected embedding dimensions"},
		{name: "unequal dimensions", embeddings: []models.EmbeddingData{{Embedding: []float64{1, 2}}, {Embedding: []float64{3}}}, wantError: "unexpected embedding dimensions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, engine, driver := newUpdateChunkEmbeddingTestService(t)
			driver.embedErr, driver.embeddings = tc.embedErr, tc.embeddings
			svc.getEmbeddingModelFunc = func(string, string) (*models.EmbeddingModel, error) {
				if tc.modelErr != nil {
					return nil, tc.modelErr
				}
				return models.NewEmbeddingModel(driver, nil, &models.APIConfig{}, 0), nil
			}
			storageCalls, wikiCalls := 0, 0
			svc.storeChunkImageFunc = func(string, string, []byte, string) error { storageCalls++; return nil }
			svc.markWikiDirtyFunc = func(string, string, string, []string) { wikiCalls++ }
			imageBase64 := base64.StdEncoding.EncodeToString([]byte("image-bytes"))
			err := svc.UpdateChunk(t.Context(), &service.UpdateChunkRequest{
				DatasetID: "kb-1", DocumentID: "doc-a", ChunkID: "chunk-1",
				Content: strPtr("new content"), ImageBase64: &imageBase64,
			}, "user-1")
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("UpdateChunk() error = %v, want %q", err, tc.wantError)
			}
			if len(engine.updateCalls) != 0 || storageCalls != 0 || wikiCalls != 0 {
				t.Fatalf("index writes=%d image stores=%d wiki refreshes=%d, want 0", len(engine.updateCalls), storageCalls, wikiCalls)
			}
		})
	}
}

func TestUpdateChunkEmbeddingPrecedesImageStorage(t *testing.T) {
	svc, engine, driver := newUpdateChunkEmbeddingTestService(t)
	storageCalls := 0
	svc.storeChunkImageFunc = func(string, string, []byte, string) error {
		if len(driver.embedRequests) != 1 || len(engine.updateCalls) != 0 {
			t.Fatalf("before image store: embedding calls=%d index writes=%d, want 1/0", len(driver.embedRequests), len(engine.updateCalls))
		}
		storageCalls++
		return nil
	}
	imageBase64 := base64.StdEncoding.EncodeToString([]byte("image-bytes"))
	if err := svc.UpdateChunk(t.Context(), &service.UpdateChunkRequest{
		DatasetID: "kb-1", DocumentID: "doc-a", ChunkID: "chunk-1",
		Content: strPtr("new content"), ImageBase64: &imageBase64,
	}, "user-1"); err != nil {
		t.Fatalf("UpdateChunk() error = %v", err)
	}
	if storageCalls != 1 || len(engine.updateCalls) != 1 {
		t.Fatalf("image stores=%d index writes=%d, want 1/1", storageCalls, len(engine.updateCalls))
	}
	if got := engine.updateCalls[0].newValue["q_3_vec"]; !reflect.DeepEqual(got, []float64{28, 38, 48}) {
		t.Fatalf("weighted vector = %#v, want [28 38 48]", got)
	}
}

func TestUpdateChunkEmbeddingPreservesDocumentAuthorization(t *testing.T) {
	cases := []struct {
		name       string
		datasetID  string
		documentID string
		chunkDocID string
		wantError  string
	}{
		{name: "chunk belongs to another document", datasetID: "kb-1", documentID: "doc-a", chunkDocID: "doc-b", wantError: "chunk not found"},
		{name: "document belongs to another dataset", datasetID: "kb-1", documentID: "doc-b", chunkDocID: "doc-b", wantError: "document does not belong to this dataset"},
		{name: "dataset is inaccessible", datasetID: "kb-2", documentID: "doc-b", chunkDocID: "doc-b", wantError: "user does not have access to this dataset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, engine, driver := newUpdateChunkEmbeddingTestService(t)
			insertChunkTestKB(t, "kb-2", "tenant-2")
			insertChunkTestDoc(t, "doc-b", "kb-2")
			engine.existingChunk.(map[string]interface{})["doc_id"] = tc.chunkDocID
			svc.getEmbeddingModelFunc = func(string, string) (*models.EmbeddingModel, error) {
				t.Fatal("authorization failure must not look up an embedding model")
				return nil, nil
			}
			err := svc.UpdateChunk(t.Context(), &service.UpdateChunkRequest{
				DatasetID: tc.datasetID, DocumentID: tc.documentID, ChunkID: "chunk-1", Content: strPtr("new content"),
			}, "user-1")
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("UpdateChunk() error = %v, want %q", err, tc.wantError)
			}
			if len(engine.updateCalls) != 0 || len(driver.embedRequests) != 0 {
				t.Fatalf("index writes=%d embedding calls=%d, want 0", len(engine.updateCalls), len(driver.embedRequests))
			}
		})
	}
}

func newUpdateChunkEmbeddingTestService(t *testing.T) (*ChunkService, *updateChunkTestEngine, *stubEmbeddingDriver) {
	t.Helper()
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")
	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id": "doc-a", "content_with_weight": "old content", "q_3_vec": []float64{1, 1, 1},
	}}
	driver := &stubEmbeddingDriver{embeddings: []models.EmbeddingData{
		{Embedding: []float64{10, 20, 30}}, {Embedding: []float64{30, 40, 50}},
	}}
	svc := &ChunkService{
		docEngine: engine, kbDAO: dao.NewKnowledgebaseDAO(), userTenantDAO: dao.NewUserTenantDAO(),
		getEmbeddingModelFunc: func(string, string) (*models.EmbeddingModel, error) {
			return models.NewEmbeddingModel(driver, nil, &models.APIConfig{}, 0), nil
		},
		markWikiDirtyFunc: func(string, string, string, []string) {},
	}
	return svc, engine, driver
}
