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

// LLMTRModel drives LLMTR (https://llmtr.com/v1), an OpenAI-compatible gateway
// for models hosted in Turkey and models from other vendors, so the embedded
// OpenAIAPICompatibleModel covers every call.
type LLMTRModel struct {
	*OpenAIAPICompatibleModel
}

// NewLLMTRModel creates an LLMTR driver.
func NewLLMTRModel(baseURL map[string]string, urlSuffix URLSuffix) *LLMTRModel {
	return &LLMTRModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *LLMTRModel) Name() string {
	return "llmtr"
}

// NewInstance returns a driver bound to the given base URL.
func (m *LLMTRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewLLMTRModel(baseURL, m.baseModel.URLSuffix)
}
