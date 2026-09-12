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

// ApiRouteModel drives API Route, an enterprise-grade OpenAI-compatible API
// aggregation platform that fronts models from several vendors behind a unified
// endpoint, so the embedded OpenAIAPICompatibleModel covers every call.
type ApiRouteModel struct {
	*OpenAIAPICompatibleModel
}

// NewApiRouteModel creates an API Route driver.
func NewApiRouteModel(baseURL map[string]string, urlSuffix URLSuffix) *ApiRouteModel {
	return &ApiRouteModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *ApiRouteModel) Name() string {
	return "apiroute"
}

// NewInstance returns a driver bound to the given base URL.
func (m *ApiRouteModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewApiRouteModel(baseURL, m.baseModel.URLSuffix)
}
