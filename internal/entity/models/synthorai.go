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

// SynthoraiModel implements Synthorai's OpenAI-compatible chat and model-list
// APIs. Synthorai speaks the OpenAI wire format without deviation, so the
// embedded OpenAIAPICompatibleModel covers every call and nothing is overridden
// beyond the driver's own identity.
type SynthoraiModel struct {
	*OpenAIAPICompatibleModel
}

// NewSynthoraiModel creates a Synthorai model driver.
func NewSynthoraiModel(baseURL map[string]string, urlSuffix URLSuffix) *SynthoraiModel {
	return &SynthoraiModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the model driver name.
func (m *SynthoraiModel) Name() string {
	return "Synthorai"
}

// NewInstance creates a new Synthorai driver bound to the given base URL.
func (m *SynthoraiModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewSynthoraiModel(baseURL, m.baseModel.URLSuffix)
}
