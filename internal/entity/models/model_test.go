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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
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

	paddleOCR := pm.FindProvider("PaddleOCR")
	if paddleOCR == nil {
		t.Fatal("PaddleOCR provider not found")
	}
	if _, ok := paddleOCR.ModelDriver.(*PaddleOCRModel); !ok {
		t.Fatalf("PaddleOCR ModelDriver=%T, want *models.PaddleOCRModel", paddleOCR.ModelDriver)
	}
	if paddleOCR.Class != "paddleocr" {
		t.Errorf("PaddleOCR class=%q", paddleOCR.Class)
	}
	if paddleOCR.URLSuffix.OCR != "v2/ocr/jobs" {
		t.Errorf("PaddleOCR OCR suffix=%q", paddleOCR.URLSuffix.OCR)
	}
}

func TestBedrockConfigPreservesEmbeddingMaxTokens(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "bedrock.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	model, err := GetProviderManager().GetModelByName("Bedrock", "cohere.embed-english-v3")
	if err != nil {
		t.Fatalf("GetModelByName: %v", err)
	}
	if model.MaxTokens == nil || *model.MaxTokens != 512 {
		t.Fatalf("MaxTokens = %v, want 512", model.MaxTokens)
	}
}

func TestLocalOCRProviderConfigsLoadLocalDrivers(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "mineru_local.json", "monkeyocrv2.json", "paddleocr_local.json")
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

	monkeyOCRv2 := pm.FindProvider("MonkeyOCRv2")
	if monkeyOCRv2 == nil {
		t.Fatal("MonkeyOCRv2 provider not found")
	}
	if _, ok := monkeyOCRv2.ModelDriver.(*MonkeyOCRv2Model); !ok {
		t.Fatalf("MonkeyOCRv2 ModelDriver=%T, want *models.MonkeyOCRv2Model", monkeyOCRv2.ModelDriver)
	}
	if monkeyOCRv2.URLSuffix.DocumentParse != "parse" {
		t.Errorf("MonkeyOCRv2 doc_parse suffix=%q", monkeyOCRv2.URLSuffix.DocumentParse)
	}

	paddleOCR := pm.FindProvider("PaddleOCR.local")
	if paddleOCR == nil {
		t.Fatal("PaddleOCR.local provider not found")
	}
	if _, ok := paddleOCR.ModelDriver.(*PaddleOCRLocalModel); !ok {
		t.Fatalf("PaddleOCR.local ModelDriver=%T, want *models.PaddleOCRLocalModel", paddleOCR.ModelDriver)
	}
	if paddleOCR.URLSuffix.OCR != "layout-parsing" {
		t.Errorf("PaddleOCR.local OCR suffix=%q", paddleOCR.URLSuffix.OCR)
	}
}

func TestModelFactoryCreatesMonkeyOCRv2Driver(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("MonkeyOCRv2", map[string]string{"default": "http://localhost:8000"}, URLSuffix{})
	if err != nil {
		t.Fatal(err)
	}
	if driver.Name() != "monkeyocrv2" {
		t.Fatalf("driver.Name()=%q", driver.Name())
	}
}

func TestMonkeyOCRv2DriverVerifiesNativeParseEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openapi.json" {
			t.Fatalf("path=%q", request.URL.Path)
		}
		_, _ = w.Write([]byte(`{"paths":{"/parse":{}}}`))
	}))
	defer server.Close()

	driver := NewMonkeyOCRv2Model(map[string]string{"default": server.URL}, URLSuffix{})
	if _, err := driver.OCRFile(context.Background(), nil, nil, nil, &APIConfig{}, nil, nil); err != nil {
		t.Fatal(err)
	}

	driver = NewMonkeyOCRv2Model(nil, URLSuffix{})
	apiKey := `{"MONKEYOCRV2_SERVER_URL":"` + server.URL + `"}`
	if err := driver.CheckConnection(context.Background(), &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("environment-provisioned API config: %v", err)
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
	cohere := pm.FindProvider("Cohere")
	if cohere == nil {
		t.Fatal("Cohere provider not found")
	}
	if cohere.URLSuffix.Embedding != "v2/embed" {
		t.Errorf("Cohere embedding suffix=%q", cohere.URLSuffix.Embedding)
	}

	xAI := pm.FindProvider("xAI")
	if xAI == nil {
		t.Fatal("xAI provider not found")
	}
	if xAI.URLSuffix.ASR != "stt" {
		t.Errorf("xAI ASR suffix=%q", xAI.URLSuffix.ASR)
	}
}

