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
//

package service

import (
	"context"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func tenantMemberPermissionEE(context.Context, *gorm.DB, string, string, int32) (*entity.UserTenant, int32, bool, error) {
	return nil, 0, false, nil
}

func recordTenantPermissionChangeEE(*gorm.DB, string, string, string, string, string, string, string, string, int32, int32, interface{}, interface{}) error {
	return nil
}
