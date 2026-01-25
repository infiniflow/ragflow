package model

// UserCanvas user canvas model
type UserCanvas struct {
	ID             string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	Avatar         *string  `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	UserID         string   `gorm:"column:user_id;size:255;not null;index" json:"user_id"`
	Title          *string  `gorm:"column:title;size:255" json:"title,omitempty"`
	Permission     string   `gorm:"column:permission;size:16;not null;default:me;index" json:"permission"`
	Description    *string  `gorm:"column:description;type:longtext" json:"description,omitempty"`
	CanvasType     *string  `gorm:"column:canvas_type;size:32;index" json:"canvas_type,omitempty"`
	CanvasCategory string   `gorm:"column:canvas_category;size:32;not null;default:agent_canvas;index" json:"canvas_category"`
	DSL            JSONMap  `gorm:"column:dsl;type:json" json:"dsl,omitempty"`
	BaseModel
}

// TableName specify table name
func (UserCanvas) TableName() string {
	return "user_canvas"
}

// CanvasTemplate canvas template model
type CanvasTemplate struct {
	ID             string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	Avatar         *string  `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	Title          JSONMap  `gorm:"column:title;type:json;default:'{}'" json:"title"`
	Description    JSONMap  `gorm:"column:description;type:json;default:'{}'" json:"description"`
	CanvasType     *string  `gorm:"column:canvas_type;size:32;index" json:"canvas_type,omitempty"`
	CanvasCategory string   `gorm:"column:canvas_category;size:32;not null;default:agent_canvas;index" json:"canvas_category"`
	DSL            JSONMap  `gorm:"column:dsl;type:json" json:"dsl,omitempty"`
	BaseModel
}

// TableName specify table name
func (CanvasTemplate) TableName() string {
	return "canvas_template"
}

// UserCanvasVersion user canvas version model
type UserCanvasVersion struct {
	ID           string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	UserCanvasID string   `gorm:"column:user_canvas_id;size:255;not null;index" json:"user_canvas_id"`
	Title        *string  `gorm:"column:title;size:255" json:"title,omitempty"`
	Description  *string  `gorm:"column:description;type:longtext" json:"description,omitempty"`
	DSL          JSONMap  `gorm:"column:dsl;type:json" json:"dsl,omitempty"`
	BaseModel
}

// TableName specify table name
func (UserCanvasVersion) TableName() string {
	return "user_canvas_version"
}