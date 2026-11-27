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
	"testing"

	"ragflow/internal/service"
)

type temporaryFetchError struct{ temporary bool }

func (e *temporaryFetchError) Error() string   { return "fetch failed" }
func (e *temporaryFetchError) Temporary() bool { return e.temporary }

func TestRetryableFetchErrorClassification(t *testing.T) {
	if got := retryableFetchError(nil); got != nil {
		t.Fatalf("nil error = %v, want nil", got)
	}

	permanent := errors.New("permanent failure")
	if got := retryableFetchError(permanent); !errors.Is(got, permanent) || service.IsRetryable(got) {
		t.Fatalf("plain error = %v, want unchanged non-retryable", got)
	}

	notTemporary := &temporaryFetchError{temporary: false}
	if got := retryableFetchError(notTemporary); !errors.Is(got, notTemporary) || service.IsRetryable(got) {
		t.Fatalf("Temporary()=false error = %v, want unchanged non-retryable", got)
	}

	temporary := &temporaryFetchError{temporary: true}
	got := retryableFetchError(temporary)
	if !errors.Is(got, temporary) {
		t.Fatalf("temporary error lost its cause: %v", got)
	}
	if !service.IsRetryable(got) {
		t.Fatalf("Temporary()=true error = %v, want retryable", got)
	}

	alreadyRetryable := &service.RetryableError{Err: permanent}
	if got := retryableFetchError(alreadyRetryable); got != alreadyRetryable {
		t.Fatalf("already retryable error = %v, want unchanged", got)
	}
}
