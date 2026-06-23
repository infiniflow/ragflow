package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/nlp"
)

type fakeChatKBStore struct {
	kbs        []*entity.Knowledgebase
	accessible map[string]bool
}

func (f fakeChatKBStore) Accessible(kbID, userID string) bool {
	if f.accessible == nil {
		return true
	}
	return f.accessible[kbID]
}

func (f fakeChatKBStore) GetByIDs(ids []string) ([]*entity.Knowledgebase, error) {
	return f.kbs, nil
}

type fakeChatMetadataService struct{}

func (fakeChatMetadataService) LabelQuestion(question string, kbs []*entity.Knowledgebase) map[string]float64 {
	return map[string]float64{"pagerank_fea": 10}
}

func (fakeChatMetadataService) GetFlattedMetaByKBs(kbIDs []string) (common.MetaData, error) {
	return common.MetaData{
		"category": common.MetaValueDocs{
			"policy": []string{"doc-policy"},
		},
	}, nil
}

type failingChatMetadataService struct{}

func (failingChatMetadataService) LabelQuestion(question string, kbs []*entity.Knowledgebase) map[string]float64 {
	return nil
}

func (failingChatMetadataService) GetFlattedMetaByKBs(kbIDs []string) (common.MetaData, error) {
	return nil, errors.New("metadata unavailable")
}

type fakeChatDocEngine struct {
	chunk map[string]interface{}
}

func (f fakeChatDocEngine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error {
	return nil
}

func (f fakeChatDocEngine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string) ([]string, error) {
	return nil, nil
}

func (f fakeChatDocEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	return nil
}

func (f fakeChatDocEngine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName string, datasetID string) (int64, error) {
	return 0, nil
}

func (f fakeChatDocEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return nil, nil
}

func (f fakeChatDocEngine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	return f.chunk, nil
}

func (f fakeChatDocEngine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	return nil
}

func (f fakeChatDocEngine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return true, nil
}

func (f fakeChatDocEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	return nil
}

func (f fakeChatDocEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	return nil, nil
}

func (f fakeChatDocEngine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	return nil
}

func (f fakeChatDocEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	return 0, nil
}

func (f fakeChatDocEngine) DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	return nil
}

func (f fakeChatDocEngine) DropMetadataStore(ctx context.Context, tenantID string) error {
	return nil
}

func (f fakeChatDocEngine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	return true, nil
}

func (f fakeChatDocEngine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}

func (f fakeChatDocEngine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	return nil
}

func (f fakeChatDocEngine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	return nil
}

func (f fakeChatDocEngine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	return nil, nil
}

func (f fakeChatDocEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	return nil
}

func (f fakeChatDocEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	return nil
}

func (f fakeChatDocEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	return nil
}

func (f fakeChatDocEngine) GetChunkIDs(chunks []map[string]interface{}) []string {
	return nil
}

func (f fakeChatDocEngine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	return nil, nil
}

func (f fakeChatDocEngine) GetScores(searchResult map[string]interface{}) map[string]float64 {
	return nil
}

func (f fakeChatDocEngine) FilterDocIdsByMetaPushdown(ctx context.Context, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	return nil
}

func (f fakeChatDocEngine) Ping(ctx context.Context) error {
	return nil
}

func (f fakeChatDocEngine) Close() error {
	return nil
}

func (f fakeChatDocEngine) GetType() string {
	return "fake"
}

type fakeChatRetrievalService struct {
	req    *nlp.RetrievalRequest
	result *nlp.RetrievalResult
}

func (f *fakeChatRetrievalService) Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error) {
	f.req = req
	return f.result, nil
}

type fakeChatModelProvider struct {
	driver *fakeChatModelDriver
}

func (f fakeChatModelProvider) GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error) {
	modelName := compositeModelName
	return modelModule.NewChatModel(f.driver, &modelName, &modelModule.APIConfig{}), nil
}

func (f fakeChatModelProvider) GetEmbeddingModel(tenantID, compositeModelName string) (*modelModule.EmbeddingModel, error) {
	modelName := compositeModelName
	return modelModule.NewEmbeddingModel(f.driver, &modelName, &modelModule.APIConfig{}, 512), nil
}

