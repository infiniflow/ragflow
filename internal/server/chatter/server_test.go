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

package chatter

import (
	"ragflow/internal/common"
	"strings"
	"testing"
)

func TestRunRequiresSharedRuntimeCache(t *testing.T) {
	err := Run(t.Context(), common.ServerTypeChatter, nil)
	if err == nil || !strings.Contains(err.Error(), "requires Kvrocks") {
		t.Fatalf("Run without Kvrocks = %v, want an explicit shared-status startup error", err)
	}
}
