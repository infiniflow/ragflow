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

import "context"

// CloudGPUModel drives CloudGPU (https://cloudgpu.app/v1), a GPU cloud whose
// inference API serves open models through OpenAI-compatible chat and
// model-list endpoints, so the embedded OpenAIAPICompatibleModel covers
// every call.
type CloudGPUModel struct {
	*OpenAIAPICompatibleModel
}

// NewCloudGPUModel creates a CloudGPU driver.
func NewCloudGPUModel(baseURL map[string]string, urlSuffix URLSuffix) *CloudGPUModel {
	return &CloudGPUModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *CloudGPUModel) Name() string {
	return "cloudgpu"
}

// NewInstance returns a driver bound to the given base URL.
func (m *CloudGPUModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCloudGPUModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels lists CloudGPU models and, for ids in the CloudGPU catalog
// (conf/models/cloudgpu.json), returns the catalogued model types so vision
// models keep their image-input capability. Unknown ids keep the name-based
// inference of the embedded driver.
func (m *CloudGPUModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	models, err := m.OpenAIAPICompatibleModel.ListModels(ctx, apiConfig)
	if err != nil {
		return nil, err
	}

	pm := GetProviderManager()
	if pm == nil {
		return models, nil
	}
	provider := pm.FindProvider("CloudGPU")
	if provider == nil {
		return models, nil
	}
	for i := range models {
		if known := pm.FindModel(provider, models[i].Name); known != nil && len(known.ModelTypes) > 0 {
			models[i].ModelTypes = append([]string(nil), known.ModelTypes...)
		}
	}
	return models, nil
}
