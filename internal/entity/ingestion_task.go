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
	DocumentID string  `gorm:"column:document_id;size:32;not null;index" json:"document_id"`
	UserID     string  `gorm:"column:user_id;size:32;not null;" json:"user_id"`
	Config     JSONMap `gorm:"column:config;type:longtext;not null" json:"config"`
	TryCount   int     `gorm:"column:try_count;type:int;default:0" json:"try_count"`
	BaseModel
}

// TableName specify table name
func (IngestionTask) TableName() string {
	return "ingestion_task"
}

type IngestionTasklet struct {
	ID       string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	TaskID   string  `gorm:"column:task_id;size:32;not null;index" json:"task_id"`
	Config   JSONMap `gorm:"column:config;type:longtext;not null" json:"config"`
	TryCount int     `gorm:"column:try_count;type:int;default:0" json:"try_count"`
	BaseModel
}

// TableName specify table name
func (IngestionTasklet) TableName() string {
	return "ingestion_tasklet"
}

type IngestionTaskLog struct {
	ID         int     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID     string  `gorm:"column:task_id;size:32;not null;index" json:"task_id"`
	Stage      int     `gorm:"column:stage;type:int;default:0;not null;" json:"stage"`
	DataSchema JSONMap `gorm:"column:config;type:longtext;not null" json:"data_schema"`
	BaseModel
}

// TableName specify table name
func (IngestionTaskLog) TableName() string {
	return "ingestion_task_log"
}

type IngestionTaskletLog struct {
	ID         int     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskletID  string  `gorm:"column:tasklet_id;size:32;not null;index" json:"task_id"`
	Stage      int     `gorm:"column:stage;type:int;default:0;not null;" json:"stage"`
	DataSchema JSONMap `gorm:"column:config;type:longtext;not null" json:"data_schema"`
	BaseModel
}

// TableName specify table name
func (IngestionTaskletLog) TableName() string {
	return "ingestion_tasklet_log"
}
