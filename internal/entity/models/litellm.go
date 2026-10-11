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

// LiteLLMModel drives a self-hosted LiteLLM Proxy gateway. The proxy speaks the
// OpenAI wire format, so the embedded OpenAIAPICompatibleModel covers every
// call. This driver talks to the proxy over HTTP and does not use the LiteLLM
// Python library.
type LiteLLMModel struct {
	*OpenAIAPICompatibleModel
}

// NewLiteLLMModel creates a LiteLLM Proxy driver.
func NewLiteLLMModel(baseURL map[string]string, urlSuffix URLSuffix) *LiteLLMModel {
	return &LiteLLMModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *LiteLLMModel) Name() string {
	return "litellm"
}

// NewInstance returns a driver bound to the given base URL.
func (m *LiteLLMModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewLiteLLMModel(baseURL, m.baseModel.URLSuffix)
}
