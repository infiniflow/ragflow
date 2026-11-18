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

package models

import (
	"testing"

	"ragflow/internal/common"
)

func TestUsageForPassthroughWithoutResolver(t *testing.T) {
	usageTemplateResolver = nil
	defer func() { usageTemplateResolver = nil }()

	usage := &common.ModelUsage{UserID: "user-1"}
	if got := usageFor(nil, usage); got != usage {
		t.Fatal("without a resolver the explicit usage must pass through untouched")
	}
	if got := usageFor(&ModelInfo{}, nil); got != nil {
		t.Fatal("without a resolver a nil usage must stay nil")
	}
}

func TestUsageForInvokesInstalledResolver(t *testing.T) {
	defer func() { usageTemplateResolver = nil }()

	info := &ModelInfo{GroupID: "group-1"}
	var gotInfo *ModelInfo
	var gotUsage *common.ModelUsage
	usageTemplateResolver = func(info *ModelInfo, usage *common.ModelUsage) *common.ModelUsage {
		gotInfo, gotUsage = info, usage
		return &common.ModelUsage{GroupID: "resolved"}
	}

	resolved := usageFor(info, nil)
	if gotInfo != info || gotUsage != nil {
		t.Fatalf("resolver received info=%v usage=%v, want the call-site arguments", gotInfo, gotUsage)
	}
	if resolved == nil || resolved.GroupID != "resolved" {
		t.Fatalf("usageFor must forward the resolver's return value, got %+v", resolved)
	}
}
