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
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ragflow/internal/logger"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

// TermWeightDealer calculates term weights for text processing
// Reference: rag/nlp/term_weight.py
type TermWeightDealer struct {
	stopWords map[string]struct{}
	ne        map[string]string // named entities
	df        map[string]int    // document frequency
}

// TermWeight represents a term and its weight
type TermWeight struct {
	Term   string
	Weight float64
}

// NewTermWeightDealer creates a new TermWeightDealer
func NewTermWeightDealer(resPath string) *TermWeightDealer {
	d := &TermWeightDealer{
		stopWords: initStopWords(),
		ne:        make(map[string]string),
		df:        make(map[string]int),
	}

	// Load named entity dictionary
	if resPath == "" {
		resPath = "rag/res"
	}

	nerPath := filepath.Join(resPath, "ner.json")
	if data, err := os.ReadFile(nerPath); err == nil {
		if err := json.Unmarshal(data, &d.ne); err != nil {
			logger.Warn("Failed to load ner.json", zap.Error(err))
		}
	} else {
		logger.Warn("Failed to load ner.json", zap.Error(err))
	}

	// Load term frequency dictionary
	freqPath := filepath.Join(resPath, "term.freq")
	d.df = loadDict(freqPath)

	return d
}

// initStopWords initializes the stop words set
func initStopWords() map[string]struct{} {
	words := []string{
		"请问", "您", "你", "我", "他", "是", "的", "就", "有", "于",
		"及", "即", "在", "为", "最", "有", "从", "以", "了", "将",
		"与", "吗", "吧", "中", "#", "什么", "怎么", "哪个", "哪些",
		"啥", "相关",
	}
	stopWords := make(map[string]struct{}, len(words))
	for _, w := range words {
		stopWords[w] = struct{}{}
	}
	return stopWords
}

// loadDict loads a dictionary file
// Format: term\tfreq or just term
func loadDict(fnm string) map[string]int {
	res := make(map[string]int)
	data, err := os.ReadFile(fnm)
	if err != nil {
		logger.Warn("Failed to load dictionary", zap.String("file", fnm), zap.Error(err))
		return res
	}

	lines := strings.Split(string(data), "\n")
	totalFreq := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		arr := strings.Split(line, "\t")
		if len(arr) >= 2 {
			if freq, err := strconv.Atoi(arr[1]); err == nil {
				res[arr[0]] = freq
				totalFreq += freq
			}
		} else {
			res[arr[0]] = 0
		}
	}

	// If no frequencies, return as set (all 0)
	if totalFreq == 0 {
		return res
	}
	return res
}

// Pretoken preprocesses and tokenizes text
// Reference: term_weight.py L92-114
func (d *TermWeightDealer) Pretoken(txt string, num bool, stpwd bool) []string {
	patt := `[~—\t @#%!<>,\.\?":;'\{\}\[\]_=\(\)\|，。？》•●○↓《；'：""【¥ 】…￥！、·（）×\` + "`" + `&/「」\]`

	res := []string{}
	tokenized, err := tokenizer.Tokenize(txt)
	if err != nil {
		// Fallback to simple split
		tokenized = txt
	}

	for _, t := range strings.Fields(tokenized) {
		tk := t
		// Check stop words
		if stpwd {
			if _, isStop := d.stopWords[tk]; isStop {
				continue
			}
		}
		// Check single digit (unless num is true)
		if matched, _ := regexp.MatchString("^[0-9]$", tk); matched && !num {
			continue
		}
		// Check patterns
		if matched, _ := regexp.MatchString(patt, t); matched {
			tk = "#"
		}
		if tk != "#" && tk != "" {
			res = append(res, tk)
		}
	}
	return res
}

