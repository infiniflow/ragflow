package model

// File file model
type File struct {
	ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	ParentID   string  `gorm:"column:parent_id;size:32;not null;index" json:"parent_id"`
	TenantID   string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	CreatedBy  string  `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	Name       string  `gorm:"column:name;size:255;not null;index" json:"name"`
	Location   *string `gorm:"column:location;size:255;index" json:"location,omitempty"`
	Size       int64   `gorm:"column:size;default:0;index" json:"size"`
	Type       string  `gorm:"column:type;size:32;not null;index" json:"type"`
	SourceType string  `gorm:"column:source_type;size:128;not null;default:'';index" json:"source_type"`
	BaseModel
}

// TableName specify table name
func (File) TableName() string {
	return "file"
}

// File2Document file to document mapping model
type File2Document struct {
	ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	FileID     *string `gorm:"column:file_id;size:32;index" json:"file_id,omitempty"`
	DocumentID *string `gorm:"column:document_id;size:32;index" json:"document_id,omitempty"`
	BaseModel
}

// TableName specify table name
func (File2Document) TableName() string {
	return "file2document"
}