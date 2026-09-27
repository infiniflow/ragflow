package common

import (
	"fmt"
	"time"
)

const retryFailureReportEvery = 3

// RetryFailureReporter keeps transient retry noise from becoming one event per
// provider attempt. It emits the first failure, then one coalesced update for
// every retryFailureReportEvery repeated failures, and a final summary when
// the caller exhausts retries.
type RetryFailureReporter struct {
	lastError string
	failures  int
}

// FailureMessage returns a user-facing progress message when this failure is
// worth emitting. attempt is one-based and total is the maximum number of
// attempts for the operation.
func (r *RetryFailureReporter) FailureMessage(attempt, total int, delay time.Duration, err error, final bool) (string, bool) {
	if r == nil {
		return "", false
	}
	compact := CompactError(err)
	if compact != r.lastError {
		r.lastError = compact
		r.failures = 1
		return formatRetryFailure(attempt, total, delay, compact), true
	}
	r.failures++
	if final {
		return fmt.Sprintf("[ERROR] LLM call failed after %d attempts (%d repeated failures): %s", attempt, r.failures-1, compact), true
	}
	if r.failures > 1 && (r.failures-1)%retryFailureReportEvery == 0 {
		return fmt.Sprintf("[ERROR] LLM call failed: %d repeated failures (latest attempt %d/%d): %s%s", r.failures-1, attempt, total, compact, retryDelaySuffix(delay)), true
	}
	return "", false
}

// RecoveryMessage returns one summary when a retry eventually succeeds after
// repeated failures. Calling it resets the reporter for a later retry cycle.
func (r *RetryFailureReporter) RecoveryMessage(attempt int) (string, bool) {
	if r == nil || r.failures <= 1 {
		return "", false
	}
	message := fmt.Sprintf("[INFO] LLM call recovered after %d failures on attempt %d", r.failures, attempt)
	r.lastError = ""
	r.failures = 0
	return message, true
}

func formatRetryFailure(attempt, total int, delay time.Duration, compact string) string {
	return fmt.Sprintf("[ERROR] LLM call failed (attempt %d/%d): %s%s", attempt, total, compact, retryDelaySuffix(delay))
}

func retryDelaySuffix(delay time.Duration) string {
	if delay <= 0 {
		return ""
	}
	return fmt.Sprintf("; retrying in %s", delay)
}
