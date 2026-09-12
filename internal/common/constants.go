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

package common

const (
	// PAGERANK_FLD is the field name for pagerank score
	PAGERANK_FLD = "pagerank_fea"
	// TAG_FLD is the field name for tag features
	TAG_FLD = "tag_feas"
	// MAX_RESULT_WINDOW is the maximum result window for ES
	MAX_RESULT_WINDOW = 10000
	// SearchAfterBatchSize caps how many hits one Elasticsearch
	// request can return per search_after iteration.
	SearchAfterBatchSize = 1000
)

// task status
const (
	CREATED   = "CREATED"
	SCHEDULED = "SCHEDULED"
	RUNNING   = "RUNNING"
	COMPLETED = "COMPLETED"
	FAILED    = "FAILED"
	STOPPED   = "STOPPED"
	STOPPING  = "STOPPING"
)

// ActiveTaskStatuses contains all in-flight, non-terminal task statuses.
var ActiveTaskStatuses = []string{CREATED, SCHEDULED, RUNNING, STOPPING}

// IsActiveTaskStatus returns true if the status represents an active (non-terminal) task:
// CREATED, SCHEDULED, RUNNING, or STOPPING.
func IsActiveTaskStatus(status string) bool {
	switch status {
	case CREATED, SCHEDULED, RUNNING, STOPPING:
		return true
	default:
		return false
	}
}

// IsTerminalTaskStatus returns true if the status represents a terminal task state:
// COMPLETED, STOPPED, or FAILED.
func IsTerminalTaskStatus(status string) bool {
	switch status {
	case COMPLETED, STOPPED, FAILED:
		return true
	default:
		return false
	}
}

// IsRunningOrStopping returns true if a worker is actively executing or shutting down:
// RUNNING or STOPPING.
func IsRunningOrStopping(status string) bool {
	return status == RUNNING || status == STOPPING
}

// StatusDialogValid is the dialog.status value that gates public bot
// access. Mirrors Python's StatusEnum.VALID.value at
// api/common/constants.py (the string "1"). All chatbot/agentbot
// authorization paths must use this constant instead of the literal.
const StatusDialogValid = "1"

// DialogStatus is a typed alias for dialog.status to avoid raw string
// comparisons in call sites.
type DialogStatus string
