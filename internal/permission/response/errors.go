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

package permissionresponse

import (
	"errors"

	"ragflow/internal/common"
	"ragflow/internal/permission"
)

// Normalize maps permission decisions to stable API codes and safe messages
// while retaining the original error for errors.Is/errors.As.
func Normalize(err error) (common.ErrorCode, error) {
	return normalize(err, false)
}

// NormalizeHidden hides whether a resource exists when the caller is not
// allowed to access it.
func NormalizeHidden(err error) (common.ErrorCode, error) {
	return normalize(err, true)
}

// IsPermissionError reports whether err carries a typed permission decision.
func IsPermissionError(err error) bool {
	return errors.Is(err, permission.ErrUnauthenticated) ||
		errors.Is(err, permission.ErrResourceNotFound) ||
		errors.Is(err, permission.ErrMembershipNotFound) ||
		errors.Is(err, permission.ErrPermissionDenied)
}

func normalize(err error, hideResource bool) (common.ErrorCode, error) {
	if err == nil {
		return common.CodeSuccess, nil
	}

	code := common.CodeServerError
	message := "Permission check failed"
	switch {
	case errors.Is(err, permission.ErrUnauthenticated):
		code = common.CodeUnauthorized
		message = "Authentication required"
	case errors.Is(err, permission.ErrResourceNotFound):
		code = common.CodeNotFound
		message = "Resource not found"
	case errors.Is(err, permission.ErrPermissionDenied), errors.Is(err, permission.ErrMembershipNotFound):
		code = common.CodeForbidden
		message = "Permission denied"
	}
	if hideResource && (code == common.CodeForbidden || code == common.CodeNotFound) {
		code = common.CodeNotFound
		message = "Resource not found"
	}

	return code, &normalizedError{cause: err, message: message}
}

type normalizedError struct {
	cause   error
	message string
}

func (e *normalizedError) Error() string { return e.message }

func (e *normalizedError) Unwrap() error { return e.cause }
