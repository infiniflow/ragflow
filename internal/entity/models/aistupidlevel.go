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

// AIStupidLevelModel drives AI Stupid Level's Smart Router, an OpenAI-compatible
// router (https://aistupidlevel.info/v1) whose model ids are routing modes such as
// "auto-coding" or "auto-value", or real model ids that pin one model. Each request
// goes to the model that currently scores best on its continuously re-run benchmarks,
// called with the user's own provider keys, so the embedded OpenAIAPICompatibleModel
// covers every call.
type AIStupidLevelModel struct {
	*OpenAIAPICompatibleModel
}

// NewAIStupidLevelModel creates an AI Stupid Level driver.
func NewAIStupidLevelModel(baseURL map[string]string, urlSuffix URLSuffix) *AIStupidLevelModel {
	return &AIStupidLevelModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *AIStupidLevelModel) Name() string {
	return "aistupidlevel"
}

// NewInstance returns a driver bound to the given base URL.
func (m *AIStupidLevelModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAIStupidLevelModel(baseURL, m.baseModel.URLSuffix)
}
