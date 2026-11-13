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
	"context"
	"net/http"

	"ragflow/internal/common"
)

// ShengsuanyunModel adapts Shengsuanyun's OpenAI-compatible API and tags its
// outbound requests for provider attribution.
type ShengsuanyunModel struct {
	*OpenAIAPICompatibleModel
}

// NewShengsuanyunModel creates a Shengsuanyun model driver.
func NewShengsuanyunModel(baseURL map[string]string, urlSuffix URLSuffix) *ShengsuanyunModel {
	compatible := NewOpenAIAPICompatibleModel(baseURL, urlSuffix)
	compatible.VllmModel.baseModel.httpClient = withShengsuanyunTitle(compatible.VllmModel.baseModel.httpClient)
	return &ShengsuanyunModel{OpenAIAPICompatibleModel: compatible}
}

// Name returns the provider identifier.
func (m *ShengsuanyunModel) Name() string {
	return "Shengsuanyun"
}

// NewInstance creates a driver for the supplied base URL and retains the
// Shengsuanyun attribution header.
func (m *ShengsuanyunModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewShengsuanyunModel(baseURL, m.VllmModel.baseModel.URLSuffix)
}

// Embed sends embeddings through Shengsuanyun's OpenAI-compatible
// /embeddings endpoint using the URL suffix configured for this provider.
func (m *ShengsuanyunModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return m.OpenAIAPICompatibleModel.Embed(ctx, modelName, request, apiConfig, embeddingConfig, modelUsage)
}

func withShengsuanyunTitle(client *http.Client) *http.Client {
	clientWithTitle := *client
	transport := clientWithTitle.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clientWithTitle.Transport = shengsuanyunTitleTransport{base: transport}
	return &clientWithTitle
}

type shengsuanyunTitleTransport struct {
	base http.RoundTripper
}

func (t shengsuanyunTitleTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	requestWithTitle := request.Clone(request.Context())
	requestWithTitle.Header = request.Header.Clone()
	if requestWithTitle.Header == nil {
		requestWithTitle.Header = make(http.Header)
	}
	requestWithTitle.Header.Set("X-Title", "ragflow")
	return t.base.RoundTrip(requestWithTitle)
}
