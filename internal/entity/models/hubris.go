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

// HubrisModel drives Hubris, a hosted gateway that fronts models from several
// vendors behind one OpenAI-compatible endpoint, so the embedded
// OpenAIAPICompatibleModel covers every call.
type HubrisModel struct {
	*OpenAIAPICompatibleModel
}

// NewHubrisModel creates a Hubris driver.
func NewHubrisModel(baseURL map[string]string, urlSuffix URLSuffix) *HubrisModel {
	return &HubrisModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *HubrisModel) Name() string {
	return "hubris"
}

// NewInstance returns a driver bound to the given base URL.
func (m *HubrisModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewHubrisModel(baseURL, m.baseModel.URLSuffix)
}
