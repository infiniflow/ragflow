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

// OnomeoModel drives onomeo (https://onomeo.com/v1), a gateway that serves
// Claude, GPT, DeepSeek and other models through OpenAI-compatible chat and
// model-list endpoints, so the embedded OpenAIAPICompatibleModel covers every
// call.
type OnomeoModel struct {
	*OpenAIAPICompatibleModel
}

// NewOnomeoModel creates a onomeo driver.
func NewOnomeoModel(baseURL map[string]string, urlSuffix URLSuffix) *OnomeoModel {
	return &OnomeoModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *OnomeoModel) Name() string {
	return "onomeo"
}

// NewInstance returns a driver bound to the given base URL.
func (m *OnomeoModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOnomeoModel(baseURL, m.baseModel.URLSuffix)
}
