package handler

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/permission"
)

func TestPermissionErrorResult(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		hide    bool
		code    common.ErrorCode
		message string
	}{
		{name: "denial", err: permission.ErrPermissionDenied, code: common.CodeForbidden, message: "Permission denied"},
		{name: "hidden denial", err: permission.ErrPermissionDenied, hide: true, code: common.CodeNotFound, message: "Resource not found"},
		{name: "hidden missing resource", err: permission.ErrResourceNotFound, hide: true, code: common.CodeNotFound, message: "Resource not found"},
		{name: "internal error is not hidden", err: errors.New("database error"), hide: true, code: common.CodeServerError, message: "Permission check failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, message := permissionErrorResult(test.err, test.hide)
			if code != test.code || message != test.message {
				t.Fatalf("permissionErrorResult() = (%d, %q), want (%d, %q)", code, message, test.code, test.message)
			}
		})
	}
}
