package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

type fakePromptTransformDriver struct {
	responses []string
	messages  [][]modelModule.Message
}

func (f *fakePromptTransformDriver) NewInstance(baseURL map[string]string) modelModule.ModelDriver {
	return f
}

func (f *fakePromptTransformDriver) Name() string {
	return "fake"
}

func (f *fakePromptTransformDriver) ChatWithMessages(modelName string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, chatModelConfig *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	f.messages = append(f.messages, messages)
	if len(f.responses) == 0 {
		return nil, errors.New("no fake response configured")
	}
	answer := f.responses[0]
	f.responses = f.responses[1:]
	return &modelModule.ChatResponse{Answer: &answer}, nil
}

func (f *fakePromptTransformDriver) ChatStreamlyWithSender(modelName string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) error {
	return nil
}

func (f *fakePromptTransformDriver) Embed(modelName *string, texts []string, apiConfig *modelModule.APIConfig, embeddingConfig *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) Rerank(modelName *string, query string, documents []string, apiConfig *modelModule.APIConfig, rerankConfig *modelModule.RerankConfig) (*modelModule.RerankResponse, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) TranscribeAudio(modelName *string, file *string, apiConfig *modelModule.APIConfig, asrConfig *modelModule.ASRConfig) (*modelModule.ASRResponse, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *modelModule.APIConfig, asrConfig *modelModule.ASRConfig, sender func(*string, *string) error) error {
	return nil
}

func (f *fakePromptTransformDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *modelModule.APIConfig, ttsConfig *modelModule.TTSConfig) (*modelModule.TTSResponse, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *modelModule.APIConfig, ttsConfig *modelModule.TTSConfig, sender func(*string, *string) error) error {
	return nil
}

func (f *fakePromptTransformDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *modelModule.APIConfig, ocrConfig *modelModule.OCRConfig) (*modelModule.OCRFileResponse, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *modelModule.APIConfig, parseFileConfig *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) ListModels(apiConfig *modelModule.APIConfig) ([]modelModule.ListModelResponse, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) Balance(apiConfig *modelModule.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) CheckConnection(apiConfig *modelModule.APIConfig) error {
	return nil
}

func (f *fakePromptTransformDriver) ListTasks(apiConfig *modelModule.APIConfig) ([]modelModule.ListTaskStatus, error) {
	return nil, nil
}

func (f *fakePromptTransformDriver) ShowTask(taskID string, apiConfig *modelModule.APIConfig) (*modelModule.TaskResponse, error) {
	return nil, nil
}

func newFakeChatModel(driver *fakePromptTransformDriver) *modelModule.ChatModel {
	modelName := "fake-chat"
	return modelModule.NewChatModel(driver, &modelName, &modelModule.APIConfig{})
}

func TestApplyPromptTransformsRefinesTranslatesAndAddsKeywords(t *testing.T) {
	driver := &fakePromptTransformDriver{
		responses: []string{
			"standalone follow-up question",
			"translated follow-up question",
			"alpha,beta",
		},
	}
	dialog := &entity.Chat{
		PromptConfig: entity.JSONMap{
			"refine_multiturn": true,
			"cross_languages":  []interface{}{"English", "Chinese"},
			"keyword":          true,
		},
	}
	messages := []map[string]interface{}{
		{"role": "user", "content": "What is RAGFlow?"},
		{"role": "assistant", "content": "RAGFlow is a RAG engine."},
		{"role": "user", "content": "Can it translate this?"},
	}

	transformed := applyPromptTransforms(context.Background(), dialog, messages, newFakeChatModel(driver))

	if got := transformed[2]["content"]; got != "translated follow-up question,alpha,beta" {
		t.Fatalf("unexpected transformed question: %#v", got)
	}
	if got := messages[2]["content"]; got != "Can it translate this?" {
		t.Fatalf("original messages were mutated: %#v", got)
	}
	if len(driver.messages) != 3 {
		t.Fatalf("expected three LLM transform calls, got %d", len(driver.messages))
	}
	if !strings.Contains(driver.messages[0][0].Content.(string), "USER: What is RAGFlow?") {
		t.Fatalf("full question prompt did not include conversation: %#v", driver.messages[0][0].Content)
	}
}

