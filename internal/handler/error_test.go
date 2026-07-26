package handler

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

func TestIngestionTaskErrorCode(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want common.ErrorCode
	}{
		{
			name: "invalid transition",
			err:  &service.InvalidTaskTransitionError{TaskID: "task-1", From: common.CREATED, To: common.COMPLETED},
			want: common.CodeConflict,
		},
		{
			name: "status conflict",
			err:  &service.TaskStatusConflictError{TaskID: "task-1", ExpectedFrom: common.CREATED, AttemptedTo: common.RUNNING, ActualCurrent: common.STOPPING},
			want: common.CodeConflict,
		},
		{
			name: "task not found",
			err:  common.ErrTaskNotFound,
			want: common.CodeNotFound,
		},
		{
			name: "fallback",
			err:  common.ErrInvalidToken,
			want: common.CodeExceptionError,
		},
	}

	for _, tc := range testCases {
		if got := IngestionTaskErrorCode(tc.err); got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
