//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

// Package extractor provides NER and relation extraction for the ingestion
// pipeline. It wraps the C++ ThincNER engine via cgo and supplements it with
// pure-Go regex-based relation extraction.
//
// The architecture mirrors the Python rag/graphrag/ner package so that both
// code paths produce identical output (verified by test).
package extractor

// #cgo CXXFLAGS: -std=c++20 -I${SRCDIR}/../../..
// #cgo linux LDFLAGS: ${SRCDIR}/../../../cpp/cmake-build-release/librag_tokenizer_c_api.a -lstdc++ -lm -lpthread -lpcre2-8
// #cgo darwin LDFLAGS: ${SRCDIR}/../../../cpp/cmake-build-release/librag_tokenizer_c_api.a -lstdc++ -lm -lpthread -lpcre2-8
//
// #include <stdlib.h>
// #include "../../../cpp/rag_analyzer_c_api.h"
import "C"
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"
)

// Entity represents an extracted named entity.
type Entity struct {
	Text        string  `json:"text"`
	Label       string  `json:"label"`
	StartChar   int     `json:"start_char"`
	EndChar     int     `json:"end_char"`
	Confidence  float64 `json:"confidence"`
	AppType     string  `json:"app_type,omitempty"`
}

// Relation represents a typed relation between two entities.
type Relation struct {
	Subject   Entity `json:"subject"`
	Predicate string `json:"predicate"`
	Object    Entity `json:"object"`
	Confidence float64 `json:"confidence"`
	Context   string `json:"context,omitempty"`
}

// ExtractionResult holds the output of a full extraction pass.
type ExtractionResult struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

// Extractor provides NER + relation extraction
type Extractor struct {
	mu sync.Mutex
	// List of supported languages
	Lang string
}

// spaCy NER label → application entity type mapping
var spacyToAppType = map[string]string{
	"PERSON":   "person",
	"ORG":      "organization",
	"GPE":      "geo",
	"LOC":      "geo",
	"FAC":      "geo",
	"EVENT":    "event",
	"PRODUCT":  "category",
	"DATE":     "event",
	"TIME":     "event",
	"MONEY":    "category",
	"QUANTITY": "category",
	"PERCENT":  "category",
	"LAW":      "category",
}

var skipLabels = map[string]bool{
	"ORDINAL":  true,
	"CARDINAL": true,
}

// NewExtractor creates a new extractor.
// lang can be "en" or "zh".
func NewExtractor(lang string) *Extractor {
	if lang == "" {
		lang = "en"
	}
	return &Extractor{Lang: lang}
}

// Extract runs NER and optionally relation extraction on text.
func (e *Extractor) Extract(text string, extractRelations bool) (*ExtractionResult, error) {
	entities, err := e.ExtractEntities(text)
	if err != nil {
		return nil, err
	}
	result := &ExtractionResult{Entities: entities}
	if extractRelations && len(entities) >= 2 {
		relations := ExtractRelations(text, entities, e.Lang)
		result.Relations = relations
	}
	return result, nil
}

