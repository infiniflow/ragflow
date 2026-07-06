//
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
//

//go:build !cgo

package rag_analyzer

import "fmt"

// Token represents a single token from the analyzer
type Token struct {
	Text      string
	Offset    uint32
	EndOffset uint32
}

// TokenWithPosition represents a token with position information
type TokenWithPosition struct {
	Text      string
	Offset    uint32
	EndOffset uint32
}

// Analyzer is a no-op stub when CGO is unavailable.
type Analyzer struct {
	initialized bool
}

// NewAnalyzer returns an error since CGO is required.
func NewAnalyzer(path string) (*Analyzer, error) {
	return nil, fmt.Errorf("tokenizer requires CGO (librag_tokenizer_c_api.a)")
}

// Load is a no-op in stub mode.
func (a *Analyzer) Load() error { return nil }

// SetFineGrained is a no-op in stub mode.
func (a *Analyzer) SetFineGrained(fineGrained bool) {}

// SetEnablePosition is a no-op in stub mode.
func (a *Analyzer) SetEnablePosition(enablePosition bool) {}

// Analyze always returns an error in stub mode.
func (a *Analyzer) Analyze(text string) ([]Token, error) {
	return nil, fmt.Errorf("tokenizer requires CGO (librag_tokenizer_c_api.a)")
}

// Tokenize always returns an error in stub mode.
func (a *Analyzer) Tokenize(text string) (string, error) {
	return "", fmt.Errorf("tokenizer requires CGO (librag_tokenizer_c_api.a)")
}

// TokenizeWithPosition always returns an error in stub mode.
func (a *Analyzer) TokenizeWithPosition(text string) ([]TokenWithPosition, error) {
	return nil, fmt.Errorf("tokenizer requires CGO (librag_tokenizer_c_api.a)")
}

// Close is a no-op in stub mode.
func (a *Analyzer) Close() { a.initialized = false }

// FineGrainedTokenize always returns an error in stub mode.
func (a *Analyzer) FineGrainedTokenize(tokens string) (string, error) {
	return "", fmt.Errorf("tokenizer requires CGO (librag_tokenizer_c_api.a)")
}

// GetTermFreq returns 0 in stub mode.
func (a *Analyzer) GetTermFreq(term string) int32 { return 0 }

// GetTermTag returns empty string in stub mode.
func (a *Analyzer) GetTermTag(term string) string { return "" }

// Copy returns nil in stub mode.
func (a *Analyzer) Copy() *Analyzer { return nil }
