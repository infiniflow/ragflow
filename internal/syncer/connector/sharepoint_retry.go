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

package connector

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// sharePointRetryDelay never shortens a valid server-requested delay to the
// backoff limit. The caller's context bounds the wait instead.
func sharePointRetryDelay(header string, fallback time.Duration, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	seconds, err := strconv.ParseUint(header, 10, 64)
	const maxDelay = time.Duration(1<<63 - 1)
	if errors.Is(err, strconv.ErrRange) || (err == nil && seconds > uint64(maxDelay/time.Second)) {
		return maxDelay
	}
	if err == nil {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(header); err == nil {
		return max(retryAt.Sub(now), 0)
	}
	return fallback
}
