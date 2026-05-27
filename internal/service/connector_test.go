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
)

func TestConnectorPruneEnabled(t *testing.T) {
	cases := []struct {
		name string
		conn *entity.Connector
		want bool
	}{
		{"nil connector", nil, false},
		{"nil config", &entity.Connector{}, false},
		{"flag absent", &entity.Connector{Config: entity.JSONMap{}}, false},
		{"flag false", &entity.Connector{Config: entity.JSONMap{"sync_deleted_files": false}}, false},
		{"flag true", &entity.Connector{Config: entity.JSONMap{"sync_deleted_files": true}}, true},
		{"flag non-bool", &entity.Connector{Config: entity.JSONMap{"sync_deleted_files": "yes"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectorPruneEnabled(tc.conn); got != tc.want {
				t.Fatalf("connectorPruneEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsCancelStatus(t *testing.T) {
	// TaskStatus.CANCEL == "2"; the Python API also accepts the literal "CANCEL".
	if !isCancelStatus(string(entity.TaskStatusCancel)) {
		t.Errorf("expected numeric CANCEL status to match")
	}
	if !isCancelStatus("CANCEL") {
		t.Errorf("expected literal CANCEL status to match")
	}
	if isCancelStatus(string(entity.TaskStatusSchedule)) {
		t.Errorf("did not expect SCHEDULE to match cancel")
	}
}

func TestIsScheduleStatus(t *testing.T) {
	// TaskStatus.SCHEDULE == "5"; the Python API also accepts the literal "SCHEDULE".
	if !isScheduleStatus(string(entity.TaskStatusSchedule)) {
		t.Errorf("expected numeric SCHEDULE status to match")
	}
	if !isScheduleStatus("SCHEDULE") {
		t.Errorf("expected literal SCHEDULE status to match")
	}
	if isScheduleStatus(string(entity.TaskStatusCancel)) {
		t.Errorf("did not expect CANCEL to match schedule")
	}
}
