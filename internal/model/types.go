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

package model

// ModelType represents the type of model
type ModelType string

const (
	// ModelTypeChat chat model
	ModelTypeChat ModelType = "chat"
	// ModelTypeEmbedding embedding model
	ModelTypeEmbedding ModelType = "embedding"
	// ModelTypeSpeech2Text speech to text model
	ModelTypeSpeech2Text ModelType = "speech2text"
	// ModelTypeImage2Text image to text model
	ModelTypeImage2Text ModelType = "image2text"
	// ModelTypeRerank rerank model
	ModelTypeRerank ModelType = "rerank"
	// ModelTypeTTS text to speech model
	ModelTypeTTS ModelType = "tts"
	// ModelTypeOCR optical character recognition model
	ModelTypeOCR ModelType = "ocr"
)

// EmbeddingModel interface for embedding models
type EmbeddingModel interface {
	// Encode encodes a list of texts into embeddings
	Encode(texts []string) ([][]float64, error)
	// EncodeQuery encodes a single query string into embedding
	EncodeQuery(query string) ([]float64, error)
}

// ChatModel interface for chat models
type ChatModel interface {
	// Chat sends a message and returns response
	Chat(system string, history []map[string]string, genConf map[string]interface{}) (string, error)
	// ChatStreamly sends a message and streams response
	ChatStreamly(system string, history []map[string]string, genConf map[string]interface{}) (<-chan string, error)
}

// RerankModel interface for rerank models
type RerankModel interface {
	// Similarity calculates similarity between query and texts
	Similarity(query string, texts []string) ([]float64, error)
}

// ModelConfig represents configuration for a model
type ModelConfig struct {
	TenantID   string    `json:"tenant_id"`
	LLMFactory string    `json:"llm_factory"`
	ModelType  ModelType `json:"model_type"`
	LLMName    string    `json:"llm_name"`
	APIKey     string    `json:"api_key"`
	APIBase    string    `json:"api_base"`
	MaxTokens  int64     `json:"max_tokens"`
	IsTools    bool      `json:"is_tools"`
}
