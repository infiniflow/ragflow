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

// AnonRouterModel drives AnonRouter, a hosted gateway that relays models from
// several labs behind one OpenAI-compatible endpoint, so the embedded
// OpenAIAPICompatibleModel covers every call.
type AnonRouterModel struct {
	*OpenAIAPICompatibleModel
}

// NewAnonRouterModel creates an AnonRouter driver.
func NewAnonRouterModel(baseURL map[string]string, urlSuffix URLSuffix) *AnonRouterModel {
	return &AnonRouterModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *AnonRouterModel) Name() string {
	return "anonrouter"
}

// NewInstance returns a driver bound to the given base URL.
func (m *AnonRouterModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAnonRouterModel(baseURL, m.baseModel.URLSuffix)
}
