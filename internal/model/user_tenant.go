package model

// UserTenant user tenant relationship model
type UserTenant struct {
	ID        string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	UserID    string  `gorm:"column:user_id;size:32;not null;index" json:"user_id"`
	TenantID  string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Role      string  `gorm:"column:role;size:32;not null;index" json:"role"`
	InvitedBy string  `gorm:"column:invited_by;size:32;not null;index" json:"invited_by"`
	Status    *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (UserTenant) TableName() string {
	return "user_tenant"
}
