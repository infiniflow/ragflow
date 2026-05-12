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

// DummyModel implements ModelDriver for Dummy AI
type DummyModel struct {
	BaseURL   map[string]string
	URLSuffix URLSuffix
}

func (d *DummyModel) ParseFile() {
	//TODO implement me
	panic("implement me")
}

// NewDummyModel creates a new Dummy AI model instance
func NewDummyModel(baseURL map[string]string, urlSuffix URLSuffix) *DummyModel {
	return &DummyModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
	}
}

func (d *DummyModel) NewInstance(baseURL map[string]string) ModelDriver {
	return nil
}

func (d *DummyModel) Name() string {
	return "dummy"
}

// ChatWithMessages sends multiple messages with roles and returns response
func (d *DummyModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (d *DummyModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}

// Embed embeds a list of texts into embeddings
func (d *DummyModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *DummyModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *DummyModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (d *DummyModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("no such method")
}

// Rerank calculates similarity scores between query and documents
func (d *DummyModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", d.Name())
}

// TranscribeAudio transcribe audio
func (d *DummyModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (z *DummyModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert audio to text
func (d *DummyModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (z *DummyModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (d *DummyModel) OCRFile(modelName *string, fileContent *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}
