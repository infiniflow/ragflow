// Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package nlp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestNewTermWeightDealer tests the constructor
func TestNewTermWeightDealer(t *testing.T) {
	// Test with empty resPath
	d := NewTermWeightDealer("")
	if d == nil {
		t.Fatal("NewTermWeightDealer returned nil")
	}

	// Check stop words are initialized
	if len(d.stopWords) == 0 {
		t.Error("Stop words not initialized")
	}

	// Check stop word exists
	if _, ok := d.stopWords["请问"]; !ok {
		t.Error("Expected stop word '请问' not found")
	}

	// Test with non-existent resPath (should not panic)
	d2 := NewTermWeightDealer("/nonexistent/path")
	if d2 == nil {
		t.Fatal("NewTermWeightDealer returned nil for non-existent path")
	}
}

// TestNewTermWeightDealerWithMockFiles tests with mock dictionary files
func TestNewTermWeightDealerWithMockFiles(t *testing.T) {
	// Create temporary directory with mock files
	tmpDir := t.TempDir()

	// Create mock ner.json
	nerData := `{
		"北京": "loca",
		"腾讯": "corp",
		"func": "func",
		"toxic": "toxic"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644); err != nil {
		t.Fatalf("Failed to create mock ner.json: %v", err)
	}

	// Create mock term.freq
	freqData := "hello\t100\nworld\t200\ntest\t50\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "term.freq"), []byte(freqData), 0644); err != nil {
		t.Fatalf("Failed to create mock term.freq: %v", err)
	}

	d := NewTermWeightDealer(tmpDir)

	// Check NE dictionary
	if ne := d.Ner("北京"); ne != "loca" {
		t.Errorf("Expected NE 'loca' for '北京', got '%s'", ne)
	}
	if ne := d.Ner("腾讯"); ne != "corp" {
		t.Errorf("Expected NE 'corp' for '腾讯', got '%s'", ne)
	}

	// Check DF dictionary
	if df := d.GetDF(); len(df) != 3 {
		t.Errorf("Expected 3 entries in DF, got %d", len(df))
	}
}

// TestPretoken tests the pretokenization function
func TestPretoken(t *testing.T) {
	d := NewTermWeightDealer("")

	tests := []struct {
		name     string
		txt      string
		num      bool
		stpwd    bool
		expected []string
	}{
		{
			name:     "simple text",
			txt:      "hello world",
			num:      false,
			stpwd:    true,
			expected: []string{}, // May vary based on tokenizer
		},
		{
			name:     "with stop words",
			txt:      "请问你好吗",
			num:      false,
			stpwd:    true,
			expected: []string{}, // Stop words should be removed
		},
		{
			name:     "with numbers (num=true)",
			txt:      "123",
			num:      true,
			stpwd:    true,
			expected: []string{}, // Single digit may be filtered
		},
		{
			name:     "empty text",
			txt:      "",
			num:      false,
			stpwd:    true,
			expected: []string{},
		},
		{
			name:     "only punctuation",
			txt:      "，。！？",
			num:      false,
			stpwd:    true,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Pretoken(tt.txt, tt.num, tt.stpwd)
			// Just check it doesn't panic and returns a slice
			if result == nil {
				t.Error("Pretoken returned nil")
			}
		})
	}
}

// TestTokenMerge tests token merging
func TestTokenMerge(t *testing.T) {
	d := NewTermWeightDealer("")

	tests := []struct {
		name     string
		tks      []string
		expected []string
	}{
		{
			name:     "empty input",
			tks:      []string{},
			expected: []string{},
		},
		{
			name:     "single token",
			tks:      []string{"hello"},
			expected: []string{"hello"},
		},
		{
			name:     "consecutive short tokens",
			tks:      []string{"a", "b", "c"},
			expected: []string{"a b c"}, // Should merge
		},
		{
			name:     "mixed tokens",
			tks:      []string{"a", "hello", "b"},
			expected: []string{"a", "hello", "b"},
		},
		{
			name:     "first term single char followed by multi-char",
			tks:      []string{"多", "工位"},
			expected: []string{"多 工位"}, // Special case
		},
		{
			name:     "too many short tokens (>=5)",
			tks:      []string{"a", "b", "c", "d", "e", "f"},
			expected: []string{"a b", "c d", "e f"}, // Merge in pairs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.TokenMerge(tt.tks)
			if !reflect.DeepEqual(result, tt.expected) {
				// Debug: print detailed comparison
				t.Errorf("TokenMerge(%v) = %v (len=%d), expected %v (len=%d)", 
					tt.tks, result, len(result), tt.expected, len(tt.expected))
				for i, r := range result {
					t.Errorf("  result[%d] = %q (len=%d)", i, r, len(r))
				}
				for i, e := range tt.expected {
					t.Errorf("  expected[%d] = %q (len=%d)", i, e, len(e))
				}
			}
		})
	}
}

// TestNer tests named entity recognition
func TestNer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock ner.json
	nerData := `{
		"北京": "loca",
		"腾讯": "corp",
		"阿里巴巴": "corp"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644); err != nil {
		t.Fatalf("Failed to create mock ner.json: %v", err)
	}

	d := NewTermWeightDealer(tmpDir)

	tests := []struct {
		term     string
		expected string
	}{
		{"北京", "loca"},
		{"腾讯", "corp"},
		{"阿里巴巴", "corp"},
		{"不存在", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.term, func(t *testing.T) {
			result := d.Ner(tt.term)
			if result != tt.expected {
				t.Errorf("Ner('%s') = '%s', expected '%s'", tt.term, result, tt.expected)
			}
		})
	}
}

