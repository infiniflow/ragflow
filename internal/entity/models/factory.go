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

import (
	"strings"
)

// ModelFactory creates ModelDriver instances based on provider name
type ModelFactory struct {
}

// NewModelFactory creates a new ModelFactory
func NewModelFactory() *ModelFactory {
	return &ModelFactory{}
}

// CreateModelDriver creates a ModelDriver for the given provider and model
func (f *ModelFactory) CreateModelDriver(providerName string, baseURL map[string]string, urlSuffix URLSuffix) (ModelDriver, error) {
	providerLower := strings.ToLower(providerName)
	switch providerLower {
	case "zhipu-ai":
		return NewZhipuAIModel(baseURL, urlSuffix), nil
	case "deepseek":
		return NewDeepSeekModel(baseURL, urlSuffix), nil
	case "moonshot":
		return NewMoonshotModel(baseURL, urlSuffix), nil
	case "minimax":
		return NewMinimaxModel(baseURL, urlSuffix), nil
	case "gitee":
		return NewGiteeModel(baseURL, urlSuffix), nil
	case "siliconflow":
		return NewSiliconflowModel(baseURL, urlSuffix), nil
	default:
		return NewDummyModel(baseURL, urlSuffix), nil
	}
}
