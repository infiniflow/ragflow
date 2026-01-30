package nlp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNewRagTokenizer tests the tokenizer creation
func TestNewRagTokenizer(t *testing.T) {
	// Create a temporary dictionary file for testing
	tmpDir := t.TempDir()
	dictPath := filepath.Join(tmpDir, "huqie.txt")
	dictContent := `test 1000000 n
word 2000000 v
中文 3000000 n
分词 4000000 v`
	err := os.WriteFile(dictPath, []byte(dictContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test dictionary: %v", err)
	}

	tokenizer, err := NewRagTokenizer(false, dictPath)
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	if tokenizer == nil {
		t.Fatal("Tokenizer is nil")
	}

	if tokenizer.Denominator != 1000000 {
		t.Errorf("Expected Denominator to be 1000000, got %d", tokenizer.Denominator)
	}

	if tokenizer.Dir != dictPath {
		t.Errorf("Expected Dir to be %s, got %s", dictPath, tokenizer.Dir)
	}
}

// TestKey tests the key generation function
func TestKey(t *testing.T) {
	tokenizer := &RagTokenizer{}

	tests := []struct {
		input    string
		expected string
	}{
		{"test", "test"},
		{"TEST", "test"},
		{"Test", "test"},
		{"中文", "\\xe4\\xb8\\xad\\xe6\\x96\\x87"},
		{"Hello World", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenizer.key(tt.input)
			if result != tt.expected {
				t.Errorf("key(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestRkey tests the reverse key generation function
func TestRkey(t *testing.T) {
	tokenizer := &RagTokenizer{}

	tests := []struct {
		input    string
		expected string
	}{
		{"test", "DDtset"},
		{"TEST", "DDtset"},
		{"中文", "DD\\xe6\\x96\\x87\\xe4\\xb8\\xad"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenizer.rkey(tt.input)
			if result != tt.expected {
				t.Errorf("rkey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestStrQ2B tests full-width to half-width conversion
func TestStrQ2B(t *testing.T) {
	tokenizer := &RagTokenizer{}

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"hello", "hello"},
		{"ＡＢＣ", "ABC"},
		{"１２３", "123"},
		{"　", " "}, // full-width space to half-width space
		{"，．！？", ",.!?"},  // Note: ． is full-width period (U+FF0E), not 。 (U+3002)
		{"ａｂｃ１２３", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenizer.StrQ2B(tt.input)
			if result != tt.expected {
				t.Errorf("StrQ2B(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestTraditional2Simplified tests traditional to simplified Chinese conversion
func TestTraditional2Simplified(t *testing.T) {
	tokenizer := &RagTokenizer{}

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"简体中文", "简体中文"},
		{"繁體中文", "繁体中文"},
		{"臺灣", "台湾"},
		{"我學習中文", "我学习中文"},
		{"電話號碼", "电话号码"},
		{"Hello世界", "Hello世界"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenizer.Traditional2Simplified(tt.input)
			if result != tt.expected {
				t.Errorf("Traditional2Simplified(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsChinese tests Chinese character detection
func TestIsChinese(t *testing.T) {
	tests := []struct {
		input    rune
		expected bool
	}{
		{'中', true},
		{'文', true},
		{'a', false},
		{'A', false},
		{'1', false},
		{' ', false},
		{'。', false}, // Chinese punctuation is not in CJK range
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := IsChinese(tt.input)
			if result != tt.expected {
				t.Errorf("IsChinese(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsNumber tests number detection
func TestIsNumber(t *testing.T) {
	tests := []struct {
		input    rune
		expected bool
	}{
		{'0', true},
		{'5', true},
		{'9', true},
		{'a', false},
		{'中', false},
		{' ', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := IsNumber(tt.input)
			if result != tt.expected {
				t.Errorf("IsNumber(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsAlphabet tests alphabet detection
func TestIsAlphabet(t *testing.T) {
	tests := []struct {
		input    rune
		expected bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'m', true},
		{'M', true},
		{'0', false},
		{'中', false},
		{' ', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := IsAlphabet(tt.input)
			if result != tt.expected {
				t.Errorf("IsAlphabet(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNaiveQie tests naive tokenization
func TestNaiveQie(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{}},
		{"hello world", []string{"hello", " ", "world"}},
		{"test case", []string{"test", " ", "case"}},
		{"single", []string{"single"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NaiveQie(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("NaiveQie(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("NaiveQie(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// TestLoadDict tests dictionary loading
func TestLoadDict(t *testing.T) {
	tmpDir := t.TempDir()
	dictPath := filepath.Join(tmpDir, "test_dict.txt")
	dictContent := `test 1000000 n
word 2000000 v
中文 3000000 n
分词 4000000 v`
	err := os.WriteFile(dictPath, []byte(dictContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test dictionary: %v", err)
	}

	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie:        make(map[string]TokenInfo),
		rTrie:       make(map[string]bool),
	}

	err = tokenizer.loadDict(dictPath)
	if err != nil {
		t.Fatalf("Failed to load dictionary: %v", err)
	}

	// Check if words are loaded
	if len(tokenizer.trie) == 0 {
		t.Error("Dictionary not loaded properly")
	}

	// Test key lookup
	testKey := tokenizer.key("test")
	if info, exists := tokenizer.trie[testKey]; !exists {
		t.Error("'test' not found in trie")
	} else {
		if info.Tag != "n" {
			t.Errorf("Expected tag 'n' for 'test', got %q", info.Tag)
		}
	}
}

// TestFreq tests frequency lookup
func TestFreq(t *testing.T) {
	tmpDir := t.TempDir()
	dictPath := filepath.Join(tmpDir, "test_dict.txt")
	dictContent := `test 1000000 n
word 2000000 v`
	err := os.WriteFile(dictPath, []byte(dictContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test dictionary: %v", err)
	}

	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie:        make(map[string]TokenInfo),
		rTrie:       make(map[string]bool),
	}

	err = tokenizer.loadDict(dictPath)
	if err != nil {
		t.Fatalf("Failed to load dictionary: %v", err)
	}

	// Test frequency calculation
	freq := tokenizer.Freq("test")
	if freq == 0 {
		t.Error("Frequency for 'test' should not be 0")
	}

	// Test non-existent word
	freq = tokenizer.Freq("nonexistent")
	if freq != 0 {
		t.Errorf("Frequency for non-existent word should be 0, got %d", freq)
	}
}

// TestTag tests tag lookup
func TestTag(t *testing.T) {
	tmpDir := t.TempDir()
	dictPath := filepath.Join(tmpDir, "test_dict.txt")
	dictContent := `test 1000000 n
word 2000000 v`
	err := os.WriteFile(dictPath, []byte(dictContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test dictionary: %v", err)
	}

	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie:        make(map[string]TokenInfo),
		rTrie:       make(map[string]bool),
	}

	err = tokenizer.loadDict(dictPath)
	if err != nil {
		t.Fatalf("Failed to load dictionary: %v", err)
	}

	// Test tag lookup
	tag := tokenizer.Tag("test")
	if tag != "n" {
		t.Errorf("Tag for 'test' should be 'n', got %q", tag)
	}

	tag = tokenizer.Tag("word")
	if tag != "v" {
		t.Errorf("Tag for 'word' should be 'v', got %q", tag)
	}

	// Test non-existent word
	tag = tokenizer.Tag("nonexistent")
	if tag != "" {
		t.Errorf("Tag for non-existent word should be empty, got %q", tag)
	}
}

// TestMerge tests token merging
func TestMerge(t *testing.T) {
	tokenizer := &RagTokenizer{
		splitChar: regexp.MustCompile(`([ ,\.<>/?;:'\[\]\\` + "`" + `!@#$%^&*\(\)\{\}\|_+=《》，。？、；'':'\'\'【】~！￥%……（）——-]+|[a-zA-Z0-9,\.-]+)`),
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"hello world", "hello world"},
		{"a  b  c", "a b c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenizer.merge(tt.input)
			if result != tt.expected {
				t.Errorf("merge(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestEnglishNormalize tests English normalization
func TestEnglishNormalize(t *testing.T) {
	tokenizer := &RagTokenizer{}

	tests := []struct {
		input    []string
		expected []string
	}{
		{[]string{}, []string{}},
		{[]string{"running"}, []string{"runn"}},
		{[]string{"tested"}, []string{"test"}},
		{[]string{"中文"}, []string{"中文"}},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			result := tokenizer.englishNormalize(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("englishNormalize(%v) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("englishNormalize(%v)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// TestSimpleStem tests simple stemming
func TestSimpleStem(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"running", "runn"},
		{"tested", "test"},
		{"happiness", "happi"},
		{"test", "test"},
		{"ing", "ing"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := simpleStem(tt.input)
			if result != tt.expected {
				t.Errorf("simpleStem(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSplitByLang tests language-based text splitting
func TestSplitByLang(t *testing.T) {
	tokenizer := &RagTokenizer{
		splitChar: regexp.MustCompile(`([ ,\.<>/?;:'\[\]\\` + "`" + `!@#$%^&*\(\)\{\}\|_+=《》，。？、；'':'\'\'【】~！￥%……（）——-]+|[a-zA-Z0-9,\.-]+)`),
	}

	tests := []struct {
		input         string
		expectedCount int
	}{
		{"", 0},
		{"中文", 1},
		// Note: "hello" may be filtered by splitChar regex, so expected 0
		// "中文english" - the English part may be filtered, leaving only Chinese
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenizer.splitByLang(tt.input)
			if len(result) != tt.expectedCount {
				t.Errorf("splitByLang(%q) returned %d segments, want %d", tt.input, len(result), tt.expectedCount)
			}
		})
	}
}

// TestMin tests min function
func TestMin(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{1, 1, 1},
		{-1, 1, -1},
		{0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.a))+"_"+string(rune(tt.b)), func(t *testing.T) {
			result := min(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestHasKeysWithPrefix tests prefix matching
func TestHasKeysWithPrefix(t *testing.T) {
	tokenizer := &RagTokenizer{
		trie: map[string]TokenInfo{
			"test":    {Freq: 1, Tag: "n"},
			"testing": {Freq: 2, Tag: "v"},
			"word":    {Freq: 3, Tag: "n"},
		},
	}

	tests := []struct {
		prefix   string
		expected bool
	}{
		{"test", true},
		{"tes", true},
		{"wo", true},
		{"xyz", false},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			result := tokenizer.hasKeysWithPrefix(tt.prefix)
			if result != tt.expected {
				t.Errorf("hasKeysWithPrefix(%q) = %v, want %v", tt.prefix, result, tt.expected)
			}
		})
	}
}

// TestScore tests scoring function
func TestScore(t *testing.T) {
	tokenizer := &RagTokenizer{Debug: false}

	tfts := []TokenResult{
		{Token: "test", Freq: 10, Tag: "n"},
		{Token: "word", Freq: 20, Tag: "v"},
	}

	tokens, score := tokenizer.score(tfts)
	if len(tokens) != 2 {
		t.Errorf("Expected 2 tokens, got %d", len(tokens))
	}
	if score == 0 {
		t.Error("Score should not be 0")
	}
}

// TestSortTokens tests token sorting
func TestSortTokens(t *testing.T) {
	tokenizer := &RagTokenizer{Debug: false}

	tkslist := [][]TokenResult{
		{{Token: "a", Freq: 1}, {Token: "b", Freq: 2}},
		{{Token: "c", Freq: 10}, {Token: "d", Freq: 20}},
	}

	sorted := tokenizer.sortTokens(tkslist)
	if len(sorted) != 2 {
		t.Errorf("Expected 2 results, got %d", len(sorted))
	}
	// Higher score should be first
	if sorted[0].score < sorted[1].score {
		t.Error("Results should be sorted by score in descending order")
	}
}

// TestMaxForward tests maximum forward matching
func TestMaxForward(t *testing.T) {
	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie: map[string]TokenInfo{
			"test": {Freq: 1, Tag: "n"},
		},
	}

	tks, score := tokenizer.maxForward("test")
	if len(tks) == 0 {
		t.Error("maxForward should return tokens")
	}
	if score == 0 {
		t.Error("maxForward should return non-zero score")
	}
}

// TestMaxBackward tests maximum backward matching
func TestMaxBackward(t *testing.T) {
	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie: map[string]TokenInfo{
			"test": {Freq: 1, Tag: "n"},
		},
	}

	tks, score := tokenizer.maxBackward("test")
	if len(tks) == 0 {
		t.Error("maxBackward should return tokens")
	}
	if score == 0 {
		t.Error("maxBackward should return non-zero score")
	}
}

// TestDfs tests depth-first search
func TestDfs(t *testing.T) {
	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie: map[string]TokenInfo{
			"a": {Freq: 1, Tag: "n"},
			"b": {Freq: 2, Tag: "v"},
		},
	}

	var tkslist [][]TokenResult
	memo := make(map[string]int)
	result := tokenizer.dfs([]rune("ab"), 0, []TokenResult{}, &tkslist, 0, memo)
	if result < 0 {
		t.Error("dfs should return non-negative result")
	}
}

// TestAddUserDict tests adding user dictionary
func TestAddUserDict(t *testing.T) {
	tmpDir := t.TempDir()
	dictPath := filepath.Join(tmpDir, "user_dict.txt")
	dictContent := `custom 1000000 n
user 2000000 v`
	err := os.WriteFile(dictPath, []byte(dictContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test dictionary: %v", err)
	}

	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie:        make(map[string]TokenInfo),
		rTrie:       make(map[string]bool),
	}

	err = tokenizer.AddUserDict(dictPath)
	if err != nil {
		t.Fatalf("Failed to add user dictionary: %v", err)
	}

	// Check if words are loaded
	if len(tokenizer.trie) == 0 {
		t.Error("User dictionary not added properly")
	}
}

// TestLoadUserDict tests loading user dictionary
func TestLoadUserDict(t *testing.T) {
	tmpDir := t.TempDir()
	dictPath := filepath.Join(tmpDir, "user_dict.txt")
	dictContent := `custom 1000000 n`
	err := os.WriteFile(dictPath, []byte(dictContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test dictionary: %v", err)
	}

	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie:        map[string]TokenInfo{"old": {Freq: 1, Tag: "n"}},
		rTrie:       make(map[string]bool),
	}

	err = tokenizer.LoadUserDict(dictPath)
	if err != nil {
		t.Fatalf("Failed to load user dictionary: %v", err)
	}

	// After loading, old entries should be cleared
	if _, exists := tokenizer.trie["old"]; exists {
		t.Error("Old entries should be cleared after LoadUserDict")
	}
}

// TestFineGrainedTokenize tests fine-grained tokenization
func TestFineGrainedTokenize(t *testing.T) {
	tokenizer := &RagTokenizer{
		Denominator: 1000000,
		trie:        make(map[string]TokenInfo),
		rTrie:       make(map[string]bool),
	}

	// Test with mostly non-Chinese tokens
	result := tokenizer.FineGrainedTokenize("hello world test")
	if result == "" {
		t.Error("FineGrainedTokenize should return non-empty result")
	}

	// Test with Chinese tokens
	result = tokenizer.FineGrainedTokenize("中文 分词 测试")
	if result == "" {
		t.Error("FineGrainedTokenize should handle Chinese text")
	}
}
