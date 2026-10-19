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

// YAPIModel implements Y-API's OpenAI-compatible chat and model-list APIs.
// Y-API speaks the OpenAI wire format without deviation, so the embedded
// OpenAIAPICompatibleModel covers every call and nothing is overridden beyond
// the driver's own identity. The catalog is account-scoped, so model listing
// reads GET /v1/models from the gateway instead of a static list.
type YAPIModel struct {
	*OpenAIAPICompatibleModel
}

// NewYAPIModel creates a Y-API model driver.
func NewYAPIModel(baseURL map[string]string, urlSuffix URLSuffix) *YAPIModel {
	return &YAPIModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the model driver name.
func (m *YAPIModel) Name() string {
	return "Y-API"
}

// NewInstance creates a new Y-API driver bound to the given base URL.
func (m *YAPIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewYAPIModel(baseURL, m.baseModel.URLSuffix)
}
