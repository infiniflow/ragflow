//go:build !cgo_thincner

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

// Stub for non-CGO builds.
//
// The C++ ThincNER engine (and its ThincParser / ThincTagger
// siblings) is wired through cgo in ner.go, parser_go.go, and
// ner_extractor.go — those files carry `//go:build cgo_thincner`.
// Without the `cgo_thincner` build tag, the test binary cannot link the missing
// ThincNER_* / ThincParser_* / ThincTagger_* symbols.
//
// This file declares the same exported surface the rest of the
// package depends on (Entity / Relation / ExtractionResult /
// Extractor / RunParser / RunTagger / NewExtractor / DetectLanguage
// / ParseTokensWithParser / ExtractRelations) so the pure-Go files
// in the package — primarily dep_relation.go and the relation-
// extraction pure-Go logic — continue to compile.
//
// The cgo-backed functions return an explicit ErrNoCGO error so
// any caller that reaches the C++ engine on a no-CGO build fails
// loudly rather than silently degrading. Production builds use
// the `cgo_thincner` path only when that explicit build tag is enabled.

package extractor

import (
	"encoding/json"
	"errors"
	"sync"
)

// ErrNoCGO is returned by all cgo-backed entry points on non-CGO
// builds. The Python-side ThincNER engine requires the C++ static
// library at internal/cpp/cmake-build-release/librag_tokenizer_c_api.a;
// without it the package compiles but the inference paths fail
// with this error.
var ErrNoCGO = errors.New("extractor: CGO disabled — ThincNER / ThincParser / ThincTagger unavailable")

// Entity mirrors the cgo-backed declaration.
type Entity struct {
	Text       string         `json:"text"`
	Label      string         `json:"label"`
	StartChar  int            `json:"start_char"`
	EndChar    int            `json:"end_char"`
	Confidence float64        `json:"confidence"`
	AppType    string         `json:"app_type,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Relation mirrors the cgo-backed declaration.
type Relation struct {
	Subject    Entity         `json:"subject"`
	Predicate  string         `json:"predicate"`
	Object     Entity         `json:"object"`
	Confidence float64        `json:"confidence"`
	Context    string         `json:"context,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ExtractionResult mirrors the cgo-backed declaration.
type ExtractionResult struct {
	Entities  []Entity       `json:"entities"`
	Relations []Relation     `json:"relations"`
	Language  string         `json:"language,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Extractor mirrors the cgo-backed declaration. The no-CGO
// instance returns ErrNoCGO from every inference call but
// otherwise satisfies the same surface as the CGO build.
type Extractor struct {
	mu                  sync.Mutex
	Lang                string
	ConfidenceThreshold float64
	IncludeTokens       bool
}

// ModelPredictor is the dependency-injection seam used by tests;
// no-CGO builds mirror the type so existing test helpers compile.
type ModelPredictor func(tokensJSON string) (string, error)

// NewExtractor returns a no-CGO Extractor. Inference calls fail
// with ErrNoCGO. The pure-Go relation-extraction helpers
// (DepExtractRelations / ExtractRelations) do not depend on cgo
// and continue to function.
func NewExtractor(lang string) *Extractor {
	return &Extractor{Lang: lang}
}

// RunParser is the no-CGO stub. The cgo-backed implementation is
// in parser_go.go (build tag: cgo_thincner).
func RunParser(nerDir, parserDir string, tokensJSON string) (string, error) {
	return "", ErrNoCGO
}

// RunTagger is the no-CGO stub. The cgo-backed implementation is
// in parser_go.go (build tag: cgo_thincner).
func RunTagger(nerDir, taggerDir string, tokensJSON string) (string, error) {
	return "", ErrNoCGO
}

// ParseTokensWithParser is the no-CGO stub for the typed wrapper
// around RunParser. Production callers should check the error
// before using the returned slice.
func ParseTokensWithParser(nerDir, parserDir string, tokens []string) ([]DepTokenC, error) {
	tj, _ := json.Marshal(tokens)
	resultJSON, err := RunParser(nerDir, parserDir, string(tj))
	if err != nil {
		return nil, err
	}
	var tokensC []DepTokenC
	if err := json.Unmarshal([]byte(resultJSON), &tokensC); err != nil {
		return nil, err
	}
	return tokensC, nil
}

// DepTokenC mirrors the cgo-backed declaration.
type DepTokenC struct {
	Text  string `json:"text"`
	Head  int    `json:"head"`
	Dep   string `json:"dep"`
	Index int    `json:"index"`
}

// DetectLanguage is pure-Go and unchanged by this stub file.
// The CGO build's DetectLanguage is identical; keeping the
// declaration here makes the no-CGO path self-contained.
func DetectLanguage(text string) string {
	return detectLanguageNoCGO(text)
}

// detectLanguageNoCGO is the pure-Go language detection helper.
// Mirrors the CGO file's DetectLanguage without any cgo deps.
//
// The detector prefers the most specific Unicode range:
//
//   - Hiragana / Katakana → "ja" (Japanese)
//   - CJK Unified Ideographs → "zh" (Chinese)
//   - Hangul Syllables → "ko" (Korean)
//   - Cyrillic → "ru"
//   - Arabic → "ar"
//   - Devanagari → "hi"
//   - Otherwise → "en"
//
// This matches the production heuristic closely enough for the
// no-CGO test binary link; production code that depends on
// DetectLanguage's accuracy should set `-tags=cgo_thincner` to
// select the real implementation.
func detectLanguageNoCGO(text string) string {
	if len(text) == 0 {
		return "en"
	}
	var ja, zh, ko, cyrillic, arabic, devanagari, latin int
	for _, r := range text {
		switch {
		case r >= 0x3040 && r <= 0x309F: // Hiragana
			ja++
		case r >= 0x30A0 && r <= 0x30FF: // Katakana
			ja++
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified
			zh++
		case r >= 0xAC00 && r <= 0xD7AF: // Hangul
			ko++
		case r >= 0x0400 && r <= 0x04FF: // Cyrillic
			cyrillic++
		case r >= 0x0600 && r <= 0x06FF: // Arabic
			arabic++
		case r >= 0x0900 && r <= 0x097F: // Devanagari
			devanagari++
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			latin++
		}
	}
	switch {
	case ja > 0:
		return "ja"
	case zh > 0:
		return "zh"
	case ko > 0:
		return "ko"
	case cyrillic > 0:
		return "ru"
	case arabic > 0:
		return "ar"
	case devanagari > 0:
		return "hi"
	default:
		return "en"
	}
}
