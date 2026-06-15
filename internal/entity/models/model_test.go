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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// joinModelNames extracts model names from a ListModelResponse slice and
// joins them with sep, for use in test assertions.
func joinModelNames(models []ListModelResponse, sep string) string {
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Name
	}
	return strings.Join(names, sep)
}

func readProviderConfig(t *testing.T, fileName string) []byte {
	t.Helper()

	for _, candidate := range []string{
		filepath.Join("..", "..", "..", "conf", "models", fileName),
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

// setupProviderTestDir creates a temporary directory populated with provider
// config files and conf/all_models.json, then changes the working directory to
// it. InitProviderManager hardcodes a read of conf/all_models.json relative to
// CWD, so the test must run from a directory that contains conf/all_models.json.
//
// Provider configs MUST be copied before the chdir because readProviderConfig
// resolves file paths relative to the test binary's original CWD.
//
// Caller must defer the returned restore function.
func setupProviderTestDir(t *testing.T, configFileNames ...string) (dir string, restore func()) {
	t.Helper()
	dir = t.TempDir()

	// Copy provider configs first — readProviderConfig uses relative paths
	// that are only valid from the original CWD.
	for _, fileName := range configFileNames {
		if err := os.WriteFile(filepath.Join(dir, fileName), readProviderConfig(t, fileName), 0o600); err != nil {
			t.Fatalf("write %s config: %v", fileName, err)
		}
	}

	confDir := filepath.Join(dir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("create conf dir: %v", err)
	}

	allModelsSrc := filepath.Join("..", "..", "..", "conf", "all_models.json")
	data, err := os.ReadFile(allModelsSrc)
	if err != nil {
		t.Fatalf("read all_models.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "all_models.json"), data, 0o600); err != nil {
		t.Fatalf("write all_models.json: %v", err)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	return dir, func() { os.Chdir(orig) }
}

func TestHostedProviderConfigsLoadSharedDrivers(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "mineru.json", "paddleocr.json")
	defer restore()

	err := InitProviderManager(dir)
	if err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	pm := GetProviderManager()

	minerU := pm.FindProvider("MinerU.Net")
	if minerU == nil {
		t.Fatal("MinerU.Net provider not found")
	}
	if _, ok := minerU.ModelDriver.(*MinerUModel); !ok {
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
	if _, ok := paddleOCR.ModelDriver.(*PaddleOCRModel); !ok {
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
	dir, restore := setupProviderTestDir(t, "mineru_local.json", "paddleocr_local.json")
	defer restore()

	err := InitProviderManager(dir)
	if err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	pm := GetProviderManager()

	minerU := pm.FindProvider("MinerU")
	if minerU == nil {
		t.Fatal("MinerU provider not found")
	}
	if _, ok := minerU.ModelDriver.(*MinerULocalModel); !ok {
		t.Fatalf("MinerU ModelDriver=%T, want *models.MinerULocalModel", minerU.ModelDriver)
	}
	if minerU.URLSuffix.DocumentParse != "file_parse" {
		t.Errorf("MinerU doc_parse suffix=%q", minerU.URLSuffix.DocumentParse)
	}

	paddleOCR := pm.FindProvider("PaddleOCR")
	if paddleOCR == nil {
		t.Fatal("PaddleOCR provider not found")
	}
	if _, ok := paddleOCR.ModelDriver.(*PaddleOCRLocalModel); !ok {
		t.Fatalf("PaddleOCR ModelDriver=%T, want *models.PaddleOCRLocalModel", paddleOCR.ModelDriver)
	}
	if paddleOCR.URLSuffix.OCR != "layout-parsing" {
		t.Errorf("PaddleOCR OCR suffix=%q", paddleOCR.URLSuffix.OCR)
	}
}

func TestProviderConfigsLoadURLSuffixKeys(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "cohere.json", "xai.json")
	defer restore()

	err := InitProviderManager(dir)
	if err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	pm := GetProviderManager()
	cohere := pm.FindProvider("CoHere")
	if cohere == nil {
		t.Fatal("CoHere provider not found")
	}
	if cohere.URLSuffix.Embedding != "v2/embed" {
		t.Errorf("CoHere embedding suffix=%q", cohere.URLSuffix.Embedding)
	}

	xAI := pm.FindProvider("xAI")
	if xAI == nil {
		t.Fatal("xAI provider not found")
	}
	if xAI.URLSuffix.ASR != "stt" {
		t.Errorf("xAI ASR suffix=%q", xAI.URLSuffix.ASR)
	}
}

func TestProviderConfigRejectsUnknownURLSuffixKey(t *testing.T) {
	dir := t.TempDir()
	config := []byte(`{
  "name": "OpenAI",
  "url": {
    "default": "https://example.com"
  },
  "url_suffix": {
    "chat": "chat/completions",
    "unknown_suffix": "ignored"
  },
  "models": [
    {
      "name": "test-model",
      "max_tokens": 4096,
      "model_types": ["chat"]
    }
  ]
}`)
	if err := os.WriteFile(filepath.Join(dir, "unknown_suffix.json"), config, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := InitProviderManager(dir)
	if err == nil {
		t.Fatal("InitProviderManager succeeded with unknown url_suffix key")
	}
	if !strings.Contains(err.Error(), `unknown field "unknown_suffix"`) {
		t.Fatalf("error=%q, want unknown_suffix field", err)
	}
	if !strings.Contains(err.Error(), "unknown_suffix.json") {
		t.Fatalf("error=%q, want config file context", err)
	}
}

func TestPPIOProviderConfigLoadsIntoProviderManager(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "ppio.json")
	defer restore()

	err := InitProviderManager(dir)
	if err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	pm := GetProviderManager()
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
	if _, ok := provider.ModelDriver.(*PPIOModel); !ok {
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
	if *model.MaxTokens != 64000 {
		t.Errorf("deepseek/deepseek-r1 max_tokens=%d", *model.MaxTokens)
	}
	model, err = pm.GetModelByName("ppio", "deepseek/deepseek-v4-pro")
	if err != nil {
		t.Fatalf("GetModelByName v4 pro: %v", err)
	}
	if *model.MaxTokens != 1048576 {
		t.Errorf("deepseek/deepseek-v4-pro max_tokens=%d", *model.MaxTokens)
	}
	model, err = pm.GetModelByName("ppio", "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatalf("GetModelByName v4 flash: %v", err)
	}
	if *model.MaxTokens != 1048576 {
		t.Errorf("deepseek/deepseek-v4-flash max_tokens=%d", *model.MaxTokens)
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
	dir, restore := setupProviderTestDir(t, "siliconflow.json")
	defer restore()

	err := InitProviderManager(dir)
	if err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	pm := GetProviderManager()
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
	if _, ok := provider.ModelDriver.(*SiliconflowModel); !ok {
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
	if *deepSeekV4Pro.MaxTokens != 1048576 {
		t.Errorf("DeepSeek-V4-Pro max_tokens=%d", *deepSeekV4Pro.MaxTokens)
	}
	if !deepSeekV4Pro.ModelTypeMap["chat"] {
		t.Errorf("DeepSeek-V4-Pro model types=%v, want chat", deepSeekV4Pro.ModelTypes)
	}

	kimiK26, err := pm.GetModelByName("SiliconFlow", "Pro/moonshotai/Kimi-K2.6")
	if err != nil {
		t.Fatalf("GetModelByName Kimi-K2.6: %v", err)
	}
	if *kimiK26.MaxTokens != 262144 {
		t.Errorf("Kimi-K2.6 max_tokens=%d", *kimiK26.MaxTokens)
	}
	if !kimiK26.ModelTypeMap["chat"] || !kimiK26.ModelTypeMap["vision"] {
		t.Errorf("Kimi-K2.6 model types=%v, want chat+vision", kimiK26.ModelTypes)
	}

	glm51, err := pm.GetModelByName("SiliconFlow", "Pro/zai-org/GLM-5.1")
	if err != nil {
		t.Fatalf("GetModelByName GLM-5.1: %v", err)
	}
	if *glm51.MaxTokens != 204800 {
		t.Errorf("GLM-5.1 max_tokens=%d", *glm51.MaxTokens)
	}
}
