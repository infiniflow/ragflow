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

package component

import (
	"context"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// This build registers no extraction mode, so construction attaches nothing
// and Invoke's extra phase stays inert.
func attachResumeHook(*ExtractorComponent, map[string]any) {}

func PrepareDocumentPreview(ctx context.Context, _ *gorm.DB, _ entity.Document) (context.Context, func(context.Context)) {
	return ctx, nil
}

func bindDocumentPreview(ctx context.Context, _ string, _ *string) context.Context {
	return ctx
}