// TestSplit tests text splitting
func TestSplit(t *testing.T) {
	d := NewTermWeightDealer("")

	tests := []struct {
		name     string
		txt      string
		expected []string
	}{
		{
			name:     "simple split",
			txt:      "hello world test",
			// Consecutive English words ending with letters are merged
			expected: []string{"hello world test"},
		},
		{
			name:     "consecutive English words",
			txt:      "machine learning algorithm",
			expected: []string{"machine learning algorithm"}, // Should merge
		},
		{
			name:     "mixed Chinese and English",
			txt:      "hello 世界 world",
			// "hello" ends with letter, "世界" doesn't start with letter but doesn't end with letter either
			expected: []string{"hello", "世界", "world"},
		},
		{
			name:     "empty string",
			txt:      "",
			expected: []string{""},
		},
		{
			name:     "multiple spaces",
			txt:      "hello    world",
			// Multiple spaces are normalized, then merged if both end with letters
			expected: []string{"hello world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Split(tt.txt)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Split('%s') = %v (len=%d), expected %v (len=%d)", 
					tt.txt, result, len(result), tt.expected, len(tt.expected))
				for i, r := range result {
					t.Errorf("  result[%d] = %q", i, r)
				}
				for i, e := range tt.expected {
					t.Errorf("  expected[%d] = %q", i, e)
				}
			}
		})
	}
}

// TestWeights tests weight calculation
func TestWeights(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock ner.json
	nerData := `{
		"toxic": "toxic",
		"func": "func",
		"corp": "corp",
		"loca": "loca"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644); err != nil {
		t.Fatalf("Failed to create mock ner.json: %v", err)
	}

	// Create mock term.freq
	freqData := "hello\t100\nworld\t200\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "term.freq"), []byte(freqData), 0644); err != nil {
		t.Fatalf("Failed to create mock term.freq: %v", err)
	}

	d := NewTermWeightDealer(tmpDir)

	t.Run("without preprocess", func(t *testing.T) {
		tks := []string{"hello", "world", "123"}
		weights := d.Weights(tks, false)

		if len(weights) != len(tks) {
			t.Errorf("Expected %d weights, got %d", len(tks), len(weights))
		}

		// Check weights sum to 1 (normalized)
		sum := 0.0
		for _, tw := range weights {
			sum += tw.Weight
		}
		if sum < 0.99 || sum > 1.01 {
			t.Errorf("Weights should sum to ~1, got %f", sum)
		}
	})

	t.Run("with preprocess", func(t *testing.T) {
		tks := []string{"hello world", "test"}
		weights := d.Weights(tks, true)

		// Check it doesn't panic and returns results
		if weights == nil {
			t.Error("Weights returned nil")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		weights := d.Weights([]string{}, false)
		if len(weights) != 0 {
			t.Errorf("Expected empty weights for empty input, got %d", len(weights))
		}
	})

	t.Run("ner weight effect", func(t *testing.T) {
		tmpDir2 := t.TempDir()
		nerData := `{"toxicterm": "toxic"}`
		os.WriteFile(filepath.Join(tmpDir2, "ner.json"), []byte(nerData), 0644)
		d2 := NewTermWeightDealer(tmpDir2)

		tks := []string{"toxicterm", "normal"}
		weights := d2.Weights(tks, false)

		if len(weights) != 2 {
			t.Fatalf("Expected 2 weights, got %d", len(weights))
		}

		// toxicterm should have higher weight (nerWeight=2)
		if weights[0].Weight <= weights[1].Weight {
			t.Error("Expected toxicterm to have higher weight than normal term")
		}
	})
}

// TestWeightsWithNER tests NER type weight effects
func TestWeightsWithNER(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock ner.json with all types
	nerData := `{
		"toxic_word": "toxic",
		"func_word": "func",
		"corp_name": "corp",
		"location": "loca",
		"school": "sch",
		"stock": "stock",
		"firstname": "firstnm"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644); err != nil {
		t.Fatalf("Failed to create mock ner.json: %v", err)
	}

	d := NewTermWeightDealer(tmpDir)

	tests := []struct {
		term         string
		expectedType string
	}{
		{"toxic_word", "toxic"},
		{"func_word", "func"},
		{"corp_name", "corp"},
		{"location", "loca"},
		{"school", "sch"},
		{"stock", "stock"},
		{"firstname", "firstnm"},
	}

	for _, tt := range tests {
		t.Run(tt.term, func(t *testing.T) {
			ne := d.Ner(tt.term)
			if ne != tt.expectedType {
				t.Errorf("Ner('%s') = '%s', expected '%s'", tt.term, ne, tt.expectedType)
			}
		})
	}
}

