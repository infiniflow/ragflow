package models

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

type captureRerankDriver struct {
	ModelDriver
	request RerankRequest
	calls   int
}

func (d *captureRerankDriver) Rerank(_ context.Context, _ *string, request RerankRequest, _ *APIConfig, _ *RerankConfig, _ *common.ModelUsage) (*RerankResponse, error) {
	d.request = request
	d.calls++
	return &RerankResponse{}, nil
}

func TestRerankModelTokenLimitModes(t *testing.T) {
	query := "question"
	document := strings.Repeat("token ", 20)
	queryTokens := tokenizer.NumTokensFromString(query)

	t.Run("truncate", func(t *testing.T) {
		t.Setenv(common.EnvRerankTokenLimitMode, "")
		driver := &captureRerankDriver{}
		model := NewRerankModel(driver, nil, nil, queryTokens+4)
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{document}}, nil, nil, nil)
		if err != nil {
			t.Fatalf("Rerank() error = %v", err)
		}
		if got := queryTokens + tokenizer.NumTokensFromString(driver.request.Documents[0]); got > model.MaxTokens {
			t.Fatalf("rerank input tokens = %d, want at most %d", got, model.MaxTokens)
		}
		if driver.request.Documents[0] == document {
			t.Fatal("document was not truncated")
		}
	})

	t.Run("passthrough", func(t *testing.T) {
		t.Setenv(common.EnvRerankTokenLimitMode, " PASSTHROUGH ")
		driver := &captureRerankDriver{}
		model := NewRerankModel(driver, nil, nil, 1)
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{document}}, nil, nil, nil)
		if err != nil {
			t.Fatalf("Rerank() error = %v", err)
		}
		if driver.request.Documents[0] != document {
			t.Fatalf("document = %q, want unchanged input", driver.request.Documents[0])
		}
	})

	t.Run("raise_error", func(t *testing.T) {
		t.Setenv(common.EnvRerankTokenLimitMode, "raise_error")
		driver := &captureRerankDriver{}
		model := NewRerankModel(driver, nil, nil, queryTokens)
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{"short"}}, nil, nil, nil)
		if !errors.Is(err, ErrRerankTokenLimitPolicy) || !strings.Contains(err.Error(), "document index 0") {
			t.Fatalf("Rerank() error = %v, want token-limit error for document 0", err)
		}
		if driver.calls != 0 {
			t.Fatalf("driver calls = %d, want 0", driver.calls)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Setenv(common.EnvRerankTokenLimitMode, "invalid")
		driver := &captureRerankDriver{}
		model := NewRerankModel(driver, nil, nil, 10)
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{"short"}}, nil, nil, nil)
		if !errors.Is(err, ErrRerankTokenLimitPolicy) || !strings.Contains(err.Error(), common.EnvRerankTokenLimitMode) {
			t.Fatalf("Rerank() error = %v, want invalid-mode error", err)
		}
		if driver.calls != 0 {
			t.Fatalf("driver calls = %d, want 0", driver.calls)
		}
	})
}

func TestNewRerankModelDefaultsMaxTokens(t *testing.T) {
	model := NewRerankModel(&captureRerankDriver{}, nil, nil, 0)
	if model.MaxTokens != defaultMaxRerankTokens {
		t.Fatalf("MaxTokens = %d, want %d", model.MaxTokens, defaultMaxRerankTokens)
	}
}
