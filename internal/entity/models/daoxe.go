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

// DaoxeModel implements DaoXE's OpenAI-compatible chat and model-list APIs.
// DaoXE speaks the OpenAI wire format without deviation, so the embedded
// OpenAIAPICompatibleModel covers every call and nothing is overridden beyond
// the driver's own identity. The live catalog is account-scoped, so model
// listing reads GET /v1/models from the gateway instead of a static list.
type DaoxeModel struct {
	*OpenAIAPICompatibleModel
}

// NewDaoxeModel creates a DaoXE model driver.
func NewDaoxeModel(baseURL map[string]string, urlSuffix URLSuffix) *DaoxeModel {
	return &DaoxeModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the model driver name.
func (m *DaoxeModel) Name() string {
	return "DaoXE"
}

// NewInstance creates a new DaoXE driver bound to the given base URL.
func (m *DaoxeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewDaoxeModel(baseURL, m.baseModel.URLSuffix)
}
