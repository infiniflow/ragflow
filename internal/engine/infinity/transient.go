//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package infinity

import (
	"errors"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

// IsTransientUnavailable reports whether err means Infinity was still starting
// or replaying its log when the call ran. The thrift port accepts requests
// during that window but every query is answered with these codes, and the
// same call succeeds once init completes — so callers must redeliver instead
// of treating the failure as terminal.
//
// The error-code check covers errors from the SDK. The fallback matches the
// SDK's rendered form ("InfinityException(2007, ...)") for an error that lost
// its type across a layer; the code keeps it from matching unrelated text that
// merely quotes the startup message.
func IsTransientUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var infinityErr *infinity.InfinityException
	if errors.As(err, &infinityErr) {
		switch infinity.ErrorCode(infinityErr.ErrorCode) {
		case infinity.ErrorCodeInfinityIsStarting, infinity.ErrorCodeInfinityIsIniting:
			return true
		}
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "infinityexception(2007") || strings.Contains(msg, "infinityexception(2008")
}
