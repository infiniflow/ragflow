package tokenizer

import (
	"fmt"
	"sync"

	"go.uber.org/zap"

	rag "ragflow/internal/go_binding"
	"ragflow/internal/logger"
)

var (
	// globalAnalyzer is the global analyzer instance
	globalAnalyzer *rag.Analyzer
	// once ensures the analyzer is initialized only once
	once sync.Once
	// initError stores any error during initialization
	initError error
)

// Config is the tokenizer configuration
type Config struct {
	DictPath string `mapstructure:"dict_path"`
}

// Init initializes the global tokenizer
// It should be called during the initialization phase of main.go
func Init(cfg *Config) error {
	once.Do(func() {
		dictPath := cfg.DictPath
		if dictPath == "" {
			dictPath = "/usr/share/infinity/resource"
		}

		logger.Info("Initializing rag_analyzer", zap.String("dict_path", dictPath))

		globalAnalyzer, initError = rag.NewAnalyzer(dictPath)
		if initError != nil {
			initError = fmt.Errorf("failed to create analyzer: %w", initError)
			logger.Error("Failed to create analyzer", initError)
			return
		}

		if initError = globalAnalyzer.Load(); initError != nil {
			initError = fmt.Errorf("failed to load analyzer: %w", initError)
			logger.Error("Failed to load analyzer", initError)
			globalAnalyzer.Close()
			globalAnalyzer = nil
			return
		}

		logger.Info("RAG analyzer initialized successfully")
	})

	return initError
}

// Close closes the global tokenizer and releases resources
// It should be called when the program exits
func Close() {
	if globalAnalyzer != nil {
		globalAnalyzer.Close()
		globalAnalyzer = nil
		logger.Info("RAG analyzer closed")
	}
}

// Tokenize tokenizes the text and returns a space-separated string of tokens
// Example: "hello world" -> "hello world"
func Tokenize(text string) (string, error) {
	if globalAnalyzer == nil {
		return "", fmt.Errorf("tokenizer not initialized")
	}
	return globalAnalyzer.Tokenize(text)
}

// TokenizeWithPosition tokenizes the text and returns a list of tokens with position information
func TokenizeWithPosition(text string) ([]rag.TokenWithPosition, error) {
	if globalAnalyzer == nil {
		return nil, fmt.Errorf("tokenizer not initialized")
	}
	return globalAnalyzer.TokenizeWithPosition(text)
}

// Analyze analyzes the text and returns all tokens
func Analyze(text string) ([]rag.Token, error) {
	if globalAnalyzer == nil {
		return nil, fmt.Errorf("tokenizer not initialized")
	}
	return globalAnalyzer.Analyze(text)
}

// SetFineGrained sets whether to use fine-grained tokenization
func SetFineGrained(fineGrained bool) {
	if globalAnalyzer != nil {
		globalAnalyzer.SetFineGrained(fineGrained)
	}
}

// FineGrainedTokenize performs fine-grained tokenization on space-separated tokens
// Input: space-separated tokens (e.g., "hello world 测试")
// Output: space-separated fine-grained tokens (e.g., "hello world 测 试")
func FineGrainedTokenize(tokens string) (string, error) {
	if globalAnalyzer == nil {
		return "", fmt.Errorf("tokenizer not initialized")
	}
	return globalAnalyzer.FineGrainedTokenize(tokens)
}

// SetEnablePosition sets whether to enable position tracking
func SetEnablePosition(enablePosition bool) {
	if globalAnalyzer != nil {
		globalAnalyzer.SetEnablePosition(enablePosition)
	}
}

// IsInitialized checks whether the tokenizer has been initialized
func IsInitialized() bool {
	return globalAnalyzer != nil
}
