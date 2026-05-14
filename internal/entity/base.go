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

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// BaseModel base model
// All time fields are nullable to match Python Peewee model (null=True)
type BaseModel struct {
	CreateTime *int64     `gorm:"column:create_time;index" json:"create_time,omitempty"`
	CreateDate *time.Time `gorm:"column:create_date;index" json:"create_date,omitempty"`
	UpdateTime *int64     `gorm:"column:update_time;index" json:"update_time,omitempty"`
	UpdateDate *time.Time `gorm:"column:update_date;index" json:"update_date,omitempty"`
}

func autoModelTime() (int64, time.Time) {
	now := time.Now().Local()
	return now.UnixMilli(), now.Truncate(time.Second)
}

func statementHasTimeField(tx *gorm.DB, fieldNames ...string) bool {
	if tx == nil || tx.Statement == nil {
		return false
	}

	switch dest := tx.Statement.Dest.(type) {
	case map[string]interface{}:
		for _, fieldName := range fieldNames {
			if _, ok := dest[fieldName]; ok {
				return true
			}
		}
	case []map[string]interface{}:
		for _, item := range dest {
			for _, fieldName := range fieldNames {
				if _, ok := item[fieldName]; ok {
					return true
				}
			}
		}
	}

	return false
}

// BeforeCreate injects timestamps for models embedding BaseModel.
func (m *BaseModel) BeforeCreate(tx *gorm.DB) error {
	timestamp, dateTime := autoModelTime()

	if m.CreateTime == nil {
		m.CreateTime = &timestamp
	}
	if m.CreateDate == nil {
		m.CreateDate = &dateTime
	}
	if m.UpdateTime == nil {
		m.UpdateTime = &timestamp
	}
	if m.UpdateDate == nil {
		m.UpdateDate = &dateTime
	}

	if tx != nil && tx.Statement != nil {
		if !statementHasTimeField(tx, "create_time", "CreateTime") && m.CreateTime != nil {
			tx.Statement.SetColumn("CreateTime", *m.CreateTime)
		}
		if !statementHasTimeField(tx, "create_date", "CreateDate") && m.CreateDate != nil {
			tx.Statement.SetColumn("CreateDate", *m.CreateDate)
		}
		if !statementHasTimeField(tx, "update_time", "UpdateTime") && m.UpdateTime != nil {
			tx.Statement.SetColumn("UpdateTime", *m.UpdateTime)
		}
		if !statementHasTimeField(tx, "update_date", "UpdateDate") && m.UpdateDate != nil {
			tx.Statement.SetColumn("UpdateDate", *m.UpdateDate)
		}
	}
	return nil
}

// BeforeUpdate injects update timestamps for models embedding BaseModel.
func (m *BaseModel) BeforeUpdate(tx *gorm.DB) error {
	timestamp, dateTime := autoModelTime()

	if !statementHasTimeField(tx, "update_time", "UpdateTime") {
		m.UpdateTime = &timestamp
	}
	if !statementHasTimeField(tx, "update_date", "UpdateDate") {
		m.UpdateDate = &dateTime
	}

	if tx != nil && tx.Statement != nil {
		if !statementHasTimeField(tx, "update_time", "UpdateTime") && m.UpdateTime != nil {
			tx.Statement.SetColumn("UpdateTime", *m.UpdateTime)
		}
		if !statementHasTimeField(tx, "update_date", "UpdateDate") && m.UpdateDate != nil {
			tx.Statement.SetColumn("UpdateDate", *m.UpdateDate)
		}
	}
	return nil
}

// JSONMap is a map type that can store JSON data
type JSONMap map[string]interface{}

// Value implements driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal([]byte(value.(string)), j)
	}
	return json.Unmarshal(b, j)
}

// JSONSlice is a slice type that can store JSON array data
type JSONSlice []interface{}

// Value implements driver.Valuer interface
func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface
func (j *JSONSlice) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal([]byte(value.(string)), j)
	}
	return json.Unmarshal(b, j)
}
