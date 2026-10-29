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

// ObscurifyModel drives Obscurify.ai (https://obscurify.ai/v1), a
// privacy-focused provider whose API exposes OpenAI-compatible chat and
// model-list endpoints, so the embedded OpenAIAPICompatibleModel covers
// every call.
type ObscurifyModel struct {
	*OpenAIAPICompatibleModel
}

// NewObscurifyModel creates an Obscurify.ai driver.
func NewObscurifyModel(baseURL map[string]string, urlSuffix URLSuffix) *ObscurifyModel {
	return &ObscurifyModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *ObscurifyModel) Name() string {
	return "obscurify"
}

// NewInstance returns a driver bound to the given base URL.
func (m *ObscurifyModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewObscurifyModel(baseURL, m.baseModel.URLSuffix)
}
