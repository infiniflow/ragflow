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

package dataset

import (
	"testing"

	"ragflow/internal/dao"
)

func TestPipelineVersionOutdatedBatch_MatchesSingleRow(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	canvasID := "0123456789abcdef0123456789abcdef"
	verOld := "version0000000000000000000000aa"
	verNew := "version0000000000000000000000bb"
	seedDataflowCanvas(t, db, canvasID)
	seedCanvasVersion(t, db, canvasID, verOld, 0)
	seedCanvasVersion(t, db, canvasID, verNew, 1000)

	ctx := t.Context()
	m, err := dao.NewUserCanvasVersionDAO().LatestIDsByCanvasIDs(ctx, db, []string{canvasID})
	if err != nil {
		t.Fatal(err)
	}
	if m[canvasID] != verNew {
		t.Fatalf("LatestIDsByCanvasIDs[%s] = %q, want %q", canvasID, m[canvasID], verNew)
	}
	if !pipelineVersionOutdatedBatch(m, &canvasID, &verOld) {
		t.Fatal("batch: pinned older version should be outdated")
	}
	if pipelineVersionOutdatedBatch(m, &canvasID, &verNew) {
		t.Fatal("batch: pinned latest version should not be outdated")
	}
	if got, want := pipelineVersionOutdatedBatch(m, &canvasID, &verOld), pipelineVersionOutdated(ctx, &canvasID, &verOld); got != want {
		t.Fatalf("batch vs single-row (old) = %v, want %v", got, want)
	}
	if got, want := pipelineVersionOutdatedBatch(m, &canvasID, &verNew), pipelineVersionOutdated(ctx, &canvasID, &verNew); got != want {
		t.Fatalf("batch vs single-row (new) = %v, want %v", got, want)
	}
	if pipelineVersionOutdatedBatch(m, &canvasID, nil) {
		t.Fatal("no pinned version should never be outdated")
	}
}
