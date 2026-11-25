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
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/progressstore"
)

func TestPhaseLabelMapping(t *testing.T) {
	cases := []struct {
		phase uint8
		want  string
	}{
		{0, "running"},
		{1, "done"},
		{2, "failed"},
		{255, ""}, // out-of-range must not corrupt the stored phase
	}
	for _, tc := range cases {
		if got := phaseLabel(tc.phase); got != tc.want {
			t.Errorf("phaseLabel(%d) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

func TestProgressTerminalStatusMapping(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{string(entity.TaskStatusDone), progressstore.StatusCompleted},
		{string(entity.TaskStatusFail), progressstore.StatusFailed},
		{string(entity.TaskStatusCancel), progressstore.StatusStopped},
		{"UNKNOWN", ""}, // non-terminal must not flip the snapshot
	}
	for _, tc := range cases {
		if got := progressTerminalStatus(tc.status); got != tc.want {
			t.Errorf("progressTerminalStatus(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
