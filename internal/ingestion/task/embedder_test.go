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
	"ragflow/internal/common"
	"testing"

	"ragflow/internal/entity/models"
)

func makeEmbeddingModelForResolver() *models.EmbeddingModel {
	return models.NewEmbeddingModel(&stubDriver{}, strPtr("embed"), &models.APIConfig{}, 128)
}

func TestEmbedderResolver_UsesKBEmbdID(t *testing.T) {
	var gotTenantID, gotEmbdID string
	resolver := newEmbedderResolver(
		func(kbID string) (string, error) {
			return "kb-embd-1", nil
		},
		func(tenantID, embdID string) (*models.EmbeddingModel, error) {
			gotTenantID, gotEmbdID = tenantID, embdID
			return makeEmbeddingModelForResolver(), nil
		},
	)
	emb, err := resolver("tenant-1", "kb-1", "should-be-ignored")
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if emb == nil {
		t.Fatal("expected embedder")
	}
	if gotTenantID != "tenant-1" || gotEmbdID != "kb-embd-1" {
		t.Fatalf("resolver args = (%q, %q), want (tenant-1, kb-embd-1)", gotTenantID, gotEmbdID)
	}
}

func TestEmbedderResolver_EmptyKBEmbdIDReturnsNil(t *testing.T) {
	resolver := newEmbedderResolver(
		func(kbID string) (string, error) {
			return "", nil
		},
		func(string, string) (*models.EmbeddingModel, error) {
			t.Fatal("model resolver should not be called when kb embd_id is empty")
			return nil, nil
		},
	)
	emb, err := resolver("tenant-1", "kb-1", "ignored")
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if emb != nil {
		t.Fatal("expected nil embedder when kb has no embd_id")
	}
}

// stubDriver records the texts passed to Embed for verification.
type stubDriver struct {
	capturedTexts []string
}

func (d *stubDriver) Embed(modelName *string, texts []string, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig, usage *common.ModelUsage) ([]models.EmbeddingData, error) {
	d.capturedTexts = texts
	result := make([]models.EmbeddingData, len(texts))
	for i := range texts {
		result[i] = models.EmbeddingData{
			Embedding:  []float64{float64(i), 0.1},
			Index:      i,
			TokenCount: len(texts[i]),
		}
	}
	return result, nil
}
func (d *stubDriver) NewInstance(baseURL map[string]string) models.ModelDriver { return d }
func (d *stubDriver) Name() string                                             { return "stub" }
func (d *stubDriver) ChatWithMessages(modelName string, messages []models.Message, apiConfig *models.APIConfig, chatModelConfig *models.ChatConfig, usage *common.ModelUsage) (*models.ChatResponse, error) {
	return nil, nil
}
func (d *stubDriver) ChatStreamlyWithSender(modelName string, messages []models.Message, apiConfig *models.APIConfig, modelConfig *models.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) Rerank(modelName *string, query string, documents []string, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig, usage *common.ModelUsage) (*models.RerankResponse, error) {
	return nil, nil
}
func (d *stubDriver) TranscribeAudio(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, usage *common.ModelUsage) (*models.ASRResponse, error) {
	return nil, nil
}
func (d *stubDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, usage *common.ModelUsage) (*models.TTSResponse, error) {
	return nil, nil
}
func (d *stubDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, ocrConfig *models.OCRConfig, usage *common.ModelUsage) (*models.OCRFileResponse, error) {
	return nil, nil
}
func (d *stubDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, parseFileConfig *models.ParseFileConfig, usage *common.ModelUsage) (*models.ParseFileResponse, error) {
	return nil, nil
}
func (d *stubDriver) ListModels(apiConfig *models.APIConfig) ([]models.ListModelResponse, error) {
	return nil, nil
}
func (d *stubDriver) Balance(apiConfig *models.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}
func (d *stubDriver) CheckConnection(apiConfig *models.APIConfig) error { return nil }
func (d *stubDriver) ListTasks(apiConfig *models.APIConfig) ([]models.ListTaskStatus, error) {
	return nil, nil
}
func (d *stubDriver) ShowTask(taskID string, apiConfig *models.APIConfig) (*models.TaskResponse, error) {
	return nil, nil
}
func (d *stubDriver) ToolCall(name string, arguments map[string]interface{}) (string, error) {
	return "", nil
}
