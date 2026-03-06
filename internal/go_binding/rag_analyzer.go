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

package rag_analyzer

/*
#cgo CXXFLAGS: -std=c++20 -I${SRCDIR}/..
#cgo linux LDFLAGS: ${SRCDIR}/../cpp/cmake-build-release/librag_tokenizer_c_api.a -lstdc++ -lm -lpthread /usr/lib/x86_64-linux-gnu/libpcre2-8.a
#cgo darwin LDFLAGS: ${SRCDIR}/../cpp/cmake-build-release/librag_tokenizer_c_api.a -lstdc++ -lm -lpthread /usr/local/lib/libpcre2-8.a

#include <stdlib.h>
#include "../cpp/rag_analyzer_c_api.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

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

// Analyzer wraps the C RAGAnalyzer
type Analyzer struct {
	handle C.RAGAnalyzerHandle
}

// NewAnalyzer creates a new RAGAnalyzer instance
// path: path to dictionary files (containing rag/, wordnet/, opencc/ directories)
func NewAnalyzer(path string) (*Analyzer, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	handle := C.RAGAnalyzer_Create(cPath)
	if handle == nil {
		return nil, fmt.Errorf("failed to create RAGAnalyzer")
	}

	return &Analyzer{handle: handle}, nil
}

// Load loads the analyzer dictionaries
func (a *Analyzer) Load() error {
	if a.handle == nil {
		return fmt.Errorf("analyzer is not initialized")
	}

	ret := C.RAGAnalyzer_Load(a.handle)
	if ret != 0 {
		return fmt.Errorf("failed to load analyzer, error code: %d", ret)
	}
	return nil
}

// SetFineGrained sets whether to use fine-grained tokenization
func (a *Analyzer) SetFineGrained(fineGrained bool) {
	if a.handle == nil {
		return
	}
	C.RAGAnalyzer_SetFineGrained(a.handle, C.bool(fineGrained))
}

// SetEnablePosition sets whether to enable position tracking
func (a *Analyzer) SetEnablePosition(enablePosition bool) {
	if a.handle == nil {
		return
	}
	C.RAGAnalyzer_SetEnablePosition(a.handle, C.bool(enablePosition))
}

// Analyze analyzes the input text and returns all tokens
func (a *Analyzer) Analyze(text string) ([]Token, error) {
	if a.handle == nil {
		return nil, fmt.Errorf("analyzer is not initialized")
	}

	// Since the C API now uses TermList instead of callback,
	// we need a different approach. Let's use Tokenize for now
	// and return the tokens parsed from the space-separated string.
	result, err := a.Tokenize(text)
	if err != nil {
		return nil, err
	}

	// Parse the space-separated result into tokens
	// This is a simplified version - for full position support,
	// we would need to modify the C API to return structured data
	tokens := parseTokens(result)
	return tokens, nil
}

// parseTokens splits a space-separated string into tokens
func parseTokens(result string) []Token {
	var tokens []Token
	start := 0
	for i := 0; i <= len(result); i++ {
		if i == len(result) || result[i] == ' ' {
			if start < i {
				tokens = append(tokens, Token{
					Text:   result[start:i],
					Offset: uint32(start),
					// EndOffset will be approximate without position tracking
					EndOffset: uint32(i),
				})
			}
			start = i + 1
		}
	}
	return tokens
}

// Tokenize analyzes text and returns a space-separated string of tokens
func (a *Analyzer) Tokenize(text string) (string, error) {
	if a.handle == nil {
		return "", fmt.Errorf("analyzer is not initialized")
	}

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cResult := C.RAGAnalyzer_Tokenize(a.handle, cText)
	if cResult == nil {
		return "", fmt.Errorf("tokenize failed")
	}
	defer C.free(unsafe.Pointer(cResult))

	return C.GoString(cResult), nil
}

// TokenizeWithPosition analyzes text and returns tokens with position information
func (a *Analyzer) TokenizeWithPosition(text string) ([]TokenWithPosition, error) {
	if a.handle == nil {
		return nil, fmt.Errorf("analyzer is not initialized")
	}

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cTokenList := C.RAGAnalyzer_TokenizeWithPosition(a.handle, cText)
	if cTokenList == nil {
		return nil, fmt.Errorf("tokenize with position failed")
	}
	defer C.RAGAnalyzer_FreeTokenList(cTokenList)

	// Convert C token list to Go slice
	tokens := make([]TokenWithPosition, cTokenList.count)

	// Iterate through tokens using helper functions
	for i := 0; i < int(cTokenList.count); i++ {
		// Calculate pointer to the i-th token
		cToken := unsafe.Pointer(
			uintptr(unsafe.Pointer(cTokenList.tokens)) +
				uintptr(i)*unsafe.Sizeof(C.struct_RAGTokenWithPosition{}),
		)

		// Use C helper functions to access fields (pass as void*)
		tokens[i] = TokenWithPosition{
			Text:      C.GoString(C.RAGToken_GetText(cToken)),
			Offset:    uint32(C.RAGToken_GetOffset(cToken)),
			EndOffset: uint32(C.RAGToken_GetEndOffset(cToken)),
		}
	}

	return tokens, nil
}

// Close destroys the analyzer and releases resources
func (a *Analyzer) Close() {
	if a.handle != nil {
		C.RAGAnalyzer_Destroy(a.handle)
		a.handle = nil
	}
}

// FineGrainedTokenize performs fine-grained tokenization on space-separated tokens
// Input: space-separated tokens (e.g., "hello world 测试")
// Output: space-separated fine-grained tokens (e.g., "hello world 测 试")
func (a *Analyzer) FineGrainedTokenize(tokens string) (string, error) {
	if a.handle == nil {
		return "", fmt.Errorf("analyzer is not initialized")
	}

	cTokens := C.CString(tokens)
	defer C.free(unsafe.Pointer(cTokens))

	cResult := C.RAGAnalyzer_FineGrainedTokenize(a.handle, cTokens)
	if cResult == nil {
		return "", fmt.Errorf("fine-grained tokenize failed")
	}
	defer C.free(unsafe.Pointer(cResult))

	return C.GoString(cResult), nil
}

// GetTermFreq returns the frequency of a term (matching Python rag_tokenizer.freq)
// Returns: frequency value, or 0 if term not found
func (a *Analyzer) GetTermFreq(term string) int32 {
	if a.handle == nil {
		return 0
	}

	cTerm := C.CString(term)
	defer C.free(unsafe.Pointer(cTerm))

	return int32(C.RAGAnalyzer_GetTermFreq(a.handle, cTerm))
}

// GetTermTag returns the POS tag of a term (matching Python rag_tokenizer.tag)
// Returns: POS tag string (e.g., "n", "v", "ns"), or empty string if term not found or no tag
func (a *Analyzer) GetTermTag(term string) string {
	if a.handle == nil {
		return ""
	}

	cTerm := C.CString(term)
	defer C.free(unsafe.Pointer(cTerm))

	cResult := C.RAGAnalyzer_GetTermTag(a.handle, cTerm)
	if cResult == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cResult))

	return C.GoString(cResult)
}

// Copy creates a new independent analyzer instance from the current one
// The new instance shares the loaded dictionaries but has independent internal state
// This is useful for creating per-request analyzer instances in concurrent environments
func (a *Analyzer) Copy() *Analyzer {
	if a.handle == nil {
		return nil
	}

	handle := C.RAGAnalyzer_Copy(a.handle)
	if handle == nil {
		return nil
	}

	return &Analyzer{handle: handle}
}
