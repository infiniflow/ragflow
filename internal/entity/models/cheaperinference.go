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

// CheaperInferenceModel implements Cheaper Inference's OpenAI-compatible chat
// and model-list APIs.
//
// Responses carry an extra "provider" member naming the upstream that served
// the request. The shared response decoder ignores members it does not know, so
// the embedded OpenAIAPICompatibleModel covers every call and nothing is
// overridden beyond the driver's own identity.
type CheaperInferenceModel struct {
	*OpenAIAPICompatibleModel
}

// NewCheaperInferenceModel creates a Cheaper Inference model driver.
func NewCheaperInferenceModel(baseURL map[string]string, urlSuffix URLSuffix) *CheaperInferenceModel {
	return &CheaperInferenceModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the model driver name.
func (m *CheaperInferenceModel) Name() string {
	return "Cheaper Inference"
}

// NewInstance creates a new Cheaper Inference driver bound to the given base URL.
func (m *CheaperInferenceModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCheaperInferenceModel(baseURL, m.baseModel.URLSuffix)
}
