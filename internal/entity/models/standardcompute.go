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

// StandardComputeModel drives Standard Compute (https://api.stdcmpt.com/v1),
// an LLM gateway that serves many labs' models through OpenAI-compatible chat
// and model-list endpoints, so the embedded OpenAIAPICompatibleModel covers
// every call.
type StandardComputeModel struct {
	*OpenAIAPICompatibleModel
}

// NewStandardComputeModel creates a Standard Compute driver.
func NewStandardComputeModel(baseURL map[string]string, urlSuffix URLSuffix) *StandardComputeModel {
	return &StandardComputeModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *StandardComputeModel) Name() string {
	return "standardcompute"
}

// NewInstance returns a driver bound to the given base URL.
func (m *StandardComputeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewStandardComputeModel(baseURL, m.baseModel.URLSuffix)
}
