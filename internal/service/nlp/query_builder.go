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
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"ragflow/internal/engine/infinity"
	"ragflow/internal/tokenizer"

	"github.com/siongui/gojianfan"
)

var (
	// globalQueryBuilder is the global query builder instance
	globalQueryBuilder *QueryBuilder
	// qbOnce ensures the query builder is initialized only once
	qbOnce sync.Once
	// qbInitError stores any error during initialization
	qbInitError error
)

// QueryBuilder provides functionality to build query expressions based on text, referencing Python's FulltextQueryer and QueryBase.
type QueryBuilder struct {
	queryFields []string
	termWeight  *TermWeightDealer
	synonym     *Synonym
}

// InitQueryBuilder initializes the global QueryBuilder with the given wordnet directory.
// It should be called during the initialization phase of main.go, after tokenizer.Init.
// The wordnetDir is typically filepath.Join(tokenizer.Config.DictPath, "wordnet")
func InitQueryBuilder(wordnetDir string) error {
	qbOnce.Do(func() {
		globalQueryBuilder = &QueryBuilder{
			queryFields: []string{
				"title_tks^10",
				"title_sm_tks^5",
				"important_kwd^30",
				"important_tks^20",
				"question_tks^20",
				"content_ltks^2",
				"content_sm_ltks",
			},
			termWeight: NewTermWeightDealer(""),
			synonym:    NewSynonym(nil, "", wordnetDir),
		}
	})
	return qbInitError
}

// InitQueryBuilderFromTokenizer initializes the global QueryBuilder using tokenizer's DictPath.
// The wordnet directory is derived from tokenizer's DictPath as: DictPath/wordnet
// This should be called after tokenizer.Init().
func InitQueryBuilderFromTokenizer(tokenizerDictPath string) error {
	wordnetDir := filepath.Join(tokenizerDictPath, "wordnet")
	return InitQueryBuilder(wordnetDir)
}

// GetQueryBuilder returns the global QueryBuilder instance.
// Returns nil if InitQueryBuilder has not been called.
func GetQueryBuilder() *QueryBuilder {
	return globalQueryBuilder
}

// NewQueryBuilder creates a new QueryBuilder with default query fields.
// Deprecated: Use GetQueryBuilder() to get the global instance for better performance.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		queryFields: []string{
			"title_tks^10",
			"title_sm_tks^5",
			"important_kwd^30",
			"important_tks^20",
			"question_tks^20",
			"content_ltks^2",
			"content_sm_ltks",
		},
		termWeight: NewTermWeightDealer(""),
		synonym:    NewSynonym(nil, "", ""),
	}
}

// IsChinese determines whether a line of text is primarily Chinese.
// Algorithm: split by whitespace, if segments <=3 return true; otherwise count ratio of non-pure-alphabet segments, return true if ratio >=0.7.
func (qb *QueryBuilder) IsChinese(line string) bool {
	fields := strings.Fields(line)
	if len(fields) <= 3 {
		return true
	}
	nonAlpha := 0
	for _, f := range fields {
		matched, _ := regexp.MatchString(`^[a-zA-Z]+$`, f)
		if !matched {
			nonAlpha++
		}
	}
	return float64(nonAlpha)/float64(len(fields)) >= 0.7
}

// SubSpecialChar escapes special characters for use in queries.
func (qb *QueryBuilder) SubSpecialChar(line string) string {
	// Regex matches : { } / [ ] - * " ( ) | + ~ ^ and prepends backslash
	re := regexp.MustCompile(`([:{}/\[\]\-\*"\(\)\|\+~\^])`)
	return re.ReplaceAllString(line, `\$1`)
}