// url_hint is a display-only example endpoint: every self-hosted provider
// advertises one so the UI can hint a base URL, and hosted providers must not
// (their fixed endpoint comes from `url`).
func TestProviderConfigsLoadURLHint(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "cohere.json", "ollama.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	pm := GetProviderManager()

	ollama := pm.FindProvider("Ollama")
	if ollama == nil {
		t.Fatal("Ollama provider not found")
	}
	if ollama.URLHint == "" {
		t.Error("Ollama url_hint is empty, want an example endpoint")
	}

	cohere := pm.FindProvider("Cohere")
	if cohere == nil {
		t.Fatal("Cohere provider not found")
	}
	if cohere.URLHint != "" {
		t.Errorf("Cohere url_hint=%q, want empty", cohere.URLHint)
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
	withSSRFBypass(t)
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
	if len(provider.Models) != 25 {
		t.Fatalf("PPIO model count=%d, want 25", len(provider.Models))
	}
	for _, model := range provider.Models {
		if len(model.ModelTypes) == 0 {
			t.Errorf("model %q missing model types", model.Name)
		}
		if model.Class == nil || *model.Class != "PPIO" {
			t.Errorf("model %q class=%v", model.Name, model.Class)
		}
	}

	models, err := pm.ListModels("PPIO")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 25 {
		t.Errorf("ListModels count=%d, want 25", len(models))
	}

	model, err := pm.GetModelByName("ppio", "deepseek/deepseek-r1")
	if err != nil {
		t.Fatalf("GetModelByName: %v", err)
	}
	if *model.MaxOutput != 32768 || *model.ContextLength != 131072 {
		t.Errorf("deepseek/deepseek-r1 max_output=%d context_length=%d", *model.MaxOutput, *model.ContextLength)
	}
	model, err = pm.GetModelByName("ppio", "deepseek/deepseek-v4-pro")
	if err != nil {
		t.Fatalf("GetModelByName v4 pro: %v", err)
	}
	if *model.MaxOutput != 393216 || *model.ContextLength != 1048576 {
		t.Errorf("deepseek/deepseek-v4-pro max_output=%d context_length=%d", *model.MaxOutput, *model.ContextLength)
	}
	model, err = pm.GetModelByName("ppio", "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatalf("GetModelByName v4 flash: %v", err)
	}
	if *model.MaxOutput != 393216 || *model.ContextLength != 1048576 {
		t.Errorf("deepseek/deepseek-v4-flash max_output=%d context_length=%d", *model.MaxOutput, *model.ContextLength)
	}
	if !model.ModelTypeMap["chat"] {
		t.Errorf("deepseek/deepseek-v4-flash missing chat type map")
	}
	model, err = pm.GetModelByName("ppio", "qwen/qwen3-embedding-8b")
	if err != nil {
		t.Fatalf("GetModelByName qwen/qwen3-embedding-8b: %v", err)
	}
	if !model.ModelTypeMap["embedding"] {
		t.Errorf("qwen/qwen3-embedding-8b missing embedding type map")
	}
	model, err = pm.GetModelByName("ppio", "baai/bge-reranker-v2-m3")
	if err != nil {
		t.Fatalf("GetModelByName baai/bge-reranker-v2-m3: %v", err)
	}
	if !model.ModelTypeMap["rerank"] {
		t.Errorf("baai/bge-reranker-v2-m3 missing rerank type map")
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
	provider := pm.FindProvider("SILICONFLOW")
	if provider == nil {
		t.Fatal("SILICONFLOW provider not found")
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
	if provider.ModelDriver.Name() != "SILICONFLOW" {
		t.Errorf("ModelDriver.Name()=%q", provider.ModelDriver.Name())
	}
	if len(provider.Models) != 13 {
		t.Fatalf("SILICONFLOW model count=%d, want 13", len(provider.Models))
	}

	deepSeekV4Pro, err := pm.GetModelByName("SILICONFLOW", "Pro/deepseek-ai/DeepSeek-V4-Pro")
	if err != nil {
		t.Fatalf("GetModelByName DeepSeek-V4-Pro: %v", err)
	}
	if *deepSeekV4Pro.MaxOutput != 393216 || *deepSeekV4Pro.ContextLength != 1048576 {
		t.Errorf("DeepSeek-V4-Pro max_output=%d context_length=%d", *deepSeekV4Pro.MaxOutput, *deepSeekV4Pro.ContextLength)
	}
	if !deepSeekV4Pro.ModelTypeMap["chat"] {
		t.Errorf("DeepSeek-V4-Pro model types=%v, want chat", deepSeekV4Pro.ModelTypes)
	}

	kimiK26, err := pm.GetModelByName("SILICONFLOW", "Pro/moonshotai/Kimi-K2.6")
	if err != nil {
		t.Fatalf("GetModelByName Kimi-K2.6: %v", err)
	}
	if *kimiK26.MaxOutput != 65536 || *kimiK26.ContextLength != 262144 {
		t.Errorf("Kimi-K2.6 max_output=%d context_length=%d", *kimiK26.MaxOutput, *kimiK26.ContextLength)
	}
	if !kimiK26.ModelTypeMap["chat"] || !kimiK26.ModelTypeMap["vision"] {
		t.Errorf("Kimi-K2.6 model types=%v, want chat+vision", kimiK26.ModelTypes)
	}

	glm51, err := pm.GetModelByName("SiliconFlow", "Pro/zai-org/GLM-5.1")
	if err != nil {
		t.Fatalf("GetModelByName GLM-5.1: %v", err)
	}
	if *glm51.MaxOutput != 128000 || *glm51.ContextLength != 200000 {
		t.Errorf("GLM-5.1 max_output=%d context_length=%d", *glm51.MaxOutput, *glm51.ContextLength)
	}
}

// TestAllModelsCatalogHasNoDuplicateKeys walks conf/all_models.json as a token stream and
// fails on any object key that occurs twice in the same object.
//
// encoding/json keeps only the last occurrence of a duplicated key, so a catalog that
// writes a field twice loads fine and behaves exactly as if it were written once - the
// duplicate is invisible to every test that goes through the parsed structs, which is how
// thirteen duplicated "tokenizer" fields got into this file and survived review. Only the
// raw token stream can see them, so the check lives here rather than in a schema validator.
func TestAllModelsCatalogHasNoDuplicateKeys(t *testing.T) {
	// Control: the detector has to report a duplicate on input that has one, otherwise the
	// catalog check below is a no-op that passes on anything. That is not hypothetical -
	// the first version of the detector drove its state machine off the commas between
	// members, and json.Decoder.Token does not emit them, so it recognised at most the
	// first key of each object and reported zero duplicates for this very sample.
	// The sample also pins that array elements are not mistaken for keys ("p", "p").
	control := []byte(`{"a": 1, "a": 2, "b": {"c": "x", "c": "y"}, "d": ["p", "p", {"e": 1, "e": 2}]}`)
	if dups := duplicateJSONKeys(t, control); len(dups) != 3 {
		t.Fatalf("detector control: got %d duplicates (%v), want 3 (a, c, e)", len(dups), dups)
	}

	target := filepath.Join(findRepoRoot(), "conf", "all_models.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	for _, dup := range duplicateJSONKeys(t, data) {
		t.Errorf("%s:%d: object key %q appears twice (byte %d); encoding/json keeps only the last one, so the file means something other than it says",
			target, dup.line, dup.key, dup.offset)
	}
}

type duplicateKey struct {
	key    string
	offset int64
	line   int
}

// duplicateJSONKeys returns every key that occurs twice inside the same JSON object.
func duplicateJSONKeys(t *testing.T, data []byte) []duplicateKey {
	t.Helper()

	// One frame per open object or array. expectKey means "the next string in this object
	// is a key": inside an object Decoder.Token yields key, value, key, value ... because
	// the commas are not tokens, so after every value one has to expect a key again.
	type frame struct {
		object    bool
		expectKey bool
		keys      map[string]bool
	}
	var (
		stack []frame
		dups  []duplicateKey
	)

	// afterValue marks that the frame on top of the stack has just received a value.
	afterValue := func() {
		if n := len(stack); n > 0 {
			stack[n-1].expectKey = true
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode catalog: %v", err)
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{', '[':
				// The enclosing object just received a value: whatever follows it is a key,
				// not a value. The nested frame tracks its own keys from here.
				if n := len(stack); n > 0 {
					stack[n-1].expectKey = false
				}
				stack = append(stack, frame{object: v == '{', expectKey: v == '{', keys: map[string]bool{}})
			case '}', ']':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				afterValue()
			}
			continue
		case string:
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
				top := &stack[n-1]
				if top.keys[v] {
					offset := dec.InputOffset()
					dups = append(dups, duplicateKey{key: v, offset: offset, line: lineAtOffset(data, offset)})
				}
				top.keys[v] = true
				top.expectKey = false
				continue
			}
		}
		afterValue()
	}
	return dups
}

// lineAtOffset reports the 1-based line that holds a byte offset.
func lineAtOffset(data []byte, offset int64) int {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	return 1 + bytes.Count(data[:offset], []byte{'\n'})
}

// TestConfFilesHaveNoDuplicateKeys applies the duplicate-key check to every JSON file
// under conf/.
//
// A definition written into a file that already had one does not win: the parser keeps the
// last occurrence, so the older copy stays in force and the newer one is silently ignored.
// That is not hypothetical - conf/infinity_mapping.json carried two deleted_doc_id
// definitions, and the duplicate key meant the column the doc-delete fix (#17685) added -
// with its analyzer - never took effect.
func TestConfFilesHaveNoDuplicateKeys(t *testing.T) {
	confDir := filepath.Join(findRepoRoot(), "conf")
	var files []string
	err := filepath.WalkDir(confDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", confDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("no JSON file under %s; the check would pass vacuously", confDir)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		for _, dup := range duplicateJSONKeys(t, data) {
			t.Errorf("%s:%d: object key %q appears twice (byte %d); encoding/json keeps only the last one, so the file means something other than it says",
				path, dup.line, dup.key, dup.offset)
		}
	}
	t.Logf("checked %d JSON files under conf/", len(files))
}

// TestAllModelsCatalogTokenizerTagsAreKnown checks the catalog's half of the tokenizer
// link: every "tokenizer" value it declares has to name a counter the Go side knows.
//
// A tag that names nothing fails nowhere. ResolveCounter falls back to cl100k_base, the
// ingest path counts with the calibrated estimate, and the only trace is a lower-precision
// count - which is the degradation this field exists to prevent. So a typo in a
// hand-edited catalog is invisible unless the two lists are compared, which is this.
//
// The known ids come from CounterStatuses, not from CounterByID: the bool CounterByID
// returns means "this counter is available", so on a checkout that has not downloaded the
// tokenizer assets it is false even for a valid tag. Keying this test off it would fail on
// every machine without the assets - for the wrong reason. CounterStatuses enumerates the
// ids whether or not their assets are present.
func TestAllModelsCatalogTokenizerTagsAreKnown(t *testing.T) {
	known := map[string]bool{}
	for _, status := range tokenizer.CounterStatuses() {
		known[status.ID] = true
	}
	if len(known) == 0 {
		t.Fatal("tokenizer.CounterStatuses reports no counters; the check would pass vacuously")
	}

	target := filepath.Join(findRepoRoot(), "conf", "all_models.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse %s: %v", target, err)
	}

	knownIDs := sortedKeys(known)
	tags, declared := map[string]int{}, 0
	for _, section := range catalog {
		entries, ok := section.([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			model, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			tag, ok := model["tokenizer"].(string)
			if !ok || tag == "" {
				continue
			}
			declared++
			tags[tag]++
			if !known[tag] {
				t.Errorf("%s: model %v declares tokenizer %q, which is not a counter id %v; ingest will silently count it with the calibrated estimate",
					target, model["name"], tag, knownIDs)
			}
		}
	}
	if declared == 0 {
		t.Fatalf("%s declares no tokenizer for any model; the check would pass vacuously", target)
	}
	t.Logf("catalog tokenizer tags: %v", tags)
}

// sortedKeys returns the keys of a set, sorted, for a deterministic message.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