func (f fakeChatModelProvider) GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error) {
	modelName := compositeModelName
	return modelModule.NewRerankModel(f.driver, &modelName, &modelModule.APIConfig{}), nil
}

func (f fakeChatModelProvider) GetModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	return f.driver, modelName, &modelModule.APIConfig{}, 0, nil
}

func (f fakeChatModelProvider) GetTenantDefaultModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelName := "default@factory"
	return f.driver, modelName, &modelModule.APIConfig{}, 0, nil
}

type fakeChatModelDriver struct {
	messages []modelModule.Message
}

func (f *fakeChatModelDriver) NewInstance(baseURL map[string]string) modelModule.ModelDriver {
	return f
}

func (f *fakeChatModelDriver) Name() string {
	return "fake"
}

func (f *fakeChatModelDriver) ChatWithMessages(modelName string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, chatModelConfig *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	f.messages = messages
	answer := "answer from knowledge"
	return &modelModule.ChatResponse{Answer: &answer}, nil
}

func (f *fakeChatModelDriver) ChatStreamlyWithSender(modelName string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) error {
	f.messages = messages
	answer := "stream answer from knowledge"
	return sender(&answer, nil)
}

func (f *fakeChatModelDriver) Embed(modelName *string, texts []string, apiConfig *modelModule.APIConfig, embeddingConfig *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) Rerank(modelName *string, query string, documents []string, apiConfig *modelModule.APIConfig, rerankConfig *modelModule.RerankConfig) (*modelModule.RerankResponse, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) TranscribeAudio(modelName *string, file *string, apiConfig *modelModule.APIConfig, asrConfig *modelModule.ASRConfig) (*modelModule.ASRResponse, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *modelModule.APIConfig, asrConfig *modelModule.ASRConfig, sender func(*string, *string) error) error {
	return nil
}

func (f *fakeChatModelDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *modelModule.APIConfig, ttsConfig *modelModule.TTSConfig) (*modelModule.TTSResponse, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *modelModule.APIConfig, ttsConfig *modelModule.TTSConfig, sender func(*string, *string) error) error {
	return nil
}

func (f *fakeChatModelDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *modelModule.APIConfig, ocrConfig *modelModule.OCRConfig) (*modelModule.OCRFileResponse, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *modelModule.APIConfig, parseFileConfig *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) ListModels(apiConfig *modelModule.APIConfig) ([]modelModule.ListModelResponse, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) Balance(apiConfig *modelModule.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) CheckConnection(apiConfig *modelModule.APIConfig) error {
	return nil
}

func (f *fakeChatModelDriver) ListTasks(apiConfig *modelModule.APIConfig) ([]modelModule.ListTaskStatus, error) {
	return nil, nil
}

func (f *fakeChatModelDriver) ShowTask(taskID string, apiConfig *modelModule.APIConfig) (*modelModule.TaskResponse, error) {
	return nil, nil
}

