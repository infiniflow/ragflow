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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modeldrivers "ragflow/internal/entity/models"
)

func providerConfigDir(t *testing.T) string {
	t.Helper()

	for _, candidate := range []string{
		filepath.Join("..", "..", "conf", "models"),
		filepath.Join("conf", "models"),
	} {
		if entries, err := os.ReadDir(candidate); err == nil && len(entries) > 0 {
			return candidate
		}
	}

	t.Fatal("could not locate conf/models")
	return ""
}

func readProviderConfig(t *testing.T, fileName string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(providerConfigDir(t), fileName))
	if err == nil {
		return data
	}

	t.Fatalf("could not locate conf/models/%s", fileName)
	return nil
}

func readPPIOProviderConfig(t *testing.T) []byte {
	t.Helper()
	return readProviderConfig(t, "ppio.json")
}

func TestProviderConfigURLSuffixKeysAreKnown(t *testing.T) {
	dir := providerConfigDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read provider config dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("read config: %v", err)
			}

			var provider Provider
			if err := json.Unmarshal(data, &provider); err != nil {
				t.Fatalf("parse provider config: %v", err)
			}
		})
	}
}

func TestProviderConfigURLSuffixRegressionFields(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		check    func(*testing.T, Provider)
	}{
		{
			name:     "cohere embedding suffix",
			fileName: "cohere.json",
			check: func(t *testing.T, provider Provider) {
				t.Helper()
				if provider.URLSuffix.Embedding != "v2/embed" {
					t.Fatalf("embedding suffix=%q, want v2/embed", provider.URLSuffix.Embedding)
				}
			},
		},
		{
			name:     "xai asr suffix",
			fileName: "xai.json",
			check: func(t *testing.T, provider Provider) {
				t.Helper()
				if provider.URLSuffix.ASR != "stt" {
					t.Fatalf("asr suffix=%q, want stt", provider.URLSuffix.ASR)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var provider Provider
			if err := json.Unmarshal(readProviderConfig(t, tt.fileName), &provider); err != nil {
				t.Fatalf("parse %s: %v", tt.fileName, err)
			}
			tt.check(t, provider)
		})
	}
}

func TestNewProviderManagerRejectsUnknownURLSuffixKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "bad.json")
	data := []byte(`{
  "name": "OpenAI",
  "url": {
    "default": "http://unused"
  },
  "url_suffix": {
    "chat": "chat/completions",
    "embeddings": "embeddings"
  },
  "models": []
}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write bad provider config: %v", err)
	}

	_, err := NewProviderManager(dir)
	if err == nil {
		t.Fatal("expected unknown url_suffix key error, got nil")
	}
	for _, want := range []string{"bad.json", "unknown url_suffix key", "embeddings"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
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
	if minerU.Class != "mineru.net" {
		t.Errorf("MinerU.Net class=%q", minerU.Class)
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
	if paddleOCR.Class != "paddleocr.net" {
		t.Errorf("PaddleOCR.Net class=%q", paddleOCR.Class)
	}
	if paddleOCR.URLSuffix.OCR != "v2/ocr/jobs" {
		t.Errorf("PaddleOCR.Net OCR suffix=%q", paddleOCR.URLSuffix.OCR)
	}
}

func TestLocalOCRProviderConfigsLoadLocalDrivers(t *testing.T) {
	dir := t.TempDir()
	for _, fileName := range []string{"mineru_local.json", "paddleocr_local.json"} {
		if err := os.WriteFile(filepath.Join(dir, fileName), readProviderConfig(t, fileName), 0o600); err != nil {
			t.Fatalf("write %s config: %v", fileName, err)
		}
	}

	pm, err := NewProviderManager(dir)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}

	minerU := pm.FindProvider("MinerU")
	if minerU == nil {
		t.Fatal("MinerU provider not found")
	}
	if _, ok := minerU.ModelDriver.(*modeldrivers.MinerULocalModel); !ok {
		t.Fatalf("MinerU ModelDriver=%T, want *models.MinerULocalModel", minerU.ModelDriver)
	}
	if minerU.URLSuffix.DocumentParse != "file_parse" {
		t.Errorf("MinerU doc_parse suffix=%q", minerU.URLSuffix.DocumentParse)
	}

	paddleOCR := pm.FindProvider("PaddleOCR")
	if paddleOCR == nil {
		t.Fatal("PaddleOCR provider not found")
	}
	if _, ok := paddleOCR.ModelDriver.(*modeldrivers.PaddleOCRLocalModel); !ok {
		t.Fatalf("PaddleOCR ModelDriver=%T, want *models.PaddleOCRLocalModel", paddleOCR.ModelDriver)
	}
	if paddleOCR.URLSuffix.OCR != "layout-parsing" {
		t.Errorf("PaddleOCR OCR suffix=%q", paddleOCR.URLSuffix.OCR)
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

func TestSiliconFlowProviderConfigLoadsLatestProModels(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "siliconflow.json"), readProviderConfig(t, "siliconflow.json"), 0o600); err != nil {
		t.Fatalf("write siliconflow config: %v", err)
	}

	pm, err := NewProviderManager(dir)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}

	provider := pm.FindProvider("SiliconFlow")
	if provider == nil {
		t.Fatal("SiliconFlow provider not found")
	}
	if provider.URL["default"] != "https://api.siliconflow.cn/v1" {
		t.Errorf("default URL=%q", provider.URL["default"])
	}
	if provider.URLSuffix.Chat != "chat/completions" {
		t.Errorf("chat suffix=%q", provider.URLSuffix.Chat)
	}
	if _, ok := provider.ModelDriver.(*modeldrivers.SiliconflowModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.SiliconflowModel", provider.ModelDriver)
	}
	if provider.ModelDriver.Name() != "siliconflow" {
		t.Errorf("ModelDriver.Name()=%q", provider.ModelDriver.Name())
	}
	if len(provider.Models) != 12 {
		t.Fatalf("SiliconFlow model count=%d, want 12", len(provider.Models))
	}

	deepSeekV4Pro, err := pm.GetModelByName("SiliconFlow", "Pro/deepseek-ai/DeepSeek-V4-Pro")
	if err != nil {
		t.Fatalf("GetModelByName DeepSeek-V4-Pro: %v", err)
	}
	if deepSeekV4Pro.MaxTokens != 1048576 {
		t.Errorf("DeepSeek-V4-Pro max_tokens=%d", deepSeekV4Pro.MaxTokens)
	}
	if !deepSeekV4Pro.ModelTypeMap["chat"] {
		t.Errorf("DeepSeek-V4-Pro model types=%v, want chat", deepSeekV4Pro.ModelTypes)
	}

	kimiK26, err := pm.GetModelByName("SiliconFlow", "Pro/moonshotai/Kimi-K2.6")
	if err != nil {
		t.Fatalf("GetModelByName Kimi-K2.6: %v", err)
	}
	if kimiK26.MaxTokens != 262144 {
		t.Errorf("Kimi-K2.6 max_tokens=%d", kimiK26.MaxTokens)
	}
	if !kimiK26.ModelTypeMap["chat"] || !kimiK26.ModelTypeMap["vision"] {
		t.Errorf("Kimi-K2.6 model types=%v, want chat+vision", kimiK26.ModelTypes)
	}

	glm51, err := pm.GetModelByName("SiliconFlow", "Pro/zai-org/GLM-5.1")
	if err != nil {
		t.Fatalf("GetModelByName GLM-5.1: %v", err)
	}
	if glm51.MaxTokens != 204800 {
		t.Errorf("GLM-5.1 max_tokens=%d", glm51.MaxTokens)
	}
}
