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
	"regexp"
	"strings"
	"time"

	"ragflow/internal/logger"

	"go.uber.org/zap"
)

// Synonym provides synonym lookup functionality
// Reference: rag/nlp/synonym.py Dealer class
type Synonym struct {
	lookupNum  int
	loadTm     time.Time
	dictionary map[string][]string
	redis      RedisClient // Optional Redis client for real-time synonym loading
	wordNet    *WordNet
	resPath    string
}

// RedisClient interface for Redis operations
// This should be implemented by the caller if Redis support is needed
type RedisClient interface {
	Get(key string) (string, error)
}

// NewSynonym creates a new Synonym instance
// Reference: synonym.py Dealer.__init__
// wordnetDir: path to wordnet directory (e.g., "/usr/share/infinity/resource/wordnet").
//
//	If empty, WordNet will not be initialized.
func NewSynonym(redis RedisClient, resPath string, wordnetDir string) *Synonym {
	s := &Synonym{
		lookupNum:  100000000,
		loadTm:     time.Now().Add(-1000000 * time.Second),
		dictionary: make(map[string][]string),
		redis:      redis,
		wordNet:    nil, // Will be initialized below
		resPath:    resPath,
	}

	if resPath == "" {
		s.resPath = "rag/res"
	}

	// Initialize WordNet with provided path
	if wordnetDir != "" {
		wordNet, err := NewWordNet(wordnetDir)
		if err != nil {
			// WordNet is optional, continue without it
			s.wordNet = nil
		} else {
			s.wordNet = wordNet
		}
	}

	// Load synonym.json
	path := filepath.Join(s.resPath, "synonym.json")
	if data, err := os.ReadFile(path); err == nil {
		var dict map[string]interface{}
		if err := json.Unmarshal(data, &dict); err == nil {
			// Convert to lowercase keys and string slices
			for k, v := range dict {
				key := strings.ToLower(k)
				switch val := v.(type) {
				case string:
					s.dictionary[key] = []string{val}
				case []interface{}:
					strSlice := make([]string, 0, len(val))
					for _, item := range val {
						if str, ok := item.(string); ok {
							strSlice = append(strSlice, str)
						}
					}
					s.dictionary[key] = strSlice
				}
			}
		} else {
			logger.Warn("Failed to parse synonym.json", zap.Error(err))
		}
	} else {
		logger.Warn("Missing synonym.json", zap.Error(err))
	}

	if redis == nil {
		logger.Warn("Realtime synonym is disabled, since no redis connection.")
	}

	if len(s.dictionary) == 0 {
		logger.Warn("Fail to load synonym")
	}

	s.load()

	return s
}

// load loads synonyms from Redis if available
// Reference: synonym.py Dealer.load
func (s *Synonym) load() {
	//if s.redis == nil {
	//	return
	//}
	//
	//if s.lookupNum < 100 {
	//	return
	//}
	//
	//tm := time.Now()
	//if tm.Sub(s.loadTm).Seconds() < 3600 {
	//	return
	//}
	//
	//s.loadTm = time.Now()
	//s.lookupNum = 0
	//
	//data, err := s.redis.Get("kevin_synonyms")
	//if err != nil || data == "" {
	//	return
	//}
	//
	//var dict map[string][]string
	//if jsonErr := json.Unmarshal([]byte(data), &dict); jsonErr != nil {
	//	logger.Error("Fail to load synonym!", jsonErr)
	//	return
	//}
	//
	//s.dictionary = dict
}

// Lookup looks up synonyms for a given token
// Reference: synonym.py Dealer.lookup
func (s *Synonym) Lookup(tk string, topN int) []string {
	if tk == "" {
		return []string{}
	}

	if topN <= 0 {
		topN = 8
	}

	// 1) Check the custom dictionary first
	//s.lookupNum++
	//s.load()

	key := regexp.MustCompile(`[ \t]+`).ReplaceAllString(strings.TrimSpace(tk), " ")
	key = strings.ToLower(key)

	if res, ok := s.dictionary[key]; ok {
		if len(res) > topN {
			return res[:topN]
		}
		return res
	}

	// 2) If not found and tk is purely alphabetical, fallback to WordNet
	if matched, _ := regexp.MatchString(`^[a-z]+$`, tk); matched && s.wordNet != nil {
		wnSet := make(map[string]struct{})
		synsets := s.wordNet.Synsets(tk, "")
		for _, syn := range synsets {
			// Extract word from synset name (format: word.pos.num)
			parts := strings.Split(syn.Name, ".")
			if len(parts) > 0 {
				word := strings.ReplaceAll(parts[0], "_", " ")
				wnSet[word] = struct{}{}
			}
		}
		// Remove the original token itself
		delete(wnSet, tk)

		// Convert to slice
		wnRes := make([]string, 0, len(wnSet))
		for w := range wnSet {
			if w != "" {
				wnRes = append(wnRes, w)
			}
		}

		if len(wnRes) > topN {
			return wnRes[:topN]
		}
		return wnRes
	}

	// 3) Nothing found in either source
	return []string{}
}

// GetDictionary returns the synonym dictionary
func (s *Synonym) GetDictionary() map[string][]string {
	return s.dictionary
}

// GetLookupNum returns the number of lookups since last load
func (s *Synonym) GetLookupNum() int {
	return s.lookupNum
}

// GetLoadTime returns the last load time
func (s *Synonym) GetLoadTime() time.Time {
	return s.loadTm
}
