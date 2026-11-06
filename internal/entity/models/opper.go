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

// OpperModel drives Opper, an EU-hosted AI gateway that serves models from
// many providers behind one OpenAI-compatible API, so the embedded
// OpenAIAPICompatibleModel covers every call.
type OpperModel struct {
	*OpenAIAPICompatibleModel
}

// NewOpperModel creates an Opper driver.
func NewOpperModel(baseURL map[string]string, urlSuffix URLSuffix) *OpperModel {
	return &OpperModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *OpperModel) Name() string {
	return "opper"
}

// NewInstance returns a driver bound to the given base URL.
func (m *OpperModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOpperModel(baseURL, m.baseModel.URLSuffix)
}
