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

// RadeonCloudModel uses Radeon Cloud's OpenAI-compatible API.
type RadeonCloudModel struct {
	*OpenAIAPICompatibleModel
}

// NewRadeonCloudModel creates a Radeon Cloud model driver.
func NewRadeonCloudModel(baseURL map[string]string, urlSuffix URLSuffix) *RadeonCloudModel {
	model := NewOpenAIAPICompatibleModel(baseURL, urlSuffix)
	model.baseModel.AllowEmptyAPIKey = false
	return &RadeonCloudModel{OpenAIAPICompatibleModel: model}
}

// Name returns the provider name.
func (m *RadeonCloudModel) Name() string {
	return "Radeon Cloud"
}

// NewInstance binds a Radeon Cloud driver to the supplied base URL.
func (m *RadeonCloudModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewRadeonCloudModel(baseURL, m.baseModel.URLSuffix)
}
