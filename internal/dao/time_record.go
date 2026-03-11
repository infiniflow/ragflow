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

package dao

import (
	"ragflow/internal/model"
)

// TimeRecordDAO time record data access object
type TimeRecordDAO struct{}

// NewTimeRecordDAO create TimeRecord DAO
func NewTimeRecordDAO() *TimeRecordDAO {
	return &TimeRecordDAO{}
}

// Create inserts a new record
func (dao *TimeRecordDAO) Create(record *model.TimeRecord) error {
	return DB.Create(record).Error
}

// GetRecent retrieves the most recently inserted records (ordered by ID descending)
func (dao *TimeRecordDAO) GetRecent(limit int) ([]*model.TimeRecord, error) {
	var records []*model.TimeRecord
	err := DB.Order("id DESC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// GetCount returns the total number of records
func (dao *TimeRecordDAO) GetCount() (int64, error) {
	var count int64
	err := DB.Model(&model.TimeRecord{}).Count(&count).Error
	return count, err
}

// DeleteOldest removes the oldest records (smallest ID) with limit
func (dao *TimeRecordDAO) DeleteOldest(limit int64) error {
	return DB.Exec("DELETE FROM time_records ORDER BY id ASC LIMIT ?", limit).Error
}

// GetByID retrieves a single record by its ID
func (dao *TimeRecordDAO) GetByID(id int64) (*model.TimeRecord, error) {
	var record model.TimeRecord
	err := DB.First(&record, id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}
