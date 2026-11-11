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

package permission

import (
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"
)

func TestIsPlatformModelAccessDenied(t *testing.T) {
	for _, err := range []error{ErrPermissionDenied, ErrMembershipNotFound, ErrIdentityMismatch, ErrUnauthenticated, fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound)} {
		if !IsPlatformModelAccessDenied(err) {
			t.Errorf("%v should mean no access", err)
		}
	}
	// A database failure is not a decision and must be reported as it is.
	if IsPlatformModelAccessDenied(errors.New("connection refused")) {
		t.Error("database error treated as no access")
	}
}
