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

package runtime_test

import (
	"testing"

	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
)

func TestCanvasStateSnapshotClonesTypedJSONComposites(t *testing.T) {
	t.Parallel()
	var nilMap map[string]string
	var nilDownloads []component.DownloadInfo
	payload := map[string]any{
		"typed_map": map[string]string{"status": "ready"},
		"nested":    []map[string][]string{{"tags": {"one", "two"}}},
		"downloads": []component.DownloadInfo{{
			DocID:    "doc-1",
			Filename: "report.pdf",
		}},
		"nil_map":       nilMap,
		"nil_downloads": nilDownloads,
	}
	state := runtime.NewCanvasState("run-copy", "task-copy")
	state.AppendHistory("assistant", payload)

	cloned := state.SnapshotHistory()[0]["payload"].(map[string]any)
	cloned["typed_map"].(map[string]any)["status"] = "changed"
	cloned["nested"].([]any)[0].(map[string]any)["tags"].([]any)[0] = "changed"
	cloned["downloads"].([]any)[0].(map[string]any)["filename"] = "changed.pdf"
	if cloned["nil_map"].(map[string]any) != nil {
		t.Fatalf("nil typed map was not preserved: %#v", cloned["nil_map"])
	}
	if cloned["nil_downloads"].([]any) != nil {
		t.Fatalf("nil typed slice was not preserved: %#v", cloned["nil_downloads"])
	}

	unchanged := state.SnapshotHistory()[0]["payload"].(map[string]any)
	if got := unchanged["typed_map"].(map[string]any)["status"]; got != "ready" {
		t.Fatalf("typed map mutation leaked into state: %v", got)
	}
	if got := unchanged["nested"].([]any)[0].(map[string]any)["tags"].([]any)[0]; got != "one" {
		t.Fatalf("nested typed slice mutation leaked into state: %v", got)
	}
	if got := unchanged["downloads"].([]any)[0].(map[string]any)["filename"]; got != "report.pdf" {
		t.Fatalf("typed struct slice mutation leaked into state: %v", got)
	}
}