func TestApplyPromptTransformsPreservesMultimodalContent(t *testing.T) {
	driver := &fakePromptTransformDriver{responses: []string{"translated text"}}
	dialog := &entity.Chat{
		PromptConfig: entity.JSONMap{"cross_languages": []interface{}{"English"}},
	}
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "original text"},
				map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/image.png"}},
			},
		},
	}

	transformed := applyPromptTransforms(context.Background(), dialog, messages, newFakeChatModel(driver))

	blocks, ok := transformed[0]["content"].([]interface{})
	if !ok {
		t.Fatalf("expected multimodal content blocks, got %#v", transformed[0]["content"])
	}
	textBlock, _ := blocks[0].(map[string]interface{})
	if textBlock["text"] != "translated text" {
		t.Fatalf("expected transformed text block, got %#v", textBlock)
	}
	if imageBlock, _ := blocks[1].(map[string]interface{}); imageBlock["type"] != "image_url" {
		t.Fatalf("expected image block to be preserved, got %#v", blocks[1])
	}
}

func TestFullQuestionFallsBackOnErrorResponse(t *testing.T) {
	driver := &fakePromptTransformDriver{responses: []string{"**ERROR** failed"}}
	messages := []map[string]interface{}{
		{"role": "user", "content": "First question"},
		{"role": "assistant", "content": "First answer"},
		{"role": "user", "content": "Follow-up?"},
	}

	question, err := FullQuestion(context.Background(), newFakeChatModel(driver), messages, "")
	if err != nil {
		t.Fatalf("FullQuestion returned error: %v", err)
	}
	if question != "Follow-up?" {
		t.Fatalf("expected fallback latest user question, got %q", question)
	}
}

func TestFullQuestionFallsBackOnIncompleteModel(t *testing.T) {
	modelName := "fake-chat"
	emptyModelName := "   "
	messages := []map[string]interface{}{
		{"role": "user", "content": "First question"},
		{"role": "assistant", "content": "First answer"},
		{"role": "user", "content": "Follow-up?"},
	}

	tests := []struct {
		name      string
		chatModel *modelModule.ChatModel
	}{
		{name: "nil model"},
		{name: "nil driver", chatModel: &modelModule.ChatModel{ModelName: &modelName}},
		{name: "nil name", chatModel: &modelModule.ChatModel{ModelDriver: &fakePromptTransformDriver{}}},
		{name: "empty name", chatModel: &modelModule.ChatModel{ModelDriver: &fakePromptTransformDriver{}, ModelName: &emptyModelName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question, err := FullQuestion(context.Background(), tt.chatModel, messages, "")
			if err != nil {
				t.Fatalf("FullQuestion returned error: %v", err)
			}
			if question != "Follow-up?" {
				t.Fatalf("expected fallback latest user question, got %q", question)
			}
		})
	}
}

func TestApplyPromptTransformsFallsBackOnIncompleteModel(t *testing.T) {
	modelName := "fake-chat"
	emptyModelName := "   "
	dialog := &entity.Chat{
		PromptConfig: entity.JSONMap{
			"refine_multiturn": true,
			"cross_languages":  []interface{}{"English"},
			"keyword":          true,
		},
	}
	messages := []map[string]interface{}{
		{"role": "user", "content": "First question"},
		{"role": "assistant", "content": "First answer"},
		{"role": "user", "content": "Follow-up?"},
	}

	tests := []struct {
		name      string
		chatModel *modelModule.ChatModel
	}{
		{name: "nil model"},
		{name: "nil driver", chatModel: &modelModule.ChatModel{ModelName: &modelName}},
		{name: "nil name", chatModel: &modelModule.ChatModel{ModelDriver: &fakePromptTransformDriver{}}},
		{name: "empty name", chatModel: &modelModule.ChatModel{ModelDriver: &fakePromptTransformDriver{}, ModelName: &emptyModelName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformed := applyPromptTransforms(context.Background(), dialog, messages, tt.chatModel)
			if transformed[2]["content"] != "Follow-up?" {
				t.Fatalf("expected latest question fallback, got %#v", transformed[2]["content"])
			}
		})
	}
}