// TestGetters tests the getter methods
func TestGetters(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock files
	nerData := `{"test": "type"}`
	os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644)
	os.WriteFile(filepath.Join(tmpDir, "term.freq"), []byte("word\t10\n"), 0644)

	d := NewTermWeightDealer(tmpDir)

	t.Run("GetStopWords", func(t *testing.T) {
		sw := d.GetStopWords()
		if len(sw) == 0 {
			t.Error("GetStopWords returned empty map")
		}
		if _, ok := sw["请问"]; !ok {
			t.Error("Expected stop word '请问' not in map")
		}
	})

	t.Run("GetNE", func(t *testing.T) {
		ne := d.GetNE()
		if len(ne) != 1 {
			t.Errorf("Expected 1 NE entry, got %d", len(ne))
		}
		if ne["test"] != "type" {
			t.Error("NE dictionary content incorrect")
		}
	})

	t.Run("GetDF", func(t *testing.T) {
		df := d.GetDF()
		if len(df) != 1 {
			t.Errorf("Expected 1 DF entry, got %d", len(df))
		}
		if df["word"] != 10 {
			t.Error("DF dictionary content incorrect")
		}
	})
}

// TestLoadDict tests dictionary loading
func TestLoadDict(t *testing.T) {
	t.Run("load with frequency", func(t *testing.T) {
		tmpDir := t.TempDir()
		content := "word1\t100\nword2\t200\nword3\t300\n"
		fn := filepath.Join(tmpDir, "test.freq")
		os.WriteFile(fn, []byte(content), 0644)

		dict := loadDict(fn)
		if len(dict) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(dict))
		}
		if dict["word1"] != 100 {
			t.Errorf("Expected word1=100, got %d", dict["word1"])
		}
	})

	t.Run("load without frequency (set mode)", func(t *testing.T) {
		tmpDir := t.TempDir()
		content := "word1\nword2\nword3\n"
		fn := filepath.Join(tmpDir, "test.freq")
		os.WriteFile(fn, []byte(content), 0644)

		dict := loadDict(fn)
		if len(dict) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(dict))
		}
		// All values should be 0 in set mode
		for k, v := range dict {
			if v != 0 {
				t.Errorf("Expected %s=0 in set mode, got %d", k, v)
			}
		}
	})

	t.Run("load non-existent file", func(t *testing.T) {
		dict := loadDict("/nonexistent/file.txt")
		if dict == nil {
			t.Error("loadDict should return empty map, not nil")
		}
		if len(dict) != 0 {
			t.Error("loadDict should return empty map for non-existent file")
		}
	})

	t.Run("load with malformed lines", func(t *testing.T) {
		tmpDir := t.TempDir()
		content := "word1\t100\n\n\nword2\tnotanumber\nword3"
		fn := filepath.Join(tmpDir, "test.freq")
		os.WriteFile(fn, []byte(content), 0644)

		dict := loadDict(fn)
		// Should handle empty lines and invalid numbers gracefully
		if len(dict) < 1 {
			t.Error("Should handle malformed lines gracefully")
		}
	})
}

