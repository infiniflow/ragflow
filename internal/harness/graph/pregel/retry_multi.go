package pregel

import (
	"context"
	"fmt"
	"time"

	"ragflow/internal/harness/graph/types"
)

// MultiPolicyRetryExecutor handles node execution with multiple retry policies.
// The first matching policy is used for each retry attempt.
// Internally delegates per-policy execution to RetryExecutor.
type MultiPolicyRetryExecutor struct {
	policies []types.RetryPolicy
}

// NewMultiPolicyRetryExecutor creates a new retry executor with multiple policies.
func NewMultiPolicyRetryExecutor(policies ...types.RetryPolicy) *MultiPolicyRetryExecutor {
	return &MultiPolicyRetryExecutor{
		policies: policies,
	}
}

// Execute executes a function with multi-policy retry logic.
// For each attempt, it finds the first matching policy and delegates retry
// behavior to RetryExecutor with that policy's configuration.
func (e *MultiPolicyRetryExecutor) Execute(
	ctx context.Context,
	name string,
	fn func(context.Context) (interface{}, error),
) (interface{}, error) {
	if len(e.policies) == 0 {
		return fn(ctx)
	}

	var lastErr error
	var lastOutput interface{}
	attempt := 0

	for {
		output, err := fn(ctx)
		if err == nil {
			return output, nil
		}

		lastErr = err
		lastOutput = output
		attempt++

		policy := e.findMatchingPolicy(err)
		if policy == nil {
			return nil, fmt.Errorf("node %s failed with non-retryable error: %w", name, err)
		}

		if attempt >= policy.MaxAttempts {
			break
		}

		// Delegate to RetryExecutor for backoff and remaining attempts
		remaining := policy.MaxAttempts - attempt
		limitedPolicy := *policy
		limitedPolicy.MaxAttempts = remaining + 1 // RetryExecutor counts from 1

		executor := NewRetryExecutor(&limitedPolicy)
		result, err := executor.Execute(ctx, name, fn)
		if err == nil {
			return result, nil
		}
		// RetryExecutor exhausted - fall through
		lastErr = err
		attempt += remaining
	}

	return nil, &RetryExhaustedError{
		NodeName:   name,
		Attempts:   attempt,
		LastErr:    lastErr,
		LastOutput: lastOutput,
	}
}

// findMatchingPolicy finds the first policy that matches the error.
func (e *MultiPolicyRetryExecutor) findMatchingPolicy(err error) *types.RetryPolicy {
	for i := range e.policies {
		policy := &e.policies[i]
		if policy.RetryOn == nil {
			return policy
		}
		if policy.RetryOn(err) {
			return policy
		}
	}
	return nil
}

// MultiRetryConfig provides configuration for multi-policy retry behavior.
type MultiRetryConfig struct {
	Policies  []types.RetryPolicy
	OnRetry   func(attempt int, policyIndex int, err error)
	OnSuccess func(attempt int)
	OnPolicyMatch func(attempt int, policyIndex int)
}

// NewMultiRetryConfig creates a new multi-retry config with defaults.
func NewMultiRetryConfig(policies ...types.RetryPolicy) *MultiRetryConfig {
	if len(policies) == 0 {
		defaultPolicy := types.DefaultRetryPolicy()
		policies = []types.RetryPolicy{defaultPolicy}
	}
	return &MultiRetryConfig{
		Policies: policies,
	}
}

// WithOnRetry sets the retry callback.
func (c *MultiRetryConfig) WithOnRetry(callback func(attempt int, policyIndex int, err error)) *MultiRetryConfig {
	c.OnRetry = callback
	return c
}

// WithOnSuccess sets the success callback.
func (c *MultiRetryConfig) WithOnSuccess(callback func(attempt int)) *MultiRetryConfig {
	c.OnSuccess = callback
	return c
}

// WithOnPolicyMatch sets the policy match callback.
func (c *MultiRetryConfig) WithOnPolicyMatch(callback func(attempt int, policyIndex int)) *MultiRetryConfig {
	c.OnPolicyMatch = callback
	return c
}

// CreateExecutor creates a MultiPolicyRetryExecutor from this config.
func (c *MultiRetryConfig) CreateExecutor() *MultiPolicyRetryExecutor {
	return NewMultiPolicyRetryExecutor(c.Policies...)
}

// Common policy presets for multi-policy retry.
// Each preset pairs error-matching predicates with RetryExecutor-compatible policies.
var RetryPolicyPresets = struct {
	NetworkErrors   types.RetryPolicy
	TemporaryErrors types.RetryPolicy
	ResourceErrors  types.RetryPolicy
	TimeoutErrors   types.RetryPolicy
}{
	NetworkErrors: types.RetryPolicy{
		MaxAttempts:     5,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          true,
		RetryOn:         RetryPredicates.NetworkErrors,
	},
	TemporaryErrors: types.RetryPolicy{
		MaxAttempts:     3,
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     10 * time.Second,
		BackoffFactor:   1.5,
		Jitter:          true,
		RetryOn:         RetryPredicates.TemporaryErrors,
	},
	ResourceErrors: types.RetryPolicy{
		MaxAttempts:     10,
		InitialInterval: 1 * time.Second,
		MaxInterval:     60 * time.Second,
		BackoffFactor:   1.5,
		Jitter:          true,
		RetryOn: func(err error) bool {
			if err == nil { return false }
			errMsg := err.Error()
			for _, kw := range []string{"resource exhausted", "too many requests", "rate limit", "quota exceeded", "429", "503"} {
				if contains(errMsg, kw) { return true }
			}
			return false
		},
	},
	TimeoutErrors: types.RetryPolicy{
		MaxAttempts:     3,
		InitialInterval: 1 * time.Second,
		MaxInterval:     30 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          false,
		RetryOn: func(err error) bool {
			if err == nil { return false }
			errMsg := err.Error()
			for _, kw := range []string{"timeout", "deadline exceeded", "context deadline", "408", "504"} {
				if contains(errMsg, kw) { return true }
			}
			return false
		},
	},
}

// CreateDefaultMultiPolicy creates a default multi-policy retry configuration.
func CreateDefaultMultiPolicy() *MultiRetryConfig {
	return NewMultiRetryConfig(
		RetryPolicyPresets.NetworkErrors,
		RetryPolicyPresets.TemporaryErrors,
		RetryPolicyPresets.TimeoutErrors,
		RetryPolicyPresets.ResourceErrors,
	)
}
