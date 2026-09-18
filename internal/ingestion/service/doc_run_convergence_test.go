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

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// TestDocRunFromTasks_Empty covers the "no tasks" case: the doc is UNSTART.
func TestDocRunFromTasks_Empty(t *testing.T) {
	if got := docRunFromTasks(nil); got != string(entity.TaskStatusUnstart) {
		t.Fatalf("empty task set run = %q, want %q", got, entity.TaskStatusUnstart)
	}
}

// TestDocRunFromTasks_LatestWins covers the single-task mapping for every
// status. tasks are newest-first; the first element is authoritative.
func TestDocRunFromTasks_LatestWins(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   entity.TaskStatus
	}{
		{"created", common.CREATED, entity.TaskStatusRunning},
		{"running", common.RUNNING, entity.TaskStatusRunning},
		{"stopping", common.STOPPING, entity.TaskStatusRunning},
		{"failed", common.FAILED, entity.TaskStatusFail},
		{"stopped", common.STOPPED, entity.TaskStatusCancel},
		{"completed", common.COMPLETED, entity.TaskStatusDone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tasks := []*entity.IngestionTask{{ID: "task-1", DocumentID: "doc-1", Status: tc.status}}
			if got := docRunFromTasks(tasks); got != string(tc.want) {
				t.Fatalf("run = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDocRunFromTasks_HistoricalIgnored covers the multi-task case: only the
// newest task (first element) determines the run label; older terminal tasks
// from previous parse rounds must not affect it.
func TestDocRunFromTasks_HistoricalIgnored(t *testing.T) {
	cases := []struct {
		name    string
		latest  string
		history []string
		want    entity.TaskStatus
	}{
		{"newest-failed-over-old-completed", common.FAILED, []string{common.COMPLETED}, entity.TaskStatusFail},
		{"newest-completed-over-old-failed", common.COMPLETED, []string{common.FAILED, common.STOPPED}, entity.TaskStatusDone},
		{"newest-running-over-old-failed", common.RUNNING, []string{common.FAILED}, entity.TaskStatusRunning},
		{"newest-stopped-over-old-done", common.STOPPED, []string{common.COMPLETED}, entity.TaskStatusCancel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build newest-first: latest task first, then older history tasks.
			tasks := []*entity.IngestionTask{{ID: "task-latest", DocumentID: "doc-1", Status: tc.latest}}
			for i, st := range tc.history {
				tasks = append(tasks, &entity.IngestionTask{ID: string(rune('a' + i)), DocumentID: "doc-1", Status: st})
			}
			if got := docRunFromTasks(tasks); got != string(tc.want) {
				t.Fatalf("run = %q, want %q (latest=%q)", got, tc.want, tc.latest)
			}
		})
	}
}
