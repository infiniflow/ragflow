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

// TrustedRouterModel drives TrustedRouter, an OpenAI-compatible LLM router
// (https://api.trustedrouter.com/v1) that serves models from many providers
// under provider-prefixed ids such as "openai/gpt-5.5", so the embedded
// OpenAIAPICompatibleModel covers every call.
type TrustedRouterModel struct {
	*OpenAIAPICompatibleModel
}

// NewTrustedRouterModel creates a TrustedRouter driver.
func NewTrustedRouterModel(baseURL map[string]string, urlSuffix URLSuffix) *TrustedRouterModel {
	return &TrustedRouterModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *TrustedRouterModel) Name() string {
	return "trustedrouter"
}

// NewInstance returns a driver bound to the given base URL.
func (m *TrustedRouterModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewTrustedRouterModel(baseURL, m.baseModel.URLSuffix)
}