// ExtractEntities extracts named entities from text using C++ ThincNER.
func (e *Extractor) ExtractEntities(text string) ([]Entity, error) {
	cText := C.CString(text)
	cLang := C.CString(e.Lang)
	defer C.free(unsafe.Pointer(cText))
	defer C.free(unsafe.Pointer(cLang))

	cTokens := C.ThincNER_Tokenize(cText, cLang)
	if cTokens == nil {
		return nil, fmt.Errorf("tokenization failed")
	}
	defer C.ThincNER_FreeString(cTokens)

	tokensJSON := C.GoString(cTokens)

	e.mu.Lock()
	// For now, use a static handle approach: each ExtractEntities call
	// creates and destroys the handle (simplified; production should cache).
	modelDir := e.findModelDir()
	cModelDir := C.CString(modelDir + "/ner")
	cModelVocab := C.CString(modelDir + "/vocab")
	handle := C.ThincNER_Create(cModelDir, cModelVocab)
	C.free(unsafe.Pointer(cModelDir))
	C.free(unsafe.Pointer(cModelVocab))
	e.mu.Unlock()

	if handle == nil {
		return nil, fmt.Errorf("failed to create ThincNER handle for model dir: %s", modelDir+"/ner")
	}
	defer C.ThincNER_Destroy(handle)

	cTokensJSON := C.CString(tokensJSON)
	defer C.free(unsafe.Pointer(cTokensJSON))

	cResult := C.ThincNER_Predict(handle, cTokensJSON)
	if cResult == nil {
		return nil, fmt.Errorf("NER prediction failed")
	}
	defer C.ThincNER_FreeString(cResult)

	resultJSON := C.GoString(cResult)

	var rawEntities []struct {
		Text       string  `json:"text"`
		Label      string  `json:"label"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &rawEntities); err != nil {
		return nil, fmt.Errorf("failed to parse NER result: %w", err)
	}

	entities := make([]Entity, 0, len(rawEntities))
	for _, re := range rawEntities {
		if skipLabels[re.Label] {
			continue
		}
		appType := spacyToAppType[re.Label]
		if appType == "" {
			appType = strings.ToLower(re.Label)
		}
		entities = append(entities, Entity{
			Text:        re.Text,
			Label:       re.Label,
			StartChar:   re.Start,
			EndChar:     re.End,
			Confidence:  re.Confidence,
			AppType:     appType,
		})
	}
	return entities, nil
}

// findModelDir locates the spaCy model directory.
// Searches standard locations.
func (e *Extractor) findModelDir() string {
	langModel := map[string]string{
		"en": "en_core_web_sm",
		"zh": "zh_core_web_sm",
	}
	modelName := langModel[e.Lang]
	if modelName == "" {
		modelName = "en_core_web_sm"
	}
	// Standard spaCy data paths (with exported model.ckpt+model.bin)
	candidates := []string{
		"./models/" + modelName,
		"models/" + modelName,
		"/usr/share/infinity/resource/spacy/" + modelName,
		"/usr/local/lib/python3.10/site-packages/" + modelName + "/" + modelName + "-3.8.0",
		"/usr/lib/python3.10/site-packages/" + modelName + "/" + modelName + "-3.8.0",
		"/opt/conda/lib/python3.10/site-packages/" + modelName + "/" + modelName + "-3.8.0",
	}
	// Also search .venv paths
	venvCandidates := []string{
		".venv/lib/python3.13/site-packages/" + modelName + "/" + modelName + "-3.8.0",
		".venv/lib/python3.12/site-packages/" + modelName + "/" + modelName + "-3.8.0",
		".venv/lib/python3.11/site-packages/" + modelName + "/" + modelName + "-3.8.0",
		".venv/lib/python3.10/site-packages/" + modelName + "/" + modelName + "-3.8.0",
	}
	candidates = append(candidates, venvCandidates...)

	for _, c := range candidates {
		if dirExists(c) {
			return c
		}
	}
	// Fallback: try `python -m spacy info` or environment variable
	if p := getenv("SPACY_MODEL_DIR"); p != "" {
		return p
	}
	return "./models/" + modelName
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func getenv(key string) string {
	return os.Getenv(key)
}

// DetectLanguage detects text language based on Unicode ranges.
// Pure Go, zero dependencies.
func DetectLanguage(text string) string {
	han, hira, kata, latin := 0, 0, 0, 0
	for _, r := range text {
		switch {
		case isHan(r):
			han++
		case isHiragana(r):
			hira++
		case isKatakana(r):
			kata++
		case isLatin(r):
			latin++
		}
	}
	total := han + hira + kata + latin
	if total == 0 {
		return "en"
	}
	// CJK majority — extractor only supports "en" and "zh"
	if float64(han+hira+kata)/float64(total) > 0.3 {
		if hira+kata > han {
			return "en" // Japanese-heavy → fallback to en (no ja extractor)
		}
		if han > 0 {
			return "zh" // Han-heavy → treat as Chinese
		}
		return "en"
	}
	return "en"
}

func isHan(r rune) bool      { return r >= 0x4E00 && r <= 0x9FFF }
func isHiragana(r rune) bool { return r >= 0x3040 && r <= 0x309F }
func isKatakana(r rune) bool { return r >= 0x30A0 && r <= 0x30FF }
func isLatin(r rune) bool    { return (r >= 0x0041 && r <= 0x005A) || (r >= 0x0061 && r <= 0x007A) }
