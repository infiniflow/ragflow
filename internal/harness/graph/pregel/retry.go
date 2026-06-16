// Package pregel provides enhanced retry policies for Pregel execution.
package pregel

import (
	"context"
	"fmt"
	"time"

	"ragflow/internal/harness/graph/types"
)

// RetryExecutor handles node execution with sophisticated retry logic.
type RetryExecutor struct {
	policy *types.RetryPolicy
}

// NewRetryExecutor creates a new retry executor with the given policy.
func NewRetryExecutor(policy *types.RetryPolicy) *RetryExecutor {
	if policy == nil {
		defaultPolicy := types.DefaultRetryPolicy()
		policy = &defaultPolicy
	}
	return &RetryExecutor{policy: policy}
}

// Execute executes a function with retry logic.
func (e *RetryExecutor) Execute(ctx context.Context, name string, fn func(context.Context) (interface{}, error)) (output interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			output = nil
			err = fmt.Errorf("node %s panicked: %v", name, r)
		}
	}()
	
	var lastErr error
	var lastOutput interface{}
	
	for attempt := 1; attempt <= e.policy.MaxAttempts; attempt++ {
		// Execute the function
		output, err = fn(ctx)
		if err == nil {
			return output, nil
		}
		
		// Check if this is a non-retryable error
		if e.policy.RetryOn != nil && !e.policy.RetryOn(err) {
			return nil, fmt.Errorf("node %s failed with non-retryable error: %w", name, err)
		}
		
		lastErr = err
		lastOutput = output
		
		// If we've exhausted attempts, break
		if attempt >= e.policy.MaxAttempts {
			break
		}
		
		// Calculate backoff with jitter
		backoff := e.calculateBackoff(attempt)
		
		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("node %s cancelled during retry: %w", name, ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}
	
	return nil, &RetryExhaustedError{
		NodeName:  name,
		Attempts:  e.policy.MaxAttempts,
		LastErr:   lastErr,
		LastOutput: lastOutput,
	}
}

// calculateBackoff calculates the backoff duration with optional jitter.
// Delegates to the shared RetryPolicy.CalculateBackoff method.
func (e *RetryExecutor) calculateBackoff(attempt int) time.Duration {
	if e.policy == nil {
		defaultPolicy := types.DefaultRetryPolicy()
		return defaultPolicy.CalculateBackoff(attempt)
	}
	return e.policy.CalculateBackoff(attempt)
}

// RetryExhaustedError is returned when all retry attempts are exhausted.
type RetryExhaustedError struct {
	NodeName   string
	Attempts   int
	LastErr    error
	LastOutput interface{}
}

// Error implements the error interface.
func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("node %s failed after %d attempts: %v", e.NodeName, e.Attempts, e.LastErr)
}

// Unwrap returns the underlying error.
func (e *RetryExhaustedError) Unwrap() error {
	return e.LastErr
}

// IsRetryExhausted checks if an error is a RetryExhaustedError.
func IsRetryExhausted(err error) bool {
	_, ok := err.(*RetryExhaustedError)
	return ok
}

// RetryPredicates provides common retry condition predicates.
var RetryPredicates = struct {
	Always          func(error) bool
	Never           func(error) bool
	NetworkErrors   func(error) bool
	TemporaryErrors func(error) bool
}{
	Always: func(error) bool {
		return true
	},
	Never: func(error) bool {
		return false
	},
	// NetworkErrors retries on common network-related errors
	NetworkErrors: func(err error) bool {
		if err == nil {
			return false
		}
		// Check for common network error patterns
		errMsg := err.Error()
		networkKeywords := []string{
			"connection refused",
			"connection reset",
			"timeout",
			"network",
			"dns",
			"temporary failure",
			"503", // Service Unavailable
			"502", // Bad Gateway
			"504", // Gateway Timeout
		}
		for _, kw := range networkKeywords {
			if contains(errMsg, kw) {
				return true
			}
		}
		return false
	},
	// TemporaryErrors retries on errors that might be transient
	TemporaryErrors: func(err error) bool {
		if err == nil {
			return false
		}
		// Check for temporary error patterns
		errMsg := err.Error()
		tempKeywords := []string{
			"temporary",
			"transient",
			"rate limit",
			"too many requests",
			"429", // Too Many Requests
		}
		for _, kw := range tempKeywords {
			if contains(errMsg, kw) {
				return true
			}
		}
		return false
	},
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 len(s) > len(substr) && 
		 (s[0:len(substr)] == substr || 
		  containsIgnoreCase(s, substr)))
}

// containsIgnoreCase performs case-insensitive substring check.
func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return len(s) >= len(substr) && s[:len(substr)] == substr
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// RetryConfig provides configuration for retry behavior.
type RetryConfig struct {
	// Policy is the retry policy to use
	Policy *types.RetryPolicy
	// OnRetry is called after each failed attempt
	OnRetry func(attempt int, err error)
	// OnSuccess is called on successful completion
	OnSuccess func(attempt int)
}

// NewRetryConfig creates a new retry config with defaults.
func NewRetryConfig() *RetryConfig {
	defaultPolicy := types.DefaultRetryPolicy()
	return &RetryConfig{
		Policy: &defaultPolicy,
	}
}

// WithRetryOn sets the retry-on predicate.
func (c *RetryConfig) WithRetryOn(predicate func(error) bool) *RetryConfig {
	c.Policy.RetryOn = predicate
	return c
}

// WithMaxAttempts sets the maximum number of attempts.
func (c *RetryConfig) WithMaxAttempts(maxAttempts int) *RetryConfig {
	c.Policy.MaxAttempts = maxAttempts
	return c
}

// WithBackoff sets the backoff parameters.
func (c *RetryConfig) WithBackoff(initial, max time.Duration, factor float64) *RetryConfig {
	c.Policy.InitialInterval = initial
	c.Policy.MaxInterval = max
	c.Policy.BackoffFactor = factor
	return c
}

// WithJitter enables or disables jitter.
func (c *RetryConfig) WithJitter(enabled bool) *RetryConfig {
	c.Policy.Jitter = enabled
	return c
}

// WithOnRetry sets the callback to call after each retry.
func (c *RetryConfig) WithOnRetry(callback func(attempt int, err error)) *RetryConfig {
	c.OnRetry = callback
	return c
}

// WithOnSuccess sets the callback to call on success.
func (c *RetryConfig) WithOnSuccess(callback func(attempt int)) *RetryConfig {
	c.OnSuccess = callback
	return c
}
