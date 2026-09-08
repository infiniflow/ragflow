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

package canvas

import "errors"

const (
	// RunErrorKindInternal marks an error whose cause is safe for server logs
	// but must not be exposed through Agent event streams.
	RunErrorKindInternal = "internal"

	// InternalRunErrorMessage is the public message used for internal Agent
	// execution failures.
	InternalRunErrorMessage = "Internal storage error while accessing the agent."
)

type internalRunError struct {
	cause error
}

// NewInternalRunError preserves cause for server-side logging and errors.Is
// while exposing only InternalRunErrorMessage in the terminal RunEvent.
func NewInternalRunError(cause error) error {
	if cause == nil {
		cause = errors.New("internal agent run error")
	}
	return &internalRunError{cause: cause}
}

func (e *internalRunError) Error() string {
	return e.cause.Error()
}

func (e *internalRunError) Unwrap() error {
	return e.cause
}

func runErrorEvent(err error) ErrorEvent {
	var internalErr *internalRunError
	if errors.As(err, &internalErr) {
		return ErrorEvent{
			Message: InternalRunErrorMessage,
			Kind:    RunErrorKindInternal,
		}
	}
	return ErrorEvent{Message: err.Error()}
}
