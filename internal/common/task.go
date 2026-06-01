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
	TaskTypeIngestionTask    = "ingestion_task"
	TaskTypeIngestionTasklet = "ingestion_tasklet"
	TaskTypeIngestionTest    = "ingestion_test"
)

type TaskMessage struct {
	TaskID   string `json:"task_id" binding:"required"`
	TaskType string `json:"task_type" binding:"required"`
}

type TaskHandle interface {
	GetMessage() TaskMessage
	Ack() error
	Nack() error
}