// RmWWW removes common stop words and question words from queries.
func (qb *QueryBuilder) RmWWW(txt string) string {
	patterns := []struct {
		regex string
		repl  string
	}{
		// Chinese stop words
		{`是*(怎么办|什么样的|哪家|一下|那家|请问|啥样|咋样了|什么时候|何时|何地|何人|是否|是不是|多少|哪里|怎么|哪儿|怎么样|如何|哪些|是啥|啥是|啊|吗|呢|吧|咋|什么|有没有|呀|谁|哪位|哪个)是*`, ""},
		// English stop words (case-insensitive)
		{`(^| )(what|who|how|which|where|why)('re|'s)? `, " "},
		{`(^| )('s|'re|is|are|were|was|do|does|did|don't|doesn't|didn't|has|have|be|there|you|me|your|my|mine|just|please|may|i|should|would|wouldn't|will|won't|done|go|for|with|so|the|a|an|by|i'm|it's|he's|she's|they|they're|you're|as|by|on|in|at|up|out|down|of|to|or|and|if) `, " "},
	}
	original := txt
	for _, p := range patterns {
		re := regexp.MustCompile(`(?i)` + p.regex)
		txt = re.ReplaceAllString(txt, p.repl)
	}
	if txt == "" {
		txt = original
	}
	return txt
}

// AddSpaceBetweenEngZh adds spaces between English letters and Chinese characters to improve tokenization.
func (qb *QueryBuilder) AddSpaceBetweenEngZh(txt string) string {
	// (ENG/ENG+NUM) + ZH: e.g., "ABC123中文" -> "ABC123 中文"
	re1 := regexp.MustCompile(`([A-Za-z]+[0-9]*)([\x{4e00}-\x{9fa5}]+)`)
	txt = re1.ReplaceAllString(txt, "$1 $2")

	// ENG + ZH: e.g., "ABC中文" -> "ABC 中文"
	re2 := regexp.MustCompile(`([A-Za-z])([\x{4e00}-\x{9fa5}]+)`)
	txt = re2.ReplaceAllString(txt, "$1 $2")

	// ZH + (ENG/ENG+NUM): e.g., "中文ABC123" -> "中文 ABC123"
	re3 := regexp.MustCompile(`([\x{4e00}-\x{9fa5}]+)([A-Za-z]+[0-9]*)`)
	txt = re3.ReplaceAllString(txt, "$1 $2")

	// ZH + ENG: e.g., "中文ABC" -> "中文 ABC"
	re4 := regexp.MustCompile(`([\x{4e00}-\x{9fa5}]+)([A-Za-z])`)
	txt = re4.ReplaceAllString(txt, "$1 $2")
	return txt
}

// StrFullWidth2HalfWidth converts full-width characters to half-width characters.
// Algorithm: For each character:
//   - Full-width space (U+3000) is converted to half-width space (U+0020).
//   - For other characters, subtract 0xFEE0 from its code point.
//   - If the resulting code point is not in the half-width character range (0x0020 to 0x7E),
//     the original character is kept.
func (qb *QueryBuilder) StrFullWidth2HalfWidth(ustring string) string {
	var rstring strings.Builder
	for _, uchar := range ustring {
		insideCode := int32(uchar)
		if insideCode == 0x3000 {
			insideCode = 0x0020
		} else {
			insideCode -= 0xFEE0
		}
		if insideCode < 0x0020 || insideCode > 0x7E {
			rstring.WriteRune(uchar)
		} else {
			rstring.WriteRune(insideCode)
		}
	}
	return rstring.String()
}

// Traditional2Simplified converts traditional Chinese characters to simplified Chinese characters.
// Uses gojianfan library which provides conversion similar to Python's HanziConv.
func (qb *QueryBuilder) Traditional2Simplified(line string) string {
	return gojianfan.T2S(line)
}

// NeedFineGrainedTokenize determines if fine-grained tokenization is needed for a token.
// Reference: rag/nlp/query.py L88-93
func (qb *QueryBuilder) NeedFineGrainedTokenize(tk string) bool {
	if len(tk) < 3 {
		return false
	}
	if matched, _ := regexp.MatchString(`^[0-9a-z\.\+#_\*-]+$`, tk); matched {
		return false
	}
	return true
}