func TestAsyncChatUsesRetrievedKnowledgeForKBDialog(t *testing.T) {
	driver := &fakeChatModelDriver{}
	retrieval := &fakeChatRetrievalService{
		result: &nlp.RetrievalResult{
			Chunks: []map[string]interface{}{
				{
					"chunk_id":            "chunk-1",
					"content_with_weight": "RAGFlow stores conversation references alongside the session.",
					"doc_id":              "doc-1",
					"docnm_kwd":           "manual.md",
					"vector":              []float64{0.1, 0.2},
				},
			},
			DocAggs: []map[string]interface{}{
				{"doc_id": "doc-1", "doc_name": "manual.md", "count": 1},
			},
		},
	}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: driver},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	reference := []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}}
	sessionMessage, err := json.Marshal(map[string]interface{}{"messages": []interface{}{}})
	if err != nil {
		t.Fatalf("failed to marshal session message: %v", err)
	}
	session := &entity.ChatSession{ID: "session-1", Message: sessionMessage}
	dialog := &entity.Chat{
		ID:                     "dialog-1",
		TenantID:               "tenant-1",
		LLMID:                  "chat@factory",
		PromptConfig:           entity.JSONMap{"system": "You are helpful."},
		LLMSetting:             entity.JSONMap{},
		KBIDs:                  entity.JSONSlice{"kb-1"},
		TopN:                   3,
		TopK:                   32,
		SimilarityThreshold:    0.2,
		VectorSimilarityWeight: 0.3,
	}

	result, err := svc.asyncChat("user-1", dialog, session, []map[string]interface{}{
		{"role": "user", "content": "Where are references stored?"},
	}, nil, "message-1", reference, false)
	if err != nil {
		t.Fatalf("asyncChat returned error: %v", err)
	}

	if retrieval.req == nil {
		t.Fatal("expected retrieval service to be called")
	}
	if retrieval.req.Question != "Where are references stored?" {
		t.Fatalf("unexpected retrieval question: %q", retrieval.req.Question)
	}
	if retrieval.req.PageSize != 3 || retrieval.req.Top == nil || *retrieval.req.Top != 32 {
		t.Fatalf("unexpected retrieval paging: page_size=%d top=%v", retrieval.req.PageSize, retrieval.req.Top)
	}
	if len(driver.messages) == 0 {
		t.Fatal("expected chat model to receive messages")
	}
	last := driver.messages[len(driver.messages)-1]
	content, ok := last.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", last.Content)
	}
	if !strings.Contains(content, "RAGFlow stores conversation references") {
		t.Fatalf("expected retrieved content in prompt, got %q", content)
	}

	ref, ok := result["reference"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reference map, got %T", result["reference"])
	}
	chunks, ok := ref["chunks"].([]interface{})
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected one reference chunk, got %#v", ref["chunks"])
	}
	chunk, ok := chunks[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected chunk map, got %T", chunks[0])
	}
	if _, exists := chunk["vector"]; exists {
		t.Fatal("reference chunk should not expose vector")
	}
	if result["answer"] != "answer from knowledge" {
		t.Fatalf("unexpected answer: %#v", result["answer"])
	}
}

func TestAsyncChatPropagatesRetrievalErrors(t *testing.T) {
	retrievalErr := errors.New("search unavailable")
	retrieval := &failingChatRetrievalService{err: retrievalErr}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	_, err := svc.asyncChat("user-1", &entity.Chat{
		ID:                     "dialog-1",
		TenantID:               "tenant-1",
		LLMID:                  "chat@factory",
		PromptConfig:           entity.JSONMap{},
		LLMSetting:             entity.JSONMap{},
		KBIDs:                  entity.JSONSlice{"kb-1"},
		TopN:                   3,
		TopK:                   32,
		SimilarityThreshold:    0.2,
		VectorSimilarityWeight: 0.3,
	}, &entity.ChatSession{ID: "session-1"}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, nil, "message-1", []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}}, false)
	if err == nil || !strings.Contains(err.Error(), "retrieval search failed") {
		t.Fatalf("expected retrieval error, got %v", err)
	}
}

func TestMessagesWithRetrievedKnowledgeFillsSystemPlaceholder(t *testing.T) {
	retrieval := &fakeChatRetrievalService{
		result: &nlp.RetrievalResult{
			Chunks: []map[string]interface{}{
				{"content_with_weight": "Knowledge inserted into the system prompt."},
			},
		},
	}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	dialog := &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{"system": "Answer from this context: {knowledge}"},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}
	messages := []map[string]interface{}{
		{"role": "user", "content": "What context is available?"},
	}

	got, ragDialog, emptyResponse, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", dialog, messages, []interface{}{
		map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
	})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if emptyResponse != nil {
		t.Fatalf("expected no empty response, got %q", *emptyResponse)
	}
	if got[0]["content"] != "What context is available?" {
		t.Fatalf("expected user content to stay unchanged, got %q", got[0]["content"])
	}
	originalPrompt, _ := dialog.PromptConfig["system"].(string)
	if !strings.Contains(originalPrompt, "{knowledge}") {
		t.Fatalf("expected original dialog prompt to remain unchanged, got %q", originalPrompt)
	}
	systemPrompt, _ := ragDialog.PromptConfig["system"].(string)
	if strings.Contains(systemPrompt, "{knowledge}") {
		t.Fatalf("expected knowledge placeholder to be replaced, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Knowledge inserted into the system prompt.") {
		t.Fatalf("expected retrieved knowledge in system prompt, got %q", systemPrompt)
	}
}

