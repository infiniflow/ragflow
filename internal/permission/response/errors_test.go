package permissionresponse

import (
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/permission"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    common.ErrorCode
		message string
	}{
		{name: "unauthenticated", err: permission.ErrUnauthenticated, code: common.CodeUnauthorized, message: "Authentication required"},
		{name: "missing resource", err: permission.ErrResourceNotFound, code: common.CodeNotFound, message: "Resource not found"},
		{name: "denied", err: permission.ErrPermissionDenied, code: common.CodeForbidden, message: "Permission denied"},
		{name: "missing membership", err: permission.ErrMembershipNotFound, code: common.CodeForbidden, message: "Permission denied"},
		{name: "internal permission failure", err: errors.New("database credentials must not leak"), code: common.CodeServerError, message: "Permission check failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, normalizedErr := Normalize(test.err)
			if code != test.code {
				t.Fatalf("code = %d, want %d", code, test.code)
			}
			if normalizedErr == nil || normalizedErr.Error() != test.message {
				t.Fatalf("error = %v, want message %q", normalizedErr, test.message)
			}
			if !errors.Is(normalizedErr, test.err) {
				t.Fatalf("normalized error %v does not wrap %v", normalizedErr, test.err)
			}
		})
	}
}

func TestNormalizeHidden(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    common.ErrorCode
		message string
	}{
		{name: "denied resource", err: permission.ErrPermissionDenied, code: common.CodeNotFound, message: "Resource not found"},
		{name: "missing resource", err: permission.ErrResourceNotFound, code: common.CodeNotFound, message: "Resource not found"},
		{name: "internal failure remains visible", err: errors.New("db error"), code: common.CodeServerError, message: "Permission check failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, normalizedErr := NormalizeHidden(test.err)
			if code != test.code || normalizedErr.Error() != test.message {
				t.Fatalf("NormalizeHidden() = (%d, %q), want (%d, %q)", code, normalizedErr, test.code, test.message)
			}
			if !errors.Is(normalizedErr, test.err) {
				t.Fatalf("normalized error %v does not wrap %v", normalizedErr, test.err)
			}
		})
	}
}

func TestIsPermissionError(t *testing.T) {
	if !IsPermissionError(fmt.Errorf("wrapped: %w", permission.ErrPermissionDenied)) {
		t.Fatal("IsPermissionError() = false for wrapped permission denial")
	}
	if IsPermissionError(errors.New("ordinary failure")) {
		t.Fatal("IsPermissionError() = true for ordinary failure")
	}
}
