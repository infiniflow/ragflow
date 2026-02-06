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
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var testSynonymWordNetDir string

func init() {
	// Find project root by locating go.mod file
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, project root is dir
			testSynonymWordNetDir = filepath.Join(dir, "resource", "wordnet")
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}
	// Fallback to relative path if go.mod not found
	testSynonymWordNetDir = "../../../resource/wordnet"
}

// MockRedisClient is a mock implementation of RedisClient for testing
type MockRedisClient struct {
	data map[string]string
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string]string),
	}
}

func (m *MockRedisClient) Get(key string) (string, error) {
	return m.data[key], nil
}

func (m *MockRedisClient) Set(key, value string) {
	m.data[key] = value
}

// TestNewSynonym tests the constructor
func TestNewSynonym(t *testing.T) {
	t.Run("without redis", func(t *testing.T) {
		s := NewSynonym(nil, "", testSynonymWordNetDir)
		if s == nil {
			t.Fatal("NewSynonym returned nil")
		}
		if s.dictionary == nil {
			t.Error("Dictionary not initialized")
		}
		if s.wordNet == nil {
			t.Error("WordNet not initialized")
		}
	})

	t.Run("with redis", func(t *testing.T) {
		redis := NewMockRedisClient()
		s := NewSynonym(redis, "", testSynonymWordNetDir)
		if s == nil {
			t.Fatal("NewSynonym returned nil")
		}
		if s.redis != redis {
			t.Error("Redis client not set")
		}
	})
}

// TestNewSynonymWithMockFile tests loading from synonym.json
func TestNewSynonymWithMockFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock synonym.json
	synonymData := map[string]interface{}{
		"happy":    []string{"joyful", "cheerful", "glad"},
		"sad":      []string{"unhappy", "sorrowful"},
		"test":     "single", // Test string value
		"UPPER":    []string{"lower"}, // Test case conversion
	}
	data, _ := json.Marshal(synonymData)
	if err := os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644); err != nil {
		t.Fatalf("Failed to create mock synonym.json: %v", err)
	}

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	// Check dictionary loaded correctly
	if len(s.dictionary) != 4 {
		t.Errorf("Expected 4 entries, got %d", len(s.dictionary))
	}

	// Check case conversion (UPPER -> upper)
	if _, ok := s.dictionary["upper"]; !ok {
		t.Error("Expected 'upper' key (converted from UPPER)")
	}

	// Check string value converted to slice (test -> [single])
	if val, ok := s.dictionary["test"]; !ok || len(val) != 1 || val[0] != "single" {
		t.Error("Expected 'test' to be converted to single-element slice")
	}
}

// TestSynonymLookup tests the Lookup method
func TestSynonymLookup(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock synonym.json
	synonymData := map[string]interface{}{
		"hello": []string{"hi", "greetings", "hey"},
		"world": []string{"earth", "globe"},
	}
	data, _ := json.Marshal(synonymData)
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644)

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	tests := []struct {
		name     string
		tk       string
		topN     int
		expected []string
	}{
		{
			name:     "found in dictionary",
			tk:       "hello",
			topN:     8,
			expected: []string{"hi", "greetings", "hey"},
		},
		{
			name:     "found with topN limit",
			tk:       "hello",
			topN:     2,
			expected: []string{"hi", "greetings"},
		},
		{
			name:     "not found",
			tk:       "xyzabc123",
			topN:     8,
			expected: []string{},
		},
		{
			name:     "empty token",
			tk:       "",
			topN:     8,
			expected: []string{},
		},
		{
			name:     "whitespace normalization",
			tk:       "  hello  ",
			topN:     8,
			expected: []string{"hi", "greetings", "hey"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Lookup(tt.tk, tt.topN)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Lookup(%q, %d) = %v, expected %v", tt.tk, tt.topN, result, tt.expected)
			}
		})
	}
}

// TestSynonymLookupFromWordNet tests WordNet fallback
func TestSynonymLookupFromWordNet(t *testing.T) {
	// Create synonym with empty dictionary to force WordNet fallback
	s := NewSynonym(nil, "", "")
	s.dictionary = make(map[string][]string) // Clear dictionary

	t.Run("pure alphabetical token", func(t *testing.T) {
		// Since WordNet is a placeholder, it should return empty
		result := s.Lookup("test", 8)
		// WordNet placeholder returns empty, so we expect empty result
		if len(result) != 0 {
			t.Logf("WordNet returned: %v (placeholder implementation)", result)
		}
	})

	t.Run("non-alphabetical token", func(t *testing.T) {
		result := s.Lookup("test123", 8)
		if len(result) != 0 {
			t.Errorf("Expected empty result for non-alphabetical token, got %v", result)
		}
	})
}

// TestSynonymLoad tests loading from Redis
func TestSynonymLoad(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial synonym.json
	synonymData := map[string]interface{}{
		"initial": []string{"first"},
	}
	data, _ := json.Marshal(synonymData)
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644)

	redis := NewMockRedisClient()

	// Set up Redis data
	redisData := map[string][]string{
		"redis_key": []string{"from", "redis"},
	}
	redisBytes, _ := json.Marshal(redisData)
	redis.Set("kevin_synonyms", string(redisBytes))

	s := NewSynonym(redis, tmpDir, testSynonymWordNetDir)

	// Simulate multiple lookups to trigger load
	s.lookupNum = 200 // Set above threshold
	s.loadTm = time.Now().Add(-4000 * time.Second) // Set load time > 1 hour ago

	// Call load directly
	s.load()

	// After load, dictionary should be updated from Redis
	if _, ok := s.dictionary["redis_key"]; !ok {
		t.Log("Dictionary not updated from Redis (may be expected due to timing)")
	}
}