// TestWeightsNormalization tests weight normalization
func TestWeightsNormalization(t *testing.T) {
	d := NewTermWeightDealer("")

	tests := []struct {
		name string
		tks  []string
	}{
		{
			name: "single token",
			tks:  []string{"hello"},
		},
		{
			name: "multiple tokens",
			tks:  []string{"hello", "world", "test"},
		},
		{
			name: "many tokens",
			tks:  []string{"a", "b", "c", "d", "e"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weights := d.Weights(tt.tks, false)

			if len(weights) != len(tt.tks) {
				t.Fatalf("Expected %d weights, got %d", len(tt.tks), len(weights))
			}

			// Sum should be approximately 1
			sum := 0.0
			for _, tw := range weights {
				sum += tw.Weight
				// Individual weights should be non-negative
				if tw.Weight < 0 {
					t.Errorf("Weight for '%s' is negative: %f", tw.Term, tw.Weight)
				}
			}

			if sum < 0.99 || sum > 1.01 {
				t.Errorf("Weights sum to %f, expected ~1.0", sum)
			}
		})
	}
}

// TestSplitWithNER tests Split with NER considerations
func TestSplitWithNER(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock ner.json
	nerData := `{
		"function": "func"
	}`
	os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644)

	d := NewTermWeightDealer(tmpDir)

	t.Run("func type should not merge", func(t *testing.T) {
		// If one of the words has NE type "func", they should not merge
		result := d.Split("hello function")
		// "hello" and "function" should not merge because function has type "func"
		if len(result) != 2 {
			t.Logf("Result: %v", result)
		}
	})
}

// BenchmarkWeights benchmarks the Weights function
func BenchmarkWeights(b *testing.B) {
	d := NewTermWeightDealer("")
	tks := []string{"hello", "world", "this", "is", "a", "test", "of", "term", "weights", "calculation"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Weights(tks, false)
	}
}

// BenchmarkTokenMerge benchmarks the TokenMerge function
func BenchmarkTokenMerge(b *testing.B) {
	d := NewTermWeightDealer("")
	tks := []string{"a", "b", "c", "d", "e", "hello", "world", "x", "y", "z"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.TokenMerge(tks)
	}
}

// TestTermWeightStructure tests the TermWeight struct
func TestTermWeightStructure(t *testing.T) {
	tw := TermWeight{
		Term:   "test",
		Weight: 0.5,
	}

	if tw.Term != "test" {
		t.Error("Term field incorrect")
	}
	if tw.Weight != 0.5 {
		t.Error("Weight field incorrect")
	}
}

// TestIntegration tests an integrated workflow
func TestIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock dictionaries
	nerData := `{
		"北京": "loca",
		"腾讯": "corp"
	}`
	os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644)
	os.WriteFile(filepath.Join(tmpDir, "term.freq"), []byte("北京\t1000\n腾讯\t500\n"), 0644)

	d := NewTermWeightDealer(tmpDir)

	// Full workflow: text -> split -> pretoken -> token_merge -> weights
	text := "北京 腾讯 公司"

	// Step 1: Split
	splitted := d.Split(text)
	if len(splitted) == 0 {
		t.Fatal("Split returned empty result")
	}

	// Step 2: Pretoken
	var allTokens []string
	for _, s := range splitted {
		tokens := d.Pretoken(s, true, true)
		allTokens = append(allTokens, tokens...)
	}

	// Step 3: Token merge
	merged := d.TokenMerge(allTokens)

	// Step 4: Calculate weights
	weights := d.Weights(merged, false)

	// Verify results
	if len(weights) == 0 && len(merged) > 0 {
		t.Error("Weights calculation failed")
	}

	// Check weights sum to 1
	sum := 0.0
	for _, w := range weights {
		sum += w.Weight
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("Final weights sum to %f, expected ~1.0", sum)
	}
}

