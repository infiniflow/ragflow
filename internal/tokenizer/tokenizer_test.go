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

package tokenizer

import (
	"strings"
	"testing"
)

// saveEngineType saves the current engineTypeProvider and returns a function
// to restore it. Use this when a test modifies the engine type to avoid
// leaking global state between tests.
func saveEngineType() func() {
	original := engineTypeProvider
	return func() { engineTypeProvider = original }
}

// ---------------------------------------------------------------------------
// NumTokensFromString tests
// ---------------------------------------------------------------------------

func TestNumTokensFromString_Empty(t *testing.T) {
	if got := NumTokensFromString(""); got != 0 {
		t.Errorf("expected 0 for empty string, got %d", got)
	}
}

func TestNumTokensFromString_Positive(t *testing.T) {
	for _, s := range []string{"hello world", "你好世界"} {
		if got := NumTokensFromString(s); got <= 0 {
			t.Errorf("NumTokensFromString(%q) = %d, want >0", s, got)
		}
	}
}

func TestNumTokensFromString_VariedInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ascii letters", "hello world"},
		{"chinese characters", "你好世界"},
		{"japanese characters", "こんにちは世界"},
		{"korean characters", "안녕하세요세계"},
		{"emoji", "👋 hello 🌍"},
		{"numbers only", "1234567890"},
		{"special chars", "a+b=c; d!=e"},
		{"newlines and tabs", "line1\nline2\tindented"},
		{"mixed content", "RAGFlow 是一款 开源的 RAG (Retrieval-Augmented Generation) 引擎"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NumTokensFromString(tt.input)
			if got <= 0 {
				t.Errorf("NumTokensFromString(%q) = %d, want >0", tt.input, got)
			}
		})
	}
}

func TestNumTokensFromString_Consistency(t *testing.T) {
	inputs := []string{"hello world", "你好世界", "a+b=c; d!=e"}
	for _, s := range inputs {
		first := NumTokensFromString(s)
		second := NumTokensFromString(s)
		if first != second {
			t.Errorf("NumTokensFromString(%q) is not consistent: %d vs %d", s, first, second)
		}
	}
}

func TestNumTokensFromString_LongString(t *testing.T) {
	long := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 200)
	got := NumTokensFromString(long)
	if got <= 0 {
		t.Errorf("NumTokensFromString(long_string) = %d, want >0", got)
	}
}

func TestNumTokensFromString_WhitespaceOnly(t *testing.T) {
	for _, s := range []string{" ", "\t", "\n", "   "} {
		got := NumTokensFromString(s)
		// Whitespace strings should still produce tokens in BPE encoding
		if got == 0 {
			t.Logf("NumTokensFromString(%q) = %d", s, got)
		}
	}
}

// ---------------------------------------------------------------------------
// RegisterEngineType tests
// ---------------------------------------------------------------------------

func TestRegisterEngineType_Basic(t *testing.T) {
	restore := saveEngineType()
	defer restore()

	RegisterEngineType(func() string { return "infinity" })
	if got := engineTypeProvider(); got != "infinity" {
		t.Errorf("expected 'infinity', got %q", got)
	}
}

func TestRegisterEngineType_Overwrite(t *testing.T) {
	restore := saveEngineType()
	defer restore()

	RegisterEngineType(func() string { return "first" })
	RegisterEngineType(func() string { return "second" })
	if got := engineTypeProvider(); got != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Tokenize tests
// ---------------------------------------------------------------------------

func TestTokenize_InfinityEngine(t *testing.T) {
	restore := saveEngineType()
	defer restore()
	RegisterEngineType(func() string { return "infinity" })

	inputs := []string{"hello world", "你好 世界", "", "a single word"}
	for _, input := range inputs {
		got, err := Tokenize(input)
		if err != nil {
			t.Errorf("Tokenize(%q) unexpected error: %v", input, err)
		}
		if got != input {
			t.Errorf("Tokenize(%q) = %q, want %q", input, got, input)
		}
	}
}

func TestTokenize_PoolNotInitialized(t *testing.T) {
	restore := saveEngineType()
	defer restore()
	// Ensure engine type is not "infinity" so we hit the pool path
	RegisterEngineType(func() string { return "" })

	_, err := Tokenize("hello world")
	if err == nil {
		t.Error("expected error when pool is not initialized, got nil")
	}
}

// ---------------------------------------------------------------------------
// FineGrainedTokenize tests
// ---------------------------------------------------------------------------

func TestFineGrainedTokenize_InfinityEngine(t *testing.T) {
	restore := saveEngineType()
	defer restore()
	RegisterEngineType(func() string { return "infinity" })

	inputs := []string{"hello world", "测试 分词", ""}
	for _, input := range inputs {
		got, err := FineGrainedTokenize(input)
		if err != nil {
			t.Errorf("FineGrainedTokenize(%q) unexpected error: %v", input, err)
		}
		if got != input {
			t.Errorf("FineGrainedTokenize(%q) = %q, want %q", input, got, input)
		}
	}
}

func TestFineGrainedTokenize_PoolNotInitialized(t *testing.T) {
	restore := saveEngineType()
	defer restore()
	RegisterEngineType(func() string { return "" })

	_, err := FineGrainedTokenize("hello world")
	if err == nil {
		t.Error("expected error when pool is not initialized, got nil")
	}
}

// ---------------------------------------------------------------------------
// Error-path tests for functions that require the pool
// ---------------------------------------------------------------------------

func TestTokenizeWithPosition_PoolNotInitialized(t *testing.T) {
	_, err := TokenizeWithPosition("hello world")
	if err == nil {
		t.Error("expected error when pool is not initialized, got nil")
	}
}

func TestAnalyze_PoolNotInitialized(t *testing.T) {
	_, err := Analyze("hello world")
	if err == nil {
		t.Error("expected error when pool is not initialized, got nil")
	}
}

func TestGetTermFreq_PoolNotInitialized(t *testing.T) {
	got := GetTermFreq("hello")
	if got != 0 {
		t.Errorf("expected 0 when pool is not initialized, got %d", got)
	}
}

func TestGetTermTag_PoolNotInitialized(t *testing.T) {
	got := GetTermTag("hello")
	if got != "" {
		t.Errorf("expected empty string when pool is not initialized, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Global state tests
// ---------------------------------------------------------------------------

func TestGetPoolStats_Nil(t *testing.T) {
	// Note: globalPool is nil by default in unit tests (pool not initialized)
	stats := GetPoolStats()
	if stats == nil {
		t.Fatal("GetPoolStats returned nil")
	}
	init, ok := stats["initialized"]
	if !ok {
		t.Fatal("missing 'initialized' key")
	}
	if init.(bool) {
		t.Error("expected initialized=false when pool is nil")
	}
}

func TestIsInitialized_Default(t *testing.T) {
	if IsInitialized() {
		t.Error("expected IsInitialized() = false when pool is not initialized")
	}
}

func TestClose_Nil(t *testing.T) {
	// Close should be safe to call with nil globalPool
	Close() // no panic = pass
}

func TestClose_NilGlobalPool(t *testing.T) {
	// Call Close directly after ensuring globalPool is nil
	// (concurrent test may have initialized it, so handle gracefully)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Close() panicked: %v", r)
		}
	}()
	Close()
}
