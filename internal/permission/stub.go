//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build !ee

package permission

import (
	"context"

	"gorm.io/gorm"
)

// newPolicyEE reports that the OSS build has no enterprise policy: the caller
// falls back to communityPolicy.
func newPolicyEE(Source) (policy, bool) { return nil, false }

// tenantRoleMembershipEnabled reports whether tenant roles are stored as
// numeric IDs. The OSS build only stores the built-in roles.
func tenantRoleMembershipEnabled(db *gorm.DB) bool { return false }

// isTenantRoleID reports whether roleValue names an enterprise tenant role.
// The OSS build has none.
func isTenantRoleID(ctx context.Context, db *gorm.DB, tenantID, roleValue string) (bool, error) {
	return false, nil
}