// Question builds a full-text query expression based on input text.
// References Python FulltextQueryer.question method.
// Currently, a simplified version, returns basic MatchTextExpr; future integration of term weight and synonyms.
func (qb *QueryBuilder) Question(txt string, tbl string, minMatch float64) (*infinity.MatchTextExpr, []string) {
	originalQuery := txt

	// Add space between English and Chinese
	txtWithSpaces := qb.AddSpaceBetweenEngZh(txt)

	// Convert to lowercase and remove punctuation (simplified)
	txtLower := strings.ToLower(txtWithSpaces)

	// Convert to half-width
	txtHalfWidth := qb.StrFullWidth2HalfWidth(txtLower)

	// Convert to simplified Chinese
	txtSimplified := qb.Traditional2Simplified(txtHalfWidth)

	// Replace punctuation and special characters with space
	// Reference: rag/nlp/query.py L44-48
	re := regexp.MustCompile(`[ :|\r\n\t,，.。?？/\` + "`" + `!！&^%()\[\]{}<>]+`)
	txtCleaned := re.ReplaceAllString(txtSimplified, " ")

	// Remove stop words
	txtNoStopWords := qb.RmWWW(txtCleaned)

	// Determine if text is Chinese
	if !qb.IsChinese(txtNoStopWords) {
		// Non-Chinese processing
		// Reference: rag/nlp/query.py L52-88

		// Remove stop words again
		txtFinal := qb.RmWWW(txtNoStopWords)

		// Tokenize using rag_tokenizer
		tokenized, err := tokenizer.Tokenize(txtFinal)
		if err != nil {
			// If tokenizer fails, use simple split
			tokenized = txtFinal
		}

		tks := strings.Fields(tokenized)
		keywords := make([]string, 0, len(tks))
		for _, t := range tks {
			if t != "" {
				keywords = append(keywords, t)
			}
		}

		// Calculate term weights using TermWeightDealer
		// Reference: rag/nlp/query.py L56
		tws := qb.termWeight.Weights(tks, false)

		// Clean tokens and filter
		// Reference: rag/nlp/query.py L57-60
		type tokenWeight struct {
			tk string
			w  float64
		}
		var tksW []tokenWeight
		for _, tw := range tws {
			tk := tw.Term
			w := tw.Weight

			// Clean token: remove special chars
			tk = regexp.MustCompile(`[ \"'^]+`).ReplaceAllString(tk, "")
			// Remove single alphanumeric chars
			tk = regexp.MustCompile(`^[a-z0-9]$`).ReplaceAllString(tk, "")
			// Remove leading +/-
			tk = regexp.MustCompile(`^[\+\-]+`).ReplaceAllString(tk, "")
			tk = strings.TrimSpace(tk)

			if tk == "" {
				continue
			}
			tksW = append(tksW, tokenWeight{tk, w})
		}

		// Limit to 256 tokens
		// Reference: rag/nlp/query.py L62
		if len(tksW) > 256 {
			tksW = tksW[:256]
		}

		// TODO: Synonym expansion (reference L61-67)
		// For now, use empty synonyms
		syns := make([]string, len(tksW))

		// Build query parts
		// Reference: rag/nlp/query.py L69-70
		var q []string
		for i, tw := range tksW {
			tk := tw.tk
			w := tw.w
			// Skip tokens with special regex chars
			if matched, _ := regexp.MatchString(`[.^+\(\)-]`, tk); matched {
				continue
			}
			// Format: (token^weight synonym)
			q = append(q, fmt.Sprintf("(%s^%.4f %s)", tk, w, syns[i]))
		}

		// Add phrase queries for adjacent tokens
		// Reference: rag/nlp/query.py L71-82
		for i := 1; i < len(tksW); i++ {
			left := strings.TrimSpace(tksW[i-1].tk)
			right := strings.TrimSpace(tksW[i].tk)
			if left == "" || right == "" {
				continue
			}
			maxW := tksW[i-1].w
			if tksW[i].w > maxW {
				maxW = tksW[i].w
			}
			q = append(q, fmt.Sprintf(`"%s %s"^%.4f`, left, right, maxW*2))
		}

		if len(q) == 0 {
			q = append(q, txtFinal)
		}

		query := strings.Join(q, " ")
		return &infinity.MatchTextExpr{
			Fields:       qb.queryFields,
			MatchingText: query,
			TopN:         100,
			ExtraOptions: map[string]interface{}{
				"original_query": originalQuery,
			},
		}, keywords
	}
	// Chinese processing
	// Reference: rag/nlp/query.py L88-172

	// Save original text before removing stop words (for fallback)
	otxt := txtNoStopWords

	// Remove stop words for Chinese processing
	txtChinese := qb.RmWWW(txtNoStopWords)

	var qs []string
	var keywords []string

	// Split text and process each segment (limit to 256)
	segments := qb.termWeight.Split(txtChinese)
	if len(segments) > 256 {
		segments = segments[:256]
	}

	for _, segment := range segments {
		if segment == "" {
			continue
		}
		keywords = append(keywords, segment)

		// Get term weights
		twts := qb.termWeight.Weights([]string{segment}, true)

		// Lookup synonyms
		syns := qb.synonym.Lookup(segment, 8)
		if len(syns) > 0 && len(keywords) < 32 {
			keywords = append(keywords, syns...)
		}

		// Sort by weight descending
		sort.Slice(twts, func(i, j int) bool {
			return twts[i].Weight > twts[j].Weight
		})

		var tms []struct {
			tk string
			w  float64
		}

		for _, tw := range twts {
			tk := tw.Term
			w := tw.Weight

			// Fine-grained tokenization if needed
			var sm []string
			if qb.NeedFineGrainedTokenize(tk) {
				fineGrained, err := tokenizer.FineGrainedTokenize(tk)
				if err == nil && fineGrained != "" {
					sm = strings.Fields(fineGrained)
				}
			}

			// Clean special characters from sm
			var cleanSm []string
			specialCharRe := regexp.MustCompile(`[,\.\/;'\[\]\\\` + "`" + `~!@#$%\^&\*\(\)=\+_<>\?:"\{\}\|，。；'‘’【】、！￥……（）——《》？："""-]+`)
			for _, m := range sm {
				m = specialCharRe.ReplaceAllString(m, "")
				m = qb.SubSpecialChar(m)
				if len(m) > 1 {
					cleanSm = append(cleanSm, m)
				}
			}
			sm = cleanSm

			// Add to keywords if under limit
			if len(keywords) < 32 {
				cleanTk := regexp.MustCompile(`[ \"']+`).ReplaceAllString(tk, "")
				if cleanTk != "" {
					keywords = append(keywords, cleanTk)
				}
				keywords = append(keywords, sm...)
			}

			// Lookup synonyms for this token
			tkSyns := qb.synonym.Lookup(tk, 8)
			for i, s := range tkSyns {
				tkSyns[i] = qb.SubSpecialChar(s)
			}
			if len(keywords) < 32 {
				for _, s := range tkSyns {
					if s != "" {
						keywords = append(keywords, s)
					}
				}
			}

			// Fine-grained tokenize synonyms
			var fineGrainedSyns []string
			for _, s := range tkSyns {
				if s == "" {
					continue
				}
				fg, err := tokenizer.FineGrainedTokenize(s)
				if err == nil && fg != "" {
					// Quote if contains space
					if strings.Contains(fg, " ") {
						fg = fmt.Sprintf(`"%s"`, fg)
					}
					fineGrainedSyns = append(fineGrainedSyns, fg)
				}
			}

			if len(keywords) >= 32 {
				break
			}

			// Clean token for query
			tk = qb.SubSpecialChar(tk)
			if tk == "" {
				continue
			}

			// Quote if contains space
			if strings.Contains(tk, " ") {
				tk = fmt.Sprintf(`"%s"`, tk)
			}

			// Build query part with synonyms
			if len(fineGrainedSyns) > 0 {
				tk = fmt.Sprintf("(%s OR (%s)^0.2)", tk, strings.Join(fineGrainedSyns, " "))
			}
			if len(sm) > 0 {
				smStr := strings.Join(sm, " ")
				tk = fmt.Sprintf(`%s OR "%s" OR ("%s"~2)^0.5`, tk, smStr, smStr)
			}

			tms = append(tms, struct {
				tk string
				w  float64
			}{tk, w})
		}

		// Build query string for this segment
		var tmsParts []string
		for _, tw := range tms {
			tmsParts = append(tmsParts, fmt.Sprintf("(%s)^%.4f", tw.tk, tw.w))
		}
		tmsStr := strings.Join(tmsParts, " ")

		// Add proximity query if multiple tokens
		if len(twts) > 1 {
			tokenized, _ := tokenizer.Tokenize(segment)
			if tokenized != "" {
				tmsStr += fmt.Sprintf(` ("%s"~2)^1.5`, tokenized)
			}
		}

		// Add segment-level synonyms
		if len(syns) > 0 && tmsStr != "" {
			var synParts []string
			for _, s := range syns {
				s = qb.SubSpecialChar(s)
				if s != "" {
					tokenized, _ := tokenizer.Tokenize(s)
					if tokenized != "" {
						synParts = append(synParts, fmt.Sprintf(`"%s"`, tokenized))
					}
				}
			}
			if len(synParts) > 0 {
				tmsStr = fmt.Sprintf("(%s)^5 OR (%s)^0.7", tmsStr, strings.Join(synParts, " OR "))
			}
		}

		if tmsStr != "" {
			qs = append(qs, tmsStr)
		}
	}

	// Build final query
	if len(qs) > 0 {
		var queryParts []string
		for _, q := range qs {
			if q != "" {
				queryParts = append(queryParts, fmt.Sprintf("(%s)", q))
			}
		}
		query := strings.Join(queryParts, " OR ")
		if query == "" {
			query = otxt
		}
		return &infinity.MatchTextExpr{
			Fields:       qb.queryFields,
			MatchingText: query,
			TopN:         100,
			ExtraOptions: map[string]interface{}{
				"minimum_should_match": minMatch,
				"original_query":       originalQuery,
			},
		}, keywords
	}

	return nil, keywords
}

// Paragraph builds a query expression based on content terms and keywords.
// References Python FulltextQueryer.paragraph method.
func (qb *QueryBuilder) Paragraph(contentTks string, keywords []string, keywordsTopN int) *infinity.MatchTextExpr {
	// Simplified implementation: merge keywords and content terms
	allTerms := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if k != "" {
			allTerms = append(allTerms, `"`+k+`"`)
		}
	}
	// Limit number of keywords
	if keywordsTopN > 0 && len(allTerms) > keywordsTopN {
		allTerms = allTerms[:keywordsTopN]
	}
	// Could add content term processing here, e.g., tokenization, weight calculation
	// Currently only uses keywords
	query := strings.Join(allTerms, " ")
	// Calculate minimum_should_match (could be used for extra_options in future)
	_ = 3
	if len(allTerms) > 0 {
		calc := int(float64(len(allTerms)) / 10.0)
		if calc < 3 {
			calc = 3
		}
		_ = calc
	}
	return &infinity.MatchTextExpr{
		Fields:       qb.queryFields,
		MatchingText: query,
		TopN:         100,
	}
}

// Similarity calculates similarity between two term weight dictionaries.
// Algorithm: s = sum(qtwt[k] for k in qtwt if k in dtwt) / sum(qtwt[k])
func (qb *QueryBuilder) Similarity(qtwt map[string]float64, dtwt map[string]float64) float64 {
	if len(qtwt) == 0 {
		return 0.0
	}
	var sum float64
	for k, v := range qtwt {
		if _, ok := dtwt[k]; ok {
			sum += v
		}
	}
	var total float64
	for _, v := range qtwt {
		total += v
	}
	if total == 0 {
		return 0.0
	}
	return sum / total
}

// TokenSimilarity calculates similarity between query terms and multiple document term sets.
// To be implemented: requires term weight processing module.
func (qb *QueryBuilder) TokenSimilarity(atks string, btkss []string) []float64 {
	// Placeholder implementation, returns zero values
	result := make([]float64, len(btkss))
	for i := range result {
		result[i] = 0.0
	}
	return result
}

// HybridSimilarity calculates weighted combination of vector similarity and term similarity.
// To be implemented: requires vector cosine similarity calculation.
func (qb *QueryBuilder) HybridSimilarity(avec []float64, bvecs [][]float64, atks string, btkss []string, tkweight float64, vtweight float64) ([]float64, []float64, []float64) {
	// Placeholder implementation, returns zero values
	n := len(btkss)
	sims := make([]float64, n)
	tksim := make([]float64, n)
	vecsim := make([]float64, n)
	return sims, tksim, vecsim
}

// SetQueryFields sets the list of query fields.
func (qb *QueryBuilder) SetQueryFields(fields []string) {
	qb.queryFields = fields
}
