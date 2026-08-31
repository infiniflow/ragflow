package service

import (
	"errors"
	"testing"

	"ragflow/internal/common"
)

func TestValidateTransitionAllowsExpectedEdges(t *testing.T) {
	testCases := []struct {
		from string
		to   string
	}{
		{from: common.CREATED, to: common.RUNNING},
		{from: common.CREATED, to: common.STOPPED},
		{from: common.RUNNING, to: common.STOPPING},
		{from: common.RUNNING, to: common.COMPLETED},
		{from: common.RUNNING, to: common.FAILED},
		{from: common.STOPPING, to: common.STOPPED},
		{from: common.FAILED, to: common.CREATED},
		{from: common.STOPPED, to: common.CREATED},
	}

	for _, tc := range testCases {
		if err := validateTransition(tc.from, tc.to); err != nil {
			t.Fatalf("validateTransition(%q, %q) failed: %v", tc.from, tc.to, err)
		}
	}
}

func TestValidateTransitionRejectsInvalidEdge(t *testing.T) {
	err := validateTransition(common.CREATED, common.COMPLETED)
	if err == nil {
		t.Fatal("expected invalid transition to be rejected")
	}
	var transitionErr *InvalidTaskTransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("expected InvalidTaskTransitionError, got %T", err)
	}
	if transitionErr.TaskID != "" || transitionErr.From != common.CREATED || transitionErr.To != common.COMPLETED {
		t.Fatalf("unexpected transition error: %+v", transitionErr)
	}
}

func TestTaskStatusConflictErrorSupportsErrorsAs(t *testing.T) {
	err := &TaskStatusConflictError{TaskID: "task-1", ExpectedFrom: common.CREATED, AttemptedTo: common.RUNNING, ActualCurrent: common.STOPPING}
	var conflictErr *TaskStatusConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected TaskStatusConflictError, got %T", err)
	}
	if conflictErr.TaskID != "task-1" || conflictErr.ExpectedFrom != common.CREATED || conflictErr.AttemptedTo != common.RUNNING || conflictErr.ActualCurrent != common.STOPPING {
		t.Fatalf("unexpected conflict error: %+v", conflictErr)
	}
}