// TokenMerge merges short tokens into phrases
// Reference: term_weight.py L116-143
func (d *TermWeightDealer) TokenMerge(tks []string) []string {
	oneTerm := func(t string) bool {
		// Use rune count for proper Unicode handling
		runeCount := len([]rune(t))
		if runeCount == 1 {
			return true
		}
		// Match 1-2 alphanumeric characters
		matched, _ := regexp.MatchString("^[0-9a-z]{1,2}$", t)
		return matched
	}

	if len(tks) == 0 {
		return []string{}
	}

	res := []string{}
	i := 0
	for i < len(tks) {
		// Special case: first term is single char and next is multi-char Chinese
		if i == 0 && len(tks) > 1 && oneTerm(tks[i]) {
			nextLen := len([]rune(tks[i+1]))
			isNextMultiChar := nextLen > 1
			isNextNotAlnum, _ := regexp.MatchString("^[0-9a-zA-Z]", tks[i+1])
			if isNextMultiChar && !isNextNotAlnum {
				res = append(res, tks[0]+" "+tks[1])
				i = 2
				continue
			}
		}

		j := i
		for j < len(tks) && tks[j] != "" {
			if _, isStop := d.stopWords[tks[j]]; isStop {
				break
			}
			if !oneTerm(tks[j]) {
				break
			}
			j++
		}

		if j-i > 1 {
			if j-i < 5 {
				res = append(res, strings.Join(tks[i:j], " "))
				i = j
			} else {
				// Split into pairs for 5+ consecutive short tokens
				for k := i; k < j; k += 2 {
					if k+1 < j {
						res = append(res, tks[k]+" "+tks[k+1])
					} else {
						res = append(res, tks[k])
					}
				}
				i = j
			}
		} else {
			if len(tks[i]) > 0 {
				res = append(res, tks[i])
			}
			i++
		}
	}

	// Filter empty strings
	filtered := []string{}
	for _, t := range res {
		if t != "" {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// Ner gets named entity type for a term
// Reference: term_weight.py L145-150
func (d *TermWeightDealer) Ner(t string) string {
	if d.ne == nil {
		return ""
	}
	if res, ok := d.ne[t]; ok {
		return res
	}
	return ""
}

// Split splits text into tokens, merging consecutive English words
// Reference: term_weight.py L152-161
func (d *TermWeightDealer) Split(txt string) []string {
	if txt == "" {
		return []string{""}
	}

	tks := []string{}
	// Normalize spaces (tabs and multiple spaces -> single space)
	txt = regexp.MustCompile("[ \\t]+").ReplaceAllString(txt, " ")
	txt = strings.TrimSpace(txt)

	for _, t := range strings.Split(txt, " ") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if len(tks) > 0 {
			prevEndsWithLetter, _ := regexp.MatchString(".*[a-zA-Z]$", tks[len(tks)-1])
			currEndsWithLetter, _ := regexp.MatchString(".*[a-zA-Z]$", t)
			prevNE := d.ne[tks[len(tks)-1]]
			currNE := d.ne[t]
			if prevEndsWithLetter && currEndsWithLetter &&
				currNE != "func" && prevNE != "func" {
				tks[len(tks)-1] = tks[len(tks)-1] + " " + t
				continue
			}
		}
		tks = append(tks, t)
	}
	return tks
}

// Weights calculates weights for tokens
// Reference: term_weight.py L163-246
func (d *TermWeightDealer) Weights(tks []string, preprocess bool) []TermWeight {
	numPattern := regexp.MustCompile("^[0-9,.]{2,}$")
	shortLetterPattern := regexp.MustCompile("^[a-z]{1,2}$")
	numSpacePattern := regexp.MustCompile("^[0-9. -]{2,}$")
	letterPattern := regexp.MustCompile("^[a-z. -]+$")

	// ner weight function
	nerWeight := func(t string) float64 {
		if numPattern.MatchString(t) {
			return 2
		}
		if shortLetterPattern.MatchString(t) {
			return 0.01
		}
		if d.ne == nil {
			return 1
		}
		if neType, ok := d.ne[t]; ok {
			weights := map[string]float64{
				"toxic": 2, "func": 1, "corp": 3, "loca": 3,
				"sch": 3, "stock": 3, "firstnm": 1,
			}
			if w, exists := weights[neType]; exists {
				return w
			}
		}
		return 1
	}

	// postag weight function (simplified without POS tagger)
	postagWeight := func(t string) float64 {
		// Simple heuristic based on term characteristics
		// Numbers
		if matched, _ := regexp.MatchString("^[0-9-]+", t); matched {
			return 2
		}
		// Single English letters
		if matched, _ := regexp.MatchString("^[a-zA-Z]$", t); matched {
			return 0.3
		}
		// Multi-character Chinese terms (likely nouns)
		if len([]rune(t)) >= 2 && !letterPattern.MatchString(t) {
			return 2
		}
		return 1
	}

	// freq function (simplified without frequency dictionary)
	var freq func(t string) float64
	freq = func(t string) float64 {
		if numSpacePattern.MatchString(t) {
			return 3
		}
		// Estimate frequency based on term characteristics
		// Long English terms are rare
		if letterPattern.MatchString(t) && len(t) >= 4 {
			return 300
		}
		// Very long terms get higher rarity score
		if len([]rune(t)) >= 4 {
			// Try fine-grained tokenization
			fgTokens, _ := tokenizer.Tokenize(t)
			var validTokens []float64
			for _, tt := range strings.Fields(fgTokens) {
				if len([]rune(tt)) > 1 {
					f := freq(tt)
					validTokens = append(validTokens, f)
				}
			}
			if len(validTokens) > 1 {
				minVal := validTokens[0]
				for _, v := range validTokens[1:] {
					if v < minVal {
						minVal = v
					}
				}
				return minVal / 6.0
			}
		}
		// Default frequency
		return 10
	}

	// df function
	var df func(t string) float64
	df = func(t string) float64 {
		if numSpacePattern.MatchString(t) {
			return 5
		}
		if v, ok := d.df[t]; ok {
			return float64(v) + 3
		}
		if letterPattern.MatchString(t) {
			return 300
		}
		if len([]rune(t)) >= 4 {
			fgTokens, _ := tokenizer.Tokenize(t)
			var validTokens []float64
			for _, tt := range strings.Fields(fgTokens) {
				if len([]rune(tt)) > 1 {
					validTokens = append(validTokens, df(tt))
				}
			}
			if len(validTokens) > 1 {
				minVal := validTokens[0]
				for _, v := range validTokens[1:] {
					if v < minVal {
						minVal = v
					}
				}
				return math.Max(3, minVal/6.0)
			}
		}
		return 3
	}

	// idf function
	idf := func(s, N float64) float64 {
		return math.Log10(10 + ((N-s+0.5)/(s+0.5)))
	}

	tw := []TermWeight{}

	if !preprocess {
		// Direct calculation without preprocessing
		idf1Vals := make([]float64, len(tks))
		idf2Vals := make([]float64, len(tks))
		nerPosVals := make([]float64, len(tks))

		for i, t := range tks {
			idf1Vals[i] = idf(freq(t), 10000000)
			idf2Vals[i] = idf(df(t), 1000000000)
			nerPosVals[i] = nerWeight(t) * postagWeight(t)
		}

		wts := make([]float64, len(tks))
		for i := range tks {
			wts[i] = (0.3*idf1Vals[i] + 0.7*idf2Vals[i]) * nerPosVals[i]
		}

		for i, t := range tks {
			tw = append(tw, TermWeight{Term: t, Weight: wts[i]})
		}
	} else {
		// With preprocessing
		for _, tk := range tks {
			tt := d.TokenMerge(d.Pretoken(tk, true, true))
			if len(tt) == 0 {
				continue
			}

			idf1Vals := make([]float64, len(tt))
			idf2Vals := make([]float64, len(tt))
			nerPosVals := make([]float64, len(tt))

			for i, t := range tt {
				idf1Vals[i] = idf(freq(t), 10000000)
				idf2Vals[i] = idf(df(t), 1000000000)
				nerPosVals[i] = nerWeight(t) * postagWeight(t)
			}

			wts := make([]float64, len(tt))
			for i := range tt {
				wts[i] = (0.3*idf1Vals[i] + 0.7*idf2Vals[i]) * nerPosVals[i]
			}

			for i, t := range tt {
				tw = append(tw, TermWeight{Term: t, Weight: wts[i]})
			}
		}
	}

	// Normalize weights
	if len(tw) == 0 {
		return tw
	}

	S := 0.0
	for _, twItem := range tw {
		S += twItem.Weight
	}

	if S > 0 {
		for i := range tw {
			tw[i].Weight = tw[i].Weight / S
		}
	}

	return tw
}

// GetStopWords returns the stop words set
func (d *TermWeightDealer) GetStopWords() map[string]struct{} {
	return d.stopWords
}

// GetNE returns the named entity dictionary
func (d *TermWeightDealer) GetNE() map[string]string {
	return d.ne
}

// GetDF returns the document frequency dictionary
func (d *TermWeightDealer) GetDF() map[string]int {
	return d.df
}
