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

package transaction

import (
	"context"
	"errors"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// PickModelFromGroup is unsupported in the community edition: model groups are
// an enterprise-only feature, so every gp- model reference fails resolution
// with this error and callers keep their non-group fallback path.
func PickModelFromGroup(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (*entity.TenantModel, error) {
	return nil, errors.New("model group references are not supported")
}
