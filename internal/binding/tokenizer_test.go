package rag_analyzer

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// allowedMismatchTokens 包含已知的不匹配token，这些在可接受范围内
var allowedMismatchTokens = map[string]bool{
	"be":     true,
	"datum":  true,
	"ccs":    true,
	"experi": true,
	"fast":   true,
	"llms":   true,
	"larg":   true,
	"ass":    true,
}

// getTestFilePath 获取测试文件的路径，优先使用当前目录
func getTestFilePath(filename string) string {
	// 首先尝试当前目录（binding 目录）
	if _, err := os.Stat(filename); err == nil {
		return filename
	}
	// 如果不在当前目录，尝试从测试目录查找
	return "../../test/" + filename
}

// TestAnalyzeEnablePosition 测试 Analyze 启用位置功能
func TestAnalyzeEnablePosition(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	inputPath := getTestFilePath("tokenizer_input.txt")

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	analyzer.SetEnablePosition(true)
	analyzer.SetFineGrained(false)

	file, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("failed to open input file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineCount++

		tokens, err := analyzer.Analyze(line)
		if err != nil {
			t.Errorf("Error analyzing line %d: %v", lineCount, err)
			continue
		}

		if len(tokens) == 0 {
			t.Errorf("Line %d: expected tokens, got none", lineCount)
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading file: %v", err)
	}

	t.Logf("TestAnalyzeEnablePosition completed, processed %d lines", lineCount)
}

// TestAnalyzeEnablePositionFineGrained 测试细粒度分词
func TestAnalyzeEnablePositionFineGrained(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	inputPath := getTestFilePath("tokenizer_input.txt")

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	analyzer.SetEnablePosition(true)
	analyzer.SetFineGrained(true)

	file, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("failed to open input file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineCount++

		tokens, err := analyzer.Analyze(line)
		if err != nil {
			t.Errorf("Error analyzing line %d: %v", lineCount, err)
			continue
		}

		if len(tokens) == 0 {
			t.Errorf("Line %d: expected tokens, got none", lineCount)
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading file: %v", err)
	}

	t.Logf("TestAnalyzeEnablePositionFineGrained completed, processed %d lines", lineCount)
}

// TestTokenizeConsistencyWithPosition 测试 Tokenize 和 TokenizeWithPosition 的一致性
func TestTokenizeConsistencyWithPosition(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	inputPath := getTestFilePath("tokenizer_input.txt")

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	file, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("failed to open input file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	mismatchCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineCount++

		// Test Tokenize (returns string)
		tokensStr, err := analyzer.Tokenize(line)
		if err != nil {
			t.Errorf("Error tokenizing line %d: %v", lineCount, err)
			continue
		}

		tokenizeResult := strings.Fields(tokensStr)

		// Test TokenizeWithPosition
		tokenizeWithPosResult, err := analyzer.TokenizeWithPosition(line)
		if err != nil {
			t.Errorf("Error tokenizing with position line %d: %v", lineCount, err)
			continue
		}

		// Check if results are identical
		tokensMatch := len(tokenizeResult) == len(tokenizeWithPosResult)
		if tokensMatch {
			for i := 0; i < len(tokenizeResult); i++ {
				if tokenizeResult[i] != tokenizeWithPosResult[i].Text {
					tokensMatch = false
					break
				}
			}
		}

		if !tokensMatch {
			mismatchCount++
			t.Errorf("Line %d: Tokenize and TokenizeWithPosition results mismatch. Tokenize count: %d, TokenizeWithPosition count: %d",
				lineCount, len(tokenizeResult), len(tokenizeWithPosResult))
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading file: %v", err)
	}

	t.Logf("TestTokenizeConsistencyWithPosition completed, processed %d lines, %d mismatches", lineCount, mismatchCount)
}

// TestTokenizeConsistencyWithPython 测试与 Python 版本的分词结果一致性
func TestTokenizeConsistencyWithPython(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	inputPath := getTestFilePath("tokenizer_input.txt")
	pythonOutputPath := getTestFilePath("tokenizer_python_output.txt")

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	file, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("failed to open input file: %v", err)
	}
	defer file.Close()

	pythonFile, err := os.Open(pythonOutputPath)
	if err != nil {
		t.Fatalf("failed to open python output file: %v", err)
	}
	defer pythonFile.Close()

	scanner := bufio.NewScanner(file)
	pythonScanner := bufio.NewScanner(pythonFile)

	lineNum := 0
	mismatchCount := 0

	for scanner.Scan() && pythonScanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineNum++

		pythonTokens := pythonScanner.Text()

		tokens, err := analyzer.Tokenize(line)
		if err != nil {
			t.Errorf("Error tokenizing line %d: %v", lineNum, err)
			continue
		}

		tokenizeResult := strings.Fields(tokens)
		pythonTokenizeResult := strings.Fields(pythonTokens)

		isSizeMatch := len(tokenizeResult) == len(pythonTokenizeResult)
		if !isSizeMatch {
			mismatchCount++
			t.Errorf("Line %d: Size mismatch: Tokenize count: %d, Python tokenize count: %d",
				lineNum, len(tokenizeResult), len(pythonTokenizeResult))
			continue
		}

		isMatch := true
		isBadToken := false
		for i := 0; i < len(tokenizeResult); i++ {
			if tokenizeResult[i] != pythonTokenizeResult[i] {
				isBadToken = allowedMismatchTokens[tokenizeResult[i]]
				if !isBadToken {
					isMatch = false
					t.Errorf("Line %d: Token mismatch at position %d: got '%s', expected '%s'",
						lineNum, i, tokenizeResult[i], pythonTokenizeResult[i])
					break
				}
			}
		}

		if !isMatch {
			mismatchCount++
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading input file: %v", err)
	}
	if err := pythonScanner.Err(); err != nil {
		t.Fatalf("error reading python output file: %v", err)
	}

	t.Logf("TestTokenizeConsistencyWithPython completed, processed %d lines, %d mismatches", lineNum, mismatchCount)
}

// TestFineGrainedTokenizeConsistencyWithPython 测试细粒度分词与 Python 版本的一致性
func TestFineGrainedTokenizeConsistencyWithPython(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	inputPath := getTestFilePath("tokenizer_input.txt")
	pythonOutputPath := getTestFilePath("fine_grained_tokenizer_python_output.txt")

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	analyzer.SetEnablePosition(false)
	analyzer.SetFineGrained(true)

	file, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("failed to open input file: %v", err)
	}
	defer file.Close()

	pythonFile, err := os.Open(pythonOutputPath)
	if err != nil {
		t.Fatalf("failed to open python output file: %v", err)
	}
	defer pythonFile.Close()

	scanner := bufio.NewScanner(file)
	pythonScanner := bufio.NewScanner(pythonFile)

	lineNum := 0
	mismatchCount := 0

	for scanner.Scan() && pythonScanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineNum++

		pythonTokens := pythonScanner.Text()

		tokens, err := analyzer.Analyze(line)
		if err != nil {
			t.Errorf("Error analyzing line %d: %v", lineNum, err)
			continue
		}

		pythonTokenizeResult := strings.Fields(pythonTokens)

		isSizeMatch := len(tokens) == len(pythonTokenizeResult)
		if !isSizeMatch {
			mismatchCount++
			t.Errorf("Line %d: Size mismatch: Tokenize count: %d, Python tokenize count: %d",
				lineNum, len(tokens), len(pythonTokenizeResult))
			continue
		}

		isMatch := true
		isBadToken := false
		for i := 0; i < len(tokens); i++ {
			if tokens[i].Text != pythonTokenizeResult[i] {
				isBadToken = allowedMismatchTokens[tokens[i].Text]
				if !isBadToken {
					isMatch = false
					t.Errorf("Line %d: Token mismatch at position %d: got '%s', expected '%s'",
						lineNum, i, tokens[i].Text, pythonTokenizeResult[i])
					break
				}
			}
		}

		if !isMatch {
			mismatchCount++
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading input file: %v", err)
	}
	if err := pythonScanner.Err(); err != nil {
		t.Fatalf("error reading python output file: %v", err)
	}

	t.Logf("TestFineGrainedTokenizeConsistencyWithPython completed, processed %d lines, %d mismatches", lineNum, mismatchCount)
}

// TestTokenizeSingleText 测试单行文本分词
func TestTokenizeSingleText(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	line := "在本研究中，我们提出了一种novel的neural network架构，用于解决multi-modal learning问题。"

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	tokens, err := analyzer.Tokenize(line)
	if err != nil {
		t.Fatalf("Error tokenizing: %v", err)
	}

	if tokens == "" {
		t.Error("Expected non-empty tokens, got empty string")
	}

	t.Logf("Input: %s", line)
	t.Logf("Tokenize result: %s", tokens)
}

// TestAnalyzerCreation 测试分析器创建和关闭
func TestAnalyzerCreation(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"

	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	// 测试设置选项
	analyzer.SetFineGrained(true)
	analyzer.SetEnablePosition(true)

	// 关闭分析器
	analyzer.Close()

	// 多次关闭应该不会 panic
	analyzer.Close()
}

// TestTokenizeWithPositionDetailed 测试带位置信息的分词详细功能
func TestTokenizeWithPositionDetailed(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	text := "Hello world 你好世界"
	tokens, err := analyzer.TokenizeWithPosition(text)
	if err != nil {
		t.Fatalf("Error tokenizing with position: %v", err)
	}

	if len(tokens) == 0 {
		t.Error("Expected tokens, got none")
	}

	// 验证每个token都有位置信息
	for i, token := range tokens {
		if token.Text == "" {
			t.Errorf("Token %d: expected non-empty text", i)
		}
		// Offset 应该小于 EndOffset
		if token.Offset >= token.EndOffset {
			t.Errorf("Token %d (%s): Offset (%d) should be less than EndOffset (%d)",
				i, token.Text, token.Offset, token.EndOffset)
		}
	}

	t.Logf("Processed %d tokens", len(tokens))
}

// TestEmptyInput 测试空输入处理
func TestEmptyInput(t *testing.T) {
	dictPath := "/usr/share/infinity/resource"
	analyzer, err := NewAnalyzer(dictPath)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	if err := analyzer.Load(); err != nil {
		t.Fatalf("failed to load analyzer: %v", err)
	}

	// 测试空字符串
	tokens, err := analyzer.Tokenize("")
	if err != nil {
		t.Errorf("Error tokenizing empty string: %v", err)
	}
	// 空字符串可能返回空字符串或空结果
	t.Logf("Empty string result: '%s'", tokens)

	// 测试只有空白的字符串
	tokens, err = analyzer.Tokenize("   ")
	if err != nil {
		t.Errorf("Error tokenizing whitespace string: %v", err)
	}
	t.Logf("Whitespace string result: '%s'", tokens)
}
