package model

// Memory memory model
type Memory struct {
	ID               string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	Name             string  `gorm:"column:name;size:128;not null" json:"name"`
	Avatar           *string `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	TenantID         string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	MemoryType       int64   `gorm:"column:memory_type;default:1;index" json:"memory_type"`
	StorageType      string  `gorm:"column:storage_type;size:32;not null;default:table;index" json:"storage_type"`
	EmbdID           string  `gorm:"column:embd_id;size:128;not null" json:"embd_id"`
	LLMID            string  `gorm:"column:llm_id;size:128;not null" json:"llm_id"`
	Permissions      string  `gorm:"column:permissions;size:16;not null;default:me;index" json:"permissions"`
	Description      *string `gorm:"column:description;type:longtext" json:"description,omitempty"`
	MemorySize       int64   `gorm:"column:memory_size;default:5242880;not null" json:"memory_size"`
	ForgettingPolicy string  `gorm:"column:forgetting_policy;size:32;not null;default:FIFO" json:"forgetting_policy"`
	Temperature      float64 `gorm:"column:temperature;default:0.5;not null" json:"temperature"`
	SystemPrompt     *string `gorm:"column:system_prompt;type:longtext" json:"system_prompt,omitempty"`
	UserPrompt       *string `gorm:"column:user_prompt;type:longtext" json:"user_prompt,omitempty"`
	BaseModel
}

// TableName specify table name
func (Memory) TableName() string {
	return "memory"
}
