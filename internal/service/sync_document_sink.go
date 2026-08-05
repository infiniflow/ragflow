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

// TODO: implement a production DocumentSink by adding an exported connector
// upload/upsert entrypoint to internal/service/document, so Go sync keeps using
// the existing Storage/File/File2Document/Document/metadata/parse-task path.

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
