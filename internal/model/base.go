package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// BaseModel base model
type BaseModel struct {
	CreateTime int64      `gorm:"column:create_time;index" json:"create_time"`
	CreateDate *time.Time `gorm:"column:create_date;index" json:"create_date,omitempty"`
	UpdateTime *int64     `gorm:"column:update_time;index" json:"update_time,omitempty"`
	UpdateDate *time.Time `gorm:"column:update_date;index" json:"update_date,omitempty"`
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