func TestAsyncChatReturnsEmptyResponseWhenRetrievalHasNoKnowledge(t *testing.T) {
	driver := &fakeChatModelDriver{}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: driver},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     &fakeChatRetrievalService{result: &nlp.RetrievalResult{}},
	}
	reference := []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}}
	sessionMessage, err := json.Marshal(map[string]interface{}{"messages": []interface{}{}})
	if err != nil {
		t.Fatalf("failed to marshal session message: %v", err)
	}
	result, err := svc.asyncChat("user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		LLMID:        "chat@factory",
		PromptConfig: entity.JSONMap{"empty_response": "No relevant content."},
		LLMSetting:   entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}, &entity.ChatSession{ID: "session-1", Message: sessionMessage}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, nil, "message-1", reference, false)
	if err != nil {
		t.Fatalf("asyncChat returned error: %v", err)
	}
	if result["answer"] != "No relevant content." {
		t.Fatalf("unexpected empty response answer: %#v", result["answer"])
	}
	if len(driver.messages) != 0 {
		t.Fatal("chat model should not be called when empty_response is returned")
	}
}

func TestMessagesWithRetrievedKnowledgeAppliesMetadataFilter(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	filter := entity.JSONMap{
		"method": "manual",
		"manual": []interface{}{
			map[string]interface{}{"key": "category", "op": "=", "value": "policy"},
		},
		"logic": "and",
	}
	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:             "dialog-1",
		TenantID:       "tenant-1",
		PromptConfig:   entity.JSONMap{},
		MetaDataFilter: &filter,
		KBIDs:          entity.JSONSlice{"kb-1"},
		TopN:           3,
		TopK:           32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if retrieval.req == nil {
		t.Fatal("expected retrieval to be called")
	}
	if len(retrieval.req.DocIDs) != 1 || retrieval.req.DocIDs[0] != "doc-policy" {
		t.Fatalf("expected metadata-filtered doc id, got %#v", retrieval.req.DocIDs)
	}
}

func TestMessagesWithRetrievedKnowledgeIntersectsDocIDsWithMetadataFilter(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	filter := entity.JSONMap{
		"method": "manual",
		"manual": []interface{}{
			map[string]interface{}{"key": "category", "op": "=", "value": "policy"},
		},
		"logic": "and",
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:             "dialog-1",
		TenantID:       "tenant-1",
		PromptConfig:   entity.JSONMap{},
		MetaDataFilter: &filter,
		KBIDs:          entity.JSONSlice{"kb-1"},
		TopN:           3,
		TopK:           32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question", "doc_ids": []interface{}{"doc-explicit", "doc-policy"}},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if len(retrieval.req.DocIDs) != 1 || retrieval.req.DocIDs[0] != "doc-policy" {
		t.Fatalf("expected metadata and message doc_ids intersection, got %#v", retrieval.req.DocIDs)
	}
}

func TestMessagesWithRetrievedKnowledgeNoMetadataIntersectionUsesSentinel(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	filter := entity.JSONMap{
		"method": "manual",
		"manual": []interface{}{
			map[string]interface{}{"key": "category", "op": "=", "value": "policy"},
		},
		"logic": "and",
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:             "dialog-1",
		TenantID:       "tenant-1",
		PromptConfig:   entity.JSONMap{},
		MetaDataFilter: &filter,
		KBIDs:          entity.JSONSlice{"kb-1"},
		TopN:           3,
		TopK:           32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question", "doc_ids": []interface{}{"doc-explicit"}},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if len(retrieval.req.DocIDs) != 1 || retrieval.req.DocIDs[0] != NoMatchDocIDSentinel {
		t.Fatalf("expected empty metadata/doc_ids intersection sentinel, got %#v", retrieval.req.DocIDs)
	}
}

