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
	"reflect"
	"testing"

	"ragflow/internal/engine/types"
)

func TestNewQueryBuilder(t *testing.T) {
	qb := NewQueryBuilder()
	if qb == nil {
		t.Fatal("NewQueryBuilder returned nil")
	}
	// Check default fields
	expectedFields := []string{
		"title_tks^10",
		"title_sm_tks^5",
		"important_kwd^30",
		"important_tks^20",
		"question_tks^20",
		"content_ltks^2",
		"content_sm_ltks",
	}
	if !reflect.DeepEqual(qb.queryFields, expectedFields) {
		t.Errorf("Default query fields mismatch, got %v, want %v", qb.queryFields, expectedFields)
	}
}

func TestQueryBuilder_IsChinese(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"Empty", "", true}, // fields <=3
		{"Single Chinese char", "中", true},
		{"Two Chinese chars", "中文", true},
		{"Three Chinese chars", "中文字", true},
		{"Four Chinese chars", "中文字符", true},                // ratio >=0.7
		{"Mixed with English", "hello world", true},         // fields=2 <=3
		{"Mostly Chinese", "hello 世界 测试", true},             // fields=3 <=3
		{"Mostly English", "hello world test", true},        // fields=3 <=3
		{"English with punctuation", "Hello, world!", true}, // fields=2 <=3 (after split)
		{"Chinese with spaces", "这 是 一个 测试", true},          // fields=4, non-alpha=4, ratio=1 >=0.7
		{"Mixed with numbers", "123 abc", true},             // fields=2 <=3
		// Additional cases where fields >3 and ratio determines result
		{"Many English words", "this is a long english sentence", false}, // fields=6, non-alpha=0, ratio=0 <0.7
		{"Mixed with mostly Chinese", "hello world 中文 测试 多个", false},     // fields=5, non-alpha=3, ratio=0.6 <0.7 => false
		{"Mostly Chinese with many words", "这 是 一个 中文 测试 多个 汉字", true},   // fields=7, non-alpha=7, ratio=1 >=0.7
		{"English with Chinese suffix", "hello world 中文", true},          // fields=3 <=3
		{"Chinese with English suffix", "中文 test", true},                 // fields=2 <=3
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.IsChinese(tt.line)
			if result != tt.expected {
				t.Errorf("IsChinese(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_SubSpecialChar(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"No special chars", "hello world", "hello world"},
		{"Colon", "test: colon", `test\: colon`},
		{"Curly braces", "{braces}", `\{braces\}`},
		{"Slash", "path/to/file", `path\/to\/file`},
		{"Square brackets", "[brackets]", `\[brackets\]`},
		{"Hyphen", "a-b-c", `a\-b\-c`},
		{"Asterisk", "a*b", `a\*b`},
		{"Quote", `"quote"`, `\"quote\"`},
		{"Parentheses", "(parens)", `\(parens\)`},
		{"Pipe", "a|b", `a\|b`},
		{"Plus", "a+b", `a\+b`},
		{"Tilde", "~tilde", `\~tilde`},
		{"Caret", "^caret", `\^caret`},
		{"Multiple", `:{}/[]-*"()|+~^`, `\:\{\}\/\[\]\-\*\"\(\)\|\+\~\^`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.SubSpecialChar(tt.input)
			if result != tt.expected {
				t.Errorf("SubSpecialChar(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_RmWWW(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty", "", ""},
		{"No stop words", "普通文本", "普通文本"},
		{"Chinese question word", "请问如何操作", "操作"}, // "请问" and "如何" both matched
		{"Chinese stop word 怎么办", "怎么办安装", "安装"},
		{"English what", "what is this", " this"},                     // removes "what " and "is "
		{"English who", "who are you", " you"},                        // removes "who " and "are "
		{"Mixed stop words", "请问what is the problem", " the problem"}, // Chinese removed, "what ", "is " removed
		{"All removed becomes empty", "请问", "请问"},                     // should revert to original
		{"English articles", "the cat is on a mat", " cat on mat"},    // removes "the ", "is ", "a "
		{"Case insensitive", "WHAT IS THIS", " THIS"},                 // removes "WHAT " and "IS "
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.RmWWW(tt.input)
			if result != tt.expected {
				t.Errorf("RmWWW(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_AddSpaceBetweenEngZh(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty", "", ""},
		{"English only", "hello world", "hello world"},
		{"Chinese only", "你好世界", "你好世界"},
		{"ENG+ZH", "hello世界", "hello 世界"},
		{"ZH+ENG", "世界hello", "世界 hello"},
		{"ENG+NUM+ZH", "abc123测试", "abc123 测试"},
		{"ZH+ENG+NUM", "测试abc123", "测试 abc123"},
		{"Multiple", "hello世界test测试", "hello 世界 test 测试"},
		{"Already spaced", "hello 世界", "hello 世界"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.AddSpaceBetweenEngZh(tt.input)
			if result != tt.expected {
				t.Errorf("AddSpaceBetweenEngZh(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_StrFullWidth2HalfWidth(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty", "", ""},
		{"Half-width remains", "hello world 123", "hello world 123"},
		{"Full-width uppercase", "ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ", "ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
		{"Full-width lowercase", "ａｂｃｄｅｆｇｈｉｊｋｌｍｎｏｐｑｒｓｔｕｖｗｘｙｚ", "abcdefghijklmnopqrstuvwxyz"},
		{"Full-width digits", "０１２３４５６７８９", "0123456789"},
		{"Full-width punctuation", "！＂＃＄％＆＇（）＊＋，－．／：；＜＝＞？＠［＼］＾＿｀｛｜｝～", "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"},
		{"Full-width space", "　", " "},
		{"Mixed full-width and half-width", "Ｈｅｌｌｏ　Ｗｏｒｌｄ！123", "Hello World!123"},
		{"Chinese characters unchanged", "你好世界", "你好世界"},
		{"Japanese characters unchanged", "こんにちは", "こんにちは"},
		{"Korean characters unchanged", "안녕하세요", "안녕하세요"},
		{"Full-width symbols outside range", "＠＠＠", "@@@"},            // Actually full-width '@' is U+FF20 which maps to U+0040
		{"Edge case: character just below range", "\u001F", "\u001F"}, // U+001F is < 0x0020, should remain
		{"Edge case: character just above range", "\u007F", "\u007F"}, // U+007F is > 0x7E, should remain
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.StrFullWidth2HalfWidth(tt.input)
			if result != tt.expected {
				t.Errorf("StrFullWidth2HalfWidth(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_Traditional2Simplified(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty", "", ""},
		{"Simplified unchanged", "简体中文测试", "简体中文测试"},
		{"Traditional conversion", "繁體中文測試", "繁体中文测试"},
		{"Traditional sentence", "我學習中文已經三年了", "我学习中文已经三年了"},
		{"Traditional with numbers", "電話號碼123", "电话号码123"},
		{"Traditional with English", "Hello世界", "Hello世界"},
		{"Traditional punctuation", "請問，你好嗎？", "请问，你好吗？"},
		{"Mixed traditional and simplified", "這是一個简体测试", "这是一个简体测试"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.Traditional2Simplified(tt.input)
			if result != tt.expected {
				t.Errorf("Traditional2Simplified(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_Question(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name          string
		txt           string
		tbl           string
		minMatch      float64
		expectNil     bool
		checkExpr     func(*types.MatchTextExpr) bool
		checkKeywords func([]string) bool
	}{
		{
			name:     "Chinese text",
			txt:      "请问如何安装软件",
			tbl:      "test",
			minMatch: 0.5,
			checkExpr: func(expr *types.MatchTextExpr) bool {
				// Should return a valid query expression with processed text
				return expr != nil && expr.MatchingText != ""
			},
			checkKeywords: func(keywords []string) bool {
				// Should return extracted keywords
				return len(keywords) > 0
			},
		},
		{
			name:     "English text",
			txt:      "How to install software",
			tbl:      "test",
			minMatch: 0.5,
			checkExpr: func(expr *types.MatchTextExpr) bool {
				// Should return a valid query expression with processed text
				return expr != nil && expr.MatchingText != ""
			},
			checkKeywords: func(keywords []string) bool {
				// Should return extracted keywords
				return len(keywords) > 0
			},
		},
		{
			name:     "Mixed text",
			txt:      "hello世界",
			tbl:      "test",
			minMatch: 0.5,
			checkExpr: func(expr *types.MatchTextExpr) bool {
				// Should return a valid query expression with processed text
				return expr != nil && expr.MatchingText != ""
			},
			checkKeywords: func(keywords []string) bool {
				// Should return extracted keywords
				return len(keywords) > 0
			},
		},
		{
			name:      "Empty text",
			txt:       "",
			tbl:       "test",
			minMatch:  0.5,
			expectNil: true,
			checkExpr: func(expr *types.MatchTextExpr) bool {
				return expr == nil
			},
			checkKeywords: func(keywords []string) bool {
				return len(keywords) == 0
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, keywords := qb.Question(tt.txt, tt.tbl, tt.minMatch)
			if tt.expectNil && expr != nil {
				t.Errorf("Question(%q) expected nil expr, got %v", tt.txt, expr)
			}
			if !tt.expectNil && expr == nil {
				t.Errorf("Question(%q) returned nil expr", tt.txt)
			}
			if expr != nil && !tt.checkExpr(expr) {
				t.Errorf("Question(%q) expr check failed, got %+v", tt.txt, expr)
			}
			if tt.checkKeywords != nil && !tt.checkKeywords(keywords) {
				t.Errorf("Question(%q) keywords check failed, got %v", tt.txt, keywords)
			}
		})
	}
}

func TestQueryBuilder_Paragraph(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name          string
		contentTks    string
		keywords      []string
		keywordsTopN  int
		expectedQuery string
	}{
		{
			name:          "No keywords",
			contentTks:    "some content terms",
			keywords:      []string{},
			keywordsTopN:  0,
			expectedQuery: "",
		},
		{
			name:          "Single keyword",
			contentTks:    "content",
			keywords:      []string{"hello"},
			keywordsTopN:  0,
			expectedQuery: `"hello"`,
		},
		{
			name:          "Multiple keywords",
			contentTks:    "content",
			keywords:      []string{"hello", "world", "test"},
			keywordsTopN:  0,
			expectedQuery: `"hello" "world" "test"`,
		},
		{
			name:          "Trim spaces",
			contentTks:    "",
			keywords:      []string{"  hello ", " world "},
			keywordsTopN:  0,
			expectedQuery: `"hello" "world"`,
		},
		{
			name:          "TopN limit",
			contentTks:    "",
			keywords:      []string{"a", "b", "c", "d", "e"},
			keywordsTopN:  3,
			expectedQuery: `"a" "b" "c"`,
		},
		{
			name:          "TopN larger than slice",
			contentTks:    "",
			keywords:      []string{"a", "b"},
			keywordsTopN:  10,
			expectedQuery: `"a" "b"`,
		},
		{
			name:          "Empty keyword filtered",
			contentTks:    "",
			keywords:      []string{"a", "", "b"},
			keywordsTopN:  0,
			expectedQuery: `"a" "b"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := qb.Paragraph(tt.contentTks, tt.keywords, tt.keywordsTopN)
			if expr == nil {
				t.Fatal("Paragraph returned nil expr")
			}
			if expr.MatchingText != tt.expectedQuery {
				t.Errorf("Paragraph query mismatch, got %q, want %q", expr.MatchingText, tt.expectedQuery)
			}
			// Check default fields
			defaultFields := []string{
				"title_tks^10",
				"title_sm_tks^5",
				"important_kwd^30",
				"important_tks^20",
				"question_tks^20",
				"content_ltks^2",
				"content_sm_ltks",
			}
			if !reflect.DeepEqual(expr.Fields, defaultFields) {
				t.Errorf("Paragraph fields mismatch, got %v, want %v", expr.Fields, defaultFields)
			}
			if expr.TopN != 100 {
				t.Errorf("Paragraph TopN mismatch, got %d, want 100", expr.TopN)
			}
		})
	}
}

func TestQueryBuilder_Similarity(t *testing.T) {
	qb := NewQueryBuilder()
	tests := []struct {
		name     string
		qtwt     map[string]float64
		dtwt     map[string]float64
		expected float64
	}{
		{"Empty query", map[string]float64{}, map[string]float64{"a": 1.0}, 0.0},
		{"Empty doc", map[string]float64{"a": 1.0}, map[string]float64{}, 0.0},
		{"Exact match", map[string]float64{"a": 1.0, "b": 2.0}, map[string]float64{"a": 5.0, "b": 3.0}, 1.0},
		{"Partial match", map[string]float64{"a": 1.0, "b": 2.0, "c": 3.0}, map[string]float64{"a": 1.0, "c": 1.0}, (1.0 + 3.0) / (1.0 + 2.0 + 3.0)}, // sum=4, total=6 => 0.666...
		{"No match", map[string]float64{"a": 1.0}, map[string]float64{"b": 2.0}, 0.0},
		{"Zero total weight", map[string]float64{"a": 0.0, "b": 0.0}, map[string]float64{"a": 1.0}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := qb.Similarity(tt.qtwt, tt.dtwt)
			// Use tolerance for floating point
			if result < tt.expected-1e-9 || result > tt.expected+1e-9 {
				t.Errorf("Similarity(%v, %v) = %v, want %v", tt.qtwt, tt.dtwt, result, tt.expected)
			}
		})
	}
}

func TestQueryBuilder_TokenSimilarity(t *testing.T) {
	qb := NewQueryBuilder()
	// Currently placeholder returns zero slice
	atks := "query terms"
	btkss := []string{"doc1", "doc2", "doc3"}
	result := qb.TokenSimilarity(atks, btkss)
	if len(result) != len(btkss) {
		t.Errorf("TokenSimilarity length mismatch, got %d, want %d", len(result), len(btkss))
	}
	for i, v := range result {
		if v != 0.0 {
			t.Errorf("TokenSimilarity[%d] = %v, want 0.0", i, v)
		}
	}
}

func TestQueryBuilder_HybridSimilarity(t *testing.T) {
	qb := NewQueryBuilder()
	avec := []float64{1.0, 2.0}
	bvecs := [][]float64{{1.0, 2.0}, {3.0, 4.0}}
	atks := "query"
	btkss := []string{"doc1", "doc2"}
	tkweight := 0.5
	vtweight := 0.5
	sims, tksim, vecsim := qb.HybridSimilarity(avec, bvecs, atks, btkss, tkweight, vtweight)
	if len(sims) != 2 || len(tksim) != 2 || len(vecsim) != 2 {
		t.Errorf("HybridSimilarity returned slices of wrong length: sims=%d, tksim=%d, vecsim=%d", len(sims), len(tksim), len(vecsim))
	}
	for i := range sims {
		if sims[i] != 0.0 || tksim[i] != 0.0 || vecsim[i] != 0.0 {
			t.Errorf("HybridSimilarity[%d] non-zero: sims=%v, tksim=%v, vecsim=%v", i, sims[i], tksim[i], vecsim[i])
		}
	}
}

func TestQueryBuilder_SetQueryFields(t *testing.T) {
	qb := NewQueryBuilder()
	newFields := []string{"field1", "field2^5"}
	qb.SetQueryFields(newFields)
	if !reflect.DeepEqual(qb.queryFields, newFields) {
		t.Errorf("SetQueryFields failed, got %v, want %v", qb.queryFields, newFields)
	}
	// Ensure other methods use updated fields
	expr := qb.Paragraph("", []string{"test"}, 0)
	if !reflect.DeepEqual(expr.Fields, newFields) {
		t.Errorf("Paragraph fields not updated after SetQueryFields, got %v, want %v", expr.Fields, newFields)
	}
}
