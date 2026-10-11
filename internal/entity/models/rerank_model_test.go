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
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{document}}, nil, nil)
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
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{document}}, nil, nil)
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
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{"short"}}, nil, nil)
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
		_, err := model.Rerank(t.Context(), RerankRequest{Query: query, Documents: []string{"short"}}, nil, nil)
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

// scriptedRerankDriver returns whatever fail produces for each call, and records
// every request's documents so a test can see whether they shrank.
type scriptedRerankDriver struct {
	ModelDriver
	fail  func(request RerankRequest, call int) error
	calls int
	docs  [][]string
}

func (d *scriptedRerankDriver) Rerank(_ context.Context, _ *string, request RerankRequest, _ *APIConfig, _ *RerankConfig, _ *common.ModelUsage) (*RerankResponse, error) {
	d.calls++
	d.docs = append(d.docs, request.Documents)
	if d.fail != nil {
		if err := d.fail(request, d.calls); err != nil {
			return nil, err
		}
	}
	return &RerankResponse{Data: []RerankResult{{Index: 0, RelevanceScore: 1}}}, nil
}

func llamaRerankOverLimit() error {
	return errors.New(`vLLM rerank API error: 500 Internal Server Error, body: {"error":{"message":"input is too large to process. increase the physical batch size"}}`)
}

// A provider that rejects a window we computed from its own declared limit must
// not fail the call: the documents are re-cut with a smaller budget and the
// calibration is told, exactly as the embedding path does.
func TestRerankModelShrinksOnOverLimit(t *testing.T) {
	t.Setenv(common.EnvRerankTokenLimitMode, "truncate")
	document := strings.Repeat("token ", 2000)
	driver := &scriptedRerankDriver{
		fail: func(request RerankRequest, _ int) error {
			if len(request.Documents[0]) > 1024 {
				return llamaRerankOverLimit()
			}
			return nil
		},
	}
	model := NewRerankModel(driver, nil, nil, 512)
	if _, err := model.Rerank(t.Context(), RerankRequest{Query: "q", Documents: []string{document}}, nil, nil); err != nil {
		t.Fatalf("Rerank() error = %v, want success after shrinking", err)
	}
	if driver.calls < 2 {
		t.Fatalf("driver calls = %d, want a retry with a smaller budget", driver.calls)
	}
	first, last := driver.docs[0][0], driver.docs[len(driver.docs)-1][0]
	if len(last) >= len(first) {
		t.Fatalf("documents did not shrink across attempts: first=%d bytes last=%d bytes", len(first), len(last))
	}
}

// Only over-limit rejections are retried; a genuine failure is returned at once.
func TestRerankModelDoesNotRetryOtherErrors(t *testing.T) {
	t.Setenv(common.EnvRerankTokenLimitMode, "truncate")
	driver := &scriptedRerankDriver{
		fail: func(RerankRequest, int) error { return errors.New("connection refused") },
	}
	model := NewRerankModel(driver, nil, nil, 512)
	_, err := model.Rerank(t.Context(), RerankRequest{Query: "q", Documents: []string{"doc"}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("Rerank() error = %v, want the driver error", err)
	}
	if driver.calls != 1 {
		t.Fatalf("driver calls = %d, want 1 (no retry for a non-size error)", driver.calls)
	}
}

func TestRerankModelQuotaKeyNamesDeployment(t *testing.T) {
	baseA, baseB, apiKey, name := "http://a/v1", "http://b/v1", "k", "bge-reranker-v2-m3"
	keyA := NewRerankModel(&captureRerankDriver{}, &name, &APIConfig{BaseURL: &baseA, ApiKey: &apiKey}, 10).QuotaKey()
	keyB := NewRerankModel(&captureRerankDriver{}, &name, &APIConfig{BaseURL: &baseB, ApiKey: &apiKey}, 10).QuotaKey()
	if keyA == "" {
		t.Fatal("QuotaKey() is empty")
	}
	if keyA == keyB {
		t.Fatalf("QuotaKey() = %q for two endpoints, want them distinct", keyA)
	}
	unnamed := NewRerankModel(&captureRerankDriver{}, nil, &APIConfig{BaseURL: &baseA, ApiKey: &apiKey}, 10).QuotaKey()
	if unnamed == keyA {
		t.Fatalf("QuotaKey() ignored the model name: %q", unnamed)
	}
}
