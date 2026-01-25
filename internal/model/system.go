package model

// SystemSettings system settings model
type SystemSettings struct {
	Name     string `gorm:"column:name;primaryKey;size:128" json:"name"`
	Source   string `gorm:"column:source;size:32;not null" json:"source"`
	DataType string `gorm:"column:data_type;size:32;not null" json:"data_type"`
	Value    string `gorm:"column:value;size:1024;not null" json:"value"`
}

// TableName specify table name
func (SystemSettings) TableName() string {
	return "system_settings"
}
