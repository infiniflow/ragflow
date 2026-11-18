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

package service

import (
	"context"
	"errors"
	syncerconnector "ragflow/internal/syncer/connector"
	"time"
)

const (
	// DocumentActionAdded means a document was newly inserted.
	DocumentActionAdded = "added"
	// DocumentActionUpdated means a document was changed.
	DocumentActionUpdated = "updated"
	// DocumentActionSkipped means a document was unchanged.
	DocumentActionSkipped = "skipped"
)

// DocumentUpsertInput describes one normalized source document write.
type DocumentUpsertInput struct {
	TaskContext    SyncTaskContext
	SourceType     string
	DocumentID     string
	LegacyID       string
	NewID          string
	SourceDocument syncerconnector.SourceDocument
	AutoParse      bool
}

// DocumentUpsertResult describes one sink write result.
type DocumentUpsertResult struct {
	DocID  string
	Action string
}

// DocumentSink stores one normalized source document.
type DocumentSink interface {
	// Upsert stores one normalized source document.
	Upsert(ctx context.Context, input DocumentUpsertInput) (DocumentUpsertResult, error)
}

// RetryableError marks a failure as safe to retry.
type RetryableError struct {
	Err        error
	After      time.Duration
	Temporary  bool
	StatusCode int
}

// Error returns the wrapped error message.
func (e *RetryableError) Error() string {
	if e == nil || e.Err == nil {
		return "retryable sync error"
	}
	return e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *RetryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsRetryable reports whether an error should be retried.
func IsRetryable(err error) bool {
	if _, ok := errors.AsType[*RetryableError](err); ok {
		return true
	}
	return false
}
