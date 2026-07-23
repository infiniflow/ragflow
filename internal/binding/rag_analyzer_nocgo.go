//go:build !cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rag_analyzer

import "fmt"

// Token represents a single token from the analyzer.
type Token struct {
	Text      string
	Offset    uint32
	EndOffset uint32
}

// TokenWithPosition represents a token with position information.
type TokenWithPosition struct {
	Text      string
	Offset    uint32
	EndOffset uint32
}

// Analyzer is the no-CGO stub used for builds that intentionally skip the
// native tokenizer binding.
type Analyzer struct{}

func NewAnalyzer(path string) (*Analyzer, error) {
	return nil, fmt.Errorf("rag_analyzer: cgo required (path=%q)", path)
}

func (a *Analyzer) Load() error {
	return fmt.Errorf("rag_analyzer: cgo required")
}

func (a *Analyzer) SetFineGrained(bool) {}

func (a *Analyzer) SetEnablePosition(bool) {}

func (a *Analyzer) SetLanguage(string) {}

func (a *Analyzer) Analyze(text string) ([]Token, error) {
	return nil, fmt.Errorf("rag_analyzer: cgo required for Analyze(%q)", text)
}

func (a *Analyzer) Tokenize(text string) (string, error) {
	return "", fmt.Errorf("rag_analyzer: cgo required for Tokenize(%q)", text)
}

func (a *Analyzer) TokenizeWithPosition(text string) ([]TokenWithPosition, error) {
	return nil, fmt.Errorf("rag_analyzer: cgo required for TokenizeWithPosition(%q)", text)
}

func (a *Analyzer) Close() {}

func (a *Analyzer) FineGrainedTokenize(tokens string) (string, error) {
	return "", fmt.Errorf("rag_analyzer: cgo required for FineGrainedTokenize(%q)", tokens)
}

func (a *Analyzer) GetTermFreq(term string) int32 { return 0 }

func (a *Analyzer) GetTermTag(term string) string { return "" }

func (a *Analyzer) Copy() *Analyzer { return nil }
