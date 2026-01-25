package model

// Tenant tenant model
type Tenant struct {
	ID          string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	Name        *string `gorm:"column:name;size:100;index" json:"name,omitempty"`
	PublicKey   *string `gorm:"column:public_key;size:255;index" json:"public_key,omitempty"`
	LLMID       string  `gorm:"column:llm_id;size:128;not null;index" json:"llm_id"`
	EmbDID      string  `gorm:"column:embd_id;size:128;not null;index" json:"embd_id"`
	ASRID       string  `gorm:"column:asr_id;size:128;not null;index" json:"asr_id"`
	Img2TxtID  string  `gorm:"column:img2txt_id;size:128;not null;index" json:"img2txt_id"`
	RerankID   string  `gorm:"column:rerank_id;size:128;not null;index" json:"rerank_id"`
	TTSID       *string `gorm:"column:tts_id;size:256;index" json:"tts_id,omitempty"`
	ParserIDs  string  `gorm:"column:parser_ids;size:256;not null" json:"parser_ids"`
	Credit      int64   `gorm:"column:credit;default:512;index" json:"credit"`
	Status      *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Tenant) TableName() string {
	return "tenant"
}

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
