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
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

func makeEmbeddingModelForResolver() *models.EmbeddingModel {
	return models.NewEmbeddingModel(&stubDriver{}, strPtr("embed"), &models.APIConfig{}, 128)
}

func TestEmbedderResolver_ExplicitEmbeddingModelWins(t *testing.T) {
	var gotTenantID, gotEmbdID string
	resolver := newEmbedderResolver(
		func(tenantID, embdID string) (*models.EmbeddingModel, error) {
			gotTenantID, gotEmbdID = tenantID, embdID
			return makeEmbeddingModelForResolver(), nil
		},
		func(string) (*entity.Knowledgebase, error) {
			t.Fatal("kb lookup should not run when embedding_model is set")
			return nil, nil
		},
	)
	emb, err := resolver("tenant-1", "kb-1", "explicit-embd")
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if emb == nil {
		t.Fatal("expected embedder")
	}
	if gotTenantID != "tenant-1" || gotEmbdID != "explicit-embd" {
		t.Fatalf("resolver args = (%q, %q), want (tenant-1, explicit-embd)", gotTenantID, gotEmbdID)
	}
}

func TestEmbedderResolver_FallsBackToDatasetEmbedding(t *testing.T) {
	var gotEmbdID string
	resolver := newEmbedderResolver(
		func(_ string, embdID string) (*models.EmbeddingModel, error) {
			gotEmbdID = embdID
			return makeEmbeddingModelForResolver(), nil
		},
		func(kbID string) (*entity.Knowledgebase, error) {
			if kbID != "kb-1" {
				t.Fatalf("kb lookup id = %q, want kb-1", kbID)
			}
			return &entity.Knowledgebase{ID: "kb-1", EmbdID: "lookup-embd"}, nil
		},
	)
	if _, err := resolver("tenant-1", "kb-1", ""); err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if gotEmbdID != "lookup-embd" {
		t.Fatalf("got embd id %q, want lookup-embd", gotEmbdID)
	}
}

func TestEmbedderResolver_MissingDatasetEmbeddingReturnsError(t *testing.T) {
	resolver := newEmbedderResolver(
		func(string, string) (*models.EmbeddingModel, error) {
			t.Fatal("model resolver should not be called")
			return nil, nil
		},
		func(string) (*entity.Knowledgebase, error) {
			return &entity.Knowledgebase{ID: "kb-1", EmbdID: ""}, nil
		},
	)
	_, err := resolver("tenant-1", "kb-1", "")
	if err == nil {
		t.Fatal("expected error when dataset embd_id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "dataset has no embd_id configured") {
		t.Fatalf("err = %v, want dataset has no embd_id configured", err)
	}
}

func TestEmbedderResolver_MissingEmbeddingModelAndKBReturnsError(t *testing.T) {
	resolver := newEmbedderResolver(
		func(string, string) (*models.EmbeddingModel, error) {
			t.Fatal("model resolver should not be called")
			return nil, nil
		},
		func(string) (*entity.Knowledgebase, error) {
			t.Fatal("kb lookup should not be called without a kb_id")
			return nil, nil
		},
	)
	_, err := resolver("tenant-1", "", "")
	if err == nil {
		t.Fatal("expected error when neither embedding_model nor kb_id provided")
	}
	if !strings.Contains(err.Error(), "neither embedding_model nor kb_id") {
		t.Fatalf("err = %v, want neither embedding_model nor kb_id", err)
	}
}

// stubDriver records the texts passed to Embed for verification.
type stubDriver struct {
	capturedTexts []string
}

func (d *stubDriver) Embed(modelName *string, texts []string, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig) ([]models.EmbeddingData, error) {
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
func (d *stubDriver) ChatWithMessages(modelName string, messages []models.Message, apiConfig *models.APIConfig, chatModelConfig *models.ChatConfig) (*models.ChatResponse, error) {
	return nil, nil
}
func (d *stubDriver) ChatStreamlyWithSender(modelName string, messages []models.Message, apiConfig *models.APIConfig, modelConfig *models.ChatConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) Rerank(modelName *string, query string, documents []string, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig) (*models.RerankResponse, error) {
	return nil, nil
}
func (d *stubDriver) TranscribeAudio(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig) (*models.ASRResponse, error) {
	return nil, nil
}
func (d *stubDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig) (*models.TTSResponse, error) {
	return nil, nil
}
func (d *stubDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, ocrConfig *models.OCRConfig) (*models.OCRFileResponse, error) {
	return nil, nil
}
func (d *stubDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, parseFileConfig *models.ParseFileConfig) (*models.ParseFileResponse, error) {
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
