package httputil

import (
	"errors"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

func IngestionTaskErrorCode(err error) common.ErrorCode {
	var transitionErr *service.InvalidTaskTransitionError
	if errors.As(err, &transitionErr) {
		return common.CodeConflict
	}
	var conflictErr *service.TaskStatusConflictError
	if errors.As(err, &conflictErr) {
		return common.CodeConflict
	}
	if errors.Is(err, common.ErrTaskNotFound) {
		return common.CodeNotFound
	}
	return common.CodeExceptionError
}
