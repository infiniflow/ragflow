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

package models

// NewAPIModel implements ModelDriver for New API / one-api compatible gateways.
// It delegates all operations to OpenAIModel since the gateway exposes an
// OpenAI-compatible /v1 API.
type NewAPIModel struct {
	inner *OpenAIModel
}

// NewNewAPIModel creates a new NewAPIModel instance.
func NewNewAPIModel(baseURL map[string]string, urlSuffix URLSuffix) *NewAPIModel {
	return &NewAPIModel{
		inner: NewOpenAIModel(baseURL, urlSuffix),
	}
}

func (n *NewAPIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNewAPIModel(baseURL, n.inner.baseModel.URLSuffix)
}

func (n *NewAPIModel) Name() string {
	return "new api"
}

func (n *NewAPIModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return n.inner.ChatWithMessages(modelName, messages, apiConfig, chatModelConfig)
}

func (n *NewAPIModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return n.inner.ChatStreamlyWithSender(modelName, messages, apiConfig, modelConfig, sender)
}

func (n *NewAPIModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return n.inner.Embed(modelName, texts, apiConfig, embeddingConfig)
}

func (n *NewAPIModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return n.inner.Rerank(modelName, query, documents, apiConfig, rerankConfig)
}

func (n *NewAPIModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return n.inner.TranscribeAudio(modelName, file, apiConfig, asrConfig)
}

func (n *NewAPIModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return n.inner.TranscribeAudioWithSender(modelName, file, apiConfig, asrConfig, sender)
}

func (n *NewAPIModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return n.inner.AudioSpeech(modelName, audioContent, apiConfig, ttsConfig)
}

func (n *NewAPIModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return n.inner.AudioSpeechWithSender(modelName, audioContent, apiConfig, ttsConfig, sender)
}

func (n *NewAPIModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return n.inner.OCRFile(modelName, content, url, apiConfig, ocrConfig)
}

func (n *NewAPIModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return n.inner.ParseFile(modelName, content, url, apiConfig, parseFileConfig)
}

func (n *NewAPIModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	return n.inner.ListModels(apiConfig)
}

func (n *NewAPIModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return n.inner.Balance(apiConfig)
}

func (n *NewAPIModel) CheckConnection(apiConfig *APIConfig) error {
	return n.inner.CheckConnection(apiConfig)
}

func (n *NewAPIModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return n.inner.ListTasks(apiConfig)
}

func (n *NewAPIModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return n.inner.ShowTask(taskID, apiConfig)
}
