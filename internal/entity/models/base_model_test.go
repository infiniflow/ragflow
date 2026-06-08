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

import "testing"

func TestAPIConfigCheckAllowsLocalProviderWithoutAPIKey(t *testing.T) {
	providerClass := "local"
	apiConfig := &APIConfig{ProviderClass: &providerClass}

	if err := (&BaseModel{}).APIConfigCheck(apiConfig); err != nil {
		t.Fatalf("expected local provider without api key to pass, got %v", err)
	}
	if apiConfig.ApiKey == nil || *apiConfig.ApiKey != "" {
		t.Fatalf("expected local provider api key to be normalized to empty string, got %v", apiConfig.ApiKey)
	}
}

func TestAPIConfigCheckRejectsRemoteProviderWithoutAPIKey(t *testing.T) {
	providerClass := "openai"
	apiKey := ""

	err := (&BaseModel{}).APIConfigCheck(&APIConfig{ApiKey: &apiKey, ProviderClass: &providerClass})
	if err == nil {
		t.Fatal("expected remote provider without api key to fail")
	}
}

func TestAPIConfigCheckRejectsMissingProviderClassWithoutPanic(t *testing.T) {
	apiKey := ""

	err := (&BaseModel{}).APIConfigCheck(&APIConfig{ApiKey: &apiKey})
	if err == nil {
		t.Fatal("expected missing provider class with empty api key to fail")
	}
}
