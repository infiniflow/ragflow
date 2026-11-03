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

// NewAPIModel drives New API (https://github.com/QuantumNous/new-api) and
// compatible aggregation gateways such as one-api. These gateways expose every
// upstream model behind a single OpenAI-compatible endpoint, so the embedded
// OpenAIAPICompatibleModel covers chat, embedding, rerank, TTS and ASR calls as
// well as dynamic model discovery via /models.
type NewAPIModel struct {
	*OpenAIAPICompatibleModel
}

// NewNewAPIModel creates a New API driver.
func NewNewAPIModel(baseURL map[string]string, urlSuffix URLSuffix) *NewAPIModel {
	return &NewAPIModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *NewAPIModel) Name() string {
	return "New API"
}

// NewInstance returns a driver bound to the given base URL.
func (m *NewAPIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNewAPIModel(baseURL, m.baseModel.URLSuffix)
}
