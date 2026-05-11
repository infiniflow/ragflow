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

package models

import (
	"fmt"
)

// DummyModel implements ModelDriver for Zhipu AI
type DummyModel struct {
	BaseURL   map[string]string
	URLSuffix URLSuffix
}

// NewDummyModel creates a new Zhipu AI model instance
func NewDummyModel(baseURL map[string]string, urlSuffix URLSuffix) *DummyModel {
	return &DummyModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
	}
}

// Chat sends a message and returns response
func (z *DummyModel) Chat(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *DummyModel) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}

// EncodeToEmbedding encodes a list of texts into embeddings
func (z *DummyModel) EncodeToEmbedding(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	return nil, fmt.Errorf("not implemented")
}

func (z *DummyModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
