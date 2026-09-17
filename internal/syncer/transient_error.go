//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package syncer

import (
	"errors"
	"io"
	"net"
	"ragflow/internal/service"
	"strings"
)

// maxTransientTaskRetries is the total attempt budget for whitelisted
// (transient) task errors: 3 retries plus the final attempt.
const maxTransientTaskRetries int64 = 4

// maxNonTransientTaskRetries is the total attempt budget for non-whitelisted
// task errors: 2 retries plus the final attempt.
const maxNonTransientTaskRetries int64 = 3

// Error class identifiers persisted in sync_logs.error_class. Each class
// carries its own retry budget so one class's failures never consume another.
const (
	taskErrorClassTransient    = "transient"
	taskErrorClassNonTransient = "non_transient"
)

// taskErrorClass returns the persisted retry-class identifier for a task error.
func taskErrorClass(err error) string {
	if isTransientSyncError(err) {
		return taskErrorClassTransient
	}
	return taskErrorClassNonTransient
}

// maxTaskRetries returns the total attempt budget for a task error:
// whitelisted (transient) errors get more retries than other errors.
func maxTaskRetries(err error) int64 {
	if isTransientSyncError(err) {
		return maxTransientTaskRetries
	}
	return maxNonTransientTaskRetries
}

// isTransientSyncError reports whether a task-level sync error is worth
// extra retries.
func isTransientSyncError(err error) bool {
	if err == nil {
		return false
	}
	if service.IsRetryable(err) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "temporary") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "http 429") ||
		strings.Contains(message, "http 500") ||
		strings.Contains(message, "http 502") ||
		strings.Contains(message, "http 503") ||
		strings.Contains(message, "http 504")
}
