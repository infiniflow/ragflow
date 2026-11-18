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

// DemonRouteModel drives Demon Route, an OpenAI-compatible LLM router
// (https://api.demonroute.com/v1). Chat only; the embedded
// OpenAIAPICompatibleModel covers every call.
type DemonRouteModel struct {
	*OpenAIAPICompatibleModel
}

// NewDemonRouteModel creates a Demon Route driver.
func NewDemonRouteModel(baseURL map[string]string, urlSuffix URLSuffix) *DemonRouteModel {
	return &DemonRouteModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *DemonRouteModel) Name() string {
	return "demonroute"
}

// NewInstance returns a driver bound to the given base URL.
func (m *DemonRouteModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewDemonRouteModel(baseURL, m.baseModel.URLSuffix)
}
