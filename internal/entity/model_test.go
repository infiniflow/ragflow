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

package entity

import (
	"os"
	"path/filepath"
	modeldrivers "ragflow/internal/entity/models"
	"testing"
)

func readPPIOProviderConfig(t *testing.T) []byte {
	t.Helper()

	for _, candidate := range []string{
		filepath.Join("..", "..", "conf", "models", "ppio.json"),
		filepath.Join("conf", "models", "ppio.json"),
	} {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data
		}
	}

	t.Fatal("could not locate conf/models/ppio.json")
	return nil
}

func TestPPIOProviderConfigLoadsIntoProviderManager(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ppio.json"), readPPIOProviderConfig(t), 0o600); err != nil {
		t.Fatalf("write ppio config: %v", err)
	}

	pm, err := NewProviderManager(dir)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}

	provider := pm.FindProvider("ppio")
	if provider == nil {
		t.Fatal("PPIO provider not found")
	}
	if provider.Name != "PPIO" {
		t.Errorf("provider.Name=%q", provider.Name)
	}
	if provider.URL["default"] != "https://api.ppinfra.com/v3/openai" {
		t.Errorf("default URL=%q", provider.URL["default"])
	}
	if provider.URLSuffix.Chat != "chat/completions" {
		t.Errorf("chat suffix=%q", provider.URLSuffix.Chat)
	}
	if provider.URLSuffix.Models != "models" {
		t.Errorf("models suffix=%q", provider.URLSuffix.Models)
	}
	if _, ok := provider.ModelDriver.(*modeldrivers.PPIOModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.PPIOModel", provider.ModelDriver)
	}
	if provider.ModelDriver.Name() != "ppio" {
		t.Errorf("ModelDriver.Name()=%q", provider.ModelDriver.Name())
	}
	if len(provider.Models) != 19 {
		t.Fatalf("PPIO model count=%d, want 19", len(provider.Models))
	}
	for _, model := range provider.Models {
		if !model.ModelTypeMap["chat"] {
			t.Errorf("model %q missing chat type map", model.Name)
		}
		if model.Class == nil || *model.Class != "PPIO" {
			t.Errorf("model %q class=%v", model.Name, model.Class)
		}
	}

	models, err := pm.ListModels("PPIO")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 19 {
		t.Errorf("ListModels count=%d, want 19", len(models))
	}

	model, err := pm.GetModelByName("ppio", "deepseek/deepseek-r1")
	if err != nil {
		t.Fatalf("GetModelByName: %v", err)
	}
	if model.MaxTokens != 64000 {
		t.Errorf("deepseek/deepseek-r1 max_tokens=%d", model.MaxTokens)
	}

	resp := pm.SearchByType("chat")
	if resp.Code != 0 {
		t.Fatalf("SearchByType code=%d message=%q", resp.Code, resp.Message)
	}
	if len(resp.Data) != 19 {
		t.Errorf("SearchByType data count=%d, want 19", len(resp.Data))
	}
}
