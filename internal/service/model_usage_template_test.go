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

package service

import (
	"context"
	"testing"

	modelModule "ragflow/internal/entity/models"
)

func TestAttachUsageTemplateWithoutAttacher(t *testing.T) {
	usageTemplateAttacher = nil
	defer func() { usageTemplateAttacher = nil }()

	// The community build has no attacher installed; the call must be a no-op.
	attachUsageTemplate(context.Background(), nil, nil)
}

func TestAttachUsageTemplateInvokesInstalledAttacher(t *testing.T) {
	defer func() { usageTemplateAttacher = nil }()

	var gotCtx context.Context
	var gotInfo *modelModule.ModelInfo
	usageTemplateAttacher = func(ctx context.Context, _ *modelModule.APIConfig, info *modelModule.ModelInfo) {
		gotCtx, gotInfo = ctx, info
	}

	ctx := context.Background()
	info := &modelModule.ModelInfo{GroupID: "group-1"}
	attachUsageTemplate(ctx, nil, info)
	if gotCtx != ctx || gotInfo != info {
		t.Fatalf("attacher received ctx=%v info=%v, want the call-site arguments", gotCtx, gotInfo)
	}
}