func TestMessagesWithRetrievedKnowledgePreservesEmptyMetadataFilterMatches(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	filter := entity.JSONMap{"method": "auto"}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:             "dialog-1",
		TenantID:       "tenant-1",
		LLMID:          "chat@factory",
		PromptConfig:   entity.JSONMap{},
		MetaDataFilter: &filter,
		KBIDs:          entity.JSONSlice{"kb-1"},
		TopN:           3,
		TopK:           32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if len(retrieval.req.DocIDs) != 1 || retrieval.req.DocIDs[0] != NoMatchDocIDSentinel {
		t.Fatalf("expected empty metadata filter sentinel, got %#v", retrieval.req.DocIDs)
	}
}

func TestMessagesWithRetrievedKnowledgeFailsClosedWhenMetadataUnavailable(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      failingChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	filter := entity.JSONMap{
		"method": "manual",
		"manual": []interface{}{
			map[string]interface{}{"key": "category", "op": "=", "value": "policy"},
		},
		"logic": "and",
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:             "dialog-1",
		TenantID:       "tenant-1",
		PromptConfig:   entity.JSONMap{},
		MetaDataFilter: &filter,
		KBIDs:          entity.JSONSlice{"kb-1"},
		TopN:           3,
		TopK:           32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question", "doc_ids": []interface{}{"doc-explicit"}},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err == nil || !strings.Contains(err.Error(), "flattened metadata") {
		t.Fatalf("expected metadata filter error, got %v", err)
	}
	if retrieval.req != nil {
		t.Fatal("retrieval should not run when metadata filtering cannot be evaluated")
	}
}

func TestMessagesWithRetrievedKnowledgeExpandsChildChunks(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{
		Chunks: []map[string]interface{}{
			{
				"chunk_id":            "child-1",
				"mom_id":              "parent-1",
				"kb_id":               "kb-1",
				"doc_id":              "doc-1",
				"docnm_kwd":           "doc.md",
				"content_ltks":        "child tokens",
				"content_with_weight": "child-only passage",
				"similarity":          0.8,
			},
		},
	}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		docEngine: fakeChatDocEngine{chunk: map[string]interface{}{
			"doc_id":              "doc-1",
			"docnm_kwd":           "doc.md",
			"kb_id":               "kb-1",
			"content_with_weight": "parent passage with surrounding context",
			"position_int":        []interface{}{1},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	ragMessages, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	content, _ := ragMessages[0]["content"].(string)
	if !strings.Contains(content, "parent passage with surrounding context") {
		t.Fatalf("expected expanded parent content in prompt, got %q", content)
	}
	if strings.Contains(content, "child-only passage") {
		t.Fatalf("expected child content to be replaced by expanded parent content, got %q", content)
	}
}

func TestMessagesWithRetrievedKnowledgeRejectsCrossTenantKnowledgebase(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{
			kbs: []*entity.Knowledgebase{
				{ID: "kb-1", TenantID: "tenant-2", Name: "Manual", EmbdID: "embed@factory"},
			},
			accessible: map[string]bool{"kb-1": false},
		},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("expected cross-tenant authorization error, got %v", err)
	}
	if retrieval.req != nil {
		t.Fatal("retrieval should not be called for an unauthorized knowledge base")
	}
}

func TestMessagesWithRetrievedKnowledgeAllowsAccessibleSharedKnowledgebase(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{
			kbs: []*entity.Knowledgebase{
				{ID: "kb-1", TenantID: "tenant-2", Name: "Shared Manual", EmbdID: "embed@factory"},
			},
			accessible: map[string]bool{"kb-1": true},
		},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if retrieval.req == nil || len(retrieval.req.TenantIDs) != 1 || retrieval.req.TenantIDs[0] != "tenant-2" {
		t.Fatalf("expected retrieval to use shared KB tenant, got %#v", retrieval.req)
	}
}

func TestMessagesWithRetrievedKnowledgeRejectsMixedEmbeddingModels(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed-a@factory"},
			{ID: "kb-2", TenantID: "tenant-1", Name: "FAQ", EmbdID: "embed-b@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1", "kb-2"},
		TopN:         3,
		TopK:         32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question"},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err == nil || !strings.Contains(err.Error(), "same embedding model") {
		t.Fatalf("expected mixed embedding model error, got %v", err)
	}
	if retrieval.req != nil {
		t.Fatal("retrieval should not run when knowledge bases use different embedding models")
	}
}

