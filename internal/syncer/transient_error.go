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

// maxTransientTaskRetries max retry time when network error occur
const maxTransientTaskRetries int64 = 3

// isTransientSyncError reports whether a task-level sync error is worth retrying.
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
