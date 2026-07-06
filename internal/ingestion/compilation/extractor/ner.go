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
//go:build cgo_thincner

package extractor

// #cgo CXXFLAGS: -std=c++20 -I${SRCDIR}/../../..
// #cgo linux LDFLAGS: ${SRCDIR}/../../../binding/cpp/cmake-build-release/librag_tokenizer_c_api.a -lstdc++ -lm -lpthread -lpcre2-8
// #cgo darwin LDFLAGS: ${SRCDIR}/../../../binding/cpp/cmake-build-release/librag_tokenizer_c_api.a -lstdc++ -lm -lpthread -lpcre2-8
//
// #include <stdlib.h>
// #include "../../../binding/cpp/rag_analyzer_c_api.h"
// #include "../../../binding/cpp/thinc_parser.h"
import "C"
import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// Entity represents an extracted named entity.
type Entity struct {
	Text       string                 `json:"text"`
	Label      string                 `json:"label"`
	StartChar  int                    `json:"start_char"`
	EndChar    int                    `json:"end_char"`
	Confidence float64                `json:"confidence"`
	AppType    string                 `json:"app_type,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Relation represents a typed relation between two entities.
type Relation struct {
	Subject    Entity                 `json:"subject"`
	Predicate  string                 `json:"predicate"`
	Object     Entity                 `json:"object"`
	Confidence float64                `json:"confidence"`
	Context    string                 `json:"context,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ExtractionResult holds the output of a full extraction pass.
type ExtractionResult struct {
	Entities  []Entity               `json:"entities"`
	Relations []Relation             `json:"relations"`
	Language  string                 `json:"language,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Extractor provides NER + relation extraction
type Extractor struct {
	mu sync.Mutex
	// Language code (en/zh/de/fr/es/pt/ja)
	Lang string
	// Minimum confidence to include an entity (default 0.0 = all)
	ConfidenceThreshold float64
	// Include token-level info (POS, dep) in ExtractionResult metadata
	IncludeTokens bool
	// Max character distance for co-occurrence relations (default 100)
	MaxDistance int
}

// spaCy NER label → application entity type mapping
var spacyToAppType = map[string]string{
	"PERSON":      "person",
	"ORG":         "organization",
	"GPE":         "geo",
	"LOC":         "geo",
	"FAC":         "geo",
	"EVENT":       "event",
	"PRODUCT":     "category",
	"DATE":        "event",
	"TIME":        "event",
	"MONEY":       "category",
	"QUANTITY":    "category",
	"PERCENT":     "category",
	"LAW":         "category",
	"NORP":        "category",
	"LANGUAGE":    "category",
	"WORK_OF_ART": "category",
}

var skipLabels = map[string]bool{
	"ORDINAL":  true,
	"CARDINAL": true,
}

// ModelPredictor is a cached predict function for a model path.
// Closure captures the C handle to avoid unsafe.Pointer type issues.
type ModelPredictor func(tokensJSON string) (string, error)

var (
	modelCacheMu sync.Mutex
	modelCache   = map[string]ModelPredictor{}
)

// langModel maps language codes to spaCy model names.
var langModel = map[string]string{
	"en": "en_core_web_sm",
	"zh": "zh_core_web_sm",
	"de": "de_core_news_sm",
	"fr": "fr_core_news_sm",
	"es": "es_core_news_sm",
	"pt": "pt_core_news_sm",
	"ja": "ja_core_news_sm",
}

// langFallback maps languages without dedicated relation patterns to a fallback.
var langFallback = map[string]string{
	"de": "en",
	"fr": "en",
	"es": "en",
	"pt": "en",
	"ja": "zh",
}

// NewExtractor creates a new extractor.
// Supported langs: en, zh, de, fr, es, pt, ja.
func NewExtractor(lang string) *Extractor {
	if lang == "" {
		lang = "en"
	}
	if _, ok := langModel[lang]; !ok {
		lang = "en"
	}
	return &Extractor{
		Lang:                lang,
		ConfidenceThreshold: 0.0, // include all by default
		MaxDistance:         100,
	}
}

// Extract runs NER and optionally relation extraction (dep-based via C++ parser, or regex fallback).
func (e *Extractor) Extract(text string, extractRelations bool) (*ExtractionResult, error) {
	entities, err := e.ExtractEntities(text)
	if err != nil {
		return nil, err
	}

	// Collect token info if requested (before entity dedup changes offsets)
	var tokensMeta []map[string]interface{}
	if e.IncludeTokens {
		tokensJSON := tokenizeText(text, e.Lang)
		if tokensJSON != "" {
			var rawTokens []string
			if err := json.Unmarshal([]byte(tokensJSON), &rawTokens); err == nil {
				for i, t := range rawTokens {
					tokensMeta = append(tokensMeta, map[string]interface{}{
						"text":  t,
						"index": i,
					})
				}
			}
		}
	}

	result := &ExtractionResult{
		Entities: entities,
		Language: e.Lang,
		Metadata: map[string]interface{}{
			"n_entities": len(entities),
			"model":      langModel[e.Lang],
		},
	}
	if len(tokensMeta) > 0 {
		result.Metadata["n_tokens"] = len(tokensMeta)
		result.Metadata["tokens"] = tokensMeta
	}

	if extractRelations && len(entities) >= 2 {
		relations := e.extractRelations(text, entities)
		result.Relations = relations
		nTyped := 0
		for _, r := range relations {
			if r.Predicate != "related_to" {
				nTyped++
			}
		}
		result.Metadata["n_relations"] = nTyped
	}
	return result, nil
}

// extractRelations attempts dep-based extraction via C++ parser; falls back to regex.
func (e *Extractor) extractRelations(text string, entities []Entity) []Relation {
	relLang := e.Lang
	if fb, ok := langFallback[e.Lang]; ok {
		relLang = fb
	}
	// Try dep-based extraction via C++ parser — uses e.Lang (not relLang) so
	// de/fr/es/pt/ja apply their language-specific DepExtractRelations rules.
	tokensJSON := tokenizeText(text, e.Lang)
	if tokensJSON == "" {
		return extractRelationsWithOpts(text, entities, relLang, e.MaxDistance)
	}
	var tokens []string
	if err := json.Unmarshal([]byte(tokensJSON), &tokens); err != nil || len(tokens) == 0 {
		return extractRelationsWithOpts(text, entities, relLang, e.MaxDistance)
	}
	modelDir := e.findModelDir()
	nerDir := modelDir + "/ner"
	parserDir := modelDir + "/parser"
	if deps, err := ParseTokensWithParser(nerDir, parserDir, tokens); err == nil && len(deps) > 0 {
		depTokens := make([]DepToken, len(deps))
		for i, d := range deps {
			depTokens[i] = DepToken{Text: d.Text, Head: d.Head, Dep: d.Dep, Index: d.Index}
		}
		if rels := DepExtractRelations(text, depTokens, entities, e.Lang, e.MaxDistance); len(rels) > 0 {
			return rels
		}
	}
	// Fallback: regex-based extraction
	return extractRelationsWithOpts(text, entities, relLang, e.MaxDistance)
}

func (e *Extractor) getPredictor(modelDir string) ModelPredictor {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	if p, ok := modelCache[modelDir]; ok {
		return p
	}
	cModelDir := C.CString(modelDir + "/ner")
	cModelVocab := C.CString(modelDir + "/vocab")
	handle := C.ThincNER_Create(cModelDir, cModelVocab)
	C.free(unsafe.Pointer(cModelDir))
	C.free(unsafe.Pointer(cModelVocab))
	// Don't cache a nil handle — return a one-shot error predictor instead.
	if handle == nil {
		fn := func(tokensJSON string) (string, error) {
			return "", fmt.Errorf("ThincNER handle is nil for model dir: %s", modelDir)
		}
		return fn
	}
	p := func(tokensJSON string) (string, error) {
		e.mu.Lock()
		cTokensJSON := C.CString(tokensJSON)
		cResult := C.ThincNER_Predict(handle, cTokensJSON)
		e.mu.Unlock()
		C.free(unsafe.Pointer(cTokensJSON))
		if cResult == nil {
			return "", fmt.Errorf("NER prediction failed")
		}
		defer C.ThincNER_FreeString(cResult)
		return C.GoString(cResult), nil
	}
	modelCache[modelDir] = p
	return p
}

// ExtractEntities extracts named entities from text using C++ ThincNER.
func (e *Extractor) ExtractEntities(text string) ([]Entity, error) {
	tokensJSON := tokenizeText(text, e.Lang)
	if tokensJSON == "" {
		return nil, fmt.Errorf("tokenization failed")
	}

	modelDir := e.findModelDir()
	predict := e.getPredictor(modelDir)

	resultJSON, err := predict(tokensJSON)
	if err != nil {
		return nil, err
	}

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

	// Dedup by (text.lower(), start_char) — matching Python NERExtractor
	// For CJK, strip spaces from entity text (BILUO decoder joins tokens with spaces)
	isCJK := e.Lang == "zh" || e.Lang == "ja"
	seen := make(map[string]bool)
	entities := make([]Entity, 0, len(rawEntities))
	for _, re := range rawEntities {
		if skipLabels[re.Label] {
			continue
		}
		if re.Confidence < e.ConfidenceThreshold {
			continue
		}
		text := re.Text
		if isCJK {
			text = strings.ReplaceAll(text, " ", "")
		}
		key := strings.ToLower(text) + "|" + strconv.Itoa(re.Start)
		if seen[key] {
			continue
		}
		seen[key] = true
		appType := spacyToAppType[re.Label]
		if appType == "" {
			appType = strings.ToLower(re.Label)
		}
		entities = append(entities, Entity{
			Text:       text,
			Label:      re.Label,
			StartChar:  re.Start,
			EndChar:    re.End,
			Confidence: re.Confidence,
			AppType:    appType,
			Metadata:   map[string]interface{}{"source": "thincner"},
		})
	}
	return entities, nil
}

// findModelDir locates the spaCy model directory under /usr/share/infinity/resource/spacy.
func (e *Extractor) findModelDir() string {
	modelName := langModel[e.Lang]
	if modelName == "" {
		modelName = "en_core_web_sm"
	}
	base := "/usr/share/infinity/resource/spacy/" + modelName
	if dirExists(base) {
		return base
	}
	if p := getenv("SPACY_MODEL_DIR"); p != "" {
		return p
	}
	return base
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// tokenizeText tokenizes text via C++ tokenizer (all languages).
// Returns JSON array of token strings.
func tokenizeText(text, lang string) string {
	cText := C.CString(text)
	cLang := C.CString(lang)
	defer C.free(unsafe.Pointer(cText))
	defer C.free(unsafe.Pointer(cLang))

	cTokens := C.ThincNER_Tokenize(cText, cLang)
	if cTokens == nil {
		return ""
	}
	defer C.ThincNER_FreeString(cTokens)
	return C.GoString(cTokens)
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
	// CJK majority
	if float64(han+hira+kata)/float64(total) > 0.3 {
		if hira+kata > han {
			return "ja" // Japanese-heavy
		}
		if han > 0 {
			return "zh" // Han-heavy → Chinese
		}
		return "en"
	}
	// Latin majority — default to en (user specifies de/fr/es/pt explicitly)
	return "en"
}

func isHan(r rune) bool      { return r >= 0x4E00 && r <= 0x9FFF }
func isHiragana(r rune) bool { return r >= 0x3040 && r <= 0x309F }
func isKatakana(r rune) bool { return r >= 0x30A0 && r <= 0x30FF }
func isLatin(r rune) bool    { return (r >= 0x0041 && r <= 0x005A) || (r >= 0x0061 && r <= 0x007A) }