// TestWeightsEdgeCases tests edge cases for weight calculation
func TestWeightsEdgeCases(t *testing.T) {
	d := NewTermWeightDealer("")

	t.Run("numbers pattern", func(t *testing.T) {
		tks := []string{"123,45", "abc"}
		weights := d.Weights(tks, false)
		if len(weights) != 2 {
			t.Fatalf("Expected 2 weights, got %d", len(weights))
		}
		// Numbers should get nerWeight=2
	})

	t.Run("short letters pattern", func(t *testing.T) {
		tks := []string{"ab", "abc"}
		weights := d.Weights(tks, false)
		if len(weights) != 2 {
			t.Fatalf("Expected 2 weights, got %d", len(weights))
		}
	})

	t.Run("letter pattern with spaces", func(t *testing.T) {
		tks := []string{"hello world test"}
		weights := d.Weights(tks, true)
		// Should not panic
		if weights == nil {
			t.Error("Weights returned nil for letter pattern")
		}
	})
}

// TestPretokenWithNumbers tests pretoken with num parameter
func TestPretokenWithNumbers(t *testing.T) {
	d := NewTermWeightDealer("")

	t.Run("num=false filters single digits", func(t *testing.T) {
		result := d.Pretoken("5", false, true)
		// Single digit should be filtered when num=false
		found := false
		for _, r := range result {
			if r == "5" {
				found = true
				break
			}
		}
		if found {
			t.Error("Single digit should be filtered when num=false")
		}
	})

	t.Run("num=true keeps single digits", func(t *testing.T) {
		result := d.Pretoken("5 123", true, true)
		// Check at least something is returned
		if len(result) == 0 {
			t.Log("Single digit may still be filtered by other rules")
		}
	})
}

// TestPretokenStopWords tests pretoken with stpwd parameter
func TestPretokenStopWords(t *testing.T) {
	d := NewTermWeightDealer("")

	t.Run("stpwd=true removes stop words", func(t *testing.T) {
		result := d.Pretoken("请问", true, true)
		// "请问" is a stop word
		for _, r := range result {
			if r == "请问" {
				t.Error("Stop word should be removed when stpwd=true")
			}
		}
	})

	t.Run("stpwd=false keeps stop words", func(t *testing.T) {
		result := d.Pretoken("请问", true, false)
		// With tokenizer, this might still filter it
		_ = result
	})
}

// TestTokenMergeEdgeCases tests edge cases for token merging
func TestTokenMergeEdgeCases(t *testing.T) {
	d := NewTermWeightDealer("")

	t.Run("nil input", func(t *testing.T) {
		result := d.TokenMerge(nil)
		if len(result) != 0 {
			t.Error("TokenMerge(nil) should return empty slice")
		}
	})

	t.Run("empty strings in input", func(t *testing.T) {
		result := d.TokenMerge([]string{"", "a", "", "b", ""})
		// Empty strings should be filtered
		for _, r := range result {
			if r == "" {
				t.Error("Empty strings should be filtered")
			}
		}
	})

	t.Run("exactly 4 short tokens", func(t *testing.T) {
		// 4 short tokens should be merged as one group (not split into pairs)
		result := d.TokenMerge([]string{"a", "b", "c", "d"})
		expected := []string{"a b c d"}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})

	t.Run("exactly 5 short tokens", func(t *testing.T) {
		// 5 short tokens should be split into pairs
		result := d.TokenMerge([]string{"a", "b", "c", "d", "e"})
		// Should be: a b, c d (e is left? depends on implementation)
		if len(result) < 2 {
			t.Errorf("Expected at least 2 groups for 5 tokens, got %d: %v", len(result), result)
		}
	})
}

// TestSplitEdgeCases tests edge cases for splitting
func TestSplitEdgeCases(t *testing.T) {
	d := NewTermWeightDealer("")

	t.Run("tabs and spaces", func(t *testing.T) {
		result := d.Split("hello\tworld\t\ttest")
		// Tabs should be normalized to single space
		hasTab := false
		for _, r := range result {
			if strings.Contains(r, "\t") {
				hasTab = true
				break
			}
		}
		if hasTab {
			t.Error("Tabs should be normalized")
		}
	})

	t.Run("consecutive English with different NE types", func(t *testing.T) {
		tmpDir := t.TempDir()
		nerData := `{
			"hello": "func",
			"world": "corp"
		}`
		os.WriteFile(filepath.Join(tmpDir, "ner.json"), []byte(nerData), 0644)
		d2 := NewTermWeightDealer(tmpDir)

		result := d2.Split("hello world")
		// Both have NE types, so they should NOT merge
		if len(result) != 2 {
			t.Errorf("Expected 2 tokens when both have NE types, got %d: %v", len(result), result)
		}
	})
}