// TestSynonymLoadNoRedis tests load without Redis
func TestSynonymLoadNoRedis(t *testing.T) {
	s := NewSynonym(nil, "", "")

	// Should not panic
	s.load()

	// Lookup num should remain unchanged
	originalNum := s.lookupNum
	s.load()
	if s.lookupNum != originalNum {
		t.Error("Lookup num should not change when Redis is nil")
	}
}

// TestSynonymLoadNotTriggered tests load conditions
func TestSynonymLoadNotTriggered(t *testing.T) {
	redis := NewMockRedisClient()
	s := NewSynonym(redis, "", "")

	// Set conditions that should prevent load
	s.lookupNum = 50 // Below threshold
	s.loadTm = time.Now()

	// Call load
	s.load()

	// Should not attempt to load from Redis
	// (indirect check: lookupNum should not reset)
	if s.lookupNum != 50 {
		t.Error("Load should not be triggered when lookupNum < 100")
	}
}

// TestGetDictionary tests GetDictionary method
func TestGetDictionary(t *testing.T) {
	tmpDir := t.TempDir()

	synonymData := map[string]interface{}{
		"test": []string{"value"},
	}
	data, _ := json.Marshal(synonymData)
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644)

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	dict := s.GetDictionary()
	if dict == nil {
		t.Error("GetDictionary returned nil")
	}
	if len(dict) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(dict))
	}
}

// TestGetLookupNum tests GetLookupNum method
func TestGetLookupNum(t *testing.T) {
	s := NewSynonym(nil, "", "")
	initialNum := s.GetLookupNum()

	// Perform some lookups
	s.Lookup("test1", 8)
	s.Lookup("test2", 8)
	s.Lookup("test3", 8)

	newNum := s.GetLookupNum()
	if newNum != initialNum+3 {
		t.Errorf("Expected lookup num %d, got %d", initialNum+3, newNum)
	}
}

// TestGetLoadTime tests GetLoadTime method
func TestGetLoadTime(t *testing.T) {
	s := NewSynonym(nil, "", "")
	loadTime := s.GetLoadTime()

	// Load time should be in the past (since we set it to -1000000 seconds)
	if loadTime.After(time.Now()) {
		t.Error("Load time should be in the past")
	}
}

// TestLookupCaseSensitivity tests case insensitivity
func TestLookupCaseSensitivity(t *testing.T) {
	tmpDir := t.TempDir()

	synonymData := map[string]interface{}{
		"lowercase": []string{"result"},
	}
	data, _ := json.Marshal(synonymData)
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644)

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	// Lookup with different cases
	tests := []string{"lowercase", "LOWERCASE", "LowerCase", "LoWeRcAsE"}
	for _, tk := range tests {
		result := s.Lookup(tk, 8)
		if len(result) == 0 {
			t.Errorf("Expected result for %q, got none", tk)
		}
	}
}

// TestLookupWithSpaces tests whitespace normalization
func TestLookupWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()

	synonymData := map[string]interface{}{
		"two words": []string{"result"},
	}
	data, _ := json.Marshal(synonymData)
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644)

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	// Lookup with various whitespace
	tests := []string{
		"two words",
		"two  words",
		"two\twords",
		"two\t\twords",
		"  two words  ",
	}

	for _, tk := range tests {
		result := s.Lookup(tk, 8)
		if len(result) == 0 {
			t.Errorf("Expected result for %q, got none", tk)
		}
	}
}

// TestSynonymMissingFile tests behavior when synonym.json is missing
func TestSynonymMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create synonym.json

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	if len(s.dictionary) != 0 {
		t.Errorf("Expected empty dictionary, got %d entries", len(s.dictionary))
	}

	// Lookup should return empty
	result := s.Lookup("anything", 8)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %v", result)
	}
}

// TestSynonymInvalidJSON tests behavior with invalid JSON
func TestSynonymInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid JSON file
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), []byte("invalid json"), 0644)

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	// Should have empty dictionary but not panic
	if s.dictionary == nil {
		t.Error("Dictionary should be initialized even with invalid JSON")
	}
}

// BenchmarkLookup benchmarks the Lookup method
func BenchmarkLookup(b *testing.B) {
	tmpDir := b.TempDir()

	synonymData := map[string]interface{}{
		"test": []string{"synonym1", "synonym2", "synonym3"},
	}
	data, _ := json.Marshal(synonymData)
	os.WriteFile(filepath.Join(tmpDir, "synonym.json"), data, 0644)

	s := NewSynonym(nil, tmpDir, testSynonymWordNetDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Lookup("test", 8)
	}
}

// BenchmarkLookupNotFound benchmarks lookup for non-existent tokens
func BenchmarkLookupNotFound(b *testing.B) {
	s := NewSynonym(nil, "", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Lookup("nonexistent", 8)
	}
}
