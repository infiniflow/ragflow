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

package entity

type IngestionTask struct {
	ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	UserID     string  `gorm:"column:user_id;size:32;not null" json:"user_id"`
	DocumentID string  `gorm:"column:document_id;size:32;not null;index" json:"document_id"`
	DatasetID  string  `gorm:"column:dataset_id;size:32;not null" json:"dataset_id"`
	Schema     JSONMap `gorm:"column:schema;type:longtext" json:"schema"`
	Status     string  `gorm:"column:status;size:32;not null;" json:"status"`
	// ComponentTotal is the number of components in the task's DSL graph.
	// It is the authoritative denominator for progress percentage so the
	// frontend does not have to count DSL nodes itself. Written once the
	// pipeline compiles the canvas (see pipeline.Run).
	ComponentTotal int `gorm:"column:component_total;default:0" json:"component_total"`
	BaseModel
}

// TableName specify table name
func (IngestionTask) TableName() string {
	return "ingestion_task"
}

type IngestionTaskLog struct {
	ID             int     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID         string  `gorm:"column:task_id;size:32;not null;index" json:"task_id"`
	Checkpoint     JSONMap `gorm:"column:checkpoint;type:longtext;not null" json:"checkpoint"`
	ComponentIndex int     `gorm:"column:component_index" json:"component_index"`
	Phase          int     `gorm:"column:phase" json:"phase"`
	Component      string  `gorm:"column:component;size:64;index" json:"component"`
	Message        string  `gorm:"column:message;type:text" json:"message"`
	BaseModel
}

// TableName specify table name
func (IngestionTaskLog) TableName() string {
	return "ingestion_task_log"
}
