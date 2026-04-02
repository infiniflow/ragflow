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
	"ragflow/internal/entity"
)

// TimeRecordDAO time record data access object
type TimeRecordDAO struct{}

// NewTimeRecordDAO create TimeRecord DAO
func NewTimeRecordDAO() *TimeRecordDAO {
	return &TimeRecordDAO{}
}

// Create inserts a new record
func (dao *TimeRecordDAO) Create(record *entity.TimeRecord) error {
	return DB.Create(record).Error
}

// GetRecent retrieves the most recently inserted records (ordered by ID descending)
func (dao *TimeRecordDAO) GetRecent(limit int) ([]*entity.TimeRecord, error) {
	var records []*entity.TimeRecord
	err := DB.Order("id DESC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// GetCount returns the total number of records
func (dao *TimeRecordDAO) GetCount() (int64, error) {
	var count int64
	err := DB.Model(&entity.TimeRecord{}).Count(&count).Error
	return count, err
}

// DeleteOldest removes the oldest records (smallest ID) with limit
func (dao *TimeRecordDAO) DeleteOldest(limit int64) error {
	return DB.Exec("DELETE FROM time_records ORDER BY id ASC LIMIT ?", limit).Error
}

// GetByID retrieves a single record by its ID
func (dao *TimeRecordDAO) GetByID(id int64) (*entity.TimeRecord, error) {
	var record entity.TimeRecord
	err := DB.First(&record, id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// GetAll retrieves all records
func (dao *TimeRecordDAO) GetAll() ([]*entity.TimeRecord, error) {
	var records []*entity.TimeRecord
	err := DB.Find(&records).Error
	return records, err
}

// KeepLatest keeps the latest N records and deletes older ones
func (dao *TimeRecordDAO) KeepLatest(count int64) error {
	// Step 1: Get the maximum ID
	var maxID int64
	if err := DB.Model(&entity.TimeRecord{}).Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
		return err
	}

	// If no records or count is 0, nothing to delete
	if maxID == 0 || count <= 0 {
		return nil
	}

	// Step 2: Calculate the threshold ID
	thresholdID := maxID - count

	// If threshold is less than 0, keep all records
	if thresholdID <= 0 {
		return nil
	}

	// Step 3: Delete records with ID <= threshold
	return DB.Where("id <= ?", thresholdID).Delete(&entity.TimeRecord{}).Error
}

// DeleteAll deletes all records
func (dao *TimeRecordDAO) DeleteAll() error {
	return DB.Where("1=1").Delete(&entity.TimeRecord{}).Error
}