func TestValidateKnowledgebaseEmbeddingModelsComparesResolvedNames(t *testing.T) {
	firstTenantEmbdID := int64(1)
	secondTenantEmbdID := int64(2)
	kbs := []*entity.Knowledgebase{
		{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "same-legacy-name", TenantEmbdID: &firstTenantEmbdID},
		{ID: "kb-2", TenantID: "tenant-1", Name: "FAQ", EmbdID: "same-legacy-name", TenantEmbdID: &secondTenantEmbdID},
	}
	resolver := func(tenantID string, kb *entity.Knowledgebase) (string, error) {
		if kb.TenantEmbdID != nil && *kb.TenantEmbdID == firstTenantEmbdID {
			return "embed-a@factory", nil
		}
		return "embed-b@factory", nil
	}

	_, _, err := validateKnowledgebaseEmbeddingModels(kbs, "tenant-1", resolver)
	if err == nil || !strings.Contains(err.Error(), "same embedding model") {
		t.Fatalf("expected resolved mixed embedding model error, got %v", err)
	}
}

func TestMessagesWithRetrievedKnowledgePreservesMultimodalContent(t *testing.T) {
	retrieval := &fakeChatRetrievalService{
		result: &nlp.RetrievalResult{
			Chunks: []map[string]interface{}{
				{"content_with_weight": "Knowledge for an image question."},
			},
		},
	}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}
	imageBlock := map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/cat.png"}}
	messages := []map[string]interface{}{
		{"role": "user", "content": []interface{}{
			map[string]interface{}{"type": "text", "text": "What is in this image?"},
			imageBlock,
		}},
	}

	got, _, emptyResponse, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}, messages, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if emptyResponse != nil {
		t.Fatalf("expected no empty response, got %q", *emptyResponse)
	}
	content, ok := got[0]["content"].([]interface{})
	if !ok {
		t.Fatalf("expected multimodal content to stay as blocks, got %T", got[0]["content"])
	}
	if len(content) != 3 {
		t.Fatalf("expected injected text plus original blocks, got %#v", content)
	}
	injected, ok := content[0].(map[string]interface{})
	if !ok || injected["type"] != "text" || !strings.Contains(injected["text"].(string), "Knowledge for an image question.") {
		t.Fatalf("expected injected knowledge text block, got %#v", content[0])
	}
	preservedImage, ok := content[2].(map[string]interface{})
	if !ok || preservedImage["type"] != "image_url" {
		t.Fatalf("expected original image block to be preserved, got %#v", content[2])
	}
	if retrieval.req == nil || retrieval.req.Question != "What is in this image?" {
		t.Fatalf("expected retrieval question from text block, got %#v", retrieval.req)
	}
}

func TestMessagesWithRetrievedKnowledgePassesMessageDocIDs(t *testing.T) {
	retrieval := &fakeChatRetrievalService{result: &nlp.RetrievalResult{}}
	svc := &ChatSessionService{
		kbDAO: fakeChatKBStore{kbs: []*entity.Knowledgebase{
			{ID: "kb-1", TenantID: "tenant-1", Name: "Manual", EmbdID: "embed@factory"},
		}},
		modelProviderSvc: fakeChatModelProvider{driver: &fakeChatModelDriver{}},
		metadataSvc:      fakeChatMetadataService{},
		retrievalSvc:     retrieval,
	}

	_, _, _, err := svc.messagesWithRetrievedKnowledge(context.Background(), "user-1", &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-1",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{"kb-1"},
		TopN:         3,
		TopK:         32,
	}, []map[string]interface{}{
		{"role": "user", "content": "question", "doc_ids": []interface{}{"doc-1", "doc-2", "doc-1"}},
	}, []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})
	if err != nil {
		t.Fatalf("messagesWithRetrievedKnowledge returned error: %v", err)
	}
	if len(retrieval.req.DocIDs) != 2 || retrieval.req.DocIDs[0] != "doc-1" || retrieval.req.DocIDs[1] != "doc-2" {
		t.Fatalf("expected scoped doc ids, got %#v", retrieval.req.DocIDs)
	}
}

type failingChatRetrievalService struct {
	err error
}

func (f *failingChatRetrievalService) Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error) {
	return nil, f.err
}
