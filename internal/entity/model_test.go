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

func readProviderConfig(t *testing.T, fileName string) []byte {
	t.Helper()

	for _, candidate := range []string{
		filepath.Join("..", "..", "conf", "models", fileName),
		filepath.Join("conf", "models", fileName),
	} {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data
		}
	}

	t.Fatalf("could not locate conf/models/%s", fileName)
	return nil
}

func readPPIOProviderConfig(t *testing.T) []byte {
	t.Helper()
	return readProviderConfig(t, "ppio.json")
}

func TestHostedProviderConfigsLoadSharedDrivers(t *testing.T) {
	dir := t.TempDir()
	for _, fileName := range []string{"mineru.json", "paddleocr.json"} {
		if err := os.WriteFile(filepath.Join(dir, fileName), readProviderConfig(t, fileName), 0o600); err != nil {
			t.Fatalf("write %s config: %v", fileName, err)
		}
	}

	pm, err := NewProviderManager(dir)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}

	minerU := pm.FindProvider("MinerU.Net")
	if minerU == nil {
		t.Fatal("MinerU.Net provider not found")
	}
	if _, ok := minerU.ModelDriver.(*modeldrivers.MinerUModel); !ok {
		t.Fatalf("MinerU.Net ModelDriver=%T, want *models.MinerUModel", minerU.ModelDriver)
	}
	if minerU.URLSuffix.DocumentParse != "v4/extract/task" {
		t.Errorf("MinerU.Net doc_parse suffix=%q", minerU.URLSuffix.DocumentParse)
	}

	paddleOCR := pm.FindProvider("PaddleOCR.Net")
	if paddleOCR == nil {
		t.Fatal("PaddleOCR.Net provider not found")
	}
	if _, ok := paddleOCR.ModelDriver.(*modeldrivers.PaddleOCRModel); !ok {
		t.Fatalf("PaddleOCR.Net ModelDriver=%T, want *models.PaddleOCRModel", paddleOCR.ModelDriver)
	}
	if paddleOCR.URLSuffix.OCR != "v2/ocr/jobs" {
		t.Errorf("PaddleOCR.Net OCR suffix=%q", paddleOCR.URLSuffix.OCR)
	}
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
	if provider.URL["default"] != "https://api.ppio.com/openai/v1" {
		t.Errorf("default URL=%q", provider.URL["default"])
	}
	if provider.URL["us"] != "https://api.ppinfra.com/v3/openai" {
		t.Errorf("us URL=%q", provider.URL["us"])
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
	if len(provider.Models) != 21 {
		t.Fatalf("PPIO model count=%d, want 21", len(provider.Models))
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
	if len(models) != 21 {
		t.Errorf("ListModels count=%d, want 21", len(models))
	}

	model, err := pm.GetModelByName("ppio", "deepseek/deepseek-r1")
	if err != nil {
		t.Fatalf("GetModelByName: %v", err)
	}
	if model.MaxTokens != 64000 {
		t.Errorf("deepseek/deepseek-r1 max_tokens=%d", model.MaxTokens)
	}
	model, err = pm.GetModelByName("ppio", "deepseek/deepseek-v4-pro")
	if err != nil {
		t.Fatalf("GetModelByName v4 pro: %v", err)
	}
	if model.MaxTokens != 1048576 {
		t.Errorf("deepseek/deepseek-v4-pro max_tokens=%d", model.MaxTokens)
	}
	model, err = pm.GetModelByName("ppio", "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatalf("GetModelByName v4 flash: %v", err)
	}
	if model.MaxTokens != 1048576 {
		t.Errorf("deepseek/deepseek-v4-flash max_tokens=%d", model.MaxTokens)
	}

	resp := pm.SearchByType("chat")
	if resp.Code != 0 {
		t.Fatalf("SearchByType code=%d message=%q", resp.Code, resp.Message)
	}
	if len(resp.Data) != 21 {
		t.Errorf("SearchByType data count=%d, want 21", len(resp.Data))
	}
}
