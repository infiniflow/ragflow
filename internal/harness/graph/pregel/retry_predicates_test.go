package pregel

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/harness/graph/types"
)

func TestRetryPredicatesMatchEmbeddedErrorMarkers(t *testing.T) {
	tests := []struct {
		name      string
		predicate func(error) bool
		err       error
		want      bool
	}{
		{
			name:      "network failure wrapped with context",
			predicate: RetryPredicates.NetworkErrors,
			err:       errors.New("rpc call failed: CONNECTION RESET by peer"),
			want:      true,
		},
		{
			name:      "HTTP status after a prefix",
			predicate: RetryPredicates.NetworkErrors,
			err:       errors.New("request failed with HTTP 503 Service Unavailable"),
			want:      true,
		},
		{
			name:      "network error at start",
			predicate: RetryPredicates.NetworkErrors,
			err:       errors.New("connection refused"),
			want:      true,
		},
		{
			name:      "unrelated network error",
			predicate: RetryPredicates.NetworkErrors,
			err:       errors.New("invalid authentication credentials"),
			want:      false,
		},
		{
			name:      "temporary error after a prefix",
			predicate: RetryPredicates.TemporaryErrors,
			err:       errors.New("provider replied: TOO MANY REQUESTS"),
			want:      true,
		},
		{
			name:      "rate limit status after a prefix",
			predicate: RetryPredicates.TemporaryErrors,
			err:       errors.New("API call returned HTTP 429"),
			want:      true,
		},
		{
			name:      "unrelated temporary error",
			predicate: RetryPredicates.TemporaryErrors,
			err:       errors.New("malformed tool arguments"),
			want:      false,
		},
		{
			name:      "nil error",
			predicate: RetryPredicates.TemporaryErrors,
			err:       nil,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.predicate(tt.err); got != tt.want {
				t.Errorf("predicate(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryExecutorRetriesEmbeddedNetworkError(t *testing.T) {
	policy := &types.RetryPolicy{
		MaxAttempts:   2,
		BackoffFactor: 1,
		RetryOn:       RetryPredicates.NetworkErrors,
	}
	executor := NewRetryExecutor(policy)
	attempts := 0

	output, err := executor.Execute(context.Background(), "tool-node", func(context.Context) (any, error) {
		attempts++
		if attempts == 1 {
			return nil, fmt.Errorf("tool request failed: %w", errors.New("connection reset by peer"))
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if output != "recovered" {
		t.Errorf("Execute() output = %v, want recovered", output)
	}
	if attempts != 2 {
		t.Errorf("Execute() attempts = %d, want 2", attempts)
	}
}
